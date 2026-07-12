import type { PageList, ResponseStruct } from '#/global'
import { toUserPayload } from './user'
import type { CurrentUserRoleVo, UserSearchVo, UserVo } from './user'
import type { SensitiveEvidence } from '~/base/utils/sensitiveOperation'

export type PlatformUserVo = UserVo
export type PlatformUserSearchVo = UserSearchVo
export type PlatformCurrentUserRoleVo = CurrentUserRoleVo
export type { UserVo }

export function page(data: PlatformUserSearchVo): Promise<ResponseStruct<PageList<PlatformUserVo>>> {
  return useHttp().get('/admin/platform/user/list', { params: data })
}

export function create(data: PlatformUserVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/platform/user', toUserPayload(data))
}

export function save(id: number, data: PlatformUserVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/user/${id}`, toUserPayload(data))
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/user', { data: ids })
}

export function resetPassword(id: number, evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().put('/admin/platform/user/password', { id, ...evidence })
}

export function getUserRole(id: number): Promise<ResponseStruct<any[]>> {
  return useHttp().get(`/admin/platform/user/${id}/roles`)
}

export function setUserRole(id: number, role_codes: string[], evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/user/${id}/roles`, { role_codes, ...evidence })
}
