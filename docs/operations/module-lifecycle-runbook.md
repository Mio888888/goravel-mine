# 模块生命周期运维 Runbook

## 替代模块移除

弃用模块移除前先检查 manifest 中的 replacement plan。计划必须包含五阶段、六条迁移/验证/切流/回滚命令及 policy hash。执行顺序固定为安装替代模块、data/config/permission migration、dual-run validation、cutover、旧模块 uninstall；cutover 后同步失败自动运行 rollback，命令晚完成则保留锁并进入 reconciliation。

`end_of_support` 前的紧急移除使用 `module.replacement.emergency-remove` resource `module-replacement:<old>:emergency-remove:<new>` 申请独立审批和 re-auth。EOS 后使用常规 `module.lifecycle.execute`。两者均不能用另一 policy 的审批替代。

## 执行详情

平台入口：`/platform-system/module-lifecycle`。

必看视图：

- State：manifest、持久化状态、最近动作、最近错误。
- Runs：按 module/action/status/owner 查执行历史。
- Steps：查看 step 级 command、stdout/stderr 摘要、error。
- Locks：查看 lifecycle lock、执行过期锁 dry-run 或受控释放。
- Diff：对比 manifest 与 `module_state`，识别 missing_state、version_mismatch、enabled_mismatch、last_error。

## 锁释放

只释放已过期锁。默认先 dry-run：

```bash
POST /admin/platform/module-lifecycle/locks/release-stale
{ "dry_run": true }
```

真实释放必须有：

- `confirm_token`: `release-stale-locks`
- `resource`: 全量释放使用 `module-lifecycle:stale-locks:all`；指定 key 使用 `module-lifecycle:stale-locks:<key>`
- `reauth_token`
- `approval_id`

锁未过期时不得人工删除；先确认执行节点是否仍存活。

## 失败重试

重试前检查：

- run 状态为 `failed`、`manual_required` 或锁已过期导致的 `lock_blocked`。
- Steps 中错误已定位并修复。
- approval 必须重新申请；真实执行会消费审批 ID，禁止复用。
- `module-lifecycle/diff` 无未解释 drift。

重试使用同一 action/module 先 dry-run，再 execute。

## 审计证据

每次生产执行需归档：

- run idempotency key
- step timeline
- stdout/stderr 摘要或外部归档 URI
- approval id、re-auth 证明、operator
- manifest/state diff
- rollback drill 或 rollback plan

## 风险提示

- `requires_restart=true`：进入维护窗口。
- `supports_hot_disable=false`：禁用需变更窗口。
- `breaking_change_policy` 非空：按模块声明执行人工确认。
- `last_error` 非空：禁止跳过 root cause。
