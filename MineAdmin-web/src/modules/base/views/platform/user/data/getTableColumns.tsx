import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import type { PlatformUserVo } from '~/base/api/platformUser.ts'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'

import defaultAvatar from '@/assets/images/defaultAvatar.jpg'
import { ElTag } from 'element-plus'
import { useMessage } from '@/hooks/useMessage.ts'
import { deleteByIds, resetPassword } from '~/base/api/platformUser.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'
import { rbacPasswordResource } from '~/base/utils/sensitiveOperation'

export default function getTableColumns(dialog: UseDialogExpose, formRef: any, t: any, requestEvidence: SensitiveEvidenceRequester): MaProTableColumns[] {
  const dictStore = useDictStore()
  const msg = useMessage()

  const showBtn = (auth: string | string[], row: PlatformUserVo) => {
    return hasAuth(auth) && row.id !== 1
  }

  return [
    {
      type: 'selection',
      showOverflowTooltip: false,
      label: () => t('crud.selection'),
      cellRender: ({ row }): any => row.id === 1 ? '-' : undefined,
      selectable: (row: PlatformUserVo) => ![1].includes(row.id as number),
    },
    { type: 'index' },
    {
      label: () => t('basePlatformUserManage.avatar'),
      prop: 'avatar',
      width: '120px',
      cellRender: ({ row }) => (
        <div class="flex-center">
          <el-avatar src={(row.avatar === '' || !row.avatar) ? defaultAvatar : row.avatar} alt={row.username} />
        </div>
      ),
    },
    { label: () => t('basePlatformUserManage.username'), prop: 'username' },
    { label: () => t('basePlatformUserManage.nickname'), prop: 'nickname' },
    {
      label: () => t('basePlatformUserManage.userType'),
      prop: 'user_type',
      cellRender: () => <ElTag type="primary">{t('basePlatformUserManage.platformUser')}</ElTag>,
    },
    { label: () => t('basePlatformUserManage.phone'), prop: 'phone' },
    { label: () => t('basePlatformUserManage.email'), prop: 'email' },
    {
      label: () => t('crud.status'),
      prop: 'status',
      cellRender: ({ row }) => (
        <ElTag type={dictStore.t('system-status', row.status, 'color')}>
          {t(dictStore.t('system-status', row.status, 'i18n'))}
        </ElTag>
      ),
    },
    {
      type: 'operation',
      label: () => t('crud.operation'),
      operationConfigure: {
        actions: [
          {
            name: 'edit',
            icon: 'material-symbols:person-edit',
            show: ({ row }) => showBtn('platform:user:update', row),
            text: () => t('crud.edit'),
            onClick: ({ row }) => {
              dialog.setTitle(t('crud.edit'))
              dialog.open({ formType: 'edit', data: row })
            },
          },
          {
            name: 'setRole',
            show: ({ row }) => showBtn(['platform:user:getRole', 'platform:user:setRole'], row),
            icon: 'material-symbols:person-add-rounded',
            text: () => t('basePlatformUserManage.setRole'),
            onClick: ({ row }) => {
              dialog.setTitle(t('basePlatformUserManage.setRole'))
              dialog.open({ formType: 'setRole', data: row })
            },
          },
          {
            name: 'initPassword',
            show: ({ row }) => showBtn('platform:user:password', row),
            icon: 'material-symbols:passkey',
            text: () => t('basePlatformUserManage.initPassword'),
            onClick: ({ row }) => {
              msg.confirm(t('basePlatformUserManage.setPassword')).then(async () => {
                const resource = rbacPasswordResource(row.id)
                const evidence = await requestEvidence({ policy_key: 'user.password.reset', scope: 'user.password.reset', resource, reason: `Reset platform user ${row.id} password` })
                const response = await resetPassword(row.id, evidence)
                if (response.code === ResultCode.SUCCESS) {
                  msg.success(t('basePlatformUserManage.setPasswordSuccess'))
                }
              })
            },
          },
          {
            name: 'del',
            show: ({ row }) => showBtn('platform:user:delete', row),
            icon: 'mdi:delete',
            text: () => t('crud.delete'),
            onClick: ({ row }, proxy: MaProTableExpose) => {
              msg.delConfirm(t('crud.delDataMessage')).then(async () => {
                const response = await deleteByIds([row.id])
                if (response.code === ResultCode.SUCCESS) {
                  msg.success(t('crud.delSuccess'))
                  await proxy.refresh()
                }
              })
            },
          },
          {
            name: 'noAllowSuperAdmin',
            show: ({ row }) => row.id === 1,
            disabled: () => true,
            text: () => t('crud.superAdminNoEdit'),
          },
        ],
      },
    },
  ]
}
