# 可观测性生产验收清单

本清单用于把仓库内可观测性资产推进到真实运营闭环。无目标集群、Grafana、Prometheus、Loki、Alertmanager、Tempo 或凭证时，只能完成静态验收，不得标记端到端通过。

真实环境验收建议使用 strict smoke。strict 模式下任何缺少的目标地址或凭证都会失败，不能静默跳过：

```bash
OBS_SMOKE_STRICT=true \
APP_URL=https://api.example.com \
OBS_METRICS_TOKEN=<metrics-token> \
PROM_URL=https://prometheus.example.com \
LOKI_URL=https://loki.example.com \
ALERTMANAGER_URL=https://alertmanager.example.com \
GRAFANA_URL=https://grafana.example.com \
GRAFANA_TOKEN=<grafana-token> \
scripts/observability-runtime-smoke.sh
```

## 1. 官方工具校验

执行：

```bash
scripts/verify-observability-assets.sh
```

通过标准：

- JSON dashboard 可解析。
- YAML 可解析。
- `promtool check rules` 通过。
- `amtool check-config` 通过。

若本机无 `promtool` / `amtool`，必须在交付说明中标注跳过，并在 CI 或运维机补跑。

## 2. Grafana 真导入

资产：

- `deploy/observability/grafana-dashboard.json`
- `deploy/observability/grafana-logs-dashboard.json`
- `deploy/observability/grafana-audit-dashboard.json`
- `deploy/observability/grafana-datasources.yaml`
- `deploy/observability/grafana-dashboard-provider.yaml`

通过标准：

- Prometheus datasource uid 为 `prometheus`。
- Loki datasource uid 为 `loki`。
- 三个 dashboard 均导入。
- 最近 15 分钟 panel 有数据。
- P95/P99、request_id 日志查询、audit failure panel 可用。

## 3. Prometheus 真加载

资产：

- `deploy/observability/prometheus-scrape.yaml`
- `deploy/observability/servicemonitor.yaml`
- `deploy/observability/prometheus-rules.yaml`
- `deploy/observability/external-alert-rules.yaml`
- `deploy/observability/kube-metrics-recording-rules.yaml`

通过标准：

- `/metrics` target 为 `UP`。
- `goravel_http_requests_total` 有样本。
- `goravel_http_request_duration_milliseconds_bucket` 有样本。
- Prometheus rules 状态为 loaded。
- `GoravelMineNoMetricsScrape` 未触发。

## 4. Loki 真采集

资产：

- `deploy/observability/promtail.yaml`
- `deploy/observability/loki-alert-rules.yaml`
- `docs/observability/logql-queries.md`

通过标准：

- 应用生产配置含 `LOG_FORMATTER=json` 与 `LOG_PRINT=true`。
- 5 分钟内能查到 `{service="goravel-mine"}`。
- 能用 `| json request_id="extra.request_id" | request_id="..."` 查到请求日志。
- 能用 `| json trace_id="extra.trace_id" | trace_id="..."` 查到关联日志。
- 能用 `| json event="extra.event" | event="audit"` 查到审计日志。
- Loki ruler 已加载 audit rules。

## 5. Alertmanager 真路由

资产：

- `deploy/observability/alertmanager-route.yaml`
- `deploy/observability/synthetic-alert-rules.yaml`
- `docs/observability/alert-drill.md`

通过标准：

- `severity=page` 到 on-call。
- `severity=ticket` 到工单。
- synthetic page/ticket 演练均收到通知。
- 演练后 synthetic rules 已移除，alert resolved。

## 6. SLO 运营节奏

资产：

- `docs/observability/slo.md`
- `docs/observability/slo-review-template.md`

通过标准：

- 指定 SLO owner。
- 固定周报时间。
- 周报留档至少包含错误预算、P95/P99、slow SQL、DB pool wait、行动项。
- 错误预算超过 80% 时有发布冻结执行人。

## 7. OpenTelemetry 端到端

资产：

- `deploy/observability/otel-collector.yaml`
- `docs/observability/otel.md`

通过标准：

- 应用 OTEL endpoint 指向 collector。
- Tempo 中能按 `X-Trace-Id` 查到 trace。
- Promtail 已采集应用 JSON stdout 到 Loki。
- Loki 中能按同一 `trace_id` 查到日志：`| json trace_id="extra.trace_id" | trace_id="..."`。
- Grafana trace-to-logs 链路通过 Promtail 日志可用。

## 8. 外部 exporter

资产：

- `deploy/observability/postgres-exporter.yaml`
- `deploy/observability/redis-exporter.yaml`
- `deploy/observability/kube-metrics-recording-rules.yaml`

通过标准：

- PostgreSQL exporter target 为 `UP`。
- Redis exporter target 为 `UP`。
- kube-state-metrics、cAdvisor、node exporter 有 goravel-mine pod 样本。
- PostgreSQL connection/deadlock 指标有样本。
- Redis memory/rejected connection 指标有样本。

## 9. Audit 运营验收

资产：

- `deploy/observability/grafana-audit-dashboard.json`
- `deploy/observability/loki-alert-rules.yaml`
- `docs/observability/logql-queries.md`

通过标准：

- 触发受 `OperationLog` middleware 保护的接口。
- Loki 查到 `| json event="extra.event" | event="audit"`。
- dashboard 展示 audit event rate。
- audit failure synthetic 或真实失败能触发 Loki alert。

## 10. 租户 DB 视角

资产：

- `docs/observability/tenant-db-monitoring.md`
- `deploy/observability/postgres-exporter.yaml`

通过标准：

- 新建或选择一个租户库。
- PostgreSQL exporter 能看到对应 database label。
- 可按 database 过滤连接、deadlock、cache hit ratio。
- 应用 `/metrics` 抓取未引起 DB pool wait 增长。
