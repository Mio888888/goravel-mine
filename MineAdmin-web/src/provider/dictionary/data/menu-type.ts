import type { Dictionary } from '#/global'

export default [
  { label: '菜单', value: 'M', i18n: 'dictionary.menuType.menu', color: 'primary' },
  { label: '按钮', value: 'B', i18n: 'dictionary.menuType.button', color: 'danger' },
  { label: '外链', value: 'L', i18n: 'dictionary.menuType.link', color: 'success' },
  { label: 'iFrame', value: 'I', i18n: 'dictionary.menuType.iframe', color: 'warning' },
] as Dictionary[]
