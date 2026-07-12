<script setup lang="tsx">
import type { MaFormExpose, MaFormItem } from '@mineadmin/form'
import type { StorageConfigVo, StorageProvider } from '~/base/api/platformStorageConfig'
import { ElOption, ElSelect } from 'element-plus'
import { create, save } from '~/base/api/platformStorageConfig'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import { storageProviders } from './data/options.ts'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'
import { secretResource } from '~/base/utils/sensitiveOperation'

defineOptions({ name: 'platform:storageConfig:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: StorageConfigVo | null
}>()

const t = useTrans().globalTrans
const configForm = ref<MaFormExpose>()
const configModel = ref<StorageConfigVo>({
  provider: 'minio',
  driver: 's3_compatible',
  path_prefix: 'uploads',
  is_default: false,
  status: 1,
  options: {},
  options_json: '{}',
})
const requestEvidence = inject<SensitiveEvidenceRequester>('requestSensitiveEvidence')!

useForm('configForm').then((form: MaFormExpose) => {
  if (formType === 'edit' && data) {
    configModel.value = {
      ...data,
      driver: driverForProvider(data.provider),
      secret_key: '',
      options_json: JSON.stringify(data.options ?? {}, null, 2),
    }
  }
  syncDriver()
  form.setItems(formItems())
  form.setOptions({ labelWidth: '120px' })
})

function isLocalProvider(provider = configModel.value.provider): boolean {
  return provider === 'local'
}

function driverForProvider(provider = configModel.value.provider) {
  return isLocalProvider(provider) ? 'local' : 's3_compatible'
}

function syncDriver() {
  configModel.value.driver = driverForProvider()
}

function clearObjectStorageFields() {
  configModel.value.bucket = ''
  configModel.value.endpoint = ''
  configModel.value.region = ''
  configModel.value.access_key = ''
  configModel.value.secret_key = ''
  configModel.value.options = {}
  configModel.value.options_json = '{}'
}

function handleProviderChange(provider: StorageProvider) {
  configModel.value.provider = provider
  configModel.value.driver = driverForProvider(provider)
  if (isLocalProvider(provider)) {
    clearObjectStorageFields()
  }
}

function isObjectStorageRequired(): boolean {
  return !isLocalProvider()
}

function requiredWhenObjectStorage(message: string) {
  return (_rule: any, value: string) => {
    if (!isObjectStorageRequired()) {
      return true
    }
    return String(value ?? '').trim() !== '' || new Error(message)
  }
}

function requiredSecretKey(message: string) {
  return (_rule: any, value: string) => {
    if (!isObjectStorageRequired() || (formType === 'edit' && data?.provider !== 'local')) {
      return true
    }
    return String(value ?? '').trim() !== '' || new Error(message)
  }
}

function formItems(): MaFormItem[] {
  return [
    {
      label: () => t('baseStorageConfigManage.name'),
      prop: 'name',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: { placeholder: t('form.pleaseInput', { msg: t('baseStorageConfigManage.name') }) },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('baseStorageConfigManage.name') }) }],
      },
    },
    {
      label: () => t('baseStorageConfigManage.provider'),
      prop: 'provider',
      render: () => ElSelect,
      cols: { md: 12, xs: 24 },
      renderProps: {
        onChange: handleProviderChange,
      },
      renderSlots: {
        default: () => storageProviders.map(item => <ElOption label={item.label} value={item.value} />),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredSelect', { msg: t('baseStorageConfigManage.provider') }) }],
      },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictRadio,
      cols: { md: 12, xs: 24 },
      renderProps: { dictName: 'system-status' },
    },
    {
      label: () => t('baseStorageConfigManage.default'),
      prop: 'is_default',
      render: 'switch',
      cols: { md: 12, xs: 24 },
      renderProps: { activeValue: true, inactiveValue: false },
    },
    {
      label: () => t('baseStorageConfigManage.pathPrefix'),
      prop: 'path_prefix',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: { placeholder: 'uploads' },
    },
    {
      label: () => t('baseStorageConfigManage.bucket'),
      prop: 'bucket',
      render: 'input',
      show: (_, model) => model.provider !== 'local',
      cols: { md: 12, xs: 24 },
      itemProps: {
        rules: [{
          validator: requiredWhenObjectStorage(t('form.requiredInput', { msg: t('baseStorageConfigManage.bucket') })),
        }],
      },
    },
    {
      label: () => t('baseStorageConfigManage.region'),
      prop: 'region',
      render: 'input',
      show: (_, model) => model.provider !== 'local',
      cols: { md: 12, xs: 24 },
    },
    {
      label: () => t('baseStorageConfigManage.endpoint'),
      prop: 'endpoint',
      render: 'input',
      show: (_, model) => model.provider !== 'local',
      cols: { xs: 24 },
      itemProps: {
        rules: [{
          validator: requiredWhenObjectStorage(t('form.requiredInput', { msg: t('baseStorageConfigManage.endpoint') })),
        }],
      },
    },
    {
      label: () => t('baseStorageConfigManage.baseUrl'),
      prop: 'base_url',
      render: 'input',
      cols: { xs: 24 },
    },
    {
      label: () => t('baseStorageConfigManage.accessKey'),
      prop: 'access_key',
      render: 'input',
      show: (_, model) => model.provider !== 'local',
      cols: { md: 12, xs: 24 },
      itemProps: {
        rules: [{
          validator: requiredWhenObjectStorage(t('form.requiredInput', { msg: t('baseStorageConfigManage.accessKey') })),
        }],
      },
    },
    {
      label: () => t('baseStorageConfigManage.secretKey'),
      prop: 'secret_key',
      render: 'input',
      show: (_, model) => model.provider !== 'local',
      cols: { md: 12, xs: 24 },
      renderProps: {
        type: 'password',
        showPassword: true,
        placeholder: formType === 'edit' ? t('baseStorageConfigManage.secretKeepTip') : '',
      },
      itemProps: {
        rules: [{
          validator: requiredSecretKey(t('form.requiredInput', { msg: t('baseStorageConfigManage.secretKey') })),
        }],
      },
    },
    {
      label: () => t('baseStorageConfigManage.options'),
      prop: 'options_json',
      render: 'input',
      show: (_, model) => model.provider !== 'local',
      cols: { xs: 24 },
      renderProps: {
        type: 'textarea',
        rows: 5,
        placeholder: '{\n  "force_path_style": true\n}',
      },
    },
    {
      label: () => t('crud.remark'),
      prop: 'remark',
      render: 'input',
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 2 },
    },
  ]
}

function payload(): StorageConfigVo {
  syncDriver()
  if (isLocalProvider()) {
    clearObjectStorageFields()
  }
  let options: Record<string, any> = {}
  const raw = configModel.value.options_json?.trim()
  if (raw && !isLocalProvider()) {
    options = JSON.parse(raw)
  }
  const { options_json, ...data } = configModel.value
  return { ...data, options }
}

async function submit(): Promise<any> {
  let data: StorageConfigVo
  try {
    data = payload()
  }
  catch {
    throw new Error(t('baseStorageConfigManage.optionsJsonError'))
  }
  const id = data.id as number
  const action = formType === 'add' ? 'create' : 'update'
  const evidence = changesProtectedConfiguration(data)
    ? await requestEvidence({ policy_key: 'storage.secret.change', scope: 'storage.secret.change', resource: secretResource('storage-config', action, action === 'update' ? [id] : undefined), reason: `${action} storage trust configuration` })
    : undefined
  const response = formType === 'add' ? await create(data, evidence) : await save(id, data, evidence)
  if (response.code !== ResultCode.SUCCESS) {
    throw response
  }
  return response
}

function changesProtectedConfiguration(next: StorageConfigVo) {
  const protectedFields: (keyof StorageConfigVo)[] = [
    'provider',
    'driver',
    'bucket',
    'endpoint',
    'access_key',
    'is_default',
  ]
  if (next.secret_key?.trim()) {
    return true
  }
  return protectedFields.some(field => normalized(next[field]) !== normalized(data?.[field]))
}

function normalized(value: unknown) {
  return String(value ?? '').trim()
}

defineExpose({
  add: submit,
  edit: submit,
  maForm: configForm,
})
</script>

<template>
  <ma-form ref="configForm" v-model="configModel" />
</template>
