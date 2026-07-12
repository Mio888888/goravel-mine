export const imagePixel
  = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII='

export const tenants = [
  {
    id: 1001,
    code: 'acme',
    name: 'Acme 租户',
    plan: 'pro',
    status: 1,
    db_host: '127.0.0.1',
    db_database: 'tenant_acme',
    db_username: 'tenant_app',
    custom_domain: 'acme.example.test',
    branding: { app_name: 'Acme Admin' },
    billing: { subscription_status: 'active', amount_cents: 9900 },
    quotas: { api_rate_per_minute: 600, max_users: 50, max_roles: 10, max_storage_mb: 1024 },
    remark: 'E2E tenant',
    created_at: '2026-07-06 10:00:00',
  },
]

export const ssoProviders = [
  {
    id: 2001,
    name: 'okta-admin',
    display_name: 'Okta Admin',
    scene: 'admin',
    type: 'oidc',
    enabled: true,
    issuer: 'https://idp.example.test',
    client_id: 'mineadmin-web',
    display_order: 1,
    remark: 'E2E provider',
    role_mapping: { admin: ['SuperAdmin'] },
    data_permission_mapping: { tenant: ['acme'] },
    created_at: '2026-07-06 10:10:00',
  },
]

export const scheduledTasks = [
  {
    id: 4001,
    name: 'Nightly Backup',
    code: 'nightly_backup',
    cron_expression: '0 0 2 * * *',
    timezone: 'UTC',
    next_run_at: '2026-07-08 02:00:00',
    task_type: 'backup',
    payload: { connection: 'postgres', target: 'storage/backups' },
    timeout_seconds: 120,
    allow_overlap: false,
    max_log_output: 4000,
    target_ips: ['127.0.0.1'],
    tenant_ids: [1001],
    run_on_one_server: true,
    status: 1,
    last_status: 'success',
    last_run_at: '2026-07-07 02:00:00',
    last_duration_ms: 1200,
    last_message: 'backup completed',
  },
]

export const scheduledTaskLogs = [
  {
    id: 4101,
    task_id: 4001,
    task_name: 'Nightly Backup',
    task_code: 'nightly_backup',
    run_token: 'manual-run-token',
    trigger_mode: 'manual',
    task_type: 'backup',
    node_ip: '127.0.0.1',
    status: 'success',
    started_at: '2026-07-07 12:00:00',
    finished_at: '2026-07-07 12:00:01',
    duration_ms: 1000,
    exit_code: 0,
    stdout: 'backup completed',
    stderr: '',
    error_message: '',
    tenants: [{ id: 1001, code: 'acme', name: 'Acme 租户' }],
  },
]

export const initialAttachments = [
  {
    id: 3001,
    origin_name: 'contract.pdf',
    object_name: 'contract.pdf',
    mime_type: 'application/pdf',
    suffix: 'pdf',
    size_byte: 1024,
    size_info: '1 KB',
    url: '/uploads/contract.pdf',
  },
  {
    id: 3002,
    origin_name: 'avatar.png',
    object_name: 'avatar.png',
    mime_type: 'image/png',
    suffix: 'png',
    size_byte: 128,
    size_info: '128 B',
    url: imagePixel,
  },
]

export const dictOptions = {
  'tenant-status': [
    { label: '启用', value: 1, i18n: 'baseSsoProviderManage.enabled', color: 'success' },
    { label: '禁用', value: 2, i18n: 'baseSsoProviderManage.disabled', color: 'danger' },
    { label: '归档', value: 3, i18n: 'baseTenantManage.archive', color: 'info' },
  ],
  'tenant-subscription-status': [
    { label: 'active', value: 'active', color: 'success' },
  ],
  'system-status': [
    { label: '启用', value: 1, i18n: 'dictionary.system.statusEnabled', color: 'primary' },
    { label: '禁用', value: 2, i18n: 'dictionary.system.statusDisabled', color: 'danger' },
  ],
}

export function tenantBrandingConfig() {
  return {
    code: 'acme',
    name: 'Acme 租户',
    plan: 'pro',
    custom_domain: 'acme.example.test',
    branding: {
      app_name: 'Acme Admin',
      primary_color: '#2563eb',
    },
    features: {
      sso: {
        providers: [
          {
            name: 'okta-tenant',
            display_name: 'Okta Tenant',
            scene: 'admin',
            type: 'oidc',
            enabled: true,
            icon: 'material-symbols:login-rounded',
          },
        ],
      },
    },
  }
}
