# 企业安全控制清单

本文定义组织级安全控制目标，作为 `SECURITY_ENTERPRISE=true` 之外的落地约束。

## 敏感操作二次认证

敏感操作包括：

- MFA 关闭
- 重置他人密码
- 权限变更
- 租户套餐变更
- 租户挂起、恢复、归档、销毁
- SSO provider 密钥或端点变更
- 储存配置密钥变更

推荐 contract：

- 前端提交敏感操作前，先调用 re-auth challenge。
- 后端校验当前用户密码、MFA code 或恢复码。
- re-auth token 有效期不超过 5 分钟，只绑定当前用户、租户、操作名、目标资源。
- 审计日志记录 challenge、confirm、operation 三段结果。
- 后端以 `EnterpriseSecurityControlService.RequireSensitiveOperation` 校验 token 与 user/tenant/operation/resource 绑定。

## 权限变更审批

权限变更包括角色授权、用户角色变更、租户权限包变更。

审批字段：

- `requester_id`
- `approver_id`
- `scope`
- `resource`
- `before`
- `after`
- `reason`
- `used_at`
- `expires_at`
- `status`

执行规则：

- 生产环境禁止 requester 自批。
- 高危权限必须至少 1 名平台管理员审批。
- 审批必须绑定具体 operation/resource，且通过真实写入后立即标记 `used_at`，禁止复用。
- 审计日志记录审批单号。
- 后端以 `EnterpriseSecurityControlService.RequireRegisteredPermissionApproval` 拒绝缺审批、过期审批、自批、资源不匹配和已使用审批。

## 审计不可篡改

当前数据库审计表用于在线查询；合规归档应追加 WORM 存储。

要求：

- 应用审计日志以 JSON 输出到 stdout。
- Loki/对象存储归档保留原始日志。
- WORM bucket 开启 Object Lock `COMPLIANCE`；当前执行器仅接受平台默认 S3-compatible storage 中的 `s3://<bucket>/<key>`。
- 删除在线审计表前，先确认归档时间窗覆盖。
- `security:audit-prune` 默认为 dry-run；生成并持久化 plan、精确 target 清单、时间窗与 target digest，不删除在线记录。
- 归档后以 `security:audit-prune --execute --plan-id=<id> --proof-file=<signed-json> --evidence-stdin` 执行；二次认证 token 与审批号仅从 stdin JSON 读取，不得置于 argv。
- 客户端 proof 仅作定位；执行器用服务端 storage credential 对固定 object version 执行 HEAD/GET，校验 version、`COMPLIANCE` retain-until、manifest SHA-256 与服务端验证新鲜度。
- manifest 必须包含 plan ID、归档时间窗及每条 target 的完整规范化审计正文与 `record_digest`；执行器逐条重算正文摘要，并与持久化 target digest 精确比对。
- target digest 同时绑定记录身份、时间戳与 `record_digest`；删除前在目标数据库事务中锁定当前行并重算正文摘要，内容漂移即拒绝删除。
- 敏感策略 `audit.prune.execute` 绑定 `audit-prune:<plan-id>:<target-digest>`；审批申请人取自服务端审批账本，并与一次性 re-auth token 互证。
- 删除仅针对 plan 持久化的精确 target，逐项记录 `completed`、`no_op`、`partially_executed` 或 `failed`；部分执行不得续跑原 plan。
- 租户 target 绑定 tenant ID、code、动态 connection name 与数据库指纹；独立 execute 进程会从平台库重载并核验租户连接，漂移或碰撞均 fail closed。

## 密钥轮换

密钥轮换记录至少包含：

- 密钥用途
- 旧版本 ID
- 新版本 ID
- 生效时间
- 回滚策略
- 验证命令
- 负责人

运行：

```bash
go run . artisan security:rotate-check
```

## CSP nonce/hash

前端生产部署推荐：

- 禁止 `unsafe-inline`。
- 内联脚本改为 nonce 或 hash。
- 第三方资源域名白名单最小化。
- 报告端点接入安全事件队列。
- 后端以 `EnterpriseSecurityControlService.ValidateCSP` 拒绝 `unsafe-inline`，并要求 `nonce-*` 或 `sha256/384/512-*`。

## 离职与禁用流程

账号禁用必须联动：

- JWT refresh token 失效
- SSO binding 禁用或解除
- MFA recovery code 作废
- 当前 session 清理
- 高危权限审批单转交
- 审计日志归档
