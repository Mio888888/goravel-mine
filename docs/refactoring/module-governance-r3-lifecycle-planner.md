# Module Governance R3 Lifecycle Planner

**Captured:** 2026-07-10
**Base:** R2 over commit `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Go 1.26.4

## Outcome

R3 makes a pure `lifecyclePlanner` the single authority for valid actions, lifecycle order, one-module filtering, selected command, destructive check, idempotency key, dependency version policy and multi-module execute preflight. `Service.LifecyclePlan` and `LifecycleService.Execute` keep their public signatures and map the shared plan into their existing DTOs.

Production boundaries:

- `lifecycle_planner.go`: pure plan values, order/filter logic and public DTO adapters.
- `lifecycle_command_policy.go`: command normalization, manual detection and allowlist validation.
- `lifecycle_version_policy.go`: dependency version validation and semantic comparison.
- `service.go`: static lifecycle-plan facade, reduced from 135 to 92 lines.
- `lifecycle_service.go`: execution orchestration only, reduced from 1,539 to 1,282 lines.

New production files remain below 300 lines; new functions remain below 50 lines.

## Preserved Contracts

- Four action values, forward/reverse dependency order, module filtering and `module not found` error text remain unchanged.
- Disabled modules remain skipped with their prior reason and status.
- Command, destructive check and idempotency key values are shared between static plans and dry-run results.
- Version-constraint error text and traversal order remain unchanged.
- Multi-module execute rejects manual or disallowed commands before any store or runner call; single-module execute still records the existing runtime failure/status.
- CLI flags, API payloads, JSON tags, plan/result ordering and all R0 goldens remain unchanged.
- Duplicate module IDs remain invalid, but rollback/uninstall filtering still selects the first entry after reverse ordering, matching R2 behavior.

## Deliberately Retained Facade Differences

R3 centralizes rule implementations without changing when each existing facade invokes them:

- `Service.LifecyclePlan` does not trim action input; `LifecycleService.Execute` trims it before planning. This preserves the prior `module:plan` versus lifecycle dry-run behavior.
- `Service.LifecyclePlan` remains a static projection and does not run dependency version validation. CLI lifecycle dry-run and Admin API dry-run still validate versions before producing results.
- Default action selection remains at CLI/Admin adapters: blank flags/payloads become `upgrade` before planner entry.

Regression tests name these boundaries explicitly so later contract changes require deliberate review rather than accidental convergence.

## Duplication Result

Command: `go run ./scripts/module-governance-baseline --root . --format json`

| Metric | R0 | R2 | R3 | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Files | 51 | 62 | 66 | +15 |
| Physical lines | 12,991 | 13,247 | 13,618 | +627 |
| Clone blocks | 38 | 36 | 36 | -2 (-5.26%) |
| Duplicated source lines | 687 | 647 | 647 | -40 (-5.82%) |

The lexical scanner is unchanged from R2 because the removed lifecycle duplicates were semantic rather than token-identical. S-003 is complete: one planner/policy set now drives static plan and executor dry-run construction. R7 retains the cumulative 30% duplicated-line target.

## Performance Result

Command: `go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=5`

| Benchmark | R0 median ns/op | R2 median ns/op | R3 median ns/op | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Manifest | 53,194 | 53,592 | 53,997 | +1.51% |
| Compatibility | 47,371 | 47,156 | 47,780 | +0.86% |
| Plan | 3,930 | 1,339 | 1,789 | -54.48% |
| DryRun | 9,911 | 4,601 | 3,308 | -66.62% |

Five isolated R3 samples in ns/op:

- Manifest: 53,803; 53,997; 53,881; 54,207; 54,437.
- Compatibility: 47,663; 47,780; 47,717; 47,913; 48,026.
- Plan: 1,826; 1,818; 1,755; 1,789; 1,779.
- DryRun: 3,259; 3,347; 3,294; 3,308; 3,418.

Plan allocates 6,632 B/op and dry-run 6,504 B/op after the planner switched from copying full module states to immutable pointers. All representative medians remain within the R0 maximum regression of 10%.

## Review

External review found no Critical issue and confirmed batch/manual/status/error compatibility. It raised three Important concerns about action trimming, static-plan version validation and missing failure-path tests. The first two suggested converging existing facade behavior, which would violate R3's zero-behavior-change gate; they were retained and documented. The valid test-gap concern was fixed with explicit action-normalization and version-validation-boundary tests.

Main-thread review also found and fixed one hidden compatibility case: with duplicate IDs, rollback/uninstall module filtering must occur after reverse ordering.

## Verification

R3 verification commands:

```bash
go test ./app/modules ./app/modules/platformobservability ./app/modulecatalog ./app/console/commands ./scripts/module-governance-baseline ./tests/unit -count=1
go test ./tests/feature/admin -run 'TestModuleLifecycleTestSuite|TestModuleLifecycle(ReadModelQueryBudgets|ServiceAPIShapes|ActionResultShapes)' -count=1
go test ./tests/unit -run '^TestModuleGovernance(Manifest|Compatibility|State|LifecyclePlan)Contract$' -count=1
go run . artisan module:manifest:check --artifacts --frontend
go test -race ./app/modulecatalog -count=1
go vet ./app/modulecatalog ./app/console/commands ./app/modules ./app/moduleboot ./bootstrap
go test -p 1 ./...
go run ./scripts/module-governance-baseline --root . --format json
cd MineAdmin-web && yarn contract:openapi && yarn lint:tsc && yarn build
git diff --check
```

## Rollback

R3 changes only lifecycle planning/policy internals, focused tests and refactoring documentation. Reverting this slice restores R2 planning internals without data migration, schema rollback, dependency change or external contract change.
