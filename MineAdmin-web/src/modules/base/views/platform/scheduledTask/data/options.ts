import type { ScheduledTaskLogStatus, ScheduledTaskType } from '~/base/api/platformScheduledTask'

export const taskTypes: Array<{ label: string, value: ScheduledTaskType }> = [
  { label: '请求 URL', value: 'url' },
  { label: '执行脚本', value: 'script' },
  { label: '执行方法', value: 'method' },
  { label: '备份任务', value: 'backup' },
]

export const logStatuses: Array<{ label: string, value: ScheduledTaskLogStatus, type: 'primary' | 'success' | 'warning' | 'danger' | 'info' }> = [
  { label: '运行中', value: 'running', type: 'primary' },
  { label: '成功', value: 'success', type: 'success' },
  { label: '失败', value: 'failed', type: 'danger' },
  { label: '跳过', value: 'skipped', type: 'info' },
]

export function taskTypeLabel(value?: string): string {
  return taskTypes.find(item => item.value === value)?.label ?? value ?? ''
}

export function logStatusMeta(value?: string) {
  return logStatuses.find(item => item.value === value) ?? { label: value ?? '', type: 'info' as const }
}
