# 发布证据清单

本清单用于正式发布、回滚演练、模块上线验收归档。没有真实环境或凭证时，不得标记端到端通过，只能记录静态证据与未实跑原因。

## 模块证据

发布前生成并归档：

```bash
go run . artisan module:manifest:check --artifacts --frontend
go run . artisan module:manifest:export --target=storage/framework/module-manifest.json
go run . artisan module:state > storage/framework/module-state.json
go run . artisan module:plan --action=upgrade > storage/framework/module-upgrade-plan.json
go run . artisan module:lifecycle --action=upgrade > storage/framework/module-upgrade-dry-run.json
```

涉及回滚或卸载演练时补：

```bash
go run . artisan module:plan --action=rollback > storage/framework/module-rollback-plan.json
go run . artisan module:plan --action=uninstall > storage/framework/module-uninstall-plan.json
go run . artisan module:lifecycle --action=rollback > storage/framework/module-rollback-dry-run.json
```

CLI 禁止真实执行。生产执行须经平台管理 API 或模块治理 UI，提交 operator 绑定的 confirm token、re-auth token、一次性 approval ID、owner 与 reason，并归档响应、run/step 记录及执行后的 `module:state`：

```bash
curl -sS -X POST "$APP_URL/admin/platform/module-lifecycle/execute" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"action":"upgrade","module_id":"<module-id>","execute":true,"owner":"<owner>","reason":"<release-id>","confirm_token":"<module-id>:upgrade","reauth_token":"<reauth-token>","approval_id":"<approval-id>"}' \
  > storage/framework/module-upgrade-run.json
go run . artisan module:state > storage/framework/module-state-after-upgrade.json
```

归档要求：

- `module-manifest.json` 与发布 Git SHA 对齐。
- `module-state.json` 记录启用 / 禁用模块与原因；执行后还应包含 persisted lifecycle 状态。
- lifecycle plan 中每个模块都有 `command`、`destructive_check`、`breaking_change_policy`。
- lifecycle run 中每个执行模块都有 `idempotency_key`、`status`、`owner`、`reason`。
- 禁用模块必须有 owner、reason、恢复条件。

## 发布前证据

- CI run 链接。
- 后端测试结果。
- 前端 `yarn lint:tsc` 或未实跑原因。
- Helm render artifact。
- Kubernetes dry-run artifact。
- 镜像 digest。
- SBOM artifact。
- cosign 验签输出。
- 数据库迁移策略：无 / expand / deploy / backfill / contract。
- 备份恢复点。

## Staging Smoke

有 staging 环境时必须归档：

```bash
APP_URL=https://staging-api.example.com scripts/observability-runtime-smoke.sh
```

可观测性 strict 验收：

```bash
OBS_SMOKE_STRICT=true \
APP_URL=https://staging-api.example.com \
PROM_URL=https://prometheus.example.com \
LOKI_URL=https://loki.example.com \
ALERTMANAGER_URL=https://alertmanager.example.com \
GRAFANA_URL=https://grafana.example.com \
GRAFANA_TOKEN=<grafana-token> \
scripts/observability-runtime-smoke.sh
```

无环境时记录：无目标环境 / 无 kubeconfig / 无凭证 / 无发布窗口。

## 发布后证据

- `/health/ready` smoke 输出。
- `/metrics` target `UP` 截图或 Prometheus query 链接。
- Grafana dashboard 链接。
- Loki request_id 查询链接。
- Alertmanager route 状态。
- 发布后 30 分钟 SLO 观察。
- 错误预算状态。
- rollback command 与最近演练链接。

## 安全与队列证据

- 权限变更审批单。
- 敏感操作二次认证策略确认。
- 密钥轮换检查：

```bash
go run . artisan security:rotate-check
```

- WORM / 审计归档确认。
- 队列积压、failed jobs、outbox 指标。
- DLQ 重放或丢弃审批记录。
