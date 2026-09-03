# GiftLstr

GiftLstr is a wishlist app for people who never know what to ask for (or buy). Keep a list of gift ideas with links and notes, share it with friends and family, and let them "claim" items so nobody buys the same thing twice. The list resets automatically after Christmas and after your birthday.

## Features

- Per-user gift lists with title, URL, and notes (URLs get link previews)
- Claims: connected people can reserve an item so gifts aren't duplicated
- Connections: a share code lets friends/family see and claim from your list
- Automatic archiving: after Christmas/birthday, the active list is archived and a fresh one is created (unclaimed items carry over)
- Optional automatic SQLite backups (daily with retention)
- Optional per-IP API rate limiting
- Admin panel for managing users
- PWA with offline support (service-worker precaching + offline queue)
- Light/dark theme

## Tech stack

- **Frontend**: Svelte 5, Vite, TypeScript, Clerk (Google OAuth)
- **Backend**: Go, serving the built frontend as one embedded static binary plus a JSON API
- **Storage**: SQLite (pure-Go driver, WAL mode)

## Getting started

1. Create a Clerk application at https://dashboard.clerk.com and note your **secret key** and **publishable key** (API keys page).
2. Copy `.env` to `.env.local` and fill in the values. `.env.local` is git-ignored.

   ```sh
   CLERK_SECRET_KEY=sk_...
   VITE_CLERK_PUBLISHABLE_KEY=pk_...
   ADMIN_USER_IDS=          # comma-separated Clerk user IDs granted admin
   ```

3. Start with Docker:

   ```sh
   pnpm docker:up
   ```

   The app is served at http://localhost:8080. Data (the SQLite database) lives in `./data`, which is mounted into the container.

   To find your own Clerk user ID: log in once as a normal user, then open the Clerk dashboard, copy your user ID into `ADMIN_USER_IDS`, and promote yourself.

## Development

The frontend proxies `/api` to `http://localhost:8080`, so run both sides:

```sh
# Backend (Go), port 8080 — needs CLERK_SECRET_KEY in the environment
DATA_DIR=./data CLERK_SECRET_KEY=sk_... go -C server run .

# Frontend with hot reload
pnpm --dir web dev
```

### Tests

```sh
# Frontend unit tests (Vitest)
pnpm --dir web test

# Frontend check + lint + format
pnpm --dir web check   # svelte-check
pnpm --dir web lint    # oxlint
pnpm --dir web fmt:check

# End-to-end tests (Playwright). Runs its own server in E2E_TEST_MODE,
# which bypasses Clerk auth and must never be enabled in production.
pnpm e2e
```

## Configuration

| Variable                     | Purpose                                                                 |
| ---------------------------- | ------------------------------------------------------------------------ |
| `CLERK_SECRET_KEY`           | Clerk backend API key (required)                                         |
| `VITE_CLERK_PUBLISHABLE_KEY` | Clerk frontend key, baked into the web build                             |
| `ADMIN_USER_IDS`             | Comma-separated Clerk user IDs granted admin                             |
| `DATA_DIR`                   | Directory for the SQLite database (default `/data`)                      |
| `PORT`                       | HTTP listen port (default `8080`)                                        |
| `GIFT_AUTO_BACKUP_ENABLED`   | `1` enables automatic backups (default `0`)                              |
| `GIFT_AUTO_BACKUP_INTERVAL`  | Backup cadence (default `24h`)                                           |
| `GIFT_AUTO_BACKUP_KEEP_DAILY`| Daily backups to retain (default `7`)                                    |
| `GIFT_AUTO_BACKUP_KEEP_WEEKLY`| Weekly backups to retain beyond the daily window (default `4`)           |
| `GIFT_BACKUP_DIR`            | Directory for backups (default `DATA_DIR`)                               |
| `GIFT_RATE_LIMIT_ENABLED`    | `1` enables per-IP rate limiting (default `0`)                           |
| `GIFT_RATE_LIMIT`            | Requests per IP per window for normal endpoints (default `120`)          |
| `GIFT_RATE_LIMIT_STRICT`     | Requests per IP per window for metadata/share-code endpoints (default `30`) |
| `GIFT_RATE_LIMIT_WINDOW`     | Rate-limit window (default `1m`)                                         |

## Backups

Set `GIFT_AUTO_BACKUP_ENABLED=1` to turn on scheduled backups. Each backup is
a full online copy of the SQLite database (safe to take while the app is
running) written to `<GIFT_BACKUP_DIR>/daily/<timestamp>.db`. Backups taken on
a Monday are also classified as weekly, so that once the daily window rolls
past you still keep the most recent Monday snapshot.

Retention:
- The newest `GIFT_AUTO_BACKUP_KEEP_DAILY` daily backups are kept.
- The newest `GIFT_AUTO_BACKUP_KEEP_WEEKLY` weekly backups are kept in addition.

Restoring is a simple file copy: stop the container and replace
`DATA_DIR/giftlstr.db` with the backup.

## License

GPL-3.0 — see [LICENSE.md](LICENSE.md).