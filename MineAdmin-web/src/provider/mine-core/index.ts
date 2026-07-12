/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import type { ProviderService } from '#/global'
import type { MaFormItem, MaFormOptions } from '@mineadmin/form'
import type { MaSearchItem, MaSearchOptions } from '@mineadmin/search'
import type { MaTableColumns, MaTableOptions, PaginationProps } from '@mineadmin/table'
import type { App } from 'vue'
import MButton from '@/components/m-button/index.vue'
import MDrawer from '@/components/m-drawer/index.vue'
import MDropdown from '@/components/m-dropdown/index.vue'
import MDropdownDivider from '@/components/m-dropdown/divider.vue'
import MDropdownItem from '@/components/m-dropdown/item.vue'
import MInput from '@/components/m-input/index.vue'
import MModal from '@/components/m-modal/index.vue'
import MSwitch from '@/components/m-switch/index.vue'
import MTabs from '@/components/m-tabs/index.vue'
import MTextarea from '@/components/m-textarea/index.vue'
import MTooltip from '@/components/m-tooltip/index.vue'
import MaSvgIcon from '@/components/ma-svg-icon/index.vue'
import ContextMenu from '@imengyu/vue3-context-menu'
import FloatingVue from 'floating-vue'
import MaForm from '@mineadmin/form'
import MaProTable from '@mineadmin/pro-table'
import MaSearch from '@mineadmin/search'
import 'floating-vue/dist/style.css'

import MaTable from '@mineadmin/table'

// maTable样式
import '@mineadmin/table/dist/style.css'
// maSearch样式
import '@mineadmin/search/dist/style.css'
// MaProTable样式
import '@mineadmin/pro-table/dist/style.css'

interface MineCoreCommonConfig {
  table?: {
    commonOptions: MaTableOptions
    commonColumns: MaTableColumns[]
    commonPagination: PaginationProps | null
  }
  form?: {
    commonOptions: MaFormOptions
    commonItems: MaFormItem[]
  }
  search?: {
    commonOptions: MaSearchOptions
    commonItems: MaSearchItem[]
  }
}

function commonConfig(): MineCoreCommonConfig {
  return {
    table: {
      commonOptions: {},
      commonColumns: [],
      commonPagination: null,
    },
    form: {
      commonOptions: {},
      commonItems: [],
    },
    search: {
      commonOptions: {},
      commonItems: [],
    },
  }
}

const provider: ProviderService.Provider = {
  name: 'mine-core',
  setProvider(app: App) {
    app.use(FloatingVue, { distance: 12 })
    app.component('MButton', MButton)
    app.component('MDrawer', MDrawer)
    app.component('MDropdown', MDropdown)
    app.component('MDropdownDivider', MDropdownDivider)
    app.component('MDropdownItem', MDropdownItem)
    app.component('MInput', MInput)
    app.component('MModal', MModal)
    app.component('MSwitch', MSwitch)
    app.component('MTabs', MTabs)
    app.component('MTextarea', MTextarea)
    app.component('MTooltip', MTooltip)
    app.use(MaTable)
    app.use(MaForm)
    app.use(MaSearch)
    app.use(MaProTable, {
      ssr: false,
      provider: {
        app,
        icon: markRaw(MaSvgIcon),
        contextMenu: ContextMenu.showContextMenu,
      },
    })
    app.config.globalProperties.$mineCore = commonConfig()
  },
  getProvider(): any {
    return useGlobal().$mineCore
  },
}

export default provider as ProviderService.Provider
