# 发布与回滚演练记录模板

用于生产发布审计归档。每次正式发布、回滚演练或真实回滚后复制本模板，保存到团队约定的变更记录系统。

## 发布记录

- 版本：
- 发布时间：
- 发布负责人：
- 审批单 / 变更单：
- Git tag：
- Git SHA：
- 后端镜像 digest：
- 前端镜像 digest：
- SBOM artifact：
- cosign 验签输出：
- Helm 渲染 artifact：
- K8s dry-run artifact：
- 目标集群 dry-run：skipped / passed / failed
- staging preflight artifact：
- release manifest artifact：
- module manifest artifact：
- module state artifact：
- module upgrade plan artifact：
- module rollback plan artifact：
- 证据负责人：
- 跳过项 owner / reason / follow-up：
- 未实跑原因：无 / 无目标环境 / 无发布窗口 / 无 kubeconfig / 其他
- 数据库迁移：无 / expand / deploy / backfill / contract
- 备份确认：备份文件、时间、恢复点
- 发布命令：

```bash
helm upgrade --install goravel-mine deploy/helm/goravel-mine \
  --namespace goravel-mine --create-namespace \
  --set image.repository=ghcr.io/<owner>/<repo>/backend \
  --set image.tag=v0.1.0 \
  --set secret.existingSecret=goravel-mine-secret
```

- 发布后 smoke test：
- 发布后 30 分钟 SLO 观察：
- Prometheus / Grafana / Loki / Alertmanager 验收：
- 发布证据清单：[docs/deployment/evidence-checklist.md](./evidence-checklist.md)
- 回滚演练记录链接：
- 结论：

## 回滚演练记录

- 演练时间：
- 演练负责人：
- 起始版本：
- 目标回滚版本：
- 数据库是否涉及不可逆变更：
- 回滚前备份：
- 未实跑原因：无 / 无目标环境 / 无发布窗口 / 无 kubeconfig / 其他
- 回滚命令：

```bash
helm rollback goravel-mine <REVISION> --namespace goravel-mine
kubectl -n goravel-mine rollout status deployment/goravel-mine
```

- 验证命令：

```bash
kubectl -n goravel-mine get pods -l app.kubernetes.io/name=goravel-mine
kubectl -n goravel-mine run smoke-curl --rm -i --restart=Never --image=curlimages/curl -- \
  curl -fsS http://goravel-mine/health/ready
```

- 回滚耗时：
- 用户影响：
- 发现问题：
- 修复项：
- 结论：
