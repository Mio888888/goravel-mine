# Resilience Test Runbook

Run only against an ephemeral staging or Kind environment. The harness refuses targets containing `prod` or `production` and requires `RESILIENCE_ENVIRONMENT=ephemeral`.

Each scenario requires an explicit `RESILIENCE_ACTION_<SCENARIO>` and `RESILIENCE_CLEANUP_<SCENARIO>` command supplied by the environment owner. Actions must execute the real load/fault and write measured JSON with `measured=true` plus non-empty `threshold_decisions`; cleanup failure blocks environment reuse.

The `resilience` GitHub environment used by the `Nightly resilience` job must define matching encrypted secrets for `MULTI_TENANT`, `CASBIN`, `REDIS`, `QUEUE`, `DB_POOL`, `MIGRATION`, `SOAK`, and `RESTORE`. Scheduled runs fail closed when any command required by the selected profile is absent.

Quick profile runs multi-tenant scale, large Casbin policy, Redis outage, queue backlog, DB pool exhaustion, concurrent migration, and measured restore RTO/RPO. Soak adds an eight-hour run and rejects monotonic goroutine, heap, or connection growth above 10%.

```bash
RESILIENCE_ENVIRONMENT=ephemeral \
RESILIENCE_TARGET_URL=https://staging.example.test \
bash scripts/run-resilience-suite.sh --profile=quick
```

The canonical output is `artifacts/resilience/resilience-report.json` using `resilience-report/v1`. Missing, stale, synthetic, or threshold-free evidence fails the suite.
