# Module Governance R1 Registry Kernel

**Captured:** 2026-07-10
**Base:** R0 at commit `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Go 1.26.4

## Outcome

R1 makes `registryKernel` the single authority for module source order, runtime registration order, lifecycle topology order, explicit/propagated disabled reasons and active module selection. `Registry` remains the public compatibility facade; no HTTP, CLI, manifest, lifecycle DTO, database or environment contract changed.

Production changes remain inside `app/modules/`:

- `registry_kernel.go`: immutable source projections, stable topology fallback, disabled-reason propagation and dependency validation.
- `registry_artifacts.go`: one typed collector for routes, menus, permissions, migrations, seeders, OpenAPI files and test templates.
- `module.go`: thin Registry facade and catalog projection.
- `dependencies.go` / `package.go`: delegate source and validation selection to the kernel.

`module.go` fell from 343 to 131 lines. New production files remain below 300 lines; new functions remain below 50 lines.

## Preserved Contracts

- `ModuleStates()` preserves module input order.
- `LifecycleStates()` uses stable dependency order; cycles and duplicate IDs fall back to input order.
- `IDs()` and runtime artifacts exclude explicitly disabled modules.
- Required dependents are transitively disabled; each reason names the immediate disabled dependency.
- Optional dependencies do not disable dependents.
- Missing, disabled, cyclic and duplicate dependency error text remains unchanged.
- Manifest, compatibility, module-state and lifecycle-plan goldens remain byte-equivalent semantically; no golden regeneration was used.

## Duplication Result

Command: `go run ./scripts/module-governance-baseline --root . --format json`

| Metric | R0 | R1 | Delta |
| --- | ---: | ---: | ---: |
| Files | 51 | 54 | +3 |
| Physical lines | 12,991 | 13,085 | +94 |
| Clone blocks | 38 | 37 | -1 (-2.63%) |
| Duplicated source lines | 687 | 667 | -20 (-2.91%) |

S-001 is merged: module source selection, topology fallback, disabled reason propagation and active selection now have one authority. Small projection loops remain where output types differ; artifact aggregation shares one typed collector. R7 retains the cumulative 30% reduction target.

## Performance Result

Command: `go test ./app/modulecatalog -run '^$' -bench '^BenchmarkModuleGovernance' -benchmem -count=5`

| Benchmark | R0 median ns/op | R1 median ns/op | Delta |
| --- | ---: | ---: | ---: |
| Manifest | 53,194 | 57,049 | +7.25% |
| Compatibility | 47,371 | 50,414 | +6.42% |
| Plan | 3,930 | 1,443 | -63.28% |
| DryRun | 9,911 | 4,908 | -50.48% |

All representative medians remain within the R0 maximum regression of 10%. Lifecycle planning benefits from topology and disabled-state projections being frozen once at Registry construction.

## Verification

Passed:

```bash
go test ./app/modules ./app/moduleboot ./bootstrap -count=1
go test ./app/modules ./app/modules/platformobservability ./app/modulecatalog ./app/console/commands ./scripts/module-governance-baseline ./tests/unit -count=1
go test ./tests/feature/admin -run 'TestModuleLifecycleTestSuite|TestModuleLifecycle(ReadModelQueryBudgets|ServiceAPIShapes|ActionResultShapes)' -count=1
go test ./tests/unit -run '^TestModuleGovernance(Manifest|Compatibility|State|LifecyclePlan)Contract$' -count=1
go run . artisan module:manifest:check --artifacts --frontend
git diff --check
```

Also passed:

```bash
go test ./...
go test -race ./app/modules -count=1
go vet ./app/modules ./app/moduleboot ./bootstrap ./app/modulecatalog
```

The requested external reviewer did not return a terminal result and was closed. Main-thread review checked the complete Registry diff, zero-value call sites, dependency/error compatibility, golden parity, package boundaries and final verification output; no Critical or Important issue was found.

## Remaining Duplication

- S-002 metadata-to-manifest/state/compatibility mapping remains for R2.
- Lifecycle planner/executor and admin read-model duplication remain unchanged for R3-R5.
- Frontend request/evidence orchestration remains unchanged for R6.
- Test fixture duplication remains eligible only when semantics and change reasons match.

## Rollback

R1 changes only the Registry implementation, Registry tests and refactoring documentation. Reverting the R1 slice restores R0 internals without data migration, schema rollback or external contract changes.
