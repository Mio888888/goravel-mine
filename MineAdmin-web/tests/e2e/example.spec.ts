import { expect, test } from '@playwright/test'
import { Buffer } from 'node:buffer'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'

test.beforeEach(async ({ page }) => {
  await mockAdminApi(page)
})

test('auth guard redirects anonymous users to login', async ({ page }) => {
  await page.goto('/#/platform/tenant')

  await expect(page).toHaveURL(/\/login/)
  await expect(page.getByText('账户')).toBeVisible()
  await expect(page.getByText('密码')).toBeVisible()
  await expect(page.getByText('验证码')).toBeVisible()
})

test('login loads platform menu and tenant table', async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto('/#/platform/tenant')

  await expect(page.getByText('租户管理').first()).toBeVisible()
  await expect(page.getByText('Acme 租户')).toBeVisible()
  await expect(page.getByText('tenant_acme')).toBeVisible()
  await expect(page.getByRole('button', { name: '新增' }).first()).toBeVisible()
})

test('tenant CRUD surface opens create dialog and usage dialog', async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto('/#/platform/tenant')

  await page.getByRole('button', { name: '新增' }).first().click()
  const createDialog = page.getByRole('dialog')
  await expect(createDialog).toContainText('新增')
  await expect(createDialog.locator('.el-form-item__label', { hasText: '租户编码' }).first()).toBeVisible()
  await page.getByRole('button', { name: '取消' }).click()
  await expect(createDialog).toBeHidden()

  await page.getByRole('row', { name: /Acme 租户/ }).getByText('用量').click()
  await expect(page.getByRole('dialog')).toContainText('用量')
  await expect(page.getByText('用户用量: 3 / 50')).toBeVisible()
})

test('tenant destroy obtains approval and bound re-auth evidence', async ({ page }) => {
  let destroyPayload: Record<string, unknown> = {}
  await page.unrouteAll()
  await mockAdminApi(page, {
    onTenantDestroyRequest: payload => destroyPayload = payload,
  })
  await loginAsAdmin(page)
  await page.goto('/#/platform/tenant')

  await page.getByRole('row', { name: /Acme 租户/ }).getByText('销毁').click()
  const requesterDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await requesterDialog.getByRole('button', { name: '申请审批' }).click()
  await expect(requesterDialog.getByLabel('审批 ID')).toHaveValue('approval-1')
  await expect(requesterDialog.getByText('待审批', { exact: true })).toBeVisible()
  await requesterDialog.getByRole('button', { name: '取消' }).click()

  await page.locator('.mine-user-bar').click()
  await page.getByText('退出系统').click()
  await loginAsAdmin(page, { username: 'approver-admin' })
  await page.goto('/#/platform/tenant')
  await page.getByRole('row', { name: /Acme 租户/ }).getByText('销毁').click()
  const approverDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await approverDialog.getByLabel('审批 ID').fill('approval-1')
  await approverDialog.getByRole('button', { name: '加载审批' }).click()
  await approverDialog.getByRole('button', { name: '批准审批' }).click()
  await expect(approverDialog.getByText('已批准', { exact: true })).toBeVisible()
  await approverDialog.getByRole('button', { name: '取消' }).click()

  await page.locator('.mine-user-bar').click()
  await page.getByText('退出系统').click()
  await loginAsAdmin(page)
  await page.goto('/#/platform/tenant')
  await page.getByRole('row', { name: /Acme 租户/ }).getByText('销毁').click()
  const evidenceDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await evidenceDialog.getByLabel('审批 ID').fill('approval-1')
  await evidenceDialog.getByRole('button', { name: '加载审批' }).click()
  await evidenceDialog.getByLabel('当前密码').fill('123456')
  await evidenceDialog.getByRole('button', { name: '二次认证' }).click()
  await expect(evidenceDialog.getByText('二次认证已就绪')).toBeVisible()
  await evidenceDialog.getByRole('button', { name: '继续操作' }).click()
  const destroyConfirm = page.getByRole('dialog').filter({ hasText: '确认销毁所选租户吗' })
  const destroyResponse = page.waitForResponse(response =>
    response.url().includes('/dev/admin/platform/tenant')
    && response.request().method() === 'DELETE',
  )
  await destroyConfirm.getByRole('button', { name: '确定' }).click()

  expect((await (await destroyResponse).json()).code).toBe(200)
  await expect(page.getByText('租户已销毁')).toBeVisible()

  await expect.poll(() => destroyPayload).toMatchObject({
    ids: [1001],
    confirm_code: 'acme',
    approval_id: 'approval-1',
  })
  expect(String(destroyPayload.reauth_token)).toMatch(/^reauth-/)
})

test('tenant destroy supports governance policy without approval', async ({ page }) => {
  let destroyPayload: Record<string, unknown> = {}
  await page.unrouteAll()
  await mockAdminApi(page, {
    tenantDeletionRequiresApproval: false,
    onTenantDestroyRequest: payload => destroyPayload = payload,
  })
  await loginAsAdmin(page)
  await page.goto('/#/platform/tenant')

  await page.getByRole('row', { name: /Acme 租户/ }).getByText('销毁').click()
  const evidenceDialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await expect(evidenceDialog.getByLabel('审批 ID')).toHaveCount(0)
  await evidenceDialog.getByLabel('当前密码').fill('123456')
  await evidenceDialog.getByRole('button', { name: '二次认证' }).click()
  await evidenceDialog.getByRole('button', { name: '继续操作' }).click()

  const destroyResponse = page.waitForResponse(response =>
    response.url().includes('/dev/admin/platform/tenant')
    && response.request().method() === 'DELETE',
  )
  await page.getByRole('dialog').filter({ hasText: '确认销毁所选租户吗' }).getByRole('button', { name: '确定' }).click()
  const response = await destroyResponse

  expect((await response.json()).code).toBe(200)
  expect(destroyPayload).toMatchObject({ ids: [1001], confirm_code: 'acme' })
  expect(destroyPayload.approval_id).toBeUndefined()
  expect(String(destroyPayload.reauth_token)).toMatch(/^reauth-/)
})

test('tenant destroy callback only observes accepted requests', async ({ page }) => {
  let destroyPayload: Record<string, unknown> | undefined
  await page.unrouteAll()
  await mockAdminApi(page, {
    onTenantDestroyRequest: payload => destroyPayload = payload,
  })
  await loginAsAdmin(page)

  const code = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    try {
      await client.delete('/admin/platform/tenant', { data: { ids: [1001], confirm_code: 'acme' } })
      return 200
    }
    catch (error: any) {
      return error?.code
    }
  })

  expect(code).toBe(422)
  expect(destroyPayload).toBeUndefined()
})

test('attachment page lists resources and accepts local upload', async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto('/#/data-center/attachment')

  await expect(page.getByText('contract.pdf')).toBeVisible()
  await expect(page.getByText('avatar.png')).toBeVisible()

  const uploadRequest = page.waitForRequest(request =>
    request.url().includes('/dev/admin/attachment/upload') && request.method() === 'POST',
  )
  await page.locator('input[name="local-image-upload"]').setInputFiles({
    name: 'uploaded.png',
    mimeType: 'image/png',
    buffer: Buffer.from('e2e image'),
  })
  await uploadRequest
})

test('SSO provider page opens mapping configuration tabs', async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto('/#/security/sso/provider')

  await expect(page.getByText('单点登录配置').first()).toBeVisible()
  await expect(page.getByText('Okta Admin')).toBeVisible()
  await page.getByRole('button', { name: '新增' }).first().click()

  await expect(page.getByRole('dialog')).toContainText('新增')
  await expect(page.getByRole('tab', { name: '基础信息' })).toBeVisible()
  await page.getByRole('tab', { name: '角色映射' }).click()
  await expect(page.getByText('角色映射').last()).toBeVisible()
  await page.getByRole('tab', { name: '数据权限映射' }).click()
  await expect(page.getByText('数据权限映射').last()).toBeVisible()
})
