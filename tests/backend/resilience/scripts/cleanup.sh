#!/usr/bin/env bash
set -euo pipefail
scenario="${1:?scenario required}"
scenario_key="$(printf '%s' "$scenario" | tr '[:lower:]-' '[:upper:]_')"
cleanup_var="RESILIENCE_CLEANUP_${scenario_key}"
command="${!cleanup_var:-}"
[[ -n "$command" ]] || { echo "$cleanup_var is required" >&2; exit 1; }
bash -o pipefail -c "$command"
