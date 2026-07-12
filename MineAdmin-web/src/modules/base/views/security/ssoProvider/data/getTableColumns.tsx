import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import type { SSOProviderVo } from '~/base/api/ssoProvider'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElTag } from 'element-plus'
import { deleteByIds } from '~/base/api/ssoProvider'
import { ssoProviderSceneDict, ssoProviderTypeDict, systemEnabledDict } from './options'
import { dictColor, dictLabel } from '@/utils/dict'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'

export default function getTableColumns(
  dialog: UseDialogExpose,
  t: any,
  requestDeleteEvidence: (ids: number[]) => ReturnType<SensitiveEvidenceRequester>,
): MaProTableColumns[] {
  const msg = useMessage()

  return [
    { type: 'selection', showOverflowTooltip: false, label: () => t('crud.selection') },
    { type: 'index' },
    { label: () => t('baseSsoProviderManage.name'), prop: 'name', width: '150px' },
    { label: () => t('baseSsoProviderManage.displayName'), prop: 'display_name', minWidth: '160px' },
    {
      label: () => t('baseSsoProviderManage.scene'),
      prop: 'scene',
      width: '110px',
      cellRender: ({ row }) => <ElTag type={dictColor(ssoProviderSceneDict, row.scene) as any}>{dictLabel(ssoProviderSceneDict, row.scene, t)}</ElTag>,
    },
    {
      label: () => t('baseSsoProviderManage.type'),
      prop: 'type',
      width: '110px',
      cellRender: ({ row }) => <ElTag type={dictColor(ssoProviderTypeDict, row.type) as any}>{dictLabel(ssoProviderTypeDict, row.type, t)}</ElTag>,
    },
    {
      label: () => t('crud.status'),
      prop: 'enabled',
      width: '100px',
      cellRender: ({ row }) => (
        <ElTag type={dictColor(systemEnabledDict, row.enabled) as any}>
          {dictLabel(systemEnabledDict, row.enabled, t)}
        </ElTag>
      ),
    },
    { label: () => t('baseSsoProviderManage.issuer'), prop: 'issuer', minWidth: '220px' },
    { label: () => t('baseSsoProviderManage.clientId'), prop: 'client_id', minWidth: '160px' },
    { label: () => t('baseSsoProviderManage.displayOrder'), prop: 'display_order', width: '100px' },
    { label: () => t('crud.remark'), prop: 'remark', minWidth: '160px' },
    { label: () => t('crud.createTime'), prop: 'created_at', width: '180px' },
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
            show: () => hasAuth('security:ssoProvider:update'),
            text: () => t('crud.edit'),
            onClick: ({ row }) => {
              dialog.setTitle(t('crud.edit'))
              dialog.open({ formType: 'edit', data: row })
            },
          },
          {
            name: 'del',
            icon: 'mdi:delete',
            show: () => hasAuth('security:ssoProvider:delete'),
            text: () => t('crud.delete'),
            onClick: ({ row }, proxy: MaProTableExpose) => {
              msg.delConfirm(t('crud.delDataMessage')).then(async () => {
                const ids = [(row as SSOProviderVo).id as number]
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
