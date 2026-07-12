import type { PageList, ResponseStruct } from '#/global'
import { toRolePayload } from './role'
import type { RoleSearchVo, RoleVo } from './role'
import type { SensitiveEvidence } from '~/base/utils/sensitiveOperation'

export type PlatformRoleVo = RoleVo
export type PlatformRoleSearchVo = RoleSearchVo
export type { RoleVo }

export function page(data: PlatformRoleSearchVo): Promise<ResponseStruct<PageList<PlatformRoleVo>>> {
  return useHttp().get('/admin/platform/role/list', { params: data })
}

export function create(data: PlatformRoleVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/platform/role', toRolePayload(data))
}

export function save(id: number, data: PlatformRoleVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/role/${id}`, toRolePayload(data))
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/role', { data: ids })
}

export function getRolePermission(id: number): Promise<ResponseStruct<null>> {
  return useHttp().get(`/admin/platform/role/${id}/permissions`)
}

export function setRolePermission(id: number, permissions: string[], evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/role/${id}/permissions`, { permissions, ...evidence })
}
