#!/usr/bin/env bash
# Builds the web bundle and Go server, then runs the server with the
# E2E_TEST_MODE auth bypass enabled. Used by playwright.config.ts.
set -euo pipefail
cd "$(dirname "$0")/.."

# VITE_E2E=true keeps the e2e provider / demo-login code in the bundle; the
# flag is left unset for normal builds so Rollup drops that code entirely.
VITE_E2E=true pnpm --dir web build
rm -rf server/static
mkdir -p server/static
cp -r web/dist/* server/static/

go -C server build -o /tmp/giftlstr-e2e .
rm -rf e2e/.data
mkdir -p e2e/.data
exec /tmp/giftlstr-e2e
