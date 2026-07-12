#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

schema_digest() {
  pg_dump --host "$RESTORE_DB_HOST" --port "$RESTORE_DB_PORT" --username "$RESTORE_DB_USERNAME" --dbname "$1" --schema-only --no-owner --no-privileges |
    sed -e '/^\\restrict /d' -e '/^\\unrestrict /d' |
    sha256sum |
    awk '{print $1}'
}

manifest=""
target_db=""
source_db=""
source_tenant_id=""
while (($#)); do
  case "$1" in
    --source) manifest="${2:-}"; shift 2 ;;
    --source-db) source_db="${2:-}"; shift 2 ;;
    --source-tenant-id) source_tenant_id="${2:-}"; shift 2 ;;
    --target-db) target_db="${2:-}"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done
[[ -f "$manifest" && -n "$target_db" ]] || { echo "usage: scripts/restore-drill.sh --source manifest.json --target-db isolated_db" >&2; exit 1; }
[[ "$target_db" =~ (_restore_|_drill_) ]] || { echo "target DB must contain _restore_ or _drill_" >&2; exit 1; }
[[ "$target_db" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]] || { echo "target DB must be a PostgreSQL identifier" >&2; exit 1; }

[[ -z "$source_tenant_id" || "$source_tenant_id" =~ ^[1-9][0-9]*$ ]] || { echo "source tenant ID must be positive" >&2; exit 1; }
[[ -n "$source_db" || -n "$source_tenant_id" ]] || source_db="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["source_database"])' "$manifest")"
if [[ -n "$source_tenant_id" ]]; then
  source_db="$(python3 - "$manifest" "$source_tenant_id" <<'PY'
import json,sys
matches=[item for item in json.load(open(sys.argv[1]))["databases"] if item.get("tenant_id")==int(sys.argv[2])]
if len(matches)!=1: raise SystemExit("source tenant is not present exactly once in manifest")
print(matches[0]["database"])
PY
)"
fi
target_db_lower="$(printf '%s' "$target_db" | tr '[:upper:]' '[:lower:]')"
source_db_lower="$(printf '%s' "$source_db" | tr '[:upper:]' '[:lower:]')"
case "$target_db_lower" in
  *prod*|*production*|"$source_db_lower") echo "refusing production-like or source database target" >&2; exit 1 ;;
esac
"$script_dir/verify-backup-manifest.sh" "$manifest"

for name in RESTORE_DB_HOST RESTORE_DB_PORT RESTORE_DB_USERNAME RESTORE_PGPASSWORD; do
  [[ -n "${!name:-}" ]] || { echo "$name is required" >&2; exit 1; }
done
for command in aws pg_dump pg_restore psql createdb python3 sed sha256sum; do command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }; done

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
dump_path="$work_dir/backup.dump"
selection="$work_dir/selection.tsv"
python3 - "$manifest" "$source_db" "$source_tenant_id" >"$selection" <<'PY'
import json, sys, urllib.parse
m=json.load(open(sys.argv[1])); matches=[item for item in m["databases"] if item.get("tenant_id")==int(sys.argv[3])] if sys.argv[3] else [item for item in m["databases"] if item["database"]==sys.argv[2]]
if len(matches)!=1: raise SystemExit("source database is not present exactly once in manifest")
item=matches[0]; u=urllib.parse.urlparse(item["object"]["uri"])
print(u.netloc, u.path.lstrip("/"), item["object"]["version_id"], item["identity"], item["database"], sep="\t")
PY
IFS=$'\t' read -r bucket key version_id source_identity source_db <"$selection"
aws s3api get-object --bucket "$bucket" --key "$key" --version-id "$version_id" "$dump_path" >/dev/null
BACKUP_ARTIFACT_PATH="$dump_path" BACKUP_DATABASE_IDENTITY="$source_identity" "$script_dir/verify-backup-manifest.sh" "$manifest"

export PGHOST="$RESTORE_DB_HOST" PGPORT="$RESTORE_DB_PORT" PGUSER="$RESTORE_DB_USERNAME" PGPASSWORD="$RESTORE_PGPASSWORD"
if psql --dbname postgres --tuples-only --no-align --command "SELECT 1 FROM pg_database WHERE datname = '$target_db'" | grep -q 1; then
  echo "target DB already exists; refusing overwrite" >&2
  exit 1
fi
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
createdb "$target_db"
restore_failed=true
trap 'if $restore_failed; then echo "restore failed; isolated DB retained: '$target_db'" >&2; fi; rm -rf "$work_dir"' EXIT
pg_restore --dbname "$target_db" --no-owner --no-privileges "$dump_path"

actual_schema="$(schema_digest "$target_db")"
validation_sql="$work_dir/validation.sql"
python3 - "$manifest" "$source_identity" "$actual_schema" "$validation_sql" <<'PY'
import json, pathlib, re, sys
m=json.load(open(sys.argv[1])); item=next(entry for entry in m["databases"] if entry["identity"]==sys.argv[2])
expected=item["validation"]
if sys.argv[3] != expected["schema_sha256"]: raise SystemExit("restore validation schema digest mismatch")
counts=expected["key_row_counts"]
for table in counts:
    if not re.fullmatch(r"[a-z_][a-z0-9_]*",table): raise SystemExit("invalid validation table")
queries=[f"SELECT '{table}', count(*) FROM \"{table}\";" for table in sorted(counts)]
pathlib.Path(sys.argv[4]).write_text("\n".join(queries)+"\n")
PY
actual_counts="$work_dir/actual-counts.tsv"
psql --dbname "$target_db" --tuples-only --no-align --field-separator=$'\t' --file "$validation_sql" >"$actual_counts"
python3 - "$manifest" "$source_identity" "$actual_counts" <<'PY'
import json, pathlib, sys
m=json.load(open(sys.argv[1])); item=next(entry for entry in m["databases"] if entry["identity"]==sys.argv[2])
expected=item["validation"]["key_row_counts"]
actual={}
for line in pathlib.Path(sys.argv[3]).read_text().splitlines():
    table,count=line.split("\t",1); actual[table]=int(count)
if actual != expected: raise SystemExit(f"restore validation row counts mismatch: expected={expected} actual={actual}")
PY

casbin_table="$(python3 - "$manifest" "$source_identity" <<'PY'
import json,sys
m=json.load(open(sys.argv[1])); item=next(entry for entry in m["databases"] if entry["identity"]==sys.argv[2])
print("platform_casbin_rule" if item["kind"]=="platform" else "casbin_rule")
PY
)"
psql --dbname "$target_db" --tuples-only --no-align --command "SELECT count(*) FROM \"$casbin_table\" WHERE ptype IN ('p','g')" | grep -Eq '^[1-9][0-9]*$'
[[ -z "${RESTORE_READY_URL:-}" ]] || curl -fsS "$RESTORE_READY_URL" >/dev/null
completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
report="${RESTORE_REPORT_OUTPUT:-artifacts/backup/restore-report.json}"
mkdir -p "$(dirname "$report")"
python3 - "$manifest" "$report" "$target_db" "$started_at" "$completed_at" <<'PY'
import datetime, json, pathlib, sys
m=json.load(open(sys.argv[1])); start=datetime.datetime.fromisoformat(sys.argv[4].replace('Z','+00:00')); end=datetime.datetime.fromisoformat(sys.argv[5].replace('Z','+00:00')); created=datetime.datetime.fromisoformat(m['created_at'].replace('Z','+00:00'))
r={"schema":"goravel-restore-report/v1","source_manifest_sha256":__import__('hashlib').sha256(pathlib.Path(sys.argv[1]).read_bytes()).hexdigest(),"target_database":sys.argv[3],"started_at":sys.argv[4],"completed_at":sys.argv[5],"rto_seconds":int((end-start).total_seconds()),"rpo_seconds":int((start-created).total_seconds()),"validation":"passed"}
pathlib.Path(sys.argv[2]).write_text(json.dumps(r,sort_keys=True,separators=(',',':'))+'\n')
PY
restore_failed=false
printf 'restore drill passed: target=%s report=%s\n' "$target_db" "$report"
