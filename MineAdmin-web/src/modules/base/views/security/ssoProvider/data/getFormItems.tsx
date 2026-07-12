import type { MaFormItem } from '@mineadmin/form'
import type { SSOProviderVo } from '~/base/api/ssoProvider'
import DataPermissionMappingConfigurator from '../components/DataPermissionMappingConfigurator.vue'
import RoleMappingConfigurator from '../components/RoleMappingConfigurator.vue'
import { ssoProviderSceneDict, ssoProviderTypeDict, systemEnabledDict } from './options'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'

export type SSOProviderFormTab
  = | 'basic'
    | 'identity'
    | 'oauth'
    | 'saml'
    | 'roleMapping'
    | 'dataPermissionMapping'

export interface SSOProviderFormTabOption {
  label: string
  name: SSOProviderFormTab
}

export interface SSOProviderFormModel extends SSOProviderVo {
  role_mapping_json?: string
  data_permission_mapping_json?: string
}

const fullCols = { xs: 24 }
const halfCols = { md: 12, xs: 24 }
const thirdCols = { md: 8, xs: 24 }

export function getSSOProviderFormTabs(t: any): SSOProviderFormTabOption[] {
  return [
    { label: t('baseSsoProviderManage.basicInfo'), name: 'basic' },
    { label: t('baseSsoProviderManage.identityInfo'), name: 'identity' },
    { label: t('baseSsoProviderManage.oauthInfo'), name: 'oauth' },
    { label: t('baseSsoProviderManage.samlInfo'), name: 'saml' },
    { label: t('baseSsoProviderManage.roleMapping'), name: 'roleMapping' },
    { label: t('baseSsoProviderManage.dataPermissionMapping'), name: 'dataPermissionMapping' },
  ]
}

function stringifyJSON(value?: Record<string, any> | null) {
  if (!value || Object.keys(value).length === 0) {
    return ''
  }
  return JSON.stringify(value, null, 2)
}

export function hydrateFormModel(model: SSOProviderFormModel) {
  model.scene = model.scene ?? 'admin'
  model.type = model.type ?? 'oidc'
  model.enabled = model.enabled ?? true
  model.scope = model.scope ?? 'openid profile email'
  model.enable_pkce = model.enable_pkce ?? true
  model.enable_nonce = model.enable_nonce ?? true
  model.auto_create = model.auto_create ?? false
  model.display_order = model.display_order ?? 0
  model.role_mapping_json = model.role_mapping_json ?? stringifyJSON(model.role_mapping)
  model.data_permission_mapping_json = model.data_permission_mapping_json ?? stringifyJSON(model.data_permission_mapping)
}

function requiredRule(t: any, key: string) {
  return [{ required: true, message: t('form.requiredInput', { msg: t(`baseSsoProviderManage.${key}`) }) }]
}

function formItem(
  t: any,
  key: string,
  prop: string,
  render: MaFormItem['render'] = 'input',
  cols: MaFormItem['cols'] = halfCols,
  renderProps?: Record<string, any>,
  itemProps?: MaFormItem['itemProps'],
): MaFormItem {
  return {
    label: () => t(`baseSsoProviderManage.${key}`),
    prop,
    render,
    cols,
    renderProps,
    itemProps,
  }
}

function passwordProps() {
  return {
    type: 'password',
    showPassword: true,
    autocomplete: 'new-password',
  }
}

function switchItem(t: any, key: string, prop: string): MaFormItem {
  return formItem(t, key, prop, 'switch', thirdCols, {
    activeValue: true,
    inactiveValue: false,
  })
}

function basicItems(t: any): MaFormItem[] {
  return [
    formItem(t, 'name', 'name', 'input', halfCols, { placeholder: 'okta-admin' }, { rules: requiredRule(t, 'name') }),
    formItem(t, 'displayName', 'display_name', 'input', halfCols, { placeholder: 'Okta Admin' }, {
      rules: requiredRule(t, 'displayName'),
    }),
    formItem(t, 'scene', 'scene', () => MaDictSelect, halfCols, {
      clearable: false,
      filterable: true,
      allowCreate: true,
      defaultFirstOption: true,
      dictName: ssoProviderSceneDict,
    }),
    formItem(t, 'type', 'type', () => MaDictSelect, halfCols, { clearable: false, dictName: ssoProviderTypeDict }),
    { ...formItem(t, 'status', 'enabled', () => MaDictRadio, halfCols, { dictName: systemEnabledDict }), label: () => t('crud.status') },
    switchItem(t, 'autoCreate', 'auto_create'),
    formItem(t, 'displayOrder', 'display_order', 'inputNumber', halfCols, { class: 'w-full', min: 0 }),
    formItem(t, 'icon', 'icon', 'input', halfCols, { placeholder: 'logos:okta' }),
    formItem(t, 'buttonColor', 'button_color', 'colorPicker'),
    { ...formItem(t, 'remark', 'remark', 'input', fullCols, { type: 'textarea', rows: 2 }), label: () => t('crud.remark') },
  ]
}

function identityItems(t: any): MaFormItem[] {
  return [
    formItem(t, 'issuer', 'issuer'),
    formItem(t, 'audience', 'audience'),
    formItem(t, 'clientId', 'client_id'),
    formItem(t, 'clientSecret', 'client_secret', 'input', halfCols, passwordProps()),
    formItem(t, 'jwtSecret', 'jwt_secret', 'input', halfCols, passwordProps()),
    formItem(t, 'discoveryUrl', 'discovery_url', 'input', fullCols),
    formItem(t, 'jwksUri', 'jwks_uri'),
    formItem(t, 'jwksJson', 'jwks_json', 'input', fullCols, {
      type: 'textarea',
      rows: 4,
      placeholder: '{ "keys": [] }',
    }),
  ]
}

function oauthItems(t: any): MaFormItem[] {
  return [
    formItem(t, 'authorizationEndpoint', 'authorization_endpoint'),
    formItem(t, 'tokenEndpoint', 'token_endpoint'),
    formItem(t, 'userinfoEndpoint', 'userinfo_endpoint'),
    formItem(t, 'scope', 'scope'),
    formItem(t, 'redirectUri', 'redirect_uri'),
    switchItem(t, 'enablePkce', 'enable_pkce'),
    switchItem(t, 'enableNonce', 'enable_nonce'),
  ]
}

function samlItems(t: any): MaFormItem[] {
  return [
    formItem(t, 'samlEntrypoint', 'saml_entrypoint'),
    formItem(t, 'samlEntityId', 'saml_entity_id'),
    formItem(t, 'samlCertificate', 'saml_certificate', 'input', fullCols, { type: 'textarea', rows: 3 }),
  ]
}

function roleMappingItems(t: any): MaFormItem[] {
  return [
    formItem(t, 'roleMapping', 'role_mapping_json', () => RoleMappingConfigurator, fullCols),
  ]
}

function dataPermissionMappingItems(t: any): MaFormItem[] {
  return [
    formItem(t, 'dataPermissionMapping', 'data_permission_mapping_json', () => DataPermissionMappingConfigurator, fullCols),
  ]
}

export default function getFormItems(
  t: any,
  model: SSOProviderFormModel,
  activeTab: SSOProviderFormTab = 'basic',
): MaFormItem[] {
  hydrateFormModel(model)

  const groups: Record<SSOProviderFormTab, MaFormItem[]> = {
    basic: basicItems(t),
    identity: identityItems(t),
    oauth: oauthItems(t),
    saml: samlItems(t),
    roleMapping: roleMappingItems(t),
    dataPermissionMapping: dataPermissionMappingItems(t),
  }

  return groups[activeTab]
}
