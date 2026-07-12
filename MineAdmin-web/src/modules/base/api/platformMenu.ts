import type { ResponseStruct } from '#/global'
import type { MenuVo } from './menu'

export type PlatformMenuVo = MenuVo
export type { MenuVo }

export function page(): Promise<ResponseStruct<PlatformMenuVo[]>> {
  return useHttp().get('/admin/platform/menu/list')
}

export function create(data: PlatformMenuVo): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/platform/menu', data)
}

export function save(id: number, data: PlatformMenuVo): Promise<ResponseStruct<null>> {
  return useHttp().put(`/admin/platform/menu/${id}`, data)
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/menu', { data: ids })
}
