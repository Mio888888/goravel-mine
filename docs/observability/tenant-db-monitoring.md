# 租户数据库监控策略

应用内 `/metrics` 只暴露 platform connection 的 DB pool 指标，不遍历租户库。

原因：

- Prometheus scrape 周期固定，遍历租户库会放大连接数。
- 新租户库动态注册，应用指标不适合作为租户库发现源。
- 租户 DB 维度属于数据库基础设施监控，应用只负责 request/trace/audit 关联。

## 推荐方案

1. PostgreSQL exporter 连接平台 PostgreSQL 实例，采集所有 database 的 `pg_stat_database`。
2. 用 database label 区分 platform DB 与 tenant DB。
3. Alert rules 按 database 聚合连接数、deadlock、xact rollback、cache hit ratio。
4. Grafana 变量选择 database。
5. 高风险租户单独建 dashboard 或告警 route。

## PromQL 示例

租户 DB 连接数：

```promql
sum by (datname) (pg_stat_activity_count{datname=~"tenant_.*"})
```

租户 DB deadlock：

```promql
increase(pg_stat_database_deadlocks{datname=~"tenant_.*"}[15m]) > 0
```

租户 DB cache hit ratio：

```promql
sum by (datname) (rate(pg_stat_database_blks_hit{datname=~"tenant_.*"}[5m]))
/
clamp_min(
  sum by (datname) (rate(pg_stat_database_blks_hit{datname=~"tenant_.*"}[5m]) + rate(pg_stat_database_blks_read{datname=~"tenant_.*"}[5m])),
  1
)
```

## 验收

- 新建租户后，PostgreSQL exporter 能看到对应 `datname`。
- 该租户 API 请求的 `request_id` 能关联应用日志与 slow SQL。
- 不因为 Prometheus scrape 增加应用侧 DB pool wait。

## 租户治理指标

应用仅查询平台库治理账本，不连接租户库：

- `goravel_tenant_governance_evidence_expired`：已过期证据数；大于零须重新验证。
- `goravel_tenant_governance_verification_failed`：失败的隔离验证 run 数。
- `goravel_tenant_governance_run_age_seconds`：最老 pending/running/awaiting_evidence run 年龄。

处置见 `docs/operations/tenant-governance-runbook.md`。
