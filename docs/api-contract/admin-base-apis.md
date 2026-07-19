# MineAdmin Admin API Contract

本文档记录 Goravel 后端当前实现的 MineAdmin V3 兼容基础 API。所有接口默认返回 HTTP 200，业务状态由 JSON `code` 表示。

范围：本文件为 `admin-base-apis` scoped subset（基础接口子集），覆盖本文列出并承诺生成 SDK 的管理端基础接口；不作为 `routes/web.go` 全量路由清单。平台租户、SSO、计划任务、观测等扩展接口需另建契约或补入本文后再生成对应 SDK。

机器可验证契约：

- OpenAPI 3.1: `docs/api-contract/openapi/admin-base-apis.openapi.json`
- 契约测试：`tests/backend/test.sh ./tests/backend/unit -run AdminOpenAPI`
- TypeScript SDK 生成：`cd MineAdmin-web && yarn gen:openapi`
- TypeScript SDK 漂移校验：`cd MineAdmin-web && yarn contract:openapi`

CI 会校验本文件、OpenAPI、`routes/web.go` 中已实现路由，以及 `MineAdmin-web/src/generated/admin-api.ts` 是否由 OpenAPI 重新生成且类型可编译。若新增接口要纳入 SDK，需同步更新本文件、OpenAPI 与生成文件。

TypeScript SDK 生成示例（仅此 scoped subset）：

```bash
npx @openapitools/openapi-generator-cli generate \
  -i docs/api-contract/openapi/admin-base-apis.openapi.json \
  -g typescript-axios \
  -o tmp/sdk/admin-api
```

## Common Response

```json
{
  "code": 200,
  "message": "成功",
  "data": {}
}
```

- 成功码：`200`
- 常见错误码：`401` 未认证，`403` 禁止访问，`422` 参数或业务校验失败，`423` 用户停用，`429` 请求或登录风控限制，`500` 服务端错误
- 空列表：`[]`
- 分页：`{ "list": [], "total": 0 }`
- 时间格式：`YYYY-MM-DD HH:mm:ss`
- 认证头：`Authorization: Bearer <token>`

## Passport

### `POST /admin/passport/sso/authorize`

创建服务端持有的 OIDC state、nonce 与 PKCE transaction，返回 opaque `transaction_id` 和 authorization URL。

### `POST /admin/passport/sso/callback`

使用 `transaction_id`、authorization code 与 state 完成一次性回调；成功返回 JWT，事务或 code 不可重放。

### `POST /admin/passport/login`

```json
{
  "username": "admin",
  "password": "123456",
  "code": "server-captcha-answer",
  "captcha_key": "server-captcha-key"
}
```

`captcha_key` / `key` 必须来自服务端验证码接口，缺失或错误时登录返回 `422`。账号连续登录失败会按 `SECURITY_ACCOUNT_LOCKOUT_*` 配置临时锁定，锁定期内返回 `429`。
当 `SECURITY_PASSWORD_MAX_AGE_DAYS` 生效、密码已过期，且账号未启用 MFA 时，密码校验通过后返回改密挑战：

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "password_change_required": true,
    "password_change_token": "short-lived-token"
  }
}
```

当 `SECURITY_MFA_TOTP_ENABLED=true` 且用户已绑定 MFA 时，密码校验通过后先返回 MFA 挑战。若该账号同时密码过期，必须先完成 MFA，随后由 MFA 登录接口返回改密挑战，而不是直接签发 JWT：

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "mfa_required": true,
    "mfa_token": "short-lived-token"
  }
}
```

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "access_token": "jwt",
    "refresh_token": "jwt",
    "expire_at": 3600
  }
}
```

### `POST /admin/passport/mfa/login`

```json
{
  "mfa_token": "short-lived-token",
  "mfa_code": "123456"
}
```

MFA 验证码或恢复码校验通过后，未过期密码返回与普通登录一致的 JWT 结构；密码已过期时返回 `password_change_required` 与 `password_change_token`，前端继续调用改密接口。

### `POST /admin/passport/password/change`

密码过期挑战改密，不需要 Authorization，但必须使用登录返回的短期 `password_change_token`。

```json
{
  "password_change_token": "short-lived-token",
  "old_password": "123456",
  "new_password": "new-password-123",
  "new_password_confirmation": "new-password-123"
}
```

改密成功后返回普通 JWT。已绑定 MFA 且密码过期的账号，改密 token 必须来自 MFA 通过后的响应。

平台端同构接口：

- `POST /admin/platform/passport/login`
- `POST /admin/platform/passport/mfa/login`
- `POST /admin/platform/passport/password/change`
- `GET /admin/platform/passport/csrf-token`
- `GET /admin/platform/passport/getInfo`
- `POST /admin/platform/passport/logout`
- `POST /admin/platform/passport/refresh`

### `GET /admin/passport/csrf-token`

返回 CSRF token 并写入 `csrf_token` Cookie。`SECURITY_CSRF_ENABLED=true` 时，非 `GET/HEAD/OPTIONS` 请求需同时带可信 `Origin` / `Referer` 与 `X-CSRF-Token`。跨站前端部署可设置 `SECURITY_CSRF_SAME_SITE=none`，此时 Cookie 默认自动带 `Secure`，也可通过 `SECURITY_CSRF_COOKIE_SECURE` 显式控制。

```json
{
  "code": 200,
  "message": "成功",
  "data": { "csrf_token": "token" }
}
```

### MFA Management

租户端：

- `POST /admin/security/mfa/setup`
- `POST /admin/security/mfa/confirm`，请求：`{ "code": "123456" }`
- `POST /admin/security/mfa/disable`，请求：`{ "code": "123456", "reauth_token": "...", "approval_id": "..." }`

平台端：

- `POST /admin/platform/security/mfa/setup`
- `POST /admin/platform/security/mfa/confirm`
- `POST /admin/platform/security/mfa/disable`，请求同上

`setup` 返回 TOTP secret 与 `otpauth://` URI；`confirm` 成功后返回一次性恢复码。已启用 MFA 的账号不能直接 setup 覆盖密钥，需先用当前验证码、二次认证 token 与另一位操作者批准的审批记录关闭 MFA。

### Sensitive Operation Control

以下接口用于为受保护的敏感操作申请二次认证与审批证据。平台端与租户端字段、响应结构一致，但证据只可在各自的授权域内使用：

- 平台端：`POST /admin/platform/security/reauth-token`、`POST /admin/platform/security/approvals`、`GET /admin/platform/security/approvals/{approval_id}`、`PUT /admin/platform/security/approvals/{approval_id}/approve`
- 租户端：`POST /admin/security/reauth-token`、`POST /admin/security/approvals`、`GET /admin/security/approvals/{approval_id}`、`PUT /admin/security/approvals/{approval_id}/approve`

二次认证请求：

```json
{
  "password": "current-password",
  "mfa_code": "123456",
  "operation": "mfa.disable",
  "resource": "mfa:user:42:disable"
}
```

`password`、`operation`、`resource` 必填；当前账号已启用 MFA 时，`mfa_code` 也必须有效。成功响应的 `data` 为 `{ "reauth_token": "...", "expires_at": "2026-07-11T12:00:00Z" }`。token 绑定当前用户、操作和资源，并在敏感 mutation 中一次性消费。

创建审批请求：

```json
{
  "policy_key": "mfa.disable",
  "resource": "mfa:user:42:disable",
  "reason": "按流程关闭 MFA"
}
```

`resource`、`reason` 必填，`policy_key` 与 `scope` 至少提供一个。服务端基于策略生成规范化 scope、resource 与绑定摘要，不接受客户端作为授权事实的快照字段。创建、详情和批准均返回审批记录：`approval_id`、请求/批准人、租户、`policy_key`、`binding_digest`、`scope`、`resource`、`status`、`reason`、`used_at?`、`expires_at?`。只有非申请人可批准仍在有效期内的 `pending` 记录。

### `POST /admin/passport/logout`

需要 access token。成功后当前 access token 失效。

```json
{ "code": 200, "message": "成功", "data": [] }
```

### `POST /admin/passport/refresh`

需要 refresh token。

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "access_token": "jwt",
    "refresh_token": "jwt",
    "expire_at": 3600
  }
}
```

### `GET /admin/passport/getInfo`

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "id": 1,
    "username": "admin",
    "nickname": "管理员",
    "avatar": "",
    "signed": "",
    "phone": "",
    "email": "",
    "departments": [],
    "positions": [],
    "roles": []
  }
}
```

`backend_setting` 仅在用户保存过非空前端设置时返回，避免空对象覆盖 MineAdmin-web 默认 `app.whiteRoute` 等配置。

## Captcha

### `GET /admin/passport/captcha`

Alias: `GET /api/system/captcha`

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "key": "captcha-id",
    "base64": "data:image/png;base64,..."
  }
}
```

验证码答案缓存 2 分钟，登录验证后立即删除。

## Current User Permission

### `GET /admin/permission/menus`

返回当前用户可访问菜单树。`children` 始终是数组。

```json
{ "code": 200, "message": "成功", "data": [] }
```

### `GET /admin/permission/roles`

返回当前用户角色；SuperAdmin 返回所有启用角色。

```json
{ "code": 200, "message": "成功", "data": [] }
```

### `POST /admin/permission/update`

更新当前用户资料、前端设置或密码。

```json
{
  "nickname": "新昵称",
  "avatar": "/storage/uploads/a.png",
  "signed": "签名",
  "backend_setting": {},
  "old_password": "old",
  "new_password": "new",
  "new_password_confirmation": "new"
}
```

平台端同构接口：

- `GET /admin/platform/permission/menus`
- `GET /admin/platform/permission/roles`
- `POST /admin/platform/permission/update`

新密码需满足 `SECURITY_PASSWORD_*` 配置的密码策略。

### `PUT /admin/user/info`

兼容 MineAdmin-web 的当前用户资料更新别名，请求体同上。

## User

### `GET /admin/user/list`

Query: `page`, `per_page`, `username`, `nickname`, `phone`, `email`, `status`

```json
{ "code": 200, "message": "成功", "data": { "list": [], "total": 0 } }
```

### `POST /admin/user`

```json
{
  "username": "staff",
  "password": "staff-pass",
  "user_type": "100",
  "nickname": "员工一号",
  "phone": "13800000000",
  "email": "staff@example.com",
  "status": 1,
  "department": [1],
  "position": [1],
  "policy": [],
  "remark": "demo"
}
```

显式传入的密码需满足 `SECURITY_PASSWORD_*` 配置的密码策略；未传时仍使用系统默认初始密码。

### `PUT /admin/user/{id}`

同创建请求；未传密码时不修改密码。

### `DELETE /admin/user`

```json
[2, 3]
```

### `PUT /admin/user/password`

```json
{ "id": 2 }
```

默认重置为 `123456`。

### `GET /admin/user/{id}/roles`

返回用户已分配角色。

### `PUT /admin/user/{id}/roles`

```json
{ "role_codes": ["Staff"] }
```

## Role

### `GET /admin/role/list`

Query: `page`, `per_page`, `name`, `code`, `status`

```json
{ "code": 200, "message": "成功", "data": { "list": [], "total": 0 } }
```

### `POST /admin/role`

```json
{
  "name": "运营",
  "code": "Operator",
  "status": 1,
  "sort": 20,
  "remark": "后台运营"
}
```

### `PUT /admin/role/{id}`

同创建请求。

### `DELETE /admin/role`

```json
[2, 3]
```

### `GET /admin/role/{id}/permissions`

返回角色菜单权限列表。

### `PUT /admin/role/{id}/permissions`

```json
{
  "permissions": ["permission:user:index", "permission:role:index"]
}
```

同步 `role_belongs_menu` 与 Casbin policy。

## Menu

### `GET /admin/menu/list`

返回菜单树。

### `POST /admin/menu`

```json
{
  "parent_id": 0,
  "name": "demo:menu",
  "path": "/demo",
  "component": "demo/index",
  "redirect": "",
  "status": 1,
  "sort": 999,
  "meta": { "title": "演示菜单", "type": "M" },
  "btnPermission": [
    { "name": "demo:menu:create", "meta": { "title": "创建", "type": "B" }, "sort": 1 }
  ],
  "remark": "demo"
}
```

### `PUT /admin/menu/{id}`

同创建请求。

### `DELETE /admin/menu`

```json
[10]
```

## Organization

### Department

- `GET /admin/department/list`
- `POST /admin/department`
- `PUT /admin/department/{id}`
- `DELETE /admin/department`

```json
{
  "name": "研发部",
  "parent_id": 0,
  "department_users": [1],
  "leader": [1]
}
```

列表返回 `{ "list": [], "total": 0 }`，部门节点包含 `children`、`department_users`、`leader`。

### Position

- `GET /admin/position/list`
- `POST /admin/position`
- `PUT /admin/position/{id}`
- `DELETE /admin/position`
- `PUT /admin/position/{id}/data_permission`

```json
{ "name": "产品经理", "dept_id": 1 }
```

```json
{ "policy_type": "CUSTOM_DEPT", "value": [1] }
```

`policy_type` 可取 `ALL`、`DEPT_SELF`、`DEPT_TREE`、`SELF`、`CUSTOM_DEPT`、`CUSTOM_FUNC`；仅 `CUSTOM_DEPT` 需要 `value` 部门 ID 列表。`CUSTOM_FUNC` 当前未注册时返回业务错误。

### Leader

- `GET /admin/leader/list`
- `POST /admin/leader`
- `PUT /admin/leader/{id}`
- `DELETE /admin/leader`

```json
{ "dept_id": 1, "user_id": [1] }
```

删除：

```json
{ "dept_id": 1, "user_ids": [1] }
```

## Platform Storage Config

平台管理员维护全局储存配置。支持 `local` 与 `s3_compatible` 两类驱动；储存方式可选 MinIO、AWS S3、阿里云 OSS、腾讯云 COS、七牛云、华为 OBS。`local` 写入本地 public storage，`s3_compatible` 使用 path-style URL 与 SigV4 签名向 `{endpoint}/{bucket}/{storage_path}` 执行 PUT / DELETE。

### `GET /admin/platform/storage-config/list`

Query: `page`, `page_size` 或 `per_page`, `name`, `provider`, `driver`, `status`

```json
{ "code": 200, "message": "成功", "data": { "list": [], "total": 0 } }
```

### `POST /admin/platform/storage-config`

```json
{
  "name": "默认 MinIO",
  "provider": "minio",
  "driver": "s3_compatible",
  "bucket": "tenant-assets",
  "endpoint": "https://minio.example.test",
  "region": "cn-east-1",
  "access_key": "ak",
  "secret_key": "sk",
  "base_url": "https://cdn.example.test",
  "path_prefix": "uploads",
  "is_default": true,
  "status": 1,
  "options": { "force_path_style": true },
  "remark": "primary"
}
```

`provider` 可选：`local`, `minio`, `aws_s3`, `aliyun_oss`, `tencent_cos`, `qiniu`, `huawei_obs`。同一时间只有一个启用默认配置；创建或更新默认配置会自动取消其他默认配置。`driver=s3_compatible` 时 `bucket`, `endpoint`, `access_key`, `secret_key` 必填。

### `PUT /admin/platform/storage-config/{id}`

同创建请求。`secret_key` 留空时保留原密钥；若配置已被平台或租户附件引用，不能修改 `provider`, `driver`, `bucket`, `endpoint`, `region`, `access_key`, `secret_key`, `base_url`, `path_prefix` 等后端连接信息。

### `DELETE /admin/platform/storage-config`

```json
[1, 2]
```

已被附件引用的储存配置不能删除。

## Tenant Export

### `POST /admin/platform/tenant/{id}/exports`

创建经 re-auth 与双人审批保护的异步租户数据导出 run。

### `GET /admin/platform/tenant/{id}/exports/{run_id}`

查询 run 状态；完成后返回与 operator、tenant、run 绑定的短期一次性下载 token。

### `GET /admin/platform/tenant/{id}/exports/{run_id}/download`

使用一次性 token 下载 `application/x-ndjson` 或 `text/csv` 二进制响应。

## Attachment

### `POST /admin/attachment/upload`

Multipart form field: `file`

平台端同构接口：

- `POST /admin/platform/attachment/upload`

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "id": 1,
    "storage_config_id": 0,
    "origin_name": "hello.txt",
    "object_name": "hash.txt",
    "hash": "...",
    "mime_type": "text/plain; charset=utf-8",
    "storage_mode": "local",
    "storage_path": "uploads/tenants/default/...",
    "suffix": "txt",
    "size_byte": 16,
    "url": "/storage/uploads/tenants/default/..."
  }
}
```

租户上传路径为 `{path_prefix}/tenants/{tenant_code}/YYYY/MM/DD/{hash}.{suffix}`；平台级上传路径为 `{path_prefix}/platform/YYYY/MM/DD/{hash}.{suffix}`。相同文件 hash 会在当前租户库与路径范围内复用已有记录。启用 `s3_compatible` 时 `url` 使用 `base_url`，未配置则回落为 endpoint/bucket/object 远端地址。

### `GET /admin/attachment/list`

Query: `page`, `page_size` 或 `per_page`, `suffix`, `mime_type`, `origin_name`

### `DELETE /admin/attachment/{id}`

软删除附件记录。

### `GET /storage/...`

静态访问本地 public storage 文件；对象存储文件按返回的远端 `url` 访问。

## Logs

### `GET /admin/user-login-log/list`

Query: `username`, `status`, `ip`, `os`, `browser`, `page`, `per_page`

### `GET /admin/user-operation-log/list`

Query: `username`, `method`, `router`, `service_name`, `page`, `per_page`

登录日志与操作日志为只读审计记录；留存期清理仅可通过受 Task 5 证据流保护的 `security:audit-prune` CLI/job 执行，管理 API 不提供删除操作。

## Tooling

### `go run . artisan security:rotate-check`

检查 `APP_KEY`、`JWT_SECRET` 等环境密钥的 `*_ROTATED_AT` 元数据，并扫描平台库/租户库内 SSO 与对象存储密钥，按 `SECURITY_KEY_ROTATION_DAYS` 给出 `ok` / `warning` / `expired` / `unknown`，不会修改 `.env`。

### `go run . artisan security:audit-prune [--dry-run]`

默认仅生成并持久化清理 plan，不删除数据。可用 `--archive-output=<manifest.json>` 同时写出包含完整规范化审计正文及 digest 的 upload-ready WORM manifest；上传其固定对象版本后据此生成 proof。可用 `--scope=all|platform|tenant:<code>` 与 `--retention-days=N`；租户若配置治理留存期，则优先使用租户策略。覆盖表：`user_login_log`、`user_operation_log`、`sso_login_log`、`tenant_permission_audit`。

```bash
go run . artisan security:audit-prune --dry-run \
  --scope=all --archive-output=artifacts/audit-prune/manifest.json
```

真实执行须显式提供：

```bash
printf '%s' '{"reauth_token":"...","approval_id":"..."}' | \
  go run . artisan security:audit-prune --execute \
    --plan-id=<plan-id> --proof-file=<signed-proof.json> --evidence-stdin
```

proof 必须绑定 plan ID、精确 target digest、归档时间窗、不可变对象 URI/version 与 manifest SHA-256。执行器不信任 proof 自报的锁状态：它会以服务端凭据 HEAD/GET S3 固定版本，要求 Object Lock `COMPLIANCE`，并从含完整 targets 的 manifest 重算 digest。缺任一项、锁/版本/数据库指纹漂移、证据过期、摘要不符或原 plan 已部分执行，命令均拒绝删除。

### `go run . artisan tenant:migrate --dry-run`

为既有租户库补跑租户业务迁移。新增安全表（如 `user_mfa`、`user_password_history`）上线后，先用 `--dry-run` 查看影响租户数，再去掉 `--dry-run` 执行。

### `go run . artisan make:crud <table> --module=<name> --force=false`

从 PostgreSQL 表结构生成：

- `app/models/<table>.go`
- `app/repositories/<module>/<table>_repository.go`
- `app/http/request/<module>/<table>_request.go`
- `app/http/controllers/admin/<module>/<table>_controller.go`
- `routes/<module>_<table>_routes.go`

默认不覆盖已有文件；需要覆盖时加 `--force`。测试或临时生成可使用 `--path=<dir>`。
