#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repository_root="$(CDPATH= cd -- "$script_dir/.." && pwd)"
release_gate_phase="${RELEASE_GATE_PHASE:-final}"
case "$release_gate_phase" in
  predeploy|final) ;;
  *) echo "RELEASE_GATE_PHASE must be predeploy or final" >&2; exit 1 ;;
esac

require_env() {
  local name="$1"
  local value="${!name:-}"
  if [ -z "$value" ]; then
    echo "$name is required" >&2
    exit 1
  fi
}

is_evidence_uri() {
  case "$1" in
    https://*|s3://*|gs://*|az://*|azblob://*|worm://*|artifact://*|oci://*) return 0 ;;
    *) return 1 ;;
  esac
}

current_release_sha() {
  if [ -n "${RELEASE_GIT_SHA:-}" ]; then
    printf '%s\n' "$RELEASE_GIT_SHA"
    return
  fi
  if [ -n "${GITHUB_SHA:-}" ]; then
    printf '%s\n' "$GITHUB_SHA"
    return
  fi
  git -C "$repository_root" rev-parse HEAD
}

require_sha() {
  local value="$1"
  if ! printf '%s' "$value" | grep -Eq '^[0-9a-f]{40}$'; then
    echo "release Git SHA must be a full 40-character lowercase SHA-1" >&2
    exit 1
  fi
}

copy_artifact() {
  local source="$1"
  local target="$2"
  mkdir -p "$(dirname "$target")"
  if [ "$source" != "$target" ]; then
    cp "$source" "$target"
  fi
}

verify_external_evidence() {
  local name="$1"
  local uri="$2"
  local target="$3"
  local verifier="${RELEASE_EVIDENCE_METADATA_VERIFIER:-}"
  if [ -z "$verifier" ]; then
    echo "RELEASE_EVIDENCE_METADATA_VERIFIER is required for external evidence" >&2
    exit 1
  fi
  if [ ! -x "$verifier" ]; then
    echo "RELEASE_EVIDENCE_METADATA_VERIFIER must be an executable verifier: $verifier" >&2
    exit 1
  fi
  "$verifier" --uri "$uri" --git-sha "$RELEASE_GIT_SHA" >"$target"
  python3 - "$name" "$target" "$uri" "$RELEASE_GIT_SHA" <<'PY'
import json
import re
import sys

name, path, uri, git_sha = sys.argv[1:]
try:
    with open(path, encoding="utf-8") as handle:
        payload = json.load(handle)
except (OSError, json.JSONDecodeError) as exc:
    raise SystemExit(f"{name} external metadata verifier must return JSON: {exc}")

required = ("uri", "object_version", "sha256", "immutable_until", "verified_at", "git_sha")
missing = [field for field in required if not str(payload.get(field, "")).strip()]
if missing:
    raise SystemExit(f"{name} external metadata missing: {', '.join(missing)}")
if payload["uri"] != uri:
    raise SystemExit(f"{name} external metadata URI does not match evidence URI")
if payload["git_sha"] != git_sha:
    raise SystemExit(f"{name} external metadata Git SHA does not match current release")
if not re.fullmatch(r"[0-9a-f]{64}", payload["sha256"]):
    raise SystemExit(f"{name} external metadata SHA-256 is invalid")
PY
}

resolve_evidence() {
  local name="$1"
  local value="$2"
  local target="$3"
  if [ -z "$value" ]; then
    echo "$name is required" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$target")"
  if [ -s "$value" ]; then
    copy_artifact "$value" "$target"
    return
  fi
  if [ -e "$value" ]; then
    echo "$name must point to a non-empty artifact: $value" >&2
    exit 1
  fi
  if is_evidence_uri "$value"; then
    verify_external_evidence "$name" "$value" "$target"
    return
  fi
  echo "$name must point to a non-empty artifact or immutable evidence URI: $value" >&2
  exit 1
}

verify_local_evidence() {
  local name="$1"
  local path="$2"
  local expected_type="$3"
  python3 - "$name" "$path" "$expected_type" "$RELEASE_GIT_SHA" <<'PY'
import hashlib
import json
import sys

name, path, expected_type, git_sha = sys.argv[1:]
try:
    with open(path, "rb") as handle:
        content = handle.read()
    payload = json.loads(content)
except (OSError, json.JSONDecodeError) as exc:
    raise SystemExit(f"{name} must be valid JSON evidence: {exc}")

if payload.get("evidence_type") != expected_type:
    raise SystemExit(f"{name} evidence_type must be {expected_type}")
if payload.get("git_sha") != git_sha:
    raise SystemExit(f"{name} Git SHA does not match current release")
digest = payload.get("digest", "")
if not isinstance(digest, str) or not digest.startswith("sha256:") or len(digest) != 71:
    raise SystemExit(f"{name} digest must be a SHA-256 value")
normalized = dict(payload)
normalized["digest"] = "sha256:pending"
canonical = json.dumps(normalized, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
if digest != "sha256:" + hashlib.sha256(canonical).hexdigest():
    raise SystemExit(f"{name} digest does not bind its artifact content")

if expected_type == "rollback-drill" and payload.get("execution", {}).get("executed") is not True:
    raise SystemExit(f"{name} must record a completed real execution")
PY
}

verify_slo_evidence() {
  local path="$1"
  python3 - "$path" "$RELEASE_GIT_SHA" "${RELEASE_IMAGE_DIGESTS:-}" "${RELEASE_SLO_MIN_WINDOW_SECONDS:-1800}" <<'PY'
import datetime as dt
import json
import re
import sys

path, git_sha, expected_images, minimum_seconds = sys.argv[1:]
minimum_seconds = int(minimum_seconds)
with open(path, encoding="utf-8") as handle:
    payload = json.load(handle)

deployment = payload.get("deployment")
observation = payload.get("observation")
if not isinstance(deployment, dict) or not str(deployment.get("uid", "")).strip():
    raise SystemExit("SLO_OBSERVATION_ARTIFACT deployment UID is required")
if not isinstance(observation, dict):
    raise SystemExit("SLO_OBSERVATION_ARTIFACT observation window is required")

def timestamp(value, label):
    if not isinstance(value, str):
        raise SystemExit(f"SLO_OBSERVATION_ARTIFACT {label} is required")
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        raise SystemExit(f"SLO_OBSERVATION_ARTIFACT {label} is invalid")

deploy_at = timestamp(deployment.get("started_at"), "deployment.started_at")
started_at = timestamp(observation.get("started_at"), "observation.started_at")
finished_at = timestamp(observation.get("finished_at"), "observation.finished_at")
window_seconds = observation.get("window_seconds")
if not isinstance(window_seconds, int) or window_seconds < minimum_seconds:
    raise SystemExit("SLO_OBSERVATION_ARTIFACT observation window is shorter than required")
if started_at < deploy_at or finished_at < started_at:
    raise SystemExit("SLO_OBSERVATION_ARTIFACT observation timestamps must follow deployment")
if int((finished_at - started_at).total_seconds()) < window_seconds:
    raise SystemExit("SLO_OBSERVATION_ARTIFACT observation timestamps do not cover declared window")

images = deployment.get("images")
if not isinstance(images, list) or not images:
    raise SystemExit("SLO_OBSERVATION_ARTIFACT deployment images are required")
actual = {}
for image in images:
    if not isinstance(image, dict):
        continue
    name, digest = image.get("name"), image.get("digest")
    if isinstance(name, str) and isinstance(digest, str) and re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        actual[name] = digest
if len(actual) != len(images):
    raise SystemExit("SLO_OBSERVATION_ARTIFACT deployment image digests are invalid")
if expected_images:
    expected = dict(item.split("=", 1) for item in expected_images.split(",") if "=" in item)
    if expected != actual:
        raise SystemExit("SLO_OBSERVATION_ARTIFACT deployment image digests do not match current release")
PY
}

verify_dependency_policy() {
  local path="$1"
  python3 - "$path" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as handle:
        payload = json.load(handle)
except (OSError, json.JSONDecodeError) as exc:
    raise SystemExit(f"DEPENDENCY_POLICY_ARTIFACT must point to a non-empty local JSON artifact: {exc}")

if payload.get("status") != "passed":
    raise SystemExit("DEPENDENCY_POLICY_ARTIFACT status must be passed")
PY
}

verify_compatibility_matrix() {
  local path="$1"
  python3 - "$path" <<'PY'
import json
import sys

try:
    with open(sys.argv[1], encoding="utf-8") as handle:
        payload = json.load(handle)
except (OSError, json.JSONDecodeError) as exc:
    raise SystemExit(f"COMPATIBILITY_MATRIX_ARTIFACT must point to a valid local JSON artifact: {exc}")

if payload.get("status") != "passed":
    raise SystemExit("COMPATIBILITY_MATRIX_ARTIFACT status must be passed")
if not str(payload.get("framework_version", "")).strip():
    raise SystemExit("COMPATIBILITY_MATRIX_ARTIFACT framework_version is required")
modules = payload.get("modules")
if not isinstance(modules, list) or not modules:
    raise SystemExit("COMPATIBILITY_MATRIX_ARTIFACT modules must be non-empty")
incompatible = sorted(
    str(module.get("id", "")).strip()
    for module in modules
    if isinstance(module, dict) and module.get("enabled") is True and module.get("framework_compatible") is not True
)
if incompatible:
    raise SystemExit(f"enabled modules must be framework compatible: {', '.join(incompatible)}")
PY
}

require_env CHANGE_TICKET
case "$CHANGE_TICKET" in
  CHG-*|INC-*|RFC-*) ;;
  *)
    echo "CHANGE_TICKET must start with CHG-, INC-, or RFC-" >&2
    exit 1
    ;;
esac

RELEASE_GIT_SHA="$(current_release_sha)"
export RELEASE_GIT_SHA
require_sha "$RELEASE_GIT_SHA"

resolve_release_approver() {
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    require_env GITHUB_TOKEN
    require_env GITHUB_REPOSITORY
    require_env GITHUB_RUN_ID
    local approver
    approver="$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID/approvals" --jq '[.[] | select(.state == "approved") | .user.login][0] // ""')"
    if [ -z "$approver" ]; then
      echo "GitHub Environment approval record is required" >&2
      exit 1
    fi
    printf '%s\n' "$approver"
    return
  fi
  require_env RELEASE_APPROVER
  printf '%s\n' "$RELEASE_APPROVER"
}

release_approver="$(resolve_release_approver)"
if [ -n "${GITHUB_ACTOR:-}" ] && [ "$release_approver" = "$GITHUB_ACTOR" ]; then
  echo "release approver must differ from GITHUB_ACTOR" >&2
  exit 1
fi

mkdir -p artifacts
resolve_evidence ROLLBACK_DRILL_ARTIFACT "${ROLLBACK_DRILL_ARTIFACT:-}" artifacts/rollback-drill.json
if [ "$release_gate_phase" = "final" ]; then
  resolve_evidence SLO_OBSERVATION_ARTIFACT "${SLO_OBSERVATION_ARTIFACT:-}" artifacts/slo-observation.json
fi

if ! is_evidence_uri "${ROLLBACK_DRILL_ARTIFACT:-}"; then
  verify_local_evidence ROLLBACK_DRILL_ARTIFACT artifacts/rollback-drill.json rollback-drill
  python3 - artifacts/rollback-drill.json <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)
state_diff = payload.get("state_diff", {})
if state_diff.get("after_rollback") != "in_sync":
    raise SystemExit("ROLLBACK_DRILL_ARTIFACT must prove an in_sync post-rollback state diff")
execution = payload.get("execution", {})
if not str(execution.get("upgrade_run_key", "")).strip() or not str(execution.get("rollback_run_key", "")).strip():
    raise SystemExit("ROLLBACK_DRILL_ARTIFACT must include upgrade and rollback lifecycle run keys")
if execution.get("smoke") != "passed":
    raise SystemExit("ROLLBACK_DRILL_ARTIFACT must include a successful upgraded-state smoke check")
PY
fi
if [ "$release_gate_phase" = "final" ] && ! is_evidence_uri "${SLO_OBSERVATION_ARTIFACT:-}"; then
  verify_local_evidence SLO_OBSERVATION_ARTIFACT artifacts/slo-observation.json slo-observation
  verify_slo_evidence artifacts/slo-observation.json
fi

require_env DEPENDENCY_POLICY_ARTIFACT
if is_evidence_uri "$DEPENDENCY_POLICY_ARTIFACT" || [ ! -s "$DEPENDENCY_POLICY_ARTIFACT" ]; then
  echo "DEPENDENCY_POLICY_ARTIFACT must point to a non-empty local JSON artifact" >&2
  exit 1
fi
copy_artifact "$DEPENDENCY_POLICY_ARTIFACT" artifacts/dependency-policy.json
verify_dependency_policy artifacts/dependency-policy.json

require_env COMPATIBILITY_MATRIX_ARTIFACT
if is_evidence_uri "$COMPATIBILITY_MATRIX_ARTIFACT" || [ ! -s "$COMPATIBILITY_MATRIX_ARTIFACT" ]; then
  echo "COMPATIBILITY_MATRIX_ARTIFACT must point to a valid local JSON artifact" >&2
  exit 1
fi
copy_artifact "$COMPATIBILITY_MATRIX_ARTIFACT" artifacts/module-compatibility-matrix.json
verify_compatibility_matrix artifacts/module-compatibility-matrix.json

module_release_manifest=""
if [ -n "${MODULE_RELEASE_EVIDENCE_DIR:-}" ]; then
  if [ ! -d "$MODULE_RELEASE_EVIDENCE_DIR" ]; then
    echo "MODULE_RELEASE_EVIDENCE_DIR must point to generated module release evidence" >&2
    exit 1
  fi
  script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
  bash "$script_dir/release/verify-module-release-evidence.sh" \
    --target "$MODULE_RELEASE_EVIDENCE_DIR" \
    --git-sha "$RELEASE_GIT_SHA"
  module_release_target="artifacts/module-release"
  if [ "$MODULE_RELEASE_EVIDENCE_DIR" != "$module_release_target" ]; then
    if [ -e "$module_release_target" ]; then
      echo "module release evidence target already exists: $module_release_target" >&2
      exit 1
    fi
    cp -R "$MODULE_RELEASE_EVIDENCE_DIR" "$module_release_target"
  fi
  module_release_manifest="$module_release_target/evidence-manifest.json"
fi

python3 - "$CHANGE_TICKET" "$release_approver" "$RELEASE_GIT_SHA" "$(date -u +%FT%TZ)" "$module_release_manifest" "$release_gate_phase" <<'PY'
import hashlib
import json
from pathlib import Path
import sys

ticket, approver, git_sha, checked_at, module_release_manifest, phase = sys.argv[1:]
artifacts = {}
required_artifacts = {
    "rollback_drill": "artifacts/rollback-drill.json",
    "dependency_policy": "artifacts/dependency-policy.json",
    "compatibility_matrix": "artifacts/module-compatibility-matrix.json",
}
if phase == "final":
    required_artifacts["slo_observation"] = "artifacts/slo-observation.json"
for key, path in required_artifacts.items():
    content = Path(path).read_bytes()
    artifacts[key] = {"path": path, "sha256": hashlib.sha256(content).hexdigest(), "size": len(content)}
if module_release_manifest:
    content = Path(module_release_manifest).read_bytes()
    artifacts["module_release"] = {
        "path": module_release_manifest,
        "sha256": hashlib.sha256(content).hexdigest(),
        "size": len(content),
    }

payload = {
    "schema_version": 1,
    "phase": phase,
    "change_ticket": ticket,
    "release_approver": approver,
    "git_sha": git_sha,
    "checked_at": checked_at,
    "artifacts": artifacts,
}
target = "artifacts/release-predeploy-gate.json" if phase == "predeploy" else "artifacts/release-hard-gate.json"
Path(target).write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

echo "release $release_gate_phase hard gate passed"
