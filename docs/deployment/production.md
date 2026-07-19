# 生产部署指南

本文档覆盖 Goravel 后端生产部署基线：非 root 镜像、prod Compose、Kubernetes/Helm、健康检查、滚动升级、迁移发布、备份与恢复、资源限制。

## 镜像

`Dockerfile` 使用 multi-stage 构建，runtime 镜像运行在 uid/gid `10001`，并只预创建 `/www/storage` 可写目录。生产环境不要把 `.env` 打进镜像，密钥只通过 Compose env、Kubernetes Secret 或外部密钥系统注入。

构建：

```bash
docker build -t ghcr.io/example/goravel-mine:2026.07.06 .
```

MineAdmin-web 前端镜像使用 `MineAdmin-web/Dockerfile` 构建，runtime 基于 Alpine Nginx，容器内以非 root uid/gid `10002` 监听 `8080`。生产入口已包含 SPA fallback、安全响应头、`index.html` no-store、hash 静态资源 immutable 缓存，以及 gzip/brotli 预压缩静态文件服务。部署时将外部 80/443 反向代理或端口映射到容器 `8080`。

```bash
docker build -t ghcr.io/example/goravel-mine-frontend:2026.07.06 MineAdmin-web
```

## Compose 生产部署

复制生产变量模板并填写强随机密钥：

```bash
cp .env.production.example .env.production
docker compose --env-file .env.production -f docker-compose.prod.yml up -d --build
```

应用容器启用：

- 非 root `10001:10001`
- `read_only: true`
- `cap_drop: [ALL]`
- `no-new-privileges`
- `/tmp` tmpfs
- `/www/storage` 持久卷
- `/health/ready` healthcheck
- CPU/内存 requests/limits

生产 Compose 默认 `QUEUE_CONNECTION=redis`。`goravel` Web 容器设置 `QUEUE_WORKER_ENABLED=false`，避免 Web 副本消费队列；`goravel-queue-worker` 独立消费 Redis queue，并通过 `QUEUE_SERVICE_WORKER_ENABLED=true`、`QUEUE_SERVICE_CONCURRENT=4` 覆盖自身 worker 开关与并发。若只想本地兼容同步执行，可显式覆盖 `QUEUE_CONNECTION=sync`，但生产不建议使用。

生产变量模板默认启用 `SECURITY_ENTERPRISE=true`。该 profile 在未显式覆盖单项 `SECURITY_*` 时启用更硬的安全默认值：TOTP MFA 能力开启、密码最小长度 12、要求大小写/数字/特殊字符、密码历史 5 次、密码 90 天过期、CSRF 开启。框架兼容模板仍保持 `SECURITY_ENTERPRISE=false` 与弱兼容默认值；生产如需放宽某项，显式设置对应 `SECURITY_PASSWORD_*`、`SECURITY_MFA_*` 或 `SECURITY_CSRF_*`。

MineAdmin-web 生产构建默认启用 CSRF Token 自动注入，与后端 enterprise profile 匹配。若后端显式关闭 `SECURITY_CSRF_ENABLED=false`，前端构建也应显式设置 `VITE_SECURITY_CSRF=false`。

CSRF 自动注入会携带 `csrf_token` Cookie。跨域部署时，生产后端必须设置 `CORS_SUPPORTS_CREDENTIALS=true`，并将 `CORS_ALLOWED_ORIGINS` 配置为前端完整 Origin，不能使用 `*`。

CSRF 可信 Origin 默认跟随 `APP_URL`。若前端访问域名与 `APP_URL` 不同，应显式设置 `SECURITY_CSRF_TRUSTED_ORIGINS` 为逗号分隔的完整 Origin 列表。

启用 enterprise profile 后，系统默认初始密码 `123456` 不再满足密码策略。上线前应为内置管理员设置符合策略的强密码，或通过显式 `password` 创建用户；重置密码能力若仍依赖默认初始密码，会按策略返回业务错误。

数据库备份示例：

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml exec postgres \
  sh -c 'pg_dump -U "$DB_USERNAME" -d "$DB_DATABASE" -Fc' > backups/postgres/goravel-mine-$(date +%Y%m%d%H%M%S).dump
```

恢复前必须停止写入流量并确认目标库：

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml exec -T postgres \
  sh -c 'pg_restore -U "$DB_USERNAME" -d "$DB_DATABASE" --clean --if-exists' < backups/postgres/backup.dump
```

## Kubernetes 部署

基础清单位于 `deploy/k8s/`。默认假设 PostgreSQL/Redis 由集群内服务或外部托管服务提供，应用通过 `DB_HOST`、`REDIS_HOST` 连接。应用 PVC 默认 `ReadWriteMany`，用于支撑多副本滚动升级；若集群只有 `ReadWriteOnce` 存储，请改用对象存储承载上传文件，或把副本数降为 1。

首次部署：

```bash
kubectl apply -f deploy/k8s/namespace.yaml
: "${APP_KEY:?set APP_KEY}"
: "${JWT_SECRET:?set JWT_SECRET}"
: "${DB_PASSWORD:?set DB_PASSWORD}"
: "${REDIS_PASSWORD:?set REDIS_PASSWORD}"
: "${OBS_METRICS_TOKEN:?set OBS_METRICS_TOKEN}"
kubectl -n goravel-mine create secret generic goravel-mine-secret \
  --from-literal=APP_KEY="$APP_KEY" \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=DB_PASSWORD="$DB_PASSWORD" \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=OBS_METRICS_TOKEN="$OBS_METRICS_TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f deploy/k8s/configmap.yaml
kubectl apply -f deploy/k8s/pvc.yaml
kubectl apply -f deploy/k8s/service.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/queue-worker-deployment.yaml
kubectl apply -f deploy/k8s/ingress.yaml
```

发布新镜像：

```bash
kubectl -n goravel-mine set image deployment/goravel-mine app=ghcr.io/example/goravel-mine:2026.07.06
kubectl -n goravel-mine rollout status deployment/goravel-mine
```

回滚：

```bash
kubectl -n goravel-mine rollout undo deployment/goravel-mine
kubectl -n goravel-mine rollout status deployment/goravel-mine
```

## Helm 部署

Chart 位于 `deploy/helm/goravel-mine/`。

先创建 Helm 引用的 Secret：

```bash
kubectl create namespace goravel-mine --dry-run=client -o yaml | kubectl apply -f -
: "${APP_KEY:?set APP_KEY}"
: "${JWT_SECRET:?set JWT_SECRET}"
: "${DB_PASSWORD:?set DB_PASSWORD}"
: "${REDIS_PASSWORD:?set REDIS_PASSWORD}"
: "${OBS_METRICS_TOKEN:?set OBS_METRICS_TOKEN}"
kubectl -n goravel-mine create secret generic goravel-mine-secret \
  --from-literal=APP_KEY="$APP_KEY" \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=DB_PASSWORD="$DB_PASSWORD" \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=OBS_METRICS_TOKEN="$OBS_METRICS_TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -
```

渲染检查：

```bash
helm template goravel-mine deploy/helm/goravel-mine \
  --set image.repository=ghcr.io/example/goravel-mine \
  --set image.tag=2026.07.06 \
  --set secret.existingSecret=goravel-mine-secret
```

HA profile render and policy assertion:

```bash
mkdir -p artifacts
helm template goravel-mine deploy/helm/goravel-mine \
  --namespace goravel-mine \
  --set image.repository=ghcr.io/example/goravel-mine \
  --set image.tag=2026.07.06 \
  --set secret.existingSecret=goravel-mine-secret \
  -f deploy/helm/goravel-mine/values-ha-test.yaml \
  > artifacts/helm-ha.yaml
bash scripts/verify-helm-ha.sh artifacts/helm-ha.yaml
```

`ha.topology.webAntiAffinity=required` 要求至少三个可调度节点方能承载默认三副本；节点不足时改为 `preferred`，并记录降低故障域隔离的风险。标准 `NetworkPolicy` 不能按 FQDN 放行动态 IdP/对象存储地址，生产集群须在 `ha.networkPolicy.destinations.*.cidrs` 固定网段，或采用具 FQDN policy 的 CNI。

CI 中通过 `scripts/verify-deploy-manifests.sh` 固化以下门禁：

- `helm lint deploy/helm/goravel-mine`
- `helm template goravel-mine deploy/helm/goravel-mine`
- `helm template` ops profile：`migration.enabled=true`、`backup.enabled=true`
- CI 中启动临时 Kind API server 后执行 `kubectl apply --dry-run=server --validate=strict -f deploy/k8s/`
- CI 中启动临时 Kind API server 后执行 `kubectl apply --dry-run=server --validate=strict -f` Helm 默认渲染与 ops profile 渲染结果

安装或升级：

```bash
helm upgrade --install goravel-mine deploy/helm/goravel-mine \
  --namespace goravel-mine --create-namespace \
  --set image.repository=ghcr.io/example/goravel-mine \
  --set image.tag=2026.07.06 \
  --set secret.existingSecret=goravel-mine-secret
```

生产建议使用 `secret.existingSecret` 接入外部密钥管理，不在命令行传密钥。若需要由 Chart 创建 Secret，必须显式设置 `secret.create=true` 并提供真实 `secret.data`。

Helm 默认 `app.env.QUEUE_CONNECTION=redis`、`app.env.QUEUE_WORKER_ENABLED=false`，并启用 `worker.enabled=true`。队列 worker Deployment 通过 `worker.env.QUEUE_WORKER_ENABLED=true`、`worker.env.APP_AUTO_RUN=false`、`worker.env.SCHEDULER_ENABLED=false` 只运行 queue runner，不启动 HTTP/scheduler；按任务量调整 `worker.replicaCount` 与 `worker.env.QUEUE_CONCURRENT`。

### Helm HA 基线

HA 资源由 `ha.enabled` 总开关控制，默认关闭以保留既有部署形态。生产设置应在检查存储可支持多副本后启用：HPA 为 Web Deployment 保持至少 3 个副本并按 CPU/内存扩缩容，PDB 保持至少 2 个可用 Web Pod，Web/worker 分别使用 required/preferred pod anti-affinity，且同时按 zone 与 hostname（`maxSkew: 1`）分散调度。

`ha.serviceAccountName` 和 `ha.priorityClassName` 只引用集群中已经存在的资源；Chart 不创建 cluster-scoped RBAC 或 PriorityClass。可用 `ha.hpa.enabled`、`ha.pdb.enabled`、`ha.topology.enabled` 和 `ha.networkPolicy.enabled` 独立关闭子项；关闭 `ha.enabled` 会移除 Task 22 新增资源和调度设置，恢复原有 Deployment 形态。

启用 `ha.networkPolicy.enabled` 后，Chart 先为本 release 的 Web、worker、migration、backup Pod 创建 ingress/egress default-deny，再显式允许：Ingress controller 到 Web HTTP、Prometheus 到 metrics、DNS、PostgreSQL、Redis、OTEL collector、对象存储与 SSO。PostgreSQL、Redis、OTEL、Ingress 和 Prometheus 通过可配置 namespace/pod selector 选定；对象存储和 SSO 仅支持 CIDR (`ha.networkPolicy.destinations.*.cidrs`)。标准 Kubernetes NetworkPolicy 不能按 FQDN 放行，因此必须为这些外部服务维护稳定出口 CIDR，或使用支持 FQDN policy 的 CNI 并在 CNI 层配置相应规则。空 CIDR 列表会保留 default-deny，不会放通公网 HTTPS。

## 队列可靠性基线

生产基线使用 Redis queue，失败任务落库到 `failed_jobs`，后台提供失败任务查询、重试和丢弃入口。Web 进程不消费队列，worker 进程独立扩缩容，避免 Web 副本数量变化直接放大任务并发。

Outbox dispatcher 默认随队列 worker 启动：`QUEUE_OUTBOX_ENABLED=true`、`QUEUE_OUTBOX_INTERVAL_SECONDS=5`、`QUEUE_OUTBOX_BATCH=20`、`QUEUE_OUTBOX_OWNER=queue-outbox-runner`。Web 进程即使继承这些变量，也会因 `QUEUE_WORKER_ENABLED=false` 不运行 outbox runner。

可靠性约定：

- 所有可重试任务必须实现 `ShouldRetry(err error, attempt int) (bool, time.Duration)`，使用指数退避并设置最大尝试次数。
- 任务参数只传稳定 ID、租户标识、时间戳或小型标量；不要传整块业务对象或敏感字段。
- 任务 `Handle` 必须可幂等：重复执行不得造成重复扣款、重复授权、重复外发不可撤销消息。
- 跨 DB 写入与队列投递的一致性场景必须走 outbox：先在同一 DB 事务内写业务表与 outbox，再由独立 dispatcher 投递队列。
- 同一业务键同一时间只能有一个 worker 执行时，必须使用 cache lock 或 DB 唯一约束，不依赖“队列只投一次”。
- 失败任务超过阈值时先保留 `failed_jobs` 证据，再决定 `queue:retry` 或丢弃，不直接清表。

运维命令：

```bash
# Compose 查看 worker 日志
docker compose --env-file .env.production -f docker-compose.prod.yml logs -f goravel-queue-worker

# Helm / Kubernetes 查看 worker
kubectl -n goravel-mine get deploy,pods -l app.kubernetes.io/component=queue-worker
kubectl -n goravel-mine logs deploy/goravel-mine-queue-worker --since=30m

# 失败任务查询与重试
kubectl -n goravel-mine exec deploy/goravel-mine -- /www/main artisan queue:failed
kubectl -n goravel-mine exec deploy/goravel-mine -- /www/main artisan queue:retry <uuid>
kubectl -n goravel-mine exec deploy/goravel-mine -- /www/main artisan queue:retry --connection=redis --queue=default
```

DLQ 处理约定：`failed_jobs` 即当前死信来源。on-call 应记录 UUID、signature、exception、payload 摘要、影响租户与处理动作；可恢复错误走重试，不可恢复错误在业务修复或数据修正后重试，确认无价值后再通过后台丢弃。

## 健康检查

- `/health/live`：只判断应用进程与 HTTP server 是否可响应，适合 liveness/startup。
- `/health/ready`：检查数据库与 cache 可用性，适合 readiness 与负载摘除。

Kubernetes 中 `readinessProbe` 失败时 Pod 不接新流量；`livenessProbe` 连续失败才重启进程。避免把依赖检查放进 liveness，否则数据库短暂抖动会导致应用被反复重启。

## 可观测性运营闭环

生产环境默认打开 `/metrics`，并通过 `OBS_METRICS_TOKEN` 要求 Prometheus 使用 bearer token 抓取。可观测性闭环材料位于 `docs/observability/` 与 `deploy/observability/`：

- Grafana dashboard：`deploy/observability/grafana-dashboard.json`
- Logs / audit dashboard：`deploy/observability/grafana-logs-dashboard.json`、`deploy/observability/grafana-audit-dashboard.json`
- Prometheus alerts：`deploy/observability/prometheus-rules.yaml`、`deploy/observability/external-alert-rules.yaml`、`deploy/observability/kube-metrics-recording-rules.yaml`
- Loki alerts：`deploy/observability/loki-alert-rules.yaml`
- Alertmanager route：`deploy/observability/alertmanager-route.yaml`
- Prometheus scrape / ServiceMonitor：`deploy/observability/prometheus-scrape.yaml`、`deploy/observability/servicemonitor.yaml`
- PostgreSQL / Redis exporter：`deploy/observability/postgres-exporter.yaml`、`deploy/observability/redis-exporter.yaml`
- 日志采集示例：`deploy/observability/promtail.yaml`
- OpenTelemetry Collector：`deploy/observability/otel-collector.yaml`
- SLO 与错误预算：`docs/observability/slo.md`
- on-call runbook：`docs/observability/on-call-runbook.md`

上线前确认：

```bash
curl -fsS -H "Authorization: Bearer $OBS_METRICS_TOKEN" https://admin.example.com/metrics
```

当前指标支持 request rate、error ratio、P95/P99 latency、in-flight、slow request、slow SQL、Go runtime、platform DB pool、scheduler heartbeat、queue failed jobs、queue outbox backlog 与 uptime。

## 滚动升级策略

Deployment 使用：

- `replicas: 2`
- `maxUnavailable: 0`
- `maxSurge: 1`
- `preStop: sleep 10`
- `terminationGracePeriodSeconds: 30`
- readiness 通过后才接流量

发布流程：

1. 构建并推送不可变 tag。
2. 先做备份。
3. 对有 schema 变更的版本先跑 migration Job。
4. 更新 Deployment image。
5. 等待 `rollout status` 成功。
6. 调用 `/health/ready`、关键登录/API smoke test。
7. 观察 error rate、latency、slow SQL、业务日志。

## 迁移发布策略

优先采用 expand-contract：

1. Expand：新增表/列/索引，保持旧代码可运行。
2. Deploy：发布同时兼容旧/新 schema 的应用。
3. Backfill：批量填充历史数据。
4. Switch：切换读写路径。
5. Contract：确认无旧版本运行后删除旧列/旧表。

Compose：

```bash
docker compose --env-file .env.production -f docker-compose.prod.yml run --rm --entrypoint /www/main goravel artisan migrate
```

Kubernetes：

```bash
kubectl -n goravel-mine delete job goravel-mine-migrate --ignore-not-found
kubectl apply -f deploy/k8s/migration-job.yaml
kubectl -n goravel-mine wait --for=condition=complete job/goravel-mine-migrate --timeout=120s
```

Helm：

```bash
helm upgrade --install goravel-mine deploy/helm/goravel-mine \
  --namespace goravel-mine \
  --reuse-values \
  --set migration.enabled=true \
  --set migration.hook=true
```

不可逆迁移必须配套备份、回滚说明与演练记录。

## 可审计发布闭环

正式版本通过 `.github/workflows/release.yml` 的手动 `workflow_dispatch` 发布。发布时必须逐次填写版本号、变更单、回滚演练证据和 SLO 观察证据；审批人从 GitHub `production` Environment approval 记录读取，tag push 不会触发生产发布，避免复用仓库级固定证据。

```bash
gh workflow run release.yml \
  -f version=v0.1.0 \
  -f change_ticket=CHG-123 \
  -f rollback_drill_artifact=artifact://change/CHG-123/rollback-drill \
  -f slo_observation_artifact=artifact://change/CHG-123/slo-observation
```

发布流水线产物：

- CI quality gate：复用 `.github/workflows/ci.yml`，通过后才构建、推送和签名镜像
- 后端镜像：`ghcr.io/<owner>/<repo>/backend:<version>`
- 前端镜像：`ghcr.io/<owner>/<repo>/frontend:<version>`
- CycloneDX SBOM：`backend.cdx.json`、`frontend.cdx.json`
- cosign keyless 镜像签名与 SBOM attestation
- Helm package 与 Helm 渲染结果
- Kind server dry-run 输出
- 目标集群 server dry-run 状态：默认 required；只有手动选择 `target_dry_run=skip` 才跳过
- `release-manifest.json`：记录版本、Git SHA、镜像 digest、签名与 dry-run artifact

### 目标集群 dry-run Secret

`PROD_KUBE_CONFIG_B64` 是生产或预生产 kubeconfig 的 Base64 文本，仅用于 release workflow 对目标集群执行 `kubectl apply --dry-run=server`。没有目标环境时默认会失败；只有手动选择 `target_dry_run=skip` 时，workflow 才会把目标集群 dry-run 记录为 skipped。

workflow 只在目标集群 dry-run step 注入 `PROD_KUBE_CONFIG_B64`，其他 build、SBOM、签名、发版步骤不能读取该 kubeconfig。

若 `PROD_KUBE_CONFIG_B64` 缺失且未显式选择 `target_dry_run=skip`，workflow 会在构建镜像前失败，避免产出缺少目标集群校验证据的正式 release。

生成 Secret 值：

```bash
base64 < ~/.kube/prod-goravel-mine.yaml | tr -d '\n'
```

GitHub 仓库配置：

1. Settings -> Secrets and variables -> Actions -> New repository secret。
2. Name 填 `PROD_KUBE_CONFIG_B64`。
3. Secret 填上一步生成的单行 Base64 文本。

建议 kubeconfig 绑定最小权限 ServiceAccount，只授予 `goravel-mine` namespace 内 Deployment、Service、ConfigMap、Secret 引用校验、PVC、Job、CronJob、Ingress 等发布所需资源的 server dry-run 权限。

验签示例：

```bash
IMAGE_REF=ghcr.io/<owner>/<repo>/backend@sha256:<digest>
cosign verify \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --certificate-identity-regexp='https://github.com/<owner>/<repo>/.github/workflows/release.yml@refs/(tags|heads)/.*' \
  "$IMAGE_REF"
```

验证 SBOM attestation：

```bash
cosign verify-attestation \
  --type cyclonedx \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  --certificate-identity-regexp='https://github.com/<owner>/<repo>/.github/workflows/release.yml@refs/(tags|heads)/.*' \
  "$IMAGE_REF"
```

生产发布前必须归档 GitHub Release 中的 `release-manifest.json`、SBOM、cosign 验签输出、Helm 渲染结果、dry-run 输出、备份记录和 smoke test 结果。

发布记录与回滚演练记录模板见 `docs/deployment/release-record-template.md`。

### 无目标环境时的静态审计

若当前没有可用 Kubernetes 集群、生产 kubeconfig 或发布窗口，不执行真实部署、真实回滚、目标集群 server dry-run、线上 smoke test。此时只保留静态审计闭环：

- 审阅 `.github/workflows/release.yml` 的权限、触发条件、镜像 tag、SBOM、签名、验签和 artifact 清单。
- 审阅 `scripts/verify-deploy-manifests.sh` 的 Helm 渲染与 K8s dry-run 参数。
- 审阅 Helm 默认渲染与 ops profile 覆盖范围，确保 migration Job 与 backup CronJob 被纳入未来 CI 校验。
- 在发布记录中标注“未实跑原因：无目标环境”，并把目标集群 server dry-run、真实发布、真实回滚、线上 smoke test 标为待执行。

## 备份与恢复

Kubernetes 示例包含 `deploy/k8s/backup-cronjob.yaml` 和 `deploy/k8s/backup-pvc.yaml`，每天 02:30 执行 `pg_dump -Fc`，保留 7 天。生产环境建议把备份同步到对象存储，并定期做恢复演练。

启用：

```bash
kubectl apply -f deploy/k8s/backup-pvc.yaml
kubectl apply -f deploy/k8s/backup-cronjob.yaml
```

手动触发：

```bash
kubectl -n goravel-mine create job --from=cronjob/goravel-mine-postgres-backup backup-manual-$(date +%Y%m%d%H%M%S)
```

恢复：

1. 暂停入口流量或缩容应用。
2. 记录当前镜像 tag 与数据库版本。
3. 使用 `pg_restore --clean --if-exists` 恢复到目标库。
4. 运行只读校验 SQL 与应用 smoke test。
5. 恢复入口流量。

## 资源限制

默认应用 requests/limits：

- requests: `cpu=250m`, `memory=128Mi`
- limits: `cpu=1`, `memory=512Mi`

迁移与备份 Job 独立设置较低资源。上线后根据 p95 latency、GC、DB 连接数与容器 OOM 记录调优。

## 验证清单

```bash
tests/backend/test.sh ./...
docker compose --env-file .env.production -f docker-compose.prod.yml config
scripts/verify-deploy-manifests.sh
```

`scripts/verify-deploy-manifests.sh` 默认需要可用 Kubernetes API server；CI 会启动临时 Kind 集群执行 server dry-run。若没有目标环境或本地未安装 Docker、Helm、kubectl，不要为了验证而临时触碰生产环境；仅保留已执行的静态审计结果，并在交付说明中明确未执行项与原因。
