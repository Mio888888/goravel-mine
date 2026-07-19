<div align="center">

<img src="https://www.goravel.dev/logo.png?v=1.14.x" width="300" alt="Logo">

[![Doc](https://pkg.go.dev/badge/github.com/goravel/framework)](https://pkg.go.dev/github.com/goravel/framework)
[![Go](https://img.shields.io/github/go-mod/go-version/goravel/framework)](https://go.dev/)
[![Release](https://img.shields.io/github/release/goravel/framework.svg)](https://github.com/goravel/framework/releases)
[![Test](https://github.com/goravel/framework/actions/workflows/test.yml/badge.svg)](https://github.com/goravel/framework/actions)
[![Report Card](https://goreportcard.com/badge/github.com/goravel/framework)](https://goreportcard.com/report/github.com/goravel/framework)
[![Codecov](https://codecov.io/gh/goravel/framework/branch/master/graph/badge.svg)](https://codecov.io/gh/goravel/framework)
![License](https://img.shields.io/github/license/goravel/framework)

</div>

[English](./README.md) | 中文

# 关于 Goravel

## Goravel MineAdmin 本地开发

本仓库当前实现 MineAdmin V3 兼容的 Goravel 后端，默认使用 PostgreSQL、Redis、JWT Auth、Casbin RBAC、附件本地存储与 Artisan 工具命令。

二开模块开发请先阅读 [模块采用者指南](./docs/framework/module-adopter-guide.md)，再使用 `make:module` 生成后端、前端、OpenAPI 与测试骨架。

### 环境要求

- Go 1.24+
- PostgreSQL 16+
- Redis 7+
- 可选：Docker Compose、Air

### 本机启动

```bash
cp .env.example .env
go mod download
go run . artisan migrate
go run . artisan db:seed
go run .
```

默认监听 `127.0.0.1:3000`。开发热重载可使用：

```bash
air
```

### Docker Compose

```bash
docker compose up -d postgres redis
go run . artisan migrate
go run . artisan db:seed
docker compose up --build goravel
```

Compose 会启动 `goravel`、`postgres`、`redis`，并为 PostgreSQL、Redis 与应用服务配置 healthcheck。

### 生产部署

生产部署基线见 [docs/deployment/production.md](./docs/deployment/production.md)，包含非 root 容器、`docker-compose.prod.yml`、Kubernetes/Helm、readiness/liveness、滚动升级、迁移发布策略、备份恢复与资源限制。

```bash
cp .env.production.example .env.production
docker compose --env-file .env.production -f docker-compose.prod.yml up -d --build
```

### 常用验证

```bash
tests/backend/test.sh ./...
go run . artisan migrate
go run . artisan db:seed
go run . artisan make:module audit-log
go run . artisan module:manifest:check
go run . artisan make:crud demo_table --module=demo --force
```

数据库 feature 测试会刷新共享 PostgreSQL 测试库，避免同时并行运行多个会调用 `RefreshDatabase()` 的测试命令。

### 配置提示

- `.env` 不应提交；新增变量请同步 `.env.example`。
- `CACHE_STORE=redis` 时使用 Redis 缓存；测试环境会强制使用 memory cache。
- 本地 `QUEUE_CONNECTION=sync` 为默认值；生产模板使用 `redis` 并通过 `QUEUE_WORKER_ENABLED` 分离 Web 与队列 worker。
- 上传文件默认保存在 `storage/app/public/uploads`；启用平台「储存配置」的 `s3_compatible` 驱动后，可写入 MinIO / AWS S3 / OSS / COS / 七牛 / OBS 等兼容端点。路径按 `platform/` 与 `tenants/{tenant_code}/` 隔离。

Goravel 是一个功能完备、具有良好扩展能力的 Web 应用程序框架。作为一个起始脚手架帮助 Golang 开发者快速构建自己的应用。

框架风格与 [Laravel](https://laravel.com/) 保持一致，让 Phper 不用学习新的框架，也可以愉快的玩转 Golang！致敬 Laravel！

欢迎 Star, PR, Issues！

## 文档

在线文档 [https://www.goravel.dev/zh_CN](https://www.goravel.dev/zh_CN)

示例 [https://github.com/goravel/example](https://github.com/goravel/example)

> 优化文档，请提交 PR 至文档仓库 [https://github.com/goravel/docs](https://github.com/goravel/docs)

## 主要功能

| 模块名称 | 描述 |
|-------------|-------------|
| [Artisan Console](https://www.goravel.dev/zh_CN/digging-deeper/artisan-console.html) | 用于应用管理和自动化的 CLI 命令行界面 |
| [Authentication](https://www.goravel.dev/zh_CN/security/authentication.html) | 使用 JWT 和 Session 驱动的用户身份验证 |
| [Authorization](https://www.goravel.dev/zh_CN/security/authorization.html) | 基于策略和守卫的访问控制 |
| [Cache](https://www.goravel.dev/zh_CN/digging-deeper/cache.html) | 使用内存、Redis 或自定义驱动存储和检索数据 |
| [Carbon](https://www.goravel.dev/zh_CN/digging-deeper/helpers.html) | 日期和时间操作的辅助函数 |
| [Config](https://www.goravel.dev/zh_CN/getting-started/configuration.html) | 从文件和环境变量管理应用配置 |
| [Crypt](https://www.goravel.dev/zh_CN/security/encryption.html) | 数据加密和解密工具 |
| [DB](https://www.goravel.dev/zh_CN/database/getting-started.html) | 数据库查询构建器 |
| [Event](https://www.goravel.dev/zh_CN/digging-deeper/event.html) | 应用事件分发和监听系统 |
| [Factory](https://www.goravel.dev/zh_CN/orm/factories.html) | 生成用于测试的模拟数据 |
| [FileStorage](https://www.goravel.dev/zh_CN/digging-deeper/filesystem.html) | 支持多个驱动的文件上传、下载和存储 |
| [Grpc](https://www.goravel.dev/zh_CN/the-basics/grpc.html) | 高性能 gRPC 服务端和客户端实现 |
| [Hash](https://www.goravel.dev/zh_CN/security/hashing.html) | 安全密码哈希 |
| [Http](https://www.goravel.dev/zh_CN/the-basics/routing.html) | HTTP 路由、控制器和中间件管理 |
| [Http Client](https://www.goravel.dev/zh_CN/digging-deeper/http-client.html) | HTTP 客户端 |
| [Localization](https://www.goravel.dev/zh_CN/digging-deeper/localization.html) | 多语言支持 |
| [Logger](https://www.goravel.dev/zh_CN/the-basics/logging.html) | 应用日志记录到文件、控制台或外部服务 |
| [Mail](https://www.goravel.dev/zh_CN/digging-deeper/mail.html) | 通过 SMTP 或队列发送邮件 |
| [Mock](https://www.goravel.dev/zh_CN/testing/mock.html) | 为 facade 和依赖创建模拟测试 |
| [Migrate](https://www.goravel.dev/zh_CN/database/migrations.html) | 支持版本控制的数据库迁移 |
| [Orm](https://www.goravel.dev/zh_CN/orm/getting-started.html) | 优雅的 ORM 数据库操作实现 |
| [Package Development](https://www.goravel.dev/zh_CN/digging-deeper/package-development.html) | 构建可重用的扩展包以扩展框架功能 |
| [Process](https://www.goravel.dev/zh_CN/digging-deeper/process.html) | 围绕 Go 标准 os/exec 包构建的表达力强且优雅的 API |
| [Queue](https://www.goravel.dev/zh_CN/digging-deeper/queues.html) | 将耗时任务延迟到后台任务处理 |
| [Seeder](https://www.goravel.dev/zh_CN/database/seeding.html) | 使用测试或初始数据填充数据库表 |
| [Session](https://www.goravel.dev/zh_CN/the-basics/session.html) | HTTP Session 会话管理 |
| [Task Scheduling](https://www.goravel.dev/zh_CN/digging-deeper/task-scheduling.html) | 使用类 cron 表达式调度周期性任务 |
| [Testing](https://www.goravel.dev/zh_CN/testing/getting-started.html) | HTTP 测试、模拟和断言工具 |
| [Validation](https://www.goravel.dev/zh_CN/the-basics/validation.html) | 使用规则验证传入的请求数据 |
| [View](https://www.goravel.dev/zh_CN/the-basics/views.html) | HTML 模板引擎 |

## 与 Laravel 对比

[查看详情](https://www.goravel.dev/zh_CN/prologue/compare-with-laravel.html)

## 路线图

[查看详情](https://github.com/goravel/goravel/issues?q=is%3Aissue+is%3Aopen)

## 优秀扩展包

[查看详情](https://www.goravel.dev/zh_CN/getting-started/packages.html)

## 贡献者

这个项目的存在要归功于所有做出贡献的人，参与贡献请查看[贡献指南](https://www.goravel.dev/zh_CN/prologue/contributions.html)。

## 打赏

开源项目的发展离不开您的支持，感谢微信打赏。

<p align="left"><img src="https://www.goravel.dev/reward-wechat.jpg" width="200"></p>

## 群组

微信入群，请备注 Goravel

<p align="left"><img src="https://www.goravel.dev/wechat.jpg" width="200"></p>

## 开源许可

Goravel 框架是在 [MIT 许可](https://opensource.org/licenses/MIT) 下的开源软件。
