import type { StorageDriver, StorageProvider } from '~/base/api/platformStorageConfig'

export const storageProviders: Array<{ label: string, value: StorageProvider }> = [
  { label: 'Local', value: 'local' },
  { label: 'MinIO', value: 'minio' },
  { label: 'AWS S3', value: 'aws_s3' },
  { label: '阿里云 OSS', value: 'aliyun_oss' },
  { label: '腾讯云 COS', value: 'tencent_cos' },
  { label: '七牛云', value: 'qiniu' },
  { label: '华为 OBS', value: 'huawei_obs' },
]

export const storageDrivers: Array<{ label: string, value: StorageDriver }> = [
  { label: 'Local', value: 'local' },
  { label: 'S3 Compatible', value: 's3_compatible' },
]

export function providerLabel(value?: string) {
  return storageProviders.find(item => item.value === value)?.label ?? value ?? '-'
}

export function driverLabel(value?: string) {
  return storageDrivers.find(item => item.value === value)?.label ?? value ?? '-'
}
