#!/usr/bin/env bash
set -euo pipefail

here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
env_file="${1:-$here/.env.production}"
backup_dir="${2:-$here/backups}"
if [[ ! -f "$env_file" ]]; then echo "missing env file: $env_file" >&2; exit 2; fi
if [[ -z "${BACKUP_AGE_RECIPIENT:-}" ]]; then echo "set BACKUP_AGE_RECIPIENT to the offsite age public recipient" >&2; exit 2; fi
mkdir -p "$backup_dir"
chmod 700 "$backup_dir"
output="$backup_dir/pinguin-$(date -u +%Y%m%dT%H%M%SZ).dump.age"
if [[ -e "$output" ]]; then echo "backup already exists: $output" >&2; exit 1; fi
temporary="$(mktemp "$backup_dir/.pinguin-backup.XXXXXX")"
cleanup() { rm -f -- "$temporary"; }
trap cleanup EXIT
trap 'exit 130' INT TERM
docker compose --env-file "$env_file" -f "$here/compose.production.yaml" exec -T postgres \
  sh -ec 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' \
  | age -r "$BACKUP_AGE_RECIPIENT" > "$temporary"
[[ -s "$temporary" ]] || { echo "encrypted backup is empty" >&2; exit 1; }
chmod 600 "$temporary"
ln -- "$temporary" "$output"
rm -- "$temporary"
trap - EXIT INT TERM
echo "encrypted backup written: $output"
