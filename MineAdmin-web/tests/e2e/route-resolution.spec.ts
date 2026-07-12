import type { Request } from '@playwright/test'
import { expect, test } from '@playwright/test'
import { mkdir, writeFile } from 'node:fs/promises'
import { loginAsAdmin, mockAdminApi, mockAdminDiagnostics } from './support/adminMock'
import { mockMenuComponentRoutes, platformMenuTree } from './support/adminMenus'

test.use({ trace: 'off' })

test('hash-mode SSO callback reads IdP query before the hash', async ({ page }) => {
  await page.goto('/?code=auth-code&state=auth-state#/login')
  const callback = await page.evaluate(async () => {
    const { resolveSSOCallbackParameters } = await import('/src/modules/base/utils/ssoCallback.ts')
    return resolveSSOCallbackParameters({})
  })
  expect(callback).toEqual({ code: 'auth-code', state: 'auth-state' })
})

test('dynamic menu keeps parent and child hierarchy', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page)

  const hierarchy = await page.evaluate(async () => {
    const { default: useRouteStore } = await import('/src/store/modules/useRouteStore.ts')
    const root = useRouteStore().routesRaw.find(route => route.name === 'MineRootLayoutRoute')
    const platform = root?.children?.find(route => route.name === 'platform')
    return {
      rootNames: root?.children?.map(route => route.name) ?? [],
      platformChildren: platform?.children?.map(route => route.name) ?? [],
    }
  })

  expect(hierarchy.rootNames).toContain('platform')
  expect(hierarchy.rootNames).not.toContain('platform:tenant')
  expect(hierarchy.platformChildren).toContain('platform:tenant')
})

test('unknown menu component rejects dynamic route initialization', async ({ page }) => {
  await mockAdminApi(page)
  await page.route('**/dev/admin/platform/permission/menus', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'success',
        data: [
          ...platformMenuTree(),
          {
            id: 999,
            name: 'platform:missingView',
            path: '/platform-system/missing-view',
            component: 'base/views/platform/missingView/index',
            meta: { title: '缺失页面', type: 'M' },
          },
        ],
      }),
    })
  })

  await loginAsAdmin(page, { waitForShell: false })

  await expect(page).toHaveURL(/\/login/)
  await expect(page.getByText('平台管理').first()).toHaveCount(0)
  await expect.poll(async () => {
    return page.evaluate(async () => {
      const router = (await import('/src/router/index.ts')).default
      return router.hasRoute('platform:missingView')
    })
  }).toBe(false)
  mockAdminDiagnostics(page).clear()
})

test('dashboard direct refresh retains its matched chain, breadcrumb, and cache metadata', async ({ page }) => {
  await mockAdminApi(page)
  await page.route('**/dev/admin/platform/permission/menus', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        code: 200,
        message: 'success',
        data: [{
          id: 1000,
          name: 'dashboard',
          path: '/dashboard',
          component: '',
          meta: { title: '仪表盘', type: 'M' },
          children: [{
            id: 1001,
            name: 'dashboard:moduleLifecycle',
            path: '/dashboard/module-lifecycle',
            component: 'base/views/platform/moduleLifecycle/index',
            meta: { title: '模块治理', type: 'M', cache: true },
          }],
        }],
      }),
    })
  })

  await loginAsAdmin(page)
  await expect.poll(async () => {
    return page.evaluate(async () => {
      const router = (await import('/src/router/index.ts')).default
      return router.hasRoute('dashboard:moduleLifecycle')
    })
  }).toBe(true)
  await page.goto('/#/dashboard/module-lifecycle')
  await page.reload()

  await expect(page.getByText('Platform RBAC', { exact: true }).first()).toBeVisible()
  await expect(page.locator('.breadcrumb')).toContainText('仪表盘')
  await expect(page.locator('.breadcrumb')).toContainText('模块治理')
  await expect.poll(async () => {
    return page.evaluate(async () => {
      const router = (await import('/src/router/index.ts')).default
      const names = router.getRoutes().map(route => route.name).filter(Boolean)
      return names.length - new Set(names).size
    })
  }).toBe(0)
})

test('dashboard route rebuild drops revoked dynamic children', async ({ page }) => {
  await mockAdminApi(page)
  await loginAsAdmin(page)

  const result = await page.evaluate(async () => {
    const router = (await import('/src/router/index.ts')).default
    const { default: useRouteStore } = await import('/src/store/modules/useRouteStore.ts')
    const routeStore = useRouteStore()
    await routeStore.initRoutes(router, [{
      id: 1000,
      name: 'dashboard',
      path: '/dashboard',
      component: '',
      meta: { title: '仪表盘', type: 'M' },
      children: [{
        id: 1001,
        name: 'dashboard:revoked',
        path: '/dashboard/revoked',
        component: 'base/views/platform/moduleLifecycle/index',
        meta: { title: '待撤销页面', type: 'M' },
      }],
    }] as any)
    const registeredBefore = router.hasRoute('dashboard:revoked')

    await routeStore.initRoutes(router, [{
      id: 1000,
      name: 'dashboard',
      path: '/dashboard',
      component: '',
      meta: { title: '仪表盘', type: 'M' },
      children: [],
    }] as any)
    const root = routeStore.routesRaw.find(route => route.name === 'MineRootLayoutRoute')
    const dashboard = root?.children?.find(route => route.name === 'dashboard')
    return {
      registeredBefore,
      registeredAfter: router.hasRoute('dashboard:revoked'),
      menuAfter: dashboard?.children?.some(route => route.name === 'dashboard:revoked') ?? false,
    }
  })

  expect(result).toEqual({ registeredBefore: true, registeredAfter: false, menuAfter: false })
})

test('every mock menu component renders without route-resolution browser failures', async ({ page, context }, testInfo) => {
  const componentMenus = uniqueComponentMenus(mockMenuComponentRoutes())
  const dynamicViewPaths = new Set(componentMenus.map(menu => `/src/modules/${menu.component}.vue`))
  const pageErrors: string[] = []
  const viteDynamicModuleFailures: string[] = []
  page.on('pageerror', error => pageErrors.push(error.message))
  page.on('requestfailed', (request) => {
    if (isViteDynamicModuleFailure(request, dynamicViewPaths)) {
      viteDynamicModuleFailures.push(`requestfailed ${request.url()}`)
    }
  })
  page.on('response', (response) => {
    if (response.status() >= 400 && isViteDynamicModulePath(response.url(), dynamicViewPaths)) {
      viteDynamicModuleFailures.push(`${response.status()} ${response.url()}`)
    }
  })

  await mockAdminApi(page)
  await loginAsAdmin(page)
  await expect.poll(async () => {
    return page.evaluate(async (menuNames) => {
      const router = (await import('/src/router/index.ts')).default
      return menuNames.filter(name => !router.hasRoute(name))
    }, componentMenus.map(menu => menu.name))
  }).toEqual([])

  const artifactDirectory = testInfo.outputPath('route-resolution')
  await mkdir(artifactDirectory, { recursive: true })
  await context.tracing.start({ screenshots: true, snapshots: true, sources: true })
  try {
    await writeFile(testInfo.outputPath('route-resolution', 'validated-component-keys.json'), JSON.stringify(
      componentMenus.map(menu => `../../modules/${menu.component}.vue`),
      null,
      2,
    ))
    for (const menu of componentMenus) {
      await page.goto(`/#${menu.path}`)
      await expect(page).toHaveURL(new RegExp(escapeRegExp(menu.path)))
      await expect(page.getByText(componentReadyText(menu.component), { exact: true }).first()).toBeVisible()
    }
  }
  finally {
    await context.tracing.stop({ path: testInfo.outputPath('route-resolution', 'route-resolution.trace.zip') })
  }

  expect(pageErrors).toEqual([])
  expect(viteDynamicModuleFailures).toEqual([])
})

function uniqueComponentMenus(menus: Array<{ name: string, path: string, component: string }>) {
  return [...new Map(menus.map(menu => [menu.path, menu])).values()]
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function isViteDynamicModuleFailure(request: Request, dynamicViewPaths: Set<string>) {
  return request.resourceType() === 'script'
    && request.failure()?.errorText !== 'net::ERR_ABORTED'
    && isViteDynamicModulePath(request.url(), dynamicViewPaths)
}

function isViteDynamicModulePath(url: string, dynamicViewPaths: Set<string>) {
  const pathname = new URL(url).pathname
  return dynamicViewPaths.has(pathname) || /^\/assets\/.+\.[cm]?js$/.test(pathname)
}

function componentReadyText(component: string) {
  const readyTextByComponent: Record<string, string> = {
    'base/views/platform/tenant/index': 'Acme 租户',
    'base/views/dataCenter/attachment/index': 'contract.pdf',
    'base/views/security/ssoProvider/index': 'Okta Admin',
    'base/views/platform/scheduledTask/index': 'Nightly Backup',
    'base/views/platform/moduleLifecycle/index': 'Platform RBAC',
    'base/views/platform/referenceCase/index': 'Golden Reference Case',
  }
  return readyTextByComponent[component]
}
