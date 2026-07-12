import type { SensitiveEvidenceRequest, SensitiveEvidenceResult } from '~/base/api/platformSecurityControl'

export type SensitiveEvidence = SensitiveEvidenceResult
export type SensitiveEvidenceRequester = (request: SensitiveEvidenceRequest) => Promise<SensitiveEvidence>

export function rbacPasswordResource(userID: number) {
  return `rbac:user:${userID}:password:reset`
}

export function rbacRolesResource(userID: number, roles: string[]) {
  return rbacDesiredResource('user', userID, 'roles', roles)
}

export function rbacPermissionsResource(roleID: number, permissions: string[]) {
  return rbacDesiredResource('role', roleID, 'permissions', permissions)
}

export function tenantChangeResource(kind: string, tenantID: number, desired: unknown) {
  return `tenant-change:${kind}:${base64Url(JSON.stringify({ tenant_id: tenantID, desired }))}`
}

export function secretResource(kind: 'sso-provider' | 'storage-config', action: 'create' | 'update' | 'delete', ids?: number[]) {
  if (action === 'create') {
    return `${kind}:create`
  }
  const values = [...new Set(ids ?? [])].sort((a, b) => a - b)
  return `${kind}:${action}:${values.join(',')}`
}

function rbacDesiredResource(subject: string, id: number, operation: string, values: string[]) {
  const desired = [...new Set(values.map(value => value.trim()).filter(Boolean))].sort()
  return `rbac:${subject}:${id}:${operation}:${base64Url(JSON.stringify(desired))}`
}

function base64Url(value: string) {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  bytes.forEach(byte => binary += String.fromCharCode(byte))
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')
}
