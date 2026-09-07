package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	for _, src := range []string{"'sha256-" + scriptHash(theme) + "'", "'sha256-" + scriptHash(config) + "'", "frame-ancestors 'none'", "object-src 'none'"} {
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
	if label != "Birthday 2026" {
		t.Errorf("label = %q, want Birthday 2026", label)
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
	if label != "Christmas 2025" {
		t.Errorf("label = %q, want Christmas 2025", label)
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
