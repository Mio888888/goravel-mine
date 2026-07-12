import { expect, test } from '@playwright/test'
import { loginAsAdmin, mockAdminApi } from './support/adminMock'

test.beforeEach(async ({ page }) => {
  await mockAdminApi(page)
})

test('menu permissions hide unauthorized enterprise operations', async ({ page }) => {
  await loginAsAdmin(page, { username: 'readonly-admin' })
  await page.goto('/#/platform/tenant')

  await expect(page.getByText('Acme 租户')).toBeVisible()
  await expect(page.getByRole('button', { name: '新增' })).toHaveCount(0)
  await expect(page.getByText('权限分配')).toHaveCount(0)
  await expect(page.getByText('用量')).toHaveCount(0)
})

test('authorized platform menus expose scheduled task route and actions', async ({ page }) => {
  await loginAsAdmin(page)
  await page.goto('/#/platform-system/scheduled-task')

  await expect(page.getByText('计划任务').first()).toBeVisible()
  await expect(page.getByText('Nightly Backup')).toBeVisible()
  await expect(page.getByRole('button', { name: '新增' }).first()).toBeVisible()
  await expect(page.getByText('执行').first()).toBeVisible()
})

test('readonly scheduled task user cannot see write operations', async ({ page }) => {
  await loginAsAdmin(page, { username: 'readonly-admin' })
  await page.goto('/#/platform-system/scheduled-task')

  await expect(page.getByText('Nightly Backup')).toBeVisible()
  await expect(page.getByRole('button', { name: '新增' })).toHaveCount(0)
  await expect(page.getByText('编辑', { exact: true })).toHaveCount(0)
  await expect(page.getByText('执行', { exact: true })).toHaveCount(0)
  await expect(page.getByText('执行日志', { exact: true })).toHaveCount(0)
  await expect(page.getByText('删除', { exact: true })).toHaveCount(0)
})

test('readonly user direct sensitive API calls are forbidden', async ({ page }) => {
  await loginAsAdmin(page, { username: 'readonly-admin' })

  const codes = await page.evaluate(async () => {
    const client = (await import('/src/utils/http.ts')).default.http
    const call = async (request: () => Promise<unknown>) => {
      try {
        await request()
        return 200
      }
      catch (error: any) {
        return error?.code
      }
    }
    return Promise.all([
      call(() => client.post('/admin/platform/security/reauth-token', {
        password: '123456',
        operation: 'tenant.data.delete',
        resource: 'tenant-data:delete:1001:metadata',
      })),
      call(() => client.post('/admin/platform/security/approvals', {
        scope: 'tenant.data.delete',
        resource: 'tenant-data:delete:1001:metadata',
        reason: 'unauthorized probe',
      })),
      call(() => client.get('/admin/platform/module-lifecycle/steps')),
      call(() => client.post('/admin/platform/module-lifecycle/execute', { module_id: 'all', action: 'upgrade', execute: true })),
      call(() => client.delete('/admin/platform/tenant', { data: { ids: [1001], confirm_code: 'acme' } })),
      call(() => client.post('/admin/platform/reference-case', { code: 'forbidden', title: 'Forbidden' })),
      call(() => client.put('/admin/platform/reference-case/1', { title: 'Forbidden' })),
      call(() => client.delete('/admin/platform/reference-case', { data: [1] })),
    ])
  })

  expect(codes).toEqual([403, 403, 403, 403, 403, 403, 403, 403])
})
