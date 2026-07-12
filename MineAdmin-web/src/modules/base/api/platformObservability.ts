import type { ResponseStruct } from '#/global'

export interface ObservabilitySummary {
  total_requests: number
  inflight: number
  route_count: number
  slow_count: number
  uptime_seconds: number
  slow_routes: Array<{
    route: string
    count: number
  }>
}

export interface SlowRequestVo {
  method: string
  route: string
  path: string
  status: number
  duration_ms: number
  request_id: string
  trace_id: string
  ip: string
  recorded_at: string
  threshold_ms: number
  retention_max: number
}

export interface SlowSqlVo {
  sql: string
  rows: string
  duration_ms: number
  request_id: string
  trace_id: string
  recorded_at: string
  retention_max: number
}

export interface ObservabilityPanelVo {
  summary: ObservabilitySummary
  slow_requests: SlowRequestVo[]
  slow_sql: SlowSqlVo[]
}

export function slowRequests(limit = 20): Promise<ResponseStruct<ObservabilityPanelVo>> {
  return useHttp().get('/admin/platform/observability/slow-requests', { params: { limit } })
}
