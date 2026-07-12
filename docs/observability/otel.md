# OpenTelemetry 接入

## 应用环境变量

```env
OTEL_PROPAGATORS=tracecontext,baggage
OTEL_TRACES_EXPORTER=otlptrace
OTEL_METRICS_EXPORTER=otlpmetric
OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://otel-collector:4318
OTEL_EXPORTER_OTLP_METRICS_ENDPOINT=http://otel-collector:4318
```

## Collector

示例：`deploy/observability/otel-collector.yaml`

默认路径：

- traces -> Tempo OTLP HTTP
- metrics -> Prometheus remote write

日志不走 OTEL logs exporter。当前应用日志链路为 JSON stdout -> Promtail -> Loki，见 `deploy/observability/promtail.yaml`。

## 验收

1. 访问任意 API，响应头包含 `X-Trace-Id`。
2. Tempo 中可按同一 trace id 搜到 span。
3. Loki 中可按同一 trace id 搜到 Promtail 采集的 JSON stdout 日志。
4. Grafana 从 trace 跳日志，能看到同一 `request_id`。

## 限制

当前仓库已有 trace id 传播与 config，但真实 exporter 需要部署 collector 与后端存储。无 Tempo、Loki、Prometheus remote write 与 Promtail 环境时，不应宣称 traces/logs/metrics 已端到端打通。
