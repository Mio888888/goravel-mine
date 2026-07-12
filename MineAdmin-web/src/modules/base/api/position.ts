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
import type { OperationRequest } from '@/generated/admin-api'

export interface PositionVo extends Partial<OperationRequest<'adminPositionCreate'>> {
  id?: number
  dept_name?: string
  [key: string]: any
}

export interface PositionSearchVo {
  name?: string
  [key: string]: any
}

export function page(data: PositionSearchVo | null = null): Promise<ResponseStruct<PageList<PositionVo>>> {
  return useHttp().get('/admin/position/list', { params: data })
}

export function create(data: PositionVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/position', toPositionPayload(data))
}

export function save(id: number, data: PositionVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/position/${id}`, toPositionPayload(data))
}

export function setDataScope(id: number, data: PositionVo & Partial<OperationRequest<'adminPositionSetDataPermission'>>): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/position/${id}/data_permission`, toDataPermissionPayload(data))
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  const data: OperationRequest<'adminPositionDelete'> = ids
  return useHttp().delete('/admin/position', { data })
}

function toPositionPayload(data: PositionVo): Partial<OperationRequest<'adminPositionCreate'>> {
  return compactUndefined({
    name: data.name,
    dept_id: data.dept_id,
  })
}

function toDataPermissionPayload(data: PositionVo & Partial<OperationRequest<'adminPositionSetDataPermission'>>): Partial<OperationRequest<'adminPositionSetDataPermission'>> {
  return compactUndefined({
    policy_type: data.policy_type,
    value: data.value,
  })
}

function compactUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>
}
