import type { PageList, ResponseStruct } from '#/global'
import type { OperationParams, OperationRequest } from '@/generated/admin-api'
import type { SensitiveEvidence } from '~/base/utils/sensitiveOperation'

export type StorageProvider = OperationRequest<'adminPlatformStorageConfigCreate'>['provider']
export type StorageDriver = OperationRequest<'adminPlatformStorageConfigCreate'>['driver']

export interface StorageConfigVo extends Partial<Omit<OperationRequest<'adminPlatformStorageConfigCreate'>, 'options' | 'status'>> {
  id?: number
  options?: Record<string, any> | null
  status?: number
  options_json?: string
  created_at?: string
  updated_at?: string
}

export interface StorageConfigSearchVo extends Omit<OperationParams<'adminPlatformStorageConfigList'>, 'provider' | 'driver' | 'status'> {
  provider?: StorageProvider
  driver?: StorageDriver
  status?: number | string
  [key: string]: any
}

export function page(data: StorageConfigSearchVo): Promise<ResponseStruct<PageList<StorageConfigVo>>> {
  return useHttp().get('/admin/platform/storage-config/list', { params: data })
}

export function create(data: StorageConfigVo, evidence?: SensitiveEvidence): Promise<ResponseStruct<StorageConfigVo>> {
  return useHttp().post('/admin/platform/storage-config', { ...toStoragePayload(data), ...evidence })
}

export function save(id: number, data: StorageConfigVo, evidence?: SensitiveEvidence): Promise<ResponseStruct<StorageConfigVo>> {
  return useHttp().put(`/admin/platform/storage-config/${id}`, { ...toStoragePayload(data), ...evidence })
}

export function deleteByIds(ids: number[], evidence: SensitiveEvidence): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/storage-config', { data: { ids, ...evidence } })
}

function toStoragePayload(data: StorageConfigVo): Partial<OperationRequest<'adminPlatformStorageConfigCreate'>> {
  return compactUndefined({
    name: data.name,
    provider: data.provider,
    driver: data.driver,
    bucket: data.bucket,
    endpoint: data.endpoint,
    region: data.region,
    access_key: data.access_key,
    secret_key: data.secret_key,
    base_url: data.base_url,
    path_prefix: data.path_prefix,
    is_default: data.is_default,
    status: statusValue(data.status),
    options: data.options ?? undefined,
    remark: data.remark,
  })
}

function statusValue(value?: number | string): 1 | 2 | undefined {
  const normalized = Number(value)
  return normalized === 1 || normalized === 2 ? normalized : undefined
}

function compactUndefined<T extends Record<string, unknown>>(value: T): Partial<T> {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as Partial<T>
}
