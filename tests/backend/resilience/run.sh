#!/usr/bin/env bash
set -euo pipefail
profile=quick
for argument in "$@"; do
  case "$argument" in
    --profile=quick) profile=quick ;;
    --profile=soak) profile=soak ;;
    *) echo "unknown argument: $argument" >&2; exit 1 ;;
  esac
done
go run ./tests/backend/resilience/runner --profile "$profile" --output artifacts/resilience/resilience-report.json
