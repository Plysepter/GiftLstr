#!/bin/sh
# Generates missing Authentik secrets into the compose env file so
# `pnpm docker:up` works out of the box. Idempotent: existing non-empty
# values are never overwritten.
#
# Usage: sh scripts/init-authentik-env.sh [env-file]   (default: .env.local)

set -eu
ENV_FILE="${1:-.env.local}"

[ -f "$ENV_FILE" ] || touch "$ENV_FILE"

rand_b64() {
  openssl rand -base64 "$1" 2>/dev/null | tr -d '\n' || head -c "$((("$1" / 3) * 4))" /dev/urandom | base64 | tr -d '\n'
}

# set_or_generate KEY LENGTH: leaves an existing non-empty value alone; fills
# an empty value or adds the key otherwise.
set_or_generate() {
  key="$1"
  length="$2"
  if grep -q "^${key}=[^[:space:]]" "$ENV_FILE"; then
    : # already set
  else
    value="$(rand_b64 "$length")"
    # replace existing `key=` line, or append a new one
    grep -q "^${key}=" "$ENV_FILE" \
      && sed -i.bak "s%^${key}=.*%${key}=${value}%" "$ENV_FILE" \
      || printf '%s=%s\n' "$key" "$value" >> "$ENV_FILE"
    rm -f "$ENV_FILE.bak"
    printf '  %s  (generated)\n' "$key"
  fi
}

echo "-> ensuring Authentik secrets in $ENV_FILE"
set_or_generate AUTHENTIK_SECRET_KEY 36
set_or_generate AUTHENTIK_POSTGRESQL__PASSWORD 24
set_or_generate AUTHENTIK_BOOTSTRAP_PASSWORD 12

bootstrap_email="$(grep -m1 '^AUTHENTIK_BOOTSTRAP_EMAIL=' "$ENV_FILE" | cut -d= -f2-)"
[ -n "$bootstrap_email" ] || bootstrap_email="akadmin"

printf '\nBootstrap sign-in for Authentik (%s/):\n' "http://localhost:9000"
printf '  username: %s\n' "$bootstrap_email"
printf '  password: %s\n' "$(grep -m1 '^AUTHENTIK_BOOTSTRAP_PASSWORD=' "$ENV_FILE" | cut -d= -f2-)"
echo "(then create an OIDC provider and paste its Client ID into AUTHENTIK_CLIENT_ID)"