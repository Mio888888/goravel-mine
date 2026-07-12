<script setup lang="ts">
import type { MaFormExpose } from '@mineadmin/form'
import type { SSOProviderVo } from '~/base/api/ssoProvider'
import { create, save } from '~/base/api/ssoProvider'
import getFormItems, { getSSOProviderFormTabs, hydrateFormModel } from './data/getFormItems'
import type { SSOProviderFormModel, SSOProviderFormTab } from './data/getFormItems'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'
import { secretResource } from '~/base/utils/sensitiveOperation'

defineOptions({ name: 'security:ssoProvider:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: SSOProviderVo | null
}>()

const t = useTrans().globalTrans
const providerForm = ref<MaFormExpose>()
const providerModel = ref<SSOProviderFormModel>({})
const activeTab = ref<SSOProviderFormTab>('basic')
const tabOptions = computed(() => getSSOProviderFormTabs(t))
let formExpose: MaFormExpose | null = null
const requestEvidence = inject<SensitiveEvidenceRequester>('requestSensitiveEvidence')!

function setTabItems(tab: SSOProviderFormTab) {
  formExpose?.setItems(getFormItems(t, providerModel.value, tab))
}

useForm('providerForm').then((form: MaFormExpose) => {
  if (formType === 'edit' && data) {
    Object.keys(data).map((key: string) => {
      providerModel.value[key] = data[key]
    })
  }
  hydrateFormModel(providerModel.value)
  formExpose = form
  setTabItems(activeTab.value)
  form.setOptions({
    labelWidth: '130px',
  })
})

watch(activeTab, tab => setTabItems(tab))

function parseJSONField(value?: string) {
  const text = value?.trim()
  if (!text) {
    return null
  }
  return JSON.parse(text)
}

function payload(): SSOProviderVo {
  const { role_mapping_json, data_permission_mapping_json, ...data } = providerModel.value
  return {
    ...data,
    role_mapping: parseJSONField(role_mapping_json),
    data_permission_mapping: parseJSONField(data_permission_mapping_json),
  }
}

async function add(): Promise<any> {
  const data = payload()
  const evidence = changesProtectedConfiguration(data, null)
    ? await requestEvidence({ policy_key: 'sso.provider.secret.change', scope: 'sso.provider.secret.change', resource: secretResource('sso-provider', 'create'), reason: 'Create SSO provider trust configuration' })
    : undefined
  const response = await create(data, evidence)
  if (response.code !== ResultCode.SUCCESS) {
    throw response
  }
  return response
}

async function edit(): Promise<any> {
  const id = providerModel.value.id as number
  const next = payload()
  const evidence = changesProtectedConfiguration(next, data)
    ? await requestEvidence({ policy_key: 'sso.provider.secret.change', scope: 'sso.provider.secret.change', resource: secretResource('sso-provider', 'update', [id]), reason: `Change SSO provider ${id} trust configuration` })
    : undefined
  const response = await save(id, next, evidence)
  if (response.code !== ResultCode.SUCCESS) {
    throw response
  }
  return response
}

function changesProtectedConfiguration(next: SSOProviderVo, existing: SSOProviderVo | null) {
  const protectedFields: (keyof SSOProviderVo)[] = [
    'issuer',
    'discovery_url',
    'authorization_endpoint',
    'token_endpoint',
    'userinfo_endpoint',
    'jwks_uri',
    'jwks_json',
    'redirect_uri',
    'saml_entrypoint',
    'saml_entity_id',
    'saml_certificate',
  ]
  if (next.jwt_secret?.trim() || next.client_secret?.trim()) {
    return true
  }
  return protectedFields.some(field => normalized(next[field]) !== normalized(existing?.[field]))
}

function normalized(value: unknown) {
  return String(value ?? '').trim()
}

async function validate(): Promise<void> {
  activeTab.value = 'basic'
  await nextTick()
  const elForm = providerForm.value?.getElFormRef()
  await elForm?.validate()
}

defineExpose({
  add,
  edit,
  validate,
  maForm: providerForm,
})
</script>

<template>
  <div class="sso-provider-form">
    <el-tabs v-model="activeTab" class="sso-provider-form-tabs">
      <el-tab-pane
        v-for="item in tabOptions"
        :key="item.name"
        :label="item.label"
        :name="item.name"
      />
    </el-tabs>
    <ma-form ref="providerForm" v-model="providerModel" />
  </div>
</template>

<style scoped lang="scss">
.sso-provider-form-tabs {
  margin-bottom: 16px;
}
</style>
