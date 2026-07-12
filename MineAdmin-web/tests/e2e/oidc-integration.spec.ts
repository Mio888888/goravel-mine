import type { APIRequestContext, Page } from '@playwright/test'
import { expect, test } from '@playwright/test'

const idpURL = process.env.OIDC_IDP_URL?.replace(/\/$/, '') ?? 'http://127.0.0.1:19090'
const apiURL = process.env.OIDC_API_URL?.replace(/\/$/, '') ?? 'http://127.0.0.1:3000'
const tenantCode = process.env.OIDC_TENANT_CODE ?? 'default'
const provider = process.env.OIDC_PROVIDER ?? 'oidc-e2e'

test.beforeEach(async ({ request }) => {
  const response = await request.post(`${idpURL}/control/reset`)
  expect(response.ok()).toBe(true)
})

test('completes authorization code with server-owned PKCE and nonce', async ({ page, request }) => {
  const callback = await startAuthorization(request)

  await completeBrowserAuthorization(page, callback.authorization_url)
  const result = await completeCallback(request, callback.transaction_id, callback.state, page)

  expect(result.code).toBe(200)
  expect(result.data.access_token).toEqual(expect.any(String))
  expect(result.data.refresh_token).toEqual(expect.any(String))
  await expectOIDCEvent(request, 'token', 'issued')
})

test('rejects callback state mismatch without consuming the transaction', async ({ page, request }) => {
  await configureFault(request, { wrong_state: true })
  const callback = await startAuthorization(request)
  await completeBrowserAuthorization(page, callback.authorization_url)
  const code = await callbackCode(page)
  const returnedState = await callbackState(page)

  const rejected = await completeCallback(request, callback.transaction_id, returnedState, page)
  expect(rejected.code).toBe(422)

  const accepted = await completeCallback(request, callback.transaction_id, callback.state, page)
  expect(accepted.code).toBe(200)
  expect(accepted.data.access_token).toEqual(expect.any(String))
  expect(code).toBeTruthy()
})

test('rejects nonce mismatch from the IdP', async ({ page, request }) => {
  await configureFault(request, { nonce_mismatch: true })
  const callback = await startAuthorization(request)

  await completeBrowserAuthorization(page, callback.authorization_url)
  const result = await completeCallback(request, callback.transaction_id, callback.state, page)

  expect(result.code).not.toBe(200)
  await expectOIDCEvent(request, 'token', 'issued')
})

test('rejects a reused authorization code', async ({ page, request }) => {
  const first = await startAuthorization(request)
  await completeBrowserAuthorization(page, first.authorization_url)
  const accepted = await completeCallback(request, first.transaction_id, first.state, page)
  expect(accepted.code).toBe(200)

  await configureFault(request, { replay_code: true })
  const second = await startAuthorization(request)
  await completeBrowserAuthorization(page, second.authorization_url)
  const replayed = await completeCallback(request, second.transaction_id, second.state, page)
  expect(replayed.code).not.toBe(200)
  await expectOIDCEvent(request, 'token', 'rejected')
})

test('accepts an ID token after JWKS rotation', async ({ page, request }) => {
  const rotation = await request.post(`${idpURL}/control/rotate`)
  expect(rotation.ok()).toBe(true)
  const callback = await startAuthorization(request)

  await completeBrowserAuthorization(page, callback.authorization_url)
  const result = await completeCallback(request, callback.transaction_id, callback.state, page)

  expect(result.code).toBe(200)
  await expectOIDCEvent(request, 'jwks.rotate', 'rotated')
})

test('fails closed when the token endpoint returns 5xx', async ({ page, request }) => {
  await configureFault(request, { token_status: 500 })
  const callback = await startAuthorization(request)

  await completeBrowserAuthorization(page, callback.authorization_url)
  const result = await completeCallback(request, callback.transaction_id, callback.state, page)

  expect(result.code).not.toBe(200)
  await expectOIDCEvent(request, 'token', 'server_error')
})

test('fails closed when the token endpoint times out', async ({ page, request }) => {
  await configureFault(request, { token_delay_ms: 12_000 })
  const callback = await startAuthorization(request)

  await completeBrowserAuthorization(page, callback.authorization_url)
  const result = await completeCallback(request, callback.transaction_id, callback.state, page)

  expect(result.code).not.toBe(200)
})

test('IdP rejects code exchange without the server-held PKCE verifier', async ({ page, request }) => {
  const callback = await startAuthorization(request)
  await completeBrowserAuthorization(page, callback.authorization_url)
  const code = await callbackCode(page)
  const token = await request.post(`${idpURL}/token`, {
    form: {
      grant_type: 'authorization_code',
      client_id: process.env.OIDC_IDP_CLIENT_ID ?? 'goravel-oidc-e2e',
      redirect_uri: process.env.OIDC_REDIRECT_URI ?? 'http://127.0.0.1:2889/#/login',
      code,
    },
  })

  expect(token.status()).toBe(400)
  await expectOIDCEvent(request, 'token', 'rejected')
})

async function startAuthorization(request: APIRequestContext): Promise<AuthorizationResult> {
  const response = await request.post(`${apiURL}/admin/passport/sso/authorize`, {
    headers: tenantHeaders(),
    data: { provider, scene: 'admin' },
  })
  expect(response.ok()).toBe(true)
  const body = await response.json()
  expect(body.code).toBe(200)
  expect(body.data.transaction_id).toEqual(expect.any(String))
  expect(body.data.state).toEqual(expect.any(String))
  expect(body.data.authorization_url).toContain(`${idpURL}/authorize`)
  return body.data as AuthorizationResult
}

async function completeBrowserAuthorization(page: Page, authorizationURL: string) {
  await page.goto(authorizationURL)
  await expect(page).toHaveURL(/code=.+&state=.+/)
}

async function callbackCode(page: Page) {
  const url = callbackURL(page)
  const code = url.searchParams.get('code')
  expect(code).toBeTruthy()
  return code as string
}

async function callbackState(page: Page) {
  const state = callbackURL(page).searchParams.get('state')
  expect(state).toBeTruthy()
  return state as string
}

function callbackURL(page: Page) {
  return new URL(page.url())
}

async function completeCallback(
  request: APIRequestContext,
  transactionID: string,
  state: string,
  page: Page,
  explicitCode?: string,
) {
  const response = await request.post(`${apiURL}/admin/passport/sso/callback`, {
    headers: tenantHeaders(),
    data: {
      transaction_id: transactionID,
      code: explicitCode ?? await callbackCode(page),
      state,
    },
  })
  if (response.ok()) {
    return response.json()
  }
  return { code: response.status(), message: await response.text(), data: [] }
}

async function configureFault(request: APIRequestContext, fault: Record<string, unknown>) {
  const response = await request.post(`${idpURL}/control/fault`, { data: fault })
  expect(response.ok()).toBe(true)
}

async function expectOIDCEvent(request: APIRequestContext, kind: string, outcome: string) {
  const response = await request.get(`${idpURL}/control/events`)
  expect(response.ok()).toBe(true)
  const body = await response.json()
  expect(body.events).toEqual(expect.arrayContaining([
    expect.objectContaining({ kind, outcome }),
  ]))
}

function tenantHeaders() {
  return { 'X-Tenant-Code': tenantCode, 'host': process.env.OIDC_TENANT_HOST ?? 'default.localhost' }
}

interface AuthorizationResult {
  transaction_id: string
  state: string
  authorization_url: string
}
