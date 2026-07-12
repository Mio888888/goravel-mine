import { expect, test } from '@playwright/test'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'

test('golden reference module supports CRUD smoke', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', error => pageErrors.push(error.message))
  await mockAdminApi(page)
  await loginAsAdmin(page)
  await page.goto('/#/platform-system/reference-case')

  await expect(page.getByText('Golden Reference Case')).toBeVisible()
  await page.getByRole('textbox', { name: '案例编码' }).fill('golden')
  await page.getByRole('button', { name: '搜索' }).click()
  await expect(page.getByText('golden-case')).toBeVisible()
  await page.getByRole('button', { name: '重置' }).click()

  await page.getByRole('button', { name: '新增' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('textbox', { name: '案例编码' }).fill('e2e-case')
  await dialog.getByRole('textbox', { name: '案例标题' }).fill('E2E Reference Case')
  await dialog.getByRole('textbox', { name: '版本' }).fill('1.0.1')
  await dialog.getByRole('textbox', { name: 'Payload JSON' }).fill('{"scenario":"canary"}')
  await dialog.getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('E2E Reference Case')).toBeVisible()

  await page.getByRole('row', { name: /E2E Reference Case/ }).getByRole('button', { name: '编辑' }).click()
  const editDialog = page.getByRole('dialog')
  await editDialog.getByRole('textbox', { name: '案例标题' }).fill('E2E Reference Case Updated')
  await editDialog.getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('E2E Reference Case Updated')).toBeVisible()

  await page.getByRole('row', { name: /E2E Reference Case Updated/ }).getByRole('button', { name: '删除' }).click()
  await page.getByRole('button', { name: '确定' }).click()
  await expect(page.getByText('E2E Reference Case Updated')).toHaveCount(0)
  expect(pageErrors).toEqual([])
})

test('golden reference module absorbs invalid submit and cancelled delete', async ({ page }) => {
  const pageErrors: string[] = []
  page.on('pageerror', error => pageErrors.push(error.message))
  await mockAdminApi(page)
  await loginAsAdmin(page)
  await page.goto('/#/platform-system/reference-case')

  await page.getByRole('button', { name: '新增' }).click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '确定' }).click()
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '取消' }).click()

  await page.getByRole('row', { name: /Golden Reference Case/ }).getByRole('button', { name: '删除' }).click()
  await page.getByRole('button', { name: '取消' }).click()
  await expect(page.getByText('Golden Reference Case')).toBeVisible()
  expect(pageErrors).toEqual([])
})
