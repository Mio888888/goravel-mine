<script setup lang="ts">
import type { FormInstance } from 'element-plus'
import type { AdapterPayload, MiddlewareAdapterVo } from '~/base/api/platformMiddleware'
import {
  checkAdapterHealth,
  createAdapter,
  setAdapterEnabled,
  testAdapterConnection,
  updateAdapter,
} from '~/base/api/platformMiddleware'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

const props = defineProps<{
  rows: MiddlewareAdapterVo[]
  loading: boolean
}>()
const emit = defineEmits<{ refresh: [] }>()

const t = useTrans().globalTrans
const msg = useMessage()
const saving = ref(false)
const dialogVisible = ref(false)
const formRef = ref<FormInstance>()
const form = reactive<AdapterPayload & { id?: number }>({
  name: '',
  connection: '',
  enabled: true,
  version: 1,
})

const rules = {
  name: [{ required: true, message: t('baseMiddleware.adapterNameRequired'), trigger: 'blur' }],
  connection: [{ required: true, message: t('baseMiddleware.connectionRequired'), trigger: 'blur' }],
}

function healthType(status: string) {
  return ({ UP: 'success', DEGRADED: 'warning', DOWN: 'danger', UNKNOWN: 'info' } as const)[status] ?? 'info'
}

function capabilityLabels(row: any) {
  const map: Array<[keyof MiddlewareAdapterVo['capabilities'], string]> = [
    ['persistent', t('baseMiddleware.capPersistent')],
    ['cluster', t('baseMiddleware.capCluster')],
    ['broadcast', t('baseMiddleware.capBroadcast')],
    ['offline_recovery', t('baseMiddleware.capOfflineRecovery')],
    ['retry', t('baseMiddleware.capRetry')],
    ['dead_letter', t('baseMiddleware.capDeadLetter')],
    ['ordering', t('baseMiddleware.capOrdering')],
  ]
  return map.filter(([key]) => row.capabilities?.[key]).map(([, label]) => label)
}

function openCreate() {
  Object.assign(form, { id: undefined, name: '', connection: '', enabled: true, version: 1 })
  dialogVisible.value = true
}

function openEdit(row: any) {
  Object.assign(form, {
    id: row.id,
    name: row.name,
    connection: row.connection,
    enabled: row.enabled,
    version: row.version,
  })
  dialogVisible.value = true
}

async function submit() {
  try {
    await formRef.value?.validate()
  }
  catch {
    return
  }
  saving.value = true
  try {
    const response = form.id
      ? await updateAdapter(form.id, {
          name: form.name,
          connection: form.connection,
          enabled: form.enabled,
          version: form.version,
          confirm: !form.enabled,
        })
      : await createAdapter(form)
    if (response.code === ResultCode.SUCCESS) {
      msg.success(form.id ? t('crud.updateSuccess') : t('crud.createSuccess'))
      dialogVisible.value = false
      emit('refresh')
    }
  }
  finally {
    saving.value = false
  }
}

async function runAction(action: 'health' | 'test', row: any) {
  const response = action === 'health'
    ? await checkAdapterHealth(row.id)
    : await testAdapterConnection(row.id)
  if (response.code === ResultCode.SUCCESS) {
    msg.success(action === 'health' ? t('baseMiddleware.healthChecked') : t('baseMiddleware.connectionPassed'))
    emit('refresh')
  }
}

async function toggle(row: any) {
  try {
    await msg.confirm(row.enabled
      ? t('baseMiddleware.disableAdapterConfirm')
      : t('baseMiddleware.enableAdapterConfirm'))
  }
  catch {
    return
  }
  const response = await setAdapterEnabled(row.id, !row.enabled, row.version, row.enabled)
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('crud.updateSuccess'))
    emit('refresh')
  }
}
</script>

<template>
  <div class="middleware-section">
    <div class="middleware-section-toolbar">
      <div>
        <h3>{{ t('baseMiddleware.adapters') }}</h3>
        <p>{{ t('baseMiddleware.adaptersHint') }}</p>
      </div>
      <el-button v-auth="['platform:middleware:configure']" type="primary" @click="openCreate">
        {{ t('baseMiddleware.registerAdapter') }}
      </el-button>
    </div>

    <el-table v-loading="props.loading" :data="props.rows" border>
      <el-table-column prop="name" :label="t('baseMiddleware.name')" min-width="180" />
      <el-table-column prop="adapter_type" :label="t('baseMiddleware.adapterType')" width="150" />
      <el-table-column prop="connection" :label="t('baseMiddleware.connection')" width="140" />
      <el-table-column :label="t('baseMiddleware.capabilities')" min-width="320">
        <template #default="{ row }">
          <el-space wrap>
            <el-tag v-for="label in capabilityLabels(row)" :key="label" type="info">
              {{ label }}
            </el-tag>
            <span v-if="capabilityLabels(row).length === 0">-</span>
          </el-space>
        </template>
      </el-table-column>
      <el-table-column :label="t('baseMiddleware.health')" width="120">
        <template #default="{ row }">
          <el-tag :type="healthType(row.health_status)">
            {{ row.health_status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('crud.status')" width="100">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">
            {{ row.enabled ? t('baseMiddleware.enabled') : t('baseMiddleware.disabled') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="version" :label="t('baseMiddleware.version')" width="90" />
      <el-table-column :label="t('crud.operation')" width="340" fixed="right">
        <template #default="{ row }">
          <el-button v-auth="['platform:middleware:configure']" type="primary" link @click="openEdit(row)">
            {{ t('crud.edit') }}
          </el-button>
          <el-button v-auth="['platform:middleware:execute']" link @click="runAction('test', row)">
            {{ t('baseMiddleware.testConnection') }}
          </el-button>
          <el-button v-auth="['platform:middleware:execute']" link @click="runAction('health', row)">
            {{ t('baseMiddleware.checkHealth') }}
          </el-button>
          <el-button
            v-auth="['platform:middleware:configure']"
            :type="row.enabled ? 'danger' : 'success'"
            link
            @click="toggle(row)"
          >
            {{ row.enabled ? t('baseMiddleware.disable') : t('baseMiddleware.enable') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog
      v-model="dialogVisible"
      :title="form.id ? t('baseMiddleware.editAdapter') : t('baseMiddleware.registerAdapter')"
      width="560px"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="110px">
        <el-form-item :label="t('baseMiddleware.name')" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.connection')" prop="connection">
          <el-input v-model="form.connection" :disabled="Boolean(form.id)" placeholder="sync / database / redis" />
        </el-form-item>
        <el-form-item :label="t('crud.status')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-alert
          :title="t('baseMiddleware.staticConfigHint')"
          type="info"
          :closable="false"
          show-icon
        />
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">
          {{ t('crud.cancel') }}
        </el-button>
        <el-button type="primary" :loading="saving" @click="submit">
          {{ t('crud.ok') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>
