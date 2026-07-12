import type { MaProTableColumns } from '@mineadmin/pro-table'
import { ElTag } from 'element-plus'
import { dictColor, dictLabel } from '@/utils/dict'

export default function getTableColumns(t: any): MaProTableColumns[] {
  return [
    { type: 'index' },
    { label: () => t('baseSsoLoginLog.username'), prop: 'username', minWidth: '130px' },
    { label: () => t('baseSsoLoginLog.providerName'), prop: 'provider_display_name', minWidth: '160px' },
    {
      label: () => t('baseSsoLoginLog.providerType'),
      prop: 'provider_type',
      width: '110px',
      cellRender: ({ row }) => <ElTag type={dictColor('sso-provider-type', row.provider_type) as any}>{dictLabel('sso-provider-type', row.provider_type, t)}</ElTag>,
    },
    { label: () => t('baseSsoLoginLog.ssoUserId'), prop: 'sso_user_id', minWidth: '170px' },
    { label: () => t('baseSsoLoginLog.ssoEmail'), prop: 'sso_email', minWidth: '190px' },
    {
      label: () => t('baseSsoLoginLog.status'),
      prop: 'status',
      width: '100px',
      cellRender: ({ row }) => (
        <ElTag type={dictColor('system-state', row.status) as any}>
          {dictLabel('system-state', row.status, t)}
        </ElTag>
      ),
    },
    { label: () => t('baseSsoLoginLog.failureReason'), prop: 'failure_reason', minWidth: '180px' },
    { label: () => t('baseSsoLoginLog.ip'), prop: 'ip', width: '130px' },
    { label: () => t('baseSsoLoginLog.deviceType'), prop: 'device_type', width: '110px' },
    { label: () => t('baseSsoLoginLog.loginAt'), prop: 'login_at', width: '180px' },
  ]
}
