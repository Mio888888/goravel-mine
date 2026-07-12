# Module Governance R0 Baseline

**Captured:** 2026-07-10
**Commit:** `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Go 1.26.4

## Scope And Exclusions

R0 freezes current module-governance contracts and behavior. It adds tests, snapshots, OpenAPI, benchmarks, E2E coverage, deterministic duplication tooling and this report. Runtime production code, database schema, migrations, dependencies, CI and environment templates remain unchanged.

Excluded: `MineAdmin/`, generated frontend sources, iconify assets, `node_modules`, `dist`, vendor and testdata content. The clone scanner accepts `.go`, `.ts`, `.tsx` and `.vue` only.

## Contract Snapshot Inventory

- Registry projections: manifest, compatibility matrix, module states and lifecycle plans under `tests/unit/testdata/module-governance/`.
- Registry dependency-sort failure: input-order fallback for IDs and lifecycle states; `Validate()` still reports cycles.
- Platform module routes, menus and permissions: `app/modules/platformobservability/testdata/module-governance-contract.golden.json`.
- CLI signatures, descriptions, categories, flags/defaults and lifecycle JSON writer: `app/console/commands/testdata/module-governance-cli.golden.json` plus writer assertions.
- Dedicated OpenAPI 3.1 contract: exactly 7 paths, route/permission/frontend wrapper parity, bearer auth and typed success/error envelopes.
- API shape characterization: state, runs, steps, locks, diff, execute dry-run and stale-lock dry-run, including populated `omitempty` fields.

## Characterization Coverage

- Dependency order and cycle fallback.
- Current serialized DTO key sets.
- Read-model query budgets.
- Frontend state/runs/steps/diff/locks workflows and readonly log permission.
- Execute dry-run payload without evidence prompt.
- Execute and stale-lock release evidence resource binding and confirm-token contracts.
- Existing lifecycle feature suite remains the authority for approval, lock conflict, retry, reconciliation and stale-lock race behavior.

## Read-Model Query Budgets

Fixture: one persisted state, run, step and lock. Query logging uses Goravel `db.EnableQueryLog` and tests run serially.

| Read model | Observed | Enforced maximum |
| --- | ---: | ---: |
| State | 0 | 1 |
| Runs | 2 | 2 |
| Steps | 2 | 2 |
| Locks | 1 | 1 |
| StateDiff | 0 | 2 |

At R0, the two zero observations were not proof of zero DB work. `State()` and `StateDiff()` called `persistedModuleStates()` through a background/default ORM context, so query logging could not see those queries. R5 has since propagated context and tightened the exact counts without increasing actual query work.

## Benchmark Samples

Command: `go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=5`

| Benchmark | Five samples, ns/op | B/op | allocs/op |
| --- | --- | ---: | ---: |
| Manifest | 53194, 54239, 53083, 53220, 53157 | 179016-179017 | 1449 |
| Compatibility | 47365, 48765, 47606, 47288, 47371 | 149448 | 1366 |
| Plan | 3929, 3930, 3930, 3940, 3936 | 7888 | 94 |
| DryRun | 9891, 9913, 9932, 9911, 9896 | 19920 | 253 |

R1-R7 comparisons must use the same command, machine class, fixture and five-sample method. Performance gate compares median time and disallows more than 10% regression.

## Textual Duplication Metrics

Command: `go run ./scripts/module-governance-baseline --root . --format json`

- Files: 51; physical lines: 12,991.
- Backend: 27 files / 6,061 lines.
- Frontend: 2 files / 1,025 lines.
- Tests: 22 files / 5,905 lines.
- Clone blocks: 38; duplicated source lines: 687.
- Largest files: `lifecycle_service.go` 1,539; `lifecycle_service_test.go` 1,443; feature lifecycle test 896; lifecycle Vue page 830; catalog service 736.

R0 itself removed two new duplicate families before freezing this value: three Go golden writers now use `tests/testsupport.RequireGoldenJSON`; frontend approval creation/approver switch flow now uses `securityEvidence.ts` from both lifecycle specs. These changes are retained because they establish one authoritative implementation for each test concern. Earlier intermediate scanner values are omitted: block-comment and multiline-import handling changed during TDD, so those measurements are not comparable to the final algorithm.

Representative remaining samples include repeated command test fixtures, metadata projection blocks, lifecycle execution branches and repeated E2E dialog submission. Scanner output is deterministic and sorted; semantic inventory below controls cases textual detection cannot judge.

## Semantic Duplication Inventory

- **S-001 Registry source selection / disabled reason recomputation -> R1 completed.** `registryKernel` now owns source order, stable topology fallback, disabled reason propagation and active selection; `Registry` remains the compatibility facade. Evidence: `module-governance-r1-registry-kernel.md`, registry unit tests and unchanged goldens.
- **S-002 Metadata-to-manifest/state/compatibility mapping -> R2 completed.** `dtoMapper` now owns dependency, lifecycle, seed, frontend, route, menu and permission projections; manifest, state and compatibility projectors reuse it. Evidence: `module-governance-r2-catalog-manifest.md`, projector tests, unchanged four projection goldens and OpenAPI parity.
- **S-003 Lifecycle action/order/command planning -> R3 completed.** `lifecyclePlanner` now owns valid actions, stable/reverse order, module filtering, command selection, idempotency keys, version policy and batch preflight; CLI plan and lifecycle dry-run retain distinct public adapters and pre-existing validation boundaries. Evidence: `module-governance-r3-lifecycle-planner.md`, planner tests, unchanged plan golden and lifecycle tests.
- **R4 Lifecycle executor ports/state writes completed.** `lifecycleExecutor` now owns execution orchestration through repository, lock, command-runner and clock ports; state machine and record writers centralize command failures, run/step/state timestamps, output truncation and error mapping. Scanner evidence: 34 clone blocks and 607 duplicated source lines, down 4 blocks and 80 lines from R0. Evidence: `module-governance-r4-lifecycle-executor.md`, executor port/clock/failure tests, concurrency suite, race and full serial verification.
- **S-004 Admin Runs/Steps pagination and equality filters -> R5 completed.** Typed run/step queries now reuse one pagination/filter/order/PageResult kernel while retaining explicit per-query allowlists; State/StateDiff persisted reads use the request context, omit the previous hidden schema probe and read persisted state once. Evidence: `module-governance-r5-admin-read-model.md`, exact query budgets, canceled-context coverage, filter/pagination tests and unchanged API shapes.
- **S-005 Execute/release preflight-vs-consume evidence flow -> R5 completed.** One admin security gate owns phase dispatch and error mapping; execute/release retain distinct confirm token, operation and resource builders. Execute consumption remains behind executor lock/idempotency/command preflight; stale-lock release consumes only after transactional identity recheck. Evidence: `module-governance-r5-admin-read-model.md`, lock-conflict, resource-binding, one-shot and reacquired-lock tests.
- **S-006 Frontend loadState/loadRuns/loadSteps/loadLocks/loadDiff -> R6 completed.** `useModuleLifecycleState` owns one typed response/loading kernel while retaining five independent loading refs, permission-gated Steps and exact query/reset semantics. Evidence: `module-governance-r6-frontend-governance-page.md`, exact initial/tab/run-step request counts and refresh-tail coverage.
- **S-007 Frontend execute/release evidence orchestration -> R6 completed.** `sensitiveOperation.ts` shares only evidence/confirm cancellation mechanics; execute/release composables retain distinct validation, token, scope, resource, payload and refresh rules. Evidence: `module-governance-r6-frontend-governance-page.md`, execute/release Playwright payload and evidence binding tests.
- **S-008 Lifecycle test module/store/security fixture setup -> R1-R5 as touched.** Files: lifecycle unit/feature tests. Add builders only for same semantics and change reason; explicit case data may remain. Evidence: clone metric delta plus unchanged test readability.

## Known Risks

- R5 fixed State/StateDiff request-context observability and removed the unobserved `HasTable` round trip; exact query budgets are now State=1, Runs=2, Steps=2, Locks=1 and StateDiff=1.
- Golden snapshots intentionally fail on ordering, nil/empty and metadata changes; intentional contract changes require explicit review.
- E2E uses admin API mocks; backend feature tests remain required for persistence and security consumption order.
- Clone detection is lexical after normalization, not semantic. Every slice must update both scanner metrics and the semantic inventory.
- R0 uses Go 1.26.4 locally while `go.mod` declares Go 1.24.0; later comparisons should record the actual toolchain.
- R6 lowers scanner duplication below R5: 33 clone blocks and 591 duplicated lines after lifecycle-specific Runs/Steps controls and repeated sensitive-evidence E2E completion were shared.
- R7 removes the legacy lifecycle store adapter path and closes at 21 clone blocks / 412 duplicated lines. Compared with R0 this is -17 blocks (-44.74%) and -275 duplicated lines (-40.03%), passing both 30% gates.

## R7 Final Comparison

R7 command remains `go run ./scripts/module-governance-baseline --root . --format json`.

| Metric | R0 | R6 | R7 | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Files | 51 | 104 | 106 | +55 |
| Physical lines | 12,991 | 15,266 | 14,985 | +1,994 |
| Clone blocks | 38 | 33 | 21 | -17 (-44.74%) |
| Duplicated source lines | 687 | 591 | 412 | -275 (-40.03%) |

R7 final isolated benchmark medians:

| Benchmark | R0 median ns/op | R7 median ns/op | Delta | R7 B/op | R7 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| Manifest | 53,194 | 55,295 | +3.95% | 178,616 | 1,425 |
| Compatibility | 47,371 | 47,031 | -0.72% | 149,048 | 1,342 |
| Plan | 3,930 | 1,739 | -55.75% | 6,632 | 35 |
| DryRun | 9,911 | 3,318 | -66.52% | 6,568 | 93 |

The lifecycle production boundary is now ports-only: `LifecycleService` directly holds repository, lock-manager, command-runner and clock interfaces. The old `LifecycleStore`, adapters, `store`/`runner` fields and old lock/run methods are removed. Same-semantics frontend-manifest and alpha lifecycle test fixtures provide the final lexical reduction; incidental cross-domain clones remain unabstracted.

## R1 Entry Criteria

- All registry, route/permission, CLI and OpenAPI contract tests pass.
- Module lifecycle feature characterization passes serially.
- Dedicated lifecycle Chromium E2E passes.
- Baseline scanner reproduces 38 clone blocks and 687 duplicated lines at this source state.
- Registry kernel changes preserve all goldens and cycle fallback, reduce S-001 duplication, add no production contract/schema/dependency change, and remain independently revertible.

## Reproduction Commands

```bash
go test ./app/modules ./app/modules/platformobservability ./app/modulecatalog ./app/console/commands ./scripts/module-governance-baseline ./tests/unit -count=1
go test ./tests/feature/admin -run 'TestModuleLifecycleTestSuite|TestModuleLifecycle(ReadModelQueryBudgets|ServiceAPIShapes|ActionResultShapes)' -count=1
go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=5
go run ./scripts/module-governance-baseline --root . --format json
go run . artisan module:manifest:check --artifacts --frontend
jq -e '.openapi == "3.1.0" and (.paths | length == 7)' docs/api-contract/openapi/module-governance.openapi.json
cd MineAdmin-web && yarn contract:openapi && yarn lint:tsc && yarn build
cd MineAdmin-web && yarn test:e2e module-lifecycle.spec.ts --project=chromium
go test ./...
```

Database-refreshing feature commands must run alone, not in parallel.

## Rollback

R0 changes tests, docs and tooling only. One revert removes all R0 gates and helper deduplication without changing runtime data, database schema or production behavior.
