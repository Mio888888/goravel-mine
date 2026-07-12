import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import { ElTag } from 'element-plus'
import { unbind } from '~/base/api/ssoUserBinding'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'

export default function getTableColumns(t: any): MaProTableColumns[] {
  const msg = useMessage()

  return [
    { type: 'index' },
    { label: () => t('baseSsoUserBinding.username'), prop: 'username', minWidth: '130px' },
    { label: () => t('baseSsoUserBinding.nickname'), prop: 'nickname', minWidth: '130px' },
    { label: () => t('baseSsoUserBinding.providerName'), prop: 'provider_display_name', minWidth: '160px' },
    {
      label: () => t('baseSsoUserBinding.providerType'),
      prop: 'provider_type',
      width: '110px',
      cellRender: ({ row }) => <ElTag type="info">{row.provider_type}</ElTag>,
    },
    { label: () => t('baseSsoUserBinding.providerScene'), prop: 'provider_scene', width: '110px' },
    { label: () => t('baseSsoUserBinding.ssoUserId'), prop: 'sso_user_id', minWidth: '180px' },
    { label: () => t('baseSsoUserBinding.ssoEmail'), prop: 'sso_email', minWidth: '190px' },
    { label: () => t('baseSsoUserBinding.ssoUsername'), prop: 'sso_username', minWidth: '150px' },
    { label: () => t('baseSsoUserBinding.loginCount'), prop: 'login_count', width: '110px' },
    { label: () => t('baseSsoUserBinding.firstLoginAt'), prop: 'first_login_at', width: '180px' },
    { label: () => t('baseSsoUserBinding.lastLoginAt'), prop: 'last_login_at', width: '180px' },
    {
      type: 'operation',
      label: () => t('crud.operation'),
      width: '110px',
      operationConfigure: {
        type: 'tile',
        actions: [
          {
            name: 'unbind',
            icon: 'mdi:link-off',
            show: () => hasAuth('security:ssoUserBinding:unbind'),
            text: () => t('baseSsoUserBinding.unbind'),
            onClick: ({ row }, proxy: MaProTableExpose) => {
              msg.delConfirm(t('baseSsoUserBinding.unbindConfirm')).then(async () => {
                const response = await unbind(row.id)
                if (response.code === ResultCode.SUCCESS) {
                  msg.success(t('baseSsoUserBinding.unbindSuccess'))
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
