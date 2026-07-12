# Enterprise Release Hard Gate

Production release requires:

- Release workflow is started with `workflow_dispatch`; tag push does not publish production releases.
- GitHub Environment approval on `production`; the workflow records the approver from GitHub deployment approval history, not from a manual text input.
- `CHANGE_TICKET` matching `CHG-*`, `INC-*`, or `RFC-*`.
- When `MODULE_RELEASE_EVIDENCE_DIR` is set, `module-release/evidence-manifest.json` must be generated from the current full Git SHA; every declared artifact must have a matching SHA-256 and size.
- Golden rollback evidence from a completed `upgrade -> smoke -> rollback -> state diff` run. A plan or dry-run is not rollback proof.
- Post-deploy SLO observation with a minimum 30-minute window after the deployment timestamp, deployment UID, and exact image digests.
- Local evidence is JSON with `evidence_type`, current `git_sha`, a digest-bound payload, and machine-readable artifact/query records.
- External evidence URI is accepted only through `RELEASE_EVIDENCE_METADATA_VERIFIER`, which returns the exact URI, object version, SHA-256, immutable-until, verification time, and current Git SHA. A URI string alone is rejected.
- Passing local dependency policy artifact generated from the current release SBOMs. External evidence cannot replace this scan result.
- Workflow does not create placeholder evidence; artifact paths must point to files produced by real pre-release or observation jobs.

Local check:

```bash
CHANGE_TICKET=CHG-123 \
RELEASE_APPROVER=platform-approver \
RELEASE_GIT_SHA="$(git rev-parse HEAD)" \
ROLLBACK_DRILL_ARTIFACT=artifacts/golden-rollback/evidence.json \
SLO_OBSERVATION_ARTIFACT=artifacts/release/slo-evidence.json \
DEPENDENCY_POLICY_ARTIFACT=artifacts/compliance/license-policy.json \
COMPATIBILITY_MATRIX_ARTIFACT=artifacts/module-compatibility-matrix.json \
bash scripts/release-hard-gate.sh
```

Workflow contract:

- Production release metadata must be entered per run through workflow inputs. Repository variables are not accepted as release evidence fallbacks.
- In GitHub Actions, `release_approver` is derived from the run approval API for the protected `production` environment. Local script checks may still pass `RELEASE_APPROVER` explicitly.
- `rollback_drill_artifact` must be the drill's `evidence.json`, created after real guarded API upgrade and rollback executions; the evidence includes lifecycle run keys, state/diff assertions, lock verification, and per-artifact SHA-256 values.
- `slo_observation_artifact` must be produced after deploy by `scripts/collect-release-slo-evidence.sh`; it records Prometheus, Loki, Alertmanager query results and threshold decisions without bearer tokens.
- `scripts/release/generate-module-release-evidence.sh` creates the hash-bound module evidence bundle. `scripts/release/verify-module-release-evidence.sh` must run before the hard gate.
- For immutable external storage, set `RELEASE_EVIDENCE_METADATA_VERIFIER=scripts/release/verify-evidence-metadata.sh` and provide the verified metadata through `RELEASE_EVIDENCE_METADATA_FILE`; the verifier rejects incomplete object metadata.
- `DEPENDENCY_POLICY_ARTIFACT` must point to the local license/vulnerability policy result generated from the current release SBOMs and `config/license_policy.yml`.
- `artifacts/release-predeploy-gate.json` proves change approval, rollback drill, dependency policy, compatibility, and module evidence passed before production mutation. `artifacts/release-hard-gate.json` adds immutable post-deploy SLO evidence; both are uploaded with the release.
