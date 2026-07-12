# 队列治理指南

本文补齐生产队列治理约束。当前项目已有 Redis queue、failed jobs、outbox、idempotency 与 worker 分离部署，本指南定义二开模块如何安全接入。

## 幂等模板

任何会产生外部副作用的任务必须提供 idempotency key：

- 邮件：`mail:<template>:<user_id>:<business_id>`
- Webhook：`webhook:<provider>:<event_id>`
- 文件处理：`file:<attachment_id>:<operation>`
- 租户任务：`tenant:<tenant_id>:<task>:<business_id>`

执行顺序：

1. 计算 idempotency key。
2. 调用 idempotency acquire。
3. 执行业务副作用。
4. 标记 success 或 failure。
5. 重试时先查 idempotency 状态。

## DLQ 分级

`failed_jobs` 是当前死信来源。处理前先按 signature、queue、exception 聚合。

- P0：数据破坏、重复扣费、权限错发。暂停入口，禁止批量重试。
- P1：关键业务任务失败。修复根因后按 UUID 重试。
- P2：通知、同步、统计类任务失败。可批量重试，需观察下游限流。
- P3：无业务价值或过期任务。记录原因后丢弃。

## 优先级队列

推荐队列名：

- `critical`：认证、安全、权限、租户生命周期。
- `default`：常规业务。
- `bulk`：导入导出、报表、批处理。
- `low`：通知、缓存预热、非关键同步。

生产 worker 可按队列独立扩容。禁止把 bulk 任务投到 `critical`。

## 延迟队列

延迟任务需写明：

- 最大延迟时间
- 最大重试次数
- retry backoff
- 是否允许乱序
- 到期后业务状态是否仍有效

过期任务应在 handler 内重新校验业务状态，而不是默认执行。

## Backpressure

触发信号：

- `goravel_queue_failed_jobs > 0`
- `goravel_queue_outbox_events{status="pending"} > 100`
- worker unavailable replicas > 0
- Redis rejected connections 增长
- 下游 API 429/5xx 增长

动作：

- 降低 producer 速率。
- 暂停 bulk queue。
- 提高 `QUEUE_OUTBOX_BATCH` 前先确认下游容量。
- 增加 worker replicas 前确认任务幂等。

## Autoscaling 指标

应用 `/metrics` 按 `queue_class=critical|default|bulk` 导出以下指标：

- `goravel_queue_pending_jobs`：当前 pending 数。
- `goravel_queue_oldest_backlog_age_seconds`：最老 pending 任务年龄。
- `goravel_queue_arrival_rate`：固定五分钟窗口内的到达速率（jobs/s）。
- `goravel_queue_completion_rate`：固定五分钟窗口内的完成速率（jobs/s）。

`worker.autoscaling.enabled=true` 时：

- `worker.autoscaling.keda.enabled=true` 渲染 KEDA `ScaledObject`，同时以 pending 数与最老 backlog 年龄查询扩缩容。
- 否则渲染 `autoscaling/v2` HPA，并通过 Prometheus Adapter 提供的 `goravel_queue_pending_jobs` 与 `goravel_queue_oldest_backlog_age_seconds` external metrics 扩缩容。
- `minReplicas`、`maxReplicas` 与 `cooldownPeriodSeconds` 必须受 DB/Redis 容量预算约束；当前 Chart 默认最高 4 个 worker。Task 26 的连接预算生效后，应把该上限同步收紧到可证明安全的值。
- 禁用 autoscaling 时保留 `worker.replicaCount` 的显式固定副本，作为回滚路径。

队列告警：

- pending backlog 在十分钟内持续正增长；
- `critical` 最老年龄超过 60 秒；
- `default` 最老年龄超过 5 分钟；
- `goravel_queue_failed_jobs > 0`。

执行 `bash scripts/queue-autoscale-smoke.sh` 前，必须确认测试命名空间、worker deployment、Prometheus 和应用 API 均已就绪。设置 `QUEUE_SMOKE_ENQUEUE_COMMAND` 为已部署测试辅助命令；脚本以 `QUEUE_SMOKE_CLASS` 和 `QUEUE_SMOKE_JOB_COUNT` 环境变量调用该命令，要求它只向指定类别写入确定性的 no-op outbox/job。脚本不改变 autoscaler 配置，记录扩容、drain、缩容时间线。

## 重放审计

每次重试或丢弃记录：

- operator
- uuid / topic / signature
- 原始异常摘要
- 处理动作
- 影响租户
- 结果
- 时间

后台 UI 或 runbook 命令都应保留该记录。
