# PostgreSQL Schema Notes

This project keeps MineAdmin V3 table names so the API and seed data can stay close to the upstream PHP implementation.

## Deviations

- `user` remains the table name for compatibility. Go models use explicit `TableName()` methods and SQL should prefer the query builder or quoted identifiers.
- JSON columns use PostgreSQL `jsonb` for `menu.meta`, `user.backend_setting`, and `data_permission_policy.value`.
- IP columns use `varchar(45)` instead of PostgreSQL `inet` to preserve MySQL-style scanning behavior and IPv6 length.
- `attachment.hash` uses a partial unique index scoped by `(hash, storage_mode, storage_config_id, storage_path) WHERE hash IS NOT NULL`. This keeps nullable duplicate rows valid while allowing the same file hash in different storage backends and isolated upload paths.
- `attachment.storage_config_id` snapshots the upload-time storage configuration so remote deletes keep using the original backend after the default changes.
- `storage_config` uses a partial unique index on enabled default rows so only one active default storage configuration can exist.
- Referenced `storage_config` rows cannot change backend connection fields or be deleted, so existing attachment objects remain deletable from their original backend.
- `user.dashboard` is included for `MineAdmin-web/src/modules/base/api/user.ts` compatibility, although the referenced MineAdmin migration does not define it.
- IDs use `bigserial/bigint` and are serialized as JSON numbers for the first compatibility pass.

## Timestamp Policy

Database columns use `timestamp without time zone`. API responses should format times as `YYYY-MM-DD HH:mm:ss` through the response time adapter rather than Go's RFC3339 default.
