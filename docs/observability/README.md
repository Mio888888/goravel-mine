# 可观测性运营闭环

本目录连接现有采集点与生产运营动作。应用已提供：

- `/metrics` Prometheus endpoint，受 `OBS_METRICS_ENABLED` 与 `OBS_METRICS_TOKEN` 控制。
- `X-Request-Id`、`X-Trace-Id` 响应头与上下文透传。
- HTTP request counter、duration histogram、in-flight、slow request、slow SQL、Go runtime、platform DB pool、scheduler heartbeat、queue failed jobs、queue outbox backlog、process uptime。
- 平台后台 API：`GET /admin/platform/observability/slow-requests`，返回 slow request 与 slow SQL 样本。
- JSON stdout 日志配置：`LOG_FORMATTER=json`、`LOG_PRINT=true`。

## 文件索引

- `deploy/observability/grafana-dashboard.json`：Grafana dashboard。
- `deploy/observability/grafana-logs-dashboard.json`：日志检索 dashboard。
- `deploy/observability/grafana-audit-dashboard.json`：审计事件 dashboard。
- `deploy/observability/prometheus-rules.yaml`：Prometheus alert rules。
- `deploy/observability/external-alert-rules.yaml`：PostgreSQL/Redis exporter alert rules。
- `deploy/observability/kube-metrics-recording-rules.yaml`：Kubernetes 资源 recording/alert rules。
- `deploy/observability/loki-alert-rules.yaml`：Loki ruler 审计日志告警。
- `deploy/observability/alertmanager-route.yaml`：Alertmanager 路由示例。
- `deploy/observability/prometheus-scrape.yaml`：原生 Prometheus scrape 示例。
- `deploy/observability/servicemonitor.yaml`：Prometheus Operator `ServiceMonitor` 示例。
- `deploy/observability/postgres-exporter.yaml`、`redis-exporter.yaml`：外部依赖 exporter 示例。
- `deploy/observability/promtail.yaml`：Promtail -> Loki 日志采集示例。
- `deploy/observability/otel-collector.yaml`：OpenTelemetry Collector 示例。
- `docs/observability/slo.md`：SLO、错误预算与发布门禁。
- `docs/observability/slo-review-template.md`：SLO 周报模板。
- `docs/observability/on-call-runbook.md`：on-call 告警处理手册。
- `docs/observability/alert-drill.md`：告警演练记录。
- `docs/observability/grafana-import.md`：Grafana 导入验收。
- `docs/observability/logql-queries.md`：LogQL 查询集。
- `docs/observability/otel.md`：OpenTelemetry 接入说明。
- `docs/observability/tenant-db-monitoring.md`：租户 DB 监控策略。
- `docs/observability/production-acceptance.md`：生产端到端验收清单。
- `scripts/verify-observability-assets.sh`：可观测性资产静态校验。
- `scripts/observability-runtime-smoke.sh`：真实监控栈 smoke 验收脚本。

## 接入顺序

1. 生产环境启用 `OBS_METRICS_ENABLED=true`，设置强随机 `OBS_METRICS_TOKEN`。
2. Prometheus 选择 `prometheus-scrape.yaml` 或 `servicemonitor.yaml` 接入 `/metrics`。
3. 部署 PostgreSQL/Redis exporter，接入 kube-state-metrics、cAdvisor、node exporter。
4. Grafana 导入 metrics、logs、audit 三个 dashboard，并按 `grafana-import.md` 验收。
5. Prometheus 加载 Prometheus rules，Loki ruler 加载 `loki-alert-rules.yaml`。
6. Alertmanager 合并 `alertmanager-route.yaml`，路由 `severity=page` 到 on-call，`severity=ticket` 到工单。
7. 设置 `LOG_PRINT=true`，Promtail 加载 `promtail.yaml` 从容器 stdout 采集日志，日志按 `request_id`、`trace_id` 与 metrics 样本串联。
8. 如需 traces，部署 `otel-collector.yaml` 并按 `otel.md` 验收。
9. 按 `slo.md` 每周审阅错误预算，使用 `slo-review-template.md` 留档。
10. 按 `production-acceptance.md` 与 `observability-runtime-smoke.sh` 验收端到端链路，按 `alert-drill.md` 做演练，按 `on-call-runbook.md` 处理真实告警。

## 当前限制

HTTP P95/P99 已由 `goravel_http_request_duration_milliseconds_bucket` 支持。bucket 为固定毫秒边界：50、100、250、500、1000、2500、5000、10000、`+Inf`。

应用仅暴露 platform connection 的 DB pool 指标，不遍历租户库，避免多租户连接被指标抓取放大。租户库容量监控应接 PostgreSQL exporter。

应用内队列指标来自 `failed_jobs` 与 `queue_outbox` 表：`goravel_queue_failed_jobs`、`goravel_queue_outbox_events{status="pending|processing|failed"}`。Prometheus rule 已覆盖失败任务与 outbox backlog 告警。

PostgreSQL、Redis、容器 CPU/memory 与队列 worker 可用性指标已提供接入模板，但需要真实监控栈部署与凭证后才能完成端到端验收。无目标环境时，只能执行静态校验。
