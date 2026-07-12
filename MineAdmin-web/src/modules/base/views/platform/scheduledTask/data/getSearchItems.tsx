import type { MaSearchItem } from '@mineadmin/search'
import { ElOption, ElSelect } from 'element-plus'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'
import { taskTypes } from './options.ts'

export default function getSearchItems(t: any): MaSearchItem[] {
  return [
    { label: () => t('baseScheduledTaskManage.name'), prop: 'name', render: 'input' },
    { label: () => t('baseScheduledTaskManage.code'), prop: 'code', render: 'input' },
    {
      label: () => t('baseScheduledTaskManage.taskType'),
      prop: 'task_type',
      render: () => ElSelect,
      renderProps: { clearable: true },
      renderSlots: {
        default: () => taskTypes.map(item => <ElOption label={item.label} value={item.value} />),
      },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictSelect,
      renderProps: { clearable: true, placeholder: '', dictName: 'system-status' },
    },
  ]
}
