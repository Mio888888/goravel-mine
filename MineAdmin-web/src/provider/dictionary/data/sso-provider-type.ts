import type { Dictionary } from '#/global'

export default [
  { label: 'OIDC', value: 'oidc', i18n: 'dictionary.ssoProvider.typeOidc', color: 'primary' },
  { label: 'OAuth2', value: 'oauth2', i18n: 'dictionary.ssoProvider.typeOauth2', color: 'success' },
  { label: 'SAML', value: 'saml', i18n: 'dictionary.ssoProvider.typeSaml', color: 'warning' },
] as Dictionary[]
