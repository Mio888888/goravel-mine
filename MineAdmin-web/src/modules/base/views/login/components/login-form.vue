<!--
 - MineAdmin is committed to providing solutions for quickly building web applications
 - Please view the LICENSE file that was distributed with this source code,
 - For the full copyright and license information.
 - Thank you very much for using MineAdmin.
 -
 - @Author X.Mo<root@imoi.cn>
 - @Link   https://github.com/mineadmin
-->
<script setup lang="ts">
import Message from 'vue-m-message'
import { useI18n } from 'vue-i18n'
import type { TenantSSOProvider } from '~/base/api/tenant'
import { loginEntry, ssoAuthorize } from '~/base/api/tenant'
import { clearOuterSSOCallbackQuery, resolveSSOCallbackParameters } from '~/base/utils/ssoCallback'
import useUserStore, {
  isLoginTokenResult,
  isMFAChallengeResult,
  isPasswordChangeChallengeResult,

} from '@/store/modules/useUserStore.ts'
import type { LoginResult } from '@/store/modules/useUserStore.ts'
import useSettingStore from '@/store/modules/useSettingStore.ts'

const { t } = useI18n()
const isProduction: boolean = import.meta.env.MODE === 'production'
const userStore = useUserStore()
const settingStore = useSettingStore()
const brandingStore = useTenantBrandingStore()
const router = useRouter()
const route = useRoute()
const isFormSubmit = ref(false)
const entryLoading = ref(true)
const entryAvailable = ref(true)
const entryMessage = ref('')
const entryFallbackMessage = computed(() => entryMessage.value || t('loginForm.tenantUnavailable'))
const ssoSubmit = ref(false)
const ssoDialogVisible = ref(false)
const selectedProvider = ref('')
const selectedScene = ref('admin')
const ssoForm = reactive({
  id_token: '',
  nonce: '',
  saml_response: '',
})
const isValidState = ref(true)
const captchaBase64 = ref('')
const captchaLoading = ref(false)
type AuthScope = 'tenant' | 'platform'
const form = reactive<{
  username: string
  password: string
  code: string
  captcha_key: string
  auth_scope: AuthScope
}>({
  username: isProduction ? '' : 'admin',
  password: isProduction ? '' : '123456',
  code: '',
  captcha_key: '',
  auth_scope: resolveInitialAuthScope(),
})
const challengeScope = ref<AuthScope>(form.auth_scope)
const mfaDialogVisible = ref(false)
const passwordChangeDialogVisible = ref(false)
const challengeSubmit = ref(false)
const mfaForm = reactive({
  mfa_token: '',
  mfa_code: '',
})
const passwordChangeForm = reactive({
  password_change_token: '',
  old_password: '',
  new_password: '',
  new_password_confirmation: '',
})
const ssoProviders = computed<TenantSSOProvider[]>(
  () => form.auth_scope === 'tenant' && entryAvailable.value ? brandingStore.providers : [],
)
const canSubmit = computed(() => !entryLoading.value && entryAvailable.value && !isFormSubmit.value)

function normalizeAuthScope(scope: unknown): AuthScope | undefined {
  return scope === 'platform' || scope === 'tenant' ? scope : undefined
}

function inferAuthScopeFromRoute(): AuthScope | undefined {
  const scope = route.query.auth_scope ?? route.query.scope
  const normalizedScope = normalizeAuthScope(Array.isArray(scope) ? scope[0] : scope)
  if (normalizedScope) {
    return normalizedScope
  }
  const redirect = Array.isArray(route.query.redirect) ? route.query.redirect[0] : route.query.redirect
  if (typeof redirect === 'string') {
    return redirect.startsWith('/platform') ? 'platform' : 'tenant'
  }
  return undefined
}

function resolveInitialAuthScope(): AuthScope {
  return inferAuthScopeFromRoute() ?? normalizeAuthScope(userStore.getLoginAuthScope()) ?? 'tenant'
}

watch(
  () => [route.query.auth_scope, route.query.scope, route.query.redirect],
  () => {
    if (!entryLoading.value) {
      return
    }
    const scope = inferAuthScopeFromRoute()
    if (scope) {
      form.auth_scope = scope
    }
  },
  { immediate: true },
)

watch(
  () => form.auth_scope,
  (scope) => {
    userStore.setLoginAuthScope(scope)
  },
  { immediate: true },
)

async function loadLoginEntry() {
  entryLoading.value = true
  entryMessage.value = ''
  try {
    const { data } = await loginEntry()
    form.auth_scope = data.mode === 'platform' ? 'platform' : 'tenant'
    entryAvailable.value = data.available
    entryMessage.value = data.message || ''

    if (data.mode === 'tenant') {
      if (data.config) {
        brandingStore.applyConfig(data.config)
      }
      else if (data.available) {
        await brandingStore.load(true)
      }
      return
    }
    brandingStore.reset()
  }
  catch {
    entryAvailable.value = false
    entryMessage.value = t('loginForm.entryLoadFailed')
    brandingStore.reset()
  }
  finally {
    entryLoading.value = false
  }
}

function resetSSOForm() {
  ssoForm.id_token = ''
  ssoForm.nonce = ''
  ssoForm.saml_response = ''
}

function providerButtonStyle(provider: TenantSSOProvider) {
  return provider.button_color ? { '--sso-provider-color': provider.button_color } : {}
}

function easyValidate(event: Event) {
  const dom = event?.target as HTMLInputElement
  if (form[dom.name] === undefined || form[dom.name] === '') {
    dom.classList.add('!ring-red-5')
    Message.error(t(`loginForm.${dom.name}Placeholder`))
    isValidState.value = false
  }
  else {
    dom.classList.remove('!ring-red-5')
    isValidState.value = true
  }
}

async function redirectAfterLogin() {
  const welcomePath = settingStore.getSettings('welcomePage').path ?? null
  const redirect = router.currentRoute.value.query?.redirect ?? undefined
  await router.push({ path: redirect ?? welcomePath ?? '/' })
}

async function handleLoginResult(result: LoginResult, scope: AuthScope = form.auth_scope) {
  if (isLoginTokenResult(result)) {
    await redirectAfterLogin()
    return
  }
  challengeScope.value = scope
  if (isMFAChallengeResult(result)) {
    mfaForm.mfa_token = result.mfa_token
    mfaForm.mfa_code = ''
    mfaDialogVisible.value = true
    return
  }
  if (isPasswordChangeChallengeResult(result)) {
    passwordChangeForm.password_change_token = result.password_change_token
    passwordChangeForm.old_password = form.password
    passwordChangeForm.new_password = ''
    passwordChangeForm.new_password_confirmation = ''
    passwordChangeDialogVisible.value = true
  }
}

async function submit() {
  if (!canSubmit.value) {
    if (!entryAvailable.value) {
      Message.error(entryFallbackMessage.value)
    }
    return false
  }
  isValidState.value = true
  const requiredKeys = ['username', 'password', 'code'] as const
  requiredKeys.forEach((key) => {
    if (form[key] === undefined || form[key] === '') {
      Message.error(t(`loginForm.${key}Placeholder`))
      isValidState.value = false
    }
  })
  if (!isValidState.value) {
    return false
  }

  if (!form.captcha_key) {
    await refreshCaptcha()
  }
  if (!form.captcha_key) {
    return false
  }

  try {
    isFormSubmit.value = true
    await handleLoginResult(await userStore.login(form) as LoginResult, form.auth_scope)
  }
  catch {
    form.code = ''
    await refreshCaptcha()
  }
  finally {
    isFormSubmit.value = false
  }
}

async function submitMFA() {
  if (!mfaForm.mfa_code) {
    Message.error(t('loginForm.mfaCodePlaceholder'))
    return
  }
  try {
    challengeSubmit.value = true
    const result = await userStore.mfaLogin({
      ...mfaForm,
      auth_scope: challengeScope.value,
      username: form.username,
    })
    mfaDialogVisible.value = false
    await handleLoginResult(result as LoginResult, challengeScope.value)
  }
  catch {
    mfaForm.mfa_code = ''
  }
  finally {
    challengeSubmit.value = false
  }
}

async function submitPasswordChange() {
  if (!passwordChangeForm.old_password || !passwordChangeForm.new_password || !passwordChangeForm.new_password_confirmation) {
    Message.error(t('loginForm.passwordChangeRequired'))
    return
  }
  if (passwordChangeForm.new_password !== passwordChangeForm.new_password_confirmation) {
    Message.error(t('loginForm.passwordChangeMismatch'))
    return
  }
  try {
    challengeSubmit.value = true
    const result = await userStore.changeExpiredPassword({
      ...passwordChangeForm,
      auth_scope: challengeScope.value,
      username: form.username,
    })
    passwordChangeDialogVisible.value = false
    await handleLoginResult(result as LoginResult, challengeScope.value)
  }
  finally {
    challengeSubmit.value = false
  }
}

async function refreshCaptcha() {
  captchaLoading.value = true
  try {
    const { data } = await userStore.captcha()
    form.captcha_key = data.key
    captchaBase64.value = data.base64
  }
  catch {
    form.captcha_key = ''
    captchaBase64.value = ''
  }
  finally {
    captchaLoading.value = false
  }
}

async function openSSO(provider: TenantSSOProvider) {
  if (!entryAvailable.value) {
    Message.error(entryFallbackMessage.value)
    return
  }
  if (provider.type === 'oidc' || provider.type === 'oauth2') {
    const { data } = await ssoAuthorize({
      provider: provider.name,
      scene: provider.scene || 'admin',
    })
    sessionStorage.setItem('tenant_sso_state', JSON.stringify({
      transaction_id: data.transaction_id,
      state: data.state,
    }))
    window.location.href = data.authorization_url
    return
  }
  selectedProvider.value = provider.name
  selectedScene.value = provider.scene || 'admin'
  resetSSOForm()
  ssoDialogVisible.value = true
}

async function submitSSO() {
  if (!entryAvailable.value) {
    Message.error(entryFallbackMessage.value)
    return false
  }
  const provider = selectedProvider.value
  const scene = selectedScene.value
  if (!provider) {
    return false
  }
  if (!ssoForm.id_token && !ssoForm.saml_response) {
    Message.error(t('loginForm.ssoTokenPlaceholder'))
    return false
  }
  ssoSubmit.value = true
  userStore.tenantSSOLogin({
    provider,
    scene,
    id_token: ssoForm.id_token,
    nonce: ssoForm.nonce,
    saml_response: ssoForm.saml_response,
  }).then(async (userData: any) => {
    const welcomePath = settingStore.getSettings('welcomePage').path ?? null
    const redirect = router.currentRoute.value.query?.redirect ?? undefined
    if (userData) {
      await router.push({ path: redirect ?? welcomePath ?? '/' })
    }
    ssoDialogVisible.value = false
    ssoSubmit.value = false
  }).catch(() => ssoSubmit.value = false)
}

async function submitSSOCallback() {
  if (form.auth_scope !== 'tenant' || !entryAvailable.value) {
    return
  }
  const { code, state } = resolveSSOCallbackParameters(route.query)
  if (!code || !state) {
    return
  }
  const raw = sessionStorage.getItem('tenant_sso_state')
  if (!raw) {
    return
  }
  let cached: {
    state?: string
    transaction_id?: string
  }
  try {
    cached = JSON.parse(raw)
  }
  catch {
    sessionStorage.removeItem('tenant_sso_state')
    return
  }
  if (cached.state !== state) {
    sessionStorage.removeItem('tenant_sso_state')
    clearOuterSSOCallbackQuery()
    return
  }
  if (!cached.transaction_id) {
    return
  }
  ssoSubmit.value = true
  try {
    await userStore.tenantSSOCallback({
      transaction_id: cached.transaction_id,
      code,
      state,
    })
    sessionStorage.removeItem('tenant_sso_state')
    clearOuterSSOCallbackQuery()
    await redirectAfterLogin()
  }
  finally {
    ssoSubmit.value = false
  }
}

onMounted(async () => {
  await loadLoginEntry()
  await refreshCaptcha()
  await submitSSOCallback()
})
</script>

<template>
  <form class="mine-login-form" @submit.prevent="submit">
    <el-alert
      v-if="entryMessage"
      class="login-entry-alert"
      type="warning"
      :closable="false"
      show-icon
      :title="entryMessage"
    />
    <div v-if="entryLoading" class="mine-login-form-item">
      <el-skeleton animated>
        <template #template>
          <el-skeleton-item variant="text" class="!h-9 !w-full" />
        </template>
      </el-skeleton>
    </div>
    <div class="mine-login-form-item">
      <div class="mine-login-form-item-title">
        {{ t('loginForm.usernameLabel') }}
      </div>
      <m-input
        v-model="form.username"
        class="!bg-white !text-black !ring-gray-2 !focus-ring-[rgb(var(--ui-primary))] !placeholder-stone-4"
        name="username"
        :placeholder="t('loginForm.usernamePlaceholder')"
        @blur="easyValidate"
      />
    </div>
    <div class="mine-login-form-item">
      <div class="mine-login-form-item-title">
        {{ t('loginForm.passwordLabel') }}
      </div>
      <m-input
        v-model="form.password"
        class="!bg-white !text-black !ring-gray-2 !focus-ring-[rgb(var(--ui-primary))] !placeholder-stone-4"
        name="password"
        type="password"
        :placeholder="t('loginForm.passwordPlaceholder')"
        @blur="easyValidate"
      />
    </div>
    <div class="mine-login-form-item">
      <div class="mine-login-form-item-title">
        {{ t('loginForm.codeLabel') }}
      </div>
      <m-input
        v-model="form.code"
        class="!bg-white !text-black !ring-gray-2 !focus-ring-[rgb(var(--ui-primary))] !placeholder-stone-4"
        name="code"
        :placeholder="t('loginForm.codePlaceholder')"
        @blur="easyValidate"
      >
        <template #suffix>
          <div class="ml-0.5 w-34 flex items-center justify-center text-sm">
            <button
              type="button"
              class="h-9 w-32 overflow-hidden rounded bg-gray-1 text-gray-5 ring-1 ring-gray-2"
              :aria-label="t('loginForm.codeLabel')"
              @click="refreshCaptcha"
            >
              <img
                v-if="captchaBase64"
                class="h-full w-full object-cover"
                :class="{ 'opacity-55': captchaLoading }"
                :src="captchaBase64"
                alt=""
              >
            </button>
          </div>
        </template>
      </m-input>
    </div>
    <div class="mine-login-form-item mt-2">
      <m-button
        type="submit"
        class="!bg-[rgb(var(--ui-primary))] !text-gray-1 !active-bg-[rgb(var(--ui-primary))] !hover-bg-[rgb(var(--ui-primary)/.75)]"
        :class="{
          'loading': isFormSubmit || entryLoading || !entryAvailable,
          'is-disabled': !canSubmit,
        }"
        :aria-disabled="!canSubmit"
      >
        <ma-svg-icon name="formkit:submit" /> {{ t('loginForm.loginButton') }}
      </m-button>
    </div>
    <div v-if="ssoProviders.length > 0" class="mine-login-form-item mt-2">
      <div class="mine-login-form-item-title">
        {{ t('loginForm.ssoLogin') }}
      </div>
      <div class="grid grid-cols-1 gap-2">
        <m-button
          v-for="provider in ssoProviders"
          :key="provider.name"
          type="button"
          class="!bg-white !text-gray-8 !ring-gray-2 !hover-bg-gray-1"
          :class="{ 'sso-provider-button': provider.button_color }"
          :style="providerButtonStyle(provider)"
          @click="openSSO(provider)"
        >
          <ma-svg-icon :name="provider.icon || 'material-symbols:login-rounded'" />
          {{ provider.display_name || provider.name }}
        </m-button>
      </div>
    </div>
  </form>

  <el-dialog v-model="ssoDialogVisible" :title="t('loginForm.ssoLogin')" width="420px" append-to-body destroy-on-close>
    <el-form label-position="top">
      <el-form-item :label="t('loginForm.ssoProvider')">
        <el-input v-model="selectedProvider" disabled />
      </el-form-item>
      <el-form-item :label="t('loginForm.ssoToken')">
        <el-input
          v-model="ssoForm.id_token"
          type="textarea"
          :rows="4"
          :placeholder="t('loginForm.ssoTokenPlaceholder')"
        />
      </el-form-item>
      <el-form-item :label="t('loginForm.ssoNonce')">
        <el-input v-model="ssoForm.nonce" :placeholder="t('loginForm.ssoNoncePlaceholder')" />
      </el-form-item>
      <el-form-item :label="t('loginForm.ssoSAMLResponse')">
        <el-input
          v-model="ssoForm.saml_response"
          type="textarea"
          :rows="4"
          :placeholder="t('loginForm.ssoSAMLResponsePlaceholder')"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="ssoDialogVisible = false">
        {{ t('crud.cancel') }}
      </el-button>
      <el-button type="primary" :loading="ssoSubmit" @click="() => submitSSO()">
        {{ t('loginForm.loginButton') }}
      </el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="mfaDialogVisible" :title="t('loginForm.mfaTitle')" width="420px" append-to-body destroy-on-close>
    <el-form label-position="top" @submit.prevent="submitMFA">
      <el-form-item :label="t('loginForm.mfaCode')">
        <el-input
          v-model="mfaForm.mfa_code"
          autocomplete="one-time-code"
          :placeholder="t('loginForm.mfaCodePlaceholder')"
          @keyup.enter="submitMFA"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="mfaDialogVisible = false">
        {{ t('crud.cancel') }}
      </el-button>
      <el-button type="primary" :loading="challengeSubmit" @click="submitMFA">
        {{ t('crud.ok') }}
      </el-button>
    </template>
  </el-dialog>

  <el-dialog
    v-model="passwordChangeDialogVisible"
    :title="t('loginForm.passwordChangeTitle')"
    width="420px"
    append-to-body
    destroy-on-close
  >
    <el-form label-position="top" @submit.prevent="submitPasswordChange">
      <el-form-item :label="t('loginForm.oldPassword')">
        <el-input
          v-model="passwordChangeForm.old_password"
          type="password"
          show-password
          :placeholder="t('loginForm.oldPasswordPlaceholder')"
        />
      </el-form-item>
      <el-form-item :label="t('loginForm.newPassword')">
        <el-input
          v-model="passwordChangeForm.new_password"
          type="password"
          show-password
          :placeholder="t('loginForm.newPasswordPlaceholder')"
        />
      </el-form-item>
      <el-form-item :label="t('loginForm.confirmPassword')">
        <el-input
          v-model="passwordChangeForm.new_password_confirmation"
          type="password"
          show-password
          :placeholder="t('loginForm.confirmPasswordPlaceholder')"
          @keyup.enter="submitPasswordChange"
        />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="passwordChangeDialogVisible = false">
        {{ t('crud.cancel') }}
      </el-button>
      <el-button type="primary" :loading="challengeSubmit" @click="submitPasswordChange">
        {{ t('crud.ok') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.loading {
  @apply cursor-wait;

  background-color: rgb(var(--ui-primary) / 45%) !important;
}

.is-disabled {
  @apply cursor-not-allowed opacity-65;
}

.login-entry-alert {
  padding: 8px !important;
}

.login-entry-alert :deep(.el-alert__title) {
  @apply leading-5;
}

.sso-provider-button {
  color: var(--sso-provider-color) !important;
  border-color: var(--sso-provider-color) !important;
}
</style>
