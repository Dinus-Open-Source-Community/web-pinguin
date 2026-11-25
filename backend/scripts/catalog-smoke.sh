#!/usr/bin/env bash
set -euo pipefail

manifest="${1:-./catalog/pinguin-public-snapshot.json}"
go run ./cmd/catalog-import "$manifest"
go run ./cmd/catalog-import "$manifest"

counts="$(psql -X -v ON_ERROR_STOP=1 -At -F '|' -c "SELECT count(*), (SELECT count(*) FROM repository_releases), (SELECT count(*) FROM repository_files) FROM repositories")"
IFS='|' read -r repositories releases files <<< "$counts"
if [[ "$repositories" != 89 || "$releases" != 1 || "$files" != 3 ]]; then
  echo "catalog import counts mismatch: repositories=$repositories releases=$releases files=$files" >&2
  exit 1
fi

temp_manifest="$(mktemp)"
trap 'rm -f "$temp_manifest"' EXIT
jq '(.repositories[0].path_prefix) = "iso/path-change-smoke"' "$manifest" > "$temp_manifest"
if go run ./cmd/catalog-import "$temp_manifest" > /tmp/pinguin-catalog-conflict.out 2>&1; then
  echo 'catalog import unexpectedly allowed an existing slug to change path_prefix' >&2
  exit 1
fi
grep -q 'cannot change its path_prefix' /tmp/pinguin-catalog-conflict.out || {
  cat /tmp/pinguin-catalog-conflict.out >&2
  exit 1
}
unchanged="$(psql -X -v ON_ERROR_STOP=1 -At -c "SELECT path_prefix FROM repositories WHERE slug='$(jq -r '.repositories[0].slug' "$manifest")'")"
[[ "$unchanged" == "$(jq -r '.repositories[0].path_prefix' "$manifest")" ]] || {
  echo 'rejected catalog update changed the stored path_prefix' >&2
  exit 1
}
echo 'Catalog smoke passed: 89 repositories, idempotent repeated import, and path_prefix change protection.'
