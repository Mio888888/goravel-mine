export interface SSOCallbackParameters {
  code?: string
  state?: string
}

export function resolveSSOCallbackParameters(
  routeQuery: Record<string, unknown>,
  locationSearch = window.location.search,
): SSOCallbackParameters {
  const code = firstQueryValue(routeQuery.code)
  const state = firstQueryValue(routeQuery.state)
  if (code && state) {
    return { code, state }
  }
  const search = new URLSearchParams(locationSearch)
  return {
    code: code || search.get('code') || undefined,
    state: state || search.get('state') || undefined,
  }
}

export function clearOuterSSOCallbackQuery() {
  if (!window.location.search) {
    return
  }
  const search = new URLSearchParams(window.location.search)
  search.delete('code')
  search.delete('state')
  search.delete('session_state')
  const suffix = search.size ? `?${search.toString()}` : ''
  window.history.replaceState(window.history.state, '', `${window.location.pathname}${suffix}${window.location.hash}`)
}

function firstQueryValue(value: unknown): string | undefined {
  const current = Array.isArray(value) ? value[0] : value
  return typeof current === 'string' && current ? current : undefined
}
