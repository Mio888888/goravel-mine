# 测试与验证

## 目录索引

- 集中布局
- 唯一后端测试入口
- 选择测试层级
- 实施节奏
- 静态和治理检查

## 集中布局

所有测试源码和测试数据只能位于：

- `tests/backend/`
- `MineAdmin-web/tests/`

后端布局：

- `tests/backend/unit/`：跨包规则、纯逻辑和架构契约。
- `tests/backend/feature/`：HTTP、数据库和完整业务行为。
- `tests/backend/integration/`：并发、迁移和外部基础设施协作。
- `tests/backend/_packages/`：映射到生产包的白盒测试。
- `tests/backend/fixtures/`：测试数据。
- `tests/backend/resilience/`、`release/`：韧性和发布门禁。
- `tests/backend/testcase/`、`testsupport/`：共享测试基类与 helper。

不要把 `*_test.go` 放在 `app/`、`bootstrap/`、`config/`、`database/` 或 `routes/`。
需要包内访问时，将测试写到：

```text
tests/backend/_packages/app/services/runtime/queue/example_test.go
```

集中 runner 会用 Go overlay 将它映射到：

```text
app/services/runtime/queue/example_test.go
```

## 唯一后端测试入口

必须使用：

```bash
tests/backend/test.sh ...
```

禁止直接运行 `go test`。`tests/backend/runner_guard_test.go` 会拒绝没有 overlay 的执行，
否则 `_packages` 白盒测试不会参与验证。

常用目标命令：

```bash
# 单个跨包/契约测试
tests/backend/test.sh ./tests/backend/unit -run TestApplicationUsesSharedORMScopes

# 管理 API feature 测试
tests/backend/test.sh ./tests/backend/feature/admin -run TestReferenceCase

# 映射到生产包的白盒测试
tests/backend/test.sh ./app/services/runtime/queue -run TestOutbox

# 集成测试
tests/backend/test.sh ./tests/backend/integration -run TestConcurrentMigration

# 后端完整边界验证
tests/backend/test.sh ./...
```

## 选择测试层级

| 变更 | 最小目标验证 |
| --- | --- |
| 纯函数、scope、parser | 对应 unit 或 `_packages` 白盒测试 |
| service 查询或业务规则 | service 白盒测试 + 相关 unit |
| controller、middleware、响应 | 对应 feature 测试 |
| 迁移、事务、租户连接 | feature 或 integration，必要时数据库刷新 |
| 模块 manifest、权限、OpenAPI | module governance/API contract 测试 |
| 队列、outbox、幂等 | runtime queue 白盒测试 + queue feature 测试 |
| 目录或复用重构 | test layout + framework reuse 契约 |

数据库 feature 测试使用项目 `TestCase` 的 `RefreshDatabase()`。共享 PostgreSQL 测试库上的
数据库 feature 测试不得并行。

## 实施节奏

1. 批量完成同一行为的相关编辑。
2. 运行最小目标测试，定位编译和行为问题。
3. 修改公共契约、跨模块边界、数据库或安全行为时扩大到相关包。
4. 任务边界只运行一次必要的广泛验证。
5. 只有用户明确要求最终完整验证，或改动确实具有全局影响时，运行全部套件。

不要在每个小编辑后重复运行完整测试、race、vet 或 build。

## 静态和治理检查

修改 Go 文件：

```bash
gofmt -w <changed-go-files>
git diff --check
```

模块或 API 契约变化：

```bash
go run . artisan module:manifest:check --artifacts --frontend
tests/backend/test.sh ./tests/backend/unit -run 'TestModule|TestAdminOpenAPI'
```

复用或目录变化：

```bash
tests/backend/test.sh ./tests/backend/unit \
  -run 'TestApplicationUses|TestAttachmentsUse|TestServices|TestConsoleCommands|TestTestCode'
```

验证失败时先判断是实现缺陷、测试环境还是文档与当前框架版本不一致。不要通过绕过集中
runner、删除契约断言或复制旧实现来让测试通过。
