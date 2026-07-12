/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import '@/layouts/style/logo.scss'
import type { SystemSettings } from '#/global'
import LogoSvg from '@/assets/images/logo.svg'

export default defineComponent({
  name: 'Logo',
  props: {
    showLogo: { type: Boolean, default: true },
    showTitle: { type: Boolean, default: true },
    title: { type: String, default: null },
  },
  setup(props) {
    const brandingStore = useTenantBrandingStore()
    const title = computed(() => props.title ?? brandingStore.appName)
    const logoSrc = computed(() => brandingStore.logoUrl || LogoSvg)
    const settings: SystemSettings.welcomePage = useSettingStore().getSettings('welcomePage')
    return () => {
      return (
        <router-link to={settings.path} class={['mine-main-logo', 'cursor-pointer']} title={title.value}>
          {props.showLogo && (
            <img src={logoSrc.value} alt={title.value} class="mine-logo-img" />
          )}
          {props.showTitle && (
            <span class="mine-logo-title">{title.value}</span>
          )}
        </router-link>
      )
    }
  },
})
