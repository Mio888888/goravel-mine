---
name: goravel-backend-development
description: 基于仓库内 Goravel 中文文档和 goravel-mine 实际架构开发、重构、审查与验证 Go 后端代码。适用于新增或修改模块、路由、控制器、中间件、请求验证、服务、ORM 查询、迁移、Seeder、认证授权、缓存、队列、调度、文件存储、外部 HTTP、命令和后端测试，以及检查重复造轮子、模块边界、租户隔离或测试布局。
---

# Goravel 后端框架开发

## 核心原则

按以下优先级选择实现：

1. 项目现有封装、服务和约定。
2. `go.mod` 当前版本支持的 Goravel facade、contract 或 API。
3. 只有前两者都不能满足，并且新能力有明确复用价值时，才新增抽象。

`docs/docs-master/zh_CN` 用于发现框架能力，不代表当前依赖一定已实现相同 API。先读
`go.mod`，再通过当前代码、依赖源码或编译结果确认 API。不要为绕过版本差异复制框架能力。

## 开发流程

1. 读取仓库根 `AGENTS.md`、`go.mod` 和任务相关代码。
2. 用 `rg` 搜索同类控制器、服务、模型、错误、迁移、命令和测试；先确认归属模块与数据库边界。
3. 按任务读取参考资料：
   - 分层、启动链、目录和租户边界：`references/project-architecture.md`
   - Goravel 能力与项目封装选择：`references/framework-apis.md`
   - 模块、路由、权限、迁移、命令和队列：`references/module-development.md`
   - 测试位置、命令和验证强度：`references/testing-validation.md`
4. 写代码前完成复用检查：
   - 是否已有项目服务、scope、support 包或 facade。
   - 是否已有 Goravel facade/API。
   - 新逻辑应属于现有模块，还是确实形成新的能力模块。
   - 平台库与租户库是否被显式区分。
5. 保持修改范围集中，沿用相邻代码的构造函数、`WithContext`、错误和响应模式。
6. 先运行最小目标测试；任务边界再运行必要的契约、模块清单或完整验证。

## 放置规则

- 业务模块放在 `app/modules/<module>/`，模块 ID 用 kebab-case，Go 包目录去掉分隔符。
- 控制器和中间件放在 `app/http/`；共享请求绑定和响应格式复用已有 helper。
- 服务必须进入 `app/services/{access,application,platform,runtime,tenancy}/` 的正确能力域。
- `app/services/` 根目录只允许兼容门面 `facade.go`；不要新增零散业务文件。
- 跨模块编排才进入 `app/services/application/`，领域实现应进入能力域下的模块目录。
- 可复用、无业务状态的基础能力放入已有 `app/support/<capability>/`；不要创建同义工具包。
- 后端测试只放 `tests/backend/`；包内白盒测试映射到
  `tests/backend/_packages/<production-package>/`。
- 前端测试只放 `MineAdmin-web/tests/`；生产目录不得包含 `testdata`、`fixtures` 或 `__tests__`。

## 实现约束

- 业务路由由模块 manifest 提供，并通过 `modules.BindRouteHandlers` 绑定；`routes/web.go`
  只保留内核级路由。
- HTTP 接口复用 MineAdmin 响应 envelope 和控制器 helper；不要为单个模块发明新响应格式。
- ORM 查询必须传递请求 context，并使用共享 scope 处理可复用过滤。
- 租户操作必须从 context 解析租户并使用租户连接；平台操作使用平台连接。
- 文件、进程、缓存锁、加密、哈希、队列和 HTTP 客户端优先使用 Goravel facade。
- 安全外呼、Cron、JWT、outbox、幂等和租户连接必须复用项目已有实现。
- 新环境变量同步更新 `.env.example`，不得提交密钥、令牌或 `.env`。

## 完成标准

- 代码放置符合模块和能力域边界。
- 没有复制项目已有 helper、服务或框架能力。
- 路由、权限、菜单、迁移、Seeder、OpenAPI 和测试清单按变更同步。
- 新增行为有对应层级测试，且测试没有散落到生产目录。
- 使用 `tests/backend/test.sh` 执行后端测试，禁止直接运行 `go test`。
- 修改过的 Go 文件已 `gofmt`，目标测试和必要契约检查通过。
