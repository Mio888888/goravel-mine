import { expect, test } from '@playwright/test'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'

test.beforeEach(async ({ page }) => {
  await mockAdminApi(page)
})

test('scheduled task can run, toggle, and show execution logs', async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto('/#/platform-system/scheduled-task')

  await expect(page.getByText('Nightly Backup')).toBeVisible()
  const row = page.getByRole('row', { name: /Nightly Backup/ })

  const runRequest = page.waitForRequest(request =>
    request.url().includes('/dev/admin/platform/scheduled-task/4001/run') && request.method() === 'POST',
  )
  await row.getByText('执行', { exact: true }).click()
  await runRequest

  const disableRequest = page.waitForRequest(request =>
    request.url().includes('/dev/admin/platform/scheduled-task/4001/disable') && request.method() === 'PUT',
  )
  await row.getByText('禁用').click()
  await disableRequest

  await row.getByText('执行日志', { exact: true }).click()
  const drawer = page.getByRole('dialog', { name: /日志|Nightly Backup/ })
  await expect(drawer).toBeVisible()
  await expect(drawer.getByText('manual')).toBeVisible()
  await expect(drawer.getByText('backup completed')).toBeVisible()
})
