import type { Dictionary } from '#/global'

export default [
  { label: '正常', value: 1, i18n: 'dictionary.tenant.statusActive', color: 'success' },
  { label: '挂起', value: 2, i18n: 'dictionary.tenant.statusSuspended', color: 'warning' },
  { label: '归档', value: 3, i18n: 'dictionary.tenant.statusArchived', color: 'info' },
] as Dictionary[]
