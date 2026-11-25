#!/usr/bin/env bash
set -euo pipefail

here="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
if [[ $# -lt 2 ]]; then echo "usage: BACKUP_AGE_IDENTITY=/path/to/key $0 <encrypted-backup.dump.age> <restore_database_name> [env-file]" >&2; exit 2; fi
backup="$1"
target="$2"
env_file="${3:-$here/.env.production}"
if [[ ! -f "$backup" || ! -f "$env_file" ]]; then echo "backup or env file does not exist" >&2; exit 2; fi
if [[ ! "$target" =~ ^restore_[a-zA-Z0-9_]{1,55}$ ]]; then echo "restore database name must start with restore_ and contain only letters, digits, and underscores" >&2; exit 2; fi
if [[ -z "${BACKUP_AGE_IDENTITY:-}" || ! -f "$BACKUP_AGE_IDENTITY" ]]; then echo "set BACKUP_AGE_IDENTITY to the private age key" >&2; exit 2; fi
compose=(docker compose --env-file "$env_file" -f "$here/compose.production.yaml")
"${compose[@]}" exec -T postgres sh -ec 'createdb -U "$POSTGRES_USER" "$1"' sh "$target"
restored=false
cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [[ "$restored" != true && $status -ne 0 ]]; then
    "${compose[@]}" exec -T postgres sh -ec 'dropdb --if-exists -U "$POSTGRES_USER" "$1"' sh "$target" >/dev/null 2>&1 || true
  fi
  return "$status"
}
trap cleanup EXIT INT TERM
age -d -i "$BACKUP_AGE_IDENTITY" "$backup" | "${compose[@]}" exec -T postgres sh -ec 'pg_restore -U "$POSTGRES_USER" -d "$1" --no-owner --no-privileges' sh "$target"
restored=true
trap - EXIT INT TERM
echo "restore completed in isolated database $target; validate the database before any cutover"
