import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import type { TenantPlanVo } from '~/base/api/platformTenantPlan.ts'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElTag } from 'element-plus'
import { deleteByIds } from '~/base/api/platformTenantPlan.ts'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'

export default function getTableColumns(dialog: UseDialogExpose, t: any): MaProTableColumns[] {
  const dictStore = useDictStore()
  const msg = useMessage()

  return [
    { type: 'selection', showOverflowTooltip: false, label: () => t('crud.selection') },
    { type: 'index' },
    { label: () => t('baseTenantPlanManage.code'), prop: 'code', width: '130px' },
    { label: () => t('baseTenantPlanManage.name'), prop: 'name', minWidth: '150px' },
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
    { label: () => t('baseTenantManage.apiRatePerMinute'), prop: 'quotas.api_rate_per_minute', width: '150px' },
    { label: () => t('baseTenantManage.maxUsers'), prop: 'quotas.max_users', width: '120px' },
    { label: () => t('baseTenantManage.maxRoles'), prop: 'quotas.max_roles', width: '120px' },
    { label: () => t('baseTenantManage.maxStorageMb'), prop: 'quotas.max_storage_mb', width: '150px' },
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
            show: () => hasAuth('platform:tenantPlan:update'),
            text: () => t('crud.edit'),
            onClick: ({ row }) => {
              dialog.setTitle(t('crud.edit'))
              dialog.open({ formType: 'edit', data: row })
            },
          },
          {
            name: 'del',
            icon: 'mdi:delete',
            show: () => hasAuth('platform:tenantPlan:delete'),
            text: () => t('crud.delete'),
            onClick: ({ row }, proxy: MaProTableExpose) => {
              msg.delConfirm(t('crud.delDataMessage')).then(async () => {
                const response = await deleteByIds([(row as TenantPlanVo).id as number])
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
