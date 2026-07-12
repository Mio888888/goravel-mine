# On-call Runbook

## 通用入口

1. 打开 Grafana `Goravel Mine Observability`。
2. 确认最近 30 分钟是否有发布、迁移、配置变更。
3. 查询最近错误日志，按 `request_id` 与 `trace_id` 串联请求。
4. 查询慢样本：

```bash
curl -H "Authorization: Bearer <admin-token>" \
  "https://admin.example.com/admin/platform/observability/slow-requests?limit=50"
```

5. 确认 `/health/ready` 与 `/metrics`：

```bash
curl -fsS https://admin.example.com/health/ready
curl -fsS -H "Authorization: Bearer <metrics-token>" https://admin.example.com/metrics
```

## 5xx Error Rate

触发告警：`GoravelMineHighErrorRate`

判断：

- Grafana 查看 `5xx error ratio` 与按 route 的 request rate。
- Loki 按 `level="error"`、`status>=500`、`request_id`、`trace_id` 查询。
- Kubernetes 查看新 pod 是否刚上线、是否重启。

常用命令：

```bash
kubectl -n goravel-mine get pods
kubectl -n goravel-mine logs deploy/goravel-mine --since=30m
kubectl -n goravel-mine rollout history deployment/goravel-mine
```

止血：

- 最近发布相关：执行 `kubectl -n goravel-mine rollout undo deployment/goravel-mine`。
- 单一路由高错误：临时下线入口、禁用相关菜单/任务，保留证据。
- DB/cache 依赖异常：恢复依赖、扩容、切换只读维护页。

复盘记录：

- 影响窗口、请求量、5xx 数量、受影响 route。
- 根因、触发条件、止血动作、永久修复。

## Slow Requests

触发告警：`GoravelMineSlowRequestsSpike`

判断：

- 调 `GET /admin/platform/observability/slow-requests?limit=50`。
- 按 `route` 聚合，查共同 `trace_id`、`request_id`、`ip`。
- 对比同时段 slow SQL、DB CPU、连接数、锁等待。

止血：

- 若集中于后台任务或批量 API：暂停任务或降低并发。
- 若集中于上传/导出：临时限制入口或加大超时时间。
- 若集中于 DB 查询：加索引、改分页、加缓存，或回滚触发版本。

## High P95/P99 Latency

触发告警：`GoravelMineHighP95Latency`、`GoravelMineHighP99Latency`

判断：

- Grafana 查看 `HTTP latency p95/p99`，定位最高 route。
- 对同 route 查看 request rate 与 5xx ratio，区分流量突增、错误重试、单纯变慢。
- 同时查看 DB pool waits、slow SQL、Go heap/goroutines。

止血：

- 若 DB pool wait 增长，先降低高并发入口或扩 DB 连接容量。
- 若 slow SQL 同时增长，优先修查询或回滚触发版本。
- 若 goroutines/heap 持续增长，排查阻塞 IO、泄漏任务、批处理。

## DB Pool Saturation

触发告警：`GoravelMineDBPoolWait`

判断：

- 查看 `DB pool connections` 与 `DB pool waits`。
- 若 `in_use` 接近 `max_open` 且 wait count 增长，说明应用侧连接池饱和。
- 对比 PostgreSQL exporter 的 max connections、active connections、lock waits。

止血：

- 暂停批量任务、导出、低优先级后台作业。
- 扩大 DB max connections 前先确认 PostgreSQL 容量。
- 修复长事务、慢 SQL、未关闭 rows/transaction 的代码路径。

## Slow SQL

触发告警：`GoravelMineSlowSQLSpike`

判断：

- 从 slow SQL 样本取 SQL、`request_id`、`trace_id`。
- 在 PostgreSQL 执行 `EXPLAIN (ANALYZE, BUFFERS)`，确认 seq scan、锁等待、排序溢出。
- 检查最近迁移、索引缺失、表膨胀、连接池耗尽。

止血：

- 回滚最近查询变更。
- 对高频慢查询加临时索引。
- 暂停导致慢查询的定时任务。
- 扩容 DB 或调低应用并发。

## Metrics Scrape Down

触发告警：`GoravelMineNoMetricsScrape`

判断：

- `OBS_METRICS_ENABLED` 是否为 `true`。
- Prometheus bearer token 是否等于 `OBS_METRICS_TOKEN`。
- Service/ServiceMonitor endpoint 是否选中 pod。

常用命令：

```bash
kubectl -n goravel-mine get svc,endpoints,pods
kubectl -n goravel-mine exec deploy/goravel-mine -- printenv | grep OBS_METRICS
kubectl -n goravel-mine port-forward svc/goravel-mine 3000:80
curl -H "Authorization: Bearer <metrics-token>" http://127.0.0.1:3000/metrics
```

止血：

- 修正 Secret token 后滚动重启 Prometheus 或应用。
- 修正 ServiceMonitor selector。
- 若监控栈异常，先切到 Prometheus 静态 scrape。

## Scheduler Node Down

触发告警：`GoravelMineSchedulerNodeDown`

判断：

- 查看 scheduler runner 日志。
- 检查 DB 可用性与 heartbeat 表写入。
- 查 pod 重启与 OOM。

止血：

- 重启异常 pod。
- 修复 DB/cache 依赖。
- 暂停依赖调度的业务变更，避免重复执行或漏执行。

## Queue Worker Unavailable

触发告警：`GoravelMineQueueWorkerUnavailable`

判断：

- 查看 worker deployment 可用副本、事件、最近 rollout。
- 查看 Redis exporter 是否异常，确认 `QUEUE_CONNECTION=redis`。
- 查看 `failed_jobs` 是否增长，按 signature 聚合失败类型。

常用命令：

```bash
kubectl -n goravel-mine get deploy,pods -l app.kubernetes.io/component=queue-worker
kubectl -n goravel-mine describe deploy/goravel-mine-queue-worker
kubectl -n goravel-mine logs deploy/goravel-mine-queue-worker --since=30m
kubectl -n goravel-mine exec deploy/goravel-mine -- /www/main artisan queue:failed
```

止血：

- Redis 连接或密码错误：修正 Secret/ConfigMap 后重启 worker。
- worker 新版本崩溃：回滚 `goravel-mine-queue-worker` deployment。
- 下游依赖故障导致失败激增：暂停相关入口或降并发，待依赖恢复后按 UUID 或 queue 重试。
- 非幂等任务失败：先确认业务状态与 outbox/锁，再重试，避免重复副作用。

复盘记录：

- 队列积压窗口、失败 UUID 数、主要 signature、异常摘要。
- 重试/丢弃数量、是否存在重复副作用、后续幂等或锁修复。

## Queue Backlog

触发告警：`GoravelMineQueueFailedJobsBacklog`、`GoravelMineQueueOutboxBacklog`

判断：

- `/metrics` 查看 `goravel_queue_failed_jobs` 与 `goravel_queue_outbox_events{status=...}`。
- `failed_jobs` 按 payload signature、queue、exception 聚合，先找最大失败来源。
- `queue_outbox` 按 status、topic、last_error 聚合，确认是投递失败、handler 未注册、下游故障还是锁长期占用。

常用命令：

```bash
kubectl -n goravel-mine exec deploy/goravel-mine -- /www/main artisan queue:failed
kubectl -n goravel-mine exec deploy/goravel-mine -- /www/main artisan queue:retry <uuid>
kubectl -n goravel-mine exec deploy/goravel-mine -- /www/main artisan queue:retry --connection=redis --queue=default
```

止血：

- `failed_jobs` 增长：先修复代码/配置/下游依赖，再按 UUID 或 queue 重试；不可恢复任务通过后台丢弃。
- `queue_outbox` pending 高：扩容 queue worker 或提高 `QUEUE_OUTBOX_BATCH`，同时检查 Redis 与 worker 可用性。
- `queue_outbox` failed 高：修复 topic handler、下游依赖或 payload 数据，再将可恢复记录改回 pending 或重新写入 outbox。
- 怀疑重复副作用：先查 idempotency key 与 task lock，再重试。

## 日志查询示例

Loki:

```logql
{service="goravel-mine", level="error"} |= "request_id"
{service="goravel-mine"} | json request_id="extra.request_id" | request_id="req-example"
{service="goravel-mine"} | json trace_id="extra.trace_id" | trace_id="trace-example"
```

本地容器：

```bash
kubectl -n goravel-mine logs deploy/goravel-mine --since=15m | jq -r 'select(.extra.request_id=="req-example")'
```

## 升级路径

- 15 分钟内无法判断根因：升级给后端 owner。
- 30 分钟内无法止血：冻结发布，召集 incident channel。
- 涉及数据错写、权限绕过、凭证泄漏：立即进入安全事件流程，暂停相关入口。
