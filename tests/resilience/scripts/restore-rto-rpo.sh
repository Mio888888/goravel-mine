#!/usr/bin/env bash
set -euo pipefail
[[ -s "${RESTORE_REPORT_PATH:-artifacts/backup/restore-report.json}" ]] || { echo "measured restore report required" >&2; exit 1; }
tests/resilience/scripts/scenario-command.sh restore
