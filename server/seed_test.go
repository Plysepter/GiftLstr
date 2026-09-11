package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// freshDB opens a throw-away sqlite database with the schema applied.
func freshDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "seed.db")+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	schema, _ := schemaFS.ReadFile("schema.sql")
	if _, err := d.Exec(string(schema)); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return d
}

func TestApplySeed(t *testing.T) {
	d := freshDB(t)
	if err := applySeed(d); err != nil {
		t.Fatalf("applySeed: %v", err)
	}

	var total int
	if err := d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 40 {
		t.Errorf("users = %d, want 40", total)
	}

	// Exactly one non-archived (active) list per seeded user.
	var bad int
	if err := d.QueryRow(`SELECT COUNT(*) FROM (
		SELECT user_id FROM lists GROUP BY user_id
		HAVING SUM(CASE WHEN archived_at IS NULL THEN 1 ELSE 0 END) != 1)`).Scan(&bad); err != nil {
		t.Fatal(err)
	}
	if bad != 0 {
		t.Errorf("%d users do not have exactly one active list", bad)
	}

	// Every active list id is longer than 10 chars (auth e2e asserts this).
	var short int
	if err := d.QueryRow(`SELECT COUNT(*) FROM lists WHERE archived_at IS NULL AND length(id) <= 10`).Scan(&short); err != nil {
		t.Fatal(err)
	}
	if short != 0 {
		t.Errorf("%d active list ids are not longer than 10 chars", short)
	}

	// admin is the only admin; alice has a birthday but no holiday reset.
	var admins int
	if err := d.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if admins != 1 {
		t.Errorf("admins = %d, want 1", admins)
	}
	var aliceHoliday string
	if err := d.QueryRow(`SELECT holiday_reset FROM users WHERE username = 'alice'`).Scan(&aliceHoliday); err != nil {
		t.Fatal(err)
	}
	if aliceHoliday != "" {
		t.Errorf("alice holiday_reset = %q, want empty (copy-list header regex)", aliceHoliday)
	}

	// alice is connected to 6 distinct users.
	var aliceConns int
	if err := d.QueryRow(`SELECT COUNT(*) FROM (
		SELECT other_user_id FROM connections WHERE user_id = (SELECT id FROM users WHERE username = 'alice')
		UNION
		SELECT user_id FROM connections WHERE other_user_id = (SELECT id FROM users WHERE username = 'alice'))`).Scan(&aliceConns); err != nil {
		t.Fatal(err)
	}
	if aliceConns != 6 {
		t.Errorf("alice connections = %d, want 6", aliceConns)
	}

	// bob must hold no claim on alice's active list (claim e2e badge).
	var bobClaimsOnAlice int
	if err := d.QueryRow(`SELECT COUNT(*) FROM claims c JOIN items i ON i.id = c.item_id
		WHERE c.user_id = (SELECT id FROM users WHERE username = 'bob')
		AND i.list_id = (SELECT id FROM lists WHERE user_id = (SELECT id FROM users WHERE username = 'alice') AND archived_at IS NULL)`).Scan(&bobClaimsOnAlice); err != nil {
		t.Fatal(err)
	}
	if bobClaimsOnAlice != 0 {
		t.Errorf("bob claims on alice's list = %d, want 0", bobClaimsOnAlice)
	}

	// Alice's suggested items are unclaimed (owner MyList shows "claimed by").
	var suggestedClaimed int
	if err := d.QueryRow(`SELECT COUNT(*) FROM items i JOIN claims c ON c.item_id = i.id
		WHERE i.list_id = (SELECT id FROM lists WHERE user_id = (SELECT id FROM users WHERE username = 'alice') AND archived_at IS NULL)
		AND i.visibility = 'suggested'`).Scan(&suggestedClaimed); err != nil {
		t.Fatal(err)
	}
	if suggestedClaimed != 0 {
		t.Errorf("claimed suggested items on alice's list = %d, want 0", suggestedClaimed)
	}

	// dana's list carries a seeded item with a cached preview URL.
	var danaItems int
	if err := d.QueryRow(`SELECT COUNT(*) FROM items WHERE list_id = (SELECT id FROM lists WHERE user_id = (SELECT id FROM users WHERE username = 'dana') AND archived_at IS NULL) AND removed_at IS NULL`).Scan(&danaItems); err != nil {
		t.Fatal(err)
	}
	if danaItems == 0 {
		t.Error("dana should have seeded items")
	}

	// Runtime-provisioned test users must not be pre-seeded.
	for _, username := range []string{"neww", "neww-skip", "neww-holiday", "dave", "eve"} {
		var n int
		if err := d.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ?`, username).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("user %q must not be seeded (specs provision it at runtime)", username)
		}
	}

	// Re-applying the seed is idempotent (repeatable across boots).
	if err := applySeed(d); err != nil {
		t.Fatalf("re-apply seed: %v", err)
	}
	if err := d.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 40 {
		t.Errorf("users after re-seed = %d, want 40", total)
	}
}

func TestDemoUsersEndpoint(t *testing.T) {
	// The endpoint is only served in e2e mode.
	t.Setenv("E2E_TEST_MODE", "1")
	t.Setenv("AUTHENTIK_ISSUER", "")

	prev := db
	db = freshDB(t)
	defer func() { db = prev }()
	if err := applySeed(db); err != nil {
		t.Fatalf("applySeed: %v", err)
	}

	mux := http.NewServeMux()
	registerAPI(mux)

	// Unauthenticated call (signed-out Login page) works.
	r := httptest.NewRequest(http.MethodGet, "/api/e2e/demo-users", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Users []struct {
			Username    string `json:"username"`
			DisplayName string `json:"displayName"`
			IsAdmin     bool   `json:"isAdmin"`
		} `json:"users"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, u := range resp.Users {
		got[u.Username] = true
	}
	for _, want := range []string{"admin", "alice", "bob", "carol", "dana", "eric", "faye", "george", "helen", "ivan"} {
		if !got[want] {
			t.Errorf("demo users missing %s", want)
		}
	}
	// The admin account is flagged as such for the picker.
	adminFlagged := false
	for _, u := range resp.Users {
		if u.Username == "admin" && u.IsAdmin {
			adminFlagged = true
		}
	}
	if !adminFlagged {
		t.Error("admin demo account should be flagged isAdmin")
	}
	// Bulk mock accounts are not demo accounts.
	if got["mock01"] || got["mock30"] {
		t.Errorf("mock accounts should not appear in demo users: %v", got)
	}
}

func TestAuthConfigScriptE2EFlag(t *testing.T) {
	t.Setenv("E2E_TEST_MODE", "1")
	t.Setenv("AUTHENTIK_ISSUER", "")
	if s := authConfigScript("", ""); !strings.Contains(s, "authE2e: true") {
		t.Errorf("e2e mode config script missing authE2e flag: %s", s)
	}
	t.Setenv("E2E_TEST_MODE", "0")
	if s := authConfigScript("", ""); strings.Contains(s, "authE2e") {
		t.Errorf("non-e2e config script should not contain authE2e: %s", s)
	}
}
