import type { MaSearchItem } from '@mineadmin/search'

export default function getSearchItems(t: any): MaSearchItem[] {
  return [
    { label: () => t('baseSsoUserBinding.username'), prop: 'username', render: 'input' },
    { label: () => t('baseSsoUserBinding.providerName'), prop: 'provider_name', render: 'input' },
    { label: () => t('baseSsoUserBinding.ssoUserId'), prop: 'sso_user_id', render: 'input' },
    { label: () => t('baseSsoUserBinding.ssoEmail'), prop: 'sso_email', render: 'input' },
    { label: () => t('baseSsoUserBinding.ssoUsername'), prop: 'sso_username', render: 'input' },
  ]
}
