# SLO 与错误预算

## 服务目标

| 指标 | SLO | 统计窗口 | 数据源 |
| --- | --- | --- | --- |
| 可用性 | 99.5% 非 5xx 请求 | 30 天 | `goravel_http_requests_total` |
| 错误率 | 5xx ratio < 0.5% | 30 天 | `goravel_http_requests_total{status=~"5.."}` |
| 快速止血 | 5xx ratio < 2% | 5 分钟 | `GoravelMineHighErrorRate` |
| HTTP P95 | p95 <= 1000ms | 5 分钟 | `goravel_http_request_duration_milliseconds_bucket` |
| HTTP P99 | p99 <= 2500ms | 5 分钟 | `goravel_http_request_duration_milliseconds_bucket` |
| 慢请求 | 10 分钟新增 slow request <= 20 | 10 分钟 | `goravel_http_slow_requests_observed_total` |
| 慢 SQL | 10 分钟新增 slow SQL <= 10 | 10 分钟 | `goravel_sql_slow_queries_observed_total` |
| DB pool | 5 分钟无连接等待 | 5 分钟 | `goravel_db_pool_wait_count_total` |
| 监控可用 | `/metrics` scrape 连续失败 < 5 分钟 | 5 分钟 | `up{job=~"goravel-mine.*"}` |

## 错误预算

月度可用性 SLO 99.5%，错误预算为 0.5% 请求可失败。

计算：

```promql
sum(increase(goravel_http_requests_total{status=~"5.."}[30d]))
/
clamp_min(sum(increase(goravel_http_requests_total[30d])), 1)
```

预算策略：

- < 50%：允许常规发布。
- 50% 到 80%：发布前必须确认回滚路径与关键 smoke test。
- 80% 到 100%：暂停非关键发布，只允许修复错误率、数据安全、权限安全、合规问题。
- >= 100%：进入稳定性冻结，复盘后恢复常规发布。

## 告警策略

- `severity=page`：影响监控可用或用户可用性，立即响应。
- `severity=ticket`：趋势异常或预算消耗，工作时间内处理。
- 所有告警必须关联 runbook、Grafana dashboard、最近发布记录。

## 发布门禁

发布前：

- `OBS_METRICS_ENABLED=true` 已生效。
- Prometheus 可抓取 `/metrics`。
- Grafana dashboard 最近 15 分钟有数据。
- `GoravelMineNoMetricsScrape` 未触发。
- 错误预算未超过冻结阈值。

发布后 30 分钟：

- 5xx ratio 未高于 2%。
- slow request 与 slow SQL 未持续增长。
- P95/P99 未越过 SLO。
- DB pool wait count 无新增。
- `/health/ready` 正常。
- 关键登录/API smoke test 通过。

## 后续增强

为容量闭环补充：

- PostgreSQL exporter：连接数、锁等待、慢查询、复制延迟。
- Redis exporter：内存、连接、命中率、慢命令。
- Kubernetes 指标：CPU、memory、restart、OOM、pod readiness。
