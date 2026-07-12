# OIDC SSO 功能说明

本文档记录租户单点登录的前后端闭环。当前实现以 OIDC 为主，同时兼容 OAuth2 userinfo 登录和 SAML 断言登录。

## 功能范围

- 租户级 SSO Provider 管理：`/admin/sso-provider/*`
- 登录页按租户公开已启用 Provider：`GET /admin/passport/branding?scene=admin`
- OIDC 授权码登录：前端跳转授权端点，后端用 code 换 token 并校验 ID Token
- OIDC 直接 ID Token 登录：用于调试或外部前端自行完成授权流程
- OAuth2 授权码登录：后端用 access token 调 userinfo 获取用户身份
- SAML 登录：后端校验签名断言或响应
- 自动创建外部用户
- SSO 用户绑定：`/admin/sso-user-binding/*`
- SSO 登录日志与统计：`/admin/sso-login-log/*`
- 角色映射与数据权限映射
- 多 scene 隔离，同名 Provider 在不同 scene 下必须显式传 scene 登录

## 管理端配置

入口菜单：

- `安全管理 -> 单点登录 -> 身份源配置`
- `安全管理 -> 单点登录 -> 用户绑定`
- `安全管理 -> 单点登录 -> 登录审计`

表单分为：

- 基础信息：配置标识、展示名称、scene、协议类型、启用状态、自动创建用户、展示顺序、图标、按钮颜色、备注
- 身份校验：issuer、audience、client、JWT/JWKS、发现文档
- OAuth / OIDC：授权端点、令牌端点、用户信息端点、scope、redirect_uri、PKCE、nonce
- SAML：登录入口、实体 ID、证书
- 角色映射
- 数据权限映射

已移除旧字段：

- `description`
- `default_role_code`

默认角色统一配置在 `role_mapping.default`。

## Provider 字段

核心字段：

```json
{
  "name": "okta-admin",
  "display_name": "Okta Admin",
  "scene": "admin",
  "type": "oidc",
  "enabled": true,
  "issuer": "https://idp.example.com",
  "audience": "mineadmin",
  "discovery_url": "https://idp.example.com/.well-known/openid-configuration",
  "authorization_endpoint": "https://idp.example.com/oauth2/v1/authorize",
  "token_endpoint": "https://idp.example.com/oauth2/v1/token",
  "userinfo_endpoint": "https://idp.example.com/oauth2/v1/userinfo",
  "jwks_uri": "https://idp.example.com/oauth2/v1/keys",
  "jwks_json": "{\"keys\":[]}",
  "client_id": "client-id",
  "client_secret": "client-secret",
  "scope": "openid profile email",
  "redirect_uri": "https://console.example.com/login",
  "enable_pkce": true,
  "enable_nonce": true,
  "auto_create": true,
  "icon": "logos:okta",
  "button_color": "#2563eb"
}
```

`discovery_url` 可补全 `issuer`、`authorization_endpoint`、`token_endpoint`、`userinfo_endpoint`、`jwks_uri`。本地显式配置优先于发现文档。

## 登录流程

### 前端授权码流程

1. 登录页加载 `/admin/passport/branding?scene=admin`。
2. 前端显示启用的 SSO Provider。
3. 登录页调用 `POST /admin/passport/sso/authorize`，仅提交 Provider 和 scene。
4. 服务端生成不透明 `transaction_id`、`state`、OIDC `nonce`、PKCE `code_verifier`，并将其与租户、Provider、受配置约束的 `redirect_uri` 一同缓存，最长五分钟。
5. 服务端返回包含 `state`、`nonce` 和 `code_challenge=S256(verifier)` 的 IdP 授权 URL；响应不会包含 verifier、原始 code 或 token。
6. IdP 回跳登录页并携带 `code`、`state`。前端比对返回的 `state` 后，调用 `POST /admin/passport/sso/callback`，提交 `transaction_id`、`code`、`state`。
7. 服务端在锁内验证 transaction 的租户、state、过期时间、Provider 与 redirect URI，用保存的 verifier 交换 code，并用保存的 nonce 校验 ID Token。验证成功即一次性消费 transaction，再继续生成 MineAdmin 登录结果；验证失败不签发应用 token，transaction 等待过期清理。

### 服务端授权事务接口

`POST /admin/passport/sso/authorize`

```json
{
  "provider": "okta-admin",
  "scene": "admin"
}
```

成功响应：

```json
{
  "code": 200,
  "message": "成功",
  "data": {
    "transaction_id": "opaque-server-issued-id",
    "state": "server-issued-state",
    "authorization_url": "https://idp.example.com/oauth2/v1/authorize?..."
  }
}
```

`POST /admin/passport/sso/callback`

```json
{
  "transaction_id": "opaque-server-issued-id",
  "code": "authorization-code",
  "state": "server-issued-state"
}
```

回调成功响应与密码登录相同。请求体不接受 `code_verifier`、`nonce`、`redirect_uri`、Provider 或 scene 作为授权事实。

旧的 `POST /admin/passport/sso/login` 仅保留直接 ID Token 和 SAML 调试入口；任何 authorization code 都必须提交到 server-owned callback 接口。

失败码：

- `422` / `SSO 授权事务无效`：transaction 不存在、租户不匹配、state 不匹配、Provider 已变更，或 redirect URI 不匹配。
- `422` / `SSO 授权事务已过期`：超过五分钟有效期。
- `422` / `SSO 授权事务已使用`：并发或已成功完成的 callback 再次提交。
- `422` / `SSO Token 无效`：code 为空、缺少 PKCE verifier、token exchange、nonce 或 ID Token 校验失败。

### 登录请求

```json
{
  "transaction_id": "opaque-server-issued-id",
  "code": "authorization-code",
  "state": "server-issued-state"
}
```

直接 ID Token 调试：

```json
{
  "provider": "okta-admin",
  "scene": "admin",
  "id_token": "jwt"
}
```

成功响应：

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

## OIDC 校验规则

- 支持 `HS256`，使用 `jwt_secret`
- 支持 `RS256`、`RS384`、`RS512`，使用 `jwks_json` 或 `jwks_uri`
- 校验 `exp`
- 配置 `issuer` 时校验 `iss`
- 配置 `audience` 时校验 `aud`
- OIDC 授权码链始终使用服务端 transaction 生成的 nonce 校验 ID Token 的 `nonce`
- OIDC 授权码链始终使用服务端 transaction 保存的 PKCE verifier；Provider 配置的 `redirect_uri` 是唯一可发送到 token endpoint 的 redirect URI
- `sub` 必须存在，用作外部用户 username
- `email`、`name` 用于自动创建用户资料

## 用户绑定

SSO 登录成功后会写入 `sso_user_binding`：

- 唯一键：`provider_id + sso_user_id`
- 保存本地 `user_id`、外部用户 ID、邮箱、用户名、头像、首次登录时间、最近登录时间、登录次数
- 后续登录会优先按绑定找到本地用户；找不到绑定时再按 `claims.sub` 查本地用户名；仍找不到且 Provider 开启 `auto_create` 时自动创建用户
- 强制解绑只删除绑定关系，不删除本地用户，也不删除历史登录日志

管理接口：

- `GET /admin/sso-user-binding/list`
- `GET /admin/sso-user-binding/{id}`
- `GET /admin/sso-user-binding/user/{userId}`
- `DELETE /admin/sso-user-binding/{id}`

列表支持查询参数：`page`、`page_size`、`user_id`、`username`、`provider_id`、`provider_name`、`sso_user_id`、`sso_email`、`sso_username`。

## 登录日志

SSO 登录成功和 Provider 已识别后的失败都会写入 `sso_login_log`：

- 成功：记录本地用户、Provider、绑定、外部用户 ID、邮箱、IP、User-Agent、设备类型、登录时间
- 失败：记录 Provider、可识别的外部用户信息、失败原因、IP、User-Agent、设备类型、登录时间
- 失败原因会按业务错误转换为前端可读文案，例如 `SSO Token 无效`

管理接口：

- `GET /admin/sso-login-log/list`
- `GET /admin/sso-login-log/stats`

列表和统计支持查询参数：`page`、`page_size`、`user_id`、`username`、`provider_id`、`provider_name`、`sso_user_id`、`sso_email`、`status`、`start_date`、`end_date`。

## 角色映射

角色映射配置结构：

```json
{
  "claim": "groups",
  "default": ["SuperAdmin"],
  "mapping": {
    "admins": ["SuperAdmin"],
    "managers": {
      "condition": "{{level}} >= 5 && (department == 'sales' || department == 'growth')",
      "roles": ["Manager"]
    }
  }
}
```

规则：

- `claim` 指定从 IdP claims 中读取的字段
- claim 值可为字符串或数组
- `mapping` 命中后同步系统角色编码
- 没有命中任何角色时使用 `default`
- 登录后会同步 `user_belongs_role` 和 Casbin `g` 规则

## 数据权限映射

数据权限映射配置结构：

```json
{
  "claim": "department",
  "default": "SELF",
  "mapping": {
    "sales": {
      "condition": "level >= 5 || email contains '@example.com'",
      "policy_type": "CUSTOM_DEPT",
      "value": [1, 2]
    },
    "it": "DEPT_TREE"
  }
}
```

支持策略：

- `ALL`
- `DEPT_TREE`
- `DEPT_SELF`
- `SELF`
- `CUSTOM_DEPT`
- `CUSTOM_FUNC` 当前会拒绝，提示自定义数据权限函数未注册

登录后会写入用户级 `data_permission_policy`。没有部门值时 `value` 写入 `[]`。

## 条件表达式

角色映射和数据权限映射都支持条件表达式：

- 比较：`==`、`!=`、`>`、`>=`、`<`、`<=`
- 逻辑：`&&`、`||`、`!`、括号
- 包含：`in`、`not_in`、`contains`
- 字符串：`starts_with`、`ends_with`
- 正则：`matches`、`not_matches`
- 变量写法：`{{claim_name}}`

示例：

```text
level >= 5 && (department == 'IT' || department == 'HR')
{{role}} in ['admin', 'manager']
email matches '.*@example\.com'
```

## 排查要点

- 登录页没有按钮：确认 Provider 已启用、scene 为 `admin`，并且当前是租户登录模式。
- 点击后没有跳转：确认有 `authorization_endpoint` 和 `client_id`；若只填 `discovery_url`，确认发现文档可访问。
- code 登录失败：确认 `token_endpoint`、`client_id`、`client_secret`、`redirect_uri` 与 IdP 配置一致。
- nonce 失败：确认前端发起授权和回调登录来自同一个浏览器会话。
- RS256 校验失败：确认 `kid` 能在 `jwks_json` 或 `jwks_uri` 中匹配。
- 自动创建失败：确认 Provider 开启 `auto_create`，且 token/userinfo 中有 `sub`。
- 已解绑后无法登录：确认 Provider 开启 `auto_create`，或重新建立 `provider_id + sso_user_id` 绑定。
- 权限未生效：确认映射中的角色编码真实存在，数据权限策略值符合系统策略。

## 验证覆盖

当前测试覆盖：

- Provider 增删改查、scene 隔离、公开 branding
- OIDC HS256 ID Token
- OIDC RS256 JWKS
- OIDC discovery document
- OAuth2 authorization code + userinfo
- SAML 签名断言
- 自动创建用户
- SSO 用户绑定创建、复用、解绑
- SSO 登录成功/失败日志与统计
- 默认角色映射
- 角色映射与数据权限映射
- 条件表达式映射
