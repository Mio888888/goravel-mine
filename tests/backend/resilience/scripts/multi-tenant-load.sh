#!/usr/bin/env bash
set -euo pipefail
command -v k6 >/dev/null || { echo "k6 is required" >&2; exit 1; }
k6 run tests/backend/resilience/scripts/multi-tenant-load.js
tests/backend/resilience/scripts/scenario-command.sh multi-tenant
