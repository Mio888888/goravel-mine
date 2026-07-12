#!/usr/bin/env bash
set -euo pipefail

target="${SLO_EVIDENCE_TARGET:-artifacts/release/slo-evidence.json}"
prom_url="${PROM_URL:-}"
loki_url="${LOKI_URL:-}"
alertmanager_url="${ALERTMANAGER_URL:-}"
deployment_uid="${RELEASE_DEPLOYMENT_UID:-}"
metrics_selector="${RELEASE_METRICS_SELECTOR:-}"
log_selector="${RELEASE_LOG_SELECTOR:-}"
deployment_started_at="${RELEASE_DEPLOYED_AT:-}"
images="${RELEASE_IMAGE_DIGESTS:-}"
window_seconds="${RELEASE_SLO_WINDOW_SECONDS:-1800}"
poll_seconds="${RELEASE_SLO_POLL_SECONDS:-60}"

if [ -z "$prom_url" ] || [ -z "$loki_url" ] || [ -z "$alertmanager_url" ]; then
  echo "PROM_URL, LOKI_URL, and ALERTMANAGER_URL are required" >&2
  exit 1
fi
if [ -z "$deployment_uid" ] || [ -z "$metrics_selector" ] || [ -z "$log_selector" ] || [ -z "$deployment_started_at" ] || [ -z "$images" ]; then
  echo "RELEASE_DEPLOYMENT_UID, RELEASE_METRICS_SELECTOR, RELEASE_LOG_SELECTOR, RELEASE_DEPLOYED_AT, and RELEASE_IMAGE_DIGESTS are required" >&2
  exit 1
fi
if ! [[ "$window_seconds" =~ ^[0-9]+$ ]] || [ "$window_seconds" -lt 1800 ]; then
  echo "RELEASE_SLO_WINDOW_SECONDS must be at least 1800" >&2
  exit 2
fi
if ! [[ "$poll_seconds" =~ ^[0-9]+$ ]] || [ "$poll_seconds" -lt 1 ]; then
  echo "RELEASE_SLO_POLL_SECONDS must be a positive integer" >&2
  exit 2
fi
if [ -e "$target" ]; then
  echo "target already exists; refusing to overwrite SLO evidence: $target" >&2
  exit 1
fi

git_sha="${RELEASE_GIT_SHA:-${GITHUB_SHA:-}}"
if [ -z "$git_sha" ]; then
  git_sha="$(git rev-parse HEAD)"
fi
temporary="$(mktemp -d "${TMPDIR:-/tmp}/release-slo-evidence.XXXXXX")"
cleanup() {
  rm -rf "$temporary"
}
trap cleanup EXIT

started_at="$(date -u +%FT%TZ)"
deadline=$(( $(date +%s) + window_seconds ))
sample=0

query_prometheus() {
  local name="$1"
  local query="$2"
  local path="$temporary/prometheus-${sample}-${name}.json"
  curl -fsS --get "$prom_url/api/v1/query" --data-urlencode "query=$query" >"$path"
}

query_loki() {
  local name="$1"
  local query="$2"
  local path="$temporary/loki-${sample}-${name}.json"
  curl -fsS --get "$loki_url/loki/api/v1/query" --data-urlencode "query=$query" >"$path"
}

wait_until() {
  local timestamp="$1"
  local target_epoch
  target_epoch="$(python3 - "$timestamp" <<'PY'
import datetime as dt
import sys

try:
    value = dt.datetime.fromisoformat(sys.argv[1].replace("Z", "+00:00"))
except ValueError:
    raise SystemExit("RELEASE_DEPLOYED_AT must be RFC3339")
if value.tzinfo is None:
    raise SystemExit("RELEASE_DEPLOYED_AT must include a timezone")
print(int(value.timestamp()))
PY
)"
  while [ "$(date +%s)" -lt "$target_epoch" ]; do
    sleep "$poll_seconds"
  done
}

wait_until "$deployment_started_at"

while :; do
  query_prometheus availability "1 - (sum(rate(goravel_http_requests_total{${metrics_selector},status=~\"5..\"}[5m])) / clamp_min(sum(rate(goravel_http_requests_total{${metrics_selector}}[5m])), 0.001))"
  query_prometheus http_5xx_rate "sum(rate(goravel_http_requests_total{${metrics_selector},status=~\"5..\"}[5m])) / clamp_min(sum(rate(goravel_http_requests_total{${metrics_selector}}[5m])), 0.001)"
  query_prometheus http_p95 "histogram_quantile(0.95, sum by (le) (rate(goravel_http_request_duration_milliseconds_bucket{${metrics_selector}}[5m])))"
  query_prometheus http_p99 "histogram_quantile(0.99, sum by (le) (rate(goravel_http_request_duration_milliseconds_bucket{${metrics_selector}}[5m])))"
  query_prometheus db_pool_wait "rate(goravel_db_pool_wait_duration_milliseconds_total{${metrics_selector}}[5m])"
  query_prometheus queue_backlog "goravel_queue_failed_jobs{${metrics_selector}} + sum(goravel_queue_outbox_events{${metrics_selector},status=~\"pending|failed\"})"
  query_prometheus governance_failures "goravel_tenant_governance_verification_failed{${metrics_selector}} + goravel_tenant_governance_evidence_expired{${metrics_selector}}"
  query_loki audit_failures "sum(rate({service=\"goravel-mine\",${log_selector}} | json event=\"extra.event\", outcome=\"extra.outcome\" | event=\"audit\" | outcome=\"failure\" [5m]))"
  curl -fsS "$alertmanager_url/api/v2/alerts?active=true" >"$temporary/alertmanager-${sample}.json"
  sample=$((sample + 1))
  if [ "$(date +%s)" -ge "$deadline" ]; then
    break
  fi
  sleep "$poll_seconds"
done

finished_at="$(date -u +%FT%TZ)"
python3 - "$temporary" "$target" "$git_sha" "$deployment_uid" "$deployment_started_at" "$images" "$started_at" "$finished_at" "$window_seconds" <<'PY'
import hashlib
import json
from pathlib import Path
import sys

root = Path(sys.argv[1])
target = Path(sys.argv[2])
git_sha, uid, deployed_at, images, started_at, finished_at, window_seconds = sys.argv[3:]
thresholds = {
    "availability": {"operator": ">=", "value": 0.995},
    "http_5xx_rate": {"operator": "<", "value": 0.005},
    "http_p95": {"operator": "<=", "value": 1000},
    "http_p99": {"operator": "<=", "value": 2500},
    "db_pool_wait": {"operator": "<=", "value": 1000},
    "queue_backlog": {"operator": "<=", "value": 0},
    "governance_failures": {"operator": "<=", "value": 0},
    "audit_failures": {"operator": "<=", "value": 0},
}

def scalar(path):
    payload = json.loads(path.read_text(encoding="utf-8"))
    if payload.get("status") != "success":
        raise SystemExit(f"query failed: {path.name}")
    result = payload.get("data", {}).get("result", [])
    if not result:
        raise SystemExit(f"query returned no data: {path.name}")
    return float(result[0].get("value", [None, None])[1])

queries = []
passed = True
for name, threshold in thresholds.items():
    paths = sorted(root.glob(f"*-{name}.json"))
    values = [scalar(path) for path in paths]
    if not values:
        raise SystemExit(f"no query samples for {name}")
    observed = min(values) if threshold["operator"] == ">=" else max(values)
    operator, expected = threshold["operator"], threshold["value"]
    decision = observed >= expected if operator == ">=" else observed <= expected if operator == "<=" else observed < expected
    passed = passed and decision
    queries.append({
        "name": name,
        "source": "loki" if name == "audit_failures" else "prometheus",
        "samples": len(values),
        "observed": observed,
        "threshold": threshold,
        "passed": decision,
        "result_sha256": hashlib.sha256(b"".join(path.read_bytes() for path in paths)).hexdigest(),
    })

alert_files = sorted(root.glob("alertmanager-*.json"))
for path in alert_files:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if [item for item in payload if item.get("labels", {}).get("service") == "goravel-mine"]:
        passed = False

image_items = []
for item in images.split(","):
    name, digest = item.split("=", 1)
    if not name or not digest.startswith("sha256:") or len(digest) != 71:
        raise SystemExit("RELEASE_IMAGE_DIGESTS must use name=sha256:<64 lowercase hex>")
    image_items.append({"name": name, "digest": digest})

payload = {
    "schema_version": 1,
    "evidence_type": "slo-observation",
    "git_sha": git_sha,
    "deployment": {"uid": uid, "started_at": deployed_at, "images": image_items},
    "observation": {"started_at": started_at, "finished_at": finished_at, "window_seconds": int(window_seconds), "samples": len(alert_files)},
    "queries": queries,
    "alertmanager": {"samples": len(alert_files), "result_sha256": hashlib.sha256(b"".join(path.read_bytes() for path in alert_files)).hexdigest()},
    "status": "passed" if passed else "failed",
}
normalized = dict(payload)
normalized["digest"] = "sha256:pending"
payload["digest"] = "sha256:" + hashlib.sha256(
    json.dumps(normalized, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
).hexdigest()
target.parent.mkdir(parents=True, exist_ok=True)
target.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
if not passed:
    raise SystemExit("SLO evidence thresholds or active alerts failed")
PY

trap - EXIT
printf '%s\n' "$target"
