# 中间件平台运维 Runbook

## 当前支持边界

中间件平台复用 Goravel 调度、Queue runner、数据库 Outbox、幂等存储、审计中间件和 Prometheus 指标。当前可注册的消息适配器只来自服务端 `config/queue.go` 中已经配置的连接：

- `sync`：单进程内存执行，非持久，进程退出可能丢失。
- `database`：Goravel 数据库队列，持久化、至少一次投递。
- `redis`：Goravel Redis Queue 驱动，持久化、至少一次投递。

以下能力当前明确为 `unsupported`，不得在页面或对外文档中描述为已实现：

- Redis Stream 消费组、Pending/Claim 和专用死信 Stream。
- Redis Pub/Sub 跨节点广播协议。
- RocketMQ、RabbitMQ、Kafka Broker 适配器。
- 在线替换 Queue 连接密钥或动态新增 Broker 配置。
- 批量死信重放。
- 保护规则多实例生效版本确认。

`redis` 连接是 Goravel Queue 驱动，不等同于 Redis Stream 或 Redis Pub/Sub 专用适配器。保护规则的计数器、熔断状态和并发状态保存在当前进程内存中，实例重启后重置，不具备跨实例共享状态。

## 上线前提

1. 执行平台迁移并确认 `middleware_adapter`、`message_route`、`message_delivery`、`message_dead_letter`、`protection_rule_set`、`protection_rule_version` 和扩展后的 `queue_outbox`、`queue_idempotency` 表存在。
2. 设置 `APP_KEY`，死信原始信封依赖现有 Crypt 服务加密；禁止使用默认或临时密钥上线。
3. 生产环境使用持久化队列时设置 `QUEUE_CONNECTION=database` 或 `QUEUE_CONNECTION=redis`，并保持 `QUEUE_WORKER_ENABLED=true`。
4. 保持 `QUEUE_OUTBOX_ENABLED=true`，按吞吐调整 `QUEUE_OUTBOX_INTERVAL_SECONDS` 与 `QUEUE_OUTBOX_BATCH`。
5. 保持 `SCHEDULER_ENABLED=true`，多实例部署必须使用共享数据库和稳定节点标识。
6. 运行 `go run . artisan migrate`、`go run . artisan module:manifest:check --artifacts --frontend`，再执行目标环境健康检查。

## 最小权限

- PostgreSQL 运行账号只授予应用数据库所需的表、序列和事务权限，不授予建库、角色管理或超级用户权限。
- Redis 账号限制到应用使用的 key 前缀和 Queue 所需命令；禁止开放管理命令和公网访问。
- 管理端按 `platform:middleware:list/configure/execute/publish/replay/payload` 分权。死信载荷查看与重放不得合并为普通查看权限。
- 外部监控只读抓取指标，不授予管理 API 权限。

## 容量规划

上线前记录以下基线：

- `queue_outbox` 的到达率、完成率、Pending 数、最老记录年龄和失败数。
- 每种消息类型与消费者的吞吐、P95/P99 耗时、重试率和死信率。
- Queue 连接并发 `QUEUE_CONCURRENT`、数据库连接池余量和 Redis 内存/连接数。
- 计划任务同时运行数、最长执行时间、重试上限和租户扇出规模。
- 保护规则匹配资源数量、限流键基数和单实例峰值并发。

`sync` 只用于允许进程级丢失且执行时间短的本地事件。关键业务副作用必须使用持久化 Queue、Outbox 和消费者幂等。

## 日常检查

管理入口：`/platform-system/middleware`。

- 适配器：确认启用状态与最近健康检查结果。
- 消息路由：确认草稿已校验、发布版本正确且主适配器健康。
- 消息投递：按消息类型、消费者和状态检查失败趋势。
- 死信：查看失败分类和脱敏摘要；查看载荷需要独立权限。
- 保护规则：检查发布版本、停启状态和当前实例运行状态。
- 监控：关注 Outbox Pending/Failed、失败任务、死信和保护拒绝计数。

计划任务入口：`/platform-system/scheduled-task`。重点检查 `runtime_state`、连续失败、跳过、重试次数和逻辑执行 ID。

## 故障恢复

### 计划任务未触发

1. 确认 `SCHEDULER_ENABLED=true`、应用进程存活和数据库可用。
2. 检查任务状态、Cron、IANA 时区、`next_run_at` 与 `runtime_state`。
3. 执行任务对账，处理 `HANDLER_UNAVAILABLE` 或历史 `LEGACY_UNSAFE`。
4. 只在确认业务幂等后使用立即执行；每次操作生成新的 `Idempotency-Key`。
5. 多实例重复触发时检查共享锁存储、节点时间同步和数据库连接，不要通过删除日志掩盖问题。

### Outbox 积压

1. 检查 `QUEUE_OUTBOX_ENABLED`、Queue worker、数据库或 Redis 连接。
2. 比较到达率与完成率，确认积压是否仍增长。
3. 修复连接或消费能力后让 Outbox runner 接管，不手工复制记录或生成新 `message_id`。
4. 对 `UNKNOWN_RESULT` 先做业务对账，确认副作用状态后再决定重试。
5. 长期失败记录需保留错误摘要与关联 ID，恢复后验证 Pending 和最老年龄下降。

### 死信处理

1. 先按 `failure_class` 判断可重试性，确认 Schema、路由、消费者和外部依赖已经修复。
2. 使用载荷权限查看受控明细；普通审计和日志不得复制完整载荷。
3. 单条重放前二次确认并生成唯一 `Idempotency-Key`。重放保留原 `message_id`。
4. 观察新投递结果后再标记解决。批量重放当前不支持。

### 保护规则误拒绝

1. 检查规则匹配范围、阈值、统计窗口和当前实例状态。
2. 优先停用有问题的规则版本；需要恢复旧内容时使用回滚，回滚会创建新版本。
3. 多实例部署逐实例检查，因为运行状态目前不共享且没有版本汇报协议。
4. 实例重启会清空限流窗口和熔断采样，只可作为明确评估后的应急措施。

## 升级与回滚

- 升级前备份平台数据库并记录当前路由、规则发布版本和环境变量。
- 先执行迁移，再部署应用；不得先启用依赖新字段的页面或任务。
- OpenAPI、模块 manifest、菜单权限和前端产物必须随版本一起发布。
- 本次新增迁移包含兼容性扩展，`202607190003_extend_queue_message_outbox` 的 Down 不删除字段。数据库回滚使用备份恢复或经审核的前向修复迁移。
- 应用回滚前确认旧版本能忽略新增表和字段，并暂停新增配置发布。

## 故障演练

每次大版本至少演练：

1. 关闭 Redis 或数据库 Queue，确认 Outbox 积压、健康状态和告警可见，恢复后消息继续投递。
2. 消费者处理期间终止实例，确认至少一次投递与 Inbox 幂等阻止重复副作用。
3. 构造可重试与不可重试错误，验证重试、死信、单条重放和解决流程。
4. 运行一个 `FORBID` 任务并制造重叠触发，确认新触发记录为跳过。
5. 触发熔断 `CLOSED -> OPEN -> HALF_OPEN -> CLOSED`，验证错误语义和指标。
6. 在两个实例分别检查保护状态，保留“当前为单进程状态”的验收证据。

演练记录需包含时间、版本、操作者、关联 ID、故障注入方式、恢复时间、指标截图和残余风险。
