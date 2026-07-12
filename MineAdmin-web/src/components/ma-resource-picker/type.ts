import type { DialogEmits } from 'element-plus'

export interface Resource {
  id?: number
  storage_mode?: number
  origin_name?: string
  object_name?: string
  hash?: string
  mime_type?: string
  storage_path?: string
  suffix?: string
  size_byte?: number
  size_info?: string
  url?: string
}

export interface FileType {
  value: string
  label: string | (() => string)
  icon?: string
  suffix: string
  [key: string]: any
}

// 定义 Props 类型
export interface ResourcePanelProps {
  multiple?: boolean
  limit?: number
  pageSize?: number
  showAction?: boolean
  dbClickConfirm?: boolean
  defaultFileType?: string
  fileTypes?: FileType[]
}

export interface ResourcePanelEmits {
  cancel: () => void
  confirm: (value: Resource[]) => void
}

export interface ResourcePickerProps extends ResourcePanelProps {
  visible: boolean
}
export interface ResourcePickerEmits extends ResourcePanelEmits, DialogEmits {

}
