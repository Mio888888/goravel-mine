import type { PageList, ResponseStruct } from '#/global'

export type ModuleLifecycleAction = 'install' | 'upgrade' | 'rollback' | 'uninstall'
export type ModuleLifecycleStatus = 'planned' | 'running' | 'succeeded' | 'failed' | 'skipped' | 'lock_blocked' | 'manual_required' | 'reconciliation_required'

export interface ModuleLifecycleStateVo {
  id: string
  name: string
  version: string
  compatible: string
  enabled: boolean
  reason?: string
  depends_on?: Array<{
    id: string
    version_constraint?: string
    required: boolean
  }>
  lifecycle: {
    install: string
    uninstall: string
    upgrade: string
    rollback: string
    destructive_check: string
    supports_hot_disable: boolean
    requires_restart: boolean
    breaking_change_policy: string
  }
  frontend: {
    module_path?: string
    api_files?: string[]
    route_files?: string[]
    locale_files?: string[]
    type_files?: string[]
    test_files?: string[]
  }
  seed_strategy: {
    mode: string
    idempotent: boolean
    command?: string
    notes?: string
  }
  persisted?: {
    status: string
    enabled: boolean
    owner?: string
    target_version?: string
    last_action?: string
    last_run_key?: string
    last_error?: string
    installed_at?: string
    upgraded_at?: string
    disabled_at?: string
    last_run_at?: string
    disabled_reason?: string
  }
}

export interface ModuleLifecycleRunVo {
  id: number
  idempotency_key: string
  module_id: string
  action: ModuleLifecycleAction
  from_version?: string
  to_version?: string
  status: ModuleLifecycleStatus
  dry_run: boolean
  owner?: string
  reason?: string
  command?: string
  error?: string
  started_at?: string
  finished_at?: string
  created_at?: string
}

export interface ModuleLifecycleStepVo {
  id: number
  attempt_key: string
  run_key: string
  module_id: string
  action: ModuleLifecycleAction
  step_name: string
  command?: string
  status: ModuleLifecycleStatus
  stdout?: string
  stderr?: string
  error?: string
  started_at?: string
  finished_at?: string
  created_at?: string
}

export interface ModuleLifecycleLockVo {
  id: number
  key: string
  owner: string
  run_key: string
  expires_at?: string
  created_at?: string
  updated_at?: string
}

export interface ModuleLifecycleDiffVo {
  module_id: string
  name: string
  manifest_version: string
  persisted_version?: string
  manifest_enabled: boolean
  persisted_enabled: boolean
  persisted_status?: string
  last_action?: string
  drift: string
}

export interface ModuleLifecycleLockReleasePayload {
  key?: string
  dry_run?: boolean
  confirm_token?: string
  reauth_token?: string
  approval_id?: string
}

export interface ModuleLifecycleLockReleaseResult {
  dry_run: boolean
  released: ModuleLifecycleLockVo[]
}

export interface ModuleLifecycleExecutePayload {
  action: ModuleLifecycleAction
  module_id?: string
  execute?: boolean
  owner?: string
  reason?: string
  confirm_token?: string
  reauth_token?: string
  approval_id?: string
}

export interface ModuleLifecycleResultItem {
  module_id: string
  name: string
  action: ModuleLifecycleAction
  status: ModuleLifecycleStatus
  skipped?: boolean
  command?: string
  destructive_check?: string
  idempotency_key: string
  error?: string
}

export interface ModuleLifecycleResultVo {
  action: ModuleLifecycleAction
  dry_run: boolean
  owner?: string
  reason?: string
  items: ModuleLifecycleResultItem[]
}

export interface ModuleLifecycleStepSearchVo {
  run_key?: string
  module_id?: string
  action?: string
  status?: string
  page?: number
  page_size?: number
}

export interface ModuleLifecycleRunSearchVo extends ModuleLifecycleStepSearchVo {
  owner?: string
  [key: string]: any
}

export function states(): Promise<ResponseStruct<PageList<ModuleLifecycleStateVo>>> {
  return useHttp().get('/admin/platform/module-lifecycle/state')
}

export function runs(data: ModuleLifecycleRunSearchVo): Promise<ResponseStruct<PageList<ModuleLifecycleRunVo>>> {
  return useHttp().get('/admin/platform/module-lifecycle/runs', { params: data })
}

export function steps(data: ModuleLifecycleStepSearchVo): Promise<ResponseStruct<PageList<ModuleLifecycleStepVo>>> {
  return useHttp().get('/admin/platform/module-lifecycle/steps', { params: data })
}

export function locks(): Promise<ResponseStruct<PageList<ModuleLifecycleLockVo>>> {
  return useHttp().get('/admin/platform/module-lifecycle/locks')
}

export function stateDiff(): Promise<ResponseStruct<PageList<ModuleLifecycleDiffVo>>> {
  return useHttp().get('/admin/platform/module-lifecycle/diff')
}

export function releaseStaleLocks(data: ModuleLifecycleLockReleasePayload): Promise<ResponseStruct<ModuleLifecycleLockReleaseResult>> {
  return useHttp().post('/admin/platform/module-lifecycle/locks/release-stale', data)
}

export function execute(data: ModuleLifecycleExecutePayload): Promise<ResponseStruct<ModuleLifecycleResultVo>> {
  return useHttp().post('/admin/platform/module-lifecycle/execute', data)
}
