# Module Governance R4 Lifecycle Executor

**Captured:** 2026-07-10
**Base:** R3 over commit `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Go 1.26.4

## Outcome

R4 turns lifecycle execution into a port-driven coordinator while preserving the public `LifecycleService` facade. The executor now resolves repository, lock, command-runner and clock dependencies once per public execution; the legacy `store` and test `runner` fields remain as compatibility fallbacks.

Production boundaries:

- `lifecycle_service.go`: public options/results/records, compatibility fields and planner-to-executor facade; reduced from 1,282 to 247 lines.
- `lifecycle_executor.go`: item lock, idempotency, validation, security gate, run and state orchestration.
- `lifecycle_command_attempt.go`: command attempt start/execute/finish phases and late-runner reconciliation records.
- `lifecycle_command_executor.go`: command deduplication, lease renewal, timeout/cancellation and late-runner watching.
- `lifecycle_execution_policy.go`: lock TTL/renew/timeout and error-status policy.
- `lifecycle_state_machine.go`: authoritative success/failure transitions and late-runner lock policy.
- `lifecycle_records.go`: authoritative run/step/state construction, timestamps, output truncation and error text.
- `lifecycle_ports.go`: repository, lock manager, command runner, clock/timer/ticker ports plus system adapters.
- `lifecycle_memory_store.go`, `lifecycle_db_store.go`, `lifecycle_db_lock.go`, `lifecycle_db_repository.go`: focused Goravel adapters.

All changed production files remain below 300 lines; changed production functions remain below 50 lines.

## Preserved Contracts

- Lock acquisition still precedes idempotency lookup, command validation and security evidence consumption.
- Security evidence is still consumed after a successful lock/idempotency/command preflight and before `BeginRun`.
- Existing successful and reconciliation-required runs still block automatic retry; ordinary failed runs remain retryable with prior step attempts retained.
- Destructive checks still run before lifecycle commands; shared normalized commands remain deduplicated across a batch.
- Each command still renews the lease before runner start and renews it periodically while running.
- Timeout, blocked renewal and renewal failure still cancel the runner; a runner that ignores cancellation retains the lock and transitions to reconciliation-required when it eventually completes.
- Late runners still cannot release a reacquired lease because release remains bound to the original owner and run key.
- Manual and disallowed single-module commands still record the prior run, step and state statuses; batch preflight remains in the planner.
- API payloads, CLI flags, JSON tags, error strings, OpenAPI, generated SDK, goldens, schema and migrations remain unchanged.

## Authoritative Writes

- `lifecycleRecorder.beginRun`, `finishRun` and `upsertState` are the only executor paths that construct run/state timestamps.
- `lifecycleRecorder.recordStep` is the only executor path that constructs step records, truncates stdout/stderr and maps error text.
- `lifecycleStateMachine.failure` is the only command-failure status/late-runner policy; destructive-check and lifecycle-command failures share the same finalizer.
- `LifecycleClock` provides `Now`, timeout contexts, timers and tickers. The default adapter retains standard-library behavior; tests inject fixed/tracking clocks.

## Duplication Result

Command: `go run ./scripts/module-governance-baseline --root . --format json`

| Metric | R0 | R3 | R4 | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Files | 51 | 66 | 79 | +28 |
| Physical lines | 12,991 | 13,618 | 14,401 | +1,410 |
| Clone blocks | 38 | 36 | 34 | -4 (-10.53%) |
| Duplicated source lines | 687 | 647 | 607 | -80 (-11.64%) |

R4 removes two lexical clone blocks and 40 duplicated lines versus R3. More importantly, the semantic duplicates for destructive-check/command failure finalization and running/completed step mapping now have one implementation. R7 retains the cumulative 30% duplicated-line target.

## Performance Result

Command: `go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=7`

| Benchmark | R0 median ns/op | R3 median ns/op | R4 median ns/op | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Manifest | 53,194 | 53,997 | 55,291 | +3.94% |
| Compatibility | 47,371 | 47,780 | 48,803 | +3.02% |
| Plan | 3,930 | 1,789 | 1,787 | -54.53% |
| DryRun | 9,911 | 3,308 | 3,391 | -65.79% |

Seven isolated R4 samples in ns/op:

- Manifest: 55,227; 55,186; 55,272; 55,291; 55,548; 55,324; 55,486.
- Compatibility: 48,536; 48,859; 48,803; 48,775; 48,722; 48,867; 48,879.
- Plan: 1,786; 1,786; 1,786; 1,788; 1,792; 1,787; 1,790.
- DryRun: 3,389; 3,391; 3,396; 3,383; 3,381; 3,508; 3,396.

Plan remains 6,632 B/op and 35 allocs/op; dry-run is 6,600 B/op and 95 allocs/op. All representative medians remain within the R0 maximum regression of 10%. Samples were run serially without concurrent backend or frontend verification.

## Security Scope

S-005 is partially addressed. R4 preserves and tests the lifecycle executor's preflight-versus-consume boundary: lock conflict and idempotent skip happen before the application security callback, and the callback is shared once across a batch. Admin execute/release gate implementation remains distinct because confirm tokens, operations and resources differ; R5 may share gate phases only if persistent approval/re-auth consumption order remains unchanged.

## Verification

R4 verification commands:

```bash
go test ./app/modules ./app/modules/platformobservability ./app/modulecatalog ./app/console/commands ./scripts/module-governance-baseline ./tests/unit -count=1
go test -p 1 ./tests/feature/admin -run 'TestModuleLifecycleTestSuite|TestModuleLifecycle(ReadModelQueryBudgets|ServiceAPIShapes|ActionResultShapes)' -count=1
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

R4 changes lifecycle execution internals, focused tests and refactoring documentation only. Reverting the executor/port/adapter files and restoring the R3 `lifecycle_service.go` returns the previous monolith without data migration, schema rollback, dependency change or external contract change.
