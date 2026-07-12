import { Buffer } from 'node:buffer'
import { expect, test } from '@playwright/test'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'
import {
  completeSensitiveEvidence,
  createApprovedEvidence,
} from './support/securityEvidence'

test.beforeEach(async ({ page }) => {
  await mockAdminApi(page)
})

test('tenant permissions can be reviewed and saved', async ({ page }) => {
  await loginAsAdmin(page)
  const approvalID = await createApprovedEvidence(page, {
    scope: 'tenant.permissions.sync',
    resource: tenantChangeResource('permissions', {
      allowed: ['tenant:permission', 'tenant:role', 'tenant:user'],
    }),
    reason: 'Sync tenant 1001 permissions',
  })
  await page.goto('/#/platform/tenant')

  await page
    .getByRole('row', { name: /Acme 租户/ })
    .getByText('权限分配')
    .click()
  const dialog = page.getByRole('dialog', { name: /租户权限|权限分配/ })
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: '展开' }).click()
  await expect(dialog.getByText('用户管理')).toBeVisible()
  await dialog.getByText('角色管理').click()
  await dialog.getByRole('button', { name: /确定|OK/ }).click()

  const saveRequest = page.waitForRequest(
    request =>
      request.url().includes('/dev/admin/platform/tenant/1001/permissions')
      && request.method() === 'PUT',
  )
  await completeSensitiveEvidence(page, approvalID)
  await saveRequest
  await expect(dialog).toBeHidden()
})

test('tenant status actions call suspend endpoint after confirmation', async ({
  page,
}) => {
  await loginAsAdmin(page)
  const approvalID = await createApprovedEvidence(page, {
    scope: 'tenant.status.change',
    resource: tenantChangeResource('status', 2),
    reason: 'Change tenant 1001 status',
  })
  await page.goto('/#/platform/tenant')

  await page
    .getByRole('row', { name: /Acme 租户/ })
    .getByText('挂起')
    .click()
  await page.getByRole('button', { name: /确定|OK/ }).click()
  const suspendRequest = page.waitForRequest(
    request =>
      request.url().includes('/dev/admin/platform/tenant/1001/suspend')
      && request.method() === 'PUT',
  )
  await completeSensitiveEvidence(page, approvalID)
  await suspendRequest
})

function tenantChangeResource(kind: string, desired: unknown) {
  const json = JSON.stringify({ tenant_id: 1001, desired })
  return `tenant-change:${kind}:${Buffer.from(json).toString('base64url')}`
}
