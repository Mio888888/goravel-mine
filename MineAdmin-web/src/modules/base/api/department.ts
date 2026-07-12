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
import type { LeaderVo } from '~/base/api/leader.ts'
import type { PositionVo } from '~/base/api/position.ts'
import type { OperationRequest } from '@/generated/admin-api'

type DepartmentPayloadContract = Partial<OperationRequest<'adminDepartmentCreate'>>

export interface DepartmentUserPivotVo {
  dept_id?: number
  user_id?: number
}

export interface DepartmentUserVo {
  id?: number
  username?: string
  nickname?: string
  avatar?: string
  phone?: string
  email?: string
  pivot?: DepartmentUserPivotVo
  [key: string]: any
}

export interface DepartmentVo extends Partial<Omit<OperationRequest<'adminDepartmentCreate'>, 'leader' | 'department_users' | 'parent_id'>> {
  id?: number
  parent_id?: number | null
  created_at?: string | null
  updated_at?: string | null
  deleted_at?: string | null
  children?: DepartmentVo[]
  leader?: LeaderVo[]
  positions?: PositionVo[]
  department_users?: DepartmentUserVo[]
  [key: string]: any
}

export interface DepartmentSearchVo {
  name?: string
  [key: string]: any
}

export function page(data: DepartmentSearchVo | null = null): Promise<ResponseStruct<PageList<DepartmentVo>>> {
  return useHttp().get('/admin/department/list?level=1', { params: data })
}

export function create(data: DepartmentVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/department', toDepartmentPayload(data))
}

export function save(id: number, data: DepartmentVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/department/${id}`, toDepartmentPayload(data))
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  const data: OperationRequest<'adminDepartmentDelete'> = ids
  return useHttp().delete('/admin/department', { data })
}

function toDepartmentPayload(data: DepartmentVo): DepartmentPayloadContract {
  return compactUndefined({
    name: data.name,
    parent_id: data.parent_id ?? undefined,
    department_users: idList(data.department_users),
    leader: idList(data.leader),
  })
}

function idList(values?: Array<{ id?: number }>): number[] | undefined {
  if (!values) {
    return undefined
  }
  return values.map(item => item.id).filter((id): id is number => typeof id === 'number')
}

function compactUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>
}
