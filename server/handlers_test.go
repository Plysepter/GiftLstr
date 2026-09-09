package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

var testDB *sql.DB

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gr-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	testDB, err = sql.Open("sqlite", filepath.Join(dir, "test.db")+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		panic(err)
	}
	schema, _ := schemaFS.ReadFile("schema.sql")
	if _, err := testDB.Exec(string(schema)); err != nil {
		panic(err)
	}
	db = testDB

	// Route auth lookups through the test DB instead of an identity provider.
	currentUserFn = func(r *http.Request) (*user, error) {
		authID := r.Header.Get("X-Test-User")
		if authID == "" {
			return nil, fmt.Errorf("no session token")
		}
		return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE auth_user_id = ?`, authID))
	}

	code := m.Run()
	testDB.Close()
	os.Exit(code)
}

// ctxWithUser returns a request carrying the given user in its context,
// bypassing identity-provider authentication.
func ctxWithUser(r *http.Request, u *user) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userCtxKey, u))
}

type body = map[string]any

func doJSON(t *testing.T, u *user, method, path string, reqBody any) (*httptest.ResponseRecorder, body) {
	t.Helper()
	var reader *strings.Reader
	if reqBody != nil {
		b, _ := json.Marshal(reqBody)
		reader = strings.NewReader(string(b))
	} else {
		reader = strings.NewReader("")
	}
	r := httptest.NewRequest(method, path, reader)
	if u != nil {
		r.Header.Set("X-Test-User", u.AuthUserID)
	}
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	registerAPI(mux)
	mux.ServeHTTP(w, r)
	var resp body
	if err := json.NewDecoder(w.Result().Body).Decode(&resp); err != nil {
		resp = nil
	}
	return w, resp
}

func mustStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d", w.Code, want)
	}
}

// seedUser inserts a local user row directly and returns it.
func seedUser(t *testing.T, authID, username string, isAdmin bool) *user {
	t.Helper()
	id := newID()
	if _, err := db.Exec(`INSERT INTO users (id, auth_user_id, username, display_name, is_admin, birthday) VALUES (?, ?, ?, ?, ?, '01-01')`,
		id, authID, username, username, isAdmin); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO lists (id, user_id, label) VALUES (?, ?, 'Active')`, newID(), id); err != nil {
		t.Fatalf("seed list: %v", err)
	}
	return &user{ID: id, AuthUserID: authID, Username: username, DisplayName: username, IsAdmin: isAdmin, Birthday: "01-01", AllowSuggestions: true}
}

func activeList(t *testing.T, userID string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT id FROM lists WHERE user_id = ? AND archived_at IS NULL`, userID).Scan(&id); err != nil {
		t.Fatalf("active list: %v", err)
	}
	return id
}

func addItemRow(t *testing.T, listID, title string) string {
	t.Helper()
	id := newID()
	if _, err := db.Exec(`INSERT INTO items (id, list_id, title) VALUES (?, ?, ?)`, id, listID, title); err != nil {
		t.Fatalf("add item: %v", err)
	}
	return id
}

func connectUsers(t *testing.T, a, b string) {
	t.Helper()
	for _, pair := range [][2]string{{a, b}, {b, a}} {
		if _, err := db.Exec(`INSERT OR IGNORE INTO connections (user_id, other_user_id) VALUES (?, ?)`, pair[0], pair[1]); err != nil {
			t.Fatalf("connect: %v", err)
		}
	}
}

func TestCanView(t *testing.T) {
	alice := seedUser(t, "c_alice_cv", "alice_cv", false)
	bob := seedUser(t, "c_bob_cv", "bob_cv", false)
	admin := seedUser(t, "c_admin_cv", "admin_cv", true)

	if !canView(alice.ID, false, alice.ID) {
		t.Error("user should always view own list")
	}
	if canView(alice.ID, false, bob.ID) {
		t.Error("unconnected user should not view other's list")
	}
	connectUsers(t, alice.ID, bob.ID)
	if !canView(alice.ID, false, bob.ID) {
		t.Error("connected user should view other's list")
	}
	if !canView(admin.ID, true, bob.ID) {
		t.Error("admin should view any list")
	}
}

func TestActiveListIDCreatesMissingList(t *testing.T) {
	u := seedUser(t, "c_active", "active_user", false)
	db.Exec(`DELETE FROM lists WHERE user_id = ?`, u.ID)
	id, err := activeListID(u.ID)
	if err != nil || id == "" {
		t.Fatalf("expected new active list, got %q, %v", id, err)
	}
}

func TestProfileUpdateBirthday(t *testing.T) {
	u := seedUser(t, "c_profile", "profile_user", false)

	_, resp := doJSON(t, u, http.MethodPatch, "/api/profile", body{"birthday": "07-04"})
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp)
	}
	var got string
	db.QueryRow(`SELECT birthday FROM users WHERE id = ?`, u.ID).Scan(&got)
	if got != "07-04" {
		t.Errorf("birthday = %q, want 07-04", got)
	}

	// Invalid format rejected.
	w, _ := doJSON(t, u, http.MethodPatch, "/api/profile", body{"birthday": "junk"})
	mustStatus(t, w, http.StatusBadRequest)
	w, _ = doJSON(t, u, http.MethodPatch, "/api/profile", body{"birthday": "13-99"})
	mustStatus(t, w, http.StatusBadRequest)
}

func TestProfileUpdateHolidayResetAndSuggestions(t *testing.T) {
	u := seedUser(t, "c_profile_hol", "profile_hol_user", false)
	u.ThemeStyle = "aurora"

	_, resp := doJSON(t, u, http.MethodPatch, "/api/profile", body{"holidayReset": "mothers"})
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp)
	}
	var got string
	db.QueryRow(`SELECT holiday_reset FROM users WHERE id = ?`, u.ID).Scan(&got)
	if got != "mothers" {
		t.Errorf("holiday_reset = %q, want mothers", got)
	}

	// Clearing works.
	_, resp = doJSON(t, u, http.MethodPatch, "/api/profile", body{"holidayReset": ""})
	if resp["ok"] != true {
		t.Errorf("expected ok=true when clearing holidayReset, got %v", resp)
	}
	db.QueryRow(`SELECT holiday_reset FROM users WHERE id = ?`, u.ID).Scan(&got)
	if got != "" {
		t.Errorf("holiday_reset = %q, want empty after clearing", got)
	}

	// Unsupported values rejected.
	w, _ := doJSON(t, u, http.MethodPatch, "/api/profile", body{"holidayReset": "solstice"})
	mustStatus(t, w, http.StatusBadRequest)

	// allowSuggestions toggle persists.
	_, resp = doJSON(t, u, http.MethodPatch, "/api/profile", body{"allowSuggestions": false})
	if resp["ok"] != true {
		t.Errorf("expected ok=true toggling allowSuggestions, got %v", resp)
	}
	var flag bool
	db.QueryRow(`SELECT allow_suggestions FROM users WHERE id = ?`, u.ID).Scan(&flag)
	if flag {
		t.Error("allow_suggestions should be 0 after disabling")
	}

	// An empty update is still rejected.
	w, _ = doJSON(t, u, http.MethodPatch, "/api/profile", body{})
	mustStatus(t, w, http.StatusBadRequest)
}

func TestAddSuggestionOnOthersList(t *testing.T) {
	owner := seedUser(t, "c_sug_owner", "sug_owner", false)
	viewer := seedUser(t, "c_sug_viewer", "sug_viewer", false)
	stranger := strangerUser(t)
	connectUsers(t, owner.ID, viewer.ID)

	// Strangers may not suggest.
	w, _ := doJSON(t, stranger, http.MethodPost, fmt.Sprintf("/api/users/%s/suggestions", owner.ID), body{"title": "Bob"})
	mustStatus(t, w, http.StatusForbidden)

	// Owners may not suggest to their own list.
	w, _ = doJSON(t, owner, http.MethodPost, fmt.Sprintf("/api/users/%s/suggestions", owner.ID), body{"title": "Me"})
	mustStatus(t, w, http.StatusBadRequest)

	// A connected viewer can suggest an item.
	w, resp := doJSON(t, viewer, http.MethodPost, fmt.Sprintf("/api/users/%s/suggestions", owner.ID), body{"title": "Headphones", "url": "https://x.co"})
	mustStatus(t, w, http.StatusCreated)
	itemID, _ := resp["id"].(string)
	if itemID == "" {
		t.Fatal("expected a suggestion item id in response")
	}
	var visibility, suggestedBy, suggestedByID string
	if err := db.QueryRow(`SELECT visibility, suggested_by, suggested_by_id FROM items WHERE id = ?`, itemID).Scan(&visibility, &suggestedBy, &suggestedByID); err != nil {
		t.Fatal(err)
	}
	if visibility != "suggested" || suggestedBy != "sug_viewer" || suggestedByID != viewer.ID {
		t.Errorf("suggestion tags = (%q, %q, %q), want (suggested, sug_viewer, %s)", visibility, suggestedBy, suggestedByID, viewer.ID)
	}

	// The owner sees the suggestion on their list.
	_, resp = doJSON(t, owner, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	sawSuggestion := false
	if resp["items"] != nil {
		for _, raw := range resp["items"].([]any) {
			it := raw.(map[string]any)
			if it["id"].(string) == itemID && it["suggestedBy"] == "sug_viewer" {
				sawSuggestion = true
			}
		}
	}
	if !sawSuggestion {
		t.Error("owner should see the viewer's suggestion on their list")
	}

	// Blank title rejected.
	w, _ = doJSON(t, viewer, http.MethodPost, fmt.Sprintf("/api/users/%s/suggestions", owner.ID), body{"title": "  "})
	mustStatus(t, w, http.StatusBadRequest)

	// Unknown owner 404s (even for an admin who bypasses the canView check).
	admin := seedUser(t, "c_sug_admin", "sug_admin", true)
	w, _ = doJSON(t, admin, http.MethodPost, "/api/users/00000000-0000-0000-0000-000000000000/suggestions", body{"title": "Ghost"})
	mustStatus(t, w, http.StatusNotFound)

	// Disabled suggestions are enforced server-side.
	db.Exec(`UPDATE users SET allow_suggestions = 0 WHERE id = ?`, viewer.ID)
	w, resp = doJSON(t, viewer, http.MethodPost, fmt.Sprintf("/api/users/%s/suggestions", owner.ID), body{"title": "Nope"})
	mustStatus(t, w, http.StatusForbidden)
	if resp["error"] == nil || !strings.Contains(resp["error"].(string), "disabled") {
		t.Errorf("disabled suggestion error = %v, want message mentioning disabled", resp["error"])
	}
}

func TestItemsCRUDOnOwnList(t *testing.T) {
	u := seedUser(t, "c_items", "items_user", false)

	// Add
	w, resp := doJSON(t, u, http.MethodPost, "/api/my/items", body{"title": "Bike", "url": "https://x.co", "notes": "red"})
	mustStatus(t, w, http.StatusCreated)
	itemID, ok := resp["id"].(string)
	if !ok {
		t.Fatal("expected item id in response")
	}

	// Add without title rejected
	w, _ = doJSON(t, u, http.MethodPost, "/api/my/items", body{"title": "  "})
	mustStatus(t, w, http.StatusBadRequest)

	// Visible via user list
	w, resp = doJSON(t, u, http.MethodGet, fmt.Sprintf("/api/users/%s/list", u.ID), nil)
	mustStatus(t, w, http.StatusOK)
	items := resp["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	// Edit
	w, _ = doJSON(t, u, http.MethodPatch, fmt.Sprintf("/api/my/items/%s", itemID), body{"title": "Blue bike"})
	mustStatus(t, w, http.StatusOK)
	var title string
	db.QueryRow(`SELECT title FROM items WHERE id = ?`, itemID).Scan(&title)
	if title != "Blue bike" {
		t.Errorf("title = %q, want Blue bike", title)
	}

	// Soft delete
	w, _ = doJSON(t, u, http.MethodDelete, fmt.Sprintf("/api/my/items/%s", itemID), nil)
	mustStatus(t, w, http.StatusOK)
	var removed sql.NullString
	db.QueryRow(`SELECT removed_at FROM items WHERE id = ?`, itemID).Scan(&removed)
	if !removed.Valid {
		t.Error("expected soft-deleted item to have removed_at set")
	}

	// Editing a removed item fails
	w, _ = doJSON(t, u, http.MethodPatch, fmt.Sprintf("/api/my/items/%s", itemID), body{"title": "nope"})
	mustStatus(t, w, http.StatusNotFound)
}

func TestItemCap(t *testing.T) {
	u := seedUser(t, "c_cap", "cap_user", false)
	listID := activeList(t, u.ID)

	for i := 0; i < maxListItems; i++ {
		addItemRow(t, listID, fmt.Sprintf("Item %d", i))
	}

	// At the cap, new adds are rejected.
	w, resp := doJSON(t, u, http.MethodPost, "/api/my/items", body{"title": "Over"})
	mustStatus(t, w, http.StatusBadRequest)
	if resp == nil || resp["error"] == nil {
		t.Error("expected an error message at the item cap")
	}

	var removedID string
	db.QueryRow(`SELECT id FROM items WHERE list_id = ? AND removed_at IS NULL ORDER BY id LIMIT 1`, listID).Scan(&removedID)

	// Soft-removing an item frees a slot; restoring it fits again.
	w, _ = doJSON(t, u, http.MethodDelete, fmt.Sprintf("/api/my/items/%s", removedID), nil)
	mustStatus(t, w, http.StatusOK)
	w, _ = doJSON(t, u, http.MethodPost, fmt.Sprintf("/api/my/items/%s/restore", removedID), nil)
	mustStatus(t, w, http.StatusOK)

	// Removing again, filling the slot with a new item, then restoring the
	// removed one would exceed the cap and is rejected.
	w, _ = doJSON(t, u, http.MethodDelete, fmt.Sprintf("/api/my/items/%s", removedID), nil)
	mustStatus(t, w, http.StatusOK)
	w, _ = doJSON(t, u, http.MethodPost, "/api/my/items", body{"title": "Fill"})
	mustStatus(t, w, http.StatusCreated)
	w, _ = doJSON(t, u, http.MethodPost, fmt.Sprintf("/api/my/items/%s/restore", removedID), nil)
	mustStatus(t, w, http.StatusBadRequest)
}

func TestClaimLifecycleAndPrivacy(t *testing.T) {
	owner := seedUser(t, "c_claim_owner", "claim_owner", false)
	viewer := seedUser(t, "c_claim_viewer", "claim_viewer", false)
	stranger := seedUser(t, "c_claim_stranger", "claim_stranger", false)
	connectUsers(t, owner.ID, viewer.ID)

	itemID := addItemRow(t, activeList(t, owner.ID), "Mug")
	path := fmt.Sprintf("/api/items/%s/claim", itemID)

	// Stranger (unconnected) cannot claim.
	w, _ := doJSON(t, stranger, http.MethodPut, path, nil)
	mustStatus(t, w, http.StatusForbidden)

	// Owner cannot claim own item.
	w, _ = doJSON(t, owner, http.MethodPut, path, nil)
	mustStatus(t, w, http.StatusForbidden)

	// Connected viewer claims.
	w, _ = doJSON(t, viewer, http.MethodPut, path, nil)
	mustStatus(t, w, http.StatusOK)

	// Duplicate claim by another connected user conflicts.
	other := seedUser(t, "c_claim_other", "claim_other", false)
	connectUsers(t, owner.ID, other.ID)
	w, _ = doJSON(t, other, http.MethodPut, path, nil)
	mustStatus(t, w, http.StatusConflict)

	// Re-claiming your own claim is idempotent.
	w, _ = doJSON(t, viewer, http.MethodPut, path, nil)
	mustStatus(t, w, http.StatusOK)

	// Viewer sees claimedByMe but not the claimer name; owner sees the name.
	_, resp := doJSON(t, viewer, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	item := resp["items"].([]any)[0].(map[string]any)
	if item["claimedByMe"] != true {
		t.Error("viewer should see claimedByMe")
	}
	if _, exposed := item["claimedByName"]; exposed {
		t.Error("claimer name should be hidden from viewers on live lists")
	}

	_, resp = doJSON(t, owner, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	item = resp["items"].([]any)[0].(map[string]any)
	if item["claimedByName"] != "claim_viewer" {
		t.Errorf("owner should see claimer name, got %v", item["claimedByName"])
	}

	// Unclaim by non-claimer fails.
	w, _ = doJSON(t, other, http.MethodDelete, path, nil)
	mustStatus(t, w, http.StatusNotFound)

	// Unclaim works.
	w, _ = doJSON(t, viewer, http.MethodDelete, path, nil)
	mustStatus(t, w, http.StatusOK)

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM claims WHERE item_id = ?`, itemID).Scan(&n)
	if n != 0 {
		t.Errorf("expected claim removed, found %d", n)
	}
}

func TestAdminRevealClaimers(t *testing.T) {
	owner := seedUser(t, "c_reveal_owner", "reveal_owner", false)
	claimer := seedUser(t, "c_reveal_claimer", "reveal_claimer", false)
	admin := seedUser(t, "c_reveal_admin", "reveal_admin", true)

	connectUsers(t, owner.ID, claimer.ID)
	itemID := addItemRow(t, activeList(t, owner.ID), "Book")
	doJSON(t, claimer, http.MethodPut, fmt.Sprintf("/api/items/%s/claim", itemID), nil)

	// Admin viewing another user's live list keeps claimers hidden by default.
	w, resp := doJSON(t, admin, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	mustStatus(t, w, http.StatusOK)
	item := resp["items"].([]any)[0].(map[string]any)
	if _, exposed := item["claimedByName"]; exposed {
		t.Error("claimer name should be hidden from admins by default on live lists")
	}

	// Admin can opt in to see who claimed on a live list.
	w, resp = doJSON(t, admin, http.MethodGet, fmt.Sprintf("/api/users/%s/list?revealClaimers=1", owner.ID), nil)
	mustStatus(t, w, http.StatusOK)
	item = resp["items"].([]any)[0].(map[string]any)
	if item["claimedByName"] != "reveal_claimer" {
		t.Errorf("admin should see claimer name with revealClaimers, got %v", item["claimedByName"])
	}

	// Non-admin viewers may not opt in, even when connected.
	connected := seedUser(t, "c_reveal_connected", "reveal_connected", false)
	connectUsers(t, owner.ID, connected.ID)
	w, resp = doJSON(t, connected, http.MethodGet, fmt.Sprintf("/api/users/%s/list?revealClaimers=1", owner.ID), nil)
	mustStatus(t, w, http.StatusOK)
	item = resp["items"].([]any)[0].(map[string]any)
	if _, exposed := item["claimedByName"]; exposed {
		t.Error("connected non-admin should not reveal claimers via revealClaimers")
	}
}

func TestRemovedItemStillVisibleToClaimer(t *testing.T) {
	owner := seedUser(t, "c_removed_owner", "removed_owner", false)
	fan := seedUser(t, "c_removed_fan", "removed_fan", false)
	connectUsers(t, owner.ID, fan.ID)

	itemID := addItemRow(t, activeList(t, owner.ID), "Kettle")
	path := fmt.Sprintf("/api/items/%s/claim", itemID)
	doJSON(t, fan, http.MethodPut, path, nil)

	// Owner removes the claimed item.
	doJSON(t, owner, http.MethodDelete, fmt.Sprintf("/api/my/items/%s", itemID), nil)

	// Claimer still sees it (with removed badge).
	_, resp := doJSON(t, fan, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	items := resp["items"].([]any)
	found := false
	for _, raw := range items {
		it := raw.(map[string]any)
		if it["id"].(string) == itemID {
			found = true
			if it["removedAt"] == nil {
				t.Error("claimed+removed item should expose removedAt to the claimer")
			}
		}
	}
	if !found {
		t.Error("claimer should still see removed item")
	}

	// Unconnected user never sees it.
	_, resp = doJSON(t, strangerUser(t), http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	if resp != nil && resp["items"] != nil {
		for _, raw := range resp["items"].([]any) {
			if raw.(map[string]any)["id"].(string) == itemID {
				t.Error("removed unclaimed item should not appear to others")
			}
		}
	}
}

var strangerCounter int

func strangerUser(t *testing.T) *user {
	strangerCounter++
	return seedUser(t, fmt.Sprintf("c_stranger_%d", strangerCounter), fmt.Sprintf("stranger_user_%d", strangerCounter), false)
}

func TestUserListAccessControl(t *testing.T) {
	owner := seedUser(t, "c_access_owner", "access_owner", false)
	stranger := strangerUser(t)
	admin := seedUser(t, "c_access_admin", "access_admin", true)

	w, _ := doJSON(t, stranger, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	mustStatus(t, w, http.StatusForbidden)

	// Admin may access any list.
	w, _ = doJSON(t, admin, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	mustStatus(t, w, http.StatusOK)

	// Unknown user 404s.
	w, _ = doJSON(t, admin, http.MethodGet, "/api/users/00000000-0000-0000-0000-000000000000/list", nil)
	mustStatus(t, w, http.StatusNotFound)

	// A non-numeric id is still a valid path but matches nothing.
	w, _ = doJSON(t, admin, http.MethodGet, "/api/users/notanumber/list", nil)
	mustStatus(t, w, http.StatusNotFound)
}

func TestListsPaginationAndClaimedLabel(t *testing.T) {
	me := seedUser(t, "c_lists_me", "lists_me", false)
	friend := seedUser(t, "c_lists_friend", "lists_friend", false)
	connectUsers(t, me.ID, friend.ID)

	itemID := addItemRow(t, activeList(t, friend.ID), "Gift")
	doJSON(t, me, http.MethodPut, fmt.Sprintf("/api/items/%s/claim", itemID), nil)

	w, resp := doJSON(t, me, http.MethodGet, "/api/lists?page=1", nil)
	mustStatus(t, w, http.StatusOK)
	users := resp["users"].([]any)
	if len(users) == 0 {
		t.Fatal("expected at least one listed user")
	}
	var friendEntry map[string]any
	for _, raw := range users {
		u := raw.(map[string]any)
		if u["id"].(string) == friend.ID {
			friendEntry = u
		}
	}
	if friendEntry == nil {
		t.Fatal("friend missing from lists page")
	}
	if friendEntry["claimedLabel"] != "claimed 1 items" {
		t.Errorf("claimedLabel = %v", friendEntry["claimedLabel"])
	}
	if friendEntry["canView"] != true {
		t.Error("connected user should be viewable")
	}
	if resp["hasMore"] != false {
		t.Error("unexpected hasMore on single page")
	}
}

func TestArchiveAccessAndDetail(t *testing.T) {
	u := seedUser(t, "c_archive", "archive_user", false)
	listID := activeList(t, u.ID)
	addItemRow(t, listID, "Thing")

	// Archive manually with a claim so detail shows names.
	db.Exec(`UPDATE lists SET archived_at = datetime('now'), label = '2026-01-01' WHERE id = ?`, listID)
	itemID := ""
	db.QueryRow(`SELECT id FROM items WHERE list_id = ?`, listID).Scan(&itemID)
	db.Exec(`INSERT INTO claims (item_id, user_id) VALUES (?, ?)`, itemID, u.ID)

	// Index shows the archive.
	w, resp := doJSON(t, u, http.MethodGet, "/api/archive", nil)
	mustStatus(t, w, http.StatusOK)
	lists := resp["lists"].([]any)
	if len(lists) != 1 {
		t.Fatalf("expected 1 archived list, got %d", len(lists))
	}

	// Detail exposes claimer name.
	w, resp = doJSON(t, u, http.MethodGet, fmt.Sprintf("/api/archive/%s", listID), nil)
	mustStatus(t, w, http.StatusOK)
	items := resp["items"].([]any)[0].(map[string]any)
	if items["claimedByName"] != "archive_user" {
		t.Errorf("archive should show who claimed what, got %v", items["claimedByName"])
	}

	// Another user cannot read someone else's archive.
	other := strangerUser(t)
	w, _ = doJSON(t, other, http.MethodGet, fmt.Sprintf("/api/archive/%s", listID), nil)
	mustStatus(t, w, http.StatusNotFound)

	// Active lists are never exposed as archives.
	active := activeList(t, other.ID)
	w, _ = doJSON(t, other, http.MethodGet, fmt.Sprintf("/api/archive/%s", active), nil)
	mustStatus(t, w, http.StatusNotFound)
}

func TestArchiveIndexPagination(t *testing.T) {
	u := seedUser(t, "c_archpg", "archpg_user", false)
	listID := activeList(t, u.ID)
	if _, err := db.Exec(`UPDATE lists SET archived_at = datetime('now'), label = 'A' WHERE id = ?`, listID); err != nil {
		t.Fatalf("archive first list: %v", err)
	}
	for i := 0; i < 24; i++ {
		if _, err := db.Exec(`INSERT INTO lists (id, user_id, label, archived_at) VALUES (?, ?, 'A', datetime('now'))`, newID(), u.ID); err != nil {
			t.Fatalf("insert archived list: %v", err)
		}
	}

	w, resp := doJSON(t, u, http.MethodGet, "/api/archive?page=1", nil)
	mustStatus(t, w, http.StatusOK)
	if n := len(resp["lists"].([]any)); n != 20 {
		t.Errorf("page 1 should return 20 lists, got %d", n)
	}
	if resp["hasMore"] != true || resp["total"] != float64(25) || resp["page"] != float64(1) {
		t.Errorf("unexpected page 1 meta: %v", resp)
	}

	w, resp = doJSON(t, u, http.MethodGet, "/api/archive?page=2", nil)
	mustStatus(t, w, http.StatusOK)
	if n := len(resp["lists"].([]any)); n != 5 {
		t.Errorf("page 2 should return 5 lists, got %d", n)
	}
	if resp["hasMore"] != false || resp["total"] != float64(25) || resp["page"] != float64(2) {
		t.Errorf("unexpected page 2 meta: %v", resp)
	}
}

func TestConnectionsAdminOnly(t *testing.T) {
	a := seedUser(t, "c_conn_a", "conn_a", false)
	b := seedUser(t, "c_conn_b", "conn_b", false)
	admin := seedUser(t, "c_conn_admin", "conn_admin", true)

	reqBody := body{"a": a.ID, "b": b.ID, "add": true}
	w, _ := doJSON(t, a, http.MethodPut, "/api/connections", reqBody)
	mustStatus(t, w, http.StatusForbidden)

	w, _ = doJSON(t, admin, http.MethodPut, "/api/connections", reqBody)
	mustStatus(t, w, http.StatusOK)

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM connections WHERE (user_id=? AND other_user_id=?) OR (user_id=? AND other_user_id=?)`,
		a.ID, b.ID, b.ID, a.ID).Scan(&n)
	if n != 2 {
		t.Errorf("expected bidirectional connection, found %d rows", n)
	}

	// Self-connection rejected.
	w, _ = doJSON(t, admin, http.MethodPut, "/api/connections", body{"a": a.ID, "b": a.ID, "add": true})
	mustStatus(t, w, http.StatusBadRequest)

	// Remove clears both directions.
	w, _ = doJSON(t, admin, http.MethodPut, "/api/connections", body{"a": a.ID, "b": b.ID, "add": false})
	mustStatus(t, w, http.StatusOK)
	db.QueryRow(`SELECT COUNT(*) FROM connections WHERE user_id IN (?, ?) AND other_user_id IN (?, ?)`, a.ID, b.ID, a.ID, b.ID).Scan(&n)
	if n != 0 {
		t.Errorf("expected connection removed, found %d", n)
	}
}

func TestAdminUpdateUserRequiresAdmin(t *testing.T) {
	target := seedUser(t, "c_admu_target", "admu_target", false)
	pleb := seedUser(t, "c_admu_pleb", "admu_pleb", false)
	admin := seedUser(t, "c_admu_admin", "admu_admin", true)

	w, _ := doJSON(t, pleb, http.MethodPatch, fmt.Sprintf("/api/admin/users/%s", target.ID), body{"birthday": "02-02"})
	mustStatus(t, w, http.StatusForbidden)

	w, _ = doJSON(t, admin, http.MethodPatch, fmt.Sprintf("/api/admin/users/%s", target.ID), body{"birthday": "02-02"})
	mustStatus(t, w, http.StatusOK)
	var got string
	db.QueryRow(`SELECT birthday FROM users WHERE id = ?`, target.ID).Scan(&got)
	if got != "02-02" {
		t.Errorf("birthday = %q, want 02-02", got)
	}

	isAdmin := true
	w, _ = doJSON(t, admin, http.MethodPatch, fmt.Sprintf("/api/admin/users/%s", target.ID), body{"isAdmin": isAdmin})
	mustStatus(t, w, http.StatusOK)
	var flag bool
	db.QueryRow(`SELECT is_admin FROM users WHERE id = ?`, target.ID).Scan(&flag)
	if !flag {
		t.Error("expected target to become admin")
	}

	// Invalid birthday rejected even for admins.
	w, _ = doJSON(t, admin, http.MethodPatch, fmt.Sprintf("/api/admin/users/%s", target.ID), body{"birthday": "00-00"})
	mustStatus(t, w, http.StatusBadRequest)
}

func TestAuthRequiredForProtectedEndpoints(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/me"},
		{http.MethodGet, "/api/lists"},
		{http.MethodPost, "/api/my/items"},
		{http.MethodPut, "/api/items/1/claim"},
		{http.MethodGet, "/api/admin/users"},
	} {
		w, resp := doJSON(t, nil, tc.method, tc.path, nil)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", tc.method, tc.path, w.Code)
		}
		if resp == nil || resp["error"] != "not logged in" {
			t.Errorf("%s %s: unexpected error payload %v", tc.method, tc.path, resp)
		}
	}
}

func TestValidItemURL(t *testing.T) {
	valid := []string{"", "https://example.com/x", "http://example.com"}
	for _, s := range valid {
		if !validItemURL(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	invalid := []string{"google.com", "<a href=\"google.com\">click</a>", "javascript:alert(1)", "ftp://x.com/a", "/relative/path", "https://"}
	for _, s := range invalid {
		if validItemURL(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}

func TestAddItemRejectsInvalidURL(t *testing.T) {
	u := seedUser(t, "c_badurl", "badurl_user", false)
	w, _ := doJSON(t, u, http.MethodPost, "/api/my/items", body{"title": "X", "url": "google.com"})
	mustStatus(t, w, http.StatusBadRequest)
	w, _ = doJSON(t, u, http.MethodPost, "/api/my/items", body{"title": "X", "url": "https://ok.example.com"})
	mustStatus(t, w, http.StatusCreated)
}

// TestSQLInjectionStoredAsData verifies that SQL metacharacters in user input
// are treated as literal data by the parameterized queries.
func TestSQLInjectionStoredAsData(t *testing.T) {
	u := seedUser(t, "c_inj", "inj_user", false)

	payloads := []string{
		"x'; DROP TABLE items; --",
		"' OR '1'='1",
		`"'; DELETE FROM users WHERE '1'='1`,
	}
	for _, p := range payloads {
		w, resp := doJSON(t, u, http.MethodPost, "/api/my/items", body{"title": p})
		if w.Code != http.StatusCreated {
			t.Fatalf("payload %q: status=%d resp=%v", p, w.Code, resp)
		}
	}

	var n int
	db.QueryRow(`SELECT COUNT(*) FROM items WHERE list_id = ?`, activeList(t, u.ID)).Scan(&n)
	if n != len(payloads) {
		t.Errorf("expected %d rows stored literally, found %d", len(payloads), n)
	}

	for _, table := range []string{"items", "users"} {
		if _, err := db.Exec(`SELECT 1 FROM ` + table + ` LIMIT 1`); err != nil {
			t.Errorf("table %s was dropped/corrupted: %v", table, err)
		}
	}
}

func TestShareCodeEnableDisable(t *testing.T) {
	u := seedUser(t, "c_share_me", "share_me", false)

	// Disabled by default: no stored code.
	w, resp := doJSON(t, u, http.MethodPut, "/api/connect/share", body{"enabled": false})
	mustStatus(t, w, http.StatusOK)
	if resp["shareCode"] != "" {
		t.Errorf("disable should clear the code, got %v", resp["shareCode"])
	}
	var stored string
	db.QueryRow(`SELECT share_code FROM users WHERE id = ?`, u.ID).Scan(&stored)
	if stored != "" {
		t.Errorf("share_code = %q, want empty after disable", stored)
	}

	// Enabling generates a code matching the share charset and persists it.
	w, resp = doJSON(t, u, http.MethodPut, "/api/connect/share", body{"enabled": true})
	mustStatus(t, w, http.StatusOK)
	code, _ := resp["shareCode"].(string)
	if !validShareCode(code) {
		t.Fatalf("expected an 8-char share code, got %q", code)
	}
	db.QueryRow(`SELECT share_code FROM users WHERE id = ?`, u.ID).Scan(&stored)
	if stored != code {
		t.Errorf("persisted code = %q, want %q", stored, code)
	}

	// Re-enabling rotates the code.
	w, resp = doJSON(t, u, http.MethodPut, "/api/connect/share", body{"enabled": true})
	mustStatus(t, w, http.StatusOK)
	rotated, _ := resp["shareCode"].(string)
	if rotated == code || !validShareCode(rotated) {
		t.Errorf("expected a fresh code after re-enable, got %q", rotated)
	}

	// Bad body rejected.
	w, _ = doJSON(t, u, http.MethodPut, "/api/connect/share", nil)
	mustStatus(t, w, http.StatusBadRequest)
}

// validShareCode sanity-checks the share-code format in tests.
func validShareCode(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune(shareCodeAlphabet, c) {
			return false
		}
	}
	return true
}

func TestAcceptShareCode(t *testing.T) {
	owner := seedUser(t, "c_accept_owner", "accept_owner", false)
	good, _ := db.Exec(`UPDATE users SET share_code = 'GOODCODE' WHERE id = ?`, owner.ID)
	if good == nil {
		t.Fatal("failed to set share code")
	}

	// Missing code -> 400.
	w, _ := doJSON(t, owner, http.MethodPost, "/api/connect/accept", body{"code": "  "})
	mustStatus(t, w, http.StatusBadRequest)

	// Unknown code -> 404.
	w, _ = doJSON(t, owner, http.MethodPost, "/api/connect/accept", body{"code": "NOTFOUND"})
	mustStatus(t, w, http.StatusNotFound)

	// Self-connect -> 400.
	w, resp := doJSON(t, owner, http.MethodPost, "/api/connect/accept", body{"code": "goodcode"})
	mustStatus(t, w, http.StatusBadRequest)
	if resp["error"] == nil || !strings.Contains(resp["error"].(string), "own") {
		t.Errorf("self-connect error = %v, want message mentioning your own code", resp["error"])
	}

	// Code is uppercase/trim-insensitive.
	fan := seedUser(t, "c_accept_fan", "accept_fan", false)
	w, resp = doJSON(t, fan, http.MethodPost, "/api/connect/accept", body{"code": "  goodcode  "})
	mustStatus(t, w, http.StatusOK)
	var display string
	if resp["displayName"] != nil {
		display = resp["displayName"].(string)
	}
	if resp["ok"] != true || display != "accept_owner" {
		t.Errorf("accept response = %v, want ok=true displayName=accept_owner", resp)
	}

	// Bidirectional connection exists.
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM connections WHERE (user_id = ? AND other_user_id = ?) OR (user_id = ? AND other_user_id = ?)`,
		fan.ID, owner.ID, owner.ID, fan.ID).Scan(&n)
	if n != 2 {
		t.Errorf("expected bidirectional connections, got %d", n)
	}

	// The connected fan can now view the owner's list.
	w, _ = doJSON(t, fan, http.MethodGet, fmt.Sprintf("/api/users/%s/list", owner.ID), nil)
	mustStatus(t, w, http.StatusOK)

	// Re-accepting the code is idempotent (no error, still ok).
	w, resp = doJSON(t, fan, http.MethodPost, "/api/connect/accept", body{"code": "GOODCODE"})
	mustStatus(t, w, http.StatusOK)
	if resp["ok"] != true {
		t.Errorf("re-accept response = %v, want ok=true", resp)
	}
}

func TestDecodeRejectsOversizedBody(t *testing.T) {
	// A body larger than maxBodyBytes must be rejected rather than silently
	// truncated. Passing the real ResponseWriter to MaxBytesReader also marks
	// the connection to close, so we verify the reject behaviour end to end.
	var target struct {
		Title string `json:"title"`
	}
	big := strings.Repeat("a", int(maxBodyBytes)+1024)
	body := `{"title":"` + big + `"}`
	r := httptest.NewRequest(http.MethodPost, "/api/my/items", strings.NewReader(body))
	w := httptest.NewRecorder()
	err := decode(w, r, &target)
	if err == nil {
		t.Fatal("expected an error decoding an oversized body")
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Errorf("error = %v, want *http.MaxBytesError", err)
	}
}
