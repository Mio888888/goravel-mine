/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import type { PageList, ResponseStruct } from '#/global'
import type { OperationParams, OperationRequest } from '@/generated/admin-api'
import type { SensitiveEvidence } from '~/base/utils/sensitiveOperation'

type RolePayloadContract = Partial<OperationRequest<'adminRoleCreate'>>

export interface RoleVo extends Partial<Omit<OperationRequest<'adminRoleCreate'>, 'status'>> {
  id?: number
  data_scope?: number
  status?: number
}

export interface RoleSearchVo extends Omit<OperationParams<'adminRoleList'>, 'status'> {
  status?: number | string
  [key: string]: any
}

export function page(data: RoleSearchVo): Promise<ResponseStruct<PageList<RoleVo>>> {
  return useHttp().get('/admin/role/list', { params: data })
}

export function create(data: RoleVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/role', toRolePayload(data))
}

export function save(id: number, data: RoleVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/role/${id}`, toRolePayload(data))
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  const data: OperationRequest<'adminRoleDelete'> = ids
  return useHttp().delete('/admin/role', { data })
}

export function getRolePermission(id: number): Promise<ResponseStruct<null>> {
  return useHttp().get(`/admin/role/${id}/permissions`)
}

export function setRolePermission(id: number, permissions: string[], evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  const data = { permissions, ...evidence }
  return useHttp().put(`/admin/role/${id}/permissions`, data)
}

export function toRolePayload(data: RoleVo): RolePayloadContract {
  return compactUndefined({
    name: data.name,
    code: data.code,
    status: statusValue(data.status),
    sort: data.sort,
    remark: data.remark,
  })
}

function statusValue(value?: number | string): 1 | 2 | undefined {
  const normalized = Number(value)
  return normalized === 1 || normalized === 2 ? normalized : undefined
}

function compactUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>
}
