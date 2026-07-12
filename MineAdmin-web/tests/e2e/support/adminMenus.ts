export function platformMenuTree(readonly = false) {
  return [
    platformTenantMenu(readonly),
    dataCenterMenu(),
    securityMenu(),
    scheduledTaskMenu(readonly),
  ]
}

export function tenantMenuTree() {
  return [
    {
      id: 101,
      name: 'tenantPlatform',
      path: '/platform',
      component: '',
      meta: { title: '租户管理', i18n: 'baseTenantManage.mainTitle', icon: 'i-material-symbols:domain', type: 'M' },
      children: [tenantRoute(102, [])],
    },
    dataCenterMenu(),
  ]
}

export function mockMenuComponentRoutes() {
  return [...platformMenuTree(), ...tenantMenuTree()].flatMap(collectComponentRoutes)
}

export function permissionCatalog() {
  return [
    {
      id: 501,
      parent_id: 0,
      name: 'tenant:permission',
      path: '/permission',
      meta: { title: '权限管理', type: 'M' },
      children: [
        { id: 502, parent_id: 501, name: 'tenant:user', path: '/permission/user', meta: { title: '用户管理', type: 'M' } },
        { id: 503, parent_id: 501, name: 'tenant:role', path: '/permission/role', meta: { title: '角色管理', type: 'M' } },
      ],
    },
  ]
}

function platformTenantMenu(readonly: boolean) {
  const children = readonly
    ? []
    : buttonPermissions(20, [
        'platform:tenant:save',
        'platform:tenant:update',
        'platform:tenant:permissions',
        'platform:tenant:usage',
        'platform:tenant:suspend',
        'platform:tenant:resume',
        'platform:tenant:archive',
        'platform:tenant:destroy',
      ])

  return {
    id: 1,
    name: 'platform',
    path: '/platform',
    component: '',
    meta: { title: '平台管理', i18n: 'menu.platform', icon: 'i-material-symbols:admin-panel-settings-outline', type: 'M' },
    children: [tenantRoute(2, children)],
  }
}

function tenantRoute(id: number, children: Array<Record<string, unknown>>) {
  return {
    id,
    name: 'platform:tenant',
    path: '/platform/tenant',
    component: 'base/views/platform/tenant/index',
    meta: { title: '租户管理', i18n: 'baseTenantManage.mainTitle', icon: 'i-material-symbols:domain', type: 'M', cache: true },
    children,
  }
}

function scheduledTaskMenu(readonly: boolean) {
  const permissions = readonly
    ? ['platform:scheduledTask:list']
    : [
        'platform:scheduledTask:list',
        'platform:scheduledTask:save',
        'platform:scheduledTask:update',
        'platform:scheduledTask:delete',
        'platform:scheduledTask:run',
        'platform:scheduledTask:log',
      ]

  return {
    id: 7,
    name: 'platformSystem',
    path: '/platform-system',
    component: '',
    meta: { title: '平台系统', icon: 'i-material-symbols:settings-applications-outline', type: 'M' },
    children: [
      {
        id: 8,
        name: 'platform:scheduledTask',
        path: '/platform-system/scheduled-task',
        component: 'base/views/platform/scheduledTask/index',
        meta: { title: '计划任务', i18n: 'baseScheduledTaskManage.mainTitle', icon: 'i-material-symbols:event-repeat-outline', type: 'M', cache: true },
        children: buttonPermissions(40, permissions),
      },
      moduleLifecycleMenu(readonly),
      referenceCaseMenu(readonly),
    ],
  }
}

function referenceCaseMenu(readonly: boolean) {
  const permissions = readonly
    ? ['platform:referenceCase:list']
    : ['platform:referenceCase:list', 'platform:referenceCase:save', 'platform:referenceCase:update', 'platform:referenceCase:delete']

  return {
    id: 76,
    name: 'platform:referenceCase',
    path: '/platform-system/reference-case',
    component: 'base/views/platform/referenceCase/index',
    meta: { title: '参考模块', i18n: 'baseMenu.platform.referenceCase', icon: 'i-material-symbols:fact-check-outline', type: 'M', cache: true },
    children: buttonPermissions(77, permissions),
  }
}

function moduleLifecycleMenu(readonly: boolean) {
  const permissions = readonly
    ? ['platform:moduleLifecycle:list']
    : ['platform:moduleLifecycle:list', 'platform:moduleLifecycle:execute', 'platform:moduleLifecycle:log']

  return {
    id: 66,
    name: 'platform:moduleLifecycle',
    path: '/platform-system/module-lifecycle',
    component: 'base/views/platform/moduleLifecycle/index',
    meta: { title: '模块治理', i18n: 'baseMenu.platform.moduleLifecycle', icon: 'i-material-symbols:deployed-code-outline', type: 'M', cache: true },
    children: buttonPermissions(67, permissions),
  }
}

function dataCenterMenu() {
  return {
    id: 3,
    name: 'dataCenter',
    path: '/data-center',
    component: '',
    meta: { title: '数据中心', i18n: 'menu.dataCenter', icon: 'i-material-symbols:database-outline', type: 'M' },
    children: [
      {
        id: 4,
        name: 'dataCenter:attachment',
        path: '/data-center/attachment',
        component: 'base/views/dataCenter/attachment/index',
        meta: { title: '附件管理', i18n: 'menu.dataCenter.attachment', icon: 'i-material-symbols:perm-media-outline', type: 'M', cache: true },
      },
    ],
  }
}

function securityMenu() {
  return {
    id: 5,
    name: 'security',
    path: '/security',
    component: '',
    meta: { title: '安全治理', i18n: 'menu.security', icon: 'i-material-symbols:security', type: 'M' },
    children: [
      {
        id: 6,
        name: 'security:ssoProvider',
        path: '/security/sso/provider',
        component: 'base/views/security/ssoProvider/index',
        meta: { title: '单点登录配置', i18n: 'baseSsoProviderManage.mainTitle', icon: 'i-material-symbols:key-outline', type: 'M', cache: true },
      },
    ],
  }
}

function buttonPermissions(startID: number, names: string[]) {
  return names.map((name, index) => ({
    id: startID + index,
    name,
    path: '',
    component: '',
    meta: { title: name, type: 'B' },
  }))
}

function collectComponentRoutes(menu: any): Array<{ name: string, path: string, component: string }> {
  const children = (menu.children ?? []).flatMap(collectComponentRoutes)
  return menu.component ? [{ name: menu.name, path: menu.path, component: menu.component }, ...children] : children
}
