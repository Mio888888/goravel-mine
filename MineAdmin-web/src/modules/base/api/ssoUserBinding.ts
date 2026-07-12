import type { PageList, ResponseStruct } from '#/global'

export interface SSOUserBindingVo {
  id: number
  user_id: number
  username?: string
  nickname?: string
  provider_id: number
  provider_name?: string
  provider_display_name?: string
  provider_type?: string
  provider_scene?: string
  sso_user_id: string
  sso_email?: string
  sso_username?: string
  sso_avatar?: string
  login_count: number
  first_login_at?: string
  last_login_at?: string
  token_expires_at?: string
  created_at?: string
  updated_at?: string
}

export interface SSOUserBindingSearchVo {
  page?: number
  page_size?: number
  user_id?: number
  username?: string
  provider_id?: number
  provider_name?: string
  sso_user_id?: string
  sso_email?: string
  sso_username?: string
}

export function page(data: SSOUserBindingSearchVo): Promise<ResponseStruct<PageList<SSOUserBindingVo>>> {
  return useHttp().get('/admin/sso-user-binding/list', { params: data })
}

export function detail(id: number): Promise<ResponseStruct<SSOUserBindingVo>> {
  return useHttp().get(`/admin/sso-user-binding/${id}`)
}

export function userBindings(userId: number): Promise<ResponseStruct<SSOUserBindingVo[]>> {
  return useHttp().get(`/admin/sso-user-binding/user/${userId}`)
}

export function unbind(id: number): Promise<ResponseStruct<null>> {
  return useHttp().delete(`/admin/sso-user-binding/${id}`)
}
