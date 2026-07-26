#!/usr/bin/env bash

set -Eeuo pipefail

readonly PROJECT_DIR="${FUT_APP_API_DIR:-/opt/fut-app-api}"
readonly COMPOSE_FILE="$PROJECT_DIR/compose.yaml"
readonly ENV_FILE="$PROJECT_DIR/.env"

if [[ $# -ne 2 || "$2" != "RESTORE" ]]; then
  echo "Uso: bash $0 /caminho/backup.dump RESTORE" >&2
  exit 2
fi

readonly BACKUP_FILE="$1"
if [[ ! -f "$BACKUP_FILE" || ! -f "$COMPOSE_FILE" || ! -f "$ENV_FILE" ]]; then
  echo "Backup, projeto ou .env nÃ£o encontrados." >&2
  exit 1
fi

compose() {
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

compose exec -T postgres sh -lc 'exec pg_restore --list' < "$BACKUP_FILE" > /dev/null

echo "Criando backup de seguranÃ§a antes da restauraÃ§Ã£o..."
FUT_APP_API_DIR="$PROJECT_DIR" bash "$PROJECT_DIR/scripts/backup-postgres.sh"

api_stopped=false
restart_api() {
  if [[ "$api_stopped" == true ]]; then
    compose up -d api
  fi
}
trap restart_api EXIT

compose stop api
api_stopped=true

compose exec -T postgres sh -lc \
  'exec pg_restore --clean --if-exists --no-owner --no-privileges -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < "$BACKUP_FILE"

compose up -d api
api_stopped=false
trap - EXIT

echo "RestauraÃ§Ã£o concluÃ­da: $BACKUP_FILE"
