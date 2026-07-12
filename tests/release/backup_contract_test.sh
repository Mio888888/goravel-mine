#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
work_dir="$(mktemp -d)"; trap 'rm -rf "$work_dir"' EXIT
artifact="$work_dir/backup.dump"; printf 'immutable backup fixture\n' > "$artifact"
digest="$(sha256sum "$artifact" | awk '{print $1}')"; size="$(wc -c < "$artifact" | tr -d ' ')"
future="$(python3 - <<'PY'
import datetime
print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(days=30)).strftime('%Y-%m-%dT%H:%M:%SZ'))
PY
)"
write_manifest() {
  local version="$1" sha="$2" retention="$3" tenant_count="${4:-1}"
  python3 - "$work_dir/manifest.json" "$version" "$sha" "$retention" "$size" "$tenant_count" <<'PY'
import json,pathlib,sys
def entry(kind,database,tenant_id):
    counts={"platform_user":1,"platform_casbin_rule":1,"tenant":1} if kind=="platform" else {"user":1,"casbin_rule":1}
    identity="platform" if kind=="platform" else f"tenant:{tenant_id}"
    return {"identity":identity,"kind":kind,"tenant_id":tenant_id,"tenant_code":"default" if tenant_id else "","host":"db.internal","port":5432,"database":database,"username":"user","artifact":{"sha256":sys.argv[3],"size":int(sys.argv[5]),"format":"postgres-custom","encryption":"aws:kms","kms_key_id":"test-key"},"object":{"uri":"s3://bucket/"+identity,"version_id":sys.argv[2],"etag":"etag","lock_mode":"COMPLIANCE","retention_until":sys.argv[4]},"validation":{"schema_sha256":"c"*64,"key_row_counts":counts}}
items=[entry("platform","goravel_mine",0),entry("tenant","tenant_default",1)]
m={"schema":"goravel-backup-manifest/v2","created_at":"2026-07-11T00:00:00Z","source_database":"goravel_mine","release_git_sha":"a"*40,"image_digest":"sha256:"+"b"*64,"databases":items,"validation":{"tenant_count":int(sys.argv[6]),"database_count":len(items)}}
pathlib.Path(sys.argv[1]).write_text(json.dumps(m))
PY
}
expect_fail() { local expected="$1"; shift; if output="$($@ 2>&1)"; then echo "command unexpectedly passed: $*" >&2; exit 1; fi; grep -Fq "$expected" <<<"$output" || { echo "missing error '$expected': $output" >&2; exit 1; }; }
write_manifest version-1 "$digest" "$future"
BACKUP_ARTIFACT_PATH="$artifact" BACKUP_DATABASE_NAME=goravel_mine "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json" >/dev/null
write_manifest "" "$digest" "$future"; expect_fail "backup object missing version_id" "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
write_manifest version-1 "$digest" "$future" 0; expect_fail "tenant coverage mismatch" "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
write_manifest version-1 "$digest" "$future"
python3 - "$work_dir/manifest.json" <<'PY'
import json,pathlib,sys
path=pathlib.Path(sys.argv[1]); manifest=json.loads(path.read_text()); manifest["databases"][0]["validation"]["key_row_counts"]["tenant"]=2; path.write_text(json.dumps(manifest))
PY
expect_fail "tenant inventory does not match platform snapshot" "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
write_manifest version-1 "$digest" "$future"; printf 'tampered\n' >> "$artifact"; expect_fail "checksum mismatch" env BACKUP_ARTIFACT_PATH="$artifact" BACKUP_DATABASE_NAME=goravel_mine "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
write_manifest version-1 "$digest" "2020-01-01T00:00:00Z"; expect_fail "retention lock is expired" "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
write_manifest version-1 "$digest" "$future"; expect_fail "refusing production-like" "$repo_root/scripts/restore-drill.sh" --source "$work_dir/manifest.json" --target-db goravel_mine_production_restore_test
expect_fail "PostgreSQL identifier" "$repo_root/scripts/restore-drill.sh" --source "$work_dir/manifest.json" --target-db 'safe_restore_;DROP_DATABASE'
printf 'immutable backup fixture\n' > "$artifact"
write_manifest version-1 "$digest" "$future"
python3 - "$work_dir/manifest.json" <<'PY'
import json,pathlib,sys
path=pathlib.Path(sys.argv[1]); manifest=json.loads(path.read_text()); duplicate=dict(manifest["databases"][1]); duplicate.update({"identity":"tenant:2","tenant_id":2,"host":"other-db.internal"}); manifest["databases"].append(duplicate); manifest["databases"][0]["validation"]["key_row_counts"]["tenant"]=2; manifest["validation"].update({"tenant_count":2,"database_count":3}); path.write_text(json.dumps(manifest))
PY
"$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json" >/dev/null
expect_fail "backup selector must identify one" env BACKUP_ARTIFACT_PATH="$artifact" BACKUP_DATABASE_NAME=tenant_default "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
BACKUP_ARTIFACT_PATH="$artifact" BACKUP_DATABASE_IDENTITY=tenant:2 "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json" >/dev/null
python3 - "$work_dir/manifest.json" <<'PY'
import json,pathlib,sys
path=pathlib.Path(sys.argv[1]); manifest=json.loads(path.read_text()); manifest["databases"][2]["host"]="DB.INTERNAL"; path.write_text(json.dumps(manifest))
PY
expect_fail "backup database location is duplicated" "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
python3 - "$work_dir/manifest.json" <<'PY'
import json,pathlib,sys
path=pathlib.Path(sys.argv[1]); manifest=json.loads(path.read_text()); manifest["databases"][0]["host"]="localhost"; manifest["databases"][0]["database"]="tenant_default"; manifest["databases"][2]["host"]="127.0.0.1"; path.write_text(json.dumps(manifest))
PY
expect_fail "backup database location is duplicated" "$repo_root/scripts/verify-backup-manifest.sh" "$work_dir/manifest.json"
grep -Fq 'RESTORE_PGPASSWORD' "$repo_root/deploy/helm/goravel-mine/templates/restore-drill-job.yaml"
grep -Fq 'sha256sum' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq 'db_host, db_port, db_database' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq "replace(encode(convert_to(db_password, 'UTF8'), 'base64')" "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq 'pg_dump --host "$host" --port "$port"' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq 'SELECT pg_export_snapshot()' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq -- '--snapshot "$snapshot_id" --format custom' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq -- '--snapshot "$6" --schema-only' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq "SET TRANSACTION SNAPSHOT '\$snapshot_id'" "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq -- "SET TRANSACTION SNAPSHOT '\$snapshot_id'; SELECT id, code, db_host" "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq -- '--quiet --tuples-only --no-align --command "BEGIN ISOLATION LEVEL REPEATABLE READ' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq '>"$tenant_inventory"' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq 'identity="tenant-$tenant_id"' "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq -- '--source-tenant-id' "$repo_root/scripts/restore-drill.sh"
grep -Fq 'sha256sum' "$repo_root/scripts/restore-drill.sh"
if grep -Fq 'shasum' "$repo_root/scripts/restore-drill.sh"; then
  echo "restore drill must not depend on shasum" >&2
  exit 1
fi
grep -Fq 'item["validation"]["key_row_counts"]' "$repo_root/scripts/restore-drill.sh"
if grep -Fq 'entry["database"]==sys.argv[2]' "$repo_root/scripts/restore-drill.sh"; then
  echo "restore validation must retain manifest identity selection" >&2
  exit 1
fi
grep -Fq 'entry["identity"]==sys.argv[2]' "$repo_root/scripts/restore-drill.sh"
grep -Fq "/^\\\\restrict /d" "$repo_root/scripts/backup-to-object-storage.sh"
grep -Fq "/^\\\\unrestrict /d" "$repo_root/scripts/restore-drill.sh"
echo "backup contract tests passed"
