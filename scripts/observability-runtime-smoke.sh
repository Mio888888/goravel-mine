#!/usr/bin/env sh
set -eu

APP_URL="${APP_URL:-}"
METRICS_TOKEN="${METRICS_TOKEN:-${OBS_METRICS_TOKEN:-}}"
PROM_URL="${PROM_URL:-}"
LOKI_URL="${LOKI_URL:-}"
ALERTMANAGER_URL="${ALERTMANAGER_URL:-}"
GRAFANA_URL="${GRAFANA_URL:-}"
GRAFANA_TOKEN="${GRAFANA_TOKEN:-}"
OBS_SMOKE_STRICT="${OBS_SMOKE_STRICT:-false}"
REQUEST_ID_INPUT="${REQUEST_ID:-}"
REQUEST_ID="${REQUEST_ID:-obs-smoke-$(date +%Y%m%d%H%M%S)}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/observability-smoke.XXXXXX")"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

skip() {
  if [ "$OBS_SMOKE_STRICT" = "true" ]; then
    echo "FAIL: $1 (OBS_SMOKE_STRICT=true)" >&2
    exit 1
  fi
  echo "SKIP: $1"
}

require_query_result() {
  label="$1"
  endpoint="$2"
  query="$3"
  outfile="$TMP_DIR/query.json"

  if ! curl -fsS --get "$endpoint" --data-urlencode "query=$query" >"$outfile"; then
    echo "FAIL: $label query request failed" >&2
    return 1
  fi

  if ! python3 - "$label" "$outfile" <<'PY'
import json
import sys

label, path = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as handle:
    payload = json.load(handle)

if payload.get("status") != "success":
    raise SystemExit(f"FAIL: {label} query status is not success")

result = payload.get("data", {}).get("result", [])
if not result:
    raise SystemExit(f"FAIL: {label} query returned no series/logs")
PY
  then
    return 1
  fi
}

require_query_result_retry() {
  label="$1"
  endpoint="$2"
  query="$3"
  attempts="${4:-5}"
  delay_seconds="${5:-3}"
  attempt=1

  while [ "$attempt" -le "$attempts" ]; do
    if require_query_result "$label" "$endpoint" "$query"; then
      return 0
    fi
    if [ "$attempt" -eq "$attempts" ]; then
      return 1
    fi
    sleep "$delay_seconds"
    attempt=$((attempt + 1))
  done
}

check_app_metrics() {
  if [ -z "$APP_URL" ]; then
    skip "APP_URL not set; app /metrics smoke skipped"
    return
  fi
  curl -fsS -H "X-Request-Id: $REQUEST_ID" "$APP_URL/health/live" >/dev/null
  if [ -z "$METRICS_TOKEN" ]; then
    skip "METRICS_TOKEN or OBS_METRICS_TOKEN not set; app /metrics smoke skipped"
    return
  fi
  curl -fsS -H "Authorization: Bearer $METRICS_TOKEN" "$APP_URL/metrics" | grep -q "goravel_http_requests_total"
}

check_prometheus() {
  if [ -z "$PROM_URL" ]; then
    skip "PROM_URL not set; Prometheus smoke skipped"
    return
  fi
  require_query_result "Prometheus goravel target" "$PROM_URL/api/v1/query" 'up{job=~"goravel-mine.*"} == 1'
  require_query_result "Prometheus HTTP duration histogram" "$PROM_URL/api/v1/query" 'goravel_http_request_duration_milliseconds_bucket'
  curl -fsS "$PROM_URL/api/v1/rules" | grep -q "GoravelMine"
}

check_loki() {
  if [ -z "$LOKI_URL" ]; then
    skip "LOKI_URL not set; Loki smoke skipped"
    return
  fi
  require_query_result_retry "Loki goravel logs" "$LOKI_URL/loki/api/v1/query" '{service="goravel-mine"}'
  if [ -z "$APP_URL" ] && [ -z "$REQUEST_ID_INPUT" ]; then
    skip "APP_URL and REQUEST_ID not set; Loki request_id correlation skipped"
    return
  fi
  require_query_result_retry "Loki request_id logs" "$LOKI_URL/loki/api/v1/query" "{service=\"goravel-mine\"} | json request_id=\"extra.request_id\" | request_id=\"$REQUEST_ID\""
}

check_alertmanager() {
  if [ -z "$ALERTMANAGER_URL" ]; then
    skip "ALERTMANAGER_URL not set; Alertmanager smoke skipped"
    return
  fi
  curl -fsS "$ALERTMANAGER_URL/api/v2/status" | grep -q '"versionInfo"'
  curl -fsS "$ALERTMANAGER_URL/api/v2/receivers" | grep -q "goravel-mine"
}

check_grafana() {
  if [ -z "$GRAFANA_URL" ] || [ -z "$GRAFANA_TOKEN" ]; then
    skip "GRAFANA_URL or GRAFANA_TOKEN not set; Grafana smoke skipped"
    return
  fi
  curl -fsS -H "Authorization: Bearer $GRAFANA_TOKEN" "$GRAFANA_URL/api/dashboards/uid/goravel-mine-observability" | grep -q "Goravel Mine Observability"
  curl -fsS -H "Authorization: Bearer $GRAFANA_TOKEN" "$GRAFANA_URL/api/dashboards/uid/goravel-mine-logs" | grep -q "Goravel Mine Logs"
  curl -fsS -H "Authorization: Bearer $GRAFANA_TOKEN" "$GRAFANA_URL/api/dashboards/uid/goravel-mine-audit" | grep -q "Goravel Mine Audit"
}

check_app_metrics
check_prometheus
check_loki
check_alertmanager
check_grafana

echo "observability runtime smoke finished"
