import type { PageList, ResponseStruct } from '#/global'

export type ScheduledTaskType = 'handler' | 'method' | 'backup' | 'governance' | 'url' | 'script'
export type ScheduledTaskStatus = 1 | 2
export type ScheduledTaskLogStatus = 'running' | 'success' | 'failed' | 'skipped'
export type ScheduledTaskConcurrencyPolicy = 'ALLOW' | 'FORBID' | 'REPLACE'
export type ScheduledTaskMisfirePolicy = 'IGNORE' | 'FIRE_ONCE_NOW' | 'SCHEDULER_DEFAULT'
export type ScheduledTaskScope = 'GLOBAL' | 'PER_TENANT'
export type ScheduledTaskRuntimeState = 'REGISTERED' | 'LEGACY_UNSAFE' | 'HANDLER_UNAVAILABLE'

export interface ScheduledTaskRetryPolicy {
  max_attempts: number
  initial_delay_seconds: number
  max_delay_seconds: number
}

export interface ScheduledTaskHandlerDefinitionVo {
  handler_key: string
  description: string
  parameter_schema: Record<string, any>
  default_timeout: number
  tenant_capability: 'GLOBAL_ONLY' | 'PER_TENANT_ALLOWED'
  supports_cancellation: boolean
  privileged: boolean
}

export interface ScheduledTaskVo {
  id?: number
  name?: string
  code?: string
  description?: string
  cron_expression?: string
  timezone?: string
  next_run_at?: string
  task_type?: ScheduledTaskType
  payload?: Record<string, any> | null
  payload_json?: string
  handler_key?: string
  parameters?: Record<string, any> | null
  timeout_seconds?: number
  allow_overlap?: boolean
  concurrency_policy?: ScheduledTaskConcurrencyPolicy
  misfire_policy?: ScheduledTaskMisfirePolicy
  retry_policy?: ScheduledTaskRetryPolicy | Record<string, any> | null
  scope?: ScheduledTaskScope
  max_log_output?: number
  target_ips?: string[]
  target_ips_text?: string
  tenant_ids?: number[]
  run_on_one_server?: boolean
  status?: ScheduledTaskStatus
  last_run_at?: string
  last_status?: string
  last_duration_ms?: number
  last_message?: string
  runtime_state?: ScheduledTaskRuntimeState
  version?: number
  remark?: string
  created_at?: string
  updated_at?: string
}

export interface ScheduledTaskLogVo {
  id?: number
  task_id?: number
  task_name?: string
  task_code?: string
  run_token?: string
  logical_execution_id?: string
  idempotency_key?: string
  attempt?: number
  correlation_id?: string
  trigger_mode?: 'schedule' | 'manual'
  task_type?: ScheduledTaskType
  node_ip?: string
  status?: ScheduledTaskLogStatus
  scheduled_at?: string
  started_at?: string
  finished_at?: string
  duration_ms?: number
  exit_code?: number
  http_status?: number
  stdout?: string
  stderr?: string
  error_message?: string
  tenants?: Array<{ id: number, code: string, name?: string }>
}

export interface ScheduledTaskTenantOptionVo {
  id: number
  code: string
  name?: string
}

export interface ScheduledTaskSearchVo {
  name?: string
  code?: string
  task_type?: ScheduledTaskType
  status?: number
  page?: number
  page_size?: number
  [key: string]: any
}

export interface ScheduledTaskLogSearchVo {
  task_id?: number
  task_code?: string
  status?: string
  trigger_mode?: string
  page?: number
  page_size?: number
  [key: string]: any
}

export interface ScheduledTaskReconciliationItemVo {
  task_id: number
  task_code: string
  handler_key: string
  state: ScheduledTaskRuntimeState
  message: string
}

export interface ScheduledTaskReconciliationReportVo {
  checked_at: string
  items: ScheduledTaskReconciliationItemVo[]
  missing: number
  legacy: number
  healthy: number
}

export function page(data: ScheduledTaskSearchVo): Promise<ResponseStruct<PageList<ScheduledTaskVo>>> {
  return useHttp().get('/admin/platform/scheduled-task/list', { params: data })
}

export function logs(data: ScheduledTaskLogSearchVo): Promise<ResponseStruct<PageList<ScheduledTaskLogVo>>> {
  return useHttp().get('/admin/platform/scheduled-task-log/list', { params: data })
}

export function detail(id: number): Promise<ResponseStruct<ScheduledTaskVo>> {
  return useHttp().get(`/admin/platform/scheduled-task/${id}`)
}

export function tenantOptions(): Promise<ResponseStruct<ScheduledTaskTenantOptionVo[]>> {
  return useHttp().get('/admin/platform/scheduled-task/tenant-options')
}

export function handlers(): Promise<ResponseStruct<ScheduledTaskHandlerDefinitionVo[]>> {
  return useHttp().get('/admin/platform/scheduled-task/handlers')
}

export function create(data: ScheduledTaskVo): Promise<ResponseStruct<ScheduledTaskVo>> {
  return useHttp().post('/admin/platform/scheduled-task', data)
}

export function save(id: number, data: ScheduledTaskVo): Promise<ResponseStruct<ScheduledTaskVo>> {
  return useHttp().put(`/admin/platform/scheduled-task/${id}`, data)
}

export function deleteByIds(ids: number[]): Promise<ResponseStruct<null>> {
  return useHttp().delete('/admin/platform/scheduled-task', { data: ids })
}

export function enable(id: number): Promise<ResponseStruct<ScheduledTaskVo>> {
  return useHttp().put(`/admin/platform/scheduled-task/${id}/enable`)
}

export function disable(id: number): Promise<ResponseStruct<ScheduledTaskVo>> {
  return useHttp().put(`/admin/platform/scheduled-task/${id}/disable`)
}

export function run(id: number, idempotencyKey: string): Promise<ResponseStruct<ScheduledTaskLogVo>> {
  return useHttp().post(`/admin/platform/scheduled-task/${id}/run`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}

export function reconcile(): Promise<ResponseStruct<ScheduledTaskReconciliationReportVo>> {
  return useHttp().post('/admin/platform/scheduled-task/reconcile')
}
