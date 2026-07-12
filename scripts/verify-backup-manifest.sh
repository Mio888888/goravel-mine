#!/usr/bin/env bash
set -euo pipefail
manifest_path="${1:?usage: scripts/verify-backup-manifest.sh <manifest.json>}"
artifact_path="${BACKUP_ARTIFACT_PATH:-}"
database_name="${BACKUP_DATABASE_NAME:-}"
database_identity="${BACKUP_DATABASE_IDENTITY:-}"
python3 - "$manifest_path" "$artifact_path" "$database_name" "$database_identity" <<'PY'
import datetime, hashlib, json, pathlib, re, sys
manifest=json.loads(pathlib.Path(sys.argv[1]).read_text()); artifact_path=pathlib.Path(sys.argv[2]) if sys.argv[2] else None; selected=sys.argv[3]; selected_identity=sys.argv[4]
for key,typ in {"created_at":str,"source_database":str,"release_git_sha":str,"image_digest":str,"databases":list,"validation":dict}.items():
    if not isinstance(manifest.get(key),typ): raise SystemExit(f"backup manifest missing or invalid {key}")
if manifest.get("schema") != "goravel-backup-manifest/v2": raise SystemExit("unsupported backup manifest schema")
if not re.fullmatch(r"[0-9a-f]{40}",manifest["release_git_sha"]): raise SystemExit("release_git_sha must be a full lowercase Git SHA")
if not re.fullmatch(r"sha256:[0-9a-f]{64}",manifest["image_digest"]): raise SystemExit("image_digest must be sha256:<64 lowercase hex>")
items=manifest["databases"]
if not items or sum(item.get("kind")=="tenant" for item in items) != int(manifest["validation"].get("tenant_count",-1)): raise SystemExit("backup manifest tenant coverage mismatch")
if len(items) != int(manifest["validation"].get("database_count",-1)): raise SystemExit("backup manifest database coverage mismatch")
identities=set(); locations=set()
platform_tenant_count=None
def normalize_host(host):
    value=host.strip().lower()
    return "127.0.0.1" if value in {"localhost","::1","[::1]"} else value
for item in items:
    name=item.get("database",""); identity=item.get("identity",""); host=item.get("host",""); port=item.get("port",0)
    expected_identity="platform" if item.get("kind")=="platform" else f"tenant:{item.get('tenant_id')}"
    if not name or not host or not isinstance(port,int) or port<=0: raise SystemExit("backup database location is invalid")
    if identity != expected_identity or identity in identities: raise SystemExit("backup database identity is missing or duplicated")
    location=(normalize_host(host),port,name);
    if location in locations: raise SystemExit("backup database location is duplicated")
    identities.add(identity); locations.add(location); artifact=item.get("artifact",{}); obj=item.get("object",{}); validation=item.get("validation",{})
    if artifact.get("encryption")!="aws:kms" or not artifact.get("kms_key_id"): raise SystemExit("backup artifact must use an explicit aws:kms key")
    if not re.fullmatch(r"[0-9a-f]{64}",str(artifact.get("sha256",""))) or int(artifact.get("size",0))<=0: raise SystemExit("backup artifact digest or size is invalid")
    if not re.fullmatch(r"[0-9a-f]{64}",str(validation.get("schema_sha256",""))): raise SystemExit("validation schema_sha256 is invalid")
    counts=validation.get("key_row_counts")
    if not isinstance(counts,dict) or not counts or any(not isinstance(value,int) or value<0 for value in counts.values()): raise SystemExit("validation key_row_counts is invalid")
    if item.get("kind")=="platform": platform_tenant_count=counts.get("tenant")
    for key in ("uri","version_id","etag","retention_until"):
        if not str(obj.get(key,"")).strip(): raise SystemExit(f"backup object missing {key}")
    if not obj["uri"].startswith("s3://") or obj.get("lock_mode")!="COMPLIANCE": raise SystemExit("backup object must be immutable s3")
    if datetime.datetime.fromisoformat(obj["retention_until"].replace("Z","+00:00")) <= datetime.datetime.now(datetime.timezone.utc): raise SystemExit("backup object retention lock is expired")
if not isinstance(platform_tenant_count,int) or platform_tenant_count != int(manifest["validation"].get("tenant_count",-1)): raise SystemExit("backup manifest tenant inventory does not match platform snapshot")
if artifact_path:
    matches=[item for item in items if item["identity"]==selected_identity] if selected_identity else [item for item in items if item["database"]==selected]
    if len(matches)!=1: raise SystemExit("backup selector must identify one manifest database")
    artifact=matches[0]["artifact"]
    if hashlib.sha256(artifact_path.read_bytes()).hexdigest()!=artifact["sha256"]: raise SystemExit("backup artifact checksum mismatch")
    if artifact_path.stat().st_size!=int(artifact["size"]): raise SystemExit("backup artifact size mismatch")
print(f"backup manifest verified: databases={len(items)}")
PY
