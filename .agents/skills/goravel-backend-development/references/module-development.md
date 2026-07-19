# 模块开发

## 目录索引

- 创建与注册
- 路由与权限
- 控制器
- 服务与依赖
- 迁移与 Seeder
- OpenAPI 与前端契约
- 命令、队列和调度
- 验证清单

## 创建与注册

优先生成骨架：

```bash
go run . artisan make:module audit-log
```

然后完成以下内容：

1. 在 `app/modules/<module>/module.go` 实现模块契约。
2. 内建模块加入 `app/moduleboot/modules.go`；外部模块遵循 admission 机制。
3. 业务实现放入对应 controller、service、model 和 contract 目录。
4. 同步 OpenAPI 和测试清单。
5. 运行模块 manifest 检查。

以 `app/modules/referencecase/module.go` 为 golden reference。

## 路由与权限

路由先声明，再用 handler map 绑定：

```go
func (m Module) Routes() []modules.Route {
    controller := admin.NewReferenceCaseController()
    return modules.BindRouteHandlers(m.ID(), routes(), modules.RouteHandlers{
        "platform.reference-case.list": controller.List,
    })
}
```

支持的 middleware 标签由 `app/modules/routing.go` 定义：

- `public`
- `platform-self-audit`
- `platform-auth`
- `platform-auth-audit`
- `platform-admin`
- `tenant`
- `tenant-rbac`
- `tenant-rbac-audit`
- `tenant-audit-only`

选择最小但完整的保护链。租户路由自动注入 tenant context 和适用的治理 middleware，
不要在控制器重复解析租户头或手工安装 Casbin。

权限 key 沿用 `platform:resource:action` 或现有租户命名约定。路由 Permission、
`Permissions()` 和 `Menus()` 必须一致；新增写操作时确认是否需要审计或敏感操作审批。

## 控制器

控制器只负责：

- 绑定和验证请求。
- 解析当前平台用户或租户。
- 调用服务。
- 使用统一 helper 返回响应。

业务规则、事务、跨记录校验和数据库查询进入服务。沿用
`app/http/controllers/admin/controller_helpers.go`，不要在每个控制器复制分页、JSON
绑定、租户解析和错误映射。

## 服务与依赖

- 单领域实现进入 `access`、`platform`、`runtime` 或 `tenancy` 下的模块目录。
- 跨模块用例编排进入 `application` 对应领域文件。
- 面向既有调用者的 API 可由 `app/services/facade.go` 暴露兼容 alias 或函数。
- 下层包不得反向 import `application`。需要 ORM、配置或审计时，使用构造参数或沿用
  `ConfigureDependencies` 模式。
- 服务方法接收或保存 context 时，沿用相邻服务的 `WithContext` 模式。

## 迁移与 Seeder

框架迁移实现位于 `database/migrations/`，但模块通过 `Migrations()` 声明归属。
`bootstrap/migrations.go` 将内核迁移与模块迁移合并，不要为模块另建迁移启动器。

租户表迁移实现 `TenantMigrationProvider`。迁移必须：

- 有稳定且按时间排序的名称。
- 明确表属于平台库还是租户库。
- 对已有数据和回滚风险做处理。
- 避免在运行时服务中执行隐式 schema 修改。

Seeder 通过模块 `Seeders()` 注册，应保持幂等，并在 `Metadata().SeedStrategy` 描述执行方式。

## OpenAPI 与前端契约

模块 `OpenAPIFiles()` 指向 `docs/api-contract/openapi/` 下真实文件。任何路由、请求、
响应、权限或分页结构变化都要同步契约。

模块 `TestTemplates()` 至少列出相关后端测试；有前端工作流时同时列出
`MineAdmin-web/tests/e2e/` 测试。路径必须真实存在或由模块生成器明确创建。

## 命令、队列和调度

命令按功能子包放在 `app/console/commands/<group>/`，由该组 `Commands()` 汇总，再由
`app/console/commands/commands.go` 注册。根目录只保留总注册文件。

Goravel queue job 必须加入 `services.QueueJobs()`。涉及数据库状态与消息投递一致性的流程
优先写入 outbox；消费者使用现有幂等 store 和重试策略，不另建任务锁表。

调度任务优先复用 `app/services/runtime/scheduledtask`。Cron 表达式通过
`app/support/cronexpr` 解析；执行外部进程用 `facades.Process()`，执行 URL 任务用安全
HTTP 客户端。

## 验证清单

```bash
go run . artisan module:manifest:check --artifacts --frontend
tests/backend/test.sh ./tests/backend/unit \
  -run 'TestModule|TestApplicationUses|TestAttachmentsUse|TestServices|TestTestCode'
```

根据改动再运行模块 feature 测试、对应生产包白盒测试和 OpenAPI 契约测试。
