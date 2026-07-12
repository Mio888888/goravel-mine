#!/usr/bin/env bash
set -euo pipefail

action=""
target=""
framework_version=""
admission_lock="${MODULE_ADMISSION_LOCK:-}"
sdk_path="${MODULE_SDK_PATH:-MineAdmin-web/src/generated/admin-api.ts}"
admission_registry="${MODULE_ADMISSION_REGISTRY:-}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --action)
      action="$2"
      shift 2
      ;;
    --target)
      target="$2"
      shift 2
      ;;
    --framework-version)
      framework_version="$2"
      shift 2
      ;;
    --admission-lock)
      admission_lock="$2"
      shift 2
      ;;
    --admission-registry)
      admission_registry="$2"
      shift 2
      ;;
    *)
      echo "unsupported argument: $1" >&2
      exit 2
      ;;
  esac
done

if [ "$action" != "upgrade" ] && [ "$action" != "rollback" ]; then
  echo "--action must be upgrade or rollback" >&2
  exit 2
fi
if [ -z "$target" ]; then
  echo "--target is required" >&2
  exit 2
fi
if [ -z "$framework_version" ]; then
  framework_version="$(go list -m -f '{{.Version}}' github.com/goravel/framework)"
  framework_version="${framework_version#v}"
fi
if [ -z "$admission_lock" ] || [ ! -s "$admission_lock" ]; then
  echo "MODULE_ADMISSION_LOCK or --admission-lock must point to a non-empty admission lock" >&2
  exit 1
fi
if [ -z "$admission_registry" ] || [ ! -s "$admission_registry" ]; then
  echo "MODULE_ADMISSION_REGISTRY or --admission-registry must point to a generated static registry" >&2
  exit 1
fi
if [ ! -s "$sdk_path" ]; then
  echo "MODULE_SDK_PATH must point to a non-empty generated SDK" >&2
  exit 1
fi
if [ -z "${MODULE_OPENAPI_BUNDLE:-}" ] || [ ! -s "$MODULE_OPENAPI_BUNDLE" ]; then
  echo "MODULE_OPENAPI_BUNDLE must point to a non-empty OpenAPI bundle" >&2
  exit 1
fi

git_sha="$(git rev-parse HEAD)"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/module-release-evidence.XXXXXX")"
cleanup() {
  rm -rf "$temporary"
}
trap cleanup EXIT

mkdir -p "$temporary/artifacts"
go run . artisan module:manifest:check --artifacts --frontend >"$temporary/artifacts/manifest-check.txt"
go run . artisan module:manifest:export --target="$temporary/artifacts/module-manifest.json" >"$temporary/artifacts/manifest-export.txt"
go run . artisan module:compatibility:export --framework-version="$framework_version" --target="$temporary/artifacts/module-compatibility-matrix.json" >"$temporary/artifacts/compatibility-export.txt"
go run . artisan module:state >"$temporary/artifacts/module-state-before.json"
go run . artisan module:plan --action="$action" >"$temporary/artifacts/module-${action}-plan.json"
go run . artisan module:lifecycle --action="$action" >"$temporary/artifacts/module-${action}-dry-run.json"
go run . artisan module:plan --action=rollback >"$temporary/artifacts/module-rollback-plan.json"
go run . artisan module:lifecycle --action=rollback >"$temporary/artifacts/module-rollback-dry-run.json"
cp "$admission_lock" "$temporary/artifacts/module-admission.lock.json"
cp "$admission_registry" "$temporary/artifacts/admitted_modules_gen.go"
cp "$sdk_path" "$temporary/artifacts/admin-api.ts"
cp "$MODULE_OPENAPI_BUNDLE" "$temporary/artifacts/modules.openapi.json"

python3 - "$temporary" "$git_sha" "$action" "$framework_version" <<'PY'
import hashlib
import json
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
git_sha, action, framework_version = sys.argv[2:]
lock = json.loads((root / "artifacts/module-admission.lock.json").read_text(encoding="utf-8"))
lock_digest = lock.get("digest")
if not isinstance(lock_digest, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", lock_digest):
    raise SystemExit("module admission lock digest is invalid")
registry = (root / "artifacts/admitted_modules_gen.go").read_text(encoding="utf-8")
if f'const AdmittedRegistryLockDigest = "{lock_digest}"' not in registry:
    raise SystemExit("generated static registry is not bound to the admission lock digest")

artifacts = []
media_types = {".json": "application/json", ".ts": "text/typescript", ".txt": "text/plain"}
for path in sorted((root / "artifacts").iterdir()):
    content = path.read_bytes()
    artifacts.append({
        "path": str(path.relative_to(root)),
        "media_type": media_types.get(path.suffix, "application/octet-stream"),
        "size": len(content),
        "sha256": hashlib.sha256(content).hexdigest(),
        "producer": "scripts/release/generate-module-release-evidence.sh",
    })

manifest = {
    "schema_version": 1,
    "evidence_type": "module-release",
    "action": action,
    "git_sha": git_sha,
    "framework_version": framework_version,
    "admission_lock_digest": lock_digest,
    "artifacts": artifacts,
}
(root / "evidence-manifest.json").write_bytes(json.dumps(manifest, ensure_ascii=False, indent=2).encode() + b"\n")
PY

parent="$(dirname "$target")"
mkdir -p "$parent"
if [ -e "$target" ]; then
  echo "target already exists; refusing to replace release evidence: $target" >&2
  exit 1
fi
mv "$temporary" "$target"
trap - EXIT
printf '%s\n' "$target/evidence-manifest.json"
