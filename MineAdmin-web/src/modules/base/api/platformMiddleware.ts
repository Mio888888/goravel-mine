import type { PageList, ResponseStruct } from '#/global'

export type MiddlewareAdapterHealth = 'UNKNOWN' | 'UP' | 'DEGRADED' | 'DOWN'
export type MessageConsumptionMode = 'CLUSTER' | 'BROADCAST'
export type MessageRouteStatus = 'DRAFT' | 'PUBLISHED'
export type MessageDeliveryStatus = 'PROCESSING' | 'SUCCEEDED' | 'RETRY_SCHEDULED' | 'DEAD_LETTERED' | 'IGNORED'
export type MessageFailureClass = 'RETRYABLE' | 'NON_RETRYABLE' | 'UNKNOWN_RESULT'
export type DeadLetterResolutionStatus = 'OPEN' | 'RESOLVED'
export type ProtectionScope = 'GLOBAL' | 'SERVICE' | 'ENDPOINT' | 'CUSTOM'
export type ProtectionRuleType = 'RATE_LIMIT' | 'SLOW_CALL_CIRCUIT' | 'FAILURE_RATE_CIRCUIT' | 'CONCURRENCY'
export type ProtectionRuleStatus = 'DRAFT' | 'PUBLISHED' | 'ARCHIVED'

export interface MiddlewareAdapterCapabilities {
  persistent?: boolean
  cluster?: boolean
  broadcast?: boolean
  offline_recovery?: boolean
  retry?: boolean
  dead_letter?: boolean
  ordering?: boolean
}

export interface MiddlewareAdapterVo {
  id: number
  adapter_key: string
  name: string
  adapter_type: string
  connection: string
  capabilities: MiddlewareAdapterCapabilities
  config_fingerprint?: string
  configured: boolean
  enabled: boolean
  health_status: MiddlewareAdapterHealth
  last_checked_at?: string
  version: number
  created_at?: string
  updated_at?: string
}

export interface AdapterPayload {
  name: string
  connection: string
  enabled?: boolean
  version?: number
  confirm?: boolean
}

export interface AdapterConnectionTestResultVo {
  adapter_id: number
  status: MiddlewareAdapterHealth
}

export interface MessageTypeDefinitionVo {
  message_type: string
  description: string
  supported_schema_versions: number[]
  sensitive_field_paths: string[]
}

export interface ConsumerDefinitionVo {
  consumer_key: string
  message_type: string
  description: string
  supported_schema_versions: number[]
  default_mode: MessageConsumptionMode
}

export interface MiddlewareRegistryVo {
  message_types: MessageTypeDefinitionVo[]
  consumers: ConsumerDefinitionVo[]
}

export interface MessageRetryPolicy {
  max_attempts: number
  initial_delay_seconds: number
  max_delay_seconds: number
}

export interface MessageDeadLetterPolicy {
  destination?: string
  retention_days?: number
  alert_enabled?: boolean
}

export interface MessageRouteVo {
  id?: number
  name: string
  message_type: string
  adapter_id: number
  destination: string
  consumption_mode: MessageConsumptionMode
  consumer_group: string
  concurrency: number
  ordering_enabled: boolean
  retry_policy: MessageRetryPolicy | Record<string, any>
  dead_letter_policy: MessageDeadLetterPolicy | Record<string, any>
  status?: MessageRouteStatus
  enabled: boolean
  version?: number
  published_at?: string
  adapter?: MiddlewareAdapterVo
  created_at?: string
  updated_at?: string
}

export interface ValidationResultVo {
  valid: boolean
  errors: string[]
  warnings: string[]
}

export interface MessageDeliveryVo {
  id: number
  message_id: string
  message_type: string
  consumer_key: string
  route_id: number
  adapter_id: number
  status: MessageDeliveryStatus
  attempt: number
  received_at?: string
  finished_at?: string
  duration_ms: number
  correlation_id: string
  external_position: string
  error_summary: string
  created_at?: string
}

export interface MessageDeadLetterVo {
  id: number
  message_id: string
  message_type: string
  consumer_key: string
  route_id: number
  adapter_id: number
  envelope?: Record<string, any>
  failure_class: MessageFailureClass
  error_summary: string
  first_failed_at?: string
  last_failed_at?: string
  replay_count: number
  resolution_status: DeadLetterResolutionStatus
  resolved_by?: number
  resolved_at?: string
  created_at?: string
}

export interface ReplayReceiptVo {
  dead_letter_id: number
  message_id: string
  status: 'QUEUED' | 'ALREADY_QUEUED'
}

export interface ProtectionRuleVo {
  type: ProtectionRuleType
  limit?: number
  window_ms?: number
  slow_call_duration_ms?: number
  threshold_percent?: number
  minimum_requests?: number
  statistical_window_ms?: number
  open_duration_ms?: number
  half_open_probes?: number
  half_open_successes?: number
  max_concurrency?: number
}

export interface ProtectionRuleSetVo {
  id?: number
  name: string
  scope: ProtectionScope
  resource_pattern: string
  rules: { rules: ProtectionRuleVo[] }
  status?: ProtectionRuleStatus
  enabled: boolean
  version?: number
  published_version?: number
  published_at?: string
  created_at?: string
  updated_at?: string
}

export interface ProtectionRuleVersionVo {
  id: number
  rule_set_id: number
  version: number
  name: string
  scope: ProtectionScope
  resource_pattern: string
  rules: { rules: ProtectionRuleVo[] }
  enabled: boolean
  published_by: number
  published_at: string
}

export interface ProtectionCircuitVo {
  rule_type: ProtectionRuleType
  state: 'CLOSED' | 'OPEN' | 'HALF_OPEN'
  sample_count: number
  failure_count: number
  slow_count: number
  half_open_in_flight: number
  half_open_successes: number
  opened_at?: string
}

export interface ProtectionRuleStateVo {
  rule_set_id: number
  version: number
  circuits: ProtectionCircuitVo[]
  concurrent: number
}

export interface MiddlewareAdapterMetricVo {
  adapter_type: string
  health_status: MiddlewareAdapterHealth
  count: number
}

export interface MiddlewareDeliveryMetricVo {
  message_type: string
  consumer_key: string
  status: MessageDeliveryStatus
  count: number
  duration_sum_ms: number
}

export interface MiddlewareDeadLetterMetricVo {
  failure_class: MessageFailureClass
  resolution_status: DeadLetterResolutionStatus
  count: number
}

export interface ProtectionMetricVo {
  rule_set_id: number
  version: number
  scope: ProtectionScope
  resource_pattern: string
  passed: number
  rate_limited: number
  circuit_rejected: number
  concurrency_rejected: number
  half_open_probes: number
  calls: number
  failures: number
  duration_sum_ms: number
}

export interface QueueClassMetricVo {
  Class: string
  Pending: number
  OldestAge: number
  ArrivalRate: number
  CompletionRate: number
}

export interface QueueBacklogMetricVo {
  FailedJobs: number
  OutboxPending: number
  OutboxProcessing: number
  OutboxFailed: number
  OutboxSent: number
  Classes: QueueClassMetricVo[]
}

export interface MiddlewareMetricsVo {
  message: {
    adapters: MiddlewareAdapterMetricVo[]
    deliveries: MiddlewareDeliveryMetricVo[]
    dead_letters: MiddlewareDeadLetterMetricVo[]
  }
  outbox: QueueBacklogMetricVo
  protection: ProtectionMetricVo[]
}

export interface PageQuery {
  page?: number
  page_size?: number
  [key: string]: any
}

export function registry(): Promise<ResponseStruct<MiddlewareRegistryVo>> {
  return useHttp().get('/admin/platform/middleware/registry')
}

export function adapters(): Promise<ResponseStruct<MiddlewareAdapterVo[]>> {
  return useHttp().get('/admin/platform/middleware/adapters')
}

export function adapterDetail(id: number): Promise<ResponseStruct<MiddlewareAdapterVo>> {
  return useHttp().get(`/admin/platform/middleware/adapters/${id}`)
}

export function createAdapter(data: AdapterPayload): Promise<ResponseStruct<MiddlewareAdapterVo>> {
  return useHttp().post('/admin/platform/middleware/adapters', data)
}

export function updateAdapter(id: number, data: AdapterPayload): Promise<ResponseStruct<MiddlewareAdapterVo>> {
  return useHttp().put(`/admin/platform/middleware/adapters/${id}`, data)
}

export function checkAdapterHealth(id: number): Promise<ResponseStruct<MiddlewareAdapterVo>> {
  return useHttp().post(`/admin/platform/middleware/adapters/${id}/health`)
}

export function testAdapterConnection(id: number): Promise<ResponseStruct<AdapterConnectionTestResultVo>> {
  return useHttp().post(`/admin/platform/middleware/adapters/${id}/test`)
}

export function setAdapterEnabled(
  id: number,
  enabled: boolean,
  version: number,
  confirm = false,
): Promise<ResponseStruct<MiddlewareAdapterVo>> {
  return useHttp().put(`/admin/platform/middleware/adapters/${id}/${enabled ? 'enable' : 'disable'}`, {
    version,
    confirm,
  })
}

export function routes(data: PageQuery): Promise<ResponseStruct<PageList<MessageRouteVo>>> {
  return useHttp().get('/admin/platform/middleware/routes', { params: data })
}

export function routeDetail(id: number): Promise<ResponseStruct<MessageRouteVo>> {
  return useHttp().get(`/admin/platform/middleware/routes/${id}`)
}

export function createRoute(data: MessageRouteVo): Promise<ResponseStruct<MessageRouteVo>> {
  return useHttp().post('/admin/platform/middleware/routes', data)
}

export function updateRoute(id: number, data: MessageRouteVo): Promise<ResponseStruct<MessageRouteVo>> {
  return useHttp().put(`/admin/platform/middleware/routes/${id}`, data)
}

export function validateRoute(id: number): Promise<ResponseStruct<ValidationResultVo>> {
  return useHttp().post(`/admin/platform/middleware/routes/${id}/validate`)
}

export function publishRoute(
  id: number,
  version: number,
  idempotencyKey: string,
): Promise<ResponseStruct<MessageRouteVo>> {
  return useHttp().post(`/admin/platform/middleware/routes/${id}/publish`, { version }, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}

export function setRouteEnabled(
  id: number,
  enabled: boolean,
  version: number,
): Promise<ResponseStruct<MessageRouteVo>> {
  return useHttp().put(`/admin/platform/middleware/routes/${id}/${enabled ? 'enable' : 'disable'}`, { version })
}

export function deliveries(data: PageQuery): Promise<ResponseStruct<PageList<MessageDeliveryVo>>> {
  return useHttp().get('/admin/platform/middleware/deliveries', { params: data })
}

export function deadLetters(data: PageQuery): Promise<ResponseStruct<PageList<MessageDeadLetterVo>>> {
  return useHttp().get('/admin/platform/middleware/dead-letters', { params: data })
}

export function deadLetterDetail(id: number): Promise<ResponseStruct<MessageDeadLetterVo>> {
  return useHttp().get(`/admin/platform/middleware/dead-letters/${id}`)
}

export function replayDeadLetter(id: number, idempotencyKey: string): Promise<ResponseStruct<ReplayReceiptVo>> {
  return useHttp().post(`/admin/platform/middleware/dead-letters/${id}/replay`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}

export function resolveDeadLetter(id: number): Promise<ResponseStruct<MessageDeadLetterVo>> {
  return useHttp().put(`/admin/platform/middleware/dead-letters/${id}/resolve`)
}

export function protectionRules(data: PageQuery): Promise<ResponseStruct<PageList<ProtectionRuleSetVo>>> {
  return useHttp().get('/admin/platform/middleware/protection-rules', { params: data })
}

export function protectionRuleDetail(id: number): Promise<ResponseStruct<ProtectionRuleSetVo>> {
  return useHttp().get(`/admin/platform/middleware/protection-rules/${id}`)
}

export function createProtectionRule(data: ProtectionRuleSetVo): Promise<ResponseStruct<ProtectionRuleSetVo>> {
  return useHttp().post('/admin/platform/middleware/protection-rules', data)
}

export function updateProtectionRule(
  id: number,
  data: ProtectionRuleSetVo,
): Promise<ResponseStruct<ProtectionRuleSetVo>> {
  return useHttp().put(`/admin/platform/middleware/protection-rules/${id}`, data)
}

export function deleteProtectionRule(id: number, version: number): Promise<ResponseStruct<null>> {
  return useHttp().delete(`/admin/platform/middleware/protection-rules/${id}`, { data: { version } })
}

export function validateProtectionRule(id: number): Promise<ResponseStruct<ValidationResultVo>> {
  return useHttp().post(`/admin/platform/middleware/protection-rules/${id}/validate`)
}

export function publishProtectionRule(
  id: number,
  version: number,
  idempotencyKey: string,
): Promise<ResponseStruct<ProtectionRuleSetVo>> {
  return useHttp().post(`/admin/platform/middleware/protection-rules/${id}/publish`, { version }, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })
}

export function setProtectionRuleEnabled(
  id: number,
  enabled: boolean,
  version: number,
): Promise<ResponseStruct<ProtectionRuleSetVo>> {
  return useHttp().put(`/admin/platform/middleware/protection-rules/${id}/${enabled ? 'enable' : 'disable'}`, { version })
}

export function protectionRuleVersions(id: number): Promise<ResponseStruct<ProtectionRuleVersionVo[]>> {
  return useHttp().get(`/admin/platform/middleware/protection-rules/${id}/versions`)
}

export function rollbackProtectionRule(
  id: number,
  version: number,
  targetVersion: number,
): Promise<ResponseStruct<ProtectionRuleSetVo>> {
  return useHttp().post(`/admin/platform/middleware/protection-rules/${id}/rollback`, {
    version,
    target_version: targetVersion,
  })
}

export function protectionRuleState(id: number): Promise<ResponseStruct<ProtectionRuleStateVo>> {
  return useHttp().get(`/admin/platform/middleware/protection-rules/${id}/state`)
}

export function metrics(): Promise<ResponseStruct<MiddlewareMetricsVo>> {
  return useHttp().get('/admin/platform/middleware/metrics')
}
