# Module Governance R6 Frontend Governance Page

**Captured:** 2026-07-10
**Base:** R5 over commit `a68496a56fd1069587773689ca8dc7f2d98a89fa`
**Environment:** macOS 26.5, arm64, Apple M4, Node 24.12.0, Yarn 1.22.22

## Outcome

R6 replaces the 830-line lifecycle governance monolith with a 168-line route coordinator, three behavior composables, narrow presentation/security helpers and focused view components. The page keeps its existing route, keep-alive name, selectors, labels, permissions, payloads and HTTP contracts.

Production boundaries:

- `index.vue`: active tab, log permission, options, summary, refresh wiring and run-to-step navigation.
- `useModuleLifecycleState.ts`: five read models, typed response normalization, independent loading state, query reset/search, refresh and run-step selection.
- `useLifecycleExecution.ts`: dry-run/execute validation, evidence binding, confirm dispatch, payload construction, result and post-success refresh.
- `useStaleLockRelease.ts`: stale-lock validation, evidence binding, confirm dispatch, payload construction and lock-only refresh.
- `sensitiveOperation.ts`: evidence cancellation and confirmation cancellation only; action-specific scope/resource/token logic remains local.
- `presentation.ts`: one status/action/bool/time presentation policy.
- View components: action/result/state/runs/steps/locks/diff plus lifecycle-specific query fields, filter actions and pagination reused by Runs and Steps.
- `moduleLifecycle.scss`: the previous visual rules, moved without redesign.

Every R6 production file remains below 300 lines; every changed function remains below 50 lines, nesting remains at most three, and no generic `any` workflow or reflection mapper was introduced.

## Preserved Contracts

- `platformModuleLifecycle.ts`, generated SDK, OpenAPI, HTTP paths/methods, query keys, payload keys and response shapes remain unchanged.
- Route path/name/component, menu key, permission codes, i18n keys and `defineOptions({ name: 'platform:moduleLifecycle' })` remain unchanged.
- Execute still requires trimmed owner/reason and confirm token `${module_id || 'all'}:${action}`.
- Stale-lock release still requires `release-stale-locks`; resource remains `module-lifecycle:stale-locks:${key || 'all'}`.
- Dry-run skips evidence; real actions remain local validation -> evidence -> confirm -> API.
- Execute success awaits one full read refresh; release success awaits one lock refresh.
- DOM selectors used by existing E2E remain stable, including `.module-lifecycle-action`, `.module-lifecycle-filter` and confirm-token ordering.

## Request And Loading Semantics

Playwright characterization now proves:

- Admin initial load: state=1, runs=1, steps=1, locks=1, diff=1.
- Readonly initial load: state=1, runs=1, steps=0, locks=1, diff=1.
- Switching all tabs issues no new read request.
- `查看 Step` adds exactly one filtered request with `run_key=upgrade:platform-rbac:1.0.0`.
- State/runs/steps/locks/diff/action/release loading refs are independent.
- The top refresh button uses the aggregate of all five read loading refs and stays loading until the slowest request settles.

The refresh-tail test was observed red before production edits because the previous button omitted step/diff loading; it passes after integration.

## Security Disposition

S-007 is complete. Execute and release now share only two narrow mechanics: evidence-dialog cancellation handling and confirmation cancellation handling. Execute still owns `module.lifecycle.execute`, its module/action resource and dynamic token; release still owns `module.lifecycle.release-lock`, its stale-lock resource and fixed token. Evidence is copied into the action form only after successful collection, and cancellation produces neither API calls nor error toasts.

Playwright covers dry-run evidence omission, execute approval/re-auth binding and stale-lock approval/re-auth binding. Backend feature tests remain the authority for one-shot consumption and transaction/lock ordering.

## Duplication Result

Command: `go run ./scripts/module-governance-baseline --root . --format json`

| Metric | R0 | R5 | R6 | Delta vs R0 |
| --- | ---: | ---: | ---: | ---: |
| Files | 51 | 89 | 104 | +53 |
| Physical lines | 12,991 | 14,818 | 15,266 | +2,275 |
| Clone blocks | 38 | 34 | 33 | -5 (-13.16%) |
| Duplicated source lines | 687 | 607 | 591 | -96 (-13.97%) |

An intermediate component split introduced Runs/Steps filter and pagination clones. R6 then extracted lifecycle-specific query fields, filter actions and pagination. Review closeout also moved repeated E2E evidence completion into `completeSensitiveEvidence`; final lexical duplication is one block and 16 lines below R5. S-006 now has one typed read loader and S-007 has one evidence/confirmation helper pair. The additional production files represent ownership boundaries rather than duplicated behavior.

## Structure Metrics

- Route coordinator: 168 lines, down from 830.
- Composables/helpers: 40-214 lines each.
- View components: 18-87 lines each.
- Lifecycle E2E: 266 lines, with page-specific setup/counting isolated in a 66-line support helper; coverage includes exact request counts, refresh-tail, readonly action visibility and cancellation.
- Frontend scanner group: 17 production files / 1,398 lines.

## Rendered QA

The in-app Browser reached the local app and reported no console errors, but the real route redirected to a CAPTCHA-protected login. Browser safety rules prohibit solving a CAPTCHA without explicit user confirmation, so rendered governance QA used the repository Playwright mock environment.

Verified flows:

- Desktop 1440x1000: page identity, run-to-step navigation, diff rendering, dry-run result, no framework overlay, no console/page errors and no control overlap.
- Mobile 390x844: title, refresh, stats and action form remain separated, readable and vertically responsive with no overlay or console/page errors.
- Screenshots: `/tmp/goravel-mine-r6-module-lifecycle-desktop.png` and `/tmp/goravel-mine-r6-module-lifecycle-mobile.png`.

## Verification

Passed:

```bash
cd MineAdmin-web && yarn contract:openapi
cd MineAdmin-web && yarn lint:tsc
cd MineAdmin-web && yarn eslint src/modules/base/views/platform/moduleLifecycle/index.vue src/modules/base/views/platform/moduleLifecycle/*.ts src/modules/base/views/platform/moduleLifecycle/components/*.vue tests/e2e/module-lifecycle.spec.ts
cd MineAdmin-web && yarn stylelint src/modules/base/views/platform/moduleLifecycle/moduleLifecycle.scss
cd MineAdmin-web && yarn build
cd MineAdmin-web && yarn test:e2e module-lifecycle.spec.ts --project=chromium
go test ./tests/unit -run '^TestModuleGovernance' -count=1
go run . artisan module:manifest:check --artifacts --frontend
go run ./scripts/module-governance-baseline --root . --format json
go test -p 1 ./...
git diff --check
```

Lifecycle E2E result: 9 passed. Temporary desktop/mobile rendered QA: 2 passed; the temporary spec was removed after screenshots were saved outside the repository.

Independent review found no Critical issue. Its Important media-query compatibility finding was resolved by restoring `max-width` syntax. Both Minor test gaps were also closed: readonly users now assert the action/release controls are absent, and evidence/confirmation cancellation asserts no execute/release API request is sent.

Repository-wide `yarn lint` remains blocked by 19 pre-existing ESLint errors outside the R6 scope, including README parsing and unrelated unused variables. The command's automatic unrelated formatting was reversed. Vue Stylelint also cannot parse `.vue` files because the existing install lacks `postcss-html`; the only R6 stylesheet is SCSS and passes targeted Stylelint.

## Rollback

R6 changes only the lifecycle route page, page-local components/composables/helpers, focused E2E characterization and refactoring documents. Restoring the prior `index.vue` and removing the new `moduleLifecycle/` support files returns the R5 frontend without database, schema, dependency, route, permission, OpenAPI or generated-client rollback.
