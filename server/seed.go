package main

import (
	"database/sql"
	"embed"
)

//go:embed seed.sql
var seedFS embed.FS

// applySeed replaces the entire database contents with the canonical demo
// dataset (see seed.sql). It is idempotent — every table is wiped first — so
// every boot while E2E_TEST_MODE=1 reproduces exactly the same demo state for
// the Playwright e2e suite and the docker demo compose.
func applySeed(d *sql.DB) error {
	b, err := seedFS.ReadFile("seed.sql")
	if err != nil {
		return err
	}
	if _, err := d.Exec(string(b)); err != nil {
		return err
	}
	return nil
}
