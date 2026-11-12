#!/usr/bin/env bash
# Restores a backup copy into the database volume (RNF-07).
# Usage, from the deploy folder in WSL2: ./restore.sh radar-20261110-030000-daily.db
set -euo pipefail

cd "$(dirname "$0")"
if [[ $# -ne 1 ]]; then
  echo "Uso: ./restore.sh <arquivo de backup>" >&2
  exit 2
fi
file="$(basename "$1")"
# shellcheck disable=SC1091
source .env
src="${RADAR_BACKUP_HOST_DIR}/${file}"
if [[ ! -f "$src" ]]; then
  echo "Backup não encontrado: $src" >&2
  exit 1
fi

# Compose project "radar" plus the volume name declared in docker-compose.yml.
volume="radar_radar-data"

echo "Parando o Radar..."
docker compose stop radar

# Each restore keeps its own copy of the previous database, with the WAL files that may hold
# committed transactions, so a second restore never overwrites the first safety copy.
stamp="$(date +%Y%m%d-%H%M%S)"
saved="/data/antes-da-restauracao/radar-${stamp}.db"

echo "Restaurando $file no volume $volume..."
docker run --rm \
  -v "${volume}:/data" \
  -v "${RADAR_BACKUP_HOST_DIR}:/backups:ro" \
  alpine:3 sh -euc "
    # The copy lands next to the database first, so a failed copy leaves the volume untouched.
    cp '/backups/${file}' /data/radar.db.restaurando
    mkdir -p /data/antes-da-restauracao
    # A WAL left without its database would be replayed onto the restored copy, so each file moves on its own.
    for ext in '' -wal -shm; do
      if [ -f /data/radar.db\$ext ]; then mv /data/radar.db\$ext ${saved}\$ext; fi
    done
    mv /data/radar.db.restaurando /data/radar.db
    chown 65532:65532 /data/radar.db
  "

echo "Iniciando o Radar..."
docker compose start radar
echo "Pronto. O banco anterior ficou em ${saved} dentro do volume."
