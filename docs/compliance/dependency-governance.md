# 依赖与许可证治理

## Policy

依赖治理以 `config/license_policy.yml` 为准。默认策略：

- 允许宽松许可证：0BSD、Apache-2.0、BlueOak、BSD、FTL、ISC、MIT、MPL-2.0、Python-2.0。
- 拒绝强 copyleft 与网络 copyleft：AGPL、GPL、LGPL。
- BUSL、Elastic、SSPL、CC-BY 系列必须人工 review。
- CVSS >= 7.0 的漏洞必须有例外记录或升级计划。

## Release Gate

发布前必须生成并归档，并由 `scripts/check-license-policy.sh` 读取真实 SBOM 进行 allow/deny 和漏洞例外校验：

- `artifacts/compliance/license-policy.json`
- 漏洞例外与许可证审批为只读输入；release workflow 从受保护 `production` environment secrets 注入 runner 临时目录，不发布原始清单。
- `VULNERABILITY_EXCEPTIONS_JSON`：`{"exceptions": [...]}`。
- `LICENSE_REVIEWS_JSON`：`{"reviews": [...]}`。
- `config/license_metadata_overrides.json`：仅修正扫描器遗漏，必须以精确 `purl` 或锚定 `purl_pattern` 绑定版本、owner 与上游证据；禁止组件名匹配，不得覆盖已识别许可证。
- 依赖门禁 SBOM：`artifacts/sbom/backend-dependencies.cdx.json`、`artifacts/sbom/frontend-dependencies.cdx.json`
- 镜像签名 SBOM：`artifacts/sbom/backend.cdx.json`、`artifacts/sbom/frontend.cdx.json`

本地校验：

```bash
SBOM_ARTIFACTS_JSON='["artifacts/sbom/backend-dependencies.cdx.json","artifacts/sbom/frontend-dependencies.cdx.json"]' \
VULNERABILITY_EXCEPTIONS_FILE=/secure/path/vulnerability-exceptions.json \
LICENSE_REVIEWS_FILE=/secure/path/license-reviews.json \
bash scripts/check-license-policy.sh
DEPENDENCY_POLICY_ARTIFACT=artifacts/compliance/license-policy.json \
CHANGE_TICKET=CHG-123 \
RELEASE_APPROVER=platform-approver \
ROLLBACK_DRILL_ARTIFACT=artifact://rollback \
SLO_OBSERVATION_ARTIFACT=artifact://slo \
bash scripts/release-hard-gate.sh
```

## Exception Record

漏洞例外最小字段：

- `purl`
- `version`
- `cve`
- `cvss`
- `status`: `accepted-risk`、`false-positive` 或 `compensating-control`
- `owner`
- `expires_at`
- `approval_id`
- `mitigation`

例外最长 30 天，到期必须关闭、续批或升级依赖。

漏洞例外及 `review_required` 许可证审批必须以精确 `purl + version` 绑定组件，禁止仅凭组件名匹配。许可证审批还须绑定许可证标识，并包含审批编号与证据。元数据 override 不等同审批，亦不得代替漏洞例外。

脚本只读上述审批输入，绝不自动创建、补写或覆盖证据文件。输出仅为 `artifacts/compliance/license-policy.json`。

## Ownership

- Go 依赖：backend platform owner。
- MineAdmin-web 依赖：frontend platform owner。
- 容器基础镜像、GitHub Actions、Helm chart 依赖：platform security owner。
- 运行时第三方服务 SDK：对应业务模块 owner。

## SLA

- Critical 漏洞：24 小时内升级、回滚或下线风险路径。
- High 漏洞：7 天内升级或提交例外。
- Medium 漏洞：30 天内升级或纳入迭代计划。
- EOL 组件：发现后 30 天内给出替换计划，90 天内完成替换或风险接受。
