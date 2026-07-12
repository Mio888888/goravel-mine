#!/usr/bin/env bash
set -euo pipefail
scenario="${1:?scenario required}"
evidence="artifacts/resilience/${scenario}.json"
[[ -s "$evidence" ]] || { echo "missing measured evidence: $evidence" >&2; exit 1; }
python3 - "$evidence" <<'PY'
import json, sys
d=json.load(open(sys.argv[1]))
if d.get("measured") is not True: raise SystemExit("evidence must set measured=true")
decisions=d.get("threshold_decisions")
if not isinstance(decisions, list) or not decisions: raise SystemExit("threshold decisions missing")
failed=[x for x in decisions if x.get("passed") is not True]
if failed: raise SystemExit(f"thresholds failed: {failed}")
PY
