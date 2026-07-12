import type { Page } from '@playwright/test'
import { expect } from '@playwright/test'
import { loginAsAdmin } from './adminMock'

export interface ApprovalEvidenceRequest {
  scope: string
  resource: string
  reason: string
}

export async function logoutAdmin(page: Page) {
  await page.locator('.mine-user-bar').click()
  await page.getByText('退出系统').click()
  await expect(page).toHaveURL(/\/login/)
}

export async function createApprovedEvidence(page: Page, request: ApprovalEvidenceRequest) {
  const approvalID = await page.evaluate(async (payload) => {
    const client = (await import('/src/utils/http.ts')).default.http
    const response = await client.post('/admin/platform/security/approvals', payload)
    return response.data.approval_id as string
  }, request)

  await logoutAdmin(page)
  await loginAsAdmin(page, { username: 'approver-admin' })
  await page.evaluate(async (id) => {
    const client = (await import('/src/utils/http.ts')).default.http
    await client.put(`/admin/platform/security/approvals/${id}/approve`)
  }, approvalID)
  await logoutAdmin(page)
  await loginAsAdmin(page)
  return approvalID
}

export async function completeSensitiveEvidence(page: Page, approvalID: string) {
  const dialog = page.getByRole('dialog', { name: '敏感操作凭证' })
  await dialog.getByLabel('审批 ID').fill(approvalID)
  await dialog.getByRole('button', { name: '加载审批' }).click()
  await dialog.getByLabel('当前密码').fill('123456')
  await dialog.getByRole('button', { name: '二次认证' }).click()
  await dialog.getByRole('button', { name: '继续操作' }).click()
}
