#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$ROOT_DIR/MineAdmin-web"
ARTIFACT_DIR="${OIDC_ARTIFACT_DIR:-$ROOT_DIR/artifacts/oidc-integration}"
IDP_ADDR="${OIDC_IDP_ADDR:-127.0.0.1:19090}"
WEB_PORT="${OIDC_E2E_PORT:-2889}"
API_URL="${OIDC_API_URL:-http://127.0.0.1:3000}"
IDP_URL="${OIDC_IDP_URL:-http://$IDP_ADDR}"
REDIRECT_URI="${OIDC_REDIRECT_URI:-http://127.0.0.1:$WEB_PORT/#/login}"
TENANT_CODE="${OIDC_TENANT_CODE:-default}"
TENANT_HOST="${OIDC_TENANT_HOST:-default.localhost}"
PROVIDER_NAME="${OIDC_PROVIDER:-oidc-e2e}"
IDP_CLIENT_ID="${OIDC_IDP_CLIENT_ID:-goravel-oidc-e2e}"

mkdir -p "$ARTIFACT_DIR"
idp_pid=""

cleanup() {
  local exit_code=$?
  local playwright_output="$WEB_DIR/tests/e2e/.output/oidc-integration"
  if [[ -n "$idp_pid" ]] && kill -0 "$idp_pid" 2>/dev/null; then
    kill "$idp_pid" 2>/dev/null || true
    wait "$idp_pid" 2>/dev/null || true
  fi
  if [[ -d "$playwright_output" ]]; then
    rm -rf "$ARTIFACT_DIR/playwright"
    mkdir -p "$ARTIFACT_DIR/playwright"
    cp -R "$playwright_output/." "$ARTIFACT_DIR/playwright/"
  fi
  write_evidence "$exit_code"
  exit "$exit_code"
}

write_evidence() {
  local exit_code="$1"
  local keys='[]'
  local events='[]'
  local results='{}'
  if curl --fail --silent --show-error "$IDP_URL/control/events" >"$ARTIFACT_DIR/idp-events.json" 2>/dev/null; then
    keys="$(node -e 'const fs = require("fs"); const value = JSON.parse(fs.readFileSync(process.argv[1], "utf8")); process.stdout.write(JSON.stringify(value.key_ids ?? []))' "$ARTIFACT_DIR/idp-events.json")"
    events="$(node -e 'const fs = require("fs"); const value = JSON.parse(fs.readFileSync(process.argv[1], "utf8")); process.stdout.write(JSON.stringify(value.events ?? []))' "$ARTIFACT_DIR/idp-events.json")"
  fi
  if [[ -f "$ARTIFACT_DIR/playwright/results.json" ]]; then
    results="$(node -e 'const fs = require("fs"); process.stdout.write(JSON.stringify(JSON.parse(fs.readFileSync(process.argv[1], "utf8"))))' "$ARTIFACT_DIR/playwright/results.json")"
  fi
  find "$ARTIFACT_DIR" -type f ! -name evidence.json -print0 \
    | while IFS= read -r -d '' file; do
        shasum -a 256 "$file"
      done \
    | node -e '
        const readline = require("readline");
        const lines = [];
        readline.createInterface({ input: process.stdin }).on("line", line => {
          const match = /^([a-f0-9]{64})\s+(.+)$/.exec(line);
          if (match) lines.push({ sha256: match[1], path: match[2] });
        }).on("close", () => process.stdout.write(JSON.stringify(lines)));
      ' >"$ARTIFACT_DIR/artifact-digests.json"
  local git_sha
  git_sha="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || printf 'unknown')"
  GIT_SHA="$git_sha" IDP_URL="$IDP_URL" EXIT_CODE="$exit_code" KEY_IDS="$keys" EVENTS="$events" \
    SCENARIO_RESULTS="$results" ARTIFACT_DIGESTS="$(cat "$ARTIFACT_DIR/artifact-digests.json")" \
    node -e '
      const fs = require("fs");
      fs.writeFileSync(process.argv[1], `${JSON.stringify({
        git_sha: process.env.GIT_SHA,
        idp_url: process.env.IDP_URL,
        exit_code: Number(process.env.EXIT_CODE),
        idp_key_ids: JSON.parse(process.env.KEY_IDS),
        events: JSON.parse(process.env.EVENTS),
        scenario_results: JSON.parse(process.env.SCENARIO_RESULTS),
        artifact_digests: JSON.parse(process.env.ARTIFACT_DIGESTS),
      }, null, 2)}\n`);
    ' "$ARTIFACT_DIR/evidence.json"
}

trap cleanup EXIT

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    printf 'required command not found: %s\n' "$1" >&2
    exit 1
  }
}

wait_for_http() {
  local url="$1"
  local label="$2"
  local attempts=0
  until curl --fail --silent --show-error "$url" >/dev/null; do
    attempts=$((attempts + 1))
    if (( attempts >= 60 )); then
      printf '%s did not become ready: %s\n' "$label" "$url" >&2
      exit 1
    fi
    sleep 1
  done
}

require_command curl
require_command yarn
require_command node

rm -rf "$WEB_DIR/tests/e2e/.output/oidc-integration"

(
  cd "$ROOT_DIR"
  OIDC_IDP_ADDR="$IDP_ADDR" \
  OIDC_IDP_ISSUER="$IDP_URL" \
  OIDC_IDP_REDIRECT_URI="$REDIRECT_URI" \
  OIDC_IDP_TEST_MODE=true \
  OIDC_IDP_EVENT_LOG="$ARTIFACT_DIR/idp-events.ndjson" \
  go run ./tests/oidc-idp
) >"$ARTIFACT_DIR/idp.log" 2>&1 &
idp_pid=$!
wait_for_http "$IDP_URL/.well-known/openid-configuration" 'OIDC test IdP'

if ! curl --fail --silent --show-error "$API_URL/health/ready" >/dev/null; then
  printf 'Goravel must be running and ready at %s before running this gate\n' "$API_URL" >&2
  exit 1
fi

(
  cd "$ROOT_DIR"
	  APP_ENV=testing SSO_TEST_ALLOW_LOOPBACK=true go run . artisan sso:test-fixture \
	    --tenant="$TENANT_CODE" --provider="$PROVIDER_NAME" --issuer="$IDP_URL" \
	    --redirect-uri="$REDIRECT_URI" --host="$TENANT_HOST" --client-id="$IDP_CLIENT_ID"
)

if ! curl --fail --silent --show-error -H "Host: $TENANT_HOST" -H "X-Tenant-Code: $TENANT_CODE" \
  "$API_URL/admin/passport/entry" \
  | OIDC_PROVIDER="$PROVIDER_NAME" OIDC_IDP_URL="$IDP_URL" node -e '
      let data = "";
      process.stdin.on("data", chunk => data += chunk).on("end", () => {
        const body = JSON.parse(data);
        const providers = body?.data?.config?.features?.sso?.providers ?? [];
        const valid = body?.code === 200 && body?.data?.mode === "tenant" && providers.some(item =>
          item.name === process.env.OIDC_PROVIDER && item.type === "oidc" &&
          typeof item.authorization_endpoint === "string" && item.authorization_endpoint.startsWith(process.env.OIDC_IDP_URL),
        );
        process.exitCode = valid ? 0 : 1;
      });
    '; then
  printf 'OIDC fixture missing: tenant=%s provider=%s issuer=%s\n' "$TENANT_CODE" "$PROVIDER_NAME" "$IDP_URL" >&2
  printf 'Controlled OIDC fixture setup failed.\n' >&2
  exit 1
fi

(
  cd "$WEB_DIR"
  OIDC_IDP_URL="$IDP_URL" \
  OIDC_API_URL="$API_URL" \
  OIDC_E2E_PORT="$WEB_PORT" \
  OIDC_REDIRECT_URI="$REDIRECT_URI" \
  OIDC_TENANT_CODE="$TENANT_CODE" \
  OIDC_TENANT_HOST="$TENANT_HOST" \
  OIDC_PROVIDER="$PROVIDER_NAME" \
  OIDC_IDP_CLIENT_ID="$IDP_CLIENT_ID" \
  yarn playwright test --config=playwright.oidc.config.ts
)
