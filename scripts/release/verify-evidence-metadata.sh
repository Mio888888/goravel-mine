#!/usr/bin/env bash
set -euo pipefail

uri=""
git_sha=""
metadata="${RELEASE_EVIDENCE_METADATA_FILE:-}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --uri)
      uri="$2"
      shift 2
      ;;
    --git-sha)
      git_sha="$2"
      shift 2
      ;;
    *)
      echo "unsupported argument: $1" >&2
      exit 2
      ;;
  esac
done

if [ -z "$uri" ] || [ -z "$git_sha" ]; then
  echo "--uri and --git-sha are required" >&2
  exit 2
fi
if [ -z "$metadata" ] || [ ! -s "$metadata" ]; then
  echo "RELEASE_EVIDENCE_METADATA_FILE must point to immutable evidence metadata JSON" >&2
  exit 1
fi

python3 - "$metadata" "$uri" "$git_sha" <<'PY'
import datetime as dt
import json
import re
import sys

path, uri, git_sha = sys.argv[1:]
try:
    with open(path, encoding="utf-8") as handle:
        payload = json.load(handle)
except (OSError, json.JSONDecodeError) as exc:
    raise SystemExit(f"invalid immutable evidence metadata: {exc}")

required = ("uri", "object_version", "sha256", "immutable_until", "verified_at", "git_sha")
missing = [field for field in required if not str(payload.get(field, "")).strip()]
if missing:
    raise SystemExit(f"immutable evidence metadata missing: {', '.join(missing)}")
if payload["uri"] != uri:
    raise SystemExit("immutable evidence metadata URI mismatch")
if payload["git_sha"] != git_sha:
    raise SystemExit("immutable evidence metadata Git SHA mismatch")
if not re.fullmatch(r"[0-9a-f]{64}", payload["sha256"]):
    raise SystemExit("immutable evidence metadata SHA-256 must be lowercase hex")

def parse_timestamp(value, field):
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        raise SystemExit(f"immutable evidence metadata {field} is invalid")

verified_at = parse_timestamp(payload["verified_at"], "verified_at")
immutable_until = parse_timestamp(payload["immutable_until"], "immutable_until")
if immutable_until <= verified_at:
    raise SystemExit("immutable evidence metadata immutable_until must follow verified_at")

print(json.dumps(payload, ensure_ascii=False, sort_keys=True))
PY
