import type { Page } from '@playwright/test'
import { expect, test } from '@playwright/test'
import {
  dictOptions,
  imagePixel,
  initialAttachments,
  scheduledTaskLogs,
  scheduledTasks,
  ssoProviders,
  tenantBrandingConfig,
  tenants,
} from './adminFixtures'
import { permissionCatalog, platformMenuTree, tenantMenuTree } from './adminMenus'
import { assertMockAdminClean, fail, forbidden, installMockAdminDiagnostics, json, ok, parseJSON, routePath, tokenFor, unauthorized, userInfo } from './adminMockHelpers'
import type { LoginOptions, LoginUser, MockAdminOptions } from './adminMockHelpers'

test.beforeEach(async ({ page }) => {
  installMockAdminDiagnostics(page)
})

test.afterEach(async ({ page }) => {
  assertMockAdminClean(page)
})

export async function mockAdminApi(page: Page, options: MockAdminOptions = {}) {
  installMockAdminDiagnostics(page)
  await mockIconifyApi(page)
  await mockOneWordApi(page)
  const entryMode = options.entryMode ?? 'platform'
  const attachments = [...initialAttachments]
  const taskRows = scheduledTasks.map(item => ({ ...item }))
  const referenceCases = [{
    id: 1,
    code: 'golden-case',
    title: 'Golden Reference Case',
    status: 1,
    version: '1.0.0',
    payload: { scenario: 'upgrade' },
    remark: 'baseline',
  }]
  const activeAccessTokens = new Map<string, { user: LoginUser, refreshToken: string }>()
  const activeRefreshTokens = new Map<string, LoginUser>()
  const ssoTransactions = new Map<string, string>()
  let sessionSequence = 0
  let expiredTenantListRequests = options.expireTenantListRequests ?? (options.expireTenantListOnce === true ? 1 : 0)
  let expiredTenantListRequestIndex = 0
  let refreshShouldFail = options.failRefreshOnce === true
  let approvalSequence = 0
  let reauthSequence = 0
  const approvals = new Map<string, {
    approval_id: string
    requester_id: number
    approver_id: number
    policy_key: string
    binding_digest: string
    scope: string
    resource: string
    status: 'pending' | 'approved'
    reason: string
    used: boolean
  }>()
  const reauthTokens = new Map<string, { user_id: number, operation: string, resource: string, used: boolean }>()
  const tenantDeletionRequiresApproval = options.tenantDeletionRequiresApproval !== false
  const hasPermission = (user: LoginUser, permission: string) => user !== 'readonly-admin' || permission.endsWith(':list')

  const addSession = (user: LoginUser, rotate = false) => {
    const sequence = rotate ? `-${++sessionSequence}` : ''
    const tokens = rotate
      ? { access_token: `${user}-access-token${sequence}`, expire_at: 3600, refresh_token: `${user}-refresh-token${sequence}` }
      : tokenFor(user)
    activeAccessTokens.set(tokens.access_token, { user, refreshToken: tokens.refresh_token })
    activeRefreshTokens.set(tokens.refresh_token, user)
    return tokens
  }

  await page.route('**/dev/**', async (route) => {
    const path = routePath(route)
    const method = route.request().method()

    if (method === 'OPTIONS') {
      await route.fulfill({ status: 204 })
      return
    }

    if (path === '/admin/passport/entry') {
      await json(route, ok({
        mode: entryMode,
        available: true,
        message: '',
        tenant: entryMode === 'tenant' ? { code: 'acme', name: 'Acme 租户', status: 1 } : null,
        config: entryMode === 'tenant' ? tenantBrandingConfig() : null,
      }))
      return
    }

    if (path === '/admin/passport/branding') {
      await json(route, ok(tenantBrandingConfig()))
      return
    }

    if (path === '/admin/passport/captcha') {
      await json(route, ok({ key: 'captcha-key', base64: imagePixel }))
      return
    }

    if (path === '/admin/platform/passport/csrf-token' || path === '/admin/passport/csrf-token') {
      await route.fulfill({
        status: 200,
        headers: {
          'content-type': 'application/json',
          'set-cookie': 'csrf_token=e2e-csrf-token; Path=/; SameSite=Lax',
        },
        body: JSON.stringify(ok({ csrf_token: 'e2e-csrf-token' })),
      })
      return
    }

    if (path === '/admin/platform/passport/login' || path === '/admin/passport/login') {
      const payload = parseJSON<{ username?: LoginUser, password?: string }>(route)
      if (payload?.username === 'mfa-admin' && payload.password === '123456') {
        await json(route, ok({ mfa_required: true, mfa_token: 'mfa-token' }))
        return
      }
      if ((payload?.username === 'admin' || payload?.username === 'approver-admin' || payload?.username === 'readonly-admin') && payload.password === '123456') {
        await json(route, ok(addSession(payload.username)))
        return
      }
      await json(route, fail('账号或密码错误'))
      return
    }

    if (path === '/admin/platform/passport/mfa/login' || path === '/admin/passport/mfa/login') {
      const payload = parseJSON<{ mfa_token?: string, mfa_code?: string }>(route)
      if (payload?.mfa_token === 'mfa-token' && payload.mfa_code === '654321') {
        await json(route, ok(addSession('mfa-admin')))
        return
      }
      await json(route, fail('MFA 验证码错误'))
      return
    }

    if (path === '/admin/platform/passport/refresh' || path === '/admin/passport/refresh') {
      options.onRefreshRequest?.(route.request().headers())
      if (refreshShouldFail) {
        refreshShouldFail = false
        if (options.refreshFailureDelayMs) {
          await new Promise(resolve => setTimeout(resolve, options.refreshFailureDelayMs))
        }
        await json(route, unauthorized())
        return
      }
      if (options.requireRefreshCsrf && route.request().headers()['x-csrf-token'] !== 'e2e-csrf-token') {
        await json(route, forbidden('CSRF Token 无效'))
        return
      }
      const refreshAuthorization = route.request().headers().authorization ?? ''
      const refreshToken = refreshAuthorization.startsWith('Bearer ') ? refreshAuthorization.slice('Bearer '.length) : ''
      const refreshUser = activeRefreshTokens.get(refreshToken)
      if (!refreshUser) {
        await json(route, unauthorized())
        return
      }
      activeRefreshTokens.delete(refreshToken)
      await json(route, ok(addSession(refreshUser, true)))
      return
    }

    if (path === '/admin/platform/passport/logout' || path === '/admin/passport/logout') {
      options.onLogoutRequest?.(route.request().headers())
      if (options.logoutDelayMs) {
        await new Promise(resolve => setTimeout(resolve, options.logoutDelayMs))
      }
      if (options.requireLogoutCsrf && route.request().headers()['x-csrf-token'] !== 'e2e-csrf-token') {
        await json(route, forbidden('CSRF Token 无效'))
        return
      }
      const authorization = route.request().headers().authorization ?? ''
      const accessToken = authorization.startsWith('Bearer ') ? authorization.slice('Bearer '.length) : ''
      const logoutSession = activeAccessTokens.get(accessToken)
      activeAccessTokens.delete(accessToken)
      if (logoutSession) {
        activeRefreshTokens.delete(logoutSession.refreshToken)
      }
      await json(route, ok(null))
      return
    }

    if (path === '/admin/passport/sso/authorize' && method === 'POST') {
      const payload = parseJSON<{ provider?: string }>(route)
      if (payload?.provider !== 'okta-tenant') {
        await json(route, fail('SSO provider unavailable'))
        return
      }
      const transactionID = 'e2e-sso-transaction'
      const state = 'e2e-sso-state'
      const origin = new URL(route.request().url()).origin
      ssoTransactions.set(transactionID, state)
      await json(route, ok({
        authorization_url: `${origin}/?sso_return=1#/login?auth_scope=tenant&code=e2e-sso-code&state=${state}`,
        transaction_id: transactionID,
        state,
        expires_at: Math.floor(Date.now() / 1000) + 300,
      }))
      return
    }

    if (path === '/admin/passport/sso/callback' && method === 'POST') {
      const payload = parseJSON<{ transaction_id?: string, code?: string, state?: string }>(route)
      const expectedState = payload?.transaction_id ? ssoTransactions.get(payload.transaction_id) : undefined
      if (payload?.code === 'e2e-sso-code' && payload.state === expectedState) {
        ssoTransactions.delete(payload.transaction_id!)
        await json(route, ok(addSession('tenant-sso')))
        return
      }
      await json(route, fail('SSO callback rejected'))
      return
    }

    if (path === '/admin/passport/sso/login') {
      const payload = parseJSON<{ provider?: string, id_token?: string }>(route)
      if (payload?.provider === 'okta-tenant' && payload.id_token === 'valid-id-token') {
        await json(route, ok(addSession('tenant-sso')))
        return
      }
      await json(route, fail('SSO 登录失败'))
      return
    }

    if (path === '/admin/platform/tenant/list') {
      options.onTenantListRequest?.()
    }

    const authorization = route.request().headers().authorization ?? ''
    const accessToken = authorization.startsWith('Bearer ') ? authorization.slice('Bearer '.length) : ''
    const currentSession = activeAccessTokens.get(accessToken)
    if (!currentSession) {
      await json(route, unauthorized())
      return
    }
    const currentUser = currentSession.user
    const currentUserID = () => userInfo(currentUser).id

    const requiredPermission = permissionForRequest(path, method)
    if (requiredPermission && !hasPermission(currentUser, requiredPermission)) {
      await json(route, forbidden())
      return
    }

    if (path === '/admin/platform/passport/getInfo' || path === '/admin/passport/getInfo') {
      await json(route, ok(userInfo(currentUser)))
      return
    }

    if (path === '/admin/platform/security/reauth-token' && method === 'POST') {
      const payload = parseJSON<{ password?: string, operation?: string, resource?: string }>(route)
      if (payload?.password !== '123456' || !payload.operation || !payload.resource) {
        await json(route, { code: 422, message: 'invalid re-auth request', data: [] })
        return
      }
      const token = `reauth-${++reauthSequence}`
      reauthTokens.set(token, {
        user_id: currentUserID(),
        operation: payload.operation,
        resource: payload.resource,
        used: false,
      })
      await json(route, ok({ reauth_token: token, expires_at: '2026-07-10T12:00:00Z' }))
      return
    }

    if (path === '/admin/platform/security/approvals' && method === 'POST') {
      const payload = parseJSON<{ policy_key?: string, scope?: string, resource?: string, reason?: string }>(route)
      const scope = payload?.policy_key || payload?.scope
      if (!scope || !payload.resource || !payload.reason) {
        await json(route, { code: 422, message: 'invalid approval request', data: [] })
        return
      }
      const approval = {
        approval_id: `approval-${++approvalSequence}`,
        requester_id: currentUserID(),
        approver_id: 0,
        policy_key: scope,
        binding_digest: 'sha256:e2e-approval-binding',
        scope,
        resource: payload.resource,
        status: 'pending' as const,
        reason: payload.reason,
        used: false,
      }
      approvals.set(approval.approval_id, approval)
      await json(route, ok(approval))
      return
    }

    if (/^\/admin\/platform\/security\/approvals\/[^/]+$/.test(path) && method === 'GET') {
      const approval = approvals.get(decodeURIComponent(path.split('/').pop()!))
      if (!approval) {
        await json(route, { code: 422, message: 'approval not found', data: [] })
        return
      }
      await json(route, ok(approval))
      return
    }

    if (/^\/admin\/platform\/security\/approvals\/[^/]+\/approve$/.test(path) && method === 'PUT') {
      const approvalID = decodeURIComponent(path.split('/').at(-2)!)
      const approval = approvals.get(approvalID)
      if (!approval || approval.requester_id === currentUserID() || approval.status !== 'pending') {
        await json(route, { code: 422, message: 'approval requires a different approver', data: [] })
        return
      }
      approval.approver_id = currentUserID()
      approval.status = 'approved'
      await json(route, ok(approval))
      return
    }

    if (path === '/admin/platform/permission/menus') {
      await json(route, ok(platformMenuTree(currentUser === 'readonly-admin')))
      return
    }

    if (path === '/admin/permission/menus') {
      await json(route, ok(tenantMenuTree()))
      return
    }

    if (path === '/admin/platform/permission/roles' || path === '/admin/permission/roles') {
      await json(route, ok(userInfo(currentUser).roles))
      return
    }

    if (path === '/admin/platform/dictionary/options' || path === '/admin/dictionary/options') {
      await json(route, ok(dictOptions))
      return
    }

    if (path === '/admin/platform/tenant/list') {
      const isExpiryTarget = !options.expireTenantListProbeOnly
        || new URL(route.request().url()).searchParams.has('probe')
      if (isExpiryTarget && expiredTenantListRequests > 0) {
        expiredTenantListRequests--
        const delay = options.expireTenantListDelayMs?.[expiredTenantListRequestIndex++] ?? 0
        if (delay > 0) {
          await new Promise(resolve => setTimeout(resolve, delay))
        }
        await json(route, unauthorized())
        return
      }
      await json(route, ok({ total: tenants.length, list: tenants }))
      return
    }

    if (path === '/admin/platform/tenant-plan/options') {
      await json(route, ok([
        {
          code: 'pro',
          name: '专业版',
          billing: { subscription_status: 'active', amount_cents: 9900 },
          quotas: { max_users: 50, max_roles: 10, max_storage_mb: 1024 },
          features: {},
        },
      ]))
      return
    }

    if (path === '/admin/platform/tenant/permission-catalog') {
      await json(route, ok(permissionCatalog()))
      return
    }

    if (/^\/admin\/platform\/tenant\/\d+\/permissions$/.test(path)) {
      await json(route, ok({ allowed: ['tenant:user'] }))
      return
    }

    if (/^\/admin\/platform\/tenant\/\d+\/usage$/.test(path)) {
      await json(route, ok({
        id: 1001,
        code: 'acme',
        name: 'Acme 租户',
        plan: 'pro',
        billing: { subscription_status: 'active' },
        quotas: tenants[0].quotas,
        usage: { users: 3, roles: 2, storage_mb: 12 },
      }))
      return
    }

    if (/^\/admin\/platform\/tenant\/\d+\/governance$/.test(path) && method === 'GET') {
      const id = Number(path.split('/').at(-2))
      const tenant = tenants.find(item => item.id === id) ?? tenants[0]
      await json(route, ok({
        tenant_id: id,
        tenant_code: tenant.code,
        modules: {},
        quotas: tenant.quotas,
        rate_limit: { per_minute: 60 },
        retention: { audit_days: 365, data_days: 365 },
        data_export: { enabled: true, requires_approval: true },
        data_deletion: { enabled: true, requires_approval: tenantDeletionRequiresApproval },
        isolation_proof: { verified: true, evidence: 'artifact://e2e', digest: 'sha256:e2e' },
      }))
      return
    }

    if (/^\/admin\/platform\/tenant\/\d+\/(?:suspend|resume|archive)$/.test(path) && method === 'PUT') {
      await json(route, ok(null))
      return
    }

    if (path === '/admin/platform/tenant' && method === 'DELETE') {
      const payload = parseJSON<Record<string, unknown>>(route) ?? {}
      const ids = Array.isArray(payload.ids) ? payload.ids.map(Number).sort((a, b) => a - b) : []
      const resource = `tenant-data:delete:${ids.join(',')}:metadata`
      const evidenceAccepted = tenantDeletionRequiresApproval
        ? consumeSensitiveEvidence(
            reauthTokens,
            approvals,
            String(payload.reauth_token ?? '').trim(),
            String(payload.approval_id ?? '').trim(),
            currentUserID(),
            'tenant.data.delete',
            resource,
          )
        : consumeReauthEvidence(
            reauthTokens,
            String(payload.reauth_token ?? '').trim(),
            currentUserID(),
            'tenant.data.delete',
            resource,
          )
      if (!evidenceAccepted) {
        await json(route, { code: 422, message: 'tenant deletion requires valid evidence', data: [] })
        return
      }
      options.onTenantDestroyRequest?.(payload)
      await json(route, ok(null, 'destroyed'))
      return
    }

    if (/^\/admin\/platform\/tenant(?:\/\d+)?$/.test(path) && ['POST', 'PUT'].includes(method)) {
      await json(route, ok(tenants[0]))
      return
    }

    if (path === '/admin/attachment/list') {
      await json(route, ok({ total: attachments.length, list: attachments }))
      return
    }

    if (path === '/admin/attachment/upload' && method === 'POST') {
      const uploaded = { id: 3003, origin_name: 'uploaded.png', object_name: 'uploaded.png', mime_type: 'image/png', suffix: 'png', size_byte: 9, size_info: '9 B', url: imagePixel }
      attachments.push(uploaded)
      await json(route, ok(uploaded))
      return
    }

    if (path === '/admin/platform/sso-provider/list' || path === '/admin/sso-provider/list') {
      await json(route, ok({ total: ssoProviders.length, list: ssoProviders }))
      return
    }

    if (/^\/admin\/platform\/sso-provider(?:\/\d+)?$/.test(path) || /^\/admin\/sso-provider(?:\/\d+)?$/.test(path)) {
      await json(route, ok(ssoProviders[0]))
      return
    }

    if (path === '/admin/platform/sso-provider' || path === '/admin/sso-provider') {
      await json(route, ok(null))
      return
    }

    if (path === '/admin/platform/scheduled-task/list') {
      await json(route, ok({ total: taskRows.length, list: taskRows }))
      return
    }

    if (/^\/admin\/platform\/scheduled-task\/\d+$/.test(path) && method === 'GET') {
      await json(route, ok(taskRows[0]))
      return
    }

    if (path === '/admin/platform/scheduled-task/tenant-options') {
      await json(route, ok([{ id: 1001, code: 'acme', name: 'Acme 租户' }]))
      return
    }

    if (/^\/admin\/platform\/scheduled-task\/\d+\/run$/.test(path) && method === 'POST') {
      taskRows[0].last_status = 'success'
      await json(route, ok(scheduledTaskLogs[0], '任务已执行'))
      return
    }

    if (/^\/admin\/platform\/scheduled-task\/\d+\/(?:enable|disable)$/.test(path) && method === 'PUT') {
      taskRows[0].status = path.endsWith('/disable') ? 2 : 1
      await json(route, ok(taskRows[0]))
      return
    }

    if (path === '/admin/platform/scheduled-task-log/list') {
      await json(route, ok({ total: scheduledTaskLogs.length, list: scheduledTaskLogs }))
      return
    }

    if (path === '/admin/platform/module-lifecycle/state') {
      await json(route, ok({
        total: 1,
        list: [{
          id: 'platform-rbac',
          name: 'Platform RBAC',
          version: '1.0.0',
          compatible: '>=1.0.0',
          enabled: true,
          lifecycle: {
            install: 'migrate',
            uninstall: 'migrate:rollback',
            upgrade: 'module:manifest:check && migrate',
            rollback: 'migrate:rollback',
            destructive_check: 'module:manifest:check',
            supports_hot_disable: false,
            requires_restart: true,
            breaking_change_policy: 'manual review',
          },
          frontend: {},
          seed_strategy: { mode: 'none', idempotent: true },
          persisted: {
            status: 'upgraded',
            enabled: true,
            owner: 'e2e',
            last_action: 'upgrade',
            last_run_key: 'upgrade:platform-rbac:1.0.0',
          },
        }],
      }))
      return
    }

    if (path === '/admin/platform/module-lifecycle/runs') {
      await json(route, ok({
        total: 1,
        list: [{
          id: 1,
          idempotency_key: 'upgrade:platform-rbac:1.0.0',
          module_id: 'platform-rbac',
          action: 'upgrade',
          to_version: '1.0.0',
          status: 'succeeded',
          dry_run: false,
          owner: 'e2e',
          reason: 'smoke',
          command: 'module:manifest:check && migrate',
          started_at: '2026-07-09T10:00:00Z',
          finished_at: '2026-07-09T10:00:01Z',
        }],
      }))
      return
    }

    if (path === '/admin/platform/module-lifecycle/steps') {
      await json(route, ok({
        total: 2,
        list: [
          {
            id: 1,
            run_key: 'upgrade:platform-rbac:1.0.0',
            module_id: 'platform-rbac',
            action: 'upgrade',
            step_name: 'destructive_check',
            command: 'module:manifest:check',
            status: 'succeeded',
            stdout: '',
            stderr: '',
            started_at: '2026-07-09T10:00:00Z',
            finished_at: '2026-07-09T10:00:00Z',
          },
          {
            id: 2,
            run_key: 'upgrade:platform-rbac:1.0.0',
            module_id: 'platform-rbac',
            action: 'upgrade',
            step_name: 'command',
            command: 'migrate',
            status: 'succeeded',
            stdout: '',
            stderr: '',
            started_at: '2026-07-09T10:00:00Z',
            finished_at: '2026-07-09T10:00:01Z',
          },
        ],
      }))
      return
    }

    if (path === '/admin/platform/module-lifecycle/locks') {
      await json(route, ok({ total: 0, list: [] }))
      return
    }

    if (path === '/admin/platform/module-lifecycle/diff') {
      await json(route, ok({
        total: 1,
        list: [{
          module_id: 'platform-rbac',
          name: 'Platform RBAC',
          manifest_version: '1.0.0',
          persisted_version: '1.0.0',
          manifest_enabled: true,
          persisted_enabled: true,
          persisted_status: 'upgraded',
          last_action: 'upgrade',
          drift: 'in_sync',
        }],
      }))
      return
    }

    if (path === '/admin/platform/module-lifecycle/locks/release-stale' && method === 'POST') {
      const payload = parseJSON<{ dry_run?: boolean, confirm_token?: string, reauth_token?: string, approval_id?: string }>(route)
      const resource = `module-lifecycle:stale-locks:${String(parseJSON<{ key?: string }>(route)?.key || 'all').trim() || 'all'}`
      if (!payload?.dry_run && (
        payload?.confirm_token !== 'release-stale-locks'
        || !consumeSensitiveEvidence(
          reauthTokens,
          approvals,
          payload?.reauth_token || '',
          payload?.approval_id || '',
          currentUserID(),
          'module.lifecycle.release-lock',
          resource,
        )
      )) {
        await json(route, { code: 422, message: 'stale lock release requires safety gate', data: [] })
        return
      }
      await json(route, ok({ dry_run: payload?.dry_run !== false, released: [] }))
      return
    }

    if (path === '/admin/platform/module-lifecycle/execute' && method === 'POST') {
      const payload = parseJSON<{ module_id?: string, action?: string, execute?: boolean, confirm_token?: string, reauth_token?: string, approval_id?: string }>(route)
      const action = payload?.action || 'upgrade'
      const moduleID = payload?.module_id || 'all'
      const resource = `module-lifecycle:${moduleID}:${action}`
      if (payload?.execute && (
        payload.confirm_token !== `${moduleID}:${action}`
        || !consumeSensitiveEvidence(
          reauthTokens,
          approvals,
          payload.reauth_token || '',
          payload.approval_id || '',
          currentUserID(),
          'module.lifecycle.execute',
          resource,
        )
      )) {
        await json(route, { code: 422, message: 'module lifecycle execute requires safety gate', data: [] })
        return
      }
      await json(route, ok({
        action: payload?.action || 'upgrade',
        dry_run: payload?.execute !== true,
        owner: 'e2e',
        reason: 'smoke',
        items: [{
          module_id: payload?.module_id || 'platform-rbac',
          name: 'Platform RBAC',
          action: payload?.action || 'upgrade',
          status: payload?.execute ? 'succeeded' : 'planned',
          command: 'module:manifest:check && migrate',
          destructive_check: 'module:manifest:check',
          idempotency_key: 'upgrade:platform-rbac:1.0.0',
        }],
      }))
      return
    }

    if (path === '/admin/platform/reference-case/list') {
      const url = new URL(route.request().url())
      const code = url.searchParams.get('code') ?? ''
      const title = url.searchParams.get('title') ?? ''
      const rows = referenceCases.filter(item =>
        (!code || item.code.includes(code))
        && (!title || item.title.includes(title)),
      )
      await json(route, ok({ total: rows.length, list: rows }))
      return
    }

    if (path === '/admin/platform/reference-case' && method === 'POST') {
      const payload = parseJSON<typeof referenceCases[number]>(route)
      const item = {
        id: referenceCases.length + 1,
        code: payload?.code || 'new-case',
        title: payload?.title || 'New Reference Case',
        status: payload?.status || 1,
        version: payload?.version || '1.0.0',
        payload: payload?.payload || {},
        remark: payload?.remark || '',
      }
      referenceCases.unshift(item)
      await json(route, ok(item))
      return
    }

    if (/^\/admin\/platform\/reference-case\/\d+$/.test(path) && method === 'PUT') {
      const id = Number(path.split('/').pop())
      const payload = parseJSON<typeof referenceCases[number]>(route)
      const index = referenceCases.findIndex(item => item.id === id)
      if (index >= 0) {
        referenceCases[index] = { ...referenceCases[index], ...payload, id }
        await json(route, ok(referenceCases[index]))
        return
      }
      await json(route, fail('reference case not found'))
      return
    }

    if (path === '/admin/platform/reference-case' && method === 'DELETE') {
      const ids = parseJSON<number[]>(route) ?? []
      ids.forEach((id) => {
        const index = referenceCases.findIndex(item => item.id === id)
        if (index >= 0) {
          referenceCases.splice(index, 1)
        }
      })
      await json(route, ok(null))
      return
    }

    await route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify(fail(`Unhandled E2E mock: ${method} ${path}`)),
    })
  })
}

async function mockIconifyApi(page: Page) {
  await page.route('https://api.iconify.design/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ prefix: '', icons: {} }),
    })
  })
}

async function mockOneWordApi(page: Page) {
  await page.route('https://api.xygeng.cn/openapi/one/**', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: { content: 'E2E quote', origin: 'E2E' } }),
    })
  })
}

function permissionForRequest(path: string, method: string) {
  if (path === '/admin/platform/security/reauth-token' && method === 'POST') {
    return 'platform:security:control'
  }
  if (path === '/admin/platform/security/approvals' && method === 'POST') {
    return 'platform:security:control'
  }
  if (/^\/admin\/platform\/security\/approvals\/[^/]+$/.test(path) && method === 'GET') {
    return 'platform:security:control'
  }
  if (/^\/admin\/platform\/security\/approvals\/[^/]+\/approve$/.test(path) && method === 'PUT') {
    return 'platform:security:control'
  }
  if (path === '/admin/platform/module-lifecycle/steps' && method === 'GET') {
    return 'platform:moduleLifecycle:log'
  }
  if ((path === '/admin/platform/module-lifecycle/execute' || path === '/admin/platform/module-lifecycle/locks/release-stale') && method === 'POST') {
    return 'platform:moduleLifecycle:execute'
  }
  if (path === '/admin/platform/tenant' && method === 'DELETE') {
    return 'platform:tenant:destroy'
  }
  if (path === '/admin/platform/reference-case' && method === 'POST') {
    return 'platform:referenceCase:save'
  }
  if (/^\/admin\/platform\/reference-case\/\d+$/.test(path) && method === 'PUT') {
    return 'platform:referenceCase:update'
  }
  if (path === '/admin/platform/reference-case' && method === 'DELETE') {
    return 'platform:referenceCase:delete'
  }
  return ''
}

function consumeReauthEvidence(
  reauthTokens: Map<string, { user_id: number, operation: string, resource: string, used: boolean }>,
  reauthToken: string,
  userID: number,
  operation: string,
  resource: string,
) {
  const reauth = reauthTokens.get(reauthToken)
  if (!reauth || reauth.used || reauth.user_id !== userID || reauth.operation !== operation || reauth.resource !== resource) {
    return false
  }
  reauth.used = true
  return true
}

function consumeSensitiveEvidence(
  reauthTokens: Map<string, { user_id: number, operation: string, resource: string, used: boolean }>,
  approvals: Map<string, { requester_id: number, scope: string, resource: string, status: string, used: boolean }>,
  reauthToken: string,
  approvalID: string,
  userID: number,
  operation: string,
  resource: string,
) {
  const reauth = reauthTokens.get(reauthToken)
  const approval = approvals.get(approvalID)
  if (!reauth || reauth.used || reauth.user_id !== userID || reauth.operation !== operation || reauth.resource !== resource) {
    return false
  }
  if (!approval || approval.used || approval.requester_id !== userID || approval.status !== 'approved' || approval.scope !== operation || approval.resource !== resource) {
    return false
  }
  reauth.used = true
  approval.used = true
  return true
}

export async function loginAsAdmin(page: Page, options: LoginOptions = {}) {
  const username = options.username ?? 'admin'
  const password = options.password ?? '123456'
  const waitForShell = options.waitForShell ?? true

  await page.goto('/#/login?auth_scope=platform')
  await expect(page.getByText('账户')).toBeVisible()
  await page.locator('input[name="username"]').fill(username)
  await page.locator('input[name="password"]').fill(password)
  await page.locator('input[name="code"]').fill('1234')
  await page.getByRole('button', { name: /登录/ }).click()

  if (waitForShell) {
    await expect(page).not.toHaveURL(/\/login/)
    await expect(page.locator('.mine-user-bar')).toBeVisible()
  }
}

export { assertMockAdminClean, mockAdminDiagnostics } from './adminMockHelpers'
