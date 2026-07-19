<script setup lang="tsx">
import type { MaFormExpose, MaFormItem } from '@mineadmin/form'
import type {
  ScheduledTaskHandlerDefinitionVo,
  ScheduledTaskTenantOptionVo,
  ScheduledTaskVo,
} from '~/base/api/platformScheduledTask'
import { ElOption, ElSelect } from 'element-plus'
import {
  create,
  handlers as fetchHandlers,
  tenantOptions as fetchTenantOptions,
  save,
} from '~/base/api/platformScheduledTask'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import { concurrencyPolicies, misfirePolicies, taskScopes } from './data/options.ts'

defineOptions({ name: 'platform:scheduledTask:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: ScheduledTaskVo | null
}>()

interface ScheduledTaskFormModel extends ScheduledTaskVo {
  parameters_text?: string
  parameter_schema_text?: string
  retry_max_attempts?: number
  retry_initial_delay_seconds?: number
  retry_max_delay_seconds?: number
}

const t = useTrans().globalTrans
const defaultCronExpression = '0 0 0 */10 * *'
const taskForm = ref<MaFormExpose>()
const tenantOptionRows = ref<ScheduledTaskTenantOptionVo[]>([])
const handlerRows = ref<ScheduledTaskHandlerDefinitionVo[]>([])
const taskModel = ref<ScheduledTaskFormModel>({
  cron_expression: defaultCronExpression,
  timezone: 'UTC',
  task_type: 'handler',
  handler_key: 'scheduler.noop',
  parameters: {},
  parameters_text: '{}',
  parameter_schema_text: '{}',
  timeout_seconds: 5,
  concurrency_policy: 'FORBID',
  misfire_policy: 'SCHEDULER_DEFAULT',
  retry_policy: {
    max_attempts: 1,
    initial_delay_seconds: 1,
    max_delay_seconds: 30,
  },
  retry_max_attempts: 1,
  retry_initial_delay_seconds: 1,
  retry_max_delay_seconds: 30,
  scope: 'GLOBAL',
  max_log_output: 4000,
  target_ips: [],
  tenant_ids: [],
  target_ips_text: '',
  run_on_one_server: true,
  status: 1,
  version: 1,
})

const selectedHandler = computed(() =>
  handlerRows.value.find(item => item.handler_key === taskModel.value.handler_key),
)

function assignTaskModel(value: ScheduledTaskFormModel) {
  Object.keys(taskModel.value).forEach((key) => {
    delete taskModel.value[key as keyof ScheduledTaskFormModel]
  })
  Object.assign(taskModel.value, value)
}

Promise.all([
  useForm('taskForm'),
  fetchTenantOptions(),
  fetchHandlers(),
]).then(([form, tenantResponse, handlerResponse]: [MaFormExpose, any, any]) => {
  tenantOptionRows.value = tenantResponse.data ?? []
  handlerRows.value = handlerResponse.data ?? []

  if (formType === 'edit' && data) {
    const retry = data.retry_policy ?? {}
    assignTaskModel({
      ...data,
      task_type: 'handler',
      parameters: normalizeObject(data.parameters),
      parameters_text: JSON.stringify(normalizeObject(data.parameters), null, 2),
      parameter_schema_text: '',
      retry_max_attempts: numberValue(retry.max_attempts, 1),
      retry_initial_delay_seconds: numberValue(retry.initial_delay_seconds, 1),
      retry_max_delay_seconds: numberValue(retry.max_delay_seconds, 30),
      tenant_ids: data.tenant_ids ?? [],
      target_ips_text: (data.target_ips ?? []).join('\n'),
    })
  }
  else if (!handlerRows.value.some(item => item.handler_key === taskModel.value.handler_key)) {
    taskModel.value.handler_key = handlerRows.value[0]?.handler_key
  }

  syncHandlerDefaults(formType === 'add')
  form.setItems(formItems())
  form.setOptions({ labelWidth: '132px' })
})

function normalizeObject(value: unknown): Record<string, any> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, any>
  }
  return {}
}

function numberValue(value: unknown, fallback: number) {
  const number = Number(value)
  return Number.isFinite(number) && number > 0 ? number : fallback
}

function syncHandlerDefaults(resetTimeout: boolean) {
  const definition = selectedHandler.value
  taskModel.value.parameter_schema_text = JSON.stringify(definition?.parameter_schema ?? {}, null, 2)
  if (resetTimeout && definition) {
    taskModel.value.timeout_seconds = definition.default_timeout
  }
  if (definition?.tenant_capability === 'GLOBAL_ONLY') {
    taskModel.value.scope = 'GLOBAL'
    taskModel.value.tenant_ids = []
  }
  if (!definition?.supports_cancellation && taskModel.value.concurrency_policy === 'REPLACE') {
    taskModel.value.concurrency_policy = 'FORBID'
  }
}

function handleHandlerChange() {
  syncHandlerDefaults(true)
  taskModel.value.parameters = {}
  taskModel.value.parameters_text = '{}'
  taskForm.value?.setItems(formItems())
}

function handleScopeChange(scope: string) {
  if (scope === 'GLOBAL') {
    taskModel.value.tenant_ids = []
  }
  taskForm.value?.setItems(formItems())
}

function required(message: string) {
  return [{ required: true, message }]
}

function formItems(): MaFormItem[] {
  return [
    {
      label: () => t('baseScheduledTaskManage.name'),
      prop: 'name',
      render: 'input',
      cols: { md: 12, xs: 24 },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.name') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.code'),
      prop: 'code',
      render: 'input',
      cols: { md: 12, xs: 24 },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.code') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.cron'),
      prop: 'cron_expression',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: { placeholder: defaultCronExpression },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.cron') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.timezone'),
      prop: 'timezone',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: { placeholder: 'UTC' },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.timezone') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.handler'),
      prop: 'handler_key',
      render: () => ElSelect,
      cols: { md: 16, xs: 24 },
      renderProps: {
        filterable: true,
        onChange: handleHandlerChange,
      },
      renderSlots: {
        default: () => handlerRows.value.map(item => (
          <ElOption
            label={`${item.handler_key} - ${item.description}`}
            value={item.handler_key}
          />
        )),
      },
      itemProps: { rules: required(t('form.requiredSelect', { msg: t('baseScheduledTaskManage.handler') })) },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictRadio,
      cols: { md: 8, xs: 24 },
      renderProps: { dictName: 'system-status' },
    },
    {
      label: () => t('baseScheduledTaskManage.concurrencyPolicy'),
      prop: 'concurrency_policy',
      render: () => ElSelect,
      cols: { md: 8, xs: 24 },
      renderSlots: {
        default: () => concurrencyPolicies.map(item => (
          <ElOption
            label={item.label}
            value={item.value}
            disabled={item.value === 'REPLACE' && !selectedHandler.value?.supports_cancellation}
          />
        )),
      },
    },
    {
      label: () => t('baseScheduledTaskManage.misfirePolicy'),
      prop: 'misfire_policy',
      render: () => ElSelect,
      cols: { md: 8, xs: 24 },
      renderSlots: {
        default: () => misfirePolicies.map(item => <ElOption label={item.label} value={item.value} />),
      },
    },
    {
      label: () => t('baseScheduledTaskManage.scope'),
      prop: 'scope',
      render: () => ElSelect,
      cols: { md: 8, xs: 24 },
      renderProps: { onChange: handleScopeChange },
      renderSlots: {
        default: () => taskScopes.map(item => (
          <ElOption
            label={item.label}
            value={item.value}
            disabled={item.value === 'PER_TENANT' && selectedHandler.value?.tenant_capability === 'GLOBAL_ONLY'}
          />
        )),
      },
    },
    {
      label: () => t('baseScheduledTaskManage.timeout'),
      prop: 'timeout_seconds',
      render: 'inputNumber',
      cols: { md: 8, xs: 24 },
      renderProps: { class: 'w-full', min: 1, max: 86400 },
    },
    {
      label: () => t('baseScheduledTaskManage.maxAttempts'),
      prop: 'retry_max_attempts',
      render: 'inputNumber',
      cols: { md: 8, xs: 24 },
      renderProps: { class: 'w-full', min: 1, max: 100 },
    },
    {
      label: () => t('baseScheduledTaskManage.initialDelay'),
      prop: 'retry_initial_delay_seconds',
      render: 'inputNumber',
      cols: { md: 8, xs: 24 },
      renderProps: { class: 'w-full', min: 1, max: 86400 },
    },
    {
      label: () => t('baseScheduledTaskManage.maxDelay'),
      prop: 'retry_max_delay_seconds',
      render: 'inputNumber',
      cols: { md: 8, xs: 24 },
      renderProps: { class: 'w-full', min: 1, max: 86400 },
    },
    {
      label: () => t('baseScheduledTaskManage.targetIps'),
      prop: 'target_ips_text',
      render: 'input',
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 2, placeholder: '10.0.0.8\n10.0.0.9' },
    },
    {
      label: () => t('baseScheduledTaskManage.tenants'),
      prop: 'tenant_ids',
      render: () => ElSelect,
      show: () => taskModel.value.scope === 'PER_TENANT',
      cols: { xs: 24 },
      renderProps: {
        multiple: true,
        filterable: true,
        clearable: true,
        collapseTags: true,
        collapseTagsTooltip: true,
        placeholder: t('baseScheduledTaskManage.allTenants'),
      },
      renderSlots: {
        default: () => tenantOptionRows.value.map(item => (
          <ElOption label={`${item.name}(${item.code})`} value={item.id} />
        )),
      },
    },
    {
      label: () => t('baseScheduledTaskManage.parameters'),
      prop: 'parameters_text',
      render: 'input',
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 6, placeholder: '{}' },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.parameters') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.parameterSchema'),
      prop: 'parameter_schema_text',
      render: 'input',
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 5, disabled: true },
    },
    {
      label: () => t('baseScheduledTaskManage.description'),
      prop: 'description',
      render: 'input',
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 2 },
    },
  ]
}

function payload(): ScheduledTaskVo {
  const parameters = JSON.parse(taskModel.value.parameters_text?.trim() || '{}')
  if (!parameters || typeof parameters !== 'object' || Array.isArray(parameters)) {
    throw new Error(t('baseScheduledTaskManage.parametersJsonError'))
  }
  const targetIps = (taskModel.value.target_ips_text ?? '')
    .split(/\r?\n|,/)
    .map(item => item.trim())
    .filter(Boolean)
  const {
    target_ips_text,
    parameters_text,
    parameter_schema_text,
    retry_max_attempts,
    retry_initial_delay_seconds,
    retry_max_delay_seconds,
    ...data
  } = taskModel.value
  return {
    ...data,
    task_type: 'handler',
    payload: { handler: data.handler_key },
    parameters,
    retry_policy: {
      max_attempts: retry_max_attempts ?? 1,
      initial_delay_seconds: retry_initial_delay_seconds ?? 1,
      max_delay_seconds: retry_max_delay_seconds ?? 30,
    },
    target_ips: targetIps,
    tenant_ids: data.scope === 'PER_TENANT' ? data.tenant_ids : [],
  }
}

function submit(): Promise<any> {
  return new Promise((resolve, reject) => {
    let requestData: ScheduledTaskVo
    try {
      requestData = payload()
    }
    catch {
      reject(new Error(t('baseScheduledTaskManage.parametersJsonError')))
      return
    }
    const request = formType === 'add'
      ? create(requestData)
      : save(requestData.id as number, requestData)
    request.then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch(reject)
  })
}

defineExpose({
  add: submit,
  edit: submit,
  maForm: taskForm,
})
</script>

<template>
  <ma-form ref="taskForm" v-model="taskModel" />
</template>
