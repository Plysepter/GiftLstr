package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// decode parses the JSON request body, enforcing a size cap. It passes the
// real ResponseWriter to MaxBytesReader so an oversized body closes the
// connection rather than silently truncating.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func logExec(what string, err error) error {
	if err != nil {
		log.Printf("%s: %v", what, err)
	}
	return err
}

func pathID(r *http.Request, name string) (string, error) {
	v := r.PathValue(name)
	if v == "" {
		return "", errors.New("invalid id")
	}
	return v, nil
}

func registerAPI(mux *http.ServeMux) {
	rl := newRateLimiter()
	mux.HandleFunc("GET /api/me", rl.rateLimit(auth(handleMe)))
	mux.HandleFunc("PATCH /api/profile", rl.rateLimit(auth(handleProfile)))

	mux.HandleFunc("GET /api/lists", rl.rateLimit(auth(handleLists)))
	mux.HandleFunc("GET /api/users/{id}/list", rl.rateLimit(auth(handleUserList)))
	mux.HandleFunc("GET /api/metadata", rl.rateLimitStrict(auth(handleMetadata)))

	// OIDC proxy: lets the SPA reach Authentik's discovery + token endpoints
	// through the app's own origin (no CORS handling in Authentik needed).
	// Not auth-protected: they're the public OIDC endpoints, rate-limited only.
	mux.HandleFunc("GET /api/oidc/discovery", rl.rateLimit(handleOIDCDiscovery))
	mux.HandleFunc("POST /api/oidc/token", rl.rateLimit(handleOIDCToken))

	mux.HandleFunc("POST /api/my/items", rl.rateLimit(auth(handleAddItem)))
	mux.HandleFunc("PATCH /api/my/items/{id}", rl.rateLimit(auth(handleEditItem)))
	mux.HandleFunc("DELETE /api/my/items/{id}", rl.rateLimit(auth(handleRemoveItem)))
	mux.HandleFunc("POST /api/my/items/{id}/restore", rl.rateLimit(auth(handleRestoreItem)))
	mux.HandleFunc("PUT /api/my/items/order", rl.rateLimit(auth(handleReorderItems)))

	mux.HandleFunc("POST /api/my/private-items", rl.rateLimit(auth(handleAddPrivateItem)))
	mux.HandleFunc("GET /api/my/private-items", rl.rateLimit(auth(handleGetPrivateItems)))
	mux.HandleFunc("DELETE /api/my/private-items/{id}", rl.rateLimit(auth(handleDeletePrivateItem)))
	mux.HandleFunc("PUT /api/my/private-items/{id}/share", rl.rateLimit(auth(handleSharePrivateItem)))
	mux.HandleFunc("DELETE /api/suggested-items/{id}", rl.rateLimit(auth(handleDeleteSuggestedItem)))

	mux.HandleFunc("PUT /api/items/{id}/claim", rl.rateLimit(auth(handleClaim)))
	mux.HandleFunc("DELETE /api/items/{id}/claim", rl.rateLimit(auth(handleUnclaim)))

	mux.HandleFunc("GET /api/archive", rl.rateLimit(auth(handleArchiveIndex)))
	mux.HandleFunc("GET /api/archive/{id}", rl.rateLimit(auth(handleArchiveList)))

	mux.HandleFunc("GET /api/admin/users", rl.rateLimit(admin(handleAdminUsers)))
	mux.HandleFunc("PATCH /api/admin/users/{id}", rl.rateLimit(admin(handleAdminUpdateUser)))
	mux.HandleFunc("DELETE /api/admin/users/{id}", rl.rateLimit(admin(handleAdminDeleteUser)))
	mux.HandleFunc("POST /api/admin/users/{id}/archive", rl.rateLimit(admin(handleAdminForceArchive)))
	mux.HandleFunc("DELETE /api/admin/users/{id}/list", rl.rateLimit(admin(handleAdminHardDelete)))
	mux.HandleFunc("PUT /api/connections", rl.rateLimit(admin(handleSetConnection)))
	mux.HandleFunc("PUT /api/connect/share", rl.rateLimit(auth(handleSetShareEnabled)))
	mux.HandleFunc("POST /api/connect/accept", rl.rateLimitStrict(auth(handleAcceptShareCode)))

	if e2eTestMode() {
		mux.HandleFunc("POST /api/e2e/archive", rl.rateLimit(auth(handleE2EArchive)))
	}
}

func e2eTestMode() bool {
	return os.Getenv("E2E_TEST_MODE") == "1" && os.Getenv("AUTHENTIK_ISSUER") == ""
}

// handleE2EArchive force-runs the archive/reset flow for the current user as if
// it were the given date. Only available when E2E_TEST_MODE=1.
func handleE2EArchive(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Date string `json:"date"`
	}
	if err := decode(w, r, &body); err != nil || body.Date == "" {
		fail(w, http.StatusBadRequest, "date required")
		return
	}
	now, err := time.Parse("2006-01-02", body.Date)
	if err != nil {
		fail(w, http.StatusBadRequest, "invalid date")
		return
	}
	u := userFrom(r)
	if u.Birthday == "" {
		fail(w, http.StatusBadRequest, "user has no birthday set")
		return
	}
	now = now.Add(23 * time.Hour)
	listID, err := activeListID(u.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Backdate the active list so it predates the reset cutoff, letting the
	// normal archive flow trigger as if the list had existed since then.
	if _, err := db.Exec(`UPDATE lists SET created_at = datetime(?) WHERE id = ? AND archived_at IS NULL`,
		now.AddDate(-1, 0, 0).UTC().Format("2006-01-02 15:04:05"), listID); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := archiveUserList(u, now.UTC()); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// currentUserFn resolves the authenticated user for a request; overridable in
// tests to bypass identity-provider verification.
var currentUserFn = currentUser

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := currentUserFn(r)
		if err != nil {
			fail(w, http.StatusUnauthorized, "not logged in")
			return
		}
		next(w, r.WithContext(contextWithUser(r, u)))
	}
}

func admin(next http.HandlerFunc) http.HandlerFunc {
	return auth(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r)
		if !u.IsAdmin {
			fail(w, http.StatusForbidden, "admin only")
			return
		}
		next(w, r)
	})
}

// ---------- session handlers ----------

type meResponse struct {
	User         *user  `json:"user"`
	ActiveListID string `json:"activeListId"`
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	listID, err := activeListID(u.ID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	writeJSON(w, 200, meResponse{User: u, ActiveListID: listID})
}

func validMMDD(s string) bool {
	if len(s) != 5 || s[2] != '-' {
		return false
	}
	t, err := time.Parse("01-02", s)
	return err == nil && t.Format("01-02") == s
}

// normalizeMMDD accepts common birthday formats ("05-15", "5-15", "5/15",
// "05/15", "2026-05-15") and returns canonical "MM-DD", or "" if unparseable.
func normalizeMMDD(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"01-02", "1-02", "01-2", "1-2", "01/02", "1/02", "01/2", "1/2", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("01-02")
		}
	}
	return ""
}

var validThemeStyles = map[string]bool{
	"aurora": true, "aurora-warm": true, "kinetic": true,
	"ocean": true, "forest": true, "berry": true,
	"gold": true, "ruby": true, "teal": true,
	"dino": true, "dogs": true,
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req struct {
		Birthday   *string `json:"birthday"` // MM-DD
		ThemeStyle *string `json:"themeStyle"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, "bad request")
		return
	}
	if req.Birthday == nil && req.ThemeStyle == nil {
		fail(w, 400, "birthday or themeStyle required")
		return
	}
	if req.Birthday != nil {
		birthday := normalizeMMDD(*req.Birthday)
		if birthday == "" {
			fail(w, 400, "valid birthday required, e.g. 05-15")
			return
		}
		if _, err := db.Exec(`UPDATE users SET birthday = ? WHERE id = ?`, birthday, u.ID); err != nil {
			log.Printf("update birthday: %v", err)
			fail(w, 500, "update failed")
			return
		}
	}
	if req.ThemeStyle != nil {
		ts := strings.TrimSpace(*req.ThemeStyle)
		if !validThemeStyles[ts] {
			fail(w, 400, "invalid themeStyle")
			return
		}
		if _, err := db.Exec(`UPDATE users SET theme_style = ? WHERE id = ?`, ts, u.ID); err != nil {
			log.Printf("update theme_style: %v", err)
			fail(w, 500, "update failed")
			return
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- lists ----------

type listItem struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Notes         string  `json:"notes"`
	RemovedAt     *string `json:"removedAt"`
	Claimed       bool    `json:"claimed"`
	ClaimedByMe   bool    `json:"claimedByMe"`
	ClaimedByName *string `json:"claimedByName,omitempty"`
	ClaimedUserID string  `json:"claimedUserId,omitempty"`
	SuggestedBy   string  `json:"suggestedBy,omitempty"`
}

func itemRows(where string, args ...any) ([]listItem, error) {
	q := `SELECT i.id, i.title, i.url, i.notes, i.removed_at, c.user_id AS claim_user_id, cu.display_name AS claim_name, i.suggested_by
		FROM items i
		LEFT JOIN claims c ON c.item_id = i.id
		LEFT JOIN users cu ON cu.id = c.user_id
		WHERE ` + where + ` ORDER BY i.sort_order, i.id`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []listItem{}
	for rows.Next() {
		var it listItem
		var removed, claimName, claimUserID sql.NullString
		if err := rows.Scan(&it.ID, &it.Title, &it.URL, &it.Notes, &removed, &claimUserID, &claimName, &it.SuggestedBy); err != nil {
			continue
		}
		if removed.Valid {
			s := removed.String
			it.RemovedAt = &s
		}
		if claimUserID.Valid {
			it.Claimed = true
			it.ClaimedUserID = claimUserID.String
			if claimName.Valid {
				s := claimName.String
				it.ClaimedByName = &s
			}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

type listOwner struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Birthday    string `json:"birthday"`
	ThemeStyle  string `json:"themeStyle"`
}

type userListResponse struct {
	User         listOwner  `json:"user"`
	Items        []listItem `json:"items"`
	PrivateItems []listItem `json:"privateItems,omitempty"`
	IsOwner      bool       `json:"isOwner"`
}

func handleUserList(w http.ResponseWriter, r *http.Request) {
	viewer := userFrom(r)
	ownerID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	if !canView(viewer.ID, viewer.IsAdmin, ownerID) {
		fail(w, 403, "no access to this list")
		return
	}
	var owner listOwner
	if err := db.QueryRow(`SELECT id, username, display_name, birthday, theme_style FROM users WHERE id = ?`, ownerID).
		Scan(&owner.ID, &owner.Username, &owner.DisplayName, &owner.Birthday, &owner.ThemeStyle); err != nil {
		fail(w, 404, "user not found")
		return
	}
	if owner.ThemeStyle == "" {
		owner.ThemeStyle = "aurora"
	}
	listID, err := activeListID(ownerID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	isOwner := viewer.ID == ownerID

	// Both the owner and connected users see public + suggested items. The
	// owner sees their shared ideas in a "Suggested" section so they can
	// manage them; private items are returned separately, owner-only.
	items, err := itemRows("i.list_id = ? AND i.visibility IN ('public', 'suggested')", listID)
	if err != nil {
		fail(w, 500, "query error")
		return
	}

	// Owner's private items returned in a separate field.
	var privateItems []listItem
	if isOwner {
		privateItems, err = itemRows("i.list_id = ? AND i.visibility = 'private'", listID)
		if err != nil {
			fail(w, 500, "query error")
			return
		}
	}

	// Admins may opt into seeing who claimed items on a live list they are
	// viewing; everyone else (and admins by default) keeps claimers hidden.
	revealClaimers := viewer.IsAdmin && r.URL.Query().Get("revealClaimers") == "1"
	for i := range items {
		it := &items[i]
		if !it.Claimed {
			continue
		}
		if it.ClaimedUserID == viewer.ID {
			it.ClaimedByMe = true
			it.ClaimedByName = nil // viewer knows it's them
		} else if ownerID != viewer.ID && !revealClaimers {
			it.ClaimedByName = nil // hide other claimers on live lists
		}
		it.ClaimedUserID = ""
	}
	// Same treatment for private items (owner view).
	for i := range privateItems {
		it := &privateItems[i]
		if !it.Claimed {
			continue
		}
		if it.ClaimedUserID == viewer.ID {
			it.ClaimedByMe = true
			it.ClaimedByName = nil
		}
		it.ClaimedUserID = ""
	}
	writeJSON(w, 200, userListResponse{User: owner, Items: items, PrivateItems: privateItems, IsOwner: isOwner})
}

type listEntry struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	Username     string `json:"username"`
	Birthday     string `json:"birthday"`
	MyClaims     int    `json:"myClaims"`
	Connected    bool   `json:"connected"`
	CanView      bool   `json:"canView"`
	ClaimedLabel string `json:"claimedLabel"`
}

type listsResponse struct {
	Users   []listEntry `json:"users"`
	Page    int         `json:"page"`
	HasMore bool        `json:"hasMore"`
	Total   int         `json:"total"`
}

func handleLists(w http.ResponseWriter, r *http.Request) {
	viewer := userFrom(r)
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const per = 20

	var total int
	if viewer.IsAdmin {
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE id != ?`, viewer.ID).Scan(&total)
	} else {
		db.QueryRow(`SELECT COUNT(*) FROM users u WHERE u.id != ? AND EXISTS(SELECT 1 FROM connections c WHERE (c.user_id = ? AND c.other_user_id = u.id) OR (c.user_id = u.id AND c.other_user_id = ?))`, viewer.ID, viewer.ID, viewer.ID).Scan(&total)
	}

	offset := (page - 1) * per
	var rows *sql.Rows
	var err error
	if viewer.IsAdmin {
		rows, err = db.Query(`
			SELECT u.id, u.display_name, u.username, u.birthday,
				(SELECT COUNT(*) FROM claims c JOIN items i ON i.id = c.item_id JOIN lists l ON l.id = i.list_id
				 WHERE l.user_id = u.id AND l.archived_at IS NULL AND c.user_id = ?) AS my_claims,
				(SELECT COUNT(*) FROM connections conn WHERE (conn.user_id = ? AND conn.other_user_id = u.id) OR (conn.user_id = u.id AND conn.other_user_id = ?))
			FROM users u WHERE u.id != ? ORDER BY u.display_name LIMIT ? OFFSET ?`,
			viewer.ID, viewer.ID, viewer.ID, viewer.ID, per, offset)
	} else {
		rows, err = db.Query(`
			SELECT u.id, u.display_name, u.username, u.birthday,
				(SELECT COUNT(*) FROM claims c JOIN items i ON i.id = c.item_id JOIN lists l ON l.id = i.list_id
				 WHERE l.user_id = u.id AND l.archived_at IS NULL AND c.user_id = ?) AS my_claims,
				(SELECT COUNT(*) FROM connections conn WHERE (conn.user_id = ? AND conn.other_user_id = u.id) OR (conn.user_id = u.id AND conn.other_user_id = ?))
			FROM users u WHERE u.id != ? AND EXISTS(SELECT 1 FROM connections c2 WHERE (c2.user_id = ? AND c2.other_user_id = u.id) OR (c2.user_id = u.id AND c2.other_user_id = ?)) ORDER BY u.display_name LIMIT ? OFFSET ?`,
			viewer.ID, viewer.ID, viewer.ID, viewer.ID, viewer.ID, viewer.ID, per, offset)
	}
	if err != nil {
		fail(w, 500, "query error")
		return
	}
	defer rows.Close()
	users := []listEntry{}
	for rows.Next() {
		var e listEntry
		var claims, connected int
		if err := rows.Scan(&e.ID, &e.DisplayName, &e.Username, &e.Birthday, &claims, &connected); err != nil {
			continue
		}
		e.Connected = connected > 0
		e.MyClaims = claims
		e.CanView = viewer.IsAdmin || e.Connected
		if claims > 0 {
			e.ClaimedLabel = "claimed " + strconv.Itoa(claims) + " items"
		}
		users = append(users, e)
	}
	writeJSON(w, 200, listsResponse{Users: users, Page: page, HasMore: page*per < total, Total: total})
}

// ---------- items (own list) ----------

// validItemURL accepts empty (no link) or an absolute http(s) URL. Relative
// or scheme-less values ("google.com", "<a href=...>") are rejected so stored
// links are always directly openable.
func validItemURL(s string) bool {
	if s == "" {
		return true
	}
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// isPublicIP reports whether an IP is a publicly routable unicast address,
// rejecting loopback, private, link-local, and unspecified addresses.
func isPublicIP(ip net.IP) bool {
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

// resolvePublicAddrs resolves a host and returns only the public IP addresses
// it maps to. If the host is a literal IP it must be public to pass. An empty
// result means the host has no public address and must be rejected.
func resolvePublicAddrs(ctx context.Context, host string) ([]net.IPAddr, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, nil
		}
		return []net.IPAddr{{IP: ip}}, nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	public := make([]net.IPAddr, 0, len(ips))
	for _, a := range ips {
		if isPublicIP(a.IP) {
			public = append(public, a)
		}
	}
	return public, nil
}

// isPublicHost resolves the URL's host and rejects anything that points at
// private, loopback, link-local, or unspecified addresses (SSRF guard). The
// authoritative check happens at connect time in the hardened transport, but
// this pre-check rejects obvious non-public literals/targets cheaply before we
// spend a request on them.
func isPublicHost(ctx context.Context, u *url.URL) bool {
	host := u.Hostname()
	if host == "" {
		return false
	}
	public, err := resolvePublicAddrs(ctx, host)
	return err == nil && len(public) > 0
}

// ssrfDialer is a net.Dialer whose DialContext re-resolves the host in a way
// that is atomic with the connect: it validates every address the resolver
// returns (against isPublicIP) and dials only public addresses. This closes
// the classic DNS-rebinding TOCTOU where a pre-flight isPublicHost lookup and
// the later connect resolve to different addresses.
type ssrfDialer struct {
	base net.Dialer
}

func (d ssrfDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	public, err := resolvePublicAddrs(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(public) == 0 {
		return nil, fmt.Errorf("host %s resolves to no public address", host)
	}
	var lastErr error
	for _, a := range public {
		// Dial by the validated numeric IP so the connection cannot be
		// redirected elsewhere between the check and the connect.
		conn, err := d.base.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// fetchClient is used for metadata lookups: bounded time and response size,
// and an SSRF-safe transport that only ever connects to public IPs.
var fetchClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DialContext:     ssrfDialer{}.DialContext,
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	},
}

type pageMetadata struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

var metaTagRe = regexp.MustCompile(
	`(?is)<meta[^>]+(?:property|name)=["'](og:title|og:description|og:image|twitter:title|twitter:description|twitter:image|description)["'][^>]*>`)

var metaContentRe = regexp.MustCompile(`(?is)content\s*=\s*["'](.*?)["']`)
var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

const metadataTTL = 30 * 24 * time.Hour

func fetchPageMetadata(ctx context.Context, raw string) (pageMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return pageMetadata{}, err
	}
	// Look like a browser: many sites reject unknown crawlers outright, and
	// social metadata is exactly what browsers get served.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-CA,en-US;q=0.9,en;q=0.8")
	resp, err := fetchClient.Do(req)
	if err != nil {
		return pageMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return pageMetadata{}, fmt.Errorf("fetch failed with status %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(strings.ToLower(ct), "text/html") {
		return pageMetadata{}, fmt.Errorf("url does not return html")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return pageMetadata{}, err
	}
	doc := string(body)
	meta := pageMetadata{URL: raw}
	for _, m := range metaTagRe.FindAllStringSubmatch(doc, -1) {
		c := metaContentRe.FindStringSubmatch(m[0])
		if c == nil {
			continue
		}
		val := strings.TrimSpace(html.UnescapeString(c[1]))
		switch m[1] {
		case "og:title", "twitter:title":
			if meta.Title == "" {
				meta.Title = val
			}
		case "og:description", "twitter:description", "description":
			if meta.Description == "" {
				meta.Description = val
			}
		case "og:image", "twitter:image":
			if meta.Image == "" && validItemURL(val) {
				meta.Image = val
			}
		}
	}
	if ms := titleRe.FindStringSubmatch(doc); ms != nil && meta.Title == "" {
		meta.Title = strings.TrimSpace(html.UnescapeString(ms[1]))
	}
	return meta, nil
}

// handleMetadata fetches Open Graph metadata for a public http(s) URL so the
// client can render a link preview card. Results are cached server-side with
// a 30-day TTL; archived lists viewed again after expiry trigger a refetch
// (TTL reset) and stale cached results are served as fallback on fetch failure
// so images degrade gracefully.
func handleMetadata(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if !validItemURL(raw) {
		fail(w, 400, "valid http(s) url required")
		return
	}
	u, _ := url.Parse(raw)
	if !isPublicHost(r.Context(), u) {
		fail(w, 400, "host is not reachable")
		return
	}

	// Check server-side cache first.
	var cachedTitle, cachedDesc, cachedImage, cachedAt string
	cachedErr := db.QueryRow(`SELECT title, description, image, fetched_at FROM metadata_cache WHERE url = ?`, raw).Scan(&cachedTitle, &cachedDesc, &cachedImage, &cachedAt)
	cachedExists := cachedErr == nil
	if cachedExists {
		if t, err := time.Parse("2006-01-02 15:04:05", cachedAt); err == nil && time.Since(t) < metadataTTL {
			writeJSON(w, 200, pageMetadata{URL: raw, Title: cachedTitle, Description: cachedDesc, Image: cachedImage})
			return
		}
	}

	meta, err := fetchPageMetadata(r.Context(), raw)
	if err != nil {
		if cachedExists {
			// Serve stale cache rather than failing — the image URL may still
			// render even if the host now blocks us.
			writeJSON(w, 200, pageMetadata{URL: raw, Title: cachedTitle, Description: cachedDesc, Image: cachedImage})
			return
		}
		// Distinguish non-HTML vs generic fetch errors for the client.
		if strings.Contains(err.Error(), "does not return html") {
			fail(w, 415, err.Error())
		} else {
			fail(w, 502, err.Error())
		}
		return
	}

	// Upsert cache (refresh TTL).
	if _, err := db.Exec(`INSERT INTO metadata_cache (url, title, description, image, fetched_at) VALUES (?, ?, ?, ?, datetime('now'))
		ON CONFLICT(url) DO UPDATE SET title=excluded.title, description=excluded.description, image=excluded.image, fetched_at=excluded.fetched_at`,
		raw, meta.Title, meta.Description, meta.Image); err != nil {
		log.Printf("metadata cache upsert: %v", err)
	}

	writeJSON(w, 200, meta)
}

// maxListItems caps how many active (non-removed) items a list may hold so
// list reads and reorder requests never grow unbounded.
const maxListItems = 200

func activeItemCount(listID string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM items WHERE list_id = ? AND removed_at IS NULL`, listID).Scan(&n)
	return n, err
}

func atItemLimit(listID string) (bool, error) {
	n, err := activeItemCount(listID)
	return n >= maxListItems, err
}

func handleAddItem(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Notes string `json:"notes"`
	}
	if err := decode(w, r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		fail(w, 400, "title required")
		return
	}
	itemURL := strings.TrimSpace(req.URL)
	if !validItemURL(itemURL) {
		fail(w, 400, "url must be an absolute http(s) link")
		return
	}
	listID, err := activeListID(u.ID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	atLimit, err := atItemLimit(listID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	if atLimit {
		fail(w, 400, fmt.Sprintf("list is full (max %d items)", maxListItems))
		return
	}
	tx, err := db.Begin()
	if err != nil {
		fail(w, 500, "insert failed")
		return
	}
	defer tx.Rollback()
	id := newID()
	if _, err := tx.Exec(
		`INSERT INTO items (id, list_id, title, url, notes, sort_order) VALUES (?, ?, ?, ?, ?, COALESCE((SELECT MAX(sort_order)+1 FROM items WHERE list_id = ?), 0))`,
		id, listID, strings.TrimSpace(req.Title), itemURL, strings.TrimSpace(req.Notes), listID,
	); err != nil {
		fail(w, 500, "insert failed")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "insert failed")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

func ownItem(u *user, itemID string) bool {
	var listID string
	err := db.QueryRow(`SELECT i.list_id FROM items i JOIN lists l ON l.id = i.list_id WHERE i.id = ? AND l.user_id = ? AND l.archived_at IS NULL AND i.removed_at IS NULL`, itemID, u.ID).Scan(&listID)
	return err == nil
}

func handleEditItem(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	itemID, err := pathID(r, "id")
	if err != nil || !ownItem(u, itemID) {
		fail(w, 404, "item not found")
		return
	}
	var req struct {
		Title *string `json:"title"`
		URL   *string `json:"url"`
		Notes *string `json:"notes"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, "bad request")
		return
	}
	sets := []string{}
	args := []any{}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			fail(w, 400, "title cannot be empty")
			return
		}
		sets = append(sets, "title = ?")
		args = append(args, title)
	}
	if req.URL != nil {
		itemURL := strings.TrimSpace(*req.URL)
		if !validItemURL(itemURL) {
			fail(w, 400, "url must be an absolute http(s) link")
			return
		}
		sets = append(sets, "url = ?")
		args = append(args, itemURL)
	}
	if req.Notes != nil {
		sets = append(sets, "notes = ?")
		args = append(args, strings.TrimSpace(*req.Notes))
	}
	if len(sets) > 0 {
		args = append(args, itemID)
		if _, err := db.Exec(`UPDATE items SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			log.Printf("edit item: %v", err)
			fail(w, 500, "update failed")
			return
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleRemoveItem(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	itemID, err := pathID(r, "id")
	if err != nil || !ownItem(u, itemID) {
		fail(w, 404, "item not found")
		return
	}
	if _, err := db.Exec(`UPDATE items SET removed_at = datetime('now') WHERE id = ? AND removed_at IS NULL`, itemID); err != nil {
		log.Printf("remove item: %v", err)
		fail(w, 500, "remove failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ownRemovedItem reports whether the item belongs to the user's active list
// and is currently soft-deleted.
func ownRemovedItem(u *user, itemID string) bool {
	var listID string
	err := db.QueryRow(`SELECT i.list_id FROM items i JOIN lists l ON l.id = i.list_id WHERE i.id = ? AND l.user_id = ? AND l.archived_at IS NULL AND i.removed_at IS NOT NULL`, itemID, u.ID).Scan(&listID)
	return err == nil
}

func handleRestoreItem(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	itemID, err := pathID(r, "id")
	if err != nil || !ownRemovedItem(u, itemID) {
		fail(w, 404, "item not found")
		return
	}
	var listID string
	if err := db.QueryRow(`SELECT list_id FROM items WHERE id = ?`, itemID).Scan(&listID); err != nil {
		fail(w, 404, "item not found")
		return
	}
	atLimit, err := atItemLimit(listID)
	if err != nil {
		fail(w, 500, "restore failed")
		return
	}
	if atLimit {
		fail(w, 400, fmt.Sprintf("list is full (max %d items)", maxListItems))
		return
	}
	if _, err := db.Exec(`UPDATE items SET removed_at = NULL WHERE id = ? AND removed_at IS NOT NULL`, itemID); err != nil {
		log.Printf("restore item: %v", err)
		fail(w, 500, "restore failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleReorderItems assigns sequential sort orders to the given item ids,
// which must all belong to the caller's active list.
func handleReorderItems(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := decode(w, r, &req); err != nil || len(req.IDs) == 0 {
		fail(w, 400, "ids required")
		return
	}
	listID, err := activeListID(u.ID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		fail(w, 500, "reorder failed")
		return
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`UPDATE items SET sort_order = ? WHERE id = ? AND list_id = ?`)
	if err != nil {
		fail(w, 500, "reorder failed")
		return
	}
	defer stmt.Close()
	for i, id := range req.IDs {
		res, err := stmt.Exec(i, id, listID)
		if err != nil {
			log.Printf("reorder item: %v", err)
			fail(w, 500, "reorder failed")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			fail(w, 404, fmt.Sprintf("item %s not found on your list", id))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "reorder failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- private / suggested items ----------

// ownPrivateItem reports whether the item belongs to the user's active list
// and is currently a private (unshared) item.
func ownPrivateItem(u *user, itemID string) bool {
	var listID string
	err := db.QueryRow(`SELECT i.list_id FROM items i JOIN lists l ON l.id = i.list_id WHERE i.id = ? AND l.user_id = ? AND l.archived_at IS NULL AND i.visibility = 'private'`, itemID, u.ID).Scan(&listID)
	return err == nil
}

// handleAddPrivateItem adds a private (owner-only) idea to the user's active
// list. Input matches the main gift-idea composer.
func handleAddPrivateItem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Notes string `json:"notes"`
	}
	if err := decode(w, r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		fail(w, 400, "title required")
		return
	}
	itemURL := strings.TrimSpace(req.URL)
	if !validItemURL(itemURL) {
		fail(w, 400, "url must be an absolute http(s) link")
		return
	}
	u := userFrom(r)
	listID, err := activeListID(u.ID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	atLimit, err := atItemLimit(listID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	if atLimit {
		fail(w, 400, fmt.Sprintf("list is full (max %d items)", maxListItems))
		return
	}
	id := newID()
	if _, err := db.Exec(
		`INSERT INTO items (id, list_id, title, url, notes, sort_order, visibility) VALUES (?, ?, ?, ?, ?, COALESCE((SELECT MAX(sort_order)+1 FROM items WHERE list_id = ?), 0), 'private')`,
		id, listID, strings.TrimSpace(req.Title), itemURL, strings.TrimSpace(req.Notes), listID,
	); err != nil {
		log.Printf("add private item: %v", err)
		fail(w, 500, "insert failed")
		return
	}
	writeJSON(w, 201, map[string]any{"id": id})
}

// handleGetPrivateItems returns the caller's private ideas.
func handleGetPrivateItems(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	listID, err := activeListID(u.ID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	items, err := itemRows("i.list_id = ? AND i.visibility = 'private'", listID)
	if err != nil {
		fail(w, 500, "query error")
		return
	}
	for i := range items {
		items[i].ClaimedUserID = ""
	}
	writeJSON(w, 200, map[string][]listItem{"items": items})
}

// handleDeletePrivateItem hard-deletes a private idea. A private idea cannot
// be restored through the UI.
func handleDeletePrivateItem(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	itemID, err := pathID(r, "id")
	if err != nil || !ownPrivateItem(u, itemID) {
		fail(w, 404, "item not found")
		return
	}
	if _, err := db.Exec(`DELETE FROM items WHERE id = ?`, itemID); err != nil {
		log.Printf("delete private item: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// handleSharePrivateItem converts a private idea into a shared "suggested" item,
// tagging it with the author's name so connected viewers can claim it.
func handleSharePrivateItem(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	itemID, err := pathID(r, "id")
	if err != nil || !ownPrivateItem(u, itemID) {
		fail(w, 404, "item not found")
		return
	}
	if _, err := db.Exec(`UPDATE items SET visibility = 'suggested', suggested_by = ?, suggested_by_id = ? WHERE id = ?`, u.DisplayName, u.ID, itemID); err != nil {
		log.Printf("share private item: %v", err)
		fail(w, 500, "share failed")
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok":          true,
		"suggestedBy": u.DisplayName,
	})
}

// canDeleteSuggestedItem reports whether the viewer (item author or admin) may
// hard-delete a suggested item. Only the idea author and admins can delete a
// shared item.
func canDeleteSuggestedItem(viewer *user, itemID string) bool {
	var suggestedByID string
	err := db.QueryRow(`SELECT suggested_by_id FROM items WHERE id = ?`, itemID).Scan(&suggestedByID)
	if err != nil {
		return false
	}
	if viewer.IsAdmin {
		return true
	}
	return suggestedByID != "" && suggestedByID == viewer.ID
}

// handleDeleteSuggestedItem hard-deletes a shared suggested item. Only the
// original author (matched by display name on the same list) or an admin may
// delete it.
func handleDeleteSuggestedItem(w http.ResponseWriter, r *http.Request) {
	viewer := userFrom(r)
	itemID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	var ownerID string
	var visibility string
	if err := db.QueryRow(`SELECT l.user_id, i.visibility FROM items i JOIN lists l ON l.id = i.list_id WHERE i.id = ?`, itemID).Scan(&ownerID, &visibility); err != nil || visibility != "suggested" {
		fail(w, 404, "item not found")
		return
	}
	if !canView(viewer.ID, viewer.IsAdmin, ownerID) {
		fail(w, 403, "no access to this item")
		return
	}
	if !canDeleteSuggestedItem(viewer, itemID) {
		fail(w, 403, "only the author or an admin can delete this item")
		return
	}
	if _, err := db.Exec(`DELETE FROM items WHERE id = ?`, itemID); err != nil {
		log.Printf("delete suggested item: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- claims ----------

func claimableItem(r *http.Request, itemID string) (viewer *user, listUserID string, ok bool) {
	viewer = userFrom(r)
	err := db.QueryRow(`SELECT l.user_id FROM items i JOIN lists l ON l.id = i.list_id WHERE i.id = ? AND l.archived_at IS NULL`, itemID).Scan(&listUserID)
	if err != nil || listUserID == viewer.ID {
		return nil, "", false
	}
	if !canView(viewer.ID, viewer.IsAdmin, listUserID) {
		return nil, "", false
	}
	return viewer, listUserID, true
}

func handleClaim(w http.ResponseWriter, r *http.Request) {
	itemID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	viewer, _, ok := claimableItem(r, itemID)
	if !ok {
		fail(w, 403, "cannot claim this item")
		return
	}
	var existing string
	err = db.QueryRow(`SELECT user_id FROM claims WHERE item_id = ?`, itemID).Scan(&existing)
	if err == nil {
		if existing == viewer.ID {
			writeJSON(w, 200, map[string]bool{"ok": true}) // idempotent
			return
		}
		fail(w, 409, "already claimed")
		return
	}
	res, err := db.Exec(`INSERT INTO claims (item_id, user_id) VALUES (?, ?)`, itemID, viewer.ID)
	if err != nil {
		// Lost a concurrent claim race; the unique PK on claims.item_id means
		// someone else got there first.
		fail(w, 409, "already claimed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 409, "already claimed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleUnclaim(w http.ResponseWriter, r *http.Request) {
	itemID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	viewer := userFrom(r)
	res, err := db.Exec(`DELETE FROM claims WHERE item_id = ? AND user_id = ?`, itemID, viewer.ID)
	if err != nil {
		fail(w, 500, "unclaim failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 404, "you have not claimed this item")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// ---------- archive ----------

// pageParams returns the clamped 1-based page and the corresponding offset.
func pageParams(r *http.Request, per int) (page, offset int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	return page, (page - 1) * per
}

type archivedList struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ArchivedAt   string `json:"archivedAt"`
	ItemCount    int    `json:"itemCount"`
	ClaimedCount int    `json:"claimedCount"`
}

type archiveIndexResponse struct {
	Lists   []archivedList `json:"lists"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
	Total   int            `json:"total"`
}

func handleArchiveIndex(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	const per = 20
	page, offset := pageParams(r, per)
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lists WHERE user_id = ? AND archived_at IS NOT NULL`, u.ID).Scan(&total); err != nil {
		fail(w, 500, "query error")
		return
	}
	rows, err := db.Query(`
		SELECT l.id, l.label, l.archived_at,
			(SELECT COUNT(*) FROM items i WHERE i.list_id = l.id AND i.removed_at IS NULL),
			(SELECT COUNT(*) FROM items i JOIN claims c ON c.item_id = i.id WHERE i.list_id = l.id)
		FROM lists l WHERE l.user_id = ? AND l.archived_at IS NOT NULL ORDER BY l.archived_at DESC LIMIT ? OFFSET ?`,
		u.ID, per, offset)
	if err != nil {
		fail(w, 500, "query error")
		return
	}
	defer rows.Close()
	lists := []archivedList{}
	for rows.Next() {
		var a archivedList
		if rows.Scan(&a.ID, &a.Label, &a.ArchivedAt, &a.ItemCount, &a.ClaimedCount) != nil {
			continue
		}
		lists = append(lists, a)
	}
	writeJSON(w, 200, archiveIndexResponse{Lists: lists, Page: page, HasMore: page*per < total, Total: total})
}

type archiveDetailResponse struct {
	ID    string     `json:"id"`
	Label string     `json:"label"`
	Items []listItem `json:"items"`
}

func handleArchiveList(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	listID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	var ownerID string
	var label string
	err = db.QueryRow(`SELECT user_id, label FROM lists WHERE id = ? AND archived_at IS NOT NULL`, listID).Scan(&ownerID, &label)
	if err != nil || ownerID != u.ID {
		fail(w, 404, "archive not found")
		return
	}
	items, err := itemRows("i.list_id = ?", listID)
	if err != nil {
		fail(w, 500, "query error")
		return
	}
	// Archived view shows who claimed what.
	for i := range items {
		items[i].ClaimedUserID = ""
	}
	writeJSON(w, 200, archiveDetailResponse{ID: listID, Label: label, Items: items})
}

// ---------- profile / connections / admin ----------

// shareCodeAlphabet is the set of characters share codes draw from, chosen to
// be unambiguous when read aloud or typed (no 0/O, 1/I/l).
const shareCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// generateShareCode returns a cryptographically random 8-character code.
func generateShareCode() (string, error) {
	n := len(shareCodeAlphabet)
	buf := make([]byte, 8)
	for i := range buf {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		buf[i] = shareCodeAlphabet[int(b[0])%n]
	}
	return string(buf), nil
}

// handleSetShareEnabled toggles whether the caller accepts incoming share-code
// connections. Enabling (re)generates a brand-new code; disabling deletes it so
// no one can connect with it again.
func handleSetShareEnabled(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, "bad request")
		return
	}
	if req.Enabled {
		code, err := generateShareCode()
		if err != nil {
			fail(w, 500, "could not generate code")
			return
		}
		if _, err := db.Exec(`UPDATE users SET share_code = ? WHERE id = ?`, code, u.ID); err != nil {
			log.Printf("enable share code: %v", err)
			fail(w, 500, "update failed")
			return
		}
		u.ShareCode = code
		writeJSON(w, 200, map[string]any{"shareCode": code})
		return
	}
	if _, err := db.Exec(`UPDATE users SET share_code = '' WHERE id = ?`, u.ID); err != nil {
		log.Printf("disable share code: %v", err)
		fail(w, 500, "update failed")
		return
	}
	u.ShareCode = ""
	writeJSON(w, 200, map[string]any{"shareCode": ""})
}

// handleAcceptShareCode connects the caller to the user owning the given code.
func handleAcceptShareCode(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, "bad request")
		return
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		fail(w, 400, "share code required")
		return
	}
	var ownerID string
	var displayName string
	err := db.QueryRow(`SELECT id, display_name FROM users WHERE share_code = ?`, code).Scan(&ownerID, &displayName)
	if err != nil {
		fail(w, 404, "no user found with that share code")
		return
	}
	if ownerID == u.ID {
		fail(w, 400, "you cannot connect with your own code")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		fail(w, 500, "connection failed")
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO connections (user_id, other_user_id) VALUES (?, ?)`, u.ID, ownerID); err != nil {
		fail(w, 500, "connection failed")
		return
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO connections (user_id, other_user_id) VALUES (?, ?)`, ownerID, u.ID); err != nil {
		fail(w, 500, "connection failed")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "connection failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "displayName": displayName})
}

type connectionEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type connectionsResponse struct {
	Connections []connectionEntry `json:"connections"`
	IsAdmin     bool              `json:"isAdmin"`
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	rows, err := db.Query(`
		SELECT u2.id, u2.display_name FROM connections c JOIN users u2 ON u2.id = c.other_user_id
		WHERE c.user_id = ? UNION
		SELECT u2.id, u2.display_name FROM connections c JOIN users u2 ON u2.id = c.user_id
		WHERE c.other_user_id = ?
		ORDER BY 2`, u.ID, u.ID)
	if err != nil {
		fail(w, 500, "query error")
		return
	}
	defer rows.Close()
	out := []connectionEntry{}
	for rows.Next() {
		var c connectionEntry
		if rows.Scan(&c.ID, &c.DisplayName) == nil {
			out = append(out, c)
		}
	}
	writeJSON(w, 200, connectionsResponse{Connections: out, IsAdmin: u.IsAdmin})
}

func handleSetConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		A     string `json:"a"`
		B     string `json:"b"`
		Add   bool   `json:"add"`
		Bidir *bool  `json:"bidirectional"`
	}
	if err := decode(w, r, &req); err != nil || req.A == "" || req.B == "" || req.A == req.B {
		fail(w, 400, "bad request")
		return
	}
	bidir := true
	if req.Bidir != nil {
		bidir = *req.Bidir
	}
	tx, err := db.Begin()
	if err != nil {
		fail(w, 500, "connection update failed")
		return
	}
	defer tx.Rollback()
	var execErr error
	if req.Add {
		if _, execErr = tx.Exec(`INSERT OR IGNORE INTO connections (user_id, other_user_id) VALUES (?, ?)`, req.A, req.B); execErr == nil && bidir {
			_, execErr = tx.Exec(`INSERT OR IGNORE INTO connections (user_id, other_user_id) VALUES (?, ?)`, req.B, req.A)
		}
	} else {
		_, execErr = tx.Exec(`DELETE FROM connections WHERE (user_id = ? AND other_user_id = ?) OR (user_id = ? AND other_user_id = ?)`, req.A, req.B, req.B, req.A)
	}
	if execErr != nil {
		log.Printf("set connection: %v", execErr)
		fail(w, 500, "connection update failed")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "connection update failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type adminUser struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	DisplayName  string   `json:"displayName"`
	IsAdmin      bool     `json:"isAdmin"`
	Birthday     string   `json:"birthday"`
	ItemCount    int      `json:"itemCount"`
	ClaimedCount int      `json:"claimedCount"`
	Connections  []string `json:"connections"`
	ConnectedBy  []string `json:"connectedBy"`
}

type adminUsersResponse struct {
	Users   []adminUser       `json:"users"`
	Page    int               `json:"page"`
	HasMore bool              `json:"hasMore"`
	Total   int               `json:"total"`
	Names   map[string]string `json:"names,omitempty"`
}

func handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	const per = 25
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	like := "%" + q + "%"
	var total int
	var countErr error
	if q != "" {
		countErr = db.QueryRow(`SELECT COUNT(*) FROM users WHERE display_name LIKE ? COLLATE NOCASE OR username LIKE ? COLLATE NOCASE`, like, like).Scan(&total)
	} else {
		countErr = db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total)
	}
	if countErr != nil {
		fail(w, 500, "query error")
		return
	}
	offset := (page - 1) * per
	var rows *sql.Rows
	var err error
	if q != "" {
		rows, err = db.Query(`
			SELECT u.id, u.username, u.display_name, u.is_admin, u.birthday,
				(SELECT COUNT(*) FROM items i JOIN lists l ON l.id = i.list_id
				 WHERE l.user_id = u.id AND l.archived_at IS NULL AND i.removed_at IS NULL),
				(SELECT COUNT(*) FROM claims c JOIN items i ON i.id = c.item_id JOIN lists l ON l.id = i.list_id
				 WHERE l.user_id = u.id AND l.archived_at IS NULL)
			FROM users u WHERE display_name LIKE ? COLLATE NOCASE OR username LIKE ? COLLATE NOCASE ORDER BY display_name LIMIT ? OFFSET ?`, like, like, per, offset)
	} else {
		rows, err = db.Query(`
			SELECT u.id, u.username, u.display_name, u.is_admin, u.birthday,
				(SELECT COUNT(*) FROM items i JOIN lists l ON l.id = i.list_id
				 WHERE l.user_id = u.id AND l.archived_at IS NULL AND i.removed_at IS NULL),
				(SELECT COUNT(*) FROM claims c JOIN items i ON i.id = c.item_id JOIN lists l ON l.id = i.list_id
				 WHERE l.user_id = u.id AND l.archived_at IS NULL)
			FROM users u ORDER BY display_name LIMIT ? OFFSET ?`, per, offset)
	}
	if err != nil {
		fail(w, 500, "query error")
		return
	}
	defer rows.Close()
	users := []adminUser{}
	for rows.Next() {
		var a adminUser
		if rows.Scan(&a.ID, &a.Username, &a.DisplayName, &a.IsAdmin, &a.Birthday, &a.ItemCount, &a.ClaimedCount) == nil {
			a.Connections = []string{}
			a.ConnectedBy = []string{}
			users = append(users, a)
		}
	}
	byID := map[string]*adminUser{}
	for i := range users {
		byID[users[i].ID] = &users[i]
	}
	connRows, err := db.Query(`SELECT user_id, other_user_id FROM connections`)
	if err == nil {
		defer connRows.Close()
		for connRows.Next() {
			var from, to string
			if connRows.Scan(&from, &to) == nil {
				if u, ok := byID[from]; ok {
					u.Connections = append(u.Connections, to)
				}
				if u, ok := byID[to]; ok {
					u.ConnectedBy = append(u.ConnectedBy, from)
				}
			}
		}
	}
	names := map[string]string{}
	idSet := map[string]bool{}
	for _, u := range users {
		for _, cid := range u.Connections {
			if _, ok := byID[cid]; !ok {
				idSet[cid] = true
			}
		}
		for _, cid := range u.ConnectedBy {
			if _, ok := byID[cid]; !ok {
				idSet[cid] = true
			}
		}
	}
	if len(idSet) > 0 {
		ids := make([]any, 0, len(idSet))
		ph := make([]string, 0, len(idSet))
		for cid := range idSet {
			ids = append(ids, cid)
			ph = append(ph, "?")
		}
		q := `SELECT id, display_name FROM users WHERE id IN (` + strings.Join(ph, ",") + `)`
		if rows2, err2 := db.Query(q, ids...); err2 == nil {
			defer rows2.Close()
			for rows2.Next() {
				var cid, dn string
				if rows2.Scan(&cid, &dn) == nil {
					names[cid] = dn
				}
			}
		}
	}
	// also include paginated users themselves for convenience
	for _, u := range users {
		names[u.ID] = u.DisplayName
	}
	if len(names) == 0 {
		names = nil
	}
	writeJSON(w, 200, adminUsersResponse{Users: users, Page: page, HasMore: page*per < total, Total: total, Names: names})
}

func handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	var req struct {
		Birthday string `json:"birthday"` // MM-DD
		IsAdmin  *bool  `json:"isAdmin"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, "bad request")
		return
	}
	if req.Birthday != "" {
		birthday := normalizeMMDD(req.Birthday)
		if birthday == "" {
			fail(w, 400, "valid birthday required, e.g. 05-15")
			return
		}
		if _, err := db.Exec(`UPDATE users SET birthday = ? WHERE id = ?`, birthday, userID); err != nil {
			log.Printf("admin update birthday: %v", err)
			fail(w, 500, "update failed")
			return
		}
	}
	if req.IsAdmin != nil && !*req.IsAdmin {
		// Prevent demoting the last remaining admin.
		var targetIsAdmin, adminCount int
		if err := db.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&targetIsAdmin); err != nil {
			fail(w, 404, "user not found")
			return
		}
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&adminCount)
		if targetIsAdmin == 1 && adminCount <= 1 {
			fail(w, 400, "cannot demote the last admin")
			return
		}
	}
	if req.IsAdmin != nil {
		isAdmin := 0
		if *req.IsAdmin {
			isAdmin = 1
		}
		if _, err := db.Exec(`UPDATE users SET is_admin = ? WHERE id = ?`, isAdmin, userID); err != nil {
			log.Printf("admin update flag: %v", err)
			fail(w, 500, "update failed")
			return
		}
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	caller := userFrom(r)
	if caller != nil && caller.ID == userID {
		fail(w, 400, "cannot delete yourself")
		return
	}
	var targetIsAdmin int
	if err := db.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, userID).Scan(&targetIsAdmin); err != nil {
		fail(w, 404, "user not found")
		return
	}
	if targetIsAdmin == 1 {
		var adminCount int
		db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&adminCount)
		if adminCount <= 1 {
			fail(w, 400, "cannot delete the last admin")
			return
		}
	}
	// Rely on FK cascades (lists, items, claims, connections) but explicitly
	// delete claims made by the user on other lists first to handle any
	// ordering issues, then delete the user row.
	tx, err := db.Begin()
	if err != nil {
		fail(w, 500, "delete failed")
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM claims WHERE user_id = ?`, userID); err != nil {
		log.Printf("delete user claims: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	if _, err := tx.Exec(`DELETE FROM connections WHERE user_id = ? OR other_user_id = ?`, userID, userID); err != nil {
		log.Printf("delete user connections: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	// Delete lists (cascade items/claims)
	if _, err := tx.Exec(`DELETE FROM lists WHERE user_id = ?`, userID); err != nil {
		log.Printf("delete user lists: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	res, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		log.Printf("delete user: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 404, "user not found")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "delete failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleAdminForceArchive(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&exists); err != nil || exists == 0 {
		fail(w, 404, "user not found")
		return
	}
	listID, err := activeListID(userID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM items WHERE list_id = ? AND removed_at IS NULL`, listID).Scan(&count)
	label := "Archived by admin " + time.Now().UTC().Format("2006-01-02")
	tx, err := db.Begin()
	if err != nil {
		fail(w, 500, "archive failed")
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE lists SET archived_at = datetime('now'), label = ? WHERE id = ? AND archived_at IS NULL`, label, listID); err != nil {
		log.Printf("force archive update: %v", err)
		fail(w, 500, "archive failed")
		return
	}
	res, err := tx.Exec(`INSERT INTO lists (user_id, label) VALUES (?, 'Active')`, userID)
	if err != nil {
		log.Printf("force archive new list: %v", err)
		fail(w, 500, "archive failed")
		return
	}
	newID, _ := res.LastInsertId()
	// Carry over items with the same rules as the normal archive:
	//   - Unclaimed public items copy as public.
	//   - Unclaimed private items roll over as private.
	//   - Unclaimed suggested items are dropped.
	//   - Claimed items (any visibility) stay in the archive; claimed
	//     private/suggested items are revealed as public on the new list.
	rows, err := tx.Query(`SELECT i.title, i.url, i.notes, i.sort_order, i.visibility,
			EXISTS(SELECT 1 FROM claims c WHERE c.item_id = i.id) AS claimed
		FROM items i WHERE i.list_id = ? AND i.removed_at IS NULL`, listID)
	if err != nil {
		fail(w, 500, "archive failed")
		return
	}
	defer rows.Close()
	type carryVal struct {
		title, url, notes, visibility string
		sort                           int
	}
	var carry []carryVal
	for rows.Next() {
		var title, url, notes, visibility string
		var sort int
		var claimed bool
		if rows.Scan(&title, &url, &notes, &sort, &visibility, &claimed) != nil {
			continue
		}
		if claimed {
			if visibility == "public" {
				continue // claimed public items stay in archive
			}
			carry = append(carry, carryVal{title, url, notes, "public", sort}) // reveal claimed private/suggested
		} else if visibility == "public" {
			carry = append(carry, carryVal{title, url, notes, "public", sort})
		} else if visibility == "private" {
			carry = append(carry, carryVal{title, url, notes, "private", sort}) // roll over
		}
		// unclaimed suggested -> dropped
	}
	rows.Close()
	for _, f := range carry {
		if _, err := tx.Exec(`INSERT INTO items (list_id, title, url, notes, sort_order, visibility, suggested_by, suggested_by_id)
			VALUES (?, ?, ?, ?, ?, ?, '', '')`, newID, f.title, f.url, f.notes, f.sort, f.visibility); err != nil {
			log.Printf("force archive copy: %v", err)
			fail(w, 500, "archive failed")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "archive failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "archivedListId": listID, "newListId": newID, "copied": len(carry)})
}

func handleAdminHardDelete(w http.ResponseWriter, r *http.Request) {
	userID, err := pathID(r, "id")
	if err != nil {
		fail(w, 400, "bad id")
		return
	}
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = ?`, userID).Scan(&exists); err != nil || exists == 0 {
		fail(w, 404, "user not found")
		return
	}
	listID, err := activeListID(userID)
	if err != nil {
		fail(w, 500, "list error")
		return
	}
	tx, err := db.Begin()
	if err != nil {
		fail(w, 500, "delete failed")
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM claims WHERE item_id IN (SELECT id FROM items WHERE list_id = ?)`, listID); err != nil {
		log.Printf("hard delete claims: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	res, err := tx.Exec(`DELETE FROM items WHERE list_id = ?`, listID)
	if err != nil {
		log.Printf("hard delete items: %v", err)
		fail(w, 500, "delete failed")
		return
	}
	n, _ := res.RowsAffected()
	if err := tx.Commit(); err != nil {
		fail(w, 500, "delete failed")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "deleted": n})
}
