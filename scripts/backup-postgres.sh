#!/usr/bin/env bash

set -Eeuo pipefail
umask 077

readonly PROJECT_DIR="${FUT_APP_API_DIR:-/opt/fut-app-api}"
readonly COMPOSE_FILE="$PROJECT_DIR/compose.yaml"
readonly ENV_FILE="$PROJECT_DIR/.env"
readonly BACKUP_DIR="${FUT_APP_BACKUP_DIR:-/var/backups/fut-app}"
readonly RETENTION_DAYS="${FUT_APP_BACKUP_RETENTION_DAYS:-14}"

if [[ ! -f "$COMPOSE_FILE" || ! -f "$ENV_FILE" ]]; then
  echo "Projeto ou .env nÃ£o encontrados em $PROJECT_DIR." >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_file="$BACKUP_DIR/fut-app-postgres-$timestamp.dump"
temporary_file="$(mktemp "$BACKUP_DIR/.fut-app-postgres-XXXXXX.dump")"

cleanup() {
  rm -f "$temporary_file"
}
trap cleanup EXIT

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
  sh -lc 'exec pg_dump --format=custom -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > "$temporary_file"

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" exec -T postgres \
  sh -lc 'exec pg_restore --list' < "$temporary_file" > /dev/null

mv "$temporary_file" "$backup_file"
trap - EXIT

find "$BACKUP_DIR" -maxdepth 1 -type f -name 'fut-app-postgres-*.dump' -mtime "+$RETENTION_DAYS" -delete

echo "Backup criado: $backup_file"
