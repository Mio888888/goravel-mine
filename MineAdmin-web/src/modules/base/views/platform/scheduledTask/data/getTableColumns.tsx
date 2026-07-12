import type { MaProTableColumns, MaProTableExpose } from '@mineadmin/pro-table'
import type { ScheduledTaskVo } from '~/base/api/platformScheduledTask'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElTag } from 'element-plus'
import { deleteByIds, detail, disable, enable, run } from '~/base/api/platformScheduledTask'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import hasAuth from '@/utils/permission/hasAuth.ts'
import { logStatusMeta, taskTypeLabel } from './options.ts'

const operationLinkProps = {
  style: {
    whiteSpace: 'nowrap',
  },
}

export default function getTableColumns(dialog: UseDialogExpose, openLogs: (row: ScheduledTaskVo) => void, t: any): MaProTableColumns[] {
  const dictStore = useDictStore()
  const msg = useMessage()

  async function refreshAfter(response: any, proxy: MaProTableExpose, message: string) {
    if (response.code === ResultCode.SUCCESS) {
      msg.success(message)
      await proxy.refresh()
    }
    else {
      msg.error(response.message)
    }
  }

  return [
    ...baseColumns(t),
    statusColumn(dictStore, t),
    operationColumn(dialog, openLogs, refreshAfter, msg, t),
  ]
}

function baseColumns(t: any): MaProTableColumns[] {
  return [
    { type: 'selection', showOverflowTooltip: false, label: () => t('crud.selection') },
    { type: 'index' },
    { label: () => t('baseScheduledTaskManage.name'), prop: 'name', minWidth: '150px' },
    { label: () => t('baseScheduledTaskManage.code'), prop: 'code', minWidth: '150px' },
    {
      label: () => t('baseScheduledTaskManage.taskType'),
      prop: 'task_type',
      width: '110px',
      cellRender: ({ row }) => <ElTag>{taskTypeLabel(row.task_type)}</ElTag>,
    },
    { label: () => t('baseScheduledTaskManage.cron'), prop: 'cron_expression', minWidth: '150px' },
    { label: () => t('baseScheduledTaskManage.nextRunAt'), prop: 'next_run_at', minWidth: '170px' },
    {
      label: () => t('baseScheduledTaskManage.lastStatus'),
      prop: 'last_status',
      width: '110px',
      cellRender: ({ row }) => row.last_status
        ? <ElTag type={logStatusMeta(row.last_status).type}>{logStatusMeta(row.last_status).label}</ElTag>
        : <ElTag type="info">-</ElTag>,
    },
  ]
}

function statusColumn(dictStore: any, t: any): MaProTableColumns {
  return {
    label: () => t('crud.status'),
    prop: 'status',
    width: '100px',
    cellRender: ({ row }) => (
      <ElTag type={dictStore.t('system-status', row.status, 'color')}>
        {t(dictStore.t('system-status', row.status, 'i18n'))}
      </ElTag>
    ),
  }
}

function operationColumn(dialog: UseDialogExpose, openLogs: (row: ScheduledTaskVo) => void, refreshAfter: any, msg: any, t: any): MaProTableColumns {
  return {
    type: 'operation',
    label: () => t('crud.operation'),
    width: '360px',
    operationConfigure: { type: 'tile', actions: operationActions(dialog, openLogs, refreshAfter, msg, t) },
  }
}

function operationActions(dialog: UseDialogExpose, openLogs: (row: ScheduledTaskVo) => void, refreshAfter: any, msg: any, t: any) {
  return [
    editAction(dialog, msg, t),
    runAction(refreshAfter, t),
    toggleAction(refreshAfter, t),
    logAction(openLogs, t),
    deleteAction(refreshAfter, msg, t),
  ]
}

function editAction(dialog: UseDialogExpose, msg: any, t: any) {
  return {
    name: 'edit',
    icon: 'material-symbols:edit-square-outline',
    show: () => hasAuth('platform:scheduledTask:update'),
    linkProps: operationLinkProps,
    text: () => t('crud.edit'),
    onClick: async ({ row }: any) => {
      const response = await detail(row.id)
      if (response.code !== ResultCode.SUCCESS) {
        msg.error(response.message)
        return
      }
      dialog.setTitle(t('crud.edit'))
      dialog.open({ formType: 'edit', data: response.data })
    },
  }
}

function runAction(refreshAfter: any, t: any) {
  return {
    name: 'run',
    icon: 'material-symbols:play-arrow-outline',
    show: () => hasAuth('platform:scheduledTask:run'),
    linkProps: operationLinkProps,
    text: () => t('baseScheduledTaskManage.run'),
    onClick: async ({ row }: any, proxy: MaProTableExpose) => {
      await refreshAfter(await run(row.id), proxy, t('baseScheduledTaskManage.runSuccess'))
    },
  }
}

function toggleAction(refreshAfter: any, t: any) {
  return {
    name: 'toggle',
    icon: 'material-symbols:power-settings-new',
    show: () => hasAuth('platform:scheduledTask:update'),
    linkProps: operationLinkProps,
    text: ({ row }: any): string => row?.status === 1 ? t('baseScheduledTaskManage.disable') : t('baseScheduledTaskManage.enable'),
    onClick: async ({ row }: any, proxy: MaProTableExpose) => {
      const response = row.status === 1 ? await disable(row.id) : await enable(row.id)
      await refreshAfter(response, proxy, t('crud.updateSuccess'))
    },
  }
}

function logAction(openLogs: (row: ScheduledTaskVo) => void, t: any) {
  return {
    name: 'logs',
    icon: 'material-symbols:history',
    show: () => hasAuth('platform:scheduledTask:log'),
    linkProps: operationLinkProps,
    text: () => t('baseScheduledTaskManage.logs'),
    onClick: ({ row }: any) => openLogs(row),
  }
}

function deleteAction(refreshAfter: any, msg: any, t: any) {
  return {
    name: 'del',
    icon: 'mdi:delete',
    show: () => hasAuth('platform:scheduledTask:delete'),
    linkProps: operationLinkProps,
    text: () => t('crud.delete'),
    onClick: ({ row }: any, proxy: MaProTableExpose) => {
      msg.delConfirm(t('crud.delDataMessage')).then(async () => {
        await refreshAfter(await deleteByIds([(row as ScheduledTaskVo).id as number]), proxy, t('crud.delSuccess'))
      })
    },
  }
}
