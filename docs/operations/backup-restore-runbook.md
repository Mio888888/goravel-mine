# Immutable Backup And Restore Drill

Production backup uses `scripts/backup-to-object-storage.sh`. For each database it keeps one exported `REPEATABLE READ, READ ONLY` snapshot alive while creating the custom dump, schema digest, and key row counts, so all validation evidence describes the exact dumped state. It uploads every artifact with SSE-KMS and S3 Object Lock `COMPLIANCE`, pins each returned object version, and emits `goravel-backup-manifest/v2`. A failed tenant inventory query, missing database dump, version, KMS key, digest, row-count proof, or live retention lock fails the run.

Required runtime values are `BACKUP_S3_BUCKET`, `BACKUP_S3_PREFIX`, `BACKUP_KMS_KEY_ID`, `BACKUP_RETENTION_DAYS`, `RELEASE_GIT_SHA`, and `RELEASE_IMAGE_DIGEST`, plus PostgreSQL connection variables. The bucket must have versioning and Object Lock enabled before first use. The Helm CronJob runs only the immutable backup image entrypoint and supplies the release SHA and immutable image digest through values; it does not prune a writable PVC.

Verify evidence with:

```bash
bash scripts/verify-backup-manifest.sh artifacts/backup/manifest.json
```

Restore only into a distinct namespace and an isolated database whose name contains `_restore_` or `_drill_`. Set `restoreDrill.targetNamespace`, `restoreDrill.targetDatabaseHost`, `restoreDrill.targetDatabaseUsername`, and `restoreDrill.targetDatabase`; the chart refuses an invalid database name and requires a source manifest Secret.

```bash
RESTORE_DB_HOST=127.0.0.1 RESTORE_DB_PORT=5432 \
RESTORE_DB_USERNAME=restore_operator RESTORE_PGPASSWORD=... \
bash scripts/restore-drill.sh \
  --source artifacts/backup/manifest.json \
  --target-db goravel_mine_restore_test
```

The drill pins the selected database object's S3 version, verifies checksum and size, refuses source/production-like names and existing databases, then compares that manifest entry's schema digest, key row counts, and Casbin policy presence. Use `--source-tenant-id` to select a tenant database. `--source-db` remains available only when the name is unique; otherwise the platform database is restored. Optional `RESTORE_READY_URL` adds `/health/ready` validation. Failed isolated databases remain for diagnosis; operators remove them explicitly after evidence capture. Standard Kubernetes NetworkPolicy cannot safely express arbitrary object-storage or IdP FQDNs, so production clusters must supply reviewed CIDRs or CNI-specific FQDN policy before enabling default-deny egress.
