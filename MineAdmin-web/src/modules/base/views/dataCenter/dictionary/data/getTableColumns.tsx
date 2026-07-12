import type { MaProTableColumns } from '@mineadmin/pro-table'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElTag } from 'element-plus'
import hasAuth from '@/utils/permission/hasAuth.ts'

export default function getTableColumns(dialog: UseDialogExpose, t: any): MaProTableColumns[] {
  const dictStore = useDictStore()

  return [
    { type: 'index' },
    { label: () => t('baseDictionaryManage.code'), prop: 'code', width: '160px' },
    { label: () => t('baseDictionaryManage.name'), prop: 'name', minWidth: '160px' },
    {
      label: () => t('crud.status'),
      prop: 'status',
      width: '110px',
      cellRender: ({ row }) => (
        <ElTag type={dictStore.t('system-status', row.status, 'color')}>
          {t(dictStore.t('system-status', row.status, 'i18n'))}
        </ElTag>
      ),
    },
    { label: () => t('crud.sort'), prop: 'sort', width: '90px' },
    { label: () => t('baseDictionaryManage.version'), prop: 'version', width: '90px' },
    { label: () => t('crud.remark'), prop: 'remark', minWidth: '180px' },
    {
      type: 'operation',
      label: () => t('crud.operation'),
      width: '120px',
      operationConfigure: {
        type: 'tile',
        actions: [
          {
            name: 'edit',
            icon: 'material-symbols:edit-square-outline',
            show: () => hasAuth('dataCenter:dictionary:update'),
            text: () => t('crud.edit'),
            onClick: ({ row }) => {
              dialog.setTitle(t('crud.edit'))
              dialog.open({ data: row })
            },
          },
        ],
      },
    },
  ]
}
