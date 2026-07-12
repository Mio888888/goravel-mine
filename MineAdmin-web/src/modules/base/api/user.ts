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

type UserPayloadContract = Partial<OperationRequest<'adminUserCreate'>>
type UserInfoUpdateContract = OperationRequest<'adminUserInfoUpdate'>

export interface CurrentUserDepartmentVo {
  id: number
  name: string
}

export interface CurrentUserPositionVo {
  id: number
  dept_id: number
  name: string
}

export interface CurrentUserRoleVo {
  id: number
  code: string
  name: string
}

export interface CurrentUserInfo {
  id: number
  username: string
  nickname: string
  avatar?: string | null
  phone?: string | null
  email?: string | null
  signed?: string | null
  dashboard?: string
  backend_setting?: Record<string, any> | any[] | null
  departments: CurrentUserDepartmentVo[]
  positions: CurrentUserPositionVo[]
  roles: CurrentUserRoleVo[]
}

export interface UserVo extends Partial<Omit<OperationRequest<'adminUserCreate'>, 'department' | 'position' | 'backend_setting' | 'status'>> {
  id?: number
  user_type?: number | string
  status?: number | string
  avatar?: string
  signed?: string
  dashboard?: string
  login_ip?: string
  login_time?: string
  backend_setting?: Record<string, any> | any[] | null
  policy?: any
  department?: Array<CurrentUserDepartmentVo | number>
  position?: Array<CurrentUserPositionVo | number>
  departments?: CurrentUserDepartmentVo[]
  positions?: CurrentUserPositionVo[]
  roles?: CurrentUserRoleVo[]
}

export type UserSearchVo = OperationParams<'adminUserList'>

export function page(data: UserSearchVo): Promise<ResponseStruct<PageList<UserVo>>> {
  return useHttp().get('/admin/user/list', { params: data })
}

export function create(data: UserVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/user', toUserPayload(data))
}

export function save(id: number, data: UserVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/user/${id}`, toUserPayload(data))
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  const data: OperationRequest<'adminUserDelete'> = ids
  return useHttp().delete('/admin/user', { data })
}

export function resetPassword(id: OperationRequest<'adminUserResetPassword'>['id'], evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  const data = { id, ...evidence }
  return useHttp().put('/admin/user/password', data)
}

export function updateInfo(data: UserVo & Partial<UserInfoUpdateContract>): Promise<ResponseStruct<null>> {
  const payload: UserInfoUpdateContract = compactUndefined({
    nickname: data.nickname,
    avatar: data.avatar,
    signed: data.signed,
    backend_setting: objectSetting(data.backend_setting),
    old_password: data.old_password,
    new_password: data.new_password,
    new_password_confirmation: data.new_password_confirmation,
  })
  return useHttp().put('/admin/user/info', payload)
}

export function getUserRole(id: number): Promise<ResponseStruct<any[]>> {
  return useHttp().get(`/admin/user/${id}/roles`)
}

export function setUserRole(id: number, role_codes: string[], evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  const data = { role_codes, ...evidence }
  return useHttp().put(`/admin/user/${id}/roles`, data)
}

export function toUserPayload(data: UserVo): UserPayloadContract {
  return compactUndefined({
    username: data.username,
    password: data.password,
    user_type: data.user_type,
    nickname: data.nickname,
    phone: data.phone,
    email: data.email,
    avatar: data.avatar,
    signed: data.signed,
    dashboard: data.dashboard,
    status: statusValue(data.status),
    department: idList(data.department),
    position: idList(data.position),
    backend_setting: objectSetting(data.backend_setting),
    remark: data.remark,
  })
}

function idList(values?: Array<{ id?: number } | number>): number[] | undefined {
  if (!values) {
    return undefined
  }
  const ids = values?.map(item => typeof item === 'number' ? item : item.id)
    .filter((id): id is number => typeof id === 'number')
  return ids
}

function objectSetting(value: UserVo['backend_setting']): Record<string, unknown> | undefined {
  return value && !Array.isArray(value) ? value : undefined
}

function statusValue(value?: number | string): 1 | 2 | undefined {
  const normalized = Number(value)
  return normalized === 1 || normalized === 2 ? normalized : undefined
}

function compactUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>
}
