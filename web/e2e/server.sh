#!/usr/bin/env bash
# Builds the app and starts it on a fresh database with a fictional user, for Playwright.
set -euo pipefail

cd "$(dirname "$0")/../.."
dir=tmp/e2e
rm -rf "$dir"
mkdir -p "$dir/backups"

export RADAR_DB_PATH="$dir/radar.db"
export RADAR_BACKUP_DIR="$dir/backups"
export RADAR_COOKIE_SECURE=false
export RADAR_ADDR=":${E2E_PORT:-8090}"

(cd web && pnpm run build >/dev/null)
CGO_ENABLED=0 go build -tags embedweb -o "$dir/radar" ./cmd/radar
printf 'senha-ficticia-123\n' | "$dir/radar" user add --login teste --name "Pessoa Teste" --password-stdin
exec "$dir/radar" serve
