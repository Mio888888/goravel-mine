import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import type { StorageConfigVo } from '~/base/api/platformStorageConfig.ts'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElTag } from 'element-plus'
import { deleteByIds } from '~/base/api/platformStorageConfig.ts'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'
import { driverLabel, providerLabel } from './options.ts'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'

export default function getTableColumns(
  dialog: UseDialogExpose,
  t: any,
  requestDeleteEvidence: (ids: number[]) => ReturnType<SensitiveEvidenceRequester>,
): MaProTableColumns[] {
  const dictStore = useDictStore()
  const msg = useMessage()

  return [
    { type: 'selection', showOverflowTooltip: false, label: () => t('crud.selection') },
    { type: 'index' },
    { label: () => t('baseStorageConfigManage.name'), prop: 'name', minWidth: '150px' },
    {
      label: () => t('baseStorageConfigManage.provider'),
      prop: 'provider',
      width: '130px',
      cellRender: ({ row }) => <ElTag>{providerLabel(row.provider)}</ElTag>,
    },
    { label: () => t('baseStorageConfigManage.driver'), prop: 'driver', width: '150px', cellRender: ({ row }) => driverLabel(row.driver) },
    { label: () => t('baseStorageConfigManage.bucket'), prop: 'bucket', minWidth: '150px' },
    { label: () => t('baseStorageConfigManage.endpoint'), prop: 'endpoint', minWidth: '220px' },
    { label: () => t('baseStorageConfigManage.pathPrefix'), prop: 'path_prefix', minWidth: '140px' },
    {
      label: () => t('baseStorageConfigManage.default'),
      prop: 'is_default',
      width: '100px',
      cellRender: ({ row }) => row.is_default
        ? <ElTag type="success">{t('baseStorageConfigManage.defaultEnabled')}</ElTag>
        : <ElTag type="info">{t('baseStorageConfigManage.defaultDisabled')}</ElTag>,
    },
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
    { label: () => t('crud.remark'), prop: 'remark', minWidth: '180px' },
    {
      type: 'operation',
      label: () => t('crud.operation'),
      width: '180px',
      operationConfigure: {
        type: 'tile',
        actions: [
          {
            name: 'edit',
            icon: 'material-symbols:edit-square-outline',
            show: () => hasAuth('platform:storageConfig:update'),
            text: () => t('crud.edit'),
            onClick: ({ row }) => {
              dialog.setTitle(t('crud.edit'))
              dialog.open({ formType: 'edit', data: row })
            },
          },
          {
            name: 'del',
            icon: 'mdi:delete',
            show: () => hasAuth('platform:storageConfig:delete'),
            text: () => t('crud.delete'),
            onClick: ({ row }, proxy: MaProTableExpose) => {
              msg.delConfirm(t('crud.delDataMessage')).then(async () => {
                const ids = [(row as StorageConfigVo).id as number]
                const evidence = await requestDeleteEvidence(ids)
                const response = await deleteByIds(ids, evidence)
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
