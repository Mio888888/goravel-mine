import { expect, test } from '@playwright/test'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'

test('MFA challenge requires code before entering admin shell', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page, { username: 'mfa-admin', password: '123456', waitForShell: false })

  const dialog = page.getByRole('dialog', { name: /MFA|验证/ })
  await expect(dialog).toBeVisible()
  await dialog.getByPlaceholder(/MFA|验证码|恢复码/).fill('654321')
  await dialog.getByRole('button', { name: /确定|OK/ }).click()

  await expect(page).not.toHaveURL(/\/login/)
  await expect(page.getByText('平台管理').first()).toBeVisible()
})

test('tenant SSO authorization callback enters tenant dashboard', async ({ page }) => {
  await mockAdminApi(page, { entryMode: 'tenant' })
  await page.goto('/#/login?auth_scope=tenant')
  await expect(page.getByRole('button', { name: /Okta Tenant/ })).toBeVisible()

  const callbackResponse = page.waitForResponse(response =>
    response.url().includes('/passport/sso/callback') && response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: /Okta Tenant/ }).click()
  await callbackResponse

  await expect(page).not.toHaveURL(/\/login/)
  await expect(page.getByText('租户管理').first()).toBeVisible()
})

test('token refresh includes CSRF token when CSRF is enabled', async ({ page }) => {
  const refreshHeaders: Record<string, string>[] = []
  await mockAdminApi(page, {
    expireTenantListOnce: true,
    requireRefreshCsrf: true,
    onRefreshRequest: headers => refreshHeaders.push(headers),
  })

  await loginAsAdmin(page)
  await page.goto('/#/platform/tenant')

  await expect(page.getByText('Acme 租户')).toBeVisible()
  expect(refreshHeaders).toHaveLength(1)
  expect(refreshHeaders[0]['x-csrf-token']).toBe('e2e-csrf-token')
})

test('logout includes CSRF token when CSRF is enabled', async ({ page }) => {
  const logoutHeaders: Record<string, string>[] = []
  await mockAdminApi(page, {
    requireLogoutCsrf: true,
    onLogoutRequest: headers => logoutHeaders.push(headers),
  })

  await loginAsAdmin(page)
  await page.locator('.mine-user-bar').click()
  await page.getByText('退出系统').click()

  await expect(page).toHaveURL(/\/login/)
  expect(logoutHeaders).toHaveLength(1)
  expect(logoutHeaders[0]['x-csrf-token']).toBe('e2e-csrf-token')
})
