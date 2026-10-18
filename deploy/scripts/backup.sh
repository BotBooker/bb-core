#!/usr/bin/env sh
# Резервная копия PostgreSQL в custom-формате (сжатие и выборочное восстановление).
# Использование: DSN=postgres://... ./deploy/scripts/backup.sh [файл.dump]
set -eu

: "${DSN:?DSN is required}"

output="${1:-bb-core-$(date -u +%Y%m%dT%H%M%SZ).dump}"

umask 077

pg_dump \
  --dbname="$DSN" \
  --format=custom \
  --no-owner \
  --no-privileges \
  --file="$output"

echo "backup written: $output"
