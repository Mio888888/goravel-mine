import type { TenantBrandingConfig } from '~/base/api/tenant'
import { branding } from '~/base/api/tenant'

const fallbackName = import.meta.env.VITE_APP_TITLE
const defaultPrimary = '59 130 246'

function hexToRgbValue(hex?: string): string {
  const value = hex?.trim()
  if (!value || !/^#([\da-f]{3}|[\da-f]{6})$/i.test(value)) {
    return defaultPrimary
  }
  const normalized = value.length === 4
    ? value.replace(/^#(.)(.)(.)$/, '#$1$1$2$2$3$3')
    : value
  const num = Number.parseInt(normalized.slice(1), 16)
  return `${(num >> 16) & 255} ${(num >> 8) & 255} ${num & 255}`
}

const useTenantBrandingStore = defineStore(
  'useTenantBrandingStore',
  () => {
    const config = ref<TenantBrandingConfig | null>(null)
    const loaded = ref(false)
    const loading = ref(false)

    const appName = computed(() => config.value?.branding?.app_name || config.value?.name || fallbackName)
    const logoUrl = computed(() => config.value?.branding?.logo_url || '')
    const primaryColor = computed(() => config.value?.branding?.primary_color || '')
    const sso = computed(() => config.value?.features?.sso ?? { providers: [] })
    const providers = computed(() => (sso.value.providers ?? []).filter(item => item?.enabled !== false && item?.name))

    function applyBranding() {
      document.documentElement.style.setProperty('--ui-primary', hexToRgbValue(primaryColor.value))
      if (appName.value) {
        document.title = appName.value
      }
    }

    function applyConfig(value: TenantBrandingConfig | null) {
      config.value = value
      loaded.value = !!value
      applyBranding()
    }

    async function load(force = false) {
      if (loading.value || (loaded.value && !force)) {
        return config.value
      }
      loading.value = true
      try {
        const response = await branding()
        config.value = response.data
        loaded.value = true
        applyBranding()
      }
      catch {
        loaded.value = true
      }
      finally {
        loading.value = false
      }
      return config.value
    }

    function reset() {
      applyConfig(null)
    }

    return {
      config,
      loaded,
      loading,
      appName,
      logoUrl,
      primaryColor,
      providers,
      applyBranding,
      applyConfig,
      load,
      reset,
    }
  },
)

export default useTenantBrandingStore
