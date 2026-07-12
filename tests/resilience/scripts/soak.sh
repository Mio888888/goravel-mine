#!/usr/bin/env bash
set -euo pipefail
[[ "${RESILIENCE_SOAK_DURATION_SECONDS:-28800}" -ge 28800 ]] || { echo "soak must run at least 8 hours" >&2; exit 1; }
tests/resilience/scripts/scenario-command.sh soak
