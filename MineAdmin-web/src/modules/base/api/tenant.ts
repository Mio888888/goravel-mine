import type { PageList, ResponseStruct } from '#/global'
import type { MenuVo } from './menu'
import type { SensitiveEvidence } from '~/base/utils/sensitiveOperation'
import type { TenantPlanVo } from './platformTenantPlan'
import * as platformTenantPlan from './platformTenantPlan'

export interface TenantVo {
  id?: number
  code?: string
  name?: string
  status?: 1 | 2 | 3
  plan?: string
  db_host?: string
  db_port?: number
  db_database?: string
  db_username?: string
  db_password?: string
  db_schema?: string
  custom_domain?: string | null
  branding?: Record<string, any> | null
  billing?: Record<string, any> | null
  quotas?: Record<string, any> | null
  features?: Record<string, any> | null
  remark?: string
  initialize?: boolean
  created_at?: string
  updated_at?: string
}

export interface TenantSearchVo {
  code?: string
  name?: string
  plan?: string
  status?: number
  page?: number
  page_size?: number
  [key: string]: any
}

export type TenantPlanOptionVo = TenantPlanVo

export interface TenantUsageReport {
  id: number
  code: string
  name: string
  plan: string
  billing: Record<string, any>
  quotas: Record<string, any>
  usage: {
    users: number
    roles: number
    storage_mb: number
    [key: string]: any
  }
}

export interface TenantGovernancePolicy {
  tenant_id: number
  tenant_code: string
  modules: Record<string, boolean>
  quotas: Record<string, any>
  rate_limit: {
    per_minute: number
  }
  retention: {
    audit_days: number
    data_days: number
  }
  data_export: {
    enabled: boolean
    requires_approval: boolean
  }
  data_deletion: {
    enabled: boolean
    requires_approval: boolean
  }
  isolation_proof: {
    verified: boolean
    evidence: string
    digest: string
    verified_at?: string
  }
}

export interface TenantPermissionPayload {
  allowed: string[]
}

export interface TenantPermissionPlanDiff {
  plan: string
  allowed: string[]
  added: string[]
  removed: string[]
  unchanged: string[]
  permission?: string[]
}

export interface TenantPlanUpdatePayload {
  plan: string
  features?: Record<string, any> | null
}

export interface TenantBrandingConfig {
  code: string
  name: string
  plan: string
  custom_domain?: string | null
  branding: {
    app_name?: string
    logo_url?: string
    primary_color?: string
    mail_from_name?: string
    [key: string]: any
  }
  features: {
    sso?: {
      providers?: TenantSSOProvider[]
    }
    [key: string]: any
  }
}

export type TenantLoginEntryMode = 'tenant' | 'platform'

export interface TenantLoginEntryTenant {
  code: string
  name: string
  status: 1 | 2 | 3
}

export interface TenantLoginEntry {
  mode: TenantLoginEntryMode
  available: boolean
  message: string
  tenant: TenantLoginEntryTenant | null
  config: TenantBrandingConfig | null
}

export interface TenantSSOProvider {
  name: string
  display_name?: string
  scene?: string
  type?: string
  issuer?: string
  discovery_url?: string
  authorization_endpoint?: string
  client_id?: string
  scope?: string
  redirect_uri?: string
  enable_pkce?: boolean
  enable_nonce?: boolean
  saml_entrypoint?: string
  saml_entity_id?: string
  icon?: string
  button_color?: string
  enabled?: boolean
}

export interface TenantSSOLoginPayload {
  provider: string
  scene?: string
  id_token?: string
  nonce?: string
  saml_response?: string
}

export interface TenantSSOAuthorizationPayload {
  provider: string
  scene?: string
}

export interface TenantSSOAuthorizationResult {
  transaction_id: string
  state: string
  authorization_url: string
}

export interface TenantSSOCallbackPayload {
  transaction_id: string
  code: string
  state: string
}

export interface TenantLoginResult {
  access_token: string
  expire_at: number
  refresh_token: string
}

export interface TenantExportRun {
  id: number
  tenant_id: number
  status: 'pending' | 'running' | 'artifact_written' | 'completed' | 'failed' | 'stale'
  error?: string
}

export interface TenantExportStatus {
  run: TenantExportRun
  download_token?: string
  expires_at?: string
}

export function page(data: TenantSearchVo): Promise<ResponseStruct<PageList<TenantVo>>> {
  return useHttp().get('/admin/platform/tenant/list', { params: data })
}

export function planOptions(): Promise<ResponseStruct<TenantPlanOptionVo[]>> {
  return platformTenantPlan.options()
}

export function create(data: TenantVo): Promise<ResponseStruct<TenantVo>> {
  return useHttp().post('/admin/platform/tenant', data)
}

export function save(id: number, data: TenantVo): Promise<ResponseStruct<TenantVo>> {
  return useHttp().put(`/admin/platform/tenant/${id}`, data)
}

export function suspend(id: number, evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/tenant/${id}/suspend`, evidence)
}

export function resume(id: number, evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/tenant/${id}/resume`, evidence)
}

export function archive(id: number, evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/tenant/${id}/archive`, evidence)
}

export function usage(id: number): Promise<ResponseStruct<TenantUsageReport>> {
  return useHttp().get(`/admin/platform/tenant/${id}/usage`)
}

export function governance(id: number): Promise<ResponseStruct<TenantGovernancePolicy>> {
  return useHttp().get(`/admin/platform/tenant/${id}/governance`)
}

export function saveGovernance(id: number, data: Partial<TenantGovernancePolicy>, evidence: SensitiveEvidence): Promise<ResponseStruct<TenantGovernancePolicy>> {
  return useHttp().put(`/admin/platform/tenant/${id}/governance`, { ...data, ...evidence })
}

export function permissionCatalog(): Promise<ResponseStruct<MenuVo[]>> {
  return useHttp().get('/admin/platform/tenant/permission-catalog')
}

export function permissions(id: number): Promise<ResponseStruct<TenantPermissionPayload>> {
  return useHttp().get(`/admin/platform/tenant/${id}/permissions`)
}

export function savePermissions(id: number, data: TenantPermissionPayload, evidence: SensitiveEvidence): Promise<ResponseStruct<TenantPermissionPayload>> {
  return useHttp().put(`/admin/platform/tenant/${id}/permissions`, { ...data, ...evidence })
}

export function permissionPlanDiff(id: number, data: TenantPlanUpdatePayload): Promise<ResponseStruct<TenantPermissionPlanDiff>> {
  return useHttp().post(`/admin/platform/tenant/${id}/permissions/plan-diff`, data)
}

export function updatePlan(id: number, data: TenantPlanUpdatePayload, evidence: SensitiveEvidence): Promise<ResponseStruct<TenantVo>> {
  return useHttp().put(`/admin/platform/tenant/${id}/plan`, { ...data, ...evidence })
}

export function destroy(data: {
  ids: number[]
  confirm_code?: string
  drop_database?: boolean
  reauth_token: string
  approval_id?: string
}): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/tenant', { data })
}

export function requestExport(id: number, data: {
  dataset: 'users'
  format: 'jsonl' | 'csv'
  filters?: { status?: '1' | '2' }
} & SensitiveEvidence): Promise<ResponseStruct<TenantExportRun>> {
  return useHttp().post(`/admin/platform/tenant/${id}/exports`, data)
}

export function exportStatus(id: number, runID: number): Promise<ResponseStruct<TenantExportStatus>> {
  return useHttp().get(`/admin/platform/tenant/${id}/exports/${runID}`)
}

export function downloadExport(id: number, runID: number, token: string): Promise<{ data: Blob, fileName: string }> {
  return useHttp().get(`/admin/platform/tenant/${id}/exports/${runID}/download`, {
    params: { token },
    responseType: 'blob',
  })
}

export function branding(scene = 'admin'): Promise<ResponseStruct<TenantBrandingConfig>> {
  return useHttp().get('/admin/passport/branding', { params: { scene } })
}

export function loginEntry(scene = 'admin'): Promise<ResponseStruct<TenantLoginEntry>> {
  return useHttp().get('/admin/passport/entry', { params: { scene } })
}

export function ssoLogin(data: TenantSSOLoginPayload): Promise<ResponseStruct<TenantLoginResult>> {
  return useHttp().post('/admin/passport/sso/login', data)
}

export function ssoAuthorize(data: TenantSSOAuthorizationPayload): Promise<ResponseStruct<TenantSSOAuthorizationResult>> {
  return useHttp().post('/admin/passport/sso/authorize', data)
}

export function ssoCallback(data: TenantSSOCallbackPayload): Promise<ResponseStruct<TenantLoginResult>> {
  return useHttp().post('/admin/passport/sso/callback', data)
}
