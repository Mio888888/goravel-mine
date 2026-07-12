# Grafana 导入验收

## Datasource

推荐固定 uid：

- Prometheus：`prometheus`
- Loki：`loki`

对应 provisioning 示例：

- `deploy/observability/grafana-datasources.yaml`
- `deploy/observability/grafana-dashboard-provider.yaml`

## Dashboard

需要导入：

- `deploy/observability/grafana-dashboard.json`
- `deploy/observability/grafana-logs-dashboard.json`
- `deploy/observability/grafana-audit-dashboard.json`

## 导入验收

1. Grafana datasource test 均成功。
2. `Goravel Mine Observability` 最近 15 分钟有 HTTP request rate。
3. P95/P99 panel 有数据，且 query 使用 `goravel_http_request_duration_milliseconds_bucket`。
4. `Goravel Mine Logs` 可按 `request_id`、`trace_id` 查日志。
5. `Goravel Mine Audit` 可见 audit failure rate。
6. 任一告警 annotation 中的 runbook 链接可打开。

## 常见问题

- dashboard 无数据：检查 datasource uid 是否为 `prometheus` / `loki`。
- logs panel 无数据：检查 Promtail labels 是否含 `service="goravel-mine"`。
- P95/P99 无数据：检查 `/metrics` 是否有 `goravel_http_request_duration_milliseconds_bucket`。
- audit 无数据：检查 `OBS_AUDIT_ENABLED=true` 且受保护路由已触发。
