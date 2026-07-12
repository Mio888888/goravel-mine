import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import type { PlatformDictTypeVo } from '~/base/api/platformDictionary'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElTag } from 'element-plus'
import { deleteByIds } from '~/base/api/platformDictionary'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'

export default function getTableColumns(dialog: UseDialogExpose, t: any): MaProTableColumns[] {
  const dictStore = useDictStore()
  const msg = useMessage()

  return [
    { type: 'selection', showOverflowTooltip: false, label: () => t('crud.selection') },
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
    { label: () => t('baseDictionaryManage.itemCount'), prop: 'items.length', width: '100px' },
    { label: () => t('crud.sort'), prop: 'sort', width: '90px' },
    { label: () => t('baseDictionaryManage.version'), prop: 'version', width: '90px' },
    { label: () => t('crud.remark'), prop: 'remark', minWidth: '180px' },
    {
      type: 'operation',
      label: () => t('crud.operation'),
      width: '190px',
      operationConfigure: {
        type: 'tile',
        actions: [
          {
            name: 'edit',
            icon: 'material-symbols:edit-square-outline',
            show: () => hasAuth('platform:dictionary:update'),
            text: () => t('crud.edit'),
            onClick: ({ row }) => {
              dialog.setTitle(t('crud.edit'))
              dialog.open({ formType: 'edit', data: row })
            },
          },
          {
            name: 'del',
            icon: 'mdi:delete',
            show: () => hasAuth('platform:dictionary:delete'),
            text: () => t('crud.delete'),
            onClick: ({ row }, proxy: MaProTableExpose) => {
              msg.delConfirm(t('baseDictionaryManage.deleteTemplateTip')).then(async () => {
                const response = await deleteByIds([(row as PlatformDictTypeVo).id as number])
                if (response.code === ResultCode.SUCCESS) {
                  msg.success(t('crud.delSuccess'))
                  await proxy.refresh()
                }
              })
            },
          },
        ],
      },
    },
  ]
}
