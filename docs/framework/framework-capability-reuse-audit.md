# Goravel 框架能力复用审计

审计基线：仓库内 `docs/docs-master/zh_CN` 文档与当前使用的
`github.com/goravel/framework v1.17.2`。

## 结论

项目没有大面积重写 Goravel 核心能力。数据库、ORM、缓存、队列、进程、
文件系统、哈希、路由和服务提供者均通过框架 Facade 或 Contract 使用。
现有自建代码多数是多租户、安全、审计和可靠性领域扩展，不能按同名能力
直接替换。

本次发现的主要复用问题位于项目内部：

- 模糊查询和精确查询过滤器分别在权限、租户、日志和 SSO 模块重复实现。
- 个别服务直接调用 `facades.Orm().Connection()`，绕过项目已有的
  `OrmForConnectionWithContext` 连接工厂。
- 多个出站 HTTP 场景直接使用 `net/http`。其中普通请求可评估迁移到
  `facades.Http()`，但 SSO、动态 URL 任务和 S3 签名请求包含安全或协议约束，
  不能机械替换。

## 已完成改造

### 查询过滤器

统一使用 `app/services/query_filters.go`：

- `applyStringFilter`：忽略空白输入并生成参数化 `LIKE` 条件。
- `equalFilter`：忽略空白输入并生成参数化等值条件。

租户、套餐、参考案例、权限、字典、调度任务、日志、存储配置和 SSO 审计
现在共享同一实现。

### ORM 连接入口

失败队列查询和租户权限审计改用 `OrmForConnectionWithContext` /
`OrmForConnection`。该入口统一处理：

- Goravel ORM 的 `WithContext` 与 `Connection`。
- 动态租户连接注册后的并发保护。
- 租户连接池约束。

### 防回退约束

`tests/unit/framework_reuse_contract_test.go` 增加静态架构测试：

- 查询过滤器只能在 `query_filters.go` 定义。
- 服务层不得直接调用 `facades.Orm().Connection()`。

## 应保留的扩展

| 项目能力 | 对应框架能力 | 结论 |
| --- | --- | --- |
| `app/facades` | Facade / 服务容器 | 官方应用层访问模式，保留 |
| `request.PageResult` | ORM `Paginate` | MineAdmin API 契约适配，保留 |
| `response.Result` | HTTP JSON Response | 前端业务码契约，保留 |
| 密码哈希兼容层 | `facades.Hash()` | 主路径已复用框架，旧 Bcrypt 兼容有迁移价值 |
| Queue Outbox / 幂等记录 | Goravel Queue | 事务消息与业务幂等扩展，框架队列不等价 |
| 数据库动态调度任务 | Goravel Schedule | 支持运行时配置、租户范围、审计日志，框架静态调度不等价 |
| SSO / 调度 URL HTTP 客户端 | `facades.Http()` | 含 DNS 固定、私网拦截和重定向复验，保留安全传输 |
| S3 签名客户端 | Goravel Filesystem | 当前实现涉及对象锁和版本校验；迁移前需验证驱动契约 |

## 待决重复能力

`QueueTaskLockStore` 与 Goravel Cache 原子锁能力重叠，且当前生产代码没有调用方，
只有单元测试、特性测试和 `queue_task_lock` 数据表契约引用。后续清理需要先确认
是否仍有外部模块使用该导出接口，再通过独立数据库迁移删除表；不应直接修改已
发布的历史迁移。

## 后续优先级

1. 为普通第三方 API 调用配置命名 HTTP Client，并使用 `facades.Http()`，
   复用连接池、超时配置和 `Fake` 测试能力。
2. 把服务构造器中的 `WithContext` 与连接字段逐步收敛到少量基础类型，
   但应避免引入继承式大基类。
3. 评估官方 S3 文件系统驱动是否完整支持对象锁、版本 ID 和签名请求；
   只有契约覆盖后再替换自建客户端。
4. 对 `QueueTaskLockStore` 做调用方审计和弃用周期设计；确认无外部依赖后，
   使用框架 Cache 原子锁并新增迁移删除 `queue_task_lock`。

## 文档依据

- `docs/docs-master/zh_CN/architecture-concepts/facades.md`
- `docs/docs-master/zh_CN/architecture-concepts/service-container.md`
- `docs/docs-master/zh_CN/orm/getting-started.md`
- `docs/docs-master/zh_CN/digging-deeper/cache.md`
- `docs/docs-master/zh_CN/digging-deeper/http-client.md`
- `docs/docs-master/zh_CN/digging-deeper/queues.md`
- `docs/docs-master/zh_CN/digging-deeper/task-scheduling.md`
- `docs/docs-master/zh_CN/digging-deeper/filesystem.md`
- `docs/docs-master/zh_CN/security/hashing.md`
