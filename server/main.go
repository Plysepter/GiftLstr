package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/google/uuid"

	_ "modernc.org/sqlite"
)

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

var jwksClient *jwks.Client

const maxBodyBytes = 1 << 20 // 1 MiB request body cap

func main() {
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	secretKey := os.Getenv("CLERK_SECRET_KEY")

	if e2eTestMode() && secretKey != "" {
		log.Fatal("E2E_TEST_MODE must not be enabled together with CLERK_SECRET_KEY (it would bypass authentication)")
	}
	if e2eTestMode() {
		log.Print("WARNING: E2E_TEST_MODE is enabled; insecure test bearer tokens are accepted. Never enable in production.")
	} else if secretKey == "" {
		log.Fatal("CLERK_SECRET_KEY is required")
	}
	if !e2eTestMode() {
		clerk.SetKey(secretKey)
		config := &clerk.ClientConfig{}
		config.Key = clerk.String(secretKey)
		jwksClient = jwks.NewClient(config)
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
	migrate()
	go archiveLoop()
	go backupLoop(context.Background(), backupConfigFromEnv(dataDir))

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
	log.Printf("listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

// columnExists reports whether a table has the given column.
func columnExists(table, column string) bool {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err == nil && name == column {
			return true
		}
	}
	return false
}

// migrate applies incremental migrations for databases created before Clerk
// integration (schema.sql only runs CREATE IF NOT EXISTS).
func migrate() {
	if !columnExists("users", "clerk_user_id") {
		if _, err := db.Exec(`ALTER TABLE users ADD COLUMN clerk_user_id TEXT`); err != nil {
			log.Fatalf("migration: %v", err)
		}
	}
	if !columnExists("users", "theme_style") {
		if _, err := db.Exec(`ALTER TABLE users ADD COLUMN theme_style TEXT NOT NULL DEFAULT 'kinetic'`); err != nil {
			log.Fatalf("migration: %v", err)
		}
	}
	if !columnExists("users", "share_code") {
		if _, err := db.Exec(`ALTER TABLE users ADD COLUMN share_code TEXT NOT NULL DEFAULT ''`); err != nil {
			log.Fatalf("migration: %v", err)
		}
	}
	migrateUUID()
}

// columnType returns the declared type of a column, or "".
func columnType(table, column string) string {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err == nil && name == column {
			return strings.ToUpper(typ)
		}
	}
	return ""
}

// migrateUUID converts databases created with INTEGER primary keys to UUID
// (TEXT) keys. Each source row gets a fresh UUID; foreign keys are remapped in
// lockstep so claims/connections/lists keep pointing at the right rows.
func migrateUUID() {
	if columnType("users", "id") != "INTEGER" {
		return
	}
	log.Println("migrating integer ids to UUIDs")
	// The _uuid child tables carry FOREIGN KEY clauses that, until the old
	// integer tables are renamed away, still point at the INTEGER parents. We
	// rebuild every table in lockstep here, so referential integrity is
	// guaranteed by the remap maps; disable FK enforcement for the migration
	// (and re-enable it right after) to avoid transient violations.
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		log.Fatalf("uuid migration: %v", err)
	}
	defer func() {
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			log.Fatalf("uuid migration: %v", err)
		}
		log.Println("uuid migration complete")
	}()
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("uuid migration: %v", err)
	}
	defer tx.Rollback()

	for _, drop := range []string{`DROP TABLE IF EXISTS users_uuid`, `DROP TABLE IF EXISTS lists_uuid`, `DROP TABLE IF EXISTS items_uuid`, `DROP TABLE IF EXISTS claims_uuid`, `DROP TABLE IF EXISTS connections_uuid`} {
		if _, err := tx.Exec(drop); err != nil {
			log.Fatalf("uuid migration: %v", err)
		}
	}
	creates := []string{
		`CREATE TABLE users_uuid (
			id TEXT PRIMARY KEY, clerk_user_id TEXT NOT NULL UNIQUE, username TEXT NOT NULL,
			display_name TEXT NOT NULL, is_admin INTEGER NOT NULL DEFAULT 0, birthday TEXT NOT NULL DEFAULT '',
			theme_style TEXT NOT NULL DEFAULT 'kinetic', share_code TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE lists_uuid (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			label TEXT NOT NULL, archived_at TEXT, created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE items_uuid (
			id TEXT PRIMARY KEY, list_id TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
			title TEXT NOT NULL, url TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0, removed_at TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE claims_uuid (
			item_id TEXT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			claimed_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE connections_uuid (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			other_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			PRIMARY KEY (user_id, other_user_id), CHECK (user_id <> other_user_id)
		)`,
	}
	for _, c := range creates {
		if _, err := tx.Exec(c); err != nil {
			log.Fatalf("uuid migration: %v", err)
		}
	}

	// users: oldID -> newUUID
	userMap := map[int64]string{}
	if rows, err := tx.Query(`SELECT id, clerk_user_id, username, display_name, is_admin, birthday, theme_style, share_code, created_at FROM users`); err != nil {
		log.Fatalf("uuid migration: %v", err)
	} else {
		for rows.Next() {
			var id int64
			var clerkID, username, displayName, birthday, theme, code, created string
			var adm int
			rows.Scan(&id, &clerkID, &username, &displayName, &adm, &birthday, &theme, &code, &created)
			nid := newID()
			userMap[id] = nid
			if _, err := tx.Exec(`INSERT INTO users_uuid (id, clerk_user_id, username, display_name, is_admin, birthday, theme_style, share_code, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, nid, clerkID, username, displayName, adm, birthday, theme, code, created); err != nil {
				log.Fatalf("uuid migration: %v", err)
			}
		}
		rows.Close()
	}

	// lists: oldID -> newUUID, plus oldUserID -> new
	listMap := map[int64]string{}
	if rows, err := tx.Query(`SELECT id, user_id, label, archived_at, created_at FROM lists`); err != nil {
		log.Fatalf("uuid migration: %v", err)
	} else {
		for rows.Next() {
			var id, owner int64
			var label, archived, created string
			rows.Scan(&id, &owner, &label, &archived, &created)
			nid := newID()
			listMap[id] = nid
			if _, err := tx.Exec(`INSERT INTO lists_uuid (id, user_id, label, archived_at, created_at)
				VALUES (?, ?, ?, ?, ?)`, nid, userMap[owner], label, archived, created); err != nil {
				log.Fatalf("uuid migration: %v", err)
			}
		}
		rows.Close()
	}

	// items: oldID -> newUUID (remaps list_id)
	itemMap := map[int64]string{}
	if rows, err := tx.Query(`SELECT id, list_id, title, url, notes, sort_order, created_at, removed_at FROM items`); err != nil {
		log.Fatalf("uuid migration: %v", err)
	} else {
		for rows.Next() {
			var id, list int64
			var title, url, notes string
			var sort int
			var created, removed string
			rows.Scan(&id, &list, &title, &url, &notes, &sort, &created, &removed)
			nid := newID()
			itemMap[id] = nid
			if _, err := tx.Exec(`INSERT INTO items_uuid (id, list_id, title, url, notes, sort_order, created_at, removed_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, nid, listMap[list], title, url, notes, sort, created, removed); err != nil {
				log.Fatalf("uuid migration: %v", err)
			}
		}
		rows.Close()
	}

	// claims remaps item_id and user_id
	if rows, err := tx.Query(`SELECT item_id, user_id, claimed_at FROM claims`); err != nil {
		log.Fatalf("uuid migration: %v", err)
	} else {
		for rows.Next() {
			var item, claimant int64
			var claimed string
			rows.Scan(&item, &claimant, &claimed)
			if _, err := tx.Exec(`INSERT INTO claims_uuid (item_id, user_id, claimed_at) VALUES (?, ?, ?)`,
				itemMap[item], userMap[claimant], claimed); err != nil {
				log.Fatalf("uuid migration: %v", err)
			}
		}
		rows.Close()
	}

	// connections remap both sides
	if rows, err := tx.Query(`SELECT user_id, other_user_id FROM connections`); err != nil {
		log.Fatalf("uuid migration: %v", err)
	} else {
		for rows.Next() {
			var a, b int64
			rows.Scan(&a, &b)
			if _, err := tx.Exec(`INSERT INTO connections_uuid (user_id, other_user_id) VALUES (?, ?)`,
				userMap[a], userMap[b]); err != nil {
				log.Fatalf("uuid migration: %v", err)
			}
		}
		rows.Close()
	}

	for _, drop := range []string{`DROP TABLE claims`, `DROP TABLE connections`, `DROP TABLE items`, `DROP TABLE lists`, `DROP TABLE users`} {
		if _, err := tx.Exec(drop); err != nil {
			log.Fatalf("uuid migration: %v", err)
		}
	}
	for _, rename := range []string{`ALTER TABLE users_uuid RENAME TO users`, `ALTER TABLE lists_uuid RENAME TO lists`, `ALTER TABLE items_uuid RENAME TO items`, `ALTER TABLE claims_uuid RENAME TO claims`, `ALTER TABLE connections_uuid RENAME TO connections`} {
		if _, err := tx.Exec(rename); err != nil {
			log.Fatalf("uuid migration: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		log.Fatalf("uuid migration: %v", err)
	}
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

func serveIndex(w http.ResponseWriter) {
	b, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// ---------- auth ----------

type user struct {
	ID          string `json:"id"`
	ClerkUserID string `json:"-"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	IsAdmin     bool   `json:"isAdmin"`
	Birthday    string `json:"birthday"`
	ThemeStyle  string `json:"themeStyle"`
	ShareCode   string `json:"shareCode"`
	IsNew       bool   `json:"isNew,omitempty"`
}

func scanUser(row *sql.Row) (*user, error) {
	var u user
	var theme sql.NullString
	err := row.Scan(&u.ID, &u.ClerkUserID, &u.Username, &u.DisplayName, &u.IsAdmin, &u.Birthday, &theme, &u.ShareCode)
	if err != nil {
		return nil, err
	}
	if theme.Valid {
		u.ThemeStyle = theme.String
	} else {
		u.ThemeStyle = "kinetic"
	}
	return &u, nil
}

const userCols = `id, clerk_user_id, username, display_name, is_admin, birthday, theme_style, share_code`

func getSessionToken(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// adminFromEnv reports whether the Clerk id is listed in ADMIN_USER_IDS.
func adminFromEnv(clerkID string) bool {
	admins := os.Getenv("ADMIN_USER_IDS")
	if admins == "" {
		return false
	}
	for _, id := range strings.Split(admins, ",") {
		if strings.TrimSpace(id) == clerkID {
			return true
		}
	}
	return false
}

// insertUserAndList creates the user row plus its active list inside one
// transaction. Falls back to a clerk-id-suffixed username if the derived
// username collides.
func insertUserAndList(clerkID, username, displayName string, isAdmin bool, birthday string) (*user, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	uid := newID()
	insert := func(name string) (sql.Result, error) {
		return tx.Exec(`INSERT INTO users (id, clerk_user_id, username, display_name, is_admin, birthday, theme_style, share_code)
			VALUES (?, ?, ?, ?, ?, ?, 'kinetic', '')
			ON CONFLICT(clerk_user_id) DO NOTHING`,
			uid, clerkID, name, displayName, isAdmin, birthday)
	}
	res, err := insert(username)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		u, err := getUserByClerkID(clerkID)
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
	return &user{ID: uid, ClerkUserID: clerkID, Username: username, DisplayName: displayName, IsAdmin: isAdmin, Birthday: birthday, ThemeStyle: "kinetic", ShareCode: "", IsNew: true}, nil
}

// provisionUser creates the local user row and active list on first login.
func provisionUser(externalID string) (*user, error) {
	ctx := context.Background()
	username, displayName, birthday, err := activeAuth.ProvisionInfo(ctx, externalID)
	if err != nil {
		return nil, err
	}
	// Reset dates are set up during account creation: the birthday comes from
	// the identity provider's metadata (MM-DD); Christmas is fixed at 12-25.
	// Admin rights come exclusively from ADMIN_USER_IDS so that provisioning
	// can never accidentally escalate a user.
	return insertUserAndList(externalID, username, displayName, adminFromEnv(externalID), normalizeMMDD(birthday))
}

func getUserByClerkID(clerkID string) (*user, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE clerk_user_id = ?`, clerkID))
}

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// e2eUser provisions/loads a local user from an "e2e:<username>:<display name>"
// bearer token. Only enabled when E2E_TEST_MODE=1 (which main() refuses to
// combine with real Clerk credentials), so automated end-to-end tests can run
// without Clerk.
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
	externalID, err := activeAuth.VerifySession(r.Context(), token)
	if err != nil {
		return nil, err
	}

	u, err := getUserByClerkID(externalID)
	if errors.Is(err, sql.ErrNoRows) {
		return provisionUser(externalID)
	}
	if err != nil {
		return nil, err
	}
	// Keep admin flags in sync with ADMIN_USER_IDS even after initial
	// provisioning.
	if !u.IsAdmin && adminFromEnv(externalID) {
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

func activeListID(userID string) (string, error) {
	var id string
	err := db.QueryRow(`SELECT id FROM lists WHERE user_id = ? AND archived_at IS NULL ORDER BY id DESC LIMIT 1`, userID).Scan(&id)
	if err == sql.ErrNoRows {
		if _, err2 := db.Exec(`INSERT INTO lists (id, user_id, label) VALUES (?, ?, 'Active')`, newID(), userID); err2 != nil {
			return "", err2
		}
		return id, db.QueryRow(`SELECT id FROM lists WHERE user_id = ? AND archived_at IS NULL LIMIT 1`, userID).Scan(&id)
	}
	return id, err
}

// ---------- reset dates / archiving ----------

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

// resetLabel returns the human-readable archive label for a reset occurrence.
func resetLabel(reset time.Time, mmdd string) string {
	if mmdd == "12-25" {
		return fmt.Sprintf("Christmas %d", reset.Year())
	}
	return fmt.Sprintf("Birthday %d", reset.Year())
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
	for _, mmdd := range []string{"12-25", u.Birthday} {
		if !validMMDD(mmdd) {
			continue
		}
		reset := lastResetOccurrence(mmdd, now)
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
		lbl := resetLabel(reset, mmdd)
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
		// Carry over unclaimed, non-removed items individually so each gets a
		// fresh UUID key.
		rows, err := tx.Query(`SELECT title, url, notes, sort_order FROM items i
			WHERE i.list_id = ? AND i.removed_at IS NULL AND i.id NOT IN (SELECT item_id FROM claims)`, listID)
		if err != nil {
			return err
		}
		type fieldVal struct {
			title, url, notes string
			sort              int
		}
		var carry []fieldVal
		for rows.Next() {
			var f fieldVal
			if rows.Scan(&f.title, &f.url, &f.notes, &f.sort) == nil {
				carry = append(carry, f)
			}
		}
		rows.Close()
		for _, f := range carry {
			if _, err := tx.Exec(`INSERT INTO items (id, list_id, title, url, notes, sort_order) VALUES (?, ?, ?, ?, ?, ?)`,
				newID(), newListID, f.title, f.url, f.notes, f.sort); err != nil {
				return err
			}
		}
		if err != nil {
			return err
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
			if rows.Scan(&u.ID, &u.ClerkUserID, &u.Username, &u.DisplayName, &u.IsAdmin, &u.Birthday, &theme, &u.ShareCode) == nil {
				if theme.Valid {
					u.ThemeStyle = theme.String
				} else {
					u.ThemeStyle = "kinetic"
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
