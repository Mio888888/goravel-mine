/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import useCache from '@/hooks/useCache.ts'
import type { ResponseStruct } from '#/global'
import useThemeColor from '@/hooks/useThemeColor.ts'
import useHttp from '@/hooks/auto-imports/useHttp.ts'
import request from '@/utils/http.ts'
import type { MenuVo, RoleVo } from '~/base/api/permission.ts'
import type { TenantSSOCallbackPayload, TenantSSOLoginPayload } from '~/base/api/tenant.ts'
import { ssoCallback, ssoLogin } from '~/base/api/tenant.ts'
import type { CurrentUserDepartmentVo, CurrentUserInfo, CurrentUserPositionVo, CurrentUserRoleVo } from '~/base/api/user.ts'
import { recursionGetKey } from '@/utils/recursionGetKey.ts'
import type { OperationRequest, OperationResponse } from '@/generated/admin-api'
import { operations } from '@/generated/admin-api'

export interface LoginParams extends OperationRequest<'adminPassportLogin'> {
  auth_scope?: AuthScope
}

export interface MFALoginParams extends OperationRequest<'adminPassportMfaLogin'> {
  auth_scope?: AuthScope
  username?: string
}

export interface PasswordChangeParams extends OperationRequest<'adminPassportPasswordChange'> {
  auth_scope?: AuthScope
  username?: string
}

export type LoginResult = Extract<OperationResponse<'adminPassportLogin'>, { code: 200 }>['data']
export type LoginTokenResult = Extract<LoginResult, { access_token: string }>
export type MFAChallengeResult = Extract<LoginResult, { mfa_required: true }>
export type PasswordChangeChallengeResult = Extract<LoginResult, { password_change_required: true }>

export type UserDepartmentInfo = CurrentUserDepartmentVo
export type UserPositionInfo = CurrentUserPositionVo
export type UserRoleInfo = CurrentUserRoleVo
export type UserInfo = CurrentUserInfo
export type AuthScope = 'tenant' | 'platform'

export type CaptchaResult = Extract<OperationResponse<'adminPassportCaptcha'>, { code: 200 }>['data']

function getInfo(scope: AuthScope): Promise<ResponseStruct<UserInfo>> {
  const path = scope === 'platform' ? operations.adminPlatformPassportGetInfo.path : operations.adminPassportGetInfo.path
  return useHttp().get(path)
}

/**
 * Passport login
 * @param data
 */
function loginApi(data: LoginParams, scope: AuthScope): Promise<ResponseStruct<LoginResult>> {
  const payload: OperationRequest<'adminPassportLogin'> = data
  const path = scope === 'platform' ? operations.adminPlatformPassportLogin.path : operations.adminPassportLogin.path
  return useHttp().post(path, payload)
}

function mfaLoginApi(data: OperationRequest<'adminPassportMfaLogin'>, scope: AuthScope): Promise<ResponseStruct<LoginResult>> {
  const path = scope === 'platform' ? operations.adminPlatformPassportMfaLogin.path : operations.adminPassportMfaLogin.path
  return useHttp().post(path, data)
}

function passwordChangeApi(data: OperationRequest<'adminPassportPasswordChange'>, scope: AuthScope): Promise<ResponseStruct<LoginTokenResult>> {
  const path = scope === 'platform' ? operations.adminPlatformPassportPasswordChange.path : operations.adminPassportPasswordChange.path
  return useHttp().post(path, data)
}

function logoutApi(scope: AuthScope, accessToken: string): Promise<ResponseStruct<null>> {
  const path = scope === 'platform' ? operations.adminPlatformPassportLogout.path : operations.adminPassportLogout.path
  return request.postWithCSRF<ResponseStruct<null>>(path, {
    Authorization: `Bearer ${accessToken}`,
  }).then((res) => {
    if (res.data.code === 200) {
      return res.data
    }
    throw res.data
  })
}

function captchaApi(): Promise<ResponseStruct<CaptchaResult>> {
  return useHttp().get(operations.adminPassportCaptcha.path)
}

export function isLoginTokenResult(result: LoginResult): result is LoginTokenResult {
  return 'access_token' in result && 'refresh_token' in result && 'expire_at' in result
}

export function isMFAChallengeResult(result: LoginResult): result is MFAChallengeResult {
  return 'mfa_required' in result && result.mfa_required === true && 'mfa_token' in result
}

export function isPasswordChangeChallengeResult(result: LoginResult): result is PasswordChangeChallengeResult {
  return 'password_change_required' in result && result.password_change_required === true && 'password_change_token' in result
}

const useUserStore = defineStore(
  'useUserStore',
  () => {
    const cache = useCache()
    const router = useRouter()
    const setting = useSettingStore()
    const token = ref<string | null>(cache.get('token', null))
    const authScope = ref<AuthScope>(cache.get('auth_scope', 'tenant'))
    const loginAuthScope = ref<AuthScope>(cache.get('login_auth_scope', 'tenant'))
    const locales = ref<any[]>([])
    const language = ref(cache.get('language', 'zh_CN'))
    const isLogin = computed(() => !!token.value)
    const userInfo = ref<UserInfo | null>(null)
    const menu = ref<MenuVo[]>([])
    const permissions = ref<string[]>([])
    const roles = ref<string[]>([])
    const dropdownMenuState = ref<{
      shortcuts: boolean
      systemInfo: boolean
    }>({
      shortcuts: false,
      systemInfo: false,
    })

    function getDropdownMenu() {
      return dropdownMenuState.value
    }

    function setDropdownMenuState(key: string, state: boolean) {
      if (dropdownMenuState.value[key] !== undefined) {
        dropdownMenuState.value[key] = state
      }
    }

    function getMenu() {
      return menu.value
    }

    function setMenu(list: MenuVo[]) {
      menu.value = list
    }

    function getDropdownMenuState(key: string) {
      return dropdownMenuState.value[key] !== undefined ? dropdownMenuState.value[key] : undefined
    }

    function permissionRolesPath() {
      return authScope.value === 'platform' ? operations.adminPlatformPermissionRoles.path : operations.adminPermissionRoles.path
    }

    function permissionMenusPath() {
      return authScope.value === 'platform' ? operations.adminPlatformPermissionMenus.path : operations.adminPermissionMenus.path
    }

    function permissionUpdatePath() {
      return authScope.value === 'platform' ? operations.adminPlatformPermissionUpdate.path : operations.adminPermissionUpdate.path
    }

    function setAuthScope(scope: AuthScope) {
      authScope.value = scope
      cache.set('auth_scope', scope)
    }

    function getLoginAuthScope(): AuthScope {
      return loginAuthScope.value
    }

    function setLoginAuthScope(scope: AuthScope) {
      loginAuthScope.value = scope
      cache.set('login_auth_scope', scope)
    }

    function clearRuntimeState() {
      userInfo.value = null
      menu.value = []
      permissions.value = []
      roles.value = []
    }

    async function persistLoginResult(result: LoginTokenResult, scope: AuthScope) {
      clearRuntimeState()
      useTabStore().clearTab()
      token.value = result.access_token
      setAuthScope(scope)
      setLoginAuthScope(scope)
      cache.set('token', result.access_token)
      if (result.expire_at > 0) {
        cache.set('expire', useDayjs().unix() + result.expire_at, { exp: result.expire_at })
      }
      else {
        cache.remove('expire')
      }
      cache.set('refresh_token', result.refresh_token)
      return result
    }

    async function handleChallengeResult(result: LoginResult, scope: AuthScope) {
      if (isLoginTokenResult(result)) {
        return persistLoginResult(result, scope)
      }
      return result
    }

    async function refreshRole() {
      const res = await useHttp().get(permissionRolesPath()) as ResponseStruct<RoleVo[]>
      setRoles(res.data)
    }

    async function refreshMenu() {
      const res = await useHttp().get(permissionMenusPath()) as ResponseStruct<MenuVo[]>
      setMenu(res.data)
    }

    async function login(data: LoginParams & { [key: string]: any }) {
      const scope = (data.auth_scope === 'platform' ? 'platform' : 'tenant') as AuthScope
      const res = await loginApi(data, scope)
      return handleChallengeResult(res.data, scope)
    }

    async function mfaLogin(data: MFALoginParams) {
      const scope = (data.auth_scope === 'platform' ? 'platform' : 'tenant') as AuthScope
      const payload: OperationRequest<'adminPassportMfaLogin'> = {
        mfa_token: data.mfa_token,
        mfa_code: data.mfa_code,
      }
      const res = await mfaLoginApi(payload, scope)
      return handleChallengeResult(res.data, scope)
    }

    async function changeExpiredPassword(data: PasswordChangeParams) {
      const scope = (data.auth_scope === 'platform' ? 'platform' : 'tenant') as AuthScope
      const payload: OperationRequest<'adminPassportPasswordChange'> = {
        password_change_token: data.password_change_token,
        old_password: data.old_password,
        new_password: data.new_password,
        new_password_confirmation: data.new_password_confirmation,
      }
      const res = await passwordChangeApi(payload, scope)
      return persistLoginResult(res.data, scope)
    }

    async function tenantSSOLogin(data: TenantSSOLoginPayload) {
      return new Promise((resolve, reject) => {
        ssoLogin(data).then(async (res) => {
          resolve(await persistLoginResult(res.data, 'tenant'))
        }).catch((error) => {
          reject(error)
        })
      })
    }

    async function tenantSSOCallback(data: TenantSSOCallbackPayload) {
      const res = await ssoCallback(data)
      return persistLoginResult(res.data, 'tenant')
    }
    async function requestUserInfo(): Promise<void> {
      try {
        const routeStore = useRouteStore()
        const { data } = await getInfo(authScope.value)
        setUserInfo(data)
        if (authScope.value === 'tenant') {
          await useTenantBrandingStore().load(true)
        }
        if ((setting.getSettings('app')?.loadUserSetting ?? true) && data.backend_setting) {
          const raw = data?.backend_setting
          const normalized = raw && !Array.isArray(raw) ? raw : null
          await setUserSetting(normalized)
        }
        await refreshMenu()
        await refreshRole()
        try {
          await useDictStore().load()
        }
        catch {
          console.warn('MineAdmin-UI：后端字典加载失败，已使用本地默认字典')
        }
        await routeStore.initRoutes(router, getMenu())
        const codes: string[] = recursionGetKey(getMenu(), 'name')
        const superRoles = ['SuperAdmin', 'PlatformSuperAdmin']
        getRoles().some(role => superRoles.includes(role)) && codes.unshift('*')
        setPermissions(codes)
      }
      // eslint-disable-next-line unused-imports/no-unused-vars
      catch (e) {
        await logout()
      }
    }

    async function logout(redirect = router.currentRoute.value.fullPath) {
      const scope = authScope.value
      if (token.value) {
        await logoutApi(scope, token.value).catch(() => undefined)
      }
      useTabStore().clearTab()
      clearInfo()
      await router.push({
        name: 'login',
        query: {
          ...(router.currentRoute.value.name !== 'login' && { redirect }),
          auth_scope: scope,
        },
      })
    }

    function setLanguage(langName: string) {
      if (!langName || typeof langName !== 'string' || !langName.trim()) {
        return false
      }
      language.value = langName.trim()
      cache.set('language', language.value)
      return true
    }

    function getLanguage() {
      return language.value
    }

    function getLocales(): any[] {
      return locales.value
    }

    function setLocales(localeArray: any[]): boolean {
      locales.value = localeArray
      return true
    }

    function getUserInfo(): UserInfo | null {
      return userInfo.value
    }

    function setUserInfo(data: UserInfo): boolean {
      userInfo.value = data
      return true
    }

    function getUserDepartments(): UserDepartmentInfo[] {
      return userInfo.value?.departments ?? []
    }

    function getUserPositions(): UserPositionInfo[] {
      return userInfo.value?.positions ?? []
    }

    function getUserRoleList(): UserRoleInfo[] {
      return userInfo.value?.roles ?? []
    }

    function getPermissions(): string[] {
      return permissions.value
    }

    function setPermissions(permissionArray: string[]): boolean {
      permissions.value = permissionArray
      return true
    }

    function getRoles(): string[] {
      return roles.value
    }

    function setRoles(roleArray: RoleVo[]): boolean {
      roles.value = roleArray.map(item => item.code) as string[]
      return true
    }

    async function setUserSetting(settings: any) {
      settings && setting.setSettings(settings)
      setting.initColorMode()

      await nextTick()
      useThemeColor().initThemeColor()
      const cacheLanguage = cache.get('language', '')?.trim?.() || ''
      const settingsLanguage = settings?.app?.useLocale?.trim?.() || ''
      const locale = cacheLanguage || settingsLanguage || 'zh_CN'
      const appSettings = setting.getSettings('app')
      if (appSettings) {
        appSettings.useLocale = locale
      }
      setLanguage(locale)
    }

    function saveSettingToSever() {
      const backend_setting = setting.getSettings()
      const payload: OperationRequest<'adminPermissionUpdate'> = { backend_setting }
      return useHttp().post(permissionUpdatePath(), payload).then(() => {
        cache.set('sys_settings', backend_setting)
      }).catch((error) => {
        console.log(error)
        return Promise.reject(error)
      })
    }

    async function clearCache() {
      // await useHttp().post('/mock/system/clearCache')
    }

    function clearInfo() {
      cache.remove('token')
      cache.remove('refresh_token')
      cache.remove('auth_scope')
      cache.remove('language')
      cache.remove('expire')
      token.value = null
      authScope.value = 'tenant'
      language.value = ''
      clearRuntimeState()
    }

    return {
      token,
      authScope,
      loginAuthScope,
      getLoginAuthScope,
      setLoginAuthScope,
      isLogin,
      login,
      mfaLogin,
      changeExpiredPassword,
      tenantSSOLogin,
      tenantSSOCallback,
      logout,
      captcha: captchaApi,
      getDropdownMenu,
      getDropdownMenuState,
      setDropdownMenuState,
      clearCache,
      setLanguage,
      getLanguage,
      requestUserInfo,
      getUserInfo,
      getUserDepartments,
      getUserPositions,
      getUserRoleList,
      setPermissions,
      getPermissions,
      getRoles,
      getLocales,
      setLocales,
      saveSettingToSever,
      getMenu,
    }
  },
)

export default useUserStore
