package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMMDD(t *testing.T) {
	d := parseMMDD("12-25", 2026)
	if d.Year() != 2026 || d.Month() != time.December || d.Day() != 25 {
		t.Errorf("parseMMDD = %v", d)
	}
}

// TestMigrationsAddPrivateListTheme guards databases upgraded from before
// private lists: lists.theme existed in schema.sql but was missing from the
// startup migrations, so creating the first private list failed with
// "table lists has no column named theme".
func TestMigrationsAddPrivateListTheme(t *testing.T) {
	d, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer d.Close()

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	// Recreate the schema as it existed before lists.theme was introduced.
	legacy := strings.Replace(string(schema), ",\n\ttheme TEXT NOT NULL DEFAULT 'aurora'", "", 1)
	if legacy == string(schema) {
		t.Fatal("schema.sql no longer defines lists.theme; update this test")
	}
	if _, err := d.Exec(legacy); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}

	applyMigrations(d)

	var hasTheme int
	if err := d.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('lists') WHERE name = 'theme'`).Scan(&hasTheme); err != nil {
		t.Fatalf("inspect lists columns: %v", err)
	}
	if hasTheme != 1 {
		t.Fatal("migrations did not add lists.theme")
	}

	// The insert handleCreatePrivateList performs must work on the migrated db.
	if _, err := d.Exec(`INSERT INTO users (id, auth_user_id, username, display_name) VALUES ('u1', 'legacy:u1', 'legacy_u1', 'Legacy User')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := d.Exec(`INSERT INTO lists (id, user_id, label, is_private, first_name, last_name, birthday, theme)
		VALUES ('l1', 'u1', 'Jane Doe', 1, 'Jane', 'Doe', '05-15', 'aurora')`); err != nil {
		t.Fatalf("insert private list after migration: %v", err)
	}
	var theme string
	if err := d.QueryRow(`SELECT theme FROM lists WHERE id = 'l1'`).Scan(&theme); err != nil || theme != "aurora" {
		t.Fatalf("private list theme = %q, %v; want aurora", theme, err)
	}
}

// makeAuthedRequest builds a request carrying the given bearer token, as the
// auth middleware receives them.
func makeAuthedRequest(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestSetAuthConfig(t *testing.T) {
	html := "<!doctype html><script>theme</script><script type=\"module\" src=\"/src/main.ts\"></script>"
	out := setAuthConfig(html, "https://auth.example.com/application/o/giftlstr/", "client-1")

	if !strings.Contains(out, `window.__GIFTLSTR__ = { authIssuer: "https://auth.example.com/application/o/giftlstr/", authClientId: "client-1" }`) {
		t.Errorf("config missing from output: %s", out)
	}
	// The config script lands before the module bundle so the value is
	// defined by the time the app module reads it.
	if !strings.Contains(out, `</script><script type="module"`) {
		t.Errorf("config script not inserted before the module script: %s", out)
	}
	if !strings.Contains(out, `src="/src/main.ts"`) {
		t.Errorf("module script must be preserved: %s", out)
	}
	if strings.Count(out, `window.__GIFTLSTR__`) != 1 {
		t.Errorf("config must appear exactly once: %s", out)
	}
}

func TestSetAuthConfigReplacesPreviousValue(t *testing.T) {
	html := `<script>window.__GIFTLSTR__ = { legacyAuthConfig: "pk_old_baked_in" };</script>` +
		`<script type="module" src="/src/main.ts"></script>`
	out := setAuthConfig(html, "https://auth.example.com/application/o/giftlstr/", "client-2")

	if strings.Contains(out, "pk_old_baked_in") {
		t.Errorf("previous config was not removed: %s", out)
	}
	if !strings.Contains(out, `authIssuer: "https://auth.example.com/application/o/giftlstr/"`) {
		t.Errorf("runtime issuer not substituted: %s", out)
	}
	if strings.Count(out, `window.__GIFTLSTR__`) != 1 {
		t.Errorf("config must appear exactly once after replacement: %s", out)
	}
}

func TestSetAuthConfigEscapesValues(t *testing.T) {
	out := setAuthConfig(`<script type="module" src="/src/main.ts"></script>`, `a"b\c<div>`, "x&y")
	if !strings.Contains(out, `authIssuer: "a\"b\\c\u003cdiv\u003e"`) {
		t.Errorf("issuer not escaped for inlining: %s", out)
	}
	if !strings.Contains(out, `authClientId: "x\u0026y"`) {
		t.Errorf("client id not escaped for inlining: %s", out)
	}
}

func TestContentSecurityPolicyHashesInlineScripts(t *testing.T) {
	theme := `<script>var t = "theme";</script>`
	config := `<script>window.__GIFTLSTR__ = { authIssuer: "https://auth.example.com/" };</script>`
	csp := contentSecurityPolicy(scriptHash(theme), scriptHash(config))

	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("missing default-src: %s", csp)
	}
	for _, src := range []string{"'sha256-" + scriptHash(theme) + "'", "'sha256-" + scriptHash(config) + "'", "frame-ancestors 'none'", "object-src 'none'", "script-src 'self' 'sha256-" + scriptHash(theme) + "' 'sha256-" + scriptHash(config) + "' https://static.cloudflareinsights.com", "connect-src 'self' https://cloudflareinsights.com"} {
		if !strings.Contains(csp, src) {
			t.Errorf("CSP missing %s: %s", src, csp)
		}
	}
	if strings.Contains(csp, "'unsafe-inline' script") || strings.Count(csp, "'unsafe-inline'") != 1 {
		t.Errorf("script-src must not be unsafe-inline: %s", csp)
	}
	if scriptHash(`<script>var x = 1;</script>`) == scriptHash(`<script>var x = 2;</script>`) {
		t.Error("script hashes must differ for different contents")
	}
}

func TestLastResetOccurrence(t *testing.T) {
	tests := []struct {
		mmdd string
		now  string
		want string
	}{
		{"12-25", "2026-08-22", "2025-12-25"}, // Christmas not yet passed this year
		{"01-01", "2026-08-22", "2026-01-01"}, // already passed this year
		{"08-21", "2026-08-22", "2026-08-21"}, // yesterday
		{"08-22", "2026-08-22", "2026-08-22"}, // today counts as the current occurrence
	}
	for _, tc := range tests {
		now, _ := time.Parse("2006-01-02", tc.now)
		got := lastResetOccurrence(tc.mmdd, now)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("lastResetOccurrence(%q at %s) = %s, want %s", tc.mmdd, tc.now, got.Format("2006-01-02"), tc.want)
		}
	}
}

func TestNthSundayOf(t *testing.T) {
	tests := []struct {
		year  int
		month time.Month
		nth   int
		want  string
	}{
		{2026, time.May, 1, "2026-05-03"},  // 2026-05-01 is a Friday
		{2026, time.May, 2, "2026-05-10"},  // Mother's Day 2026
		{2027, time.May, 2, "2027-05-09"},  // Mother's Day 2027
		{2025, time.May, 2, "2025-05-11"},  // Mother's Day 2025
		{2026, time.June, 3, "2026-06-21"}, // Father's Day 2026
		{2025, time.June, 3, "2025-06-15"}, // Father's Day 2025
	}
	for _, tc := range tests {
		got := nthSundayOf(tc.year, tc.month, tc.nth)
		if got.Format("2006-01-02") != tc.want {
			t.Errorf("nthSundayOf(%d, %s, %d) = %s, want %s", tc.year, tc.month, tc.nth, got.Format("2006-01-02"), tc.want)
		}
	}
}

func TestHolidayOccurrence(t *testing.T) {
	tests := []struct {
		kind string
		now  string
		want string
	}{
		{resetMothers, "2026-05-15", "2026-05-10"},
		{resetMothers, "2026-05-10", "2026-05-10"}, // today counts as the current occurrence
		{resetMothers, "2026-04-15", "2025-05-11"}, // not yet passed this year -> last year's date
		{resetFathers, "2026-06-21", "2026-06-21"},
		{resetFathers, "2026-06-20", "2025-06-15"},
	}
	for _, tc := range tests {
		now, _ := time.Parse("2006-01-02", tc.now)
		got, ok := holidayOccurrence(tc.kind, now)
		if !ok || got.Format("2006-01-02") != tc.want {
			t.Errorf("holidayOccurrence(%q at %s) = %s (%v), want %s", tc.kind, tc.now, got.Format("2006-01-02"), ok, tc.want)
		}
	}
	if _, ok := holidayOccurrence("solstice", time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC)); ok {
		t.Error("unknown holiday should report ok=false")
	}
}

func TestArchiveUserListArchivesAndCarriesOver(t *testing.T) {
	u := seedUser(t, "c_arch_u", "arch_user", false)
	u.Birthday = "07-01"
	db.Exec(`UPDATE users SET birthday = '07-01' WHERE id = ?`, u.ID)

	listID := activeList(t, u.ID)
	// Backdate the active list so it predates the cutoff.
	db.Exec(`UPDATE lists SET created_at = '2026-06-01 00:00:00' WHERE id = ?`, listID)
	claimed := addItemRow(t, listID, "claimed item")
	addItemRow(t, listID, "unclaimed item")
	removed := addItemRow(t, listID, "removed item")
	db.Exec(`INSERT INTO claims (item_id, user_id) VALUES (?, ?)`, claimed, u.ID)
	db.Exec(`UPDATE items SET removed_at = datetime('now') WHERE id = ?`, removed)

	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC) // > 1 week after Jul 1
	if err := archiveUserList(u, now); err != nil {
		t.Fatalf("archiveUserList: %v", err)
	}

	// Old list archived with the reset label.
	var label string
	var archivedAt any
	if err := db.QueryRow(`SELECT label, archived_at FROM lists WHERE id = ?`, listID).Scan(&label, &archivedAt); err != nil {
		t.Fatal(err)
	}
	if label != "2026 Birthday" {
		t.Errorf("label = %q, want 2026 Birthday", label)
	}
	if archivedAt == nil {
		t.Error("list should be archived")
	}

	// New active list carries only unclaimed, non-removed items.
	newID, err := activeListID(u.ID)
	if err != nil || newID == listID {
		t.Fatalf("expected a new active list, got %s (%v)", newID, err)
	}
	rows, _ := db.Query(`SELECT title FROM items WHERE list_id = ? ORDER BY title`, newID)
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		titles = append(titles, s)
	}
	if len(titles) != 1 || titles[0] != "unclaimed item" {
		t.Errorf("carried-over items = %v, want only [unclaimed item]", titles)
	}

	// Running again is a no-op: the fresh list was created after the cutoff.
	if err := archiveUserList(u, now); err != nil {
		t.Fatalf("second archiveUserList: %v", err)
	}
	again, _ := activeListID(u.ID)
	if again != newID {
		t.Error("second run should not archive the fresh list")
	}
}

func TestArchiveUserListSkipsWhenCutoffNotPassed(t *testing.T) {
	u := seedUser(t, "c_arch_skip", "arch_skip_user", false)
	u.Birthday = "07-01"
	db.Exec(`UPDATE users SET birthday = '07-01' WHERE id = ?`, u.ID)

	listID := activeList(t, u.ID)
	db.Exec(`UPDATE lists SET created_at = '2026-06-01 00:00:00' WHERE id = ?`, listID)

	// Only 3 days after reset: no archive yet.
	now := time.Date(2026, time.July, 4, 12, 0, 0, 0, time.UTC)
	if err := archiveUserList(u, now); err != nil {
		t.Fatalf("archiveUserList: %v", err)
	}

	current, _ := activeListID(u.ID)
	if current != listID {
		t.Error("list should not be archived before cutoff")
	}
}

// TestArchiveUserListCutoffBoundary pins the reset timing: with the cutoff at
// 8 days after the reset date, a birthday only 7 days past must not archive
// yet (the timezone buffer), and 9+ days past must.
func TestArchiveUserListCutoffBoundary(t *testing.T) {
	stages := []struct {
		name         string
		now          time.Time
		wantArchived bool
	}{
		{"7 days after reset (still within buffer)", time.Date(2026, time.July, 8, 12, 0, 0, 0, time.UTC), false},
		{"9 days after reset (past cutoff)", time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC), true},
	}
	for i, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			u := seedUser(t, "c_cutoff_b_"+string(rune('a'+i)), "cutoff_b_"+string(rune('a'+i)), false)
			u.Birthday = "07-01"
			db.Exec(`UPDATE users SET birthday = '07-01' WHERE id = ?`, u.ID)

			listID := activeList(t, u.ID)
			// Backdate past the birthday cutoff (Jul 9) but after the recent
			// Christmas cutoff so only the birthday reset can fire.
			db.Exec(`UPDATE lists SET created_at = '2026-06-01 00:00:00' WHERE id = ?`, listID)

			if err := archiveUserList(u, stage.now); err != nil {
				t.Fatalf("archiveUserList: %v", err)
			}
			current, _ := activeListID(u.ID)
			archived := current != listID
			if archived != stage.wantArchived {
				t.Errorf("archived = %v, want %v", archived, stage.wantArchived)
			}
		})
	}
}

func TestValidMMDD(t *testing.T) {
	valid := []string{"01-01", "12-31", "02-29", "07-04"}
	for _, s := range valid {
		if !validMMDD(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	invalid := []string{"", "0101", "13-01", "00-10", "12-32", "1-01"}
	for _, s := range invalid {
		if validMMDD(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

// TestArchiveUserListHoliday pins the Mother's Day reset: with the active list
// created after the previous Christmas cutoff, the holiday reset fires once its
// 8-day cutoff passes (label "Mother's Day 2026") but not while this year's
// occurrence is still upcoming.
func TestArchiveUserListHoliday(t *testing.T) {
	t.Run("archives after the holiday cutoff passes", func(t *testing.T) {
		u := seedUser(t, "c_arch_hol", "arch_hol_user", false)
		u.Birthday = ""
		u.HolidayReset = resetMothers
		db.Exec(`UPDATE users SET birthday = '', holiday_reset = 'mothers' WHERE id = ?`, u.ID)

		listID := activeList(t, u.ID)
		// Backdated after the Christmas cutoff (2026-01-02) but well before
		// Mother's Day (2026-05-10) so only the holiday reset can fire.
		db.Exec(`UPDATE lists SET created_at = '2026-01-20 00:00:00' WHERE id = ?`, listID)

		now := time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC) // > 8 days after May 10
		if err := archiveUserList(u, now); err != nil {
			t.Fatalf("archiveUserList: %v", err)
		}
		var label string
		if err := db.QueryRow(`SELECT label FROM lists WHERE id = ?`, listID).Scan(&label); err != nil {
			t.Fatal(err)
		}
		if label != "2026 Mother's Day" {
			t.Errorf("label = %q, want 2026 Mother's Day", label)
		}
		current, _ := activeListID(u.ID)
		if current == listID {
			t.Error("list should have been archived by the Mother's Day reset")
		}
	})

	t.Run("skips while the holiday is still upcoming", func(t *testing.T) {
		u := seedUser(t, "c_arch_hol2", "arch_hol2_user", false)
		u.Birthday = ""
		u.HolidayReset = resetMothers
		db.Exec(`UPDATE users SET birthday = '', holiday_reset = 'mothers' WHERE id = ?`, u.ID)

		listID := activeList(t, u.ID)
		db.Exec(`UPDATE lists SET created_at = '2026-01-20 00:00:00' WHERE id = ?`, listID)

		// Two weeks before Mother's Day 2026: last year's occurrence already
		// predates the list, so nothing fires.
		now := time.Date(2026, time.April, 25, 12, 0, 0, 0, time.UTC)
		if err := archiveUserList(u, now); err != nil {
			t.Fatalf("archiveUserList: %v", err)
		}
		current, _ := activeListID(u.ID)
		if current != listID {
			t.Error("list should not be archived before the holiday occurs this year")
		}
	})
}

func TestArchiveUserListWithNoBirthday(t *testing.T) {
	u := seedUser(t, "c_nobday", "no_bday_user", false)
	db.Exec(`UPDATE users SET birthday = '' WHERE id = ?`, u.ID)
	u.Birthday = ""

	listID := activeList(t, u.ID)
	db.Exec(`UPDATE lists SET created_at = '2024-01-01 00:00:00' WHERE id = ?`, listID)

	// Long past the Christmas cutoff; must not panic on the empty birthday
	// and should still archive via the Christmas reset.
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	if err := archiveUserList(u, now); err != nil {
		t.Fatalf("archiveUserList: %v", err)
	}
	var archived any
	if err := db.QueryRow(`SELECT archived_at FROM lists WHERE id = ?`, listID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived == nil {
		t.Error("list should be archived by the Christmas reset")
	}
	var label string
	if err := db.QueryRow(`SELECT label FROM lists WHERE id = ?`, listID).Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != "2025 Christmas" {
		t.Errorf("label = %q, want 2025 Christmas", label)
	}
}

func TestArchivePrivateListsRollsOver(t *testing.T) {
	u := seedUser(t, "c_arch_priv", "arch_priv_user", false)

	listID := newID()
	if _, err := db.Exec(`INSERT INTO lists (id, user_id, label, is_private, first_name, last_name, birthday, theme, created_at)
		VALUES (?, ?, 'Jamie Doe', 1, 'Jamie', 'Doe', '07-01', 'ember', '2026-06-01 00:00:00')`, listID, u.ID); err != nil {
		t.Fatalf("insert private list: %v", err)
	}
	addItemRow(t, listID, "unclaimed idea")
	claimed := addItemRow(t, listID, "bought idea")
	removed := addItemRow(t, listID, "removed idea")
	db.Exec(`INSERT INTO claims (item_id, user_id) VALUES (?, ?)`, claimed, u.ID)
	db.Exec(`UPDATE items SET removed_at = datetime('now') WHERE id = ?`, removed)

	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC) // > 8 days after Jul 1
	if err := archivePrivateLists(u, now); err != nil {
		t.Fatalf("archivePrivateLists: %v", err)
	}

	// Old list archived with the birthday label.
	var label string
	var archivedAt any
	if err := db.QueryRow(`SELECT label, archived_at FROM lists WHERE id = ?`, listID).Scan(&label, &archivedAt); err != nil {
		t.Fatal(err)
	}
	if label != "2026 Birthday" || archivedAt == nil {
		t.Errorf("archived private list = (%q, %v), want (2026 Birthday, set)", label, archivedAt)
	}

	// A fresh private list for the same person carries only the unclaimed idea.
	var freshID, firstName, lastName, birthday, theme string
	if err := db.QueryRow(`SELECT id, first_name, last_name, birthday, theme FROM lists
		WHERE user_id = ? AND is_private = 1 AND archived_at IS NULL`, u.ID).
		Scan(&freshID, &firstName, &lastName, &birthday, &theme); err != nil {
		t.Fatalf("fresh private list: %v", err)
	}
	if freshID == listID {
		t.Fatal("expected a fresh private list")
	}
	if firstName != "Jamie" || lastName != "Doe" || birthday != "07-01" || theme != "ember" {
		t.Errorf("fresh list person = %q %q %q %q, want Jamie Doe 07-01 ember", firstName, lastName, birthday, theme)
	}
	rows, _ := db.Query(`SELECT title FROM items WHERE list_id = ? ORDER BY title`, freshID)
	defer rows.Close()
	var titles []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		titles = append(titles, s)
	}
	if len(titles) != 1 || titles[0] != "unclaimed idea" {
		t.Errorf("carried items = %v, want only [unclaimed idea]", titles)
	}

	// Running again is a no-op: the fresh list was created after the cutoff.
	if err := archivePrivateLists(u, now); err != nil {
		t.Fatalf("second archivePrivateLists: %v", err)
	}
	var live int
	db.QueryRow(`SELECT COUNT(*) FROM lists WHERE user_id = ? AND is_private = 1 AND archived_at IS NULL`, u.ID).Scan(&live)
	if live != 1 {
		t.Errorf("live private lists = %d, want 1", live)
	}
}

func TestArchivePrivateListsSkipsBeforeCutoff(t *testing.T) {
	u := seedUser(t, "c_arch_priv_skip", "arch_priv_skip", false)
	listID := newID()
	if _, err := db.Exec(`INSERT INTO lists (id, user_id, label, is_private, first_name, last_name, birthday, created_at)
		VALUES (?, ?, 'Jamie Doe', 1, 'Jamie', 'Doe', '07-01', '2026-06-01 00:00:00')`, listID, u.ID); err != nil {
		t.Fatalf("insert private list: %v", err)
	}

	now := time.Date(2026, time.July, 4, 12, 0, 0, 0, time.UTC) // only 3 days after reset
	if err := archivePrivateLists(u, now); err != nil {
		t.Fatalf("archivePrivateLists: %v", err)
	}
	var archived any
	if err := db.QueryRow(`SELECT archived_at FROM lists WHERE id = ?`, listID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != nil {
		t.Error("private list should not archive before the cutoff")
	}
}

func TestClaimConflictReturns409(t *testing.T) {
	owner := seedUser(t, "c_race_owner", "race_owner", false)
	viewerA := seedUser(t, "c_race_a", "race_a", false)
	viewerB := seedUser(t, "c_race_b", "race_b", false)
	connectUsers(t, owner.ID, viewerA.ID)
	connectUsers(t, owner.ID, viewerB.ID)

	itemID := addItemRow(t, activeList(t, owner.ID), "contested item")

	w, _ := doJSON(t, viewerA, "PUT", fmt.Sprintf("/api/items/%s/claim", itemID), nil)
	mustStatus(t, w, 200)

	// A second claimant loses the race and gets a conflict, not a 500.
	w, resp := doJSON(t, viewerB, "PUT", fmt.Sprintf("/api/items/%s/claim", itemID), nil)
	mustStatus(t, w, 409)
	if resp["error"] != "already claimed" {
		t.Errorf("error = %v, want already claimed", resp["error"])
	}

	// Re-claiming your own claim is idempotent.
	w, _ = doJSON(t, viewerA, "PUT", fmt.Sprintf("/api/items/%s/claim", itemID), nil)
	mustStatus(t, w, 200)
}

func TestProvisionUserIdempotent(t *testing.T) {
	id := &Identity{Subject: "c_provision_dup", Username: "provisioned", DisplayName: "Provisioned User"}
	first, err := provisionUser(id)
	if err != nil {
		t.Fatalf("first provision: %v", err)
	}
	second, err := provisionUser(id)
	if err != nil {
		t.Fatalf("second provision: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("provisioning twice returned users %s and %s; want the same row", first.ID, second.ID)
	}
	var lists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lists WHERE user_id = ?`, first.ID).Scan(&lists); err != nil {
		t.Fatal(err)
	}
	if lists != 1 {
		t.Errorf("user has %d lists; want exactly 1 active list", lists)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE auth_user_id = ?`, id.Subject).Scan(&count); err != nil || count != 1 {
		t.Errorf("user rows for identity = %d, want 1 (err=%v)", count, err)
	}
}

func TestProvisionUserReadsBirthdayFromMetadata(t *testing.T) {
	u, err := provisionUser(&Identity{Subject: "c_meta_user", Username: "meta", DisplayName: "Meta User", Birthday: "04-21"})
	if err != nil {
		t.Fatalf("provisionUser: %v", err)
	}
	if u.Birthday != "04-21" {
		t.Errorf("birthday = %q, want 04-21 set during account creation", u.Birthday)
	}
	var stored string
	if err := db.QueryRow(`SELECT birthday FROM users WHERE id = ?`, u.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "04-21" {
		t.Errorf("stored birthday = %q, want 04-21", stored)
	}

	bad, err := provisionUser(&Identity{Subject: "c_badmeta_user", Username: "badmeta", DisplayName: "Bad Meta", Birthday: "not-a-date"})
	if err != nil {
		t.Fatalf("provisionUser (invalid metadata): %v", err)
	}
	if bad.Birthday != "" {
		t.Errorf("birthday = %q, want empty when metadata is invalid", bad.Birthday)
	}
}

// stubAuth lets tests drive provisioning with a fixed identity instead of a
// real identity provider.
type stubAuth struct{ ident *Identity }

func (s stubAuth) VerifySession(ctx context.Context, _ string) (*Identity, error) {
	return s.ident, nil
}

func TestCurrentUserProvisionsFromVerifiedIdentity(t *testing.T) {
	orig := activeAuth
	defer func() { activeAuth = orig }()
	id := &Identity{Subject: "c_curuser", Username: "cur_user", DisplayName: "Cur User", Birthday: "06-15"}
	activeAuth = stubAuth{ident: id}

	r := makeAuthedRequest("some.token")
	u, err := currentUser(r)
	if err != nil {
		t.Fatalf("currentUser: %v", err)
	}
	if u.Username != "cur_user" || u.DisplayName != "Cur User" || u.Birthday != "06-15" {
		t.Errorf("provisioned user = %+v, want profile carried over from identity", u)
	}

	// A second request reuses the same local row.
	r2 := makeAuthedRequest("some.token")
	u2, err := currentUser(r2)
	if err != nil {
		t.Fatalf("second currentUser: %v", err)
	}
	if u2.ID != u.ID {
		t.Errorf("second request provisioned a different user (%s vs %s)", u2.ID, u.ID)
	}
}
