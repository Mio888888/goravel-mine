#!/usr/bin/env bash
set -euo pipefail

namespace="${QUEUE_SMOKE_NAMESPACE:-goravel-mine}"
deployment="${QUEUE_SMOKE_DEPLOYMENT:-goravel-mine-queue-worker}"
app_url="${QUEUE_SMOKE_APP_URL:-}"
metrics_token="${QUEUE_SMOKE_METRICS_TOKEN:-${OBS_METRICS_TOKEN:-}}"
prom_url="${QUEUE_SMOKE_PROM_URL:-}"
queue_class="${QUEUE_SMOKE_CLASS:-critical}"
enqueue_command="${QUEUE_SMOKE_ENQUEUE_COMMAND:-}"
job_count="${QUEUE_SMOKE_JOB_COUNT:-100}"
scale_up_timeout="${QUEUE_SMOKE_SCALE_UP_TIMEOUT_SECONDS:-300}"
drain_timeout="${QUEUE_SMOKE_DRAIN_TIMEOUT_SECONDS:-300}"
scale_down_timeout="${QUEUE_SMOKE_SCALE_DOWN_TIMEOUT_SECONDS:-600}"
poll_seconds="${QUEUE_SMOKE_POLL_SECONDS:-10}"

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "required command not found: $1" >&2
    exit 1
  }
}

require_positive_integer() {
  [[ "$2" =~ ^[1-9][0-9]*$ ]] || {
    echo "$1 must be a positive integer" >&2
    exit 2
  }
}

wait_for_replicas() {
  local label="$1"
  local timeout="$2"
  local comparison="$3"
  local baseline="$4"
  local deadline=$(( $(date +%s) + timeout ))

  while :; do
    local replicas
    replicas="$(worker_replicas)"
    if [ "$comparison" = "greater" ] && [ "$replicas" -gt "$baseline" ]; then
      break
    fi
    if [ "$comparison" = "not-greater" ] && [ "$replicas" -le "$baseline" ]; then
      break
    fi
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "timed out waiting for $label" >&2
      exit 1
    fi
    sleep "$poll_seconds"
  done
  printf '%s: %s\n' "$label" "$(date -u +%FT%TZ)"
}

wait_for_drain() {
  local deadline=$(( $(date +%s) + drain_timeout ))

  while ! metric_value_is_zero; do
    if [ "$(date +%s)" -ge "$deadline" ]; then
      echo "timed out waiting for queue-drain" >&2
      exit 1
    fi
    sleep "$poll_seconds"
  done
  printf '%s: %s\n' "queue-drain" "$(date -u +%FT%TZ)"
}

query_prometheus() {
  local query="$1"
  curl -fsS --get "$prom_url/api/v1/query" --data-urlencode "query=$query"
}

metric_value_is_zero() {
  query_prometheus "goravel_queue_pending_jobs{queue_class=\"$queue_class\"}" |
    jq -e '.status == "success" and ([.data.result[].value[1] | tonumber] | max // 0) == 0' >/dev/null
}

worker_replicas() {
  kubectl -n "$namespace" get deployment "$deployment" -o jsonpath='{.status.replicas}' | tr -d '[:space:]'
}

require_command curl
require_command jq
require_command kubectl
require_positive_integer QUEUE_SMOKE_JOB_COUNT "$job_count"
require_positive_integer QUEUE_SMOKE_SCALE_UP_TIMEOUT_SECONDS "$scale_up_timeout"
require_positive_integer QUEUE_SMOKE_DRAIN_TIMEOUT_SECONDS "$drain_timeout"
require_positive_integer QUEUE_SMOKE_SCALE_DOWN_TIMEOUT_SECONDS "$scale_down_timeout"
require_positive_integer QUEUE_SMOKE_POLL_SECONDS "$poll_seconds"

if [ -z "$app_url" ] || [ -z "$metrics_token" ] || [ -z "$prom_url" ] || [ -z "$enqueue_command" ]; then
  echo "QUEUE_SMOKE_APP_URL, QUEUE_SMOKE_METRICS_TOKEN, QUEUE_SMOKE_PROM_URL, and QUEUE_SMOKE_ENQUEUE_COMMAND are required" >&2
  exit 1
fi

curl -fsS -H "Authorization: Bearer $metrics_token" "$app_url/metrics" |
  grep -q "goravel_queue_pending_jobs{queue_class=\"$queue_class\"}"

kubectl -n "$namespace" rollout status "deployment/$deployment" --timeout="${scale_up_timeout}s"
baseline_replicas="$(worker_replicas)"
require_positive_integer "current worker replicas" "$baseline_replicas"
printf 'baseline: %s replicas=%s\n' "$(date -u +%FT%TZ)" "$baseline_replicas"

QUEUE_SMOKE_CLASS="$queue_class" QUEUE_SMOKE_JOB_COUNT="$job_count" bash -c "$enqueue_command"
printf 'enqueue: %s class=%s jobs=%s\n' "$(date -u +%FT%TZ)" "$queue_class" "$job_count"

wait_for_replicas "scale-up" "$scale_up_timeout" greater "$baseline_replicas"
wait_for_drain
wait_for_replicas "scale-down" "$scale_down_timeout" not-greater "$baseline_replicas"

echo "queue autoscaling smoke completed"
