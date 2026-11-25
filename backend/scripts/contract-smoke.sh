#!/usr/bin/env bash
set -euo pipefail

base="${API_BASE_URL:-http://127.0.0.1:8080}"
token="${ADMIN_JWT_TOKEN:-}"
if [[ -z "$token" ]]; then
  : "${JWT_SECRET:?JWT_SECRET is required to issue admin JWT}"
  token="$(go run ./cmd/admin-jwt -subject contract-smoke)"
fi
expect_status() {
  local expected="$1"; shift
  local actual
  actual="$(curl -sS -o /tmp/pinguin-contract-response.json -w '%{http_code}' "$@")"
  if [[ "$actual" != "$expected" ]]; then
    cat /tmp/pinguin-contract-response.json
    echo "expected HTTP $expected, received $actual" >&2
    exit 1
  fi
}

expect_status 200 "$base/health/live"
expect_status 200 "$base/api/v1/repositories?page=1&page_size=5"
catalog_page="$(curl -fsS "$base/api/v1/repositories?q=Catalog%20Prefix%20Smoke&category=iso&page=1&page_size=10")"
[[ "$(jq -r '.pagination.total' <<<"$catalog_page")" == "2" ]] || { echo 'catalog fixture total did not match' >&2; exit 1; }
[[ "$(jq -r '[.data[].path_prefix] | sort | join(",")' <<<"$catalog_page")" == "iso/catalog-prefix-smoke,iso/nested/catalog-prefix-smoke" ]] || { echo 'catalog list path_prefix values did not match' >&2; exit 1; }
nested_id="$(jq -r '.data[] | select(.slug == "catalog-nested-prefix-smoke") | .id' <<<"$catalog_page")"
expect_status 200 "$base/api/v1/repositories/$nested_id"
[[ "$(jq -r '.data.path_prefix' /tmp/pinguin-contract-response.json)" == "iso/nested/catalog-prefix-smoke" ]] || { echo 'catalog detail path_prefix did not match' >&2; exit 1; }
region_data="$(curl -fsS "$base/api/v1/stats/regions?period=24h")"
jq -e '
  ([.data[] | select(.region == "Smoke High" and .downloads == 5)] | length) == 1 and
  ([.data[] | select(.region == "Other" and .country == "United Kingdom" and .downloads == 5 and .latitude == null and .longitude == null)] | length) == 1 and
  ([.data[] | select(.region == "Smoke Low A" or .region == "Smoke Low B" or .region == "Smoke Tiny")] | length) == 0
' <<<"$region_data" >/dev/null || { echo 'region minimum-threshold aggregation did not match' >&2; exit 1; }
expect_status 400 "$base/api/v1/stats/summary?period=invalid"
expect_status 400 "$base/api/v1/stats/summary?repository=not-a-uuid"
expect_status 200 "$base/api/v1/stats/summary?repository=00000000-0000-4000-8000-000000000001"
[[ "$(jq -r '.data.download_requests' /tmp/pinguin-contract-response.json)" == "0" ]] || { echo 'unmatched valid repository filter should return an empty summary' >&2; exit 1; }
expect_status 401 "$base/api/v1/admin/surveys"
expect_status 401 "$base/api/v1/admin/workers/analytics"
expect_status 200 -H "Authorization: Bearer $token" "$base/api/v1/admin/workers/analytics"
if [[ -n "${JWT_SECRET_PREVIOUS:-}" ]]; then
  previous_token="$(go run ./cmd/admin-jwt -subject contract-smoke-previous -use-previous)"
  expect_status 200 -H "Authorization: Bearer $previous_token" "$base/api/v1/admin/workers/analytics"
fi
manager_token="$(go run ./cmd/admin-jwt -subject contract-smoke-manager -roles survey_manager)"
reader_token="$(go run ./cmd/admin-jwt -subject contract-smoke-reader -roles response_reader)"
fulfillment_token="$(go run ./cmd/admin-jwt -subject contract-smoke-fulfillment -roles fulfillment_manager)"
audit_token="$(go run ./cmd/admin-jwt -subject contract-smoke-auditor -roles audit_reader)"
expect_status 200 -H "Authorization: Bearer $manager_token" "$base/api/v1/admin/surveys"
expect_status 403 -H "Authorization: Bearer $reader_token" -H 'Content-Type: application/json' -d '{}' "$base/api/v1/admin/surveys"
expect_status 403 -H "Authorization: Bearer $reader_token" "$base/api/v1/admin/audit-logs"
expect_status 200 -H "Authorization: Bearer $audit_token" "$base/api/v1/admin/audit-logs"
expect_status 403 -H "Authorization: Bearer $reader_token" -H 'Content-Type: application/json' -d '{}' -X PATCH "$base/api/v1/admin/survey-options/00000000-0000-4000-8000-000000000001/fulfillment"
fulfillment_validation="$(curl -sS -o /tmp/pinguin-contract-response.json -w '%{http_code}' -H "Authorization: Bearer $fulfillment_token" -H 'Content-Type: application/json' -d '{}' -X PATCH "$base/api/v1/admin/survey-options/00000000-0000-4000-8000-000000000001/fulfillment")"
if [[ "$fulfillment_validation" != "422" ]]; then
  cat /tmp/pinguin-contract-response.json
  echo "fulfillment role failed to reach validation, received HTTP $fulfillment_validation" >&2
  exit 1
fi

starts="$(date -u -d '1 minute ago' +%Y-%m-%dT%H:%M:%SZ)"
ends="$(date -u -d '1 day' +%Y-%m-%dT%H:%M:%SZ)"
draft="$(jq -n --arg start "$starts" --arg end "$ends" '{title:"Contract smoke survey",starts_at:$start,ends_at:$end,questions:[{label:"What should we mirror?",type:"short_text",required:true}]}')"
expect_status 201 -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$draft" "$base/api/v1/admin/surveys"
survey_id="$(jq -r '.data.id' /tmp/pinguin-contract-response.json)"
expect_status 200 -H "Authorization: Bearer $token" -X POST "$base/api/v1/admin/surveys/$survey_id/publish"
expect_status 200 "$base/api/v1/surveys/active"
question_id="$(jq -r '.data.questions[0].id' /tmp/pinguin-contract-response.json)"
submission="$(jq -n --arg question "$question_id" '{answers:[{question_id:$question,text:"Debian"}],contact:{email:"privacy-smoke@example.test"}}')"
key="contract-smoke-key-000001"
expect_status 201 -H 'Content-Type: application/json' -H "Idempotency-Key: $key" -d "$submission" "$base/api/v1/surveys/$survey_id/responses"
expect_status 201 -H 'Content-Type: application/json' -H "Idempotency-Key: $key" -d "$submission" "$base/api/v1/surveys/$survey_id/responses"
conflict="$(jq -n --arg question "$question_id" '{answers:[{question_id:$question,text:"Fedora"}],contact:{email:"privacy-smoke@example.test"}}')"
expect_status 409 -H 'Content-Type: application/json' -H "Idempotency-Key: $key" -d "$conflict" "$base/api/v1/surveys/$survey_id/responses"
expect_status 404 -H 'Content-Type: application/json' -H 'Idempotency-Key: contract-missing-key-000001' -d '{"answers":[{"question_id":"00000000-0000-4000-8000-000000000001","text":"x"}]}' "$base/api/v1/surveys/00000000-0000-4000-8000-000000000000/responses"
expect_status 200 -H "Authorization: Bearer $reader_token" "$base/api/v1/admin/surveys/$survey_id/responses"
jq -e '.data | (length == 1 and .[0].contact_email == "p********@example.test")' /tmp/pinguin-contract-response.json >/dev/null || { echo 'response reader did not receive a masked contact address' >&2; exit 1; }
if grep -q 'privacy-smoke@example.test' /tmp/pinguin-contract-response.json; then echo 'response API exposed the raw contact address' >&2; exit 1; fi
expect_status 200 -H "Authorization: Bearer $token" "$base/api/v1/admin/audit-logs?actor_id=contract-smoke-reader&action=read_survey_responses"
[[ "$(jq -r '.pagination.total' /tmp/pinguin-contract-response.json)" -ge 1 ]] || { echo 'response read was not attributed to the reader identity' >&2; exit 1; }
echo "API contract smoke checks passed, including catalog prefixes, analytics filters, masked contacts, and audit actor identity."
