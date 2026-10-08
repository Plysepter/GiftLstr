package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	_ "modernc.org/sqlite"
)

// logInfo writes regular (non-error) logs to stdout.
var logInfo = log.New(os.Stdout, "", log.LstdFlags)

// newID returns a fresh UUID suitable for use as a users/lists/items key.
func newID() string {
	return uuid.NewString()
}

//go:embed schema.sql
var schemaFS embed.FS

//go:embed all:static
var staticFS embed.FS

//go:embed openapi.yaml
var openapiSpec []byte

var db *sql.DB

// indexHTML is the SPA shell with the runtime Authentik OIDC config baked in,
// rendered once at startup. nil when the frontend wasn't embedded.
var indexHTML []byte

// indexCSP is the Content-Security-Policy header for the served SPA shell,
// computed alongside indexHTML (it hashes the inline scripts it contains).
var indexCSP string

const maxBodyBytes = 1 << 20 // 1 MiB request body cap

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	issuer := os.Getenv("AUTHENTIK_ISSUER")

	if e2eTestMode() && issuer != "" {
		log.Fatal("E2E_TEST_MODE must not be enabled together with AUTHENTIK_ISSUER (it would bypass authentication)")
	}
	if e2eTestMode() {
		log.Print("WARNING: E2E_TEST_MODE is enabled; insecure test bearer tokens are accepted. Never enable in production.")
	} else if issuer == "" {
		log.Fatal("AUTHENTIK_ISSUER is required")
	}

	var err error
	db, err = sql.Open("sqlite", dataDir+"/giftlstr.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	// SQLite performs best with a single serialized connection; WAL +
	// busy_timeout handle the rest.
	db.SetMaxOpenConns(1)
	schema, _ := schemaFS.ReadFile("schema.sql")
	if _, err := db.Exec(string(schema)); err != nil {
		log.Fatalf("apply schema: %v", err)
	}
	// Migrations: add columns that may not exist in older databases.
	for _, m := range []string{
		`ALTER TABLE items ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public'`,
		`ALTER TABLE items ADD COLUMN suggested_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE items ADD COLUMN suggested_by_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE items ADD COLUMN created_by_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN holiday_reset TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN allow_suggestions INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE users ADD COLUMN spending_enabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE claims ADD COLUMN price_cents INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE lists ADD COLUMN is_private INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE lists ADD COLUMN first_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lists ADD COLUMN last_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE lists ADD COLUMN birthday TEXT NOT NULL DEFAULT ''`,
	} {
		db.Exec(m) // ignore "duplicate column" errors
	}
	// E2E / demo mode starts from the canonical seeded dataset on every boot so
	// the Playwright suite and the docker demo always see identical data.
	if e2eTestMode() {
		if err := applySeed(db); err != nil {
			log.Fatalf("apply e2e seed: %v", err)
		}
	}
	go archiveLoop()
	go backupLoop(context.Background(), backupConfigFromEnv(dataDir))

	// Render the SPA shell once at startup, overwriting any Authentik OIDC
	// config that may have been baked into the build with the one from the
	// runtime environment, so the image can be built without credentials.
	if html, ok := renderIndex(); ok {
		indexHTML = html
	} else {
		log.Print("static/index.html not embedded; SPA shell will not be served")
	}

	mux := http.NewServeMux()
	registerAPI(mux)
	mux.HandleFunc("GET /api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write(openapiSpec)
	})
	static, _ := fs.Sub(staticFS, "static")
	mux.HandleFunc("GET /manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		b, err := staticFS.ReadFile("static/manifest.webmanifest")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	})
	// The SPA shell (`/` and `/index.html`) is served from the indexHTML
	// rendered at startup; FileServerFS covers the remaining static assets.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })
	mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })
	mux.Handle("/", cacheStatic(http.FileServerFS(static)))
	mux.HandleFunc("/app/", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })
	// Endpoint Google returns to after SSO. The SPA reads the flow params on
	// this page and completes the session via completeOAuthSignIn().
	mux.HandleFunc("/signin-oauth", func(w http.ResponseWriter, r *http.Request) { serveIndex(w) })

	handler := secureHeaders(logMiddleware(mux))
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	logInfo.Printf("listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

// secureHeaders adds baseline security headers to every response.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000")
		}
		next.ServeHTTP(w, r)
	})
}

// cacheStatic sets long-lived immutable caching for hashed build assets and
// no-cache for the HTML shell.
func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case r.URL.Path == "/" || r.URL.Path == "/index.html":
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// authConfig returns the runtime Authentik OIDC settings embedded in the
// served page. Both values are public (the SPA needs them to start its OIDC
// flow) so they can be supplied per-deployment without requiring them at
// Docker build time.
func authConfig() (issuer, clientID string) {
	return os.Getenv("AUTHENTIK_ISSUER"), os.Getenv("AUTHENTIK_CLIENT_ID")
}

// The window.__GIFTLSTR__ script baked into the shell from an earlier render,
// whichever config it holds. `(?s)` so a wrapped/multiline script still
// matches; `.*?` is non-greedy so it stops at the config script's own closing
// </script>.
var authConfigRe = regexp.MustCompile(`(?s)<script>window\.__GIFTLSTR__.*?</script>`)

// escapeJS makes an env value safe to inline inside a JS string literal.
func escapeJS(s string) string {
	r := strings.NewReplacer("\\", "\\\\", `"`, `\"`, "\n", "\\n", "\r", "\\r", "<", "\\u003c", ">", "\\u003e", "&", "\\u0026")
	return r.Replace(s)
}

// authConfigScript returns the inline script that defines the runtime OIDC
// config for the SPA.
func authConfigScript(issuer, clientID string) string {
	authE2e := ""
	if e2eTestMode() {
		// Expose e2e mode to the signed-out Login page so it can offer the
		// seeded demo accounts as one-click sign-in.
		authE2e = `, authE2e: true`
	}
	return `<script>window.__GIFTLSTR__ = { authIssuer: "` + escapeJS(issuer) + `", authClientId: "` + escapeJS(clientID) + `"` + authE2e + ` };</script>`
}

// inlineScriptRe matches a plain (attribute-free) inline script block; these
// are the only inline scripts the SPA shell contains.
var inlineScriptRe = regexp.MustCompile(`(?s)<script\s*>([\s\S]*?)</script>`)

// scriptHash returns the base64 SHA-256 of an inline script block's contents,
// in the form CSP `'sha256-…'` sources take, or "" when the block holds no
// inline script.
func scriptHash(block string) string {
	m := inlineScriptRe.FindStringSubmatch(block)
	if m == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(m[1]))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// contentSecurityPolicy returns the CSP header value for the SPA shell.
// Everything loads from the same origin except the theme bootstrap and the
// runtime OIDC config, which are inline scripts allowed by hash, and images,
// which may come from any https source (item tiles embed third-party previews
// and favicons).
func contentSecurityPolicy(themeHash, configHash string) string {
	scripts := "script-src 'self'"
	for _, h := range []string{themeHash, configHash} {
		if h != "" {
			scripts += " 'sha256-" + h + "'"
		}
	}
	return "default-src 'self'; " + scripts +
		"; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:;" +
		" connect-src 'self'; worker-src 'self'; manifest-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"
}

// setAuthConfig replaces any previously set window.__GIFTLSTR__ config script
// in the SPA shell with the given runtime Auth settings, then inserts it
// before the first module script so it is defined before the app bundle runs.
func setAuthConfig(html, issuer, clientID string) string {
	html = authConfigRe.ReplaceAllString(html, "")
	return strings.Replace(html, "<script type=\"module\"", authConfigScript(issuer, clientID)+"<script type=\"module\"", 1)
}

// renderIndex loads the embedded SPA shell and bakes the runtime Authentik
// OIDC config into it, deriving the CSP that matches the inline scripts it
// contains. Called once at startup so the config is resolved from the
// deployment environment, not the build.
func renderIndex() ([]byte, bool) {
	b, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		return nil, false
	}
	issuer, clientID := authConfig()
	cfg := authConfigScript(issuer, clientID)
	indexCSP = contentSecurityPolicy(scriptHash(string(b)), scriptHash(cfg))
	return []byte(setAuthConfig(string(b), issuer, clientID)), true
}

// serveIndex serves the SPA HTML shell rendered at startup.
func serveIndex(w http.ResponseWriter) {
	if indexHTML == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if indexCSP != "" {
		w.Header().Set("Content-Security-Policy", indexCSP)
	}
	w.Write(indexHTML)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logInfo.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// ---------- auth ----------

type user struct {
	ID               string `json:"id"`
	AuthUserID       string `json:"-"`
	Username         string `json:"username"`
	DisplayName      string `json:"displayName"`
	IsAdmin          bool   `json:"isAdmin"`
	Birthday         string `json:"birthday"`
	HolidayReset     string `json:"holidayReset"`
	AllowSuggestions bool   `json:"allowSuggestions"`
	SpendingEnabled  bool   `json:"spendingEnabled"`
	ThemeStyle       string `json:"themeStyle"`
	ShareCode        string `json:"shareCode"`
	IsNew            bool   `json:"isNew,omitempty"`
}

func scanUser(row *sql.Row) (*user, error) {
	var u user
	var theme sql.NullString
	err := row.Scan(&u.ID, &u.AuthUserID, &u.Username, &u.DisplayName, &u.IsAdmin, &u.Birthday, &u.HolidayReset, &u.AllowSuggestions, &u.SpendingEnabled, &theme, &u.ShareCode)
	if err != nil {
		return nil, err
	}
	if theme.Valid {
		u.ThemeStyle = theme.String
	} else {
		u.ThemeStyle = "aurora"
	}
	return &u, nil
}

const userCols = `id, auth_user_id, username, display_name, is_admin, birthday, holiday_reset, allow_suggestions, spending_enabled, theme_style, share_code`

func getSessionToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// adminFromEnv reports whether the external identity (Authentik `sub`) is
// listed in ADMIN_USER_IDS.
func adminFromEnv(subject string) bool {
	admins := os.Getenv("ADMIN_USER_IDS")
	if admins == "" {
		return false
	}
	for _, id := range strings.Split(admins, ",") {
		if strings.TrimSpace(id) == subject {
			return true
		}
	}
	return false
}

// insertUserAndList creates the user row plus its active list inside one
// transaction. Falls back to an auth-id-suffixed username if the derived
// username collides.
func insertUserAndList(authID, username, displayName string, isAdmin bool, birthday string) (*user, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	uid := newID()
	insert := func(name string) (sql.Result, error) {
		return tx.Exec(`INSERT INTO users (id, auth_user_id, username, display_name, is_admin, birthday, theme_style, share_code)
			VALUES (?, ?, ?, ?, ?, ?, 'aurora', '')
			ON CONFLICT(auth_user_id) DO NOTHING`,
			uid, authID, name, displayName, isAdmin, birthday)
	}
	res, err := insert(username)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		u, err := getUserByAuthID(authID)
		if err != nil {
			return nil, err
		}
		return u, tx.Commit()
	}
	if _, err := tx.Exec(`INSERT INTO lists (id, user_id, label) VALUES (?, ?, 'Active')`, newID(), uid); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &user{ID: uid, AuthUserID: authID, Username: username, DisplayName: displayName, IsAdmin: isAdmin, Birthday: birthday, HolidayReset: "", AllowSuggestions: true, SpendingEnabled: false, ThemeStyle: "aurora", ShareCode: "", IsNew: true}, nil
}

// provisionUser creates the local user row and active list on first login.
func provisionUser(id *Identity) (*user, error) {
	// Reset dates are set up during account creation: the birthday comes from
	// the identity provider's metadata (MM-DD); Christmas is fixed at 12-25.
	// Admin rights come exclusively from ADMIN_USER_IDS so that provisioning
	// can never accidentally escalate a user.
	return insertUserAndList(id.Subject, id.Username, id.DisplayName, adminFromEnv(id.Subject), normalizeMMDD(id.Birthday))
}

func getUserByAuthID(authID string) (*user, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE auth_user_id = ?`, authID))
}

// e2eUser provisions/loads a local user from an "e2e:<username>:<display name>"
// bearer token. Only enabled when E2E_TEST_MODE=1 (which main() refuses to
// combine with real Authentik credentials), so automated end-to-end tests can
// run without an identity provider.
func e2eUser(token string) (*user, error) {
	parts := strings.SplitN(strings.TrimPrefix(token, "e2e:"), ":", 3)
	if len(parts) < 2 || parts[0] == "" {
		return nil, errors.New("invalid e2e token")
	}
	username, displayName := parts[0], parts[1]
	u, err := getUserByUsername(username)
	if errors.Is(err, sql.ErrNoRows) {
		// First e2e user becomes admin to exercise admin flows in tests.
		isAdmin := false
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
		isAdmin = n == 0
		return insertUserAndList("e2e:"+username, username, displayName, isAdmin, "")
	}
	return u, err
}

func getUserByUsername(username string) (*user, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE username = ?`, username))
}

func currentUser(r *http.Request) (*user, error) {
	token := getSessionToken(r)
	if token == "" {
		return nil, errors.New("no session token")
	}
	if e2eTestMode() && strings.HasPrefix(token, "e2e:") {
		return e2eUser(token)
	}
	ident, err := activeAuth.VerifySession(r.Context(), token)
	if err != nil {
		log.Printf("auth rejected session token: %v", err)
		return nil, err
	}

	u, err := getUserByAuthID(ident.Subject)
	if errors.Is(err, sql.ErrNoRows) {
		return provisionUser(ident)
	}
	if err != nil {
		return nil, err
	}
	// Keep admin flags in sync with ADMIN_USER_IDS even after initial
	// provisioning.
	if !u.IsAdmin && adminFromEnv(ident.Subject) {
		if _, err := db.Exec(`UPDATE users SET is_admin = 1 WHERE id = ?`, u.ID); err == nil {
			u.IsAdmin = true
		}
	}
	return u, nil
}

// canView reports whether viewer may see owner's list. Connections are
// treated as bidirectional: a single row in either direction suffices.
func canView(viewerID string, viewerAdmin bool, ownerID string) bool {
	if viewerID == ownerID || viewerAdmin {
		return true
	}
	var ok bool
	db.QueryRow(`SELECT EXISTS(SELECT 1 FROM connections WHERE (user_id = ? AND other_user_id = ?) OR (user_id = ? AND other_user_id = ?))`, viewerID, ownerID, ownerID, viewerID).Scan(&ok)
	return ok
}

// activeListID returns the user's ordinary (non-private) active list,
// creating it if missing. Private lists never qualify: each one is addressed
// explicitly by id, so they must not hijack the implicit "active list".
func activeListID(userID string) (string, error) {
	var id string
	err := db.QueryRow(`SELECT id FROM lists WHERE user_id = ? AND is_private = 0 AND archived_at IS NULL ORDER BY id DESC LIMIT 1`, userID).Scan(&id)
	if err == sql.ErrNoRows {
		if _, err2 := db.Exec(`INSERT INTO lists (id, user_id, label) VALUES (?, ?, 'Active')`, newID(), userID); err2 != nil {
			return "", err2
		}
		return id, db.QueryRow(`SELECT id FROM lists WHERE user_id = ? AND is_private = 0 AND archived_at IS NULL ORDER BY id DESC LIMIT 1`, userID).Scan(&id)
	}
	return id, err
}

// ---------- reset dates / archiving ----------

// Reset kinds a user may be configured with. "mothers" (Mother's Day, the
// second Sunday of May) and "fathers" (Father's Day, the third Sunday of
// June) are optional holiday resets whose calendar date varies by year.
const (
	resetChristmas = "christmas"
	resetBirthday  = "birthday"
	resetMothers   = "mothers"
	resetFathers   = "fathers"
)

// validHolidayReset reports whether s is a supported holiday reset identifier.
func validHolidayReset(s string) bool {
	return s == "" || s == resetMothers || s == resetFathers
}

// lastResetOccurrence returns the most recent occurrence of a yearly MM-DD
// date relative to now.
func lastResetOccurrence(mmdd string, now time.Time) time.Time {
	year := now.Year()
	t := parseMMDD(mmdd, year)
	if t.After(now) {
		t = parseMMDD(mmdd, year-1)
	}
	return t
}

func parseMMDD(mmdd string, year int) time.Time {
	parts := strings.SplitN(mmdd, "-", 2)
	m, _ := strconv.Atoi(parts[0])
	d, _ := strconv.Atoi(parts[1])
	return time.Date(year, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// nthSundayOf returns the nth Sunday of the given month (1 = first Sunday).
func nthSundayOf(year int, month time.Month, nth int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (7 - int(first.Weekday())) % 7 // days from the 1st to its first Sunday
	return time.Date(year, month, 1+offset+(nth-1)*7, 0, 0, 0, 0, time.UTC)
}

// holidayDate returns the calendar date of the named holiday for the given
// year, or ok=false for an unknown holiday.
func holidayDate(kind string, year int) (time.Time, bool) {
	switch kind {
	case resetMothers:
		return nthSundayOf(year, time.May, 2), true
	case resetFathers:
		return nthSundayOf(year, time.June, 3), true
	}
	return time.Time{}, false
}

// holidayOccurrence returns the most recent occurrence of the named holiday
// relative to now (this year if it has passed, otherwise last year).
func holidayOccurrence(kind string, now time.Time) (time.Time, bool) {
	var best time.Time
	found := false
	// The holiday date varies by year, so evaluate both candidate years and
	// keep the latest one that is not in the future.
	for _, year := range []int{now.Year(), now.Year() - 1} {
		if d, ok := holidayDate(kind, year); ok && !d.After(now) && (!found || d.After(best)) {
			best = d
			found = true
		}
	}
	return best, found
}

// userResetDates returns the user's configured yearly reset kinds in a stable
// discovery order: Christmas, then birthday, then the optional holiday reset.
func userResetDates(u *user) []string {
	kinds := []string{resetChristmas}
	if validMMDD(u.Birthday) {
		kinds = append(kinds, resetBirthday)
	}
	if validHolidayReset(u.HolidayReset) && u.HolidayReset != "" {
		kinds = append(kinds, u.HolidayReset)
	}
	return kinds
}

// lastResetDate returns the most recent occurrence of the given reset kind
// relative to now, or ok=false when the user has no such reset configured.
func lastResetDate(u *user, kind string, now time.Time) (time.Time, bool) {
	switch kind {
	case resetChristmas:
		return lastResetOccurrence("12-25", now), true
	case resetBirthday:
		if !validMMDD(u.Birthday) {
			return time.Time{}, false
		}
		return lastResetOccurrence(u.Birthday, now), true
	case resetMothers, resetFathers:
		return holidayOccurrence(kind, now)
	}
	return time.Time{}, false
}

// resetLabel returns the human-readable archive label for a reset occurrence.
func resetLabel(kind string, reset time.Time) string {
	switch kind {
	case resetChristmas:
		return fmt.Sprintf("%d Christmas", reset.Year())
	case resetMothers:
		return fmt.Sprintf("%d Mother's Day", reset.Year())
	case resetFathers:
		return fmt.Sprintf("%d Father's Day", reset.Year())
	default:
		return fmt.Sprintf("%d Birthday", reset.Year())
	}
}

// archiveUserList archives the active list if a reset date passed more than a
// week ago and the active list predates that cutoff. Unclaimed items are
// copied to the new active list.
func archiveUserList(u *user, now time.Time) error {
	listID, err := activeListID(u.ID)
	if err != nil {
		return err
	}
	var createdStr string
	var label string
	if err := db.QueryRow(`SELECT created_at, label FROM lists WHERE id = ?`, listID).Scan(&createdStr, &label); err != nil {
		return err
	}
	for _, kind := range userResetDates(u) {
		reset, ok := lastResetDate(u, kind, now)
		if !ok {
			continue
		}
		// Cutoff is 8 days (not 7) after the reset date so the archive never
		// happens a day early for users ahead of UTC: a birthday on 07-01 local
		// time can be 06-30 UTC for users far ahead, and the extra day gives
		// every timezone a full, unambiguous buffer before the list rolls over.
		cutoff := reset.AddDate(0, 0, 8)
		if !now.After(cutoff) {
			continue
		}
		created, err := time.Parse("2006-01-02 15:04:05", createdStr)
		if err != nil || !created.Before(cutoff) {
			continue // already handled by an earlier archive
		}
		lbl := resetLabel(kind, reset)
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE lists SET archived_at = datetime('now'), label = ? WHERE id = ? AND archived_at IS NULL`, lbl, listID); err != nil {
			return err
		}
		newListID := newID()
		if _, err := tx.Exec(`INSERT INTO lists (id, user_id, label) VALUES (?, ?, 'Active')`, newListID, u.ID); err != nil {
			return err
		}
		// Carry over items to the new list. Each gets a fresh UUID key.
		//   - Unclaimed public items    -> copy as public
		//   - Claimed public items      -> stay in archive (not copied)
		//   - Claimed private items     -> revealed to owner: copy as public
		//   - Unclaimed private items   -> roll over: copy as private
		//   - Claimed suggested items   -> revealed to owner: copy as public
		//   - Unclaimed suggested items -> dropped (not rolled over)
		rows, err := tx.Query(`SELECT i.id, i.title, i.url, i.notes, i.sort_order, i.visibility, i.created_by_id,
				EXISTS(SELECT 1 FROM claims c WHERE c.item_id = i.id) AS claimed
			FROM items i WHERE i.list_id = ? AND i.removed_at IS NULL`, listID)
		if err != nil {
			return err
		}
		type fieldVal struct {
			title, url, notes, visibility, createdBy string
			sort                                     int
		}
		var carry []fieldVal
		for rows.Next() {
			var id, title, url, notes, visibility, createdBy string
			var sort int
			var claimed bool
			if rows.Scan(&id, &title, &url, &notes, &sort, &visibility, &createdBy, &claimed) != nil {
				continue
			}
			switch visibility {
			case "private", "suggested":
				// Unclaimed private items roll over (stay private, still tagged
				// with whoever saved them); claimed private/suggested items are
				// revealed as public. Unclaimed suggested items are dropped.
				if !claimed && visibility == "private" {
					carry = append(carry, fieldVal{title, url, notes, "private", createdBy, sort})
				} else if claimed {
					carry = append(carry, fieldVal{title, url, notes, "public", "", sort})
				}
			default: // public
				if !claimed {
					carry = append(carry, fieldVal{title, url, notes, visibility, "", sort})
				}
			}
		}
		rows.Close()
		for _, f := range carry {
			if _, err := tx.Exec(`INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id, created_by_id) VALUES (?, ?, ?, ?, ?, ?, ?, '', '', ?)`,
				newID(), newListID, f.title, f.url, f.notes, f.sort, f.visibility, f.createdBy); err != nil {
				return err
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		break
	}
	return nil
}

func archiveLoop() {
	runAll := func() {
		rows, err := db.Query(`SELECT ` + userCols + ` FROM users`)
		if err != nil {
			return
		}
		defer rows.Close()
		var us []*user
		for rows.Next() {
			var u user
			var theme sql.NullString
			if rows.Scan(&u.ID, &u.AuthUserID, &u.Username, &u.DisplayName, &u.IsAdmin, &u.Birthday, &u.HolidayReset, &u.AllowSuggestions, &theme, &u.ShareCode) == nil {
				if theme.Valid {
					u.ThemeStyle = theme.String
				} else {
					u.ThemeStyle = "aurora"
				}
				us = append(us, &u)
			}
		}
		now := time.Now().UTC()
		for _, u := range us {
			if err := archiveUserList(u, now); err != nil {
				log.Printf("archive user %s: %v", u.Username, err)
			}
		}
	}
	runAll()
	t := time.NewTicker(6 * time.Hour)
	for range t.C {
		runAll()
	}
}
