import type { PageList, ResponseStruct } from '#/global'

export interface TenantPlanVo {
  id?: number
  code?: string
  name?: string
  status?: 1 | 2
  sort?: number
  billing?: Record<string, any> | null
  quotas?: Record<string, any> | null
  features?: Record<string, any> | null
  remark?: string
  created_at?: string
  updated_at?: string
}

export interface TenantPlanSearchVo {
  code?: string
  name?: string
  status?: number
  page?: number
  page_size?: number
  [key: string]: any
}

export function page(data: TenantPlanSearchVo): Promise<ResponseStruct<PageList<TenantPlanVo>>> {
  return useHttp().get('/admin/platform/tenant-plan/list', { params: data })
}

export function options(): Promise<ResponseStruct<TenantPlanVo[]>> {
  return useHttp().get('/admin/platform/tenant-plan/options')
}

export function create(data: TenantPlanVo): Promise<ResponseStruct<TenantPlanVo>> {
  return useHttp().post('/admin/platform/tenant-plan', data)
}

export function save(id: number, data: TenantPlanVo): Promise<ResponseStruct<TenantPlanVo>> {
  return useHttp().put(`/admin/platform/tenant-plan/${id}`, data)
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/tenant-plan', { data: ids })
}
