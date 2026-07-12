import { expect, test } from '@playwright/test'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'
import { createApprovedEvidence } from './support/securityEvidence'

test('SSO authorization callback enters tenant dashboard', async ({ page }) => {
  await mockAdminApi(page, { entryMode: 'tenant' })
  await page.goto('/#/login?auth_scope=tenant')

  const callbackResponse = page.waitForResponse(response =>
    response.url().includes('/passport/sso/callback') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: /Okta Tenant/ }).click()
  await callbackResponse

  await expect(page).not.toHaveURL(/\/login/)
  await expect(page.getByText('租户管理').first()).toBeVisible()
})

test('MFA recovery boundary keeps shell locked until valid code', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page, { username: 'mfa-admin', password: '123456', waitForShell: false })

  const dialog = page.getByRole('dialog', { name: /MFA|验证/ })
  await expect(dialog).toBeVisible()
  await dialog.getByPlaceholder(/MFA|验证码|恢复码/).fill('000000')
  const rejectedMFA = page.waitForResponse(response =>
    response.url().includes('/passport/mfa/login') && response.request().method() === 'POST',
  )
  await dialog.getByRole('button', { name: /确定|OK/ }).click()
  await rejectedMFA
  await expect(dialog).toBeVisible()
  await expect(dialog.getByPlaceholder(/MFA|验证码|恢复码/)).toHaveValue('')

  await dialog.getByPlaceholder(/MFA|验证码|恢复码/).fill('654321')
  await dialog.getByRole('button', { name: /确定|OK/ }).click()
  await expect(page).not.toHaveURL(/\/login/)
})

test('token refresh failure rejects pending requests and forces relogin', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', error => pageErrors.push(error.message))
  await mockAdminApi(page, {
    expireTenantListRequests: 3,
    expireTenantListDelayMs: [0, 20, 700],
    failRefreshOnce: true,
    refreshFailureDelayMs: 100,
    logoutDelayMs: 100,
  })
  await loginAsAdmin(page)

  const outcomes = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    return Promise.race([
      Promise.allSettled([
        client.get('/admin/platform/tenant/list?probe=1'),
        client.get('/admin/platform/tenant/list?probe=2'),
        client.get('/admin/platform/tenant/list?probe=3'),
      ]),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('concurrent 401 requests did not settle')), 2000)),
    ])
  })

  expect(outcomes).toHaveLength(3)
  for (const outcome of outcomes) {
    expect(outcome.status).toBe('rejected')
    if (outcome.status === 'rejected') {
      expect(outcome.reason).toMatchObject({ code: 401 })
    }
  }
  await expect(page).toHaveURL(/\/login/)
  expect(pageErrors).toEqual([])
})

test('late unauthorized response retries current token without a second refresh', async ({ page }) => {
  let refreshRequests = 0
  await mockAdminApi(page, {
    expireTenantListRequests: 3,
    expireTenantListDelayMs: [0, 20, 700],
    onRefreshRequest: () => refreshRequests++,
  })
  await loginAsAdmin(page)

  const outcomes = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    return Promise.race([
      Promise.all([
        client.get('/admin/platform/tenant/list?probe=late-1'),
        client.get('/admin/platform/tenant/list?probe=late-2'),
        client.get('/admin/platform/tenant/list?probe=late-3'),
      ]).then(responses => responses.map(response => response.code)),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('late 401 requests did not settle')), 3000)),
    ])
  })

  expect(outcomes).toEqual([200, 200, 200])
  expect(refreshRequests).toBe(1)
})

test('anonymous unauthorized responses remain rejected', async ({ page }) => {
  let tenantListRequests = 0
  await mockAdminApi(page, {
    expireTenantListRequests: 1,
    onTenantListRequest: () => tenantListRequests++,
  })
  await loginAsAdmin(page)
  await page.locator('.mine-user-bar').click()
  await page.getByText('退出系统').click()
  await expect(page).toHaveURL(/\/login/)

  const outcome = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    try {
      await client.get('/admin/platform/tenant/list?probe=anonymous')
      return 'resolved'
    }
    catch (error: any) {
      return {
        code: error?.code,
        message: error?.message,
        name: error?.name,
        type: typeof error,
        value: String(error),
      }
    }
  })

  expect(outcome).toMatchObject({ code: 401 })
  expect(tenantListRequests).toBe(1)
})

test('protected mocks reject anonymous module and reference APIs', async ({ page }) => {
  await mockAdminApi(page)
  await page.goto('/#/login?auth_scope=platform')

  const outcomes = await page.evaluate(async () => {
    return Promise.all([
      '/admin/platform/module-lifecycle/state',
      '/admin/platform/reference-case/list',
    ].map(async url => (await fetch(`/dev${url}`)).json().then(body => body.code)))
  })

  expect(outcomes).toEqual([401, 401])
})

test('mock auth isolates token identity and logout revokes only caller', async ({ page }) => {
  await mockAdminApi(page)
  await page.goto('/#/login?auth_scope=platform')

  const result = await page.evaluate(async () => {
    async function login(username: string) {
      const response = await fetch('/dev/admin/platform/passport/login', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ username, password: '123456' }),
      })
      return (await response.json()).data as { access_token: string }
    }
    async function info(token: string) {
      const response = await fetch('/dev/admin/platform/passport/getInfo', {
        headers: { authorization: `Bearer ${token}` },
      })
      return response.json()
    }

    const admin = await login('admin')
    const approver = await login('approver-admin')
    const adminBeforeLogout = await info(admin.access_token)
    await fetch('/dev/admin/platform/passport/logout', {
      method: 'POST',
      headers: { authorization: `Bearer ${approver.access_token}` },
    })
    const adminAfterLogout = await info(admin.access_token)
    const approverAfterLogout = await info(approver.access_token)
    return { adminBeforeLogout, adminAfterLogout, approverAfterLogout }
  })

  expect(result.adminBeforeLogout.data.username).toBe('admin')
  expect(result.adminAfterLogout.data.username).toBe('admin')
  expect(result.approverAfterLogout.code).toBe(401)
})

test('missing refresh token does not poison later refresh attempts', async ({ page }) => {
  await mockAdminApi(page, { expireTenantListRequests: 2 })
  await loginAsAdmin(page)

  await page.evaluate(() => window.localStorage.removeItem('mine_refresh_token'))
  const firstOutcome = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    try {
      await client.get('/admin/platform/tenant/list?probe=missing-refresh-token')
      return 'resolved'
    }
    catch (error: any) {
      return { code: error?.code }
    }
  })
  expect(firstOutcome).toEqual({ code: 401 })
  await expect(page).toHaveURL(/\/login/)

  await loginAsAdmin(page)
  const secondCode = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    return Promise.race([
      client.get('/admin/platform/tenant/list?probe=after-relogin').then(response => response.code),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('refresh state remained stuck')), 2000)),
    ])
  })
  expect(secondCode).toBe(200)
})

test('request rejects when refreshed token is still unauthorized', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page)
  await page.route('**/dev/admin/platform/tenant/list?probe=refresh-rejected', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ code: 401, message: 'still unauthorized', data: [] }),
    })
  })

  const outcome = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    return Promise.race([
      client.get('/admin/platform/tenant/list?probe=refresh-rejected').then(
        () => 'resolved',
        (error: any) => ({ code: error?.code }),
      ),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error('retried 401 request did not settle')), 2000)),
    ])
  })

  expect(outcome).toEqual({ code: 401 })
  await expect(page).toHaveURL(/\/login/)
})

test('unauthorized batch operations stay hidden across enterprise tables', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page, { username: 'readonly-admin' })
  await page.goto('/#/platform/tenant')

  await expect(page.getByText('Acme 租户')).toBeVisible()
  await expect(page.getByRole('button', { name: '新增' })).toHaveCount(0)
  await expect(page.getByText('权限分配')).toHaveCount(0)
  await expect(page.getByText('用量')).toHaveCount(0)
})

test('complex table filtering preserves query state after navigation', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page)
  await page.goto('/#/platform/tenant')

  const searchInput = page.getByRole('textbox', { name: '租户名称' })
  await searchInput.fill('Acme')
  await page.getByRole('button', { name: /搜索|查询/ }).click()
  await expect(page.getByText('Acme 租户')).toBeVisible()

  await page.goto('/#/platform-system/module-lifecycle')
  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()
  await page.goBack()
  await expect(searchInput).toHaveValue('Acme')
})

test('critical admin smoke covers module lifecycle run and step detail', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page)
  await page.goto('/#/platform-system/module-lifecycle')

  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()
  await page.getByText('执行历史').click()
  await expect(page.getByRole('row', { name: /platform-rbac/ })).toBeVisible()
  await page.getByRole('button', { name: '查看 Step' }).click()
  await expect(page.getByRole('row', { name: /destructive_check.*module:manifest:check/ })).toBeVisible()
  await expect(page.getByRole('row', { name: /command.*migrate/ })).toBeVisible()
})

test('module lifecycle obtains bound evidence before execution', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page)
  const approvalID = await createApprovedEvidence(page, {
    scope: 'module.lifecycle.execute',
    resource: 'module-lifecycle:all:upgrade',
    reason: 'controlled upgrade',
  })
  await page.goto('/#/platform-system/module-lifecycle')

  const form = page.locator('.module-lifecycle-action')
  await form.locator('.el-switch').click()
  await form.getByLabel('Owner').fill(' platform-owner ')
  await form.getByLabel('原因').fill(' controlled upgrade ')
  await form.getByLabel('确认 Token').fill(' all:upgrade ')
  await form.getByRole('button', { name: '执行' }).click()

  const evidenceDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await evidenceDialog.getByLabel('审批 ID').fill(approvalID)
  await evidenceDialog.getByRole('button', { name: '加载审批' }).click()
  await evidenceDialog.getByLabel('当前密码').fill('123456')
  await evidenceDialog.getByRole('button', { name: '二次认证' }).click()
  await evidenceDialog.getByRole('button', { name: '继续操作' }).click()

  const requestPromise = page.waitForRequest(request =>
    request.url().includes('/admin/platform/module-lifecycle/execute')
    && request.method() === 'POST',
  )
  await page.getByRole('dialog').getByRole('button', { name: /确定|OK/ }).click()
  const payload = (await requestPromise).postDataJSON()

  expect(payload).toMatchObject({
    owner: 'platform-owner',
    reason: 'controlled upgrade',
    confirm_token: 'all:upgrade',
    approval_id: approvalID,
  })
  expect(payload.reauth_token).toMatch(/^reauth-/)
})

test('readonly module lifecycle user cannot inspect step command output', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page, { username: 'readonly-admin' })
  await page.goto('/#/platform-system/module-lifecycle')

  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()
  await page.getByText('执行历史').click()
  await expect(page.getByRole('row', { name: /platform-rbac/ })).toBeVisible()
  await expect(page.getByRole('button', { name: '查看 Step' })).toHaveCount(0)
  await expect(page.getByText('stdout')).toHaveCount(0)
  await expect(page.getByText('stderr')).toHaveCount(0)
})
