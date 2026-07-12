import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import type { TenantVo } from '~/base/api/tenant.ts'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElMessageBox, ElTag } from 'element-plus'
import { archive, destroy, resume, suspend, usage } from '~/base/api/tenant.ts'
import { dictColor, dictLabel } from '@/utils/dict'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'
import type { SensitiveEvidenceResult } from '~/base/api/platformSecurityControl'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'
import { tenantChangeResource } from '~/base/utils/sensitiveOperation'

export default function getTableColumns(
  dialog: UseDialogExpose,
  t: any,
  requestEvidence: (ids: number[]) => Promise<SensitiveEvidenceResult>,
  requestSensitiveEvidence: SensitiveEvidenceRequester,
  openTenantExport: (row: TenantVo) => void,
): MaProTableColumns[] {
  const dictStore = useDictStore()
  const msg = useMessage()

  const usageLine = (label: string, value: unknown) => h('p', null, `${label}: ${String(value ?? '-')}`)

  const showUsage = async (row: TenantVo) => {
    const response = await usage(row.id as number)
    if (response.code !== ResultCode.SUCCESS) {
      msg.error(response.message)
      return
    }
    const data = response.data
    await ElMessageBox.alert(
      h('div', null, [
        usageLine(t('baseTenantManage.code'), data.code),
        usageLine(t('baseTenantManage.subscriptionStatus'), data.billing?.subscription_status ?? '-'),
        usageLine(
          t('baseTenantManage.userUsage'),
          `${data.usage?.users ?? 0} / ${data.quotas?.max_users || t('baseTenantManage.unlimited')}`,
        ),
        usageLine(
          t('baseTenantManage.roleUsage'),
          `${data.usage?.roles ?? 0} / ${data.quotas?.max_roles || t('baseTenantManage.unlimited')}`,
        ),
        usageLine(
          t('baseTenantManage.storageUsage'),
          `${data.usage?.storage_mb ?? 0} MB / ${data.quotas?.max_storage_mb || t('baseTenantManage.unlimited')}`,
        ),
        usageLine(
          t('baseTenantManage.apiRatePerMinute'),
          data.quotas?.api_rate_per_minute || t('baseTenantManage.unlimited'),
        ),
      ]),
      t('baseTenantManage.usage'),
    )
  }

  const statusAction = async (
    api: (id: number, evidence: SensitiveEvidenceResult) => Promise<any>,
    status: 1 | 2 | 3,
    row: TenantVo,
    proxy: MaProTableExpose,
    message: string,
    successMessage: string,
  ) => {
    msg.confirm(message).then(async () => {
      const id = row.id as number
      const selector = tenantChangeResource('status', id, status)
      const evidence = await requestSensitiveEvidence({ policy_key: 'tenant.status.change', scope: 'tenant.status.change', resource: selector, reason: `Change tenant ${id} status` })
      const response = await api(id, evidence)
      if (response.code === ResultCode.SUCCESS) {
        msg.success(successMessage)
        await proxy.refresh()
      }
    })
  }

  return [
    { type: 'selection', showOverflowTooltip: false, label: () => t('crud.selection') },
    { type: 'index' },
    { label: () => t('baseTenantManage.code'), prop: 'code', width: '140px' },
    { label: () => t('baseTenantManage.name'), prop: 'name', minWidth: '160px' },
    { label: () => t('baseTenantManage.plan'), prop: 'plan', width: '130px' },
    {
      label: () => t('baseTenantManage.subscriptionStatus'),
      prop: 'billing.subscription_status',
      width: '140px',
      cellRender: ({ row }) => {
        const value = row.billing?.subscription_status ?? 'active'
        return <ElTag type={dictColor('tenant-subscription-status', value) as any}>{dictLabel('tenant-subscription-status', value, t)}</ElTag>
      },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      width: '110px',
      cellRender: ({ row }) => (
        <ElTag type={dictStore.t('tenant-status', row.status, 'color')}>
          {t(dictStore.t('tenant-status', row.status, 'i18n'))}
        </ElTag>
      ),
    },
    { label: () => t('baseTenantManage.dbHost'), prop: 'db_host', minWidth: '150px' },
    { label: () => t('baseTenantManage.dbDatabase'), prop: 'db_database', minWidth: '150px' },
    { label: () => t('baseTenantManage.dbUsername'), prop: 'db_username', minWidth: '140px' },
    { label: () => t('baseTenantManage.customDomain'), prop: 'custom_domain', minWidth: '180px' },
    { label: () => t('baseTenantManage.appName'), prop: 'branding.app_name', minWidth: '150px' },
    { label: () => t('baseTenantManage.apiRatePerMinute'), prop: 'quotas.api_rate_per_minute', width: '150px' },
    { label: () => t('crud.remark'), prop: 'remark', minWidth: '180px' },
    { label: () => t('baseTenantManage.createdAt'), prop: 'created_at', width: '180px' },
    {
      type: 'operation',
      label: () => t('crud.operation'),
      width: '390px',
      operationConfigure: {
        type: 'tile',
        actions: [
          {
            name: 'edit',
            icon: 'material-symbols:edit-square-outline',
            show: () => hasAuth('platform:tenant:update'),
            text: () => t('crud.edit'),
            onClick: ({ row }) => {
              dialog.setTitle(t('crud.edit'))
              dialog.open({ formType: 'edit', data: row })
            },
          },
          {
            name: 'permission',
            icon: 'material-symbols:rule-settings-outline',
            show: () => hasAuth('platform:tenant:permissions'),
            text: () => t('baseTenantManage.setPermission'),
            onClick: ({ row }) => {
              dialog.setTitle(t('baseTenantManage.setPermission'))
              dialog.open({ formType: 'permission', data: row })
            },
          },
          {
            name: 'usage',
            icon: 'material-symbols:monitoring-outline-rounded',
            show: () => hasAuth('platform:tenant:usage'),
            text: () => t('baseTenantManage.usage'),
            onClick: ({ row }) => showUsage(row),
          },
          {
            name: 'export',
            icon: 'material-symbols:download-rounded',
            show: () => hasAuth('platform:tenant:export'),
            text: () => 'Export',
            onClick: ({ row }) => openTenantExport(row),
          },
          {
            name: 'suspend',
            icon: 'material-symbols:pause-circle-outline',
            show: ({ row }) => hasAuth('platform:tenant:suspend') && row.status === 1,
            text: () => t('baseTenantManage.suspend'),
            onClick: ({ row }, proxy: MaProTableExpose) => statusAction(
              suspend,
              2,
              row,
              proxy,
              t('baseTenantManage.suspendConfirm'),
              t('baseTenantManage.suspendSuccess'),
            ),
          },
          {
            name: 'resume',
            icon: 'material-symbols:play-circle-outline',
            show: ({ row }) => hasAuth('platform:tenant:resume') && row.status !== 1,
            text: () => t('baseTenantManage.resume'),
            onClick: ({ row }, proxy: MaProTableExpose) => statusAction(
              resume,
              1,
              row,
              proxy,
              t('baseTenantManage.resumeConfirm'),
              t('baseTenantManage.resumeSuccess'),
            ),
          },
          {
            name: 'archive',
            icon: 'material-symbols:archive-outline',
            show: ({ row }) => hasAuth('platform:tenant:archive') && row.status !== 3,
            text: () => t('baseTenantManage.archive'),
            onClick: ({ row }, proxy: MaProTableExpose) => statusAction(
              archive,
              3,
              row,
              proxy,
              t('baseTenantManage.archiveConfirm'),
              t('baseTenantManage.archiveSuccess'),
            ),
          },
          {
            name: 'destroy',
            icon: 'mdi:delete',
            show: () => hasAuth('platform:tenant:destroy'),
            text: () => t('baseTenantManage.destroy'),
            onClick: async ({ row }, proxy: MaProTableExpose) => {
              try {
                const evidence = await requestEvidence([row.id as number])
                await msg.delConfirm(t('baseTenantManage.destroyConfirm'))
                const response = await destroy({
                  ids: [row.id as number],
                  confirm_code: row.code,
                  ...evidence,
                })
                if (response.code === ResultCode.SUCCESS) {
                  msg.success(t('baseTenantManage.destroySuccess'))
                  await proxy.refresh()
                }
              }
              catch {}
            },
          },
        ],
      },
    },
  ]
}
