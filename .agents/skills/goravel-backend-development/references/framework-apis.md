# 框架能力与复用选择

## 使用方法

先在 `docs/docs-master/zh_CN` 查找能力，再以 `go.mod` 中
`github.com/goravel/framework` 的实际版本和仓库现有代码确认 API。文档的 master
分支可能领先于项目依赖，编译结果优先于示例文本。

## 能力索引

| 任务 | 框架文档 | 项目首选入口 |
| --- | --- | --- |
| 启动、容器、provider、facade | `architecture-concepts/` | `bootstrap/app.go`、`app/facades` |
| 路由、控制器、中间件 | `the-basics/routing.md` 等 | `app/modules/routing.go`、`app/http/` |
| 请求、响应、验证 | `the-basics/request.md` 等 | admin controller helpers、`app/http/request`、`app/http/response` |
| 查询、事务、ORM、关系 | `database/`、`orm/` | `services.OrmForConnectionWithContext`、`app/scopes` |
| 迁移和 Seeder | `database/migrations.md`、`seeding.md` | 模块 manifest、`bootstrap/migrations.go` |
| 认证和授权 | `security/` | `app/services/access`、平台/租户 middleware |
| 缓存和原子锁 | `digging-deeper/cache.md` | `facades.Cache()` |
| 队列 | `digging-deeper/queues.md` | `services.QueueJobs()`、`runtime/queue` |
| 调度和 Cron | `digging-deeper/task-scheduling.md` | `runtime/scheduledtask`、`app/support/cronexpr` |
| 文件存储 | `digging-deeper/filesystem.md` | `facades.Storage()`、platform storage service |
| 外部 HTTP | `digging-deeper/http-client.md` | `facades.Http()`、`app/support/safehttp` |
| 进程 | `digging-deeper/processes.md` | `facades.Process()` |
| 加密和哈希 | `security/encryption.md`、`hashing.md` | `facades.Crypt()`、`facades.Hash()` |
| 测试和 mock | `testing/` | `tests/backend/test.sh`、项目 TestCase |

## 强制复用项

以下能力已有契约测试保护，不得重新实现：

- 查询过滤：使用 `app/scopes`，不要定义 `applyStringFilter`、`equalFilter` 等同义 helper。
- 安全外呼：使用 `app/support/safehttp`，不要在业务服务复制 DNS/IP/SSRF 校验。
- 附件存储：使用 Goravel Storage facade 和现有 storage service，不要手写本地文件复制删除。
- Cron 解析：使用 `app/support/cronexpr`，不要在其他包再次配置 `cron.NewParser`。
- 应用 JWT：使用 `app/services/access/auth/jwt.go`，不要分散调用 `jwt.NewWithClaims`
  或 `jwt.ParseWithClaims`。
- 子进程：使用 `facades.Process()`，不要在应用代码引入 `os/exec`。
- 分布式/原子锁：使用 Goravel Cache atomic lock，不要恢复 QueueTaskLockStore。
- 队列可靠性：复用 `app/services/runtime/queue` 的 outbox、幂等、重试和失败任务能力。

相关契约位于 `tests/backend/unit/framework_reuse_contract_test.go`。

## ORM 与事务

- 从请求或任务传入 `context.Context`，使用
  `services.OrmForConnectionWithContext(ctx, connection)`。
- 可组合过滤写成 `app/scopes` 中的 scope，不在每个服务复制空值判断。
- 平台查询显式使用 `services.PlatformConnection()`。
- 租户查询从 `services.CurrentTenant(ctx)` 或任务载荷恢复租户后创建租户服务。
- 事务使用 Goravel ORM transaction API，并确保同一事务内使用相同 connection/context。
- 避免直接依赖底层 GORM，除非当前 Goravel contract 无法表达，且相邻代码已有同类用法。

## 安全与认证

- 平台与租户认证链不同，不要把平台 token 用于租户路由或反向复用。
- 应用 token 的签发和解析通过共享 JWT codec。
- 密码处理使用 `facades.Hash()` 和 `app/services/access/auth` 的安全策略、历史及重哈希逻辑。
- 敏感配置使用 `facades.Crypt()`；密钥来自 config/env，不写入源码。
- 权限使用模块声明的 permission key 和既有 Casbin/敏感操作审批服务。
- 对外 HTTP 必须评估 SSRF、重定向、超时和私网地址，优先使用 `safehttp`。

## 何时新增抽象

只有同时满足以下条件才新增共享抽象：

1. 项目和 Goravel 均无可复用能力。
2. 至少两个调用点存在稳定、相同的行为或一个高风险行为必须集中治理。
3. 抽象有清晰归属，不会跨越平台/租户或模块边界。
4. 有测试证明契约，而不是仅减少少量代码行。

单一调用点优先保留在所属模块。不要创建 `utils`、`common`、`helpers` 等无边界汇总包。
