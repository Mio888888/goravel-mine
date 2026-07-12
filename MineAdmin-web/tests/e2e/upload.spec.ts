import { expect, test } from '@playwright/test'
import { Buffer } from 'node:buffer'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'

test.beforeEach(async ({ page }) => {
  await mockAdminApi(page)
})

test('attachment upload refreshes resource list with uploaded file', async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto('/#/data-center/attachment')

  await expect(page.getByText('contract.pdf')).toBeVisible()
  await page.locator('input[name="local-image-upload"]').setInputFiles({
    name: 'uploaded.png',
    mimeType: 'image/png',
    buffer: Buffer.from('e2e image'),
  })

  await expect(page.getByText('uploaded.png')).toBeVisible()
})
