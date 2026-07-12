# Module Governance API Contract

当前模块治理代码面向平台管理端暴露 **7 个 path / 7 个 route name / 7 个 OpenAPI operation**，不得扩展为第 8 个 endpoint。

所有接口统一使用 HTTP 200 + JSON envelope：

- 成功：`code=200`，字段为 `code`、`message`、`data`
- 业务错误：`code=422`，字段为 `code`、`message`、`data=[]`
- 鉴权、禁用、限流、服务端异常沿用现有 admin 响应模型，仍通过同一 envelope 返回

两个 POST 接口在请求体绑定失败时，当前业务错误文案为 `请求参数错误`。

## Endpoints

| Endpoint | Route Name | Permission | Request | Success `data` |
| --- | --- | --- | --- | --- |
| `GET /admin/platform/module-lifecycle/state` | `platform.module-lifecycle.state` | `platform:moduleLifecycle:list` | 无请求体；无查询条件 | 分页对象：`list` 为模块状态数组，`total` 为总数 |
| `GET /admin/platform/module-lifecycle/runs` | `platform.module-lifecycle.runs` | `platform:moduleLifecycle:list` | 查询参数沿用当前页面 wrapper：`run_key`、`module_id`、`action`、`status`、`owner`、`page`、`page_size` | 分页对象：`list` 为 run 数组，`total` 为总数 |
| `GET /admin/platform/module-lifecycle/steps` | `platform.module-lifecycle.steps` | `platform:moduleLifecycle:log` | 查询参数沿用当前页面 wrapper：`run_key`、`module_id`、`action`、`status`、`page`、`page_size` | 分页对象：`list` 为 step 数组，`total` 为总数 |
| `GET /admin/platform/module-lifecycle/locks` | `platform.module-lifecycle.locks` | `platform:moduleLifecycle:list` | 无请求体；无查询条件 | 分页对象：`list` 为 lock 数组，`total` 为总数 |
| `GET /admin/platform/module-lifecycle/diff` | `platform.module-lifecycle.diff` | `platform:moduleLifecycle:list` | 无请求体；无查询条件 | 分页对象：`list` 为 diff 数组，`total` 为总数 |
| `POST /admin/platform/module-lifecycle/locks/release-stale` | `platform.module-lifecycle.release` | `platform:moduleLifecycle:execute` | JSON 字段：`key?`、`dry_run?`、`confirm_token?`、`reauth_token?`、`approval_id?` | `dry_run` 与 `released[]` |
| `POST /admin/platform/module-lifecycle/execute` | `platform.module-lifecycle.execute` | `platform:moduleLifecycle:execute` | JSON 字段：`action`、`module_id?`、`execute?`、`owner?`、`reason?`、`confirm_token?`、`reauth_token?`、`approval_id?` | `action`、`dry_run`、`owner?`、`reason?`、`items[]` |

## Dry-Run Semantics

- 执行接口在 `execute` 缺省或为 `false` 时执行 dry-run，只返回计划结果，不写入 lifecycle run / step / state。
- stale lock 释放接口在 `dry_run=true` 时只返回当前可释放锁列表，不删除记录，也不消费安全凭据。
- 锁冲突语义、安全凭据校验顺序、审批复用限制与 stale lock 竞态保护遵循当前实现，不在本契约中放宽。

## Business 422 Cases

- 执行接口
  - 缺少或错误的 `confirm_token`
  - 缺少有效的 `reauth_token`
  - 缺少已批准且与资源绑定的 `approval_id`
  - 请求体绑定失败
- stale lock 释放接口
  - 缺少或错误的 `confirm_token`
  - 缺少有效的 `reauth_token`
  - 缺少已批准且与资源绑定的 `approval_id`
  - 请求体绑定失败
