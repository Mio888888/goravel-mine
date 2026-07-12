# SLO 周报模板

周期：

负责人：

## 摘要

- 可用性：
- 5xx ratio：
- 错误预算消耗：
- P95：
- P99：
- slow request：
- slow SQL：
- DB pool wait：
- 监控缺口：

## 错误预算状态

```promql
sum(increase(goravel_http_requests_total{status=~"5.."}[30d]))
/
clamp_min(sum(increase(goravel_http_requests_total[30d])), 1)
```

- 当前预算消耗：
- 发布策略：常规 / 加强确认 / 冻结
- 冻结原因：
- 解冻条件：

## Top Incidents

| 时间 | 告警 | 影响 | 根因 | 修复 | 后续 |
| --- | --- | --- | --- | --- | --- |

## Top Slow Routes

| Route | P95 | P99 | 请求量 | 关联 slow SQL | 处理 |
| --- | --- | --- | --- | --- | --- |

## 发布影响

| 版本 | 时间 | 变更 | 发布后 30 分钟 SLO | 结论 |
| --- | --- | --- | --- | --- |

## 行动项

| 优先级 | 事项 | Owner | 截止 | 状态 |
| --- | --- | --- | --- | --- |
