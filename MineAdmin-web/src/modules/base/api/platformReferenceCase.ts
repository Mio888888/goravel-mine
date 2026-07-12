import type { PageList, ResponseStruct } from '#/global'

export interface ReferenceCaseVo {
  id?: number
  code?: string
  title?: string
  status?: 1 | 2
  version?: string
  payload?: Record<string, any> | null
  remark?: string
  created_at?: string
  updated_at?: string
}

export interface ReferenceCaseSearchVo {
  code?: string
  title?: string
  status?: number
  version?: string
  page?: number
  page_size?: number
  [key: string]: any
}

export function page(data: ReferenceCaseSearchVo): Promise<ResponseStruct<PageList<ReferenceCaseVo>>> {
  return useHttp().get('/admin/platform/reference-case/list', { params: data })
}

export function create(data: ReferenceCaseVo): Promise<ResponseStruct<ReferenceCaseVo>> {
  return useHttp().post('/admin/platform/reference-case', data)
}

export function save(id: number, data: ReferenceCaseVo): Promise<ResponseStruct<ReferenceCaseVo>> {
  return useHttp().put(`/admin/platform/reference-case/${id}`, data)
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/reference-case', { data: ids })
}
