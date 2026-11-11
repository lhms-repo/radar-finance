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

echo "Restaurando $file no volume $volume..."
docker run --rm \
  -v "${volume}:/data" \
  -v "${RADAR_BACKUP_HOST_DIR}:/backups:ro" \
  alpine:3 sh -euc "
    if [ -f /data/radar.db ]; then cp /data/radar.db /data/radar.db.antes-da-restauracao; fi
    rm -f /data/radar.db-wal /data/radar.db-shm
    cp '/backups/${file}' /data/radar.db
    chown 65532:65532 /data/radar.db
  "

echo "Iniciando o Radar..."
docker compose start radar
echo "Pronto. O banco anterior ficou em /data/radar.db.antes-da-restauracao dentro do volume."
