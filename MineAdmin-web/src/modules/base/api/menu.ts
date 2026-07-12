/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import type { ResponseStruct } from '#/global'
import type { OperationRequest } from '@/generated/admin-api'

export interface MenuVo extends Partial<Omit<OperationRequest<'adminMenuCreate'>, 'btnPermission'>> {
  id?: number
  redirect?: string
  remark?: string
  btnPermission?: MenuVo[]
  [key: string]: any
}

export function page(): Promise<ResponseStruct<MenuVo[]>> {
  return useHttp().get('/admin/menu/list')
}

export function create(data: MenuVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/menu', toMenuPayload(data))
}

export function save(id: number, data: MenuVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/menu/${id}`, toMenuPayload(data))
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  const data: OperationRequest<'adminMenuDelete'> = ids
  return useHttp().delete('/admin/menu', { data })
}

function toMenuPayload(data: MenuVo): Partial<OperationRequest<'adminMenuCreate'>> {
  return compactUndefined({
    parent_id: data.parent_id,
    name: data.name,
    path: data.path,
    component: data.component,
    redirect: data.redirect,
    status: data.status,
    sort: data.sort,
    meta: data.meta,
    btnPermission: data.btnPermission,
    remark: data.remark,
  })
}

function compactUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>
}
