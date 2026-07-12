<script setup lang="tsx">
import type { MaFormExpose, MaFormItem } from '@mineadmin/form'
import type { ScheduledTaskTenantOptionVo, ScheduledTaskType, ScheduledTaskVo } from '~/base/api/platformScheduledTask'
import { ElOption, ElSelect } from 'element-plus'
import { create, tenantOptions as fetchTenantOptions, save } from '~/base/api/platformScheduledTask'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import { taskTypes } from './data/options.ts'

defineOptions({ name: 'platform:scheduledTask:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: ScheduledTaskVo | null
}>()

interface ScheduledTaskFormModel extends ScheduledTaskVo {
  url_method?: string
  url?: string
  url_headers?: string
  url_body?: string
  script_command?: string
  script_args?: string
  script_workdir?: string
  method_handler?: string
  backup_connection?: string
  backup_target?: string
}

const t = useTrans().globalTrans
const defaultCronExpression = '0 0 0 */10 * *'
const taskForm = ref<MaFormExpose>()
const tenantOptionRows = ref<ScheduledTaskTenantOptionVo[]>([])
const taskModel = ref<ScheduledTaskFormModel>({
  cron_expression: defaultCronExpression,
  timezone: 'UTC',
  task_type: 'method',
  payload: { handler: 'scheduler.noop' },
  method_handler: 'scheduler.noop',
  timeout_seconds: 60,
  allow_overlap: false,
  max_log_output: 4000,
  target_ips: [],
  tenant_ids: [],
  target_ips_text: '',
  run_on_one_server: true,
  status: 1,
})

function assignTaskModel(value: ScheduledTaskFormModel) {
  Object.keys(taskModel.value).forEach((key) => {
    delete taskModel.value[key as keyof ScheduledTaskFormModel]
  })
  Object.assign(taskModel.value, value)
}

Promise.all([
  useForm('taskForm'),
  fetchTenantOptions(),
]).then(([form, tenantResponse]: [MaFormExpose, any]) => {
  tenantOptionRows.value = tenantResponse.data ?? []
  if (formType === 'edit' && data) {
    assignTaskModel({
      ...data,
      tenant_ids: data.tenant_ids ?? [],
      target_ips_text: (data.target_ips ?? []).join('\n'),
      ...payloadFields(data.task_type, normalizePayload(data.task_type, data.payload)),
    })
  }
  form.setItems(formItems())
  form.setOptions({ labelWidth: '120px' })
})

function defaultPayload(type?: ScheduledTaskType) {
  if (type === 'url') {
    return { method: 'GET', url: 'https://example.com/health', headers: {} }
  }
  if (type === 'script') {
    return { command: 'storage/scripts/example.sh', args: [], workdir: '' }
  }
  if (type === 'backup') {
    return { connection: 'postgres', target: 'storage/backups' }
  }
  return { handler: 'scheduler.noop' }
}

function handleTypeChange(type: ScheduledTaskType) {
  taskModel.value.task_type = type
  taskModel.value.payload = defaultPayload(type)
  Object.assign(taskModel.value, emptyPayloadFields(), payloadFields(type, taskModel.value.payload))
  taskForm.value?.setItems(formItems())
}

function required(message: string) {
  return [{ required: true, message }]
}

function emptyPayloadFields() {
  return {
    url_method: undefined,
    url: undefined,
    url_headers: undefined,
    url_body: undefined,
    script_command: undefined,
    script_args: undefined,
    script_workdir: undefined,
    method_handler: undefined,
    backup_connection: undefined,
    backup_target: undefined,
  }
}

function normalizePayload(type: ScheduledTaskType | undefined, payload: unknown): Record<string, any> {
  if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
    return payload as Record<string, any>
  }
  if (typeof payload === 'string' && payload.trim()) {
    try {
      const parsed = JSON.parse(payload)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed
      }
    }
    catch {}
  }
  return defaultPayload(type)
}

function payloadFields(type: ScheduledTaskType | undefined, payload: Record<string, any>) {
  if (type === 'url') {
    return {
      url_method: payload.method ?? 'GET',
      url: payload.url ?? '',
      url_headers: JSON.stringify(payload.headers ?? {}, null, 2),
      url_body: payload.body ?? '',
    }
  }
  if (type === 'script') {
    return {
      script_command: payload.command ?? 'storage/scripts/example.sh',
      script_args: (payload.args ?? []).join('\n'),
      script_workdir: payload.workdir ?? '',
    }
  }
  if (type === 'backup') {
    return {
      backup_connection: payload.connection ?? 'postgres',
      backup_target: payload.target ?? 'storage/backups',
    }
  }
  return {
    method_handler: payload.handler ?? 'scheduler.noop',
  }
}

function isTaskType(type: ScheduledTaskType) {
  return () => taskModel.value.task_type === type
}

function formItems(): MaFormItem[] {
  return [
    ...baseItems(),
    ...executionItems(),
    ...contentItems(),
  ]
}

function baseItems(): MaFormItem[] {
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
    },
  ]
}

function executionItems(): MaFormItem[] {
  return [
    {
      label: () => t('baseScheduledTaskManage.taskType'),
      prop: 'task_type',
      render: () => ElSelect,
      cols: { md: 12, xs: 24 },
      renderProps: { onChange: handleTypeChange },
      renderSlots: {
        default: () => taskTypes.map(item => <ElOption label={item.label} value={item.value} />),
      },
      itemProps: { rules: required(t('form.requiredSelect', { msg: t('baseScheduledTaskManage.taskType') })) },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictRadio,
      cols: { md: 12, xs: 24 },
      renderProps: { dictName: 'system-status' },
    },
    {
      label: () => t('baseScheduledTaskManage.timeout'),
      prop: 'timeout_seconds',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: { class: 'w-full', min: 1, max: 86400 },
    },
    {
      label: () => t('baseScheduledTaskManage.allowOverlap'),
      prop: 'allow_overlap',
      render: 'switch',
      cols: { md: 12, xs: 24 },
      renderProps: { activeValue: true, inactiveValue: false },
    },
  ]
}

function contentItems(): MaFormItem[] {
  return [
    {
      label: () => t('baseScheduledTaskManage.targetIps'),
      prop: 'target_ips_text',
      render: 'input',
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 3, placeholder: '10.0.0.8\n10.0.0.9' },
    },
    {
      label: () => t('baseScheduledTaskManage.tenants'),
      prop: 'tenant_ids',
      render: () => ElSelect,
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
      label: () => t('baseScheduledTaskManage.payload'),
      prop: 'method_handler',
      render: 'input',
      show: isTaskType('method'),
      cols: { xs: 24 },
      renderProps: { placeholder: 'scheduler.noop' },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.payload') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.urlMethod'),
      prop: 'url_method',
      render: () => ElSelect,
      show: isTaskType('url'),
      cols: { md: 8, xs: 24 },
      renderSlots: {
        default: () => ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map(method => <ElOption label={method} value={method} />),
      },
    },
    {
      label: () => t('baseScheduledTaskManage.url'),
      prop: 'url',
      render: 'input',
      show: isTaskType('url'),
      cols: { md: 16, xs: 24 },
      renderProps: { placeholder: 'https://example.com/health' },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.url') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.urlHeaders'),
      prop: 'url_headers',
      render: 'input',
      show: isTaskType('url'),
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 4, placeholder: '{\n  "Authorization": "Bearer token"\n}' },
    },
    {
      label: () => t('baseScheduledTaskManage.urlBody'),
      prop: 'url_body',
      render: 'input',
      show: isTaskType('url'),
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 4 },
    },
    {
      label: () => t('baseScheduledTaskManage.scriptCommand'),
      prop: 'script_command',
      render: 'input',
      show: isTaskType('script'),
      cols: { xs: 24 },
      renderProps: { placeholder: 'storage/scripts/example.sh' },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.scriptCommand') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.scriptArgs'),
      prop: 'script_args',
      render: 'input',
      show: isTaskType('script'),
      cols: { xs: 24 },
      renderProps: { type: 'textarea', rows: 3, placeholder: t('baseScheduledTaskManage.scriptArgsPlaceholder') },
    },
    {
      label: () => t('baseScheduledTaskManage.scriptWorkdir'),
      prop: 'script_workdir',
      render: 'input',
      show: isTaskType('script'),
      cols: { xs: 24 },
      renderProps: { placeholder: t('baseScheduledTaskManage.scriptWorkdirPlaceholder') },
    },
    {
      label: () => t('baseScheduledTaskManage.backupConnection'),
      prop: 'backup_connection',
      render: 'input',
      show: isTaskType('backup'),
      cols: { md: 12, xs: 24 },
      renderProps: { placeholder: 'postgres' },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.backupConnection') })) },
    },
    {
      label: () => t('baseScheduledTaskManage.backupTarget'),
      prop: 'backup_target',
      render: 'input',
      show: isTaskType('backup'),
      cols: { md: 12, xs: 24 },
      renderProps: { placeholder: 'storage/backups' },
      itemProps: { rules: required(t('form.requiredInput', { msg: t('baseScheduledTaskManage.backupTarget') })) },
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
  const payloadData = buildPayload()
  const targetIps = (taskModel.value.target_ips_text ?? '')
    .split(/\r?\n|,/)
    .map(item => item.trim())
    .filter(Boolean)
  const {
    target_ips_text,
    url_method,
    url,
    url_headers,
    url_body,
    script_command,
    script_args,
    script_workdir,
    method_handler,
    backup_connection,
    backup_target,
    ...data
  } = taskModel.value
  return { ...data, payload: payloadData, target_ips: targetIps }
}

function buildPayload(): Record<string, any> {
  if (taskModel.value.task_type === 'url') {
    const headers = taskModel.value.url_headers?.trim()
      ? JSON.parse(taskModel.value.url_headers)
      : {}
    return {
      method: taskModel.value.url_method || 'GET',
      url: taskModel.value.url,
      headers,
      body: taskModel.value.url_body ?? '',
    }
  }
  if (taskModel.value.task_type === 'script') {
    const args = (taskModel.value.script_args ?? '')
      .split(/\r?\n/)
      .map(item => item.trim())
      .filter(Boolean)
    return {
      command: taskModel.value.script_command,
      args,
      workdir: taskModel.value.script_workdir ?? '',
    }
  }
  if (taskModel.value.task_type === 'backup') {
    return {
      connection: taskModel.value.backup_connection,
      target: taskModel.value.backup_target,
    }
  }
  return {
    handler: taskModel.value.method_handler,
  }
}

function submit(): Promise<any> {
  return new Promise((resolve, reject) => {
    let data: ScheduledTaskVo
    try {
      data = payload()
    }
    catch {
      reject(new Error(t('baseScheduledTaskManage.payloadJsonError')))
      return
    }
    const request = formType === 'add'
      ? create(data)
      : save(data.id as number, data)
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
