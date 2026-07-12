<script setup lang="ts">
import type {
  SensitiveApprovalVo,
  SensitiveEvidenceRequest,
  SensitiveEvidenceResult,
} from '~/base/api/platformSecurityControl'
import {
  approvalDetail,
  approveApproval,
  createApproval,
  issueReAuthToken,
} from '~/base/api/platformSecurityControl'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const msg = useMessage()
const visible = ref(false)
const loading = ref(false)
const request = reactive<SensitiveEvidenceRequest>({
  scope: '',
  resource: '',
  reason: '',
})
const form = reactive({
  approval_id: '',
  password: '',
  mfa_code: '',
  reauth_token: '',
})
const approval = ref<SensitiveApprovalVo | null>(null)
const approvalRequired = ref(true)
let settle: ((value: SensitiveEvidenceResult) => void) | null = null
let cancel: ((reason?: unknown) => void) | null = null

const ready = computed(() => {
  const currentApproval = approval.value
  const approvalReady
    = !approvalRequired.value
      || (currentApproval?.status === 'approved'
        && currentApproval.approval_id === form.approval_id.trim()
        && currentApproval.policy_key === (request.policy_key || request.scope)
        && currentApproval.resource === request.resource)
  return approvalReady && Boolean(form.reauth_token)
})

function open(
  input: SensitiveEvidenceRequest,
): Promise<SensitiveEvidenceResult> {
  Object.assign(request, input)
  approvalRequired.value = input.approval_required !== false
  Object.assign(form, {
    approval_id: '',
    password: '',
    mfa_code: '',
    reauth_token: '',
  })
  approval.value = null
  visible.value = true
  return new Promise((resolve, reject) => {
    settle = resolve
    cancel = reject
  })
}

async function run(action: () => Promise<void>) {
  loading.value = true
  try {
    await action()
  }
  catch (error: any) {
    msg.error(error?.message ?? t('baseSecurityEvidence.requestFailed'))
  }
  finally {
    loading.value = false
  }
}

function requestApproval() {
  return run(async () => {
    const response = await createApproval({
      policy_key: request.policy_key || request.scope,
      resource: request.resource,
      reason: request.reason,
    })
    if (response.code !== ResultCode.SUCCESS) {
      throw response
    }
    approval.value = response.data
    form.approval_id = response.data.approval_id
    form.reauth_token = ''
    request.policy_key = response.data.policy_key
    request.scope = response.data.scope
    request.resource = response.data.resource
  })
}

function loadApproval() {
  if (!form.approval_id.trim()) {
    msg.warning(t('baseSecurityEvidence.approvalRequired'))
    return
  }
  return run(async () => {
    const response = await approvalDetail(form.approval_id.trim())
    if (response.code !== ResultCode.SUCCESS) {
      throw response
    }
    approval.value = response.data
    form.reauth_token = ''
  })
}

function approve() {
  if (!form.approval_id.trim()) {
    msg.warning(t('baseSecurityEvidence.approvalRequired'))
    return
  }
  return run(async () => {
    const response = await approveApproval(form.approval_id.trim())
    if (response.code !== ResultCode.SUCCESS) {
      throw response
    }
    approval.value = response.data
    form.reauth_token = ''
  })
}

function authenticate() {
  if (approvalRequired.value && approval.value?.status !== 'approved') {
    msg.warning(t('baseSecurityEvidence.approvalRequired'))
    return
  }
  if (!form.password) {
    msg.warning(t('baseSecurityEvidence.passwordRequired'))
    return
  }
  return run(async () => {
    const response = await issueReAuthToken({
      password: form.password,
      mfa_code: form.mfa_code.trim() || undefined,
      operation: request.scope,
      resource: request.resource,
    })
    if (response.code !== ResultCode.SUCCESS) {
      throw response
    }
    form.reauth_token = response.data.reauth_token
    form.password = ''
    form.mfa_code = ''
  })
}

function continueOperation() {
  if (!ready.value || !settle) {
    msg.warning(t('baseSecurityEvidence.notReady'))
    return
  }
  settle({
    reauth_token: form.reauth_token,
    ...(approvalRequired.value ? { approval_id: form.approval_id.trim() } : {}),
  })
  close(false)
}

function close(reject = true) {
  visible.value = false
  if (reject) {
    cancel?.(new Error('sensitive evidence dialog canceled'))
  }
  settle = null
  cancel = null
}

function handleClosed() {
  close()
}

function statusLabel(status?: string) {
  return t(`baseSecurityEvidence.status.${status || 'missing'}`)
}

defineExpose({ open })
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="t('baseSecurityEvidence.title')"
    width="640px"
    append-to-body
    :close-on-click-modal="false"
    @closed="handleClosed"
  >
    <el-form v-loading="loading" label-position="top">
      <div class="evidence-context">
        <div>
          <span>{{ t("baseSecurityEvidence.operation") }}</span><strong>{{ request.scope }}</strong>
        </div>
        <div>
          <span>{{ t("baseSecurityEvidence.resource") }}</span><strong>{{ request.resource }}</strong>
        </div>
      </div>

      <template v-if="approvalRequired">
        <el-form-item :label="t('baseSecurityEvidence.approvalId')">
          <el-input v-model="form.approval_id" clearable>
            <template #append>
              <el-button @click="loadApproval">
                {{ t("baseSecurityEvidence.loadApproval") }}
              </el-button>
            </template>
          </el-input>
        </el-form-item>
        <div class="evidence-actions">
          <el-button @click="requestApproval">
            {{ t("baseSecurityEvidence.requestApproval") }}
          </el-button>
          <el-button @click="approve">
            {{ t("baseSecurityEvidence.approveApproval") }}
          </el-button>
          <el-tag
            :type="approval?.status === 'approved' ? 'success' : 'warning'"
          >
            {{ statusLabel(approval?.status) }}
          </el-tag>
        </div>

        <el-divider />
      </template>
      <el-form-item :label="t('baseSecurityEvidence.password')">
        <el-input
          v-model="form.password"
          type="password"
          show-password
          autocomplete="current-password"
        />
      </el-form-item>
      <el-form-item :label="t('baseSecurityEvidence.mfaCode')">
        <el-input v-model="form.mfa_code" clearable />
      </el-form-item>
      <div class="evidence-actions">
        <el-button type="primary" plain @click="authenticate">
          {{ t("baseSecurityEvidence.reauthenticate") }}
        </el-button>
        <el-tag :type="form.reauth_token ? 'success' : 'info'">
          {{
            form.reauth_token
              ? t("baseSecurityEvidence.reauthReady")
              : t("baseSecurityEvidence.reauthMissing")
          }}
        </el-tag>
      </div>
    </el-form>

    <template #footer>
      <el-button @click="() => close()">
        {{ t("crud.cancel") }}
      </el-button>
      <el-button type="primary" :disabled="!ready" @click="continueOperation">
        {{ t("baseSecurityEvidence.continue") }}
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.evidence-context {
  display: grid;
  gap: 8px;
  margin-bottom: 18px;
  padding: 12px;
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
}

.evidence-context div {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr);
  gap: 12px;
}

.evidence-context strong {
  overflow-wrap: anywhere;
}

.evidence-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}
</style>
