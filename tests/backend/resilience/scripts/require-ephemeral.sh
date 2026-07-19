#!/usr/bin/env bash
set -euo pipefail
[[ "${RESILIENCE_ENVIRONMENT:-}" == "ephemeral" ]] || { echo "RESILIENCE_ENVIRONMENT=ephemeral is required" >&2; exit 1; }
[[ "${RESILIENCE_TARGET_URL:-}" =~ ^https?:// ]] || { echo "RESILIENCE_TARGET_URL is required" >&2; exit 1; }
target_lower="$(printf '%s' "$RESILIENCE_TARGET_URL" | tr '[:upper:]' '[:lower:]')"
case "$target_lower" in *prod*|*production*) echo "production-like target refused" >&2; exit 1;; esac
mkdir -p artifacts/resilience
