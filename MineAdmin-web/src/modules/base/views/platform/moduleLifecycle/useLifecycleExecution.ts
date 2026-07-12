import type {
  ModuleLifecycleAction,
  ModuleLifecycleExecutePayload,
  ModuleLifecycleResultVo,
  ModuleLifecycleStateVo,
} from '~/base/api/platformModuleLifecycle'
import type { EvidenceDialogExpose } from './sensitiveOperation'
import type { Ref } from 'vue'
import { execute } from '~/base/api/platformModuleLifecycle'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmSensitiveOperation, requestSensitiveEvidence } from './sensitiveOperation'

export interface LifecycleActionForm {
  action: ModuleLifecycleAction
  module_id: string
  execute: boolean
  owner: string
  reason: string
  confirm_token: string
  reauth_token: string
  approval_id: string
}

interface UseLifecycleExecutionOptions {
  stateRows: Readonly<Ref<ModuleLifecycleStateVo[]>>
  evidenceDialog: Readonly<Ref<EvidenceDialogExpose | undefined>>
  refreshAll: () => Promise<void>
}

function createExecutionContext(options: UseLifecycleExecutionOptions) {
  const { t } = useI18n()
  const form = reactive<LifecycleActionForm>({
    action: 'upgrade',
    module_id: '',
    execute: false,
    owner: '',
    reason: '',
    confirm_token: '',
    reauth_token: '',
    approval_id: '',
  })
  const expectedConfirmToken = computed(() => `${form.module_id || 'all'}:${form.action}`)
  return {
    options, t, form, expectedConfirmToken,
    msg: useMessage(),
    loading: ref(false),
    result: ref<ModuleLifecycleResultVo | null>(null),
    moduleOptions: computed(() => options.stateRows.value.map(item => ({
      label: `${item.name} (${item.id})`,
      value: item.id,
    }))),
  }
}

type ExecutionContext = ReturnType<typeof createExecutionContext>

function validateExecution(context: ExecutionContext) {
  const { form, msg, t } = context
  if (!form.execute) {
    return true
  }
  if (!form.owner.trim() || !form.reason.trim()) {
    msg.warning(t('baseModuleLifecycle.executeRequirement'))
    return false
  }
  if (form.confirm_token.trim() !== context.expectedConfirmToken.value) {
    msg.warning(t('baseModuleLifecycle.securityRequirement', { token: context.expectedConfirmToken.value }))
    return false
  }
  return true
}

async function attachExecutionEvidence(context: ExecutionContext) {
  const { form, options } = context
  if (!form.execute) {
    return true
  }
  const evidence = await requestSensitiveEvidence({
    dialog: options.evidenceDialog.value,
    request: {
      scope: 'module.lifecycle.execute',
      resource: `module-lifecycle:${form.module_id || 'all'}:${form.action}`,
      reason: form.reason.trim(),
    },
  })
  if (!evidence) {
    return false
  }
  Object.assign(form, evidence)
  return true
}

function executionPayload(context: ExecutionContext): ModuleLifecycleExecutePayload {
  const { form } = context
  return {
    action: form.action,
    module_id: form.module_id || undefined,
    execute: form.execute,
    owner: form.owner.trim(),
    reason: form.reason.trim(),
    confirm_token: form.confirm_token.trim(),
    reauth_token: form.reauth_token.trim(),
    approval_id: form.approval_id.trim(),
  }
}

async function runLifecycleAction(context: ExecutionContext) {
  const { form, loading, msg, options, result, t } = context
  loading.value = true
  try {
    const response = await execute(executionPayload(context))
    if (response.code !== ResultCode.SUCCESS) {
      msg.error(response.message)
      return
    }
    result.value = response.data
    msg.success(form.execute
      ? t('baseModuleLifecycle.executeSuccess')
      : t('baseModuleLifecycle.dryRunSuccess'))
    await options.refreshAll()
  }
  finally {
    loading.value = false
  }
}

async function submitLifecycleAction(context: ExecutionContext) {
  if (!validateExecution(context) || !await attachExecutionEvidence(context)) {
    return
  }
  if (!context.form.execute) {
    await runLifecycleAction(context)
    return
  }
  await confirmSensitiveOperation({
    confirm: () => context.msg.confirm(context.t('baseModuleLifecycle.executeConfirm')),
    run: () => runLifecycleAction(context),
  })
}

export function useLifecycleExecution(options: UseLifecycleExecutionOptions) {
  const context = createExecutionContext(options)
  return {
    form: context.form,
    result: context.result,
    loading: context.loading,
    expectedConfirmToken: context.expectedConfirmToken,
    moduleOptions: context.moduleOptions,
    submit: () => submitLifecycleAction(context),
  }
}
