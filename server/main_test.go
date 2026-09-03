package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/google/uuid"
)

func TestParseMMDD(t *testing.T) {
	d := parseMMDD("12-25", 2026)
	if d.Year() != 2026 || d.Month() != time.December || d.Day() != 25 {
		t.Errorf("parseMMDD = %v", d)
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
	orig := clerkUser
	defer func() { clerkUser = orig }()
	clerkUser = func(_ context.Context, id string) (*clerk.User, error) {
		name := "Provisioned User"
		return &clerk.User{
			ID:        id,
			FirstName: &name,
		}, nil
	}

	const clerkID = "c_provision_dup"
	first, err := provisionUser(clerkID)
	if err != nil {
		t.Fatalf("first provision: %v", err)
	}
	second, err := provisionUser(clerkID)
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE clerk_user_id = ?`, clerkID).Scan(&count); err != nil || count != 1 {
		t.Errorf("user rows for clerk id = %d, want 1 (err=%v)", count, err)
	}
}

func TestProvisionUserReadsBirthdayFromMetadata(t *testing.T) {
	orig := clerkUser
	defer func() { clerkUser = orig }()
	clerkUser = func(_ context.Context, id string) (*clerk.User, error) {
		name := "Meta User"
		return &clerk.User{
			ID:             id,
			FirstName:      &name,
			PublicMetadata: json.RawMessage(`{"birthday":"04-21"}`),
		}, nil
	}

	u, err := provisionUser("c_meta_user")
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

	invalid := func(_ context.Context, id string) (*clerk.User, error) {
		name := "Bad Meta"
		return &clerk.User{
			ID:             id,
			FirstName:      &name,
			PublicMetadata: json.RawMessage(`{"birthday":"not-a-date"}`),
		}, nil
	}
	clerkUser = invalid
	bad, err := provisionUser("c_badmeta_user")
	if err != nil {
		t.Fatalf("provisionUser (invalid metadata): %v", err)
	}
	if bad.Birthday != "" {
		t.Errorf("birthday = %q, want empty when metadata is invalid", bad.Birthday)
	}
}

// TestMigrateUUID verifies the integer→UUID migration rebuilds every table and
// remaps foreign keys in lockstep.
func TestMigrateUUID(t *testing.T) {
	dir := t.TempDir()
	mig, err := sql.Open("sqlite", filepath.Join(dir, "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mig.Close()
	oldSchema := `
CREATE TABLE users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	clerk_user_id TEXT NOT NULL UNIQUE,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	is_admin INTEGER NOT NULL DEFAULT 0,
	birthday TEXT NOT NULL DEFAULT '',
	theme_style TEXT NOT NULL DEFAULT 'kinetic',
	share_code TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE lists (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	label TEXT NOT NULL,
	archived_at TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	url TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	removed_at TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE claims (
	item_id INTEGER PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	claimed_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE connections (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	other_user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	PRIMARY KEY (user_id, other_user_id)
);
`
	if _, err := mig.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	// Seed an old-style graph: two connected users, one with an item the other claims.
	if _, err := mig.Exec(`INSERT INTO users (clerk_user_id, username, display_name) VALUES ('m_a', 'a', 'A'), ('m_b', 'b', 'B')`); err != nil {
		t.Fatal(err)
	}
	if _, err := mig.Exec(`INSERT INTO lists (user_id, label) VALUES (1, 'Active'), (2, 'Active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := mig.Exec(`INSERT INTO items (list_id, title) VALUES (1, 'present')`); err != nil {
		t.Fatal(err)
	}
	if _, err := mig.Exec(`INSERT INTO claims (item_id, user_id) VALUES (1, 2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := mig.Exec(`INSERT INTO connections (user_id, other_user_id) VALUES (1, 2), (2, 1)`); err != nil {
		t.Fatal(err)
	}

	prev := db
	db = mig
	defer func() { db = prev }()

	migrate()

	// users were rebuilt with UUID keys.
	var uidA string
	if err := mig.QueryRow(`SELECT id FROM users WHERE clerk_user_id = 'm_a'`).Scan(&uidA); err != nil {
		t.Fatalf("load migrated user: %v", err)
	}
	if _, err := uuid.Parse(uidA); err != nil {
		t.Fatalf("user id %q is not a UUID after migration", uidA)
	}

	// lists.user_id remapped to the new user uuid.
	var listOwner string
	if err := mig.QueryRow(`SELECT user_id FROM lists WHERE label = 'Active' ORDER BY rowid LIMIT 1`).Scan(&listOwner); err != nil {
		t.Fatal(err)
	}
	if listOwner != uidA {
		t.Errorf("lists.user_id = %q, want %q", listOwner, uidA)
	}

	// items/claims/connections remapped consistently.
	var itemID, claimItem, claimUser string
	if err := mig.QueryRow(`SELECT id FROM items WHERE title = 'present'`).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(itemID); err != nil {
		t.Fatalf("item id %q is not a UUID", itemID)
	}
	if err := mig.QueryRow(`SELECT item_id, user_id FROM claims`).Scan(&claimItem, &claimUser); err != nil {
		t.Fatal(err)
	}
	if claimItem != itemID {
		t.Errorf("claim.item_id = %q, want %q", claimItem, itemID)
	}
	var uidB string
	mig.QueryRow(`SELECT id FROM users WHERE clerk_user_id = 'm_b'`).Scan(&uidB)
	if claimUser != uidB {
		t.Errorf("claim.user_id = %q, want %q", claimUser, uidB)
	}
	var connA, connB string
	if err := mig.QueryRow(`SELECT user_id, other_user_id FROM connections LIMIT 1`).Scan(&connA, &connB); err != nil {
		t.Fatal(err)
	}
	if !((connA == uidA && connB == uidB) || (connA == uidB && connB == uidA)) {
		t.Errorf("connection (%q, %q) does not link the migrated users", connA, connB)
	}
}
