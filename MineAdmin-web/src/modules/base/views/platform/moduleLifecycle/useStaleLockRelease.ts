import type {
  ModuleLifecycleLockReleasePayload,
  ModuleLifecycleLockVo,
} from '~/base/api/platformModuleLifecycle'
import type { EvidenceDialogExpose } from './sensitiveOperation'
import type { Ref } from 'vue'
import { releaseStaleLocks } from '~/base/api/platformModuleLifecycle'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { confirmSensitiveOperation, requestSensitiveEvidence } from './sensitiveOperation'

export interface StaleLockReleaseForm {
  key: string
  dry_run: boolean
  confirm_token: string
  reauth_token: string
  approval_id: string
}

interface UseStaleLockReleaseOptions {
  evidenceDialog: Readonly<Ref<EvidenceDialogExpose | undefined>>
  lockRows: Ref<ModuleLifecycleLockVo[]>
  loadLocks: () => Promise<void>
}

function createLockReleaseContext(options: UseStaleLockReleaseOptions) {
  const { t } = useI18n()
  return {
    options, t,
    msg: useMessage(),
    loading: ref(false),
    form: reactive<StaleLockReleaseForm>({
      key: '',
      dry_run: true,
      confirm_token: '',
      reauth_token: '',
      approval_id: '',
    }),
  }
}

type LockReleaseContext = ReturnType<typeof createLockReleaseContext>

function validateLockRelease(context: LockReleaseContext) {
  if (context.form.dry_run || context.form.confirm_token.trim() === 'release-stale-locks') {
    return true
  }
  context.msg.warning(context.t('baseModuleLifecycle.releaseRequirement'))
  return false
}

async function attachLockReleaseEvidence(context: LockReleaseContext) {
  const { form, options } = context
  if (form.dry_run) {
    return true
  }
  const evidence = await requestSensitiveEvidence({
    dialog: options.evidenceDialog.value,
    request: {
      scope: 'module.lifecycle.release-lock',
      resource: `module-lifecycle:stale-locks:${form.key.trim() || 'all'}`,
      reason: 'Release stale module lifecycle locks',
    },
  })
  if (!evidence) {
    return false
  }
  Object.assign(form, evidence)
  return true
}

function lockReleasePayload(context: LockReleaseContext): ModuleLifecycleLockReleasePayload {
  const { form } = context
  return {
    key: form.key.trim() || undefined,
    dry_run: form.dry_run,
    confirm_token: form.confirm_token.trim(),
    reauth_token: form.reauth_token.trim(),
    approval_id: form.approval_id.trim(),
  }
}

async function runLockRelease(context: LockReleaseContext) {
  const { form, loading, msg, options, t } = context
  loading.value = true
  try {
    const response = await releaseStaleLocks(lockReleasePayload(context))
    if (response.code !== ResultCode.SUCCESS) {
      msg.error(response.message)
      return
    }
    msg.success(form.dry_run
      ? t('baseModuleLifecycle.releaseDryRunSuccess')
      : t('baseModuleLifecycle.releaseSuccess'))
    options.lockRows.value = response.data.released ?? []
    await options.loadLocks()
  }
  finally {
    loading.value = false
  }
}

async function submitLockRelease(context: LockReleaseContext) {
  if (!validateLockRelease(context) || !await attachLockReleaseEvidence(context)) {
    return
  }
  if (context.form.dry_run) {
    await runLockRelease(context)
    return
  }
  await confirmSensitiveOperation({
    confirm: () => context.msg.confirm(context.t('baseModuleLifecycle.releaseConfirm')),
    run: () => runLockRelease(context),
  })
}

export function useStaleLockRelease(options: UseStaleLockReleaseOptions) {
  const context = createLockReleaseContext(options)
  return {
    form: context.form,
    loading: context.loading,
    submit: () => submitLockRelease(context),
  }
}
