# 项目架构

## 目录索引

- 目录
- 启动链
- 模块系统
- HTTP 层
- 服务分层
- 数据库边界

## 目录

- `main.go`：进程入口。
- `bootstrap/`：应用组装、provider、迁移和模块资源合并。
- `app/modules/`：业务模块 manifest 与模块治理。
- `app/http/`：控制器、中间件、请求绑定、响应结构。
- `app/services/`：按能力域组织的业务与运行时服务。
- `app/models/`、`app/contracts/`：持久化模型与稳定契约。
- `app/scopes/`、`app/support/`：共享 ORM scope 和无业务状态基础能力。
- `database/`：迁移和 Seeder 实现。
- `routes/`：内核级 HTTP 与 gRPC 路由入口。
- `tests/backend/`：全部 Go、脚本和测试数据。
- `docs/api-contract/`：OpenAPI 契约。
- `docs/docs-master/zh_CN/`：仓库内 Goravel 中文框架文档。

## 启动链

应用按 `main.go -> bootstrap.Boot() -> foundation.Setup()` 启动。
`bootstrap/app.go` 是组装入口，统一注册：

- `commands.All()`
- `bootstrap.Migrations`
- `services.QueueJobs`
- 全局 middleware
- operation log、scheduled task、queue outbox runners
- 数据 Seeder 和模块 Seeder
- HTTP/gRPC routing
- providers 与 config

增加全局基础设施前先检查 `bootstrap/app.go` 和 provider。模块级能力不要直接塞入启动链。

## 模块系统

`app/modules/module.go` 的 `Module` 是业务模块主契约：

```go
type Module interface {
    ID() string
    Routes() []Route
    Menus() []Menu
    Permissions() []Permission
    Migrations() []schema.Migration
    Seeders() []seeder.Seeder
    OpenAPIFiles() []string
    TestTemplates() []string
}
```

可选契约包括：

- `MetadataProvider`：版本、依赖、生命周期和 Seeder 策略。
- `PackageProvider`：包归属、兼容矩阵和发布轨道。
- `TenantMigrationProvider`：租户数据库迁移。

内建模块在 `app/moduleboot/modules.go` 注册。新增模块优先使用：

```bash
go run . artisan make:module audit-log
```

实现时以 `app/modules/referencecase/module.go` 为完整参考，不要手工拼出另一套模块协议。

## HTTP 层

`routes/web.go` 仅保存首页、静态资源、健康检查、指标等内核路由，最终调用
`moduleRegistry.RegisterRoutes()`。业务路由必须由模块声明。

后台控制器复用 `app/http/controllers/admin/controller_helpers.go`：

- `jsonResult`、`jsonError`
- `bindJSONBody`、`bindIDList`、`bindIDsObject`
- `queryFilters`、`page`、`pageSize`
- 平台/租户当前用户和租户服务解析 helper

响应使用 `app/http/response` 的 MineAdmin envelope：

```json
{"code": 200, "message": "成功", "data": []}
```

现有管理 API 通常返回 HTTP 200，通过业务 `code` 表达错误。新增接口保持相邻接口行为，
不要混入另一种 envelope 或自行暴露内部错误。

## 服务分层

`app/services/README.md` 定义能力域：

- `access/`：认证、授权、Casbin、SSO。
- `application/`：跨模块应用编排和兼容 API 的实际实现。
- `platform/`：字典、日志、组织、参考案例、存储。
- `runtime/`：迁移锁、可观测性、队列、调度、密钥轮换。
- `tenancy/`：租户运行时基础能力。
- `facade.go`：原 `services.*` API 的兼容门面。

调用方可继续通过 `app/services/facade.go` 使用稳定入口。新增领域实现放进对应能力域和
模块目录；只有真正跨模块的编排进入 `application/`。如果拆分会形成循环依赖，沿用现有
`ConfigureDependencies` 或显式构造参数注入模式，不要把实现搬回根目录。

## 数据库边界

项目同时存在平台数据库和动态租户数据库。

- 平台连接：`services.PlatformConnection()`
- 当前租户：`services.CurrentTenant(ctx)`
- 租户服务：`services.New...ForTenant(tenant)`
- 带上下文 ORM：`services.OrmForConnectionWithContext(ctx, connection)`
- 租户连接名：`services.TenantConnectionName(tenant)`

控制器应从请求 context 取得租户，不接受客户端直接指定任意数据库连接。服务查询使用
`WithContext` 或带 context 的 ORM 工厂，避免丢失取消、审计和可观测性信息。

修改数据边界时至少检查：

- 平台服务不会访问租户默认连接。
- 租户服务不会回退到平台表。
- 缓存键、幂等键、锁键包含必要的租户或连接维度。
- 批处理和队列载荷能重新解析租户，不依赖原请求内存状态。
