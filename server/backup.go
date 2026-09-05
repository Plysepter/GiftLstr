package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite"
)

// backupConfig holds the scheduled backup settings, all configurable via the
// environment so operators can adjust schedules and retention without code
// changes.
//
//   - GIFT_AUTO_BACKUP_ENABLED: "1" enables the automatic daily backup loop
//     (default "0"/disabled).
//   - GIFT_AUTO_BACKUP_INTERVAL: how often to take a backup (default "24h").
//   - GIFT_AUTO_BACKUP_KEEP_DAILY: number of recent daily backups to retain
//     (default 7).
//   - GIFT_AUTO_BACKUP_KEEP_WEEKLY: number of weekly backups to retain beyond
//     the daily window (default 4).
//   - GIFT_BACKUP_DIR: directory to write backups into (default DATA_DIR).
type backupConfig struct {
	Enabled    bool
	Interval   time.Duration
	KeepDaily  int
	KeepWeekly int
	Dir        string
}

// backupConfigFromEnv builds the configuration from the environment, applying
// the documented defaults.
func backupConfigFromEnv(dataDir string) backupConfig {
	cfg := backupConfig{
		Enabled:    os.Getenv("GIFT_AUTO_BACKUP_ENABLED") == "1",
		Interval:   24 * time.Hour,
		KeepDaily:  7,
		KeepWeekly: 4,
		Dir:        dataDir,
	}
	if d := os.Getenv("GIFT_AUTO_BACKUP_INTERVAL"); d != "" {
		if v, err := time.ParseDuration(d); err == nil && v >= time.Minute {
			cfg.Interval = v
		}
	}
	if v, err := strconv.Atoi(os.Getenv("GIFT_AUTO_BACKUP_KEEP_DAILY")); err == nil && v >= 0 {
		cfg.KeepDaily = v
	}
	if v, err := strconv.Atoi(os.Getenv("GIFT_AUTO_BACKUP_KEEP_WEEKLY")); err == nil && v >= 0 {
		cfg.KeepWeekly = v
	}
	if d := os.Getenv("GIFT_BACKUP_DIR"); d != "" {
		cfg.Dir = d
	}
	return cfg
}

// backupDB makes an online backup of the current database to path using
// SQLite's backup API, which is safe to run while the database is in use.
func backupDB(ctx context.Context, dstPath string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(driverConn any) error {
		bc, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("driver does not support online backup")
		}
		b, err := bc.NewBackup(dstPath)
		if err != nil {
			return err
		}
		for more := true; more && ctx.Err() == nil; {
			more, err = b.Step(-1)
			if err != nil {
				b.Finish()
				return err
			}
		}
		return b.Finish()
	})
}

// backupFilename is the on-disk naming convention for a backup taken at a
// given time: backups/daily/2026-09-02T000000Z.db or backups/weekly/...
func backupFilename(t time.Time) string {
	return t.UTC().Format("2006-01-02T150405Z") + ".db"
}

// runBackup takes a single backup and prunes retired files according to the
// configured retention policy. It is safe to call from a test or the loop.
func runBackup(cfg backupConfig, now time.Time) error {
	if err := os.MkdirAll(filepath.Join(cfg.Dir, "daily"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(cfg.Dir, "weekly"), 0o755); err != nil {
		return err
	}

	// Decide whether this backup is a weekly one: on a weekly cadence (we
	// snap a weekly bucket for each interval that crosses a 7-day boundary)
	// classify past the daily window.
	weekly := isWeeklyBackup(now)

	sub := "daily"
	if weekly {
		sub = "weekly"
	}
	path := filepath.Join(cfg.Dir, sub, backupFilename(now))
	if err := backupDB(context.Background(), path); err != nil {
		return err
	}
	logInfo.Printf("backup written: %s", path)

	if err := pruneBackups(cfg, now); err != nil {
		log.Printf("backup prune: %v", err)
	}
	return nil
}

// isWeeklyBackup reports whether a backup time should be classified as weekly
// rather than daily. We treat the first backup of each UTC week (Monday) as
// weekly so pruned daily files still leave a per-week checkpoint behind.
func isWeeklyBackup(t time.Time) bool {
	utc := t.UTC()
	return utc.Weekday() == time.Monday
}

// pruneBackups removes daily backups older than the daily window and weekly
// backups older than the daily window plus the weekly window. Retention counts
// are upper bounds; files are sorted by name (which sorts by time) and the
// newest N are kept in each bucket.
func pruneBackups(cfg backupConfig, now time.Time) error {
	if cfg.KeepDaily > 0 {
		if err := pruneBucket(filepath.Join(cfg.Dir, "daily"), cfg.KeepDaily, now); err != nil {
			return err
		}
	}
	if cfg.KeepWeekly > 0 {
		if err := pruneBucket(filepath.Join(cfg.Dir, "weekly"), cfg.KeepWeekly, now); err != nil {
			return err
		}
	}
	return nil
}

func pruneBucket(dir string, keep int, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".db") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	// Keep the newest `keep`; names sort lexicographically as timestamps do
	// only if they share the same fixed-width layout, which backupFilename
	// guarantees.
	if keep >= len(names) {
		return nil
	}
	for _, name := range names[:len(names)-keep] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			log.Printf("backup prune remove %s: %v", name, err)
		}
	}
	return nil
}

// backupLoop runs on a schedule defined by cfg until the context is done. It
// runs once immediately on start to establish a baseline, then on the set
// interval.
func backupLoop(ctx context.Context, cfg backupConfig) {
	if !cfg.Enabled {
		return
	}
	run := func() {
		if err := runBackup(cfg, time.Now()); err != nil {
			log.Printf("backup: %v", err)
		}
	}
	run()
	t := time.NewTicker(cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
