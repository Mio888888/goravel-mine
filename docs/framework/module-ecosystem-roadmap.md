# 模块生态路线图

本文定义企业级模块生态的目标形态。当前系统继续禁止运行时加载任意 Go 插件；外部模块必须先进入源码、通过签名与兼容矩阵校验，再由编译期 registry 接纳。

## 安全边界

- Go 后端模块不做运行时动态加载。
- 模块仓库只分发 source package、manifest、SBOM 与签名证据。
- 接纳流程是 compile-time admission：拉取源码、验证 digest/signature、运行 manifest check、编译、测试、发布。
- 前端扩展使用 `MineAdmin-web/src/modules/<module>` 常规模块，不使用旧 plugin 系统。

## Repository Manifest

模块仓库 manifest 最小结构：

```json
{
  "modules": [
    {
      "id": "audit-log",
      "import_path": "goravel/app/modules/auditlog",
      "version": "1.2.3",
      "digest": "sha256:<artifact digest>",
      "signature": "cosign:<signature reference>",
      "owner": "platform-team",
      "release_track": "stable",
      "compatibility": [">=1.17.0 <2.0.0"],
      "dependencies": [
        { "id": "tenant-rbac", "version_constraint": ">=1.0.0", "required": true }
      ],
      "deprecated": false,
      "replaced_by": ""
    }
  ]
}
```

规则：

- `release_track` 仅允许 `stable`、`beta`、`experimental`、`internal`、`deprecated`。
- 非 `internal` 模块必须有 `digest` 与 `signature`。
- `deprecated=true` 时必须声明 `replaced_by`。
- `compatibility` 必须覆盖框架版本范围。
- `dependencies` 必须与模块代码中的 `Metadata.Dependencies` 一致。

## 已交付能力

- compile-time registry、依赖排序、禁用传播、manifest 与 compatibility projection 已落地。
- signed source package 所需的 digest/signature/package metadata 已进入 runtime validation。
- lifecycle plan、dry-run、审批执行、持久锁、续锁、run/step/state、stale-lock release 与管理页已落地。
- tenant governance module flags 已通过 `InstallTenant*Route` 自动注入的 middleware 进入租户请求路径。
- release workflow 已生成并 hard-gate compatibility matrix。

## Compatibility Matrix

框架发布时输出 compatibility matrix，字段来自 `modulecatalog.Service.CompatibilityMatrix()`：

- framework version
- module id/name/version
- package import path、owner、release track、digest、signature
- compatibility constraints
- dependencies
- requires restart / supports hot disable
- breaking change policy
- enabled / disabled reason

目标发布页应同时附：

- `module:manifest:export` 产物
- compatibility matrix
- SBOM
- lifecycle dry-run plan
- rollback drill artifact

## Disable Strategy

禁用分四层：

- 环境层：`MODULE_DISABLED` / `MODULES_DISABLED`，用于编译后部署实例级禁用。
- 租户层：tenant governance module flags，控制租户可见和可调用模块。
- 热禁用：仅 `supports_hot_disable=true` 的模块可不重启禁用。
- 重启禁用：`requires_restart=true` 的模块必须经过变更窗口。

禁用时必须记录：

- requester / approver
- reason
- impacted tenants
- dependency cascade
- rollback path

## Upgrade Strategy

升级标准流程：

1. 验证仓库 manifest digest/signature。
2. 运行 `module:manifest:check --artifacts --frontend`。
3. 执行 `module:compatibility:export --framework-version=<goravel-version>` 导出 compatibility matrix。
4. 执行 `module:lifecycle --action=upgrade` dry-run。
5. 获取审批与二次认证。
6. 选择 canary tenant 或 staging 环境执行。
7. 归档 step-level run、stdout/stderr 摘要、state diff。
8. 执行 rollback drill 或保留已验证 rollback plan。

失败策略：

- allowlist 之外命令直接拒绝。
- manual step 进入 `manual_required`，等待人工确认。
- timeout 后保留锁，避免并发二次执行。
- failed/manual_required 重试必须重新审批。

## 下一步

- 建立真实 module repository index。
- 为 golden reference module 提供完整升级/回滚证据。
- 将 manifest、plan、lifecycle dry-run/execute 与 rollback drill 产物接入 release workflow/hard gate。
- 将模块治理 OpenAPI 纳入 generated frontend types，并为所有 module OpenAPI fragments 增加通用语义 lint。
- 将 tenant retention policy 与 isolation proof 接入真实 job/验收流程并归档审计证据。
- 在 CI 中部署独立 OIDC IdP，并以浏览器端 authorization code + PKCE 回调作为 SSO E2E 门禁；现有 enterprise matrix 的 SSO 用例仅验证前端 fallback API 路径，不能视为真实 IdP 集成证明。
