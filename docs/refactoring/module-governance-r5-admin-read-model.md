# Module Governance R5 Admin Read Model

**Captured:** 2026-07-10
**Base:** R4 over commit `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Go 1.26.4

## Outcome

R5 turns `AdminService` into a 54-line compatibility façade. Read models now have dedicated context-bound query objects, public JSON DTOs are separated from ORM records, and execute/release security checks share one phase policy without sharing action-specific confirm tokens, operations or resources.

Production boundaries:

- `admin_service.go`: constructor, immutable `WithContext` clone and five read façades.
- `admin_contracts.go`: unchanged public admin payload/result/row JSON contracts.
- `admin_state_query.go`: one persisted-state read per State/StateDiff call, manifest projection, drift mapping and sorted orphan rows.
- `admin_list_query.go`: page defaults, count/order/offset/limit, explicit equality filters and PageResult construction.
- `admin_run_query.go`, `admin_step_query.go`, `admin_lock_query.go`: focused ORM records, allowlists and DTO mappers.
- `admin_execute.go`: execute preflight and delayed executor consumption callback.
- `admin_security_gate.go`: shared confirm/re-auth/approval phase policy and error mapping.
- `admin_lock_release.go`: stale-row discovery, `FOR UPDATE` identity recheck, evidence consumption and conditional delete.
- `state_reader.go`: request-context-bound persisted state query with PostgreSQL undefined-table compatibility and no schema probe on the request path.

All R5 production files remain below 300 lines; all changed production functions remain below 50 lines and use no more than three positional parameters.

## Preserved Contracts

- Public `AdminService` method signatures, `WithContext`, payload/result type names and every JSON tag remain unchanged.
- Runs filters remain `module_id`, `action`, `status`, `owner`; Steps filters remain `run_key`, `module_id`, `action`, `status`; unknown filters remain ignored.
- Pagination still defaults to page 1 and size 15; Runs/Steps remain `id DESC`; Locks remain `expires_at DESC`; stale-lock discovery remains `expires_at ASC`.
- StateDiff keeps manifest order first, lexically sorted orphan module IDs second, and the same five drift strings.
- Execute still performs admin preflight before lifecycle planning, while evidence consumption remains after executor lock/idempotency/command preflight and occurs once per batch.
- Stale-lock release still performs preflight, reads expired candidates, rechecks the exact key/owner/run_key/expires_at tuple under `FOR UPDATE`, consumes evidence only when an observed stale row remains, then deletes by the same identity tuple.
- HTTP, CLI, OpenAPI, generated SDK, permission/menu, schema, migration, dependencies and error text remain unchanged.

## Query Budgets

Command: `go test -p 1 ./tests/feature/admin -run '^TestModuleLifecycleReadModelQueryBudgets$' -count=1`

| Read model | R0 visible count | R5 exact count | Result |
| --- | ---: | ---: | --- |
| State | 0 | 1 | request context now observes persisted-state query |
| Runs | 2 | 2 | unchanged |
| Steps | 2 | 2 | unchanged |
| Locks | 1 | 1 | unchanged |
| StateDiff | 0 | 1 | request context now observes one shared persisted-state query |

R0's zero State/StateDiff values were instrumentation gaps, not zero database work. R5 binds the only persisted-state query to the supplied request context, removes the unobserved `HasTable` schema round trip, and prevents StateDiff from issuing a second persisted-state read. A canceled-context regression also covers the undefined-table compatibility path.

## Duplication Result

Command: `go run ./scripts/module-governance-baseline --root . --format json`

| Metric | R0 | R4 | R5 | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Files | 51 | 79 | 89 | +38 |
| Physical lines | 12,991 | 14,401 | 14,818 | +1,827 |
| Clone blocks | 38 | 34 | 34 | -4 (-10.53%) |
| Duplicated source lines | 687 | 607 | 607 | -80 (-11.64%) |

Lexical duplication remains flat versus R4. Semantic duplication improved:

- Runs/Steps pagination, equality filtering, ordering and PageResult construction now have one implementation.
- Admin read paths use one context policy and explicit per-query allowlists.
- Execute/release share one security phase/error policy; their distinct confirm/resource/operation rules remain separate builders.
- Public DTOs no longer double as ORM records.

## Performance Result

Command: `go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=7`

| Benchmark | R0 median ns/op | R4 median ns/op | R5 median ns/op | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Manifest | 53,194 | 55,291 | 55,697 | +4.71% |
| Compatibility | 47,371 | 48,803 | 48,544 | +2.48% |
| Plan | 3,930 | 1,787 | 1,804 | -54.10% |
| DryRun | 9,911 | 3,391 | 3,411 | -65.58% |

Seven isolated R5 samples in ns/op:

- Manifest: 55,401; 54,934; 55,479; 55,759; 55,697; 56,376; 56,604.
- Compatibility: 49,936; 50,011; 48,544; 48,223; 48,456; 48,160; 51,665.
- Plan: 1,804; 1,819; 1,801; 1,818; 1,787; 1,810; 1,804.
- DryRun: 3,409; 3,411; 3,437; 3,415; 3,466; 3,401; 3,385.

Plan remains 6,632 B/op and 35 allocs/op; dry-run remains 6,600 B/op and 95 allocs/op. All medians remain within the R0 maximum regression of 10%.

## Security Disposition

S-005 is complete for the admin lifecycle scope. The shared gate owns only confirm comparison, sensitive-operation request construction, preflight/consume dispatch and error mapping. Execute and release builders still own their exact confirm token, operation, resource and messages. Feature coverage proves lock conflict does not consume execute evidence, approval/re-auth binding remains one-shot after a successful stale-lock release, and stale-lock release does not consume until an identity-matching expired row survives the transactional recheck.

## Review Disposition

Independent review found no critical issues. Its request-context finding was reproduced with a failing canceled-context test: the schema probe bypassed the supplied context and added a hidden database round trip. R5 removes that probe, preserves missing-table compatibility through PostgreSQL SQLSTATE `42P01`, and keeps other query errors visible. The two minor coverage gaps are closed by asserting the complete manifest-first/orphan-last StateDiff order and stale-lock release evidence consumption after success.

## Verification

```bash
go test ./app/modulecatalog ./app/http/controllers/admin ./app/modules/platformobservability ./tests/unit -count=1
go test -p 1 ./tests/feature/admin -run 'TestModuleLifecycleTestSuite|TestModuleLifecycle(ReadModelQueryBudgets|ServiceAPIShapes|ActionResultShapes|ReadModelFiltersAndPagination)' -count=1
go test -race ./app/modulecatalog -count=1
go vet ./app/modulecatalog ./app/http/controllers/admin ./app/modules/platformobservability
go test ./tests/unit -run '^TestModuleGovernance(Manifest|Compatibility|State|LifecyclePlan)Contract$' -count=1
go run . artisan module:manifest:check --artifacts --frontend
go run ./scripts/module-governance-baseline --root . --format json
go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=7
go test -p 1 ./...
cd MineAdmin-web && yarn contract:openapi && yarn lint:tsc && yarn build
cd MineAdmin-web && yarn test:e2e module-lifecycle.spec.ts --project=chromium
git diff --check
```

## Rollback

R5 changes admin query/action internals, context propagation, focused tests and refactoring documentation only. Reverting the `admin_*` query/action files and restoring the R4 `admin_service.go`, `state_reader.go` and `service.go` returns the previous monolith without data migration, schema rollback, dependency change or external contract change.
