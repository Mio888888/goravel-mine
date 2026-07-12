import type { Page, Request, Response, Route } from '@playwright/test'

export type LoginUser = 'admin' | 'approver-admin' | 'readonly-admin' | 'mfa-admin' | 'tenant-sso'

export interface MockAdminOptions {
  entryMode?: 'platform' | 'tenant'
  expireTenantListOnce?: boolean
  expireTenantListRequests?: number
  expireTenantListDelayMs?: number[]
  failRefreshOnce?: boolean
  refreshFailureDelayMs?: number
  logoutDelayMs?: number
  onRefreshRequest?: (headers: Record<string, string>) => void
  onLogoutRequest?: (headers: Record<string, string>) => void
  requireRefreshCsrf?: boolean
  requireLogoutCsrf?: boolean
  tenantDeletionRequiresApproval?: boolean
  onTenantDestroyRequest?: (payload: Record<string, unknown>) => void
  onTenantListRequest?: () => void
}

export interface LoginOptions {
  username?: LoginUser
  password?: string
  waitForShell?: boolean
}

export interface MockAdminDiagnostics {
  pageErrors: string[]
  failedRequests: string[]
  viteChunkFailures: string[]
  clear: () => void
}

const diagnosticsByPage = new WeakMap<Page, MockAdminDiagnostics>()

export function installMockAdminDiagnostics(page: Page) {
  const existingDiagnostics = diagnosticsByPage.get(page)
  if (existingDiagnostics) {
    return existingDiagnostics
  }
  const diagnostics: MockAdminDiagnostics = {
    pageErrors: [],
    failedRequests: [],
    viteChunkFailures: [],
    clear: () => {
      diagnostics.pageErrors.length = 0
      diagnostics.failedRequests.length = 0
      diagnostics.viteChunkFailures.length = 0
    },
  }
  diagnosticsByPage.set(page, diagnostics)
  page.on('pageerror', error => diagnostics.pageErrors.push(error.message))
  page.on('requestfailed', (request) => {
    if (isUnexpectedRequestFailure(request)) {
      diagnostics.failedRequests.push(formatRequestFailure(request))
    }
  })
  page.on('response', response => collectViteChunkFailure(response, diagnostics))
  return diagnostics
}

export function mockAdminDiagnostics(page: Page) {
  const diagnostics = diagnosticsByPage.get(page)
  if (!diagnostics) {
    throw new Error('mockAdminApi(page) must install diagnostics before they are read')
  }
  return diagnostics
}

export function assertMockAdminClean(page: Page) {
  const diagnostics = mockAdminDiagnostics(page)
  const failures = [
    ...diagnostics.pageErrors.map(error => `pageerror: ${error}`),
    ...diagnostics.failedRequests.map(error => `requestfailed: ${error}`),
    ...diagnostics.viteChunkFailures.map(error => `vite dynamic chunk failure: ${error}`),
  ]
  if (failures.length > 0) {
    throw new Error(`Mock E2E diagnostics detected failures:\n${failures.join('\n')}`)
  }
}

function collectViteChunkFailure(response: Response, diagnostics: MockAdminDiagnostics) {
  if (response.status() >= 400 && isViteDynamicChunk(response.url())) {
    diagnostics.viteChunkFailures.push(`${response.status()} ${response.url()}`)
  }
}

function formatRequestFailure(request: Request) {
  return `${request.method()} ${request.url()} (${request.failure()?.errorText || 'unknown network failure'})`
}

function isUnexpectedRequestFailure(request: Request) {
  const errorText = request.failure()?.errorText || ''
  return errorText !== 'net::ERR_ABORTED' || !isNavigationAsset(request.url())
}

function isViteDynamicChunk(url: string) {
  const pathname = new URL(url).pathname
  return /\/(?:assets|src)\/.+\.(?:[cm]?js|css|vue|tsx?)$/.test(pathname)
}

function isViteDevelopmentAsset(url: string) {
  const pathname = new URL(url).pathname
  return pathname.startsWith('/src/')
    || pathname.startsWith('/node_modules/.vite/')
    || pathname === '/__uno.css'
}

function isNavigationAsset(url: string) {
  const pathname = new URL(url).pathname
  return isViteDevelopmentAsset(url)
    || pathname.startsWith('/font/')
    || /\.(?:woff2?|ttf|otf)$/.test(pathname)
}

export function ok(data: unknown, message = 'success') {
  return { code: 200, message, data }
}

export function fail(message = 'error') {
  return { code: 500, message, data: null }
}

export function unauthorized(message = '未登录或登录已过期') {
  return { code: 401, message, data: [] }
}

export function forbidden(message = '禁止访问') {
  return { code: 403, message, data: [] }
}

export function tokenFor(user: LoginUser) {
  return { access_token: `${user}-access-token`, expire_at: 3600, refresh_token: `${user}-refresh-token` }
}

export async function json(route: Route, body: unknown) {
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(body),
  })
}

export function routePath(route: Route) {
  const url = new URL(route.request().url())
  return url.pathname.replace(/^\/dev/, '')
}

export function parseJSON<T>(route: Route): T | undefined {
  try {
    return route.request().postDataJSON() as T
  }
  catch {
    return undefined
  }
}

export function userInfo(user: LoginUser) {
  const platformRole = user === 'readonly-admin'
    ? { id: 2, code: 'PlatformReader', name: '平台只读员' }
    : { id: 1, code: 'PlatformSuperAdmin', name: '平台超级管理员' }
  const tenantRole = { id: 3, code: 'TenantAdmin', name: '租户管理员' }
  return {
    id: user === 'readonly-admin' ? 2 : user === 'approver-admin' ? 3 : 1,
    username: user,
    nickname: user === 'readonly-admin' ? 'Read Only' : 'Admin',
    avatar: '',
    dashboard: 'welcome',
    backend_setting: null,
    departments: [],
    positions: [],
    roles: [user === 'tenant-sso' ? tenantRole : platformRole],
  }
}
