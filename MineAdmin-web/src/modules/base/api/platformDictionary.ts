import type { PageList, ResponseStruct } from '#/global'
import type { DictSearchVo, DictTypeVo } from './dictionary'

export type PlatformDictTypeVo = DictTypeVo

export function page(data: DictSearchVo): Promise<ResponseStruct<PageList<PlatformDictTypeVo>>> {
  return useHttp().get('/admin/platform/dictionary/list', { params: data })
}

export function detail(id: number): Promise<ResponseStruct<PlatformDictTypeVo>> {
  return useHttp().get(`/admin/platform/dictionary/${id}`)
}

export function create(data: PlatformDictTypeVo): Promise<ResponseStruct<PlatformDictTypeVo>> {
  return useHttp().post('/admin/platform/dictionary', data)
}

export function save(id: number, data: PlatformDictTypeVo): Promise<ResponseStruct<PlatformDictTypeVo>> {
  return useHttp().put(`/admin/platform/dictionary/${id}`, data)
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/dictionary', { data: ids })
}

export function dispatchAll(): Promise<ResponseStruct<null>> {
  return useHttp().post('/admin/platform/dictionary/dispatch')
}

export function dispatchTenant(id: number): Promise<ResponseStruct<null>> {
  return useHttp().post(`/admin/platform/dictionary/dispatch/${id}`)
}
