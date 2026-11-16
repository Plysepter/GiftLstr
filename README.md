# GiftLstr

GiftLstr is a wishlist app for you to share with family and friends. Keep a list of gift ideas with links and notes and let others "claim" items so nobody buys the same thing twice. The list resets automatically after Christmas and after your birthday.

## Features

- Per-user gift lists with title, URL, and notes (URLs get link previews)
- Private lists: Keep a list of ideas for others that only you see
- Suggested list: Promote private ideas to be shared with others except the acceptee. Crowd source gift ideas!
- Claims: connected people can reserve an item so gifts aren't duplicated
- Optional spend tracking: Keep track of per person spending for each event 
- Connections: a share code lets friends/family see and claim from your list
- Automatic archiving: after Christmas/birthday, the active list is archived and a fresh one is created (unclaimed items carry over)
- Optional automatic SQLite backups (daily with retention)
- Optional per-IP API rate limiting
- Admin panel for managing users
- Admins can right-click any item on a list they can open to edit its text and regenerate its link preview
- PWA with offline support (service-worker precaching + offline queue)
- Light/dark theme

## Tech stack

- **Frontend**: Svelte 5, Vite, TypeScript, self-hosted Authentik (OIDC, SSO and/or email/password)
- **Backend**: Go, serving the built frontend as one embedded static binary plus a JSON API
- **Storage**: SQLite (pure-Go driver, WAL mode)

## Getting started

1. Start Authentik. The compose file includes the `postgresql`, `redis`,
   `server` and `worker` services and publishes Authentik on
   `http://localhost:9000`. Values not already in `.env.local` are generated
   for you (`pnpm docker:genenv` fills random passwords/secrets), so the first
   run needs no prep:

   ```sh
   pnpm docker:up
   ```

   Sign in to `http://localhost:9000` (you'll be redirected to the login flow)
   with the bootstrap account: username `akadmin`, password the generated
   `AUTHENTIK_BOOTSTRAP_PASSWORD` from `.env.local`. Then create an
   **OIDC provider** for GiftLstr:

   - Name it e.g. `GiftLstr`; client type **Public** (the app uses PKCE, no secret).
   - Redirect URI: `http://localhost:8080/signin-oauth`
     (use your real origin in production).
   - A valid sign-out redirect URI: `http://localhost:8080/`.
   - Scopes: `openid`, `profile`, `email`.
   - Note the resulting **Client ID** (e.g. `b8m2c...`) and the provider's
     **Issuer URL**, which looks like
     `http://localhost:9000/application/o/giftlstr/`.

   In the Authentik provider **Advanced** settings you can also map a
   `birthday` claim (MM-DD) from the user's `attributes` so new accounts get
   their birthday reset pre-filled (see `AUTHENTIK_BIRTHDAY_CLAIM`).

2. Fill in `.env.local`:

   ```sh
   AUTHENTIK_ISSUER=http://localhost:9000/application/o/giftlstr/
   AUTHENTIK_CLIENT_ID=b8m2c...      # from the OIDC provider
   ADMIN_USER_IDS=                   # comma-separated Authentik sub values (see below)
   ```

   The compose file defaults `AUTHENTIK_ISSUER_BACKEND` to Authentik's
   compose-internal DNS name (`http://server:9000/...`), so same-host setups
   need no extra config. Only set it explicitly when Authentik runs somewhere
   the server can reach directly (e.g. a Caddy-fronted reverse proxy).

3. Start with Docker:

   ```sh
   pnpm docker:up
   ```

   The app is served at http://localhost:8080. Data (the SQLite database) lives in `./data`, which is mounted into the container.

   ### Granting admin

   `ADMIN_USER_IDS` is matched against the OIDC `sub` claim — the exact subject
   value GiftLstr stores for the user. **Authentik hashes the `sub` by default**
   (its provider "Subject mode" is set to the hashed user ID, a SHA-256 of the
   user's UUID), so the UUID the admin UI shows is *not* the value to use. To
   grant admin to an account:

   1. Sign in to the app once as that account (this provisions the user).
   2. Read the stored subject from the database:

      ```sh
      sqlite3 ./data/giftlstr.db "SELECT auth_user_id, username FROM users;"
      ```

      `auth_user_id` *is* the `sub` to put in `ADMIN_USER_IDS`.
   3. Restart; the user is granted admin on their next request.

   > If the account hasn't signed in yet, you can instead change the OIDC
   > provider's **Subject mode** to "Based on the User's UUID" so `sub` equals
   > the admin-UI UUID, and use those UUIDs directly. Only do this **before**
   > the first login — changing it later changes every `sub`, so all existing
   > accounts would be treated as new users.

## Deployment notes

### Behind Cloudflare

The app sends a strict `Content-Security-Policy`. Cloudflare Web Analytics
(`https://static.cloudflareinsights.com` for the beacon script and
`https://cloudflareinsights.com` for its reporting endpoint) is allowed.

Cloudflare's **JavaScript Detections** feature (part of Bot Fight Mode / Super
Bot Fight Mode) injects an inline script whose body changes on every request
(it embeds the current `cf-ray` and timestamp), so it cannot be allowed by
hash. Disable JavaScript Detections for the GiftLstr hostname; no app-side CSP
change can permit that script without weakening `script-src`.

## Development

The frontend proxies `/api` to `http://localhost:8080`, so run both sides. The
backend needs the Authentik issuer in the environment (used to fetch the JWKS
that validates session tokens):

```sh
# Backend (Go), port 8080 — needs AUTHENTIK_ISSUER in the environment
DATA_DIR=./data AUTHENTIK_ISSUER=http://localhost:9000/application/o/giftlstr/ go -C server run .

# Frontend with hot reload (OIDC config comes from the backend-served page;
# for standalone dev set VITE_AUTHENTIK_ISSUER / VITE_AUTHENTIK_CLIENT_ID)
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
# which bypasses Authentik auth and must never be enabled in production.
pnpm e2e
```

## Demo mode (no Authentik)

`docker/giftlstr/docker-compose.demo.yml` runs the app without any identity
provider, useful for evaluating GiftLstr quickly:

```sh
docker compose -f docker/giftlstr/docker-compose.demo.yml up --build
```

It starts with `E2E_TEST_MODE=1`, so:

- **No Authentik required.** Auth is replaced by the same bearer-token scheme
  the Playwright suite uses. On the login page, pick a seeded account under
  "Demo accounts" to sign in instantly (or send
  `Authorization: Bearer e2e:alice:Alice Apple` directly).
- **A canonical seeded dataset.** Every boot wipes the database and applies
  `server/seed.sql` (the exact dataset the e2e suite asserts against), so each
  run starts from identical data. Additional users (`neww:Wren Newguy`,
  `dave:Dave Durian`, `eve:Eve Elderberry`) are created lazily by the e2e suite
  and are *not* part of the seed.
- **Nothing persists.** `PORT` (default `8080`) peers the container port;
  data lives on a `tmpfs` mount, so a restart resets to the seed. Never put
  this mode in front of real traffic or real data.

Seeded accounts include `admin:Ada Root` (admin), `alice:Alice Apple` (the
main demo user, connected to `bob`, `carol`, `dana`, `eric`, `faye` and
`george`), plus `helen:Helen Honeydew` and `ivan:Ivan Kiwi`, who are *not*
connected to alice — handy for seeing the "no access" behavior. Bulk
`mock01..mock30` accounts exist to exercise list pagination.

## Releasing Docker images

`scripts/release-docker.sh` builds the main and demo images and exports each as
a tar archive, tagged with a version:

```sh
# Version defaults to `git describe --tags --always --dirty`
pnpm docker:release

# Explicit version, custom output directory, or just one image
sh scripts/release-docker.sh 1.4.0
sh scripts/release-docker.sh 1.4.0 --out /tmp/releases
sh scripts/release-docker.sh 1.4.0 --only demo
sh scripts/release-docker.sh 1.4.0 --no-latest   # don't also tag :latest
```

The script writes:

| Image                           | Archive                            |
| ------------------------------- | ---------------------------------- |
| `giftlstr-giftlstr:<ver>`       | `releases/giftlstr-<ver>.tar`      |
| `giftlstr-giftlstr-demo:<ver>`  | `releases/giftlstr-demo-<ver>.tar` |

Both images also get a `:latest` tag unless `--no-latest` is passed, and the
archives include both tags. Move a release to another host and load it with:

```sh
docker load -i releases/giftlstr-1.4.0.tar
docker load -i releases/giftlstr-demo-1.4.0.tar
```

The compose files use these image names (`giftlstr-giftlstr:latest` for the
main app, `giftlstr-giftlstr-demo:latest` for the demo), so after loading a
release you can start it without rebuilding:

```sh
docker compose -f docker/giftlstr/docker-compose.yml up -d
docker compose -f docker/giftlstr/docker-compose.demo.yml up -d
```

Use `--build` to rebuild from source instead, or override the image names with
`MAIN_IMAGE` / `DEMO_IMAGE` when calling the script.

### Deploying a release

The release tags are local-only. As the images are not currently pushed to any docker repository, you will need to manually deploy the image. Depending onyour hosting solution, you may need to disable image pulling for the app to deploy successfully.

The release tags are local-only: `giftlstr-giftlstr:latest` exists on the host only after `docker load` (or an upload on Arcane's **Images** page). They are not published to a registry, so anything that pulls by name fails with `pull access denied for giftlstr-giftlstr`.

The demo image is built with `VITE_E2E=true`, which bundles the seeded demo
accounts and e2e token provider — keep it out of production.

## Configuration

| Variable                  | Purpose                                                                 |
| ------------------------- | ----------------------------------------------------------------------- |
| `AUTHENTIK_ISSUER`        | Public Authentik OIDC issuer (required); also the accepted `iss` claim value |
| `AUTHENTIK_ISSUER_BACKEND`| URL the server fetches discovery/JWKS/token from; defaults to `AUTHENTIK_ISSUER` (defaults to `http://server:9000/application/o/giftlstr/` under the compose stack) |
| `AUTHENTIK_CLIENT_ID`     | OIDC client id for the SPA flow (required for sign-in)                   |
| `AUTHENTIK_BIRTHDAY_CLAIM`| Claim/attribute name carrying the user's MM-DD birthday (default `birthday`) |
| `ADMIN_USER_IDS`          | Comma-separated Authentik `sub` values granted admin (hashed by default — see README) |
| `AUTH_PROVIDER`           | Backend identity provider name (default `authentik`)                    |
| `DATA_DIR`                | Directory for the SQLite database (default `/data`)                      |
| `PORT`                    | HTTP listen port (default `8080`)                                        |
| `GIFT_AUTO_BACKUP_ENABLED`  | `1` enables automatic backups (default `0`)                          |
| `GIFT_AUTO_BACKUP_INTERVAL` | Backup cadence (default `24h`)                                       |
| `GIFT_AUTO_BACKUP_KEEP_DAILY`| Daily backups to retain (default `7`)                               |
| `GIFT_AUTO_BACKUP_KEEP_WEEKLY`| Weekly backups to retain beyond the daily window (default `4`)      |
| `GIFT_BACKUP_DIR`         | Directory for backups (default `DATA_DIR`)                               |
| `GIFT_RATE_LIMIT_ENABLED` | `1` enables per-IP rate limiting (default `0`)                           |
| `GIFT_RATE_LIMIT`         | Requests per IP per window for normal endpoints (default `120`)          |
| `GIFT_RATE_LIMIT_STRICT`  | Requests per IP per window for metadata/share-code endpoints (default `30`) |
| `GIFT_RATE_LIMIT_WINDOW`  | Rate-limit window (default `1m`)                                         |

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
