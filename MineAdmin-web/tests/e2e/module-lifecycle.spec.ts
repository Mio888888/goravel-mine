import { expect, test } from '@playwright/test'
import { loginAsAdmin, mockAdminApi, mockAdminDiagnostics } from './support/adminMock'
import { expectNoMatchingRequest, mockLifecycleLocks, openLifecycle, waitForLifecycleReady } from './support/moduleLifecycle'
import { completeSensitiveEvidence, createApprovedEvidence, logoutAdmin } from './support/securityEvidence'

test.beforeEach(async ({ page }) => {
  await mockAdminApi(page)
})

test('mock E2E diagnostics isolate Vite chunk 404 from business API 404', async ({ page }) => {
  const diagnostics = mockAdminDiagnostics(page)
  await page.route('**/assets/e2e-missing-chunk.js', route => route.fulfill({ status: 404 }))
  await page.route('**/dev/admin/e2e-not-found', route => route.fulfill({ status: 404 }))
  await page.goto('/')

  await page.evaluate(async () => {
    await fetch('/dev/admin/e2e-not-found')
    await new Promise<void>((resolve) => {
      const script = document.createElement('script')
      script.src = '/assets/e2e-missing-chunk.js'
      script.addEventListener('error', () => resolve(), { once: true })
      document.head.append(script)
    })
  })

  await expect.poll(() => diagnostics.viteChunkFailures).toHaveLength(1)
  expect(diagnostics.viteChunkFailures[0]).toContain('/assets/e2e-missing-chunk.js')
  diagnostics.clear()
})

test('module lifecycle reloads fresh state after logout and readonly login', async ({ page }) => {
  await openLifecycle(page)
  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()

  for (const tab of ['执行历史', 'Step 记录', '锁状态', '状态差异', '模块状态']) {
    await page.getByRole('tab', { name: tab }).click()
  }

  await logoutAdmin(page)
  await openLifecycle(page, 'readonly-admin')
  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()
  await expect(page.getByRole('tab', { name: 'Step 记录' })).toHaveCount(0)
  await expect(page.locator('.module-lifecycle-action').getByRole('button', { name: 'Dry-run' })).toHaveCount(0)
})

test('module lifecycle viewing run steps requests the selected run', async ({ page }) => {
  await openLifecycle(page)
  await page.getByRole('tab', { name: '执行历史' }).click()

  const filteredRequest = page.waitForRequest((request) => {
    const url = new URL(request.url())
    return url.pathname.endsWith('/module-lifecycle/steps')
      && url.searchParams.get('run_key') === 'upgrade:platform-rbac:1.0.0'
  })
  await page.getByRole('button', { name: '查看 Step' }).click()
  await filteredRequest

  await expect(page.getByRole('row', { name: /destructive_check.*module:manifest:check/ })).toBeVisible()
})

test('module lifecycle refresh button stays loading until diff request settles', async ({ page }) => {
  await openLifecycle(page)
  let releaseDiff: (() => void) | undefined
  let markDiffRequested: (() => void) | undefined
  const diffPending = new Promise<void>(resolve => releaseDiff = resolve)
  const diffRequested = new Promise<void>(resolve => markDiffRequested = resolve)
  await page.route('**/dev/admin/platform/module-lifecycle/diff', async (route) => {
    markDiffRequested?.()
    await diffPending
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 200, message: 'success', data: { total: 0, list: [] } }),
    })
  })

  const refresh = page.getByRole('button', { name: '刷新' })
  const fastResponses = Promise.all(['state', 'runs', 'steps', 'locks'].map(kind => page.waitForResponse((response) => {
    const url = new URL(response.url())
    return response.request().method() === 'GET' && url.pathname.endsWith(`/module-lifecycle/${kind}`)
  })))
  await refresh.click()
  await diffRequested
  await fastResponses

  try {
    await expect(refresh).toHaveClass(/is-loading/)
  }
  finally {
    releaseDiff?.()
  }
  await expect(refresh).not.toHaveClass(/is-loading/)
})

test('module lifecycle read workflows stay stable across tabs and readonly log permission', async ({ page }) => {
  await mockLifecycleLocks(page)
  await openLifecycle(page)

  await expect(page.getByRole('heading', { name: '模块治理' })).toBeVisible()
  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()
  await expect(page.getByText('module:manifest:check && migrate').first()).toBeVisible()

  await page.getByRole('tab', { name: '执行历史' }).click()
  const runFilter = page.locator('.module-lifecycle-filter').first()
  await runFilter.getByLabel('Owner').fill('e2e')
  const runsRequest = page.waitForRequest(request =>
    request.url().includes('/admin/platform/module-lifecycle/runs')
    && request.method() === 'GET'
    && request.url().includes('owner=e2e'),
  )
  await runFilter.getByRole('button', { name: /搜索|查询/ }).click()
  await runsRequest
  await expect(page.getByRole('row', { name: /platform-rbac/ })).toBeVisible()
  await page.getByRole('button', { name: '查看 Step' }).click()

  await expect(page.getByRole('row', { name: /destructive_check.*module:manifest:check/ })).toBeVisible()
  await expect(page.getByRole('row', { name: /command.*migrate/ })).toBeVisible()

  await page.getByRole('tab', { name: '状态差异' }).click()
  await expect(page.getByRole('row', { name: /platform-rbac.*in_sync/ })).toBeVisible()

  await page.getByRole('tab', { name: '锁状态' }).click()
  await expect(page.getByText('module-lifecycle-lock:all')).toBeVisible()
  await expect(page.getByText('lock-owner-e2e')).toBeVisible()

  await logoutAdmin(page)
  await openLifecycle(page, 'readonly-admin')
  await expect(page.locator('.module-lifecycle-action').getByRole('button', { name: 'Dry-run' })).toHaveCount(0)
  await page.getByRole('tab', { name: '执行历史' }).click()
  await expect(page.getByRole('button', { name: '查看 Step' })).toHaveCount(0)
  await expect(page.getByText('stdout')).toHaveCount(0)
  await expect(page.getByText('stderr')).toHaveCount(0)
  await page.getByRole('tab', { name: '锁状态' }).click()
  await expect(page.getByRole('button', { name: '释放过期锁' })).toHaveCount(0)
})

test('module lifecycle dry-run keeps payload stable and skips evidence dialog', async ({ page }) => {
  await openLifecycle(page)
  const actionForm = page.locator('.module-lifecycle-action')
  const evidenceDialog = page.getByRole('dialog', { name: '敏感操作凭证' })

  const requestPromise = page.waitForRequest(request =>
    request.url().includes('/admin/platform/module-lifecycle/execute')
    && request.method() === 'POST',
  )
  await actionForm.getByRole('button', { name: 'Dry-run' }).click()
  const payload = (await requestPromise).postDataJSON()

  expect(payload).toMatchObject({
    action: 'upgrade',
    execute: false,
    owner: '',
    reason: '',
    confirm_token: '',
    reauth_token: '',
    approval_id: '',
  })
  await expect(evidenceDialog).toBeHidden()
  await expect(page.getByText('Dry-run 结果')).toBeVisible()
  await expect(page.getByRole('row', { name: /Platform RBAC.*已规划/ })).toBeVisible()
})

test('module lifecycle evidence cancellation does not execute', async ({ page }) => {
  await openLifecycle(page)
  const actionForm = page.locator('.module-lifecycle-action')
  await actionForm.locator('.el-switch').click()
  await actionForm.getByLabel('Owner').fill('platform-owner')
  await actionForm.getByLabel('原因').fill('controlled upgrade')
  await actionForm.getByLabel('确认 Token').fill('all:upgrade')
  await actionForm.getByRole('button', { name: '执行' }).click()

  const evidenceDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await evidenceDialog.getByRole('button', { name: '取消' }).click()
  await expect(evidenceDialog).toBeHidden()
  await expectNoMatchingRequest(page, request =>
    request.method() === 'POST' && request.url().includes('/admin/platform/module-lifecycle/execute'),
  )
})

test('module lifecycle execute uses bound evidence and keeps confirm token contract', async ({ page }) => {
  await loginAsAdmin(page)
  const approvalID = await createApprovedEvidence(page, {
    scope: 'module.lifecycle.execute',
    resource: 'module-lifecycle:all:upgrade',
    reason: 'controlled upgrade',
  })
  await page.goto('/#/platform-system/module-lifecycle')
  await waitForLifecycleReady(page)

  const actionForm = page.locator('.module-lifecycle-action')
  await actionForm.locator('.el-switch').click()
  await actionForm.getByLabel('Owner').fill(' platform-owner ')
  await actionForm.getByLabel('原因').fill(' controlled upgrade ')
  await actionForm.getByLabel('确认 Token').fill(' all:upgrade ')
  await actionForm.getByRole('button', { name: '执行' }).click()

  const evidenceDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await expect(evidenceDialog.getByText('module.lifecycle.execute')).toBeVisible()
  await expect(evidenceDialog.getByText('module-lifecycle:all:upgrade')).toBeVisible()
  await completeSensitiveEvidence(page, approvalID)

  const requestPromise = page.waitForRequest(request =>
    request.url().includes('/admin/platform/module-lifecycle/execute')
    && request.method() === 'POST',
  )
  await page.locator('.el-message-box').getByRole('button', { name: /确定|OK/ }).click()
  const payload = (await requestPromise).postDataJSON()

  expect(payload).toMatchObject({
    action: 'upgrade',
    execute: true,
    owner: 'platform-owner',
    reason: 'controlled upgrade',
    confirm_token: 'all:upgrade',
    approval_id: approvalID,
  })
  expect(payload.reauth_token).toMatch(/^reauth-/)
})

test('module lifecycle confirmation cancellation does not release stale locks', async ({ page }) => {
  await mockLifecycleLocks(page)
  await loginAsAdmin(page)
  const approvalID = await createApprovedEvidence(page, {
    scope: 'module.lifecycle.release-lock',
    resource: 'module-lifecycle:stale-locks:all',
    reason: 'release stale module lifecycle locks',
  })
  await page.goto('/#/platform-system/module-lifecycle')
  await waitForLifecycleReady(page)
  await page.getByRole('tab', { name: '锁状态' }).click()

  const lockForm = page.locator('.module-lifecycle-filter').filter({ has: page.getByLabel('锁 Key') })
  await lockForm.locator('.el-switch').click()
  await page.getByLabel('确认 Token').last().fill('release-stale-locks')
  await lockForm.getByRole('button', { name: '释放过期锁' }).click()
  await completeSensitiveEvidence(page, approvalID)
  const confirmDialog = page.locator('.el-message-box')
  await confirmDialog.getByRole('button', { name: '取消' }).click()
  await expect(confirmDialog).toBeHidden()
  await expectNoMatchingRequest(page, request =>
    request.method() === 'POST' && request.url().includes('/admin/platform/module-lifecycle/locks/release-stale'),
  )
})

test('module lifecycle stale-lock release uses bound evidence and keeps confirm token contract', async ({ page }) => {
  await mockLifecycleLocks(page)
  await loginAsAdmin(page)
  const approvalID = await createApprovedEvidence(page, {
    scope: 'module.lifecycle.release-lock',
    resource: 'module-lifecycle:stale-locks:all',
    reason: 'release stale module lifecycle locks',
  })
  await page.goto('/#/platform-system/module-lifecycle')
  await waitForLifecycleReady(page)
  await page.getByRole('tab', { name: '锁状态' }).click()
  await expect(page.getByText('module-lifecycle-lock:all')).toBeVisible()

  const lockForm = page.locator('.module-lifecycle-filter').filter({ has: page.getByLabel('锁 Key') })
  await lockForm.locator('.el-switch').click()
  await page.getByLabel('确认 Token').last().fill('release-stale-locks')
  await lockForm.getByRole('button', { name: '释放过期锁' }).click()

  const evidenceDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await expect(evidenceDialog.getByText('module.lifecycle.release-lock')).toBeVisible()
  await expect(evidenceDialog.getByText('module-lifecycle:stale-locks:all')).toBeVisible()
  await completeSensitiveEvidence(page, approvalID)

  const requestPromise = page.waitForRequest(request =>
    request.url().includes('/admin/platform/module-lifecycle/locks/release-stale')
    && request.method() === 'POST',
  )
  await page.locator('.el-message-box').getByRole('button', { name: /确定|OK/ }).click()
  const payload = (await requestPromise).postDataJSON()

  expect(payload).toMatchObject({
    dry_run: false,
    confirm_token: 'release-stale-locks',
    approval_id: approvalID,
  })
  expect(payload.reauth_token).toMatch(/^reauth-/)
})
