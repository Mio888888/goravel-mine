import type { SystemSettings } from '#/global'
import type { MenuVo } from '~/base/api/menu'
/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import type { Router, RouteRecordRaw } from 'vue-router'
import dashboardRoute from '@/router/static-routes/dashboardRoute'
import welcomeRoute from '@/router/static-routes/welcomeRoute'
import { resolveModuleView } from './routeViewResolver'
import DynamicRouteViewOutlet from './routeViewOutlet'

const useRouteStore = defineStore(
  'useRouteStore',
  () => {
    const defaultSetting = ref<SystemSettings.all>(useDefaultSetting())
    // 原始路由
    const routesRaw = ref<RouteRecordRaw[]>([])
    const flatteningRoutesList = ref<RouteRecordRaw[]>([])
    async function initRoutes(router: Router, menus: MenuVo[]) {
      const MineRootLayoutRoute = getMineRootLayoutRoute()
      const menuRoutes = menuToRoutes(menus)
      const routes = mergeDashboardRoutes(menuRoutes, MineRootLayoutRoute)
      const flattenedRoutes = flatteningRoutes(routes)

      const menuRootRoute: RouteRecordRaw = {
        ...MineRootLayoutRoute,
        children: [...(MineRootLayoutRoute.children ?? []), ...routes],
      }

      MineRootLayoutRoute.children?.push(...flattenedRoutes)

      router.hasRoute('MineRootLayoutRoute') && router.removeRoute('MineRootLayoutRoute')
      router.addRoute(MineRootLayoutRoute)
      routesRaw.value = router.getRoutes().map(route => route.name === 'MineRootLayoutRoute' ? menuRootRoute : route)
      flatteningRoutesList.value = flattenedRoutes
    }

    function getMineRootLayoutRoute(): RouteRecordRaw {
      const welcomePage: SystemSettings.welcomePage = defaultSetting.value.welcomePage
      return {
        name: 'MineRootLayoutRoute',
        path: '/',
        component: () => import('@/layouts'),
        redirect: welcomePage.path,
        children: [
          Object.assign(welcomeRoute, {
            name: welcomePage.name,
            path: welcomePage.path,
            meta: {
              title: welcomePage.title,
              i18n: 'menu.welcome',
              icon: welcomePage.icon,
              type: 'M',
              affix: true,
              breadcrumbEnable: true,
              copyright: true,
              cache: true,
            },
          }),
          cloneRouteRecord(dashboardRoute),
          toRecordRawRoute({
            path: '/:pathMatch(.*)*',
            name: 'MineSystemError',
            component: () => import(('@/layouts/[...all].tsx')),
            meta: {
              hidden: true,
              i18n: 'menu.pageError',
            },
          }),
        ],
      }
    }

    /**
     * 扁平化为一层路由
     */
    function flatteningRoutes(routes: any[] = [], breadcrumb: any[] = []) {
      const res: any = []
      routes.forEach((route) => {
        const tmp = { ...route }
        if (tmp.children) {
          const childrenBreadcrumb = [...breadcrumb]
          childrenBreadcrumb.push(route)
          const tmpRoute = { ...route }
          tmpRoute.meta = tmpRoute?.meta ?? {}
          tmpRoute.meta.breadcrumb = childrenBreadcrumb
          delete tmpRoute.children
          res.push(tmpRoute)
          const childrenRoutes = flatteningRoutes(tmp.children, childrenBreadcrumb)
          childrenRoutes.map((item: any) => res.push(item))
        }
        else {
          const tmpBreadcrumb = [...breadcrumb]
          tmpBreadcrumb.push(tmp)
          tmp.meta = tmp?.meta ?? {}
          tmp.meta.breadcrumb = tmpBreadcrumb
          res.push(tmp)
        }
      })
      return res
    }

    function toRecordRawRoute(route: any) {
      return flatteningRoutes([route])[0].meta.breadcrumb[0]
    }

    function cloneRouteRecord(route: RouteRecordRaw): RouteRecordRaw {
      return {
        ...route,
        meta: route.meta ? { ...route.meta } : undefined,
        children: route.children?.map(child => cloneRouteRecord(child)),
      } as RouteRecordRaw
    }

    function mergeDashboardRoutes(routes: RouteRecordRaw[], rootRoute: RouteRecordRaw) {
      const dashboardMenuIndex = routes.findIndex(route => route.name === 'dashboard')
      if (dashboardMenuIndex < 0) {
        return routes
      }

      const dynamicDashboard = routes[dashboardMenuIndex]
      const dynamicChildren = dynamicDashboard.children ?? []
      const mergedRoutes = routes.filter((_, index) => index !== dashboardMenuIndex)
      const rootDashboard = rootRoute.children?.find(route => route.name === 'dashboard')
      if (rootDashboard) {
        rootDashboard.component = DynamicRouteViewOutlet
        const dynamicNames = new Set(dynamicChildren.map(route => route.name).filter(Boolean))
        rootDashboard.children = [
          ...(rootDashboard.children ?? []).filter(route => !dynamicNames.has(route.name)),
          ...dynamicChildren,
        ]
        dynamicChildren.forEach((route) => {
          route.meta = route.meta ?? {}
          route.meta.breadcrumb = [rootDashboard as any, route as any]
        })
      }

      return mergedRoutes
    }

    /**
     * 菜单转路由
     * @param routerMap
     */
    function menuToRoutes(routerMap: MenuVo[]) {
      const accessedRouters: any = []
      routerMap.forEach((item: any) => {
        if (item.meta?.type !== 'B') {
          if (item.meta.type === 'I') {
            item.path = `/MineIframe/${item.name}`
            item.component = () => import(('@/layouts/components/iframe/index.tsx'))
          }

          const suffix: string = item.meta?.componentSuffix ?? '.vue'

          let component: RouteRecordRaw['component']
          if (item.component && item.meta?.type !== 'I') {
            component = resolveModuleView(item.component, suffix, item.name)
          }

          const route = {
            path: item.path,
            name: item.name,
            meta: item.meta,
            children: item.children ? menuToRoutes(item.children) : null,
            component: item.meta?.type === 'I' ? item.component : component,
          }
          accessedRouters.push(route)
        }
      })
      return accessedRouters
    }
    return {
      initRoutes,
      toRecordRawRoute,
      flatteningRoutes,
      routesRaw,
      flatteningRoutesList,
    }
  },
)

export default useRouteStore
