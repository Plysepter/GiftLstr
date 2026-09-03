package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsWeeklyBackup(t *testing.T) {
	// 2026-09-07 is a Monday.
	if !isWeeklyBackup(time.Date(2026, time.September, 7, 10, 0, 0, 0, time.UTC)) {
		t.Error("Monday should be classified as weekly")
	}
	if isWeeklyBackup(time.Date(2026, time.September, 9, 10, 0, 0, 0, time.UTC)) {
		t.Error("Wednesday should not be classified as weekly")
	}
}

func TestRunBackupCreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := backupConfig{Enabled: true, Dir: dir, KeepDaily: 7, KeepWeekly: 4}

	now := time.Date(2026, time.September, 9, 12, 0, 0, 0, time.UTC)
	if err := runBackup(cfg, now); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	daily := filepath.Join(dir, "daily")
	if _, err := os.Stat(daily); err != nil {
		t.Fatalf("daily dir: %v", err)
	}
	entries, err := os.ReadDir(daily)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 daily backup, got %d", len(entries))
	}
	// The backup file must be a valid SQLite database (non-empty).
	if info, _ := entries[0].Info(); info.Size() == 0 {
		t.Error("backup file is empty")
	}
}

func TestPruneBackupsRetainsNewest(t *testing.T) {
	dir := t.TempDir()
	daily := filepath.Join(dir, "daily")
	if err := os.MkdirAll(daily, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create 5 stale daily files (names sort oldest to newest).
	for _, day := range []string{"2026-09-01T000000Z.db", "2026-09-02T000000Z.db", "2026-09-03T000000Z.db", "2026-09-04T000000Z.db", "2026-09-05T000000Z.db"} {
		if err := os.WriteFile(filepath.Join(daily, day), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := backupConfig{Dir: dir, KeepDaily: 2, KeepWeekly: 4}
	now := time.Date(2026, time.September, 9, 12, 0, 0, 0, time.UTC)
	if err := pruneBackups(cfg, now); err != nil {
		t.Fatalf("pruneBackups: %v", err)
	}

	entries, err := os.ReadDir(daily)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 retained backups, got %d: %v", len(entries), entries)
	}
	if entries[0].Name() != "2026-09-04T000000Z.db" {
		t.Errorf("newest retained = %s, want 09-04", entries[0].Name())
	}
}
