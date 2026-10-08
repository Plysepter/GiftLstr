CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	auth_user_id TEXT NOT NULL UNIQUE,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	is_admin INTEGER NOT NULL DEFAULT 0,
	birthday TEXT NOT NULL DEFAULT '',
	holiday_reset TEXT NOT NULL DEFAULT '',
	allow_suggestions INTEGER NOT NULL DEFAULT 1,
	spending_enabled INTEGER NOT NULL DEFAULT 0,
	theme_style TEXT NOT NULL DEFAULT 'aurora',
	share_code TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS lists (
	id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	label TEXT NOT NULL,
	archived_at TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	is_private INTEGER NOT NULL DEFAULT 0,
	first_name TEXT NOT NULL DEFAULT '',
	last_name TEXT NOT NULL DEFAULT '',
	birthday TEXT NOT NULL DEFAULT '',
	theme TEXT NOT NULL DEFAULT 'aurora'
);

CREATE TABLE IF NOT EXISTS items (
	id TEXT PRIMARY KEY,
	list_id TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	url TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	removed_at TEXT,
	visibility TEXT NOT NULL DEFAULT 'public',
	suggested_by TEXT NOT NULL DEFAULT '',
	suggested_by_id TEXT NOT NULL DEFAULT '',
	created_by_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS claims (
	item_id TEXT PRIMARY KEY REFERENCES items(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	price_cents INTEGER NOT NULL DEFAULT 0,
	claimed_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS connections (
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	other_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	PRIMARY KEY (user_id, other_user_id)
);

CREATE TABLE IF NOT EXISTS metadata_cache (
	url TEXT PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	image TEXT NOT NULL DEFAULT '',
	fetched_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_items_list ON items(list_id);
CREATE INDEX IF NOT EXISTS idx_lists_user ON lists(user_id);
CREATE INDEX IF NOT EXISTS idx_claims_user ON claims(user_id);
CREATE INDEX IF NOT EXISTS idx_connections_other ON connections(other_user_id);
