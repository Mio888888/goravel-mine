# LogQL 查询集

## 基础查询

最近错误：

```logql
{service="goravel-mine", level=~"error|fatal|panic"}
```

按 `request_id` 串联：

```logql
{service="goravel-mine"} | json request_id="extra.request_id" | request_id="req-example"
```

按 `trace_id` 串联：

```logql
{service="goravel-mine"} | json trace_id="extra.trace_id" | trace_id="trace-example"
```

5xx 请求：

```logql
{service="goravel-mine"} | json status="extra.status" | status >= 500
```

慢请求日志：

```logql
{service="goravel-mine"} | json duration_ms="extra.duration_ms" | duration_ms >= 1000
```

审计事件：

```logql
{service="goravel-mine"} | json event="extra.event" | event="audit"
```

审计失败：

```logql
{service="goravel-mine"} | json event="extra.event", outcome="extra.outcome" | event="audit" | outcome="failure"
```

## 指标化查询

5xx 日志速率：

```logql
sum(rate({service="goravel-mine"} | json status="extra.status" | status >= 500 [5m]))
```

错误日志速率：

```logql
sum(rate({service="goravel-mine", level=~"error|fatal|panic"}[5m]))
```

审计失败速率：

```logql
sum(rate({service="goravel-mine"} | json event="extra.event", outcome="extra.outcome" | event="audit" | outcome="failure" [5m]))
```

按 route 统计慢请求：

```logql
sum by (path) (count_over_time({service="goravel-mine"} | json path="extra.path", duration_ms="extra.duration_ms" | duration_ms >= 1000 [10m]))
```

## 采集验收

1. 触发一个带 `X-Request-Id` 的请求。
2. 5 分钟内用同一 `request_id` 查到 HTTP request log。
3. 触发一个受 audit middleware 保护的接口。
4. 5 分钟内查到 `event="audit"` 日志。
5. 从 Grafana logs dashboard 点击任意日志行，能看到 `request_id`、`trace_id`、`path`、`status`。
