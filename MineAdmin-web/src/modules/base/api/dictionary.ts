import type { Dictionary, PageList, ResponseStruct } from '#/global'

export interface DictItemVo {
  id?: number
  type_id?: number
  source_id?: number
  source_code?: string
  type_code?: string
  label?: string
  value?: string
  i18n?: string
  color?: string
  status?: 1 | 2
  sort?: number
  version?: number
  is_system?: boolean
  remark?: string
  created_at?: string
  updated_at?: string
}

export interface DictTypeVo {
  id?: number
  source_id?: number
  source_code?: string
  code?: string
  name?: string
  status?: 1 | 2
  sort?: number
  version?: number
  is_system?: boolean
  remark?: string
  items?: DictItemVo[]
  created_at?: string
  updated_at?: string
}

export interface DictSearchVo {
  code?: string
  name?: string
  status?: number
  page?: number
  page_size?: number
  [key: string]: any
}

export function page(data: DictSearchVo): Promise<ResponseStruct<PageList<DictTypeVo>>> {
  return useHttp().get('/admin/dictionary/list', { params: data })
}

export function items(id: number): Promise<ResponseStruct<DictItemVo[]>> {
  return useHttp().get(`/admin/dictionary/${id}/items`)
}

export function options(code?: string): Promise<ResponseStruct<Record<string, Dictionary[]> | Dictionary[]>> {
  const scope = useUserStore().authScope
  const url = scope === 'platform' ? '/admin/platform/dictionary/options' : '/admin/dictionary/options'
  return useHttp().get(url, { params: code ? { code } : {} })
}

export function saveType(id: number, data: DictTypeVo): Promise<ResponseStruct<DictTypeVo>> {
  return useHttp().put(`/admin/dictionary/${id}`, data)
}

export function saveItem(id: number, data: DictItemVo): Promise<ResponseStruct<DictItemVo>> {
  return useHttp().put(`/admin/dictionary-item/${id}`, data)
}
