<script setup lang="ts">
import type { FormInstance } from 'element-plus'
import type {
  MessageRouteVo,
  MiddlewareAdapterVo,
  MiddlewareRegistryVo,
} from '~/base/api/platformMiddleware'
import {
  createRoute,
  publishRoute,
  routeDetail,
  routes,
  setRouteEnabled,
  updateRoute,
  validateRoute,
} from '~/base/api/platformMiddleware'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

const props = defineProps<{
  registry: MiddlewareRegistryVo
  adapters: MiddlewareAdapterVo[]
}>()

const t = useTrans().globalTrans
const msg = useMessage()
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const validationVisible = ref(false)
const validation = ref({ valid: false, errors: [] as string[], warnings: [] as string[] })
const formRef = ref<FormInstance>()
const rows = ref<MessageRouteVo[]>([])
const total = ref(0)
const query = reactive({
  name: '',
  message_type: '',
  status: '',
  adapter_id: undefined as number | undefined,
  page: 1,
  page_size: 15,
})
const form = reactive<MessageRouteVo>({
  name: '',
  message_type: '',
  adapter_id: 0,
  destination: '',
  consumption_mode: 'CLUSTER',
  consumer_group: '',
  concurrency: 1,
  ordering_enabled: false,
  retry_policy: {
    max_attempts: 4,
    initial_delay_seconds: 1,
    max_delay_seconds: 30,
  },
  dead_letter_policy: {
    destination: '',
    retention_days: 30,
    alert_enabled: true,
  },
  enabled: true,
  version: 1,
})

const rules = {
  name: [{ required: true, message: t('baseMiddleware.routeNameRequired'), trigger: 'blur' }],
  message_type: [{ required: true, message: t('baseMiddleware.messageTypeRequired'), trigger: 'change' }],
  adapter_id: [{ required: true, message: t('baseMiddleware.adapterRequired'), trigger: 'change' }],
  destination: [{ required: true, message: t('baseMiddleware.destinationRequired'), trigger: 'blur' }],
}

function adapterName(id: number) {
  return props.adapters.find(item => item.id === id)?.name ?? String(id)
}

async function loadRows() {
  loading.value = true
  try {
    const response = await routes(query)
    rows.value = response.data?.list ?? []
    total.value = response.data?.total ?? 0
  }
  finally {
    loading.value = false
  }
}

function resetQuery() {
  Object.assign(query, { name: '', message_type: '', status: '', adapter_id: undefined, page: 1 })
  loadRows()
}

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    message_type: props.registry.message_types[0]?.message_type ?? '',
    adapter_id: props.adapters.find(item => item.enabled)?.id ?? props.adapters[0]?.id ?? 0,
    destination: '',
    consumption_mode: 'CLUSTER',
    consumer_group: '',
    concurrency: 1,
    ordering_enabled: false,
    retry_policy: { max_attempts: 4, initial_delay_seconds: 1, max_delay_seconds: 30 },
    dead_letter_policy: { destination: '', retention_days: 30, alert_enabled: true },
    enabled: true,
    version: 1,
  })
}

function openCreate() {
  resetForm()
  dialogVisible.value = true
}

async function openEdit(row: any) {
  const response = await routeDetail(row.id as number)
  if (response.code === ResultCode.SUCCESS) {
    Object.assign(form, structuredClone(response.data))
    dialogVisible.value = true
  }
}

async function submit() {
  try {
    await formRef.value?.validate()
  }
  catch {
    return
  }
  if (form.consumption_mode === 'CLUSTER' && !form.consumer_group.trim()) {
    msg.error(t('baseMiddleware.consumerGroupRequired'))
    return
  }
  saving.value = true
  try {
    const response = form.id
      ? await updateRoute(form.id, form)
      : await createRoute(form)
    if (response.code === ResultCode.SUCCESS) {
      msg.success(form.id ? t('crud.updateSuccess') : t('crud.createSuccess'))
      dialogVisible.value = false
      await loadRows()
    }
  }
  finally {
    saving.value = false
  }
}

async function showValidation(row: any) {
  const response = await validateRoute(row.id as number)
  if (response.code === ResultCode.SUCCESS) {
    validation.value = response.data
    validationVisible.value = true
  }
}

async function publish(row: any) {
  try {
    await msg.confirm(t('baseMiddleware.publishRouteConfirm'))
  }
  catch {
    return
  }
  const response = await publishRoute(row.id as number, row.version as number, crypto.randomUUID())
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('baseMiddleware.published'))
    await loadRows()
  }
}

async function toggle(row: any) {
  try {
    await msg.confirm(row.enabled
      ? t('baseMiddleware.disableRouteConfirm')
      : t('baseMiddleware.enableRouteConfirm'))
  }
  catch {
    return
  }
  const response = await setRouteEnabled(row.id as number, !row.enabled, row.version as number)
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('crud.updateSuccess'))
    await loadRows()
  }
}

onMounted(loadRows)
</script>

<template>
  <div class="middleware-section">
    <div class="middleware-section-toolbar">
      <div>
        <h3>{{ t('baseMiddleware.routes') }}</h3>
        <p>{{ t('baseMiddleware.routesHint') }}</p>
      </div>
      <el-button v-auth="['platform:middleware:configure']" type="primary" @click="openCreate">
        {{ t('crud.add') }}
      </el-button>
    </div>

    <el-form :model="query" inline>
      <el-form-item :label="t('baseMiddleware.name')">
        <el-input v-model="query.name" clearable />
      </el-form-item>
      <el-form-item :label="t('baseMiddleware.messageType')">
        <el-select v-model="query.message_type" clearable filterable class="middleware-filter">
          <el-option
            v-for="item in props.registry.message_types"
            :key="item.message_type"
            :label="item.message_type"
            :value="item.message_type"
          />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('baseMiddleware.adapter')">
        <el-select v-model="query.adapter_id" clearable class="middleware-filter">
          <el-option v-for="item in props.adapters" :key="item.id" :label="item.name" :value="item.id" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('baseMiddleware.routeStatus')">
        <el-select v-model="query.status" clearable class="middleware-filter">
          <el-option label="DRAFT" value="DRAFT" />
          <el-option label="PUBLISHED" value="PUBLISHED" />
        </el-select>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="loadRows">
          {{ t('crud.search') }}
        </el-button>
        <el-button @click="resetQuery">
          {{ t('crud.reset') }}
        </el-button>
      </el-form-item>
    </el-form>

    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="name" :label="t('baseMiddleware.name')" min-width="170" />
      <el-table-column prop="message_type" :label="t('baseMiddleware.messageType')" min-width="210" />
      <el-table-column :label="t('baseMiddleware.adapter')" min-width="180">
        <template #default="{ row }">
          {{ adapterName(row.adapter_id) }}
        </template>
      </el-table-column>
      <el-table-column prop="destination" :label="t('baseMiddleware.destination')" min-width="160" />
      <el-table-column prop="consumption_mode" :label="t('baseMiddleware.mode')" width="120" />
      <el-table-column prop="concurrency" :label="t('baseMiddleware.concurrency')" width="100" />
      <el-table-column :label="t('baseMiddleware.ordering')" width="100">
        <template #default="{ row }">
          <el-tag :type="row.ordering_enabled ? 'success' : 'info'">
            {{ row.ordering_enabled ? t('baseMiddleware.yes') : t('baseMiddleware.no') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="status" :label="t('baseMiddleware.routeStatus')" width="120" />
      <el-table-column :label="t('crud.status')" width="100">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">
            {{ row.enabled ? t('baseMiddleware.enabled') : t('baseMiddleware.disabled') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="version" :label="t('baseMiddleware.version')" width="90" />
      <el-table-column :label="t('crud.operation')" width="330" fixed="right">
        <template #default="{ row }">
          <el-button v-auth="['platform:middleware:configure']" type="primary" link @click="openEdit(row)">
            {{ t('crud.edit') }}
          </el-button>
          <el-button v-auth="['platform:middleware:configure']" link @click="showValidation(row)">
            {{ t('baseMiddleware.validate') }}
          </el-button>
          <el-button v-auth="['platform:middleware:publish']" type="success" link @click="publish(row)">
            {{ t('baseMiddleware.publish') }}
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

    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      class="middleware-pagination"
      layout="total, sizes, prev, pager, next"
      :total="total"
      @change="loadRows"
    />

    <el-drawer v-model="dialogVisible" :title="form.id ? t('crud.edit') : t('crud.add')" size="680px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="132px">
        <el-form-item :label="t('baseMiddleware.name')" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.messageType')" prop="message_type">
          <el-select v-model="form.message_type" filterable class="w-full">
            <el-option
              v-for="item in props.registry.message_types"
              :key="item.message_type"
              :label="`${item.message_type} - ${item.description}`"
              :value="item.message_type"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.adapter')" prop="adapter_id">
          <el-select v-model="form.adapter_id" class="w-full">
            <el-option
              v-for="item in props.adapters"
              :key="item.id"
              :label="`${item.name} (${item.health_status})`"
              :value="item.id"
              :disabled="!item.enabled"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.destination')" prop="destination">
          <el-input v-model="form.destination" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.mode')">
          <el-segmented
            v-model="form.consumption_mode"
            :options="[
              { label: 'CLUSTER', value: 'CLUSTER' },
              { label: 'BROADCAST', value: 'BROADCAST' },
            ]"
          />
        </el-form-item>
        <el-form-item v-if="form.consumption_mode === 'CLUSTER'" :label="t('baseMiddleware.consumerGroup')">
          <el-input v-model="form.consumer_group" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.concurrency')">
          <el-input-number v-model="form.concurrency" :min="1" :max="1000" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.ordering')">
          <el-switch
            v-model="form.ordering_enabled"
            @change="form.ordering_enabled && (form.concurrency = 1)"
          />
        </el-form-item>
        <el-divider content-position="left">
          {{ t('baseMiddleware.retryPolicy') }}
        </el-divider>
        <el-form-item :label="t('baseMiddleware.maxAttempts')">
          <el-input-number v-model="form.retry_policy.max_attempts" :min="1" :max="100" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.initialDelay')">
          <el-input-number v-model="form.retry_policy.initial_delay_seconds" :min="1" :max="86400" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.maxDelay')">
          <el-input-number v-model="form.retry_policy.max_delay_seconds" :min="1" :max="86400" />
        </el-form-item>
        <el-divider content-position="left">
          {{ t('baseMiddleware.deadLetterPolicy') }}
        </el-divider>
        <el-form-item :label="t('baseMiddleware.deadLetterDestination')">
          <el-input v-model="form.dead_letter_policy.destination" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.retentionDays')">
          <el-input-number v-model="form.dead_letter_policy.retention_days" :min="1" :max="3650" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.alertEnabled')">
          <el-switch v-model="form.dead_letter_policy.alert_enabled" />
        </el-form-item>
        <el-form-item :label="t('crud.status')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">
          {{ t('crud.cancel') }}
        </el-button>
        <el-button type="primary" :loading="saving" @click="submit">
          {{ t('crud.ok') }}
        </el-button>
      </template>
    </el-drawer>

    <el-dialog v-model="validationVisible" :title="t('baseMiddleware.validationResult')" width="560px">
      <el-result
        :icon="validation.valid ? 'success' : 'error'"
        :title="validation.valid ? t('baseMiddleware.validationPassed') : t('baseMiddleware.validationFailed')"
      />
      <el-alert
        v-for="item in validation.errors"
        :key="item"
        class="middleware-alert"
        type="error"
        :title="item"
        :closable="false"
      />
      <el-alert
        v-for="item in validation.warnings"
        :key="item"
        class="middleware-alert"
        type="warning"
        :title="item"
        :closable="false"
      />
    </el-dialog>
  </div>
</template>
