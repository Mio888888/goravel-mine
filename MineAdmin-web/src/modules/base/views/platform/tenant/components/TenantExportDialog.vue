<script setup lang="ts">
import type { TenantVo } from '~/base/api/tenant'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'
import { downloadExport, exportStatus, governance, requestExport } from '~/base/api/tenant'
import download from '@/utils/download'
import { ResultCode } from '@/utils/ResultCode'
import { useMessage } from '@/hooks/useMessage'

const requestSensitiveEvidence = inject<SensitiveEvidenceRequester>('requestSensitiveEvidence')!
const msg = useMessage()
const visible = ref(false)
const loading = ref(false)
const tenant = ref<TenantVo>()
const form = reactive({ format: 'jsonl' as 'jsonl' | 'csv', status: '' as '' | '1' | '2' })

function open(row: TenantVo) {
  tenant.value = row
  form.format = 'jsonl'
  form.status = ''
  visible.value = true
}

async function submit() {
  const current = tenant.value
  if (!current?.id) {
    return
  }
  loading.value = true
  try {
    const policy = await governance(current.id)
    if (policy.code !== ResultCode.SUCCESS || !policy.data.data_export.enabled) {
      throw new Error('Tenant export is disabled')
    }
    const emptyDigest = 'sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'
    const filters = form.status ? { status: form.status } : {}
    const filterDigest = form.status ? await sha256(`status=${form.status}`) : emptyDigest
    const resource = `tenant-data:export:${current.id}:users:${form.format}:${filterDigest}`
    const evidence = await requestSensitiveEvidence({
      policy_key: 'tenant.data.export',
      scope: 'tenant.data.export',
      resource,
      reason: `Export tenant ${current.id} users`,
      approval_required: policy.data.data_export.requires_approval,
    })
    const created = await requestExport(current.id, { dataset: 'users', format: form.format, filters, ...evidence })
    if (created.code !== ResultCode.SUCCESS) {
      throw created
    }
    const completed = await waitForExport(current.id, created.data.id)
    if (!completed.download_token) {
      throw new Error(completed.run.error || 'Export did not complete')
    }
    const file = await downloadExport(current.id, completed.run.id, completed.download_token)
    form.format === 'csv' ? download.csv(file.data, file.fileName) : download.jsonl(file.data, file.fileName)
    msg.success('Export completed')
    visible.value = false
  }
  catch (error: any) {
    msg.error(error?.message ?? 'Export failed')
  }
  finally {
    loading.value = false
  }
}

async function waitForExport(tenantID: number, runID: number) {
  for (let attempt = 0; attempt < 30; attempt++) {
    const response = await exportStatus(tenantID, runID)
    if (response.code !== ResultCode.SUCCESS) {
      throw response
    }
    if (response.data.run.status === 'completed' || response.data.run.status === 'failed') {
      return response.data
    }
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  throw new Error('Export timed out')
}

async function sha256(value: string) {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
  return `sha256:${Array.from(new Uint8Array(digest)).map(byte => byte.toString(16).padStart(2, '0')).join('')}`
}

defineExpose({ open })
</script>

<template>
  <el-dialog v-model="visible" title="Tenant data export" width="520px" append-to-body :close-on-click-modal="false">
    <el-form v-loading="loading" label-position="top">
      <el-form-item label="Dataset">
        <el-input model-value="Users" disabled />
      </el-form-item>
      <el-form-item label="Format">
        <el-segmented v-model="form.format" :options="[{ label: 'JSONL', value: 'jsonl' }, { label: 'CSV', value: 'csv' }]" />
      </el-form-item>
      <el-form-item label="User status">
        <el-select v-model="form.status" clearable>
          <el-option label="All" value="" />
          <el-option label="Enabled" value="1" />
          <el-option label="Disabled" value="2" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">
        Cancel
      </el-button>
      <el-button type="primary" :loading="loading" @click="submit">
        Export
      </el-button>
    </template>
  </el-dialog>
</template>
