import type { PageList, ResponseStruct } from '#/global'
import type { SensitiveEvidence } from '~/base/utils/sensitiveOperation'

export type SSOProviderType = 'oidc' | 'oauth2' | 'saml'

export interface SSOProviderVo {
  id?: number
  name?: string
  display_name?: string
  scene?: string
  type?: SSOProviderType
  enabled?: boolean
  issuer?: string
  audience?: string
  jwt_secret?: string
  discovery_url?: string
  authorization_endpoint?: string
  token_endpoint?: string
  userinfo_endpoint?: string
  jwks_uri?: string
  jwks_json?: string
  client_id?: string
  client_secret?: string
  scope?: string
  redirect_uri?: string
  enable_pkce?: boolean
  enable_nonce?: boolean
  auto_create?: boolean
  icon?: string
  button_color?: string
  display_order?: number
  saml_entrypoint?: string
  saml_entity_id?: string
  saml_certificate?: string
  role_mapping?: Record<string, any> | null
  data_permission_mapping?: Record<string, any> | null
  remark?: string
  created_at?: string
  updated_at?: string
  [key: string]: any
}

export interface SSOProviderSearchVo {
  name?: string
  display_name?: string
  scene?: string
  type?: string
  enabled?: string | boolean
  page?: number
  page_size?: number
  [key: string]: any
}

export function page(data: SSOProviderSearchVo): Promise<ResponseStruct<PageList<SSOProviderVo>>> {
  return useHttp().get('/admin/sso-provider/list', { params: data })
}

export function create(data: SSOProviderVo, evidence?: SensitiveEvidence): Promise<ResponseStruct<SSOProviderVo>> {
  return useHttp().post('/admin/sso-provider', { ...data, ...evidence })
}

export function save(id: number, data: SSOProviderVo, evidence?: SensitiveEvidence): Promise<ResponseStruct<SSOProviderVo>> {
  return useHttp().put(`/admin/sso-provider/${id}`, { ...data, ...evidence })
}

export function deleteByIds(ids: number[], evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/sso-provider', { data: { ids, ...evidence } })
}
