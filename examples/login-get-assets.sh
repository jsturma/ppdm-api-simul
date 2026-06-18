#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${PPDM_URL:-https://localhost:8443}"
USERNAME="${PPDM_USER:-admin}"
PASSWORD="${PPDM_PASSWORD:-admin}"

pretty_json() {
  jq .
}

echo "Logging in to ${BASE_URL}..."
LOGIN_RESPONSE="$(curl -sk -X POST "${BASE_URL}/api/v2/login" \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")"

echo "Login response:"
printf '%s' "${LOGIN_RESPONSE}" | pretty_json

TOKEN="$(printf '%s' "${LOGIN_RESPONSE}" | jq -r .access_token)"

echo "Fetching assets..."
ASSETS_RESPONSE="$(curl -sk "${BASE_URL}/api/v2/assets?page=1&pageSize=10" \
  -H "Authorization: Bearer ${TOKEN}")"

echo "Assets response:"
printf '%s' "${ASSETS_RESPONSE}" | pretty_json

echo
