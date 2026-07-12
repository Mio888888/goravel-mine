# 租户治理处置手册

## 告警分诊

1. 以 `tenant_id`、`kind`、`run_id` 查询 `tenant_governance_run`，确认状态、策略版本、幂等键、`plan_id` 和错误；retention 的 `awaiting_evidence` 须由 `security:audit-prune --execute` 提交 WORM proof 与一次性审批后收口。
2. 对照 `tenant_governance_evidence` 核验 URI、object version、SHA-256、验证时间、过期时间和 stale 状态。
3. 在 Loki 以 `event="tenant_governance"`、`outcome="failure"` 关联 `request_id` 与 `trace_id`。

## 过期证据

- 隔离证明过期时，先确认租户 DB、schema、role 未变，再触发隔离验证。
- 验证必须同时证明跨租户 sentinel 与平台 sentinel 不可见。
- 不得手改 `expires_at` 或复制旧证据；新 run 须写入新的不可变对象版本。

## 失败验证

- 身份不符：暂停治理结论，核对租户 connection 配置与数据库 role。
- sentinel 可见：按隔离事故处理，暂停租户流量并保全日志，不得重试掩盖结果。
- 租户数据库位于平台 PostgreSQL 实例之外时，隔离验证 fail closed；须由该实例管理员收紧 database ACL，并接入可信跨实例 network isolation attestor，方可生成通过证据。schema migration 仅收紧平台同实例 ACL，不使用租户业务账号修改远端 ACL，也不阻断后续 schema 发布。
- 对象写入失败：检查 WORM bucket、Object Lock、versioning 与服务凭证；无 immutable URI/version/digest 不得完成 run。

## 卡住任务

- 超过 30 分钟的 pending/running run，先核 worker、scheduler 与 DB 连接。
- artifact 已写而 run 未完成时，核对 digest 后按状态机恢复；不得重复写同一 run 证据。
- 仅确认无活跃 owner 后标 stale；新执行使用新幂等键，旧 run 不续跑。

## 恢复验收

- 三项 Prometheus 指标回落至预期。
- 最新 completed run 存在未过期、未 stale 的 immutable evidence。
- Loki 无持续 `tenant_governance` failure，关联审计链完整。
## 计划任务连接迁移

当 `TENANT_PLATFORM_CONNECTION` 与 `DB_CONNECTION` 不同时，升级前须将既有 `scheduled_task` 与 `scheduled_task_log` 数据迁入平台数据库。任一旧表仍有数据时迁移将 fail closed，避免静默丢失计划任务或执行历史。
