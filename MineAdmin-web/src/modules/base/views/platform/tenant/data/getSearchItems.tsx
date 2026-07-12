import type { MaSearchItem } from '@mineadmin/search'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'

export default function getSearchItems(t: any): MaSearchItem[] {
  return [
    {
      label: () => t('baseTenantManage.code'),
      prop: 'code',
      render: 'input',
    },
    {
      label: () => t('baseTenantManage.name'),
      prop: 'name',
      render: 'input',
    },
    {
      label: () => t('baseTenantManage.plan'),
      prop: 'plan',
      render: 'input',
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictSelect,
      renderProps: {
        clearable: true,
        placeholder: '',
        dictName: 'tenant-status',
      },
    },
  ]
}
