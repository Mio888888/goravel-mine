import type { MaSearchItem } from '@mineadmin/search'
import { ssoProviderSceneDict, ssoProviderTypeDict, systemEnabledSearchDict } from './options'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'

export default function getSearchItems(t: any): MaSearchItem[] {
  return [
    {
      label: () => t('baseSsoProviderManage.name'),
      prop: 'name',
      render: 'input',
    },
    {
      label: () => t('baseSsoProviderManage.displayName'),
      prop: 'display_name',
      render: 'input',
    },
    {
      label: () => t('baseSsoProviderManage.scene'),
      prop: 'scene',
      render: () => MaDictSelect,
      renderProps: {
        clearable: true,
        filterable: true,
        allowCreate: true,
        defaultFirstOption: true,
        dictName: ssoProviderSceneDict,
      },
    },
    {
      label: () => t('baseSsoProviderManage.type'),
      prop: 'type',
      render: () => MaDictSelect,
      renderProps: {
        clearable: true,
        dictName: ssoProviderTypeDict,
      },
    },
    {
      label: () => t('crud.status'),
      prop: 'enabled',
      render: () => MaDictSelect,
      renderProps: {
        clearable: true,
        dictName: systemEnabledSearchDict,
      },
    },
  ]
}
