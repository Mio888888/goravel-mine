#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
required=(DB_HOST DB_PORT DB_DATABASE DB_USERNAME PGPASSWORD BACKUP_S3_BUCKET BACKUP_S3_PREFIX BACKUP_KMS_KEY_ID BACKUP_RETENTION_DAYS RELEASE_GIT_SHA RELEASE_IMAGE_DIGEST)
for name in "${required[@]}"; do
  [[ -n "${!name:-}" ]] || { echo "$name is required" >&2; exit 1; }
done
[[ "$RELEASE_GIT_SHA" =~ ^[0-9a-f]{40}$ ]] || { echo "RELEASE_GIT_SHA must be full lowercase SHA" >&2; exit 1; }
[[ "$RELEASE_IMAGE_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "RELEASE_IMAGE_DIGEST is invalid" >&2; exit 1; }
(( BACKUP_RETENTION_DAYS > 0 )) || { echo "BACKUP_RETENTION_DAYS must be positive" >&2; exit 1; }
for command in pg_dump psql aws python3 sha256sum base64; do
  command -v "$command" >/dev/null || { echo "$command is required" >&2; exit 1; }
done

work_dir="$(mktemp -d)"
active_snapshot_pid=""
cleanup() {
	if [[ -n "$active_snapshot_pid" ]] && kill -0 "$active_snapshot_pid" 2>/dev/null; then
		kill "$active_snapshot_pid" 2>/dev/null || true
		wait "$active_snapshot_pid" 2>/dev/null || true
	fi
	rm -rf "$work_dir"
}
trap cleanup EXIT
created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
retention_until="$(python3 - "$BACKUP_RETENTION_DAYS" <<'PY'
import datetime, sys
print((datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=int(sys.argv[1]))).strftime("%Y-%m-%dT%H:%M:%SZ"))
PY
)"
entries="$work_dir/entries.jsonl"
tenant_inventory="$work_dir/tenants.tsv"

schema_digest() {
	PGPASSWORD="$3" pg_dump --host "$1" --port "$2" --username "$4" --dbname "$5" --snapshot "$6" --schema-only --no-owner --no-privileges |
    sed -e '/^\\restrict /d' -e '/^\\unrestrict /d' |
    sha256sum |
    awk '{print $1}'
}

upload_database() {
  local host="$1" port="$2" database="$3" username="$4" password="$5" kind="$6" tenant_id="$7" tenant_code="$8"
	local identity safe_name dump_path object_key artifact_sha artifact_size schema_sha row_counts put_output version_id etag head_output snapshot_id snapshot_pid snapshot_commands snapshot_results
  identity="$kind"
  [[ "$kind" == "platform" ]] || identity="tenant-$tenant_id"
  safe_name="$(printf '%s-%s' "$identity" "$database" | tr -c 'A-Za-z0-9._-' '_')"
	dump_path="$work_dir/$safe_name.dump"
	object_key="${BACKUP_S3_PREFIX%/}/${created_at//:/-}/${safe_name}.dump"
	snapshot_commands="$work_dir/$safe_name.snapshot.in"
	snapshot_results="$work_dir/$safe_name.snapshot.out"
	mkfifo "$snapshot_commands" "$snapshot_results"
	PGPASSWORD="$password" psql --host "$host" --port "$port" --username "$username" --dbname "$database" --no-psqlrc --quiet --tuples-only --no-align <"$snapshot_commands" >"$snapshot_results" &
	snapshot_pid="$!"
	active_snapshot_pid="$snapshot_pid"
	exec 8>"$snapshot_commands"
	exec 9<"$snapshot_results"
	printf 'BEGIN ISOLATION LEVEL REPEATABLE READ, READ ONLY;\nSELECT pg_export_snapshot();\n' >&8
	IFS= read -r snapshot_id <&9
	[[ -n "$snapshot_id" ]] || { echo "failed to export snapshot for $database" >&2; exit 1; }
	PGPASSWORD="$password" pg_dump --host "$host" --port "$port" --username "$username" --dbname "$database" --snapshot "$snapshot_id" --format custom --file "$dump_path"
	artifact_sha="$(sha256sum "$dump_path" | awk '{print $1}')"
	artifact_size="$(wc -c < "$dump_path" | tr -d ' ')"
	schema_sha="$(schema_digest "$host" "$port" "$password" "$username" "$database" "$snapshot_id")"
	if [[ "$kind" == "platform" ]]; then
		row_counts="$(PGPASSWORD="$password" psql --host "$host" --port "$port" --username "$username" --dbname "$database" --quiet --tuples-only --no-align --command "BEGIN ISOLATION LEVEL REPEATABLE READ, READ ONLY; SET TRANSACTION SNAPSHOT '$snapshot_id'; SELECT json_build_object('tenant', (SELECT count(*) FROM tenant), 'platform_user', (SELECT count(*) FROM platform_user), 'platform_casbin_rule', (SELECT count(*) FROM platform_casbin_rule)); COMMIT")"
		PGPASSWORD="$password" psql --host "$host" --port "$port" --username "$username" --dbname "$database" --quiet --tuples-only --no-align --field-separator=$'\t' --command "BEGIN ISOLATION LEVEL REPEATABLE READ, READ ONLY; SET TRANSACTION SNAPSHOT '$snapshot_id'; SELECT id, code, db_host, db_port, db_database, db_username, replace(encode(convert_to(db_password, 'UTF8'), 'base64'), E'\\n', '') FROM tenant ORDER BY id; COMMIT" >"$tenant_inventory"
	else
		row_counts="$(PGPASSWORD="$password" psql --host "$host" --port "$port" --username "$username" --dbname "$database" --quiet --tuples-only --no-align --command "BEGIN ISOLATION LEVEL REPEATABLE READ, READ ONLY; SET TRANSACTION SNAPSHOT '$snapshot_id'; SELECT json_build_object('user', (SELECT count(*) FROM \"user\"), 'casbin_rule', (SELECT count(*) FROM casbin_rule)); COMMIT")"
	fi
	printf 'COMMIT;\n' >&8
	exec 8>&-
	exec 9>&-
	wait "$snapshot_pid"
	active_snapshot_pid=""
  put_output="$(aws s3api put-object --bucket "$BACKUP_S3_BUCKET" --key "$object_key" --body "$dump_path" --server-side-encryption aws:kms --ssekms-key-id "$BACKUP_KMS_KEY_ID" --object-lock-mode COMPLIANCE --object-lock-retain-until-date "$retention_until" --checksum-algorithm SHA256 --output json)"
  version_id="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("VersionId", ""))' <<<"$put_output")"
  etag="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("ETag", "").strip(chr(34)))' <<<"$put_output")"
  [[ -n "$version_id" && -n "$etag" ]] || { echo "immutable upload returned no version/etag for $database" >&2; exit 1; }
  head_output="$(aws s3api head-object --bucket "$BACKUP_S3_BUCKET" --key "$object_key" --version-id "$version_id" --output json)"
  python3 - "$head_output" "$BACKUP_KMS_KEY_ID" "$retention_until" <<'PY'
import datetime, json, sys
head=json.loads(sys.argv[1])
if head.get("ServerSideEncryption") != "aws:kms" or head.get("SSEKMSKeyId") != sys.argv[2]: raise SystemExit("uploaded backup KMS metadata mismatch")
if head.get("ObjectLockMode") != "COMPLIANCE": raise SystemExit("uploaded backup is not COMPLIANCE locked")
if datetime.datetime.fromisoformat(head["ObjectLockRetainUntilDate"].replace("Z", "+00:00")) < datetime.datetime.fromisoformat(sys.argv[3].replace("Z", "+00:00")): raise SystemExit("uploaded backup retention is shorter than requested")
PY
  python3 - "$entries" "$kind" "$tenant_id" "$tenant_code" "$host" "$port" "$database" "$username" "$artifact_sha" "$artifact_size" "$schema_sha" "$row_counts" "$object_key" "$version_id" "$etag" "$retention_until" "$BACKUP_S3_BUCKET" "$BACKUP_KMS_KEY_ID" <<'PY'
import json, pathlib, sys
path=pathlib.Path(sys.argv[1]); kind,tenant_id,tenant_code,host,port,database,username,sha,size,schema,counts,key,version,etag,retention,bucket,kms=sys.argv[2:]
identity="platform" if kind=="platform" else f"tenant:{tenant_id}"
item={"identity":identity,"kind":kind,"tenant_id":int(tenant_id),"tenant_code":tenant_code,"host":host,"port":int(port),"database":database,"username":username,"artifact":{"sha256":sha,"size":int(size),"format":"postgres-custom","encryption":"aws:kms","kms_key_id":kms},"object":{"uri":f"s3://{bucket}/{key}","version_id":version,"etag":etag,"lock_mode":"COMPLIANCE","retention_until":retention},"validation":{"schema_sha256":schema,"key_row_counts":json.loads(counts)}}
with path.open("a", encoding="utf-8") as handle: handle.write(json.dumps(item,sort_keys=True,separators=(",",":"))+"\n")
PY
}

upload_database "$DB_HOST" "$DB_PORT" "$DB_DATABASE" "$DB_USERNAME" "$PGPASSWORD" platform 0 ""
while IFS=$'\t' read -r tenant_id tenant_code tenant_host tenant_port tenant_database tenant_username password_b64; do
  [[ -n "$tenant_id" ]] || continue
  upload_database "$tenant_host" "$tenant_port" "$tenant_database" "$tenant_username" "$(printf '%s' "$password_b64" | base64 -d)" tenant "$tenant_id" "$tenant_code"
done <"$tenant_inventory"

manifest_path="$work_dir/manifest.json"
python3 - "$entries" "$manifest_path" "$created_at" "$DB_DATABASE" "$RELEASE_GIT_SHA" "$RELEASE_IMAGE_DIGEST" <<'PY'
import json, pathlib, sys
entries=[json.loads(line) for line in pathlib.Path(sys.argv[1]).read_text().splitlines() if line]
tenant_count=sum(1 for item in entries if item["kind"]=="tenant")
manifest={"schema":"goravel-backup-manifest/v2","created_at":sys.argv[3],"source_database":sys.argv[4],"release_git_sha":sys.argv[5],"image_digest":sys.argv[6],"databases":entries,"validation":{"tenant_count":tenant_count,"database_count":len(entries)}}
pathlib.Path(sys.argv[2]).write_text(json.dumps(manifest,sort_keys=True,separators=(",",":"))+"\n")
PY
"$script_dir/verify-backup-manifest.sh" "$manifest_path"
manifest_key="${BACKUP_S3_PREFIX%/}/${created_at//:/-}/manifest.json"
manifest_put="$(aws s3api put-object --bucket "$BACKUP_S3_BUCKET" --key "$manifest_key" --body "$manifest_path" --content-type application/json --server-side-encryption aws:kms --ssekms-key-id "$BACKUP_KMS_KEY_ID" --object-lock-mode COMPLIANCE --object-lock-retain-until-date "$retention_until" --checksum-algorithm SHA256 --output json)"
manifest_version="$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("VersionId", ""))' <<<"$manifest_put")"
[[ -n "$manifest_version" ]] || { echo "manifest upload returned no version" >&2; exit 1; }
output="${BACKUP_MANIFEST_OUTPUT:-artifacts/backup/manifest.json}"
mkdir -p "$(dirname "$output")"
cp "$manifest_path" "$output"
printf 'backup complete: %s manifest_version=%s\n' "s3://$BACKUP_S3_BUCKET/$manifest_key" "$manifest_version"
