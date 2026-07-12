#!/usr/bin/env bash
set -euo pipefail
scenario="${1:?scenario required}"
scenario_key="$(printf '%s' "$scenario" | tr '[:lower:]-' '[:upper:]_')"
variable="RESILIENCE_ACTION_${scenario_key}"
command="${!variable:-}"
[[ -n "$command" ]] || { echo "$variable is required" >&2; exit 1; }
bash -o pipefail -c "$command"
evidence="artifacts/resilience/${scenario}.json"
[[ -s "$evidence" ]] || { echo "action did not produce $evidence" >&2; exit 1; }
