#!/usr/bin/env bash
set -euo pipefail

target=""
expected_sha="${RELEASE_GIT_SHA:-${GITHUB_SHA:-}}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --target)
      target="$2"
      shift 2
      ;;
    --git-sha)
      expected_sha="$2"
      shift 2
      ;;
    *)
      echo "unsupported argument: $1" >&2
      exit 2
      ;;
  esac
done

if [ -z "$target" ] || [ -z "$expected_sha" ]; then
  echo "--target and --git-sha (or RELEASE_GIT_SHA/GITHUB_SHA) are required" >&2
  exit 2
fi

python3 - "$target" "$expected_sha" <<'PY'
import hashlib
import json
from pathlib import Path
import sys

root = Path(sys.argv[1])
expected_sha = sys.argv[2]
manifest_path = root / "evidence-manifest.json"
try:
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
except (OSError, json.JSONDecodeError) as exc:
    raise SystemExit(f"invalid module release evidence manifest: {exc}")

if manifest.get("evidence_type") != "module-release":
    raise SystemExit("module release evidence manifest has invalid evidence_type")
if manifest.get("git_sha") != expected_sha:
    raise SystemExit("module release evidence manifest Git SHA does not match current release")
artifacts = manifest.get("artifacts")
if not isinstance(artifacts, list) or not artifacts:
    raise SystemExit("module release evidence manifest artifacts are required")

required = {
    "artifacts/module-manifest.json",
    "artifacts/module-compatibility-matrix.json",
    "artifacts/module-admission.lock.json",
    "artifacts/admitted_modules_gen.go",
    "artifacts/modules.openapi.json",
    "artifacts/admin-api.ts",
    f"artifacts/module-{manifest.get('action')}-plan.json",
    f"artifacts/module-{manifest.get('action')}-dry-run.json",
}
seen = set()
admission_lock_digest = ""
admission_registry = ""
for artifact in artifacts:
    if not isinstance(artifact, dict):
        raise SystemExit("module release evidence manifest artifact must be an object")
    relative = artifact.get("path")
    if not isinstance(relative, str) or not relative.startswith("artifacts/"):
        raise SystemExit("module release evidence manifest artifact path is invalid")
    path = root / relative
    if not path.is_file():
        raise SystemExit(f"module release evidence artifact is missing: {relative}")
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    if artifact.get("sha256") != digest:
        raise SystemExit(f"module release evidence artifact digest mismatch: {relative}")
    if artifact.get("size") != path.stat().st_size:
        raise SystemExit(f"module release evidence artifact size mismatch: {relative}")
    seen.add(relative)
    if relative == "artifacts/module-admission.lock.json":
        admission_lock_digest = json.loads(path.read_text(encoding="utf-8")).get("digest", "")
    if relative == "artifacts/admitted_modules_gen.go":
        admission_registry = path.read_text(encoding="utf-8")
missing = sorted(required - seen)
if missing:
    raise SystemExit("module release evidence manifest missing required artifacts: " + ", ".join(missing))
if manifest.get("admission_lock_digest") != admission_lock_digest:
    raise SystemExit("module release evidence manifest admission lock digest mismatch")
if f'const AdmittedRegistryLockDigest = "{admission_lock_digest}"' not in admission_registry:
    raise SystemExit("module release evidence static registry binding does not match admission lock")
PY
