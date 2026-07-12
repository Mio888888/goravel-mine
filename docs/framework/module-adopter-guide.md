# 模块采用者指南

本文面向二开团队，说明如何在当前 Goravel MineAdmin 框架中创建、声明、验证和发布模块。

## 创建模块

使用脚手架生成后端、OpenAPI、测试与前端骨架：

```bash
go run . artisan make:module audit-log
```

输出文件：

- `app/modules/auditlog/module.go`
- `app/modules/auditlog/model.go`
- `app/modules/auditlog/migration.go`
- `app/modules/auditlog/repository.go`
- `app/modules/auditlog/service.go`
- `docs/api-contract/openapi/audit-log.openapi.json`
- `docs/framework/modules/audit-log/README.md`
- `tests/feature/admin/audit_log_test.go`
- `MineAdmin-web/src/modules/audit-log/api/index.ts`
- `MineAdmin-web/src/modules/audit-log/manifest.ts`
- `MineAdmin-web/src/modules/audit-log/locales/zh_CN.yaml`
- `MineAdmin-web/src/modules/audit-log/types/index.ts`
- `MineAdmin-web/src/modules/audit-log/views/index.vue`
- `MineAdmin-web/tests/e2e/audit-log.spec.ts`

若目标文件已存在，命令会停止。确认覆盖时使用：

```bash
go run . artisan make:module audit-log --force
```

## 注册模块

内置模块仍由 `app/moduleboot/modules.go` 汇总注册。新增模块完成后，将 `New()` 加入 `modules.NewRegistry` 列表。

外部模块不支持运行时任意加载。企业分发流程是 signed source package + compile-time admission：先验证仓库 manifest、digest、signature 和 compatibility matrix，再把源码纳入构建并注册。

模块必须实现 `modules.Module`：

- `ID()`：稳定唯一标识，使用 kebab-case。
- `Routes()`：模块拥有的 HTTP route manifest 和 installer。
- `Menus()`：模块菜单。
- `Permissions()`：模块权限。
- `Migrations()`：模块迁移。
- `Seeders()`：模块 seed。
- `OpenAPIFiles()`：模块 OpenAPI fragment。
- `TestTemplates()`：模块测试入口。

## 声明元数据

模块可实现 `Metadata() modules.Metadata`。建议所有业务模块显式声明：

```go
func (m Module) Metadata() modules.Metadata {
    metadata := modules.BuiltinMetadata(
        "Audit Log",
        modules.RequiredDependency("tenant-rbac"),
    )
    metadata.Frontend = modules.FrontendArtifact{
        ModulePath: "MineAdmin-web/src/modules/audit-log",
        ApiFiles: []string{
            "MineAdmin-web/src/modules/audit-log/api/index.ts",
        },
        TypeFiles: []string{
            "MineAdmin-web/src/modules/audit-log/types/index.ts",
        },
        TestFiles: []string{
            "tests/feature/admin/audit_log_test.go",
        },
    }
    return metadata
}
```

关键字段：

- `Version`：模块版本，内置模块默认 `1.0.0`。
- `Compatible`：兼容框架版本。
- `Dependencies`：依赖模块，registry 会按依赖排序并校验缺失依赖。
- `Lifecycle`：安装、卸载、升级、回滚、破坏性检查策略。
- `SeedStrategy`：seed 模式，默认应幂等。
- `Frontend`：前端模块/API/types/test 产物。

## 路由归属

`routes/web.go` 只保留 kernel 级入口：

- `/`
- static assets
- `/health/live`
- `/health/ready`
- `/metrics`
- `moduleRegistry.RegisterRoutes()`

业务路由必须归属于模块。新增路由时：

1. 在模块 `Routes()` 中写 manifest。
2. 设置 `Permission` 或 `Permissions`。
3. 设置 `Middlewares`。
4. 给 route 设置 `Install` 函数。
5. 更新 OpenAPI fragment 与测试。

## Manifest 验证与导出

验证 manifest：

```bash
go run . artisan module:manifest:check --artifacts --frontend
```

导出到 stdout：

```bash
go run . artisan module:manifest:export
```

导出到文件：

```bash
go run . artisan module:manifest:export --target=storage/framework/module-manifest.json
```

manifest 当前覆盖：

- 模块 ID、名称、版本、兼容性、启用状态
- 依赖声明
- 生命周期策略
- seed 策略
- 前端产物
- routes、menus、permissions、migrations
- OpenAPI files、test templates

## Package 与兼容矩阵

模块可实现 `Package() modules.Package`。内置模块可使用：

```go
func (m Module) Package() modules.Package {
    return modules.BuiltinPackage(m.ID(), "platform-team")
}
```

外部分发模块必须显式声明：

- `ImportPath`：源码接纳后的 Go import path。
- `RegistryKey`：必须等于模块 `ID()`。
- `Version`：模块包版本。
- `Owner`：责任团队。
- `ReleaseTrack`：`stable`、`beta`、`experimental`、`internal`、`deprecated`。
- `Compatibility`：框架版本约束。
- `Digest`：source package 或 artifact digest。
- `Signature`：签名引用。
- `Deprecated` / `ReplacedBy`：弃用与替代模块。

非 `internal` 模块缺少 `Digest` 或 `Signature` 会被 runtime validation 拒绝。发布时导出 compatibility matrix，作为模块市场、升级审批、灰度发布和回滚策略的共同依据。

## 供应链接纳

外部模块只能在构建前接纳，运行时不下载、不解压、不执行外部代码，也不使用 Go plugin 或反射加载。`module-admission.lock.json` 与生成的 `app/moduleboot/admitted_modules_gen.go` 是唯一允许进入编译产物的外部模块声明；`moduleboot.Modules()` 会把生成 registry 合并到内建 registry，因此 admitted 模块的路由、迁移和 seed 都在编译时确定。生成 registry 内嵌 lock digest，发布证据必须验证两者绑定。

仓库 index 使用 `v1` 严格 JSON（未知字段、重复 `id@version`、非精确版本和非法 digest 均拒绝）。每个外部条目必须提供精确 `id`、`version`、`source_uri`、`sha256:<64 hex>`、Go import path、依赖、Cosign issuer/identity、SBOM 与 provenance digest。依赖只能引用精确版本；解析顺序、lock JSON、图 digest 与 registry 源码均固定排序。解包前先下载到受限临时工作区并验证大小与 SHA-256；zip 解包拒绝绝对路径、`..`、符号链接、超量文件和超限展开大小。

Cosign 校验必须实际执行 `cosign verify-blob` 和 attestation 命令，且绑定指定 issuer、identity 与源 bundle digest。SBOM 与 provenance 的 subject digest 都必须等于同一 bundle SHA-256，并要求 Rekor inclusion proof。成功证据记录 Cosign 版本、issuer、identity、Rekor 状态、SBOM/provenance digest 与验证时间；校验失败不会生成或替换 lock/static registry。

先执行只读预检。预检下载、解包、解析依赖并验证外部证据，只输出服务端计算的 `resource`、`binding_digest` 与 `module_binding`，不写 lock 或 registry：

```bash
go run . artisan module:admission:check \
  --index=app/moduleadmission/testdata/index.valid.json \
  --workspace=tmp/module-admission \
  --requester-id=<platform-user-id> \
  --prepare
```

远程 index 必须同时指定其固定 digest，且 host 必须列入 `MODULE_ADMISSION_ALLOWED_INDEX_HOSTS`：

```bash
go run . artisan module:admission:check \
  --index=https://registry.example/modules/index.json \
  --index-digest=sha256:<64-hex> \
  --workspace=tmp/module-admission
```

用预检返回的 `resource` 创建 `module.admission.approve` 审批并完成双人批准，再申请同一 resource 的 re-auth token。执行接纳时仅传 `--requester-id` 与 `--evidence-stdin`，并从 stdin JSON 读取 `approval_id`、`reauth_token`，不得把凭据放入 argv。命令会再次计算并验证全部 digest，在临时目录完成 Cosign/SBOM/provenance、依赖和 registry 验证，最后才一次性消费审批与 re-auth、替换 lock/registry。任何输入变化均需重新预检和审批；预检或外部验证失败不消费凭证、不替换上一版 registry。

```bash
printf '%s' '{"approval_id":"<approval-id>","reauth_token":"<reauth-token>"}' | \
  go run . artisan module:admission:check \
    --index=<index.json> --requester-id=<platform-user-id> --evidence-stdin
```

## 弃用与替代

`Deprecated=true` 与 `ReplacedBy` 只表示通知，不足以允许移除。弃用模块必须实现 `ReplacementPlanProvider`，声明 `prepare -> dual_run -> cutover -> rollback_window -> retired`，以及 data/config/permission migration、validation、cutover、rollback 命令和各命令 policy hash。lifecycle planner 依次安装替代模块、执行前向迁移、验证和切流，最后才卸载旧模块；cutover 后同步失败会执行声明的 rollback，晚完成或锁租约不确定时进入 reconciliation。

在 `end_of_support` 前移除必须使用 `module.replacement.emergency-remove` 的独立 re-auth 与双人审批；普通 `module.lifecycle.execute` 证据不能绕过该门。到达 EOS 后仍需正常 lifecycle 敏感操作证据。

## 禁用模块

开发或验证时可用环境变量禁用模块：

```bash
MODULE_DISABLED=scheduled-task go test ./tests/unit
MODULES_DISABLED=scheduled-task,data-center go run . artisan module:manifest:check --artifacts --frontend
```

若启用模块依赖被禁模块，manifest 校验会失败。

企业禁用策略分四层：

- 环境层：`MODULE_DISABLED` / `MODULES_DISABLED`。
- 租户层：tenant governance module flags。
- 热禁用：仅 `supports_hot_disable=true` 的模块可不重启禁用。
- 重启禁用：`requires_restart=true` 的模块必须进入变更窗口。

## 生命周期编排

`module:plan` 只生成发布证据，不写入数据库、不执行命令：

```bash
go run . artisan module:plan --action=upgrade
go run . artisan module:plan --action=rollback
```

`module:lifecycle` 仅生成 dry-run JSON，不获取锁、不写状态、不执行命令：

```bash
go run . artisan module:lifecycle --action=upgrade --module=audit-log
```

CLI 传 `--execute` 会被拒绝，避免绕过审批链。真实编排仅允许经平台管理 API `/admin/platform/module-lifecycle/execute` 发起，并强制绑定 operator、confirm token、re-auth token、一次性 approval ID、owner 与 reason；成功过门后才获取持久锁、写入 `module_state` / `module_lifecycle_run` / `module_lifecycle_step`，并按依赖顺序执行 manifest 命令。

行为约束：

- `install` / `upgrade` 按依赖正序执行。
- `rollback` / `uninstall` 按依赖反序执行。
- 幂等 key 为 `action:module_id:version`；成功执行过的同 key 再次执行会跳过。
- 同一模块同一时间只允许一个 lifecycle 执行者持有锁。
- 失败会写入 run 与 state，可修复后重试。
- 依赖版本约束支持 `>=`、`>`、`<=`、`<`、`=` 与精确版本。

内部实现已收敛为 ports：`LifecycleRepository` 负责 run/step/state，`LifecycleLockManager` 负责 lease，`LifecycleCommandRunner` 负责 allowlist 命令，`LifecycleClock` 负责时间与 timeout。采用者只依赖 `LifecycleService`、CLI 与管理 API，不应直接依赖 DB store 或 adapter。

## OpenAPI 与前端

每个模块至少声明一个 OpenAPI fragment。fragment 路径写入 `OpenAPIFiles()`；`module:manifest:check --artifacts` 会读取 JSON 并执行通用 OpenAPI 3.x 语义 lint。lint 检查内部 `$ref`、重复 `operationId`、重复 method/path、重复参数，以及跨 fragment 的同名 component 冲突；对声明 `x-permission` 的 operation，还会校验其属于同一 method/path 的模块 route 权限集合。

共享 `admin-base-apis.openapi.json` 可以覆盖多个模块的 route 子集，旧 fragment 未声明 `x-permission` 时保持兼容；新增或变更 operation 应声明稳定、全局唯一的 `operationId`，并在权限可确定时增加 `x-permission`。同名 component 必须为 canonical JSON 完全一致，或具有可证明的通用/具体兼容形状；否则请为模块 component 加命名空间，避免 bundle 冲突。

模块 OpenAPI bundle 可通过服务层 `BuildManifestOpenAPIBundle()` 生成。它在写入前完成 lint，以稳定顺序合并 paths/components，并返回内容的 SHA-256；调用方应以临时文件写入后原子替换目标，避免将失败或半成品 bundle 交给 SDK 生成和发布证据流程。

模块治理自身的 7 endpoint OpenAPI 由 Go contract test 校验 runtime、Markdown、权限、response envelope 与前端 wrapper parity。通用 `admin-base-apis.openapi.json` 已生成 `src/generated/admin-api.ts`；模块治理 wrapper 目前仍为手写 typed API，尚未纳入 generated SDK。前端代码放在 `MineAdmin-web/src/modules/<module>/`，API wrapper 使用 `useHttp()`，不要使用旧 plugin 系统。

## Golden Reference Module

内置 `reference-case` 是当前 golden reference module。它展示完整模块闭环：

- 后端模块：`app/modules/referencecase/module.go` 已注册 routes、menus、permissions、migration、OpenAPI 与 test templates。
- 基准数据：模块 seeder 幂等写入 `golden-case`，随 `db:seed` 执行。
- 迁移与模型：`reference_case` 表、`app/models/reference_case.go`。
- 服务与 API：`ReferenceCaseService` 与 `/admin/platform/reference-case/*` CRUD。
- 权限菜单：`platform:referenceCase:*` 与平台系统菜单 seed。
- 前端页面：`MineAdmin-web/src/modules/base/views/platform/referenceCase/index.vue` 与 typed API wrapper。
- 验收：`tests/feature/admin/reference_case_test.go`、`MineAdmin-web/tests/e2e/reference-case.spec.ts`。

升级/回滚样例写在模块 lifecycle：`reference-case:upgrade` 原子新增示例列并升至 `1.1.0`，`reference-case:rollback` 定向撤销并回到 `1.0.0`；二者均受生命周期审批、二次认证、step 记录与 allowlist 约束，不调用全局 `migrate:rollback`。

## 测试策略

模块最小测试：

- route/permission manifest 单元测试
- feature test 覆盖主要 HTTP 流程
- service unit test 覆盖核心业务规则
- 前端页面或权限变化补 Playwright E2E

推荐验证：

```bash
go test ./app/modules ./app/modulecatalog ./app/console/commands ./tests/unit -count=1
go test ./tests/feature/admin -run 'TestModuleLifecycleTestSuite|TestModuleLifecycle' -count=1
go run . artisan module:manifest:check --artifacts --frontend
```

涉及前端时补：

```bash
cd MineAdmin-web
yarn contract:openapi
yarn lint:tsc
yarn test:e2e module-lifecycle.spec.ts --project=chromium
yarn build
```

## 发布与升级

发布前必须确认：

- `module:manifest:check --artifacts --frontend` 通过。
- `module:manifest:export --target=storage/framework/module-manifest.json` 已归档。
- `module:plan --action=upgrade` 已归档；涉及回滚演练时补 `module:plan --action=rollback`。
- 管理 API lifecycle execute 响应、runs、steps、diff 与 `module:state` 已归档。
- 新迁移具备可解释 rollback 策略。
- seed 幂等。
- OpenAPI fragment 与前端 API 同步。
- 权限与菜单 seed 来源可追溯。
- 破坏性变更写入 release notes，并走人工 review。

当前仓库 release workflow 已自动生成并 hard-gate compatibility matrix。manifest export、plan、lifecycle dry-run/execute、rollback drill 与审批记录仍属于发布流程证据，尚未全部进入 `.github/workflows/release.yml` 与 `scripts/release-hard-gate.sh`；不得把人工归档清单误称为现有自动门禁。

外部模块升级补充要求：

- 仓库 manifest digest/signature 已验证。
- compatibility matrix 已归档。
- canary tenant 或 staging 执行结果已归档。
- step-level run、stdout/stderr 摘要、state diff 已归档。
- failed/manual_required 重试必须重新审批。

完整生态路线见 [模块生态路线图](module-ecosystem-roadmap.md)。

## 后续缺口

- `make:module` 已生成可校验的 CRUD-like baseline；新业务模块应继续对齐 `reference-case` 的真实 controller/service/model/E2E 闭环。
- frontend menu/permission parity 已进入 `module:manifest:check --frontend`；后续可继续补强 API schema 与 route 参数级 parity。
- 将 `module-governance.openapi.json` 纳入 generated frontend types，消除手写契约维护面。
- 将 manifest、plan、lifecycle dry-run/execute、审批、回滚演练与 WORM 审计证据逐步接入 release hard gate。

发布证据按 [发布证据清单](../deployment/evidence-checklist.md) 归档。
