import type { MaSearchItem } from '@mineadmin/search'
import { ElOption, ElSelect } from 'element-plus'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'
import { storageDrivers, storageProviders } from './options.ts'

export default function getSearchItems(t: any): MaSearchItem[] {
  return [
    {
      label: () => t('baseStorageConfigManage.name'),
      prop: 'name',
      render: 'input',
    },
    {
      label: () => t('baseStorageConfigManage.provider'),
      prop: 'provider',
      render: () => ElSelect,
      renderProps: { clearable: true },
      renderSlots: {
        default: () => storageProviders.map(item => <ElOption label={item.label} value={item.value} />),
      },
    },
    {
      label: () => t('baseStorageConfigManage.driver'),
      prop: 'driver',
      render: () => ElSelect,
      renderProps: { clearable: true },
      renderSlots: {
        default: () => storageDrivers.map(item => <ElOption label={item.label} value={item.value} />),
      },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictSelect,
      renderProps: {
        clearable: true,
        placeholder: '',
        dictName: 'system-status',
      },
    },
  ]
}
