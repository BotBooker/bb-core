#!/usr/bin/env sh
# Восстановление PostgreSQL из custom-дампа.
# Использование: DSN=postgres://... ./deploy/scripts/restore.sh файл.dump
set -eu

: "${DSN:?DSN is required}"
dump="${1:?dump file is required}"

if [ ! -f "$dump" ]; then
  echo "dump not found: $dump" >&2
  exit 1
fi

pg_restore \
  --dbname="$DSN" \
  --clean \
  --if-exists \
  --no-owner \
  --no-privileges \
  --exit-on-error \
  "$dump"

echo "restore completed: $dump"
