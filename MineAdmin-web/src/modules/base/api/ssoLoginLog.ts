import type { PageList, ResponseStruct } from '#/global'

export interface SSOLoginLogVo {
  id: number
  user_id?: number
  username?: string
  provider_id: number
  provider_name?: string
  provider_display_name?: string
  provider_type?: string
  provider_scene?: string
  binding_id?: number
  sso_user_id?: string
  sso_email?: string
  status: number
  failure_reason?: string
  ip?: string
  user_agent?: string
  device_type?: string
  login_at?: string
}

export interface SSOLoginLogSearchVo {
  page?: number
  page_size?: number
  user_id?: number
  username?: string
  provider_id?: number
  provider_name?: string
  sso_user_id?: string
  sso_email?: string
  status?: number
  start_date?: string
  end_date?: string
}

export interface SSOProviderLogStatVo {
  provider_id: number
  provider_name?: string
  provider_display_name?: string
  total: number
  success_count: number
  fail_count: number
}

export interface SSOLoginStatsVo {
  total: number
  success_count: number
  fail_count: number
  success_rate: number
  providers: SSOProviderLogStatVo[]
}

export function page(data: SSOLoginLogSearchVo): Promise<ResponseStruct<PageList<SSOLoginLogVo>>> {
  return useHttp().get('/admin/sso-login-log/list', { params: data })
}

export function stats(data: SSOLoginLogSearchVo = {}): Promise<ResponseStruct<SSOLoginStatsVo>> {
  return useHttp().get('/admin/sso-login-log/stats', { params: data })
}
