# 告警演练记录

## 演练前检查

- Prometheus 已加载 `deploy/observability/prometheus-rules.yaml`。
- Alertmanager 已合并 `deploy/observability/alertmanager-route.yaml`。
- `severity=page` 接 on-call 渠道。
- `severity=ticket` 接工单渠道。
- 演练环境可临时加载 `deploy/observability/synthetic-alert-rules.yaml`。

## 演练项目

| 场景 | 触发方式 | 期望 |
| --- | --- | --- |
| Page route | 加载 `GoravelMineSyntheticPage` | 1 到 2 分钟内进入 on-call 渠道 |
| Ticket route | 加载 `GoravelMineSyntheticTicket` | 1 到 2 分钟内进入工单渠道 |
| Metrics down | 临时禁用 scrape 或改错 token | 5 分钟后 `GoravelMineNoMetricsScrape` 触发 |
| Slow SQL | 压测慢查询或降低 slow SQL 阈值 | `GoravelMineSlowSQLSpike` 触发 |
| DB pool wait | 降低 DB max open 或压测并发 | `GoravelMineDBPoolWait` 触发 |

## 记录模板

- 日期：
- 环境：
- 演练人：
- 观察人：
- 影响范围：
- 加载规则：
- 触发时间：
- 首次通知时间：
- 首次响应时间：
- 恢复时间：
- 通知渠道：
- Grafana 链接：
- Alertmanager 链接：
- 结果：通过 / 未通过
- 问题：
- 修复项：
- 下次复测日期：

## 收尾

演练后必须移除 `synthetic-alert-rules.yaml`，确认 synthetic alert 已 resolved，不得保留常开。
