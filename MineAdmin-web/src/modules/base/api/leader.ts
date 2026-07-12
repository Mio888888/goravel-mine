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
import type { UserVo } from '~/base/api/user.ts'
import type { OperationRequest } from '@/generated/admin-api'

export interface LeaderVo extends Partial<Omit<OperationRequest<'adminLeaderCreate'>, 'user_id'>> {
  id?: number
  user_id?: number | number[] | null
  dept_name?: string
  users?: UserVo[]
}

export interface LeaderSearchVo {
  user_id?: string
  [key: string]: any
}

export function page(data: LeaderSearchVo | null = null): Promise<ResponseStruct<PageList<LeaderVo>>> {
  return useHttp().get('/admin/leader/list', { params: data })
}

export function create(data: LeaderVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/leader', toLeaderPayload(data))
}

export function save(id: number, data: LeaderVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/leader/${id}`, toLeaderPayload(data))
}

export function deleteByDoubleKey(dept_id: number, user_ids: number[]): Promise<ResponseStruct<null>> {
  const data: OperationRequest<'adminLeaderDelete'> = { dept_id, user_ids }
  return useHttp().delete('/admin/leader', { data })
}

function toLeaderPayload(data: LeaderVo): Partial<OperationRequest<'adminLeaderCreate'>> {
  return compactUndefined({
    dept_id: data.dept_id,
    user_id: userIdList(data),
  })
}

function userIdList(data: LeaderVo): number[] | undefined {
  if (Array.isArray(data.user_id)) {
    return data.user_id
  }
  if (typeof data.user_id === 'number') {
    return [data.user_id]
  }
  const ids = data.users?.map(item => item.id).filter((id): id is number => typeof id === 'number')
  return ids?.length ? ids : undefined
}

function compactUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>
}
