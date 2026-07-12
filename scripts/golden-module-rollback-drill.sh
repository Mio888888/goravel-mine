#!/usr/bin/env bash
set -euo pipefail

target="${GOLDEN_ROLLBACK_TARGET:-artifacts/golden-rollback}"
api_url="${MODULE_LIFECYCLE_API_URL:-}"
token="${MODULE_LIFECYCLE_TOKEN:-}"
module_id="${GOLDEN_MODULE_ID:-reference-case}"
owner="${GOLDEN_MODULE_OWNER:-release-drill}"
reason="${GOLDEN_MODULE_REASON:-golden module rollback drill}"

if [ -z "$api_url" ] || [ -z "$token" ]; then
  echo "MODULE_LIFECYCLE_API_URL and MODULE_LIFECYCLE_TOKEN are required; direct CLI execution is not accepted" >&2
  exit 1
fi
api_url="${api_url%/}"
platform_api_url="${MODULE_PLATFORM_API_URL:-${api_url%/module-lifecycle}}"
if [ -e "$target" ]; then
  echo "target already exists; refusing to overwrite drill evidence: $target" >&2
  exit 1
fi

git_sha="$(git rev-parse HEAD)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/golden-module-rollback.XXXXXX")"
cleanup() {
  rm -rf "$temporary"
}
trap cleanup EXIT

mkdir -p "$temporary"
fetch_state() {
  local name="$1"
  curl -fsS -H "Authorization: Bearer $token" "$api_url/state" >"$temporary/$name"
}

fetch_diff() {
  local name="$1"
  curl -fsS -H "Authorization: Bearer $token" "$api_url/diff" >"$temporary/$name"
}

execute_lifecycle() {
  local action="$1"
  local output="$2"
  local prefix="GOLDEN_${action^^}"
  local reauth_name="${prefix}_REAUTH_TOKEN"
  local approval_name="${prefix}_APPROVAL_ID"
  local reauth="${!reauth_name:-}"
  local approval="${!approval_name:-}"
  if [ -z "$reauth" ] || [ -z "$approval" ]; then
    echo "GOLDEN_${action^^}_REAUTH_TOKEN and GOLDEN_${action^^}_APPROVAL_ID are required" >&2
    exit 1
  fi
  curl -fsS -X POST "$api_url/execute" \
    -H "Authorization: Bearer $token" \
    -H "Content-Type: application/json" \
    --data "$(python3 - "$action" "$module_id" "$owner" "$reason" "$reauth" "$approval" <<'PY'
import json
import sys
action, module_id, owner, reason, reauth, approval = sys.argv[1:]
print(json.dumps({
    "action": action,
    "module_id": module_id,
    "execute": True,
    "owner": owner,
    "reason": reason,
    "confirm_token": f"{module_id}:{action}",
    "reauth_token": reauth,
    "approval_id": approval,
}))
PY
)" >"$temporary/$output"
}

fetch_state state-before.json
fetch_diff diff-before.json
execute_lifecycle upgrade upgrade-run.json
fetch_state state-after-upgrade.json
fetch_diff diff-after-upgrade.json
curl -fsS -H "Authorization: Bearer $token" "$api_url/state" >"$temporary/smoke-state.json"
curl -fsS -H "Authorization: Bearer $token" "$platform_api_url/reference-case/list?code=golden-case" >"$temporary/smoke-reference-case.json"
execute_lifecycle rollback rollback-run.json
fetch_state state-after-rollback.json
fetch_diff diff-after-rollback.json
curl -fsS -H "Authorization: Bearer $token" "$api_url/runs?module_id=$module_id" >"$temporary/runs.json"
curl -fsS -H "Authorization: Bearer $token" "$api_url/steps?module_id=$module_id" >"$temporary/steps.json"
curl -fsS -H "Authorization: Bearer $token" "$api_url/locks" >"$temporary/locks.json"

python3 - "$temporary" "$git_sha" "$module_id" <<'PY'
import hashlib
import json
from pathlib import Path
import sys

root = Path(sys.argv[1])
git_sha, module_id = sys.argv[2:]

def load(name):
    return json.loads((root / name).read_text(encoding="utf-8"))

def data(payload):
    if isinstance(payload, dict) and isinstance(payload.get("data"), dict):
        return payload["data"]
    return payload

def lifecycle_run(name, action):
    payload = data(load(name))
    items = payload.get("items", []) if isinstance(payload, dict) else []
    if not items or not isinstance(items[0], dict):
        raise SystemExit(f"{name} has no lifecycle execution result")
    item = items[0]
    if item.get("module_id") != module_id or item.get("action") != action or item.get("status") != "succeeded":
        raise SystemExit(f"{name} does not prove succeeded {action} for {module_id}")
    if not item.get("idempotency_key"):
        raise SystemExit(f"{name} lacks lifecycle run idempotency key")
    return item

upgrade = lifecycle_run("upgrade-run.json", "upgrade")
rollback = lifecycle_run("rollback-run.json", "rollback")
state_after_upgrade = data(load("state-after-upgrade.json"))
state_after_rollback = data(load("state-after-rollback.json"))
smoke_state = data(load("smoke-state.json"))
smoke_reference_case = data(load("smoke-reference-case.json"))
diff_after_rollback = data(load("diff-after-rollback.json"))
locks = data(load("locks.json"))

def rows(value):
    if isinstance(value, dict):
        return value.get("list", [])
    return value if isinstance(value, list) else []

upgraded = [row for row in rows(state_after_upgrade) if isinstance(row, dict) and row.get("id") == module_id]
rolled_back = [row for row in rows(state_after_rollback) if isinstance(row, dict) and row.get("id") == module_id]
smoked = [row for row in rows(smoke_state) if isinstance(row, dict) and row.get("id") == module_id]
if not upgraded or upgraded[0].get("persisted", {}).get("last_action") != "upgrade":
    raise SystemExit("state-after-upgrade does not prove upgraded persisted state")
if not rolled_back or rolled_back[0].get("persisted", {}).get("last_action") != "rollback":
    raise SystemExit("state-after-rollback does not prove rollback persisted state")
if not smoked or smoked[0].get("persisted", {}).get("last_action") != "upgrade":
    raise SystemExit("upgrade smoke does not prove the upgraded module state")
if not isinstance(smoke_reference_case, dict) or not isinstance(smoke_reference_case.get("list"), list):
    raise SystemExit("upgrade smoke does not return the reference-case data response")
golden_rows = [row for row in smoke_reference_case["list"] if isinstance(row, dict) and row.get("code") == "golden-case"]
if not golden_rows or golden_rows[0].get("version") != "1.1.0":
    raise SystemExit("upgrade smoke does not prove upgraded reference-case data")
for row in rows(diff_after_rollback):
    if isinstance(row, dict) and row.get("module_id") == module_id and row.get("drift") != "in_sync":
        raise SystemExit("rollback state diff is not in_sync")
for lock in rows(locks):
    if isinstance(lock, dict) and lock.get("key") == f"module-lifecycle:{module_id}":
        raise SystemExit("golden module lifecycle lock remains after rollback")

files = []
for path in sorted(root.glob("*.json")):
    content = path.read_bytes()
    files.append({"path": path.name, "sha256": hashlib.sha256(content).hexdigest(), "size": len(content)})
payload = {
    "schema_version": 1,
    "evidence_type": "rollback-drill",
    "git_sha": git_sha,
    "module_id": module_id,
    "execution": {"executed": True, "upgrade_run_key": upgrade["idempotency_key"], "smoke": "passed", "rollback_run_key": rollback["idempotency_key"]},
    "state_diff": {"after_rollback": "in_sync"},
    "artifacts": files,
}
normalized = dict(payload)
normalized["digest"] = "sha256:pending"
payload["digest"] = "sha256:" + hashlib.sha256(
    json.dumps(normalized, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
).hexdigest()
(root / "evidence.json").write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

mkdir -p "$(dirname "$target")"
mv "$temporary" "$target"
trap - EXIT
printf '%s\n' "$target/evidence.json"
