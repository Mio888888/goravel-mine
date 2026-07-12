# Module Governance R7 Contract Closeout

**Captured:** 2026-07-10
**Base:** R0-R6 over commit `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Go 1.26.4, Node 24.12.0, Yarn 1.22.22

## Outcome

R7 closes the module-governance pilot without changing HTTP, CLI, OpenAPI, database, permission, menu, route or frontend behavior. It removes the lifecycle store dual path, deletes unused internal security-gate wrappers, reduces same-semantics test setup and records the remaining contract/release debt.

Final scanner result is 21 clone blocks / 412 duplicated source lines. R0 was 38 / 687, so both measures fall more than 30%.

## Lifecycle Closeout

`LifecycleService` now directly owns:

- `LifecycleRepository` for run, step and state persistence.
- `LifecycleLockManager` for acquire, renew and release.
- `LifecycleCommandRunner` for allowlisted command execution.
- `LifecycleClock` for time, timeout, timer and ticker behavior.

Removed:

- `LifecycleStore` compatibility interface.
- `lifecycleStoreRepository` and `lifecycleStoreLockManager` adapters.
- `LifecycleService.store` and `LifecycleService.runner` fallback fields.
- DB/memory `AcquireLock`, `RenewLock`, `ReleaseLock` and `FinishRun` methods.
- unused execute/release gate preflight/validation wrappers.

`SetRunnerForTest` remains a narrow test seam and now replaces the command-runner port. Lifecycle tests explicitly bind repository and lock-manager ports, preserving independent-port, clock, lock identity, renewal, timeout, late-runner and reconciliation coverage.

Independent closeout review found one late-runner lease-loss risk: background renewal failures were previously ignored. The watcher now records a `reconciliation_required` run, step and state error naming the renewal failure, then stops treating the stale lease as renewable. It cannot stop a runner that already ignored cancellation, so operator reconciliation remains required before intervention.

## Retained Facades

- `modules.Registry`: retained public compatibility entry; delegates source selection, topology and disabled-reason rules to `registryKernel`. Remove only after a versioned public replacement exists and every caller migrates.
- `modulecatalog.Service`: retained public manifest/compatibility/plan/parity entry; delegates to projectors, planner and validators. Remove only after CLI, tests and adopters consume replacement use cases.
- `LifecycleService`: retained lifecycle application facade for CLI/admin callers; now contains no legacy store implementation.
- `AdminService`: retained HTTP-facing application facade; query/security helpers own internal behavior.

These facades contain delegation/orchestration, not duplicate business rules.

## Contract Disposition

- OpenAPI remains 3.1.0 with exactly 7 paths and 7 operations.
- HTTP methods, route names, permissions, envelopes, request/response fields and business 422 semantics remain unchanged.
- CLI signatures, flags, defaults, categories and machine-readable output remain covered by golden tests.
- Manifest, compatibility, lifecycle plan and platform observability goldens remain authoritative.
- `yarn contract:openapi` continues to validate generated `admin-base-apis` SDK output.
- Module-governance frontend types remain in the hand-written `platformModuleLifecycle.ts`; Go contract tests validate its endpoint and envelope parity, and `yarn lint:tsc` validates usage. Moving this OpenAPI into generated frontend types remains debt, not a claimed R7 delivery.

## Structure

| Boundary | R0 | R7 |
| --- | ---: | ---: |
| `lifecycle_service.go` | 1,539 lines | 217 lines |
| `modulecatalog/service.go` | 736 lines | 97 lines |
| lifecycle route page | 830 lines | 168 lines |
| Scanner files | 51 | 106 |
| Scanner physical lines | 12,991 | 14,985 |

File growth is from named kernels, policies, adapters, queries, components and contract tests. R7 production files changed in this slice remain under 300 lines. Existing test suites and `module_manifest_check.go` remain larger and are listed as debt.

## Duplication

| Metric | R0 | R6 | R7 | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Clone blocks | 38 | 33 | 21 | -17 (-44.74%) |
| Duplicated source lines | 687 | 591 | 412 | -275 (-40.03%) |

R7 shares only same-domain fixtures:

- one frontend manifest fixture object/source renderer for canonical file/menu/permission cases;
- one alpha lifecycle registry plus memory-port fixture;
- one lifecycle persistence-port binder.

Multiline parser cases, timing/channel flows, explicit security assertions and cross-module declaration similarities remain separate because they have different change reasons.

## Performance

Command: `go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=5`

| Benchmark | R0 median ns/op | R7 samples ns/op | R7 median | Delta | B/op | allocs/op |
| --- | ---: | --- | ---: | ---: | ---: | ---: |
| Manifest | 53,194 | 54,763 / 55,295 / 53,643 / 56,283 / 55,713 | 55,295 | +3.95% | 178,616 | 1,425 |
| Compatibility | 47,371 | 47,031 / 47,277 / 47,158 / 46,988 / 47,007 | 47,031 | -0.72% | 149,048 | 1,342 |
| Plan | 3,930 | 1,740 / 1,739 / 1,748 / 1,739 / 1,737 | 1,739 | -55.75% | 6,632 | 35 |
| DryRun | 9,911 | 3,258 / 3,262 / 3,329 / 3,454 / 3,318 | 3,318 | -66.52% | 6,568 | 93 |

All medians remain inside the no-more-than-10% regression gate.

## Data And Security

- No migration, schema, seed contract, dependency, root config, CI or environment template changed.
- Idempotency keys and run/step/state/lock rows retain their existing shapes and meanings.
- Confirm token, re-auth, approval resource binding, one-shot consumption, lock conflict and stale-lock race rules remain unchanged.
- Command allowlist, timeout, output truncation and reconciliation semantics remain centralized and unchanged.

## Remaining Debt

- Generate module-governance frontend types from `module-governance.openapi.json`; current wrapper is typed and parity-tested but hand-maintained.
- Add generic JSON/OpenAPI semantic lint for every `OpenAPIFiles()` fragment; manifest validation currently proves existence only.
- Release automation hard-gates compatibility matrix, but not all manifest/plan/lifecycle/approval/rollback evidence named by the adopter guide.
- `app/console/commands/module_manifest_check.go` is 374 lines and mixes command, TypeScript parsing, path validation and seed parity.
- Large characterization suites remain: lifecycle unit test 1,277 lines and lifecycle feature test 933 lines.
- Repository-wide `yarn lint` still reports 19 pre-existing errors outside this pilot; targeted frontend type, ESLint, Stylelint, E2E and build gates remain the R6/R7 evidence.

## Next Pilot

Recommend tenant platform/runtime governance.

Evidence:

- `app/services/tenant_service.go` is 1,030 lines and mixes CRUD, plan/permission changes, destructive deletion, connection registration, PostgreSQL provisioning, migration and seeding.
- `app/services/tenant_runtime_service.go` is 815 lines and mixes effective policy projection, quotas/rate limit, public features, SSO login, host trust and JSON helpers.
- tenant module flags already reach runtime middleware, so the next pilot can focus on boundaries rather than adding missing behavior.

Suggested slices: baseline contracts -> tenant repository/query -> provisioning ports -> plan/permission use cases -> runtime policy projector -> SSO boundary. Non-goals: no tenant schema change, no authentication semantic change, no IdP redesign, no billing/retention feature expansion.

## Rollback

R7 rollback restores the legacy lifecycle store fields/adapters/methods, reverts test fixture extraction and restores the prior guide/roadmap wording. No data rollback, migration rollback, dependency rollback or generated-source rollback is required.
