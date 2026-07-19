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

export interface AttachmentVo extends Omit<Partial<OperationParams<'adminAttachmentList'>>, 'suffix'> {
  id?: number
  storage_mode?: string
  storage_config_id?: number
  origin_name?: string
  object_name?: string
  hash?: string
  mime_type?: string
  storage_path?: string
  suffix?: string | string[]
  size_byte?: number
  size_info?: string
  url?: string
  remark?: string
}

export function upload(file: File): Promise<ResponseStruct<AttachmentVo>> {
  const formData = new FormData()
  const contractFile: OperationRequest<'adminAttachmentUpload'>['file'] = file
  formData.append('file', contractFile)
  return useHttp().post('/admin/attachment/upload', formData)
}

export function pageList(params: AttachmentVo): Promise<ResponseStruct<PageList<AttachmentVo>>> {
  return useHttp().get('/admin/attachment/list', { params })
}

export function deleteById(id: OperationParams<'adminAttachmentDelete'>['id']): Promise<ResponseStruct<null>> {
  return useHttp().delete(`/admin/attachment/${id}`)
}
