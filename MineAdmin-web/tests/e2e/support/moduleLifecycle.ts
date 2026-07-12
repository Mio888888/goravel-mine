import type { Page, Request } from '@playwright/test'
import { expect } from '@playwright/test'
import { loginAsAdmin } from './adminMock'

const NO_REQUEST_WINDOW_MS = 250

export async function openLifecycle(page: Page, username: 'admin' | 'readonly-admin' = 'admin') {
  await loginAsAdmin(page, { username })
  await page.goto('/#/platform-system/module-lifecycle')
  await waitForLifecycleReady(page)
}

export async function waitForLifecycleReady(page: Page) {
  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()
}

export async function mockLifecycleLocks(page: Page) {
  await page.route('**/dev/admin/platform/module-lifecycle/locks', async (route) => {
    if (route.request().method() !== 'GET') {
      await route.fallback()
      return
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'success',
        data: {
          total: 1,
          list: [{
            id: 1,
            key: 'module-lifecycle-lock:all',
            owner: 'lock-owner-e2e',
            run_key: 'upgrade:platform-rbac:1.0.0',
            expires_at: '2026-07-09T10:05:00Z',
            updated_at: '2026-07-09T10:04:00Z',
          }],
        },
      }),
    })
  })
}

export async function expectNoMatchingRequest(page: Page, matches: (request: Request) => boolean) {
  try {
    await page.waitForRequest(matches, { timeout: NO_REQUEST_WINDOW_MS })
    throw new Error('Unexpected matching request')
  }
  catch (error) {
    if (!(error instanceof Error) || error.name !== 'TimeoutError') {
      throw error
    }
  }
}
