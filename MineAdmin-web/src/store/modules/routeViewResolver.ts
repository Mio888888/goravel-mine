import type { RouteRecordRaw } from 'vue-router'

const moduleViews = import.meta.glob('../../modules/**/views/**/*.{vue,jsx,tsx}') as Record<string, NonNullable<RouteRecordRaw['component']>>

export class DynamicRouteResolutionError extends Error {
  public readonly expectedGlobKey: string

  constructor(
    public readonly routeName: string,
    public readonly componentPath: string,
    expectedGlobKey = moduleViewKey(componentPath),
  ) {
    super(`动态路由 ${routeName} 找不到组件 ${componentPath}，预期模块键为 ${expectedGlobKey}`)
    this.name = 'DynamicRouteResolutionError'
    this.expectedGlobKey = expectedGlobKey
  }
}

export function resolveModuleView(componentPath: string, suffix = '.vue', routeName = componentPath): RouteRecordRaw['component'] {
  const expectedGlobKey = moduleViewKey(componentPath, suffix)
  const loader = moduleViews[expectedGlobKey]
  if (!loader) {
    throw new DynamicRouteResolutionError(routeName, componentPath, expectedGlobKey)
  }
  return loader
}

function moduleViewKey(componentPath: string, suffix = '.vue') {
  return `../../modules/${componentPath}${suffix}`
}
