import type {
  ScheduledTaskConcurrencyPolicy,
  ScheduledTaskLogStatus,
  ScheduledTaskMisfirePolicy,
  ScheduledTaskRuntimeState,
  ScheduledTaskScope,
  ScheduledTaskType,
} from '~/base/api/platformScheduledTask'

export const taskTypes: Array<{ label: string, value: ScheduledTaskType }> = [
  { label: '注册处理器', value: 'handler' },
  { label: '历史方法任务', value: 'method' },
  { label: '历史备份任务', value: 'backup' },
  { label: '历史治理任务', value: 'governance' },
  { label: '历史 URL 任务', value: 'url' },
  { label: '历史脚本任务', value: 'script' },
]

export const logStatuses: Array<{ label: string, value: ScheduledTaskLogStatus, type: 'primary' | 'success' | 'warning' | 'danger' | 'info' }> = [
  { label: '运行中', value: 'running', type: 'primary' },
  { label: '成功', value: 'success', type: 'success' },
  { label: '失败', value: 'failed', type: 'danger' },
  { label: '跳过', value: 'skipped', type: 'info' },
]

export const concurrencyPolicies: Array<{ label: string, value: ScheduledTaskConcurrencyPolicy }> = [
  { label: '允许并发', value: 'ALLOW' },
  { label: '禁止重叠', value: 'FORBID' },
  { label: '取消旧执行并替换', value: 'REPLACE' },
]

export const misfirePolicies: Array<{ label: string, value: ScheduledTaskMisfirePolicy }> = [
  { label: '忽略错过触发', value: 'IGNORE' },
  { label: '立即补执行一次', value: 'FIRE_ONCE_NOW' },
  { label: '调度器默认策略', value: 'SCHEDULER_DEFAULT' },
]

export const taskScopes: Array<{ label: string, value: ScheduledTaskScope }> = [
  { label: '全局', value: 'GLOBAL' },
  { label: '逐租户', value: 'PER_TENANT' },
]

export function taskTypeLabel(value?: string): string {
  return taskTypes.find(item => item.value === value)?.label ?? value ?? ''
}

export function logStatusMeta(value?: string) {
  return logStatuses.find(item => item.value === value) ?? { label: value ?? '', type: 'info' as const }
}

export function runtimeStateMeta(value?: ScheduledTaskRuntimeState) {
  const rows: Record<ScheduledTaskRuntimeState, { label: string, type: 'success' | 'warning' | 'danger' }> = {
    REGISTERED: { label: '已注册', type: 'success' },
    LEGACY_UNSAFE: { label: '历史高风险', type: 'warning' },
    HANDLER_UNAVAILABLE: { label: '处理器不可用', type: 'danger' },
  }
  return value ? rows[value] : { label: '-', type: 'warning' as const }
}

export function isLegacyTask(row?: { task_type?: ScheduledTaskType, runtime_state?: ScheduledTaskRuntimeState }) {
  return row?.runtime_state === 'LEGACY_UNSAFE' || row?.task_type === 'url' || row?.task_type === 'script'
}
