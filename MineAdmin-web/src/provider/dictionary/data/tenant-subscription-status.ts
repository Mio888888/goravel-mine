import type { Dictionary } from '#/global'

export default [
  { label: 'Active', value: 'active', i18n: 'dictionary.tenantSubscription.active', color: 'success' },
  { label: 'Trialing', value: 'trialing', i18n: 'dictionary.tenantSubscription.trialing', color: 'primary' },
  { label: 'Past Due', value: 'past_due', i18n: 'dictionary.tenantSubscription.pastDue', color: 'warning' },
  { label: 'Canceled', value: 'canceled', i18n: 'dictionary.tenantSubscription.canceled', color: 'danger' },
  { label: 'Expired', value: 'expired', i18n: 'dictionary.tenantSubscription.expired', color: 'info' },
] as Dictionary[]
