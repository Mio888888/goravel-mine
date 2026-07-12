import type { MaSearchItem } from '@mineadmin/search'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'

export default function getSearchItems(t: any): MaSearchItem[] {
  return [
    { label: () => t('baseSsoLoginLog.username'), prop: 'username', render: 'input' },
    { label: () => t('baseSsoLoginLog.providerName'), prop: 'provider_name', render: 'input' },
    { label: () => t('baseSsoLoginLog.ssoUserId'), prop: 'sso_user_id', render: 'input' },
    { label: () => t('baseSsoLoginLog.ssoEmail'), prop: 'sso_email', render: 'input' },
    {
      label: () => t('baseSsoLoginLog.status'),
      prop: 'status',
      render: () => MaDictSelect,
      renderProps: {
        clearable: true,
        dictName: 'system-state',
      },
    },
    { label: () => t('baseSsoLoginLog.startDate'), prop: 'start_date', render: 'date-picker' },
    { label: () => t('baseSsoLoginLog.endDate'), prop: 'end_date', render: 'date-picker' },
  ]
}
