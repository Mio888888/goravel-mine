/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import type { AxiosInstance, AxiosRequestConfig, AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import axios, { AxiosHeaders } from 'axios'
import Message from 'vue-m-message'
import { useDebounceFn } from '@vueuse/core'
import { useNProgress } from '@vueuse/integrations/useNProgress'
import useCache from '@/hooks/useCache.ts'
import { ResultCode } from './ResultCode.ts'
import type { OperationResponse } from '@/generated/admin-api'
import { operations } from '@/generated/admin-api'

type RefreshTokenResponse = Extract<OperationResponse<'adminPassportRefresh'>, { code: 200 }>
type CSRFTokenResponse = Extract<OperationResponse<'adminPassportCsrfToken'>, { code: 200 }>

interface CSRFRequestConfig extends InternalAxiosRequestConfig {
  _authRetry?: boolean
  _csrfRetry?: boolean
  skipCsrf?: boolean
}

const { isLoading } = useNProgress()
const cache = useCache()
interface PendingRefreshRequest {
  retry: () => void
  abort: (reason: unknown) => void
}

const requestList = ref<PendingRefreshRequest[]>([])
const isRefreshToken = ref<boolean>(false)
let csrfToken = ''
let csrfTokenRequest: Promise<string> | null = null

function createHttp(baseUrl: string | null = null, config: AxiosRequestConfig = {}): AxiosInstance {
  const env = import.meta.env
  const isInternalApi = baseUrl === null
  return axios.create({
    baseURL: baseUrl ?? (env.VITE_OPEN_PROXY === 'true' ? env.VITE_PROXY_PREFIX : env.VITE_APP_API_BASEURL),
    timeout: 1000 * 5,
    responseType: 'json',
    withCredentials: isInternalApi && csrfEnabled(),
    ...config,
  })
}

const http: AxiosInstance = createHttp()

function csrfEnabled(): boolean {
  if (import.meta.env.VITE_SECURITY_CSRF === 'true') {
    return true
  }
  if (import.meta.env.VITE_SECURITY_CSRF === 'false') {
    return false
  }
  return import.meta.env.PROD
}

function csrfTokenPath(): string {
  return cache.get('auth_scope', 'tenant') === 'platform'
    ? operations.adminPlatformPassportCsrfToken.path
    : operations.adminPassportCsrfToken.path
}

function isUnsafeMethod(method?: string): boolean {
  return !['get', 'head', 'options'].includes((method ?? 'get').toLowerCase())
}

function sameRequestPath(url = ''): string {
  return url.split('?')[0] ?? ''
}

function shouldAttachCsrf(config: CSRFRequestConfig): boolean {
  return csrfEnabled()
    && !config.skipCsrf
    && isUnsafeMethod(config.method)
    && sameRequestPath(config.url) !== operations.adminPassportCsrfToken.path
    && sameRequestPath(config.url) !== operations.adminPlatformPassportCsrfToken.path
}

async function getCSRFToken(): Promise<string> {
  if (csrfToken) {
    return csrfToken
  }
  if (!csrfTokenRequest) {
    csrfTokenRequest = createHttp().get<CSRFTokenResponse>(csrfTokenPath(), {
      skipCsrf: true,
    } as CSRFRequestConfig).then((res) => {
      if (res.data.code !== ResultCode.SUCCESS) {
        throw res.data
      }
      csrfToken = res.data.data.csrf_token
      return csrfToken
    }).finally(() => {
      csrfTokenRequest = null
    })
  }
  return csrfTokenRequest
}

function isCSRFTokenError(data: unknown): boolean {
  if (!data || typeof data !== 'object') {
    return false
  }
  const response = data as { code?: number, message?: string }
  return response.code === ResultCode.FORBIDDEN && response.message?.includes('CSRF Token') === true
}

async function csrfRequestHeaders(headers: Record<string, string> = {}): Promise<AxiosHeaders> {
  const nextHeaders = AxiosHeaders.from(headers)
  if (csrfEnabled()) {
    nextHeaders.set('X-CSRF-Token', await getCSRFToken())
  }
  return nextHeaders
}

async function postWithCSRF<T>(url: string, headers: Record<string, string> = {}): Promise<AxiosResponse<T>> {
  let response = await createHttp(null, {
    headers: await csrfRequestHeaders(headers),
  }).post<T>(url)
  if (csrfEnabled() && isCSRFTokenError(response.data)) {
    csrfToken = ''
    response = await createHttp(null, {
      headers: await csrfRequestHeaders(headers),
    }).post<T>(url)
  }
  return response
}

async function attachCSRFHeader(config: CSRFRequestConfig): Promise<CSRFRequestConfig> {
  if (!shouldAttachCsrf(config)) {
    return config
  }
  const token = await getCSRFToken()
  const headers = AxiosHeaders.from(config.headers)
  headers.set('X-CSRF-Token', token)
  config.headers = headers
  return config
}

http.interceptors.request.use(

  async (config: CSRFRequestConfig) => {
    isLoading.value = true
    const userStore = useUserStore()
    /**
     * 全局拦截请求发送前提交的参数
     */
    if (userStore.isLogin && config.headers) {
      config.headers = Object.assign({
        'Authorization': `Bearer ${userStore.token}`,
        'Accept-Language': userStore.getLanguage(),
      }, config.headers)
    }

    return attachCSRFHeader(config)
  },
)

let isLogout = false

function drainRefreshQueue(action: 'retry'): void
function drainRefreshQueue(action: 'abort', reason: unknown): void
function drainRefreshQueue(action: 'retry' | 'abort', reason?: unknown) {
  const pending = requestList.value.splice(0)
  pending.forEach((item) => {
    if (action === 'retry') {
      item.retry()
      return
    }
    item.abort(reason)
  })
}

function requestHasAuthorization(config: InternalAxiosRequestConfig): boolean {
  return Boolean(AxiosHeaders.from(config.headers).get('Authorization'))
}

function requestUsesStaleAccessToken(config: InternalAxiosRequestConfig, currentToken: string | null): boolean {
  const authorization = AxiosHeaders.from(config.headers).get('Authorization')
  return Boolean(currentToken && authorization && authorization !== `Bearer ${currentToken}`)
}

function scheduleForcedLogout(message: string, reason: unknown) {
  isRefreshToken.value = false
  drainRefreshQueue('abort', reason)
  if (isLogout) {
    return
  }
  isLogout = true
  Message.error(message, { zIndex: 9999 })
  void useUserStore().logout().catch(() => undefined).finally(() => isLogout = false)
}

http.interceptors.response.use(
  async (response: AxiosResponse): Promise<any> => {
    isLoading.value = false
    const userStore = useUserStore()
    const config = response.config

    if (response.request.responseType === 'blob' || response.request.responseType === 'arraybuffer') {
      // 处理 JSON 格式的错误响应
      if (response.data instanceof Blob && response.data.type === 'application/json') {
        return new Promise((resolve, reject) => {
          const reader = new FileReader()
          reader.onload = () => {
            const result = JSON.parse(reader.result as string)
            if (result.code !== ResultCode.SUCCESS) {
              Message.error(result.message || '下载失败', { zIndex: 9999 })
              reject(result)
            }
          }
          reader.readAsText(response.data)
        })
      }

      // 正常的文件下载响应
      const disposition = response.headers['content-disposition']
      let fileName = '未命名文件'
      if (disposition) {
        const match = disposition.match(/filename[^;=\n]*=((['"]).*?\2|[^;\n]*)/)
        if (match && match[1]) {
          fileName = decodeURIComponent(match[1].replace(/['"]/g, ''))
        }
      }

      return Promise.resolve({
        data: response.data,
        fileName,
        headers: response.headers,
      })
    }

    if (response?.data?.code === ResultCode.SUCCESS) {
      return Promise.resolve(response.data)
    }
    else {
      switch (response?.data?.code) {
        case ResultCode.UNAUTHORIZED:
        {
          const logout = () => {
            scheduleForcedLogout(response?.data?.message ?? '登录已过期', response.data)
            return Promise.reject(response.data ?? null)
          }
          if (isLogout || (!userStore.isLogin && requestHasAuthorization(config))) {
            return Promise.reject(response.data ?? null)
          }
          if (!userStore.isLogin) {
            return Promise.reject(response.data ?? null)
          }
          if ((config as CSRFRequestConfig)._authRetry) {
            return logout()
          }
          if (requestUsesStaleAccessToken(config, userStore.token)) {
            ;(config as CSRFRequestConfig)._authRetry = true
            config.headers!.Authorization = `Bearer ${userStore.token}`
            return http(config)
          }
          // 检查token是否需要刷新
          if (!isRefreshToken.value) {
            isRefreshToken.value = true
            try {
              const refreshToken = cache.get('refresh_token')
              if (!refreshToken) {
                return logout()
              }
              const refreshPath = cache.get('auth_scope', 'tenant') === 'platform'
                ? operations.adminPlatformPassportRefresh.path
                : operations.adminPassportRefresh.path
              const refreshTokenResponse = await postWithCSRF<RefreshTokenResponse>(refreshPath, {
                Authorization: `Bearer ${refreshToken}`,
              })

              if (refreshTokenResponse.data.code !== 200) {
                return logout()
              }
              else {
                const { data } = refreshTokenResponse.data
                userStore.token = data.access_token
                cache.set('token', data.access_token)
                if (data.expire_at > 0) {
                  cache.set('expire', useDayjs().unix() + data.expire_at, { exp: data.expire_at })
                }
                else {
                  cache.remove('expire')
                }
                cache.set('refresh_token', data.refresh_token)

                isRefreshToken.value = false
                ;(config as CSRFRequestConfig)._authRetry = true
                config.headers!.Authorization = `Bearer ${userStore.token}`
                drainRefreshQueue('retry')
                return http(config)
              }
            }
            // eslint-disable-next-line unused-imports/no-unused-vars
            catch (e: any) {
              return logout()
            }
            finally {
              isRefreshToken.value = false
            }
          }
          else {
            return new Promise((resolve, reject) => {
              requestList.value.push({
                retry: () => {
                  ;(config as CSRFRequestConfig)._authRetry = true
                  config.headers!.Authorization = `Bearer ${cache.get('token')}`
                  resolve(http(config))
                },
                abort: reason => reject(reason ?? response.data ?? null),
              })
            })
          }
        }
        case ResultCode.DISABLED: {
          scheduleForcedLogout(response?.data?.message ?? '账号已被禁用', response.data)
          return Promise.reject(response.data ?? null)
        }
        case ResultCode.FORBIDDEN: {
          if (csrfEnabled() && response?.data?.message?.includes?.('CSRF Token') && !(config as CSRFRequestConfig)._csrfRetry) {
            csrfToken = ''
            ;(config as CSRFRequestConfig)._csrfRetry = true
            return http(config)
          }
          Message.error(response?.data?.message ?? '无权访问', { zIndex: 9999 })
          break
        }
        default:
          Message.error(response?.data?.message ?? '服务器错误', { zIndex: 9999 })
          break
      }

      return Promise.reject(response.data ? response.data : null)
    }
  },
  async (error: any) => {
    isLoading.value = false
    const serverError = useDebounceFn(async () => {
      if (error && error.response && error.response.status === 500) {
        Message.error(error.message ?? '服务器错误', { zIndex: 9999 })
      }
    }, 3000, { maxWait: 5000 })
    await serverError()
    return Promise.reject(error)
  },
)

export default {
  http,
  createHttp,
  postWithCSRF,
}
