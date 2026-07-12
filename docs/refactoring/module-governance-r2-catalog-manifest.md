# Module Governance R2 Catalog And Manifest

**Captured:** 2026-07-10
**Base:** R1 over commit `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Go 1.26.4

## Outcome

R2 separates catalog validation, DTO contracts, manifest/state/compatibility projection, parity validation and persisted-state reading. `modulecatalog.Service` remains the public facade; JSON tags, ordering, error text, nil/empty behavior and lifecycle planning remain unchanged.

Production changes remain inside `app/modules/` and `app/modulecatalog/`:

- `catalog_validator.go`: catalog duplicate, reference and repository-file validation.
- `contracts.go`: unchanged public manifest, compatibility, state and lifecycle DTO contracts.
- `dto_mapper.go`: one authority for dependency, lifecycle, seed, frontend, route, menu and permission projection.
- `manifest_projector.go` / `compatibility_projector.go`: endpoint-specific orchestration over the shared mapper.
- `parity_validator.go`: seed/frontend parity rules and unchanged error strings.
- `state_reader.go`: unchanged ORM query and persisted-row mapping boundary.
- `service.go`: reduced from 736 to 135 lines and now coordinates the extracted boundaries.

New production files remain below 300 lines; new functions remain below 50 lines.

## Preserved Contracts

- Manifest, compatibility, module-state and lifecycle-plan golden snapshots remain unchanged without regeneration.
- Manifest and state projectors retain source order and non-nil empty arrays where the existing JSON contract requires them.
- `omitempty` fields, route permission normalization and disabled reasons remain unchanged.
- Compatibility status still fails only for enabled incompatible modules; `GeneratedAt` remains UTC and is tested with a fixed clock.
- Seed/frontend parity error wording and traversal order remain unchanged.
- Persisted state keeps the existing default ORM context; context propagation remains R5 work.
- Catalog runtime validation still skips metadata file existence while preserving all other checks and joined-error ordering.

## Duplication Result

Command: `go run ./scripts/module-governance-baseline --root . --format json`

| Metric | R0 | R1 | R2 | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Files | 51 | 54 | 62 | +11 |
| Physical lines | 12,991 | 13,085 | 13,247 | +256 |
| Clone blocks | 38 | 37 | 36 | -2 (-5.26%) |
| Duplicated source lines | 687 | 667 | 647 | -40 (-5.82%) |

S-002 is complete: metadata-to-manifest/state/compatibility mapping now has one typed mapper. Endpoint wrappers remain distinct because status, persistence lookup and timestamp behavior differ. R7 retains the cumulative 30% duplicated-line reduction target.

## Performance Result

Command: `go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=5`

| Benchmark | R0 median ns/op | R1 median ns/op | R2 median ns/op | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Manifest | 53,194 | 57,049 | 53,592 | +0.75% |
| Compatibility | 47,371 | 50,414 | 47,156 | -0.45% |
| Plan | 3,930 | 1,443 | 1,339 | -65.93% |
| DryRun | 9,911 | 4,908 | 4,601 | -53.58% |

Five R2 samples in ns/op:

- Manifest: 53,305; 53,605; 53,489; 53,970; 53,592.
- Compatibility: 47,033; 47,040; 47,346; 47,156; 47,162.
- Plan: 1,388; 1,335; 1,339; 1,340; 1,337.
- DryRun: 4,609; 4,599; 4,601; 4,620; 4,600.

All representative medians remain within the R0 maximum regression of 10%.

## Retained Differences

- Public DTO types remain in package `modulecatalog`; moving them would change imports without reducing business-rule duplication.
- Manifest, state and compatibility retain separate projectors because their output envelopes, persistence and status/time rules differ.
- `state_reader.go` deliberately retains facade-based ORM access until R5 introduces context-bound read models.
- Catalog file validation remains repository-aware and synchronous; no filesystem port is added because only one production implementation exists.
- Lifecycle plan construction remains in `service.go`; R3 owns its extraction and deduplication.

## Verification

R2 verification commands:

```bash
go test ./app/modules ./app/modules/platformobservability ./app/modulecatalog ./app/console/commands ./scripts/module-governance-baseline ./tests/unit -count=1
go test ./tests/feature/admin -run 'TestModuleLifecycleTestSuite|TestModuleLifecycle(ReadModelQueryBudgets|ServiceAPIShapes|ActionResultShapes)' -count=1
go test ./tests/unit -run '^TestModuleGovernance(Manifest|Compatibility|State|LifecyclePlan)Contract$' -count=1
go run . artisan module:manifest:check --artifacts --frontend
go test -race ./app/modules ./app/modulecatalog -count=1
go vet ./app/modules ./app/moduleboot ./bootstrap ./app/modulecatalog
go test ./...
go run ./scripts/module-governance-baseline --root . --format json
git diff --check
```

All commands above passed. The full backend gate used `go test -p 1 ./...` so database-refreshing feature packages could not contend for the shared PostgreSQL test database. Frontend OpenAPI contract generation check, `yarn lint:tsc` and `yarn build` also passed.

The requested external reviewer remained running without returning findings and was closed after two waits. Main-thread review compared the extracted code with the R1 implementation, checked JSON tags and nil/empty behavior, compatibility status/time rules, parity error text/order, catalog joined-error order and package boundaries; no Critical or Important issue was found.

## Rollback

R2 changes only catalog/modulecatalog internals, focused tests and refactoring documentation. Reverting this slice restores R1 projection and validation internals without data migration, schema rollback, dependency change or external contract change.
