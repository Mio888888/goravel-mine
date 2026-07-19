<script setup lang="ts">
import type { FormInstance } from 'element-plus'
import type {
  ProtectionRuleSetVo,
  ProtectionRuleType,
  ProtectionRuleVersionVo,
  ProtectionRuleVo,
} from '~/base/api/platformMiddleware'
import {
  createProtectionRule,
  deleteProtectionRule,
  protectionRuleDetail,
  protectionRules,
  protectionRuleState,
  protectionRuleVersions,
  publishProtectionRule,
  rollbackProtectionRule,
  setProtectionRuleEnabled,
  updateProtectionRule,
  validateProtectionRule,
} from '~/base/api/platformMiddleware'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

const t = useTrans().globalTrans
const msg = useMessage()
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const versionsVisible = ref(false)
const stateVisible = ref(false)
const activeRuleSet = ref<ProtectionRuleSetVo | null>(null)
const versionRows = ref<ProtectionRuleVersionVo[]>([])
const stateData = ref<any>(null)
const formRef = ref<FormInstance>()
const rows = ref<ProtectionRuleSetVo[]>([])
const total = ref(0)
const query = reactive({
  name: '',
  scope: '',
  status: '',
  page: 1,
  page_size: 15,
})
const form = reactive<ProtectionRuleSetVo>({
  name: '',
  scope: 'GLOBAL',
  resource_pattern: '*',
  rules: { rules: [] },
  enabled: true,
  version: 1,
})

const ruleTypeOptions: Array<{ label: string, value: ProtectionRuleType }> = [
  { label: t('baseMiddleware.rateLimit'), value: 'RATE_LIMIT' },
  { label: t('baseMiddleware.slowCallCircuit'), value: 'SLOW_CALL_CIRCUIT' },
  { label: t('baseMiddleware.failureRateCircuit'), value: 'FAILURE_RATE_CIRCUIT' },
  { label: t('baseMiddleware.concurrencyIsolation'), value: 'CONCURRENCY' },
]

const rules = {
  name: [{ required: true, message: t('baseMiddleware.ruleSetNameRequired'), trigger: 'blur' }],
  scope: [{ required: true, message: t('baseMiddleware.scopeRequired'), trigger: 'change' }],
  resource_pattern: [{ required: true, message: t('baseMiddleware.resourcePatternRequired'), trigger: 'blur' }],
}

async function loadRows() {
  loading.value = true
  try {
    const response = await protectionRules(query)
    rows.value = response.data?.list ?? []
    total.value = response.data?.total ?? 0
  }
  finally {
    loading.value = false
  }
}

function resetQuery() {
  Object.assign(query, { name: '', scope: '', status: '', page: 1 })
  loadRows()
}

function resetForm() {
  Object.assign(form, {
    id: undefined,
    name: '',
    scope: 'GLOBAL',
    resource_pattern: '*',
    rules: { rules: [] },
    enabled: true,
    version: 1,
    published_version: 0,
  })
}

function openCreate() {
  resetForm()
  addRule('RATE_LIMIT')
  dialogVisible.value = true
}

async function openEdit(row: any) {
  const response = await protectionRuleDetail(row.id as number)
  if (response.code === ResultCode.SUCCESS) {
    Object.assign(form, structuredClone(response.data))
    dialogVisible.value = true
  }
}

function defaultRule(type: ProtectionRuleType): ProtectionRuleVo {
  if (type === 'RATE_LIMIT') {
    return { type, limit: 100, window_ms: 1000 }
  }
  if (type === 'CONCURRENCY') {
    return { type, max_concurrency: 20 }
  }
  return {
    type,
    slow_call_duration_ms: type === 'SLOW_CALL_CIRCUIT' ? 1000 : undefined,
    threshold_percent: 50,
    minimum_requests: 20,
    statistical_window_ms: 60000,
    open_duration_ms: 30000,
    half_open_probes: 5,
    half_open_successes: 3,
  }
}

function addRule(type: ProtectionRuleType) {
  if (form.rules.rules.some(item => item.type === type)) {
    msg.warning(t('baseMiddleware.duplicateRuleType'))
    return
  }
  form.rules.rules.push(defaultRule(type))
}

function handleScopeChange() {
  if (form.scope === 'GLOBAL') {
    form.resource_pattern = '*'
  }
  else if (form.resource_pattern === '*') {
    form.resource_pattern = ''
  }
}

async function submit() {
  try {
    await formRef.value?.validate()
  }
  catch {
    return
  }
  if (form.rules.rules.length === 0) {
    msg.error(t('baseMiddleware.ruleRequired'))
    return
  }
  saving.value = true
  try {
    const response = form.id
      ? await updateProtectionRule(form.id, form)
      : await createProtectionRule(form)
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

async function validateRow(row: any) {
  const response = await validateProtectionRule(row.id as number)
  if (response.code === ResultCode.SUCCESS) {
    response.data.valid
      ? msg.alertSuccess(t('baseMiddleware.validationPassed'))
      : msg.alertError(response.data.errors.join('\n'))
  }
}

async function publish(row: any) {
  try {
    await msg.confirm(t('baseMiddleware.publishRuleConfirm'))
  }
  catch {
    return
  }
  const response = await publishProtectionRule(row.id as number, row.version as number, crypto.randomUUID())
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('baseMiddleware.published'))
    await loadRows()
  }
}

async function toggle(row: any) {
  try {
    await msg.confirm(row.enabled
      ? t('baseMiddleware.disableRuleConfirm')
      : t('baseMiddleware.enableRuleConfirm'))
  }
  catch {
    return
  }
  const response = await setProtectionRuleEnabled(row.id as number, !row.enabled, row.version as number)
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('crud.updateSuccess'))
    await loadRows()
  }
}

async function remove(row: any) {
  try {
    await msg.delConfirm(t('crud.delMessage'))
  }
  catch {
    return
  }
  const response = await deleteProtectionRule(row.id as number, row.version as number)
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('crud.delSuccess'))
    await loadRows()
  }
}

async function showVersions(row: any) {
  const response = await protectionRuleVersions(row.id as number)
  if (response.code === ResultCode.SUCCESS) {
    activeRuleSet.value = row
    versionRows.value = response.data ?? []
    versionsVisible.value = true
  }
}

async function rollback(version: any) {
  if (!activeRuleSet.value) {
    return
  }
  try {
    await msg.confirm(t('baseMiddleware.rollbackConfirm', { version: version.version }))
  }
  catch {
    return
  }
  const response = await rollbackProtectionRule(
    activeRuleSet.value.id as number,
    activeRuleSet.value.version as number,
    version.version,
  )
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('baseMiddleware.rollbackSucceeded'))
    versionsVisible.value = false
    await loadRows()
  }
}

async function showState(row: any) {
  const response = await protectionRuleState(row.id as number)
  if (response.code === ResultCode.SUCCESS) {
    activeRuleSet.value = row
    stateData.value = response.data
    stateVisible.value = true
  }
}

onMounted(loadRows)
</script>

<template>
  <div class="middleware-section">
    <div class="middleware-section-toolbar">
      <div>
        <h3>{{ t('baseMiddleware.protectionRules') }}</h3>
        <p>{{ t('baseMiddleware.protectionHint') }}</p>
      </div>
      <el-button v-auth="['platform:middleware:configure']" type="primary" @click="openCreate">
        {{ t('crud.add') }}
      </el-button>
    </div>

    <el-form :model="query" inline>
      <el-form-item :label="t('baseMiddleware.name')">
        <el-input v-model="query.name" clearable />
      </el-form-item>
      <el-form-item :label="t('baseMiddleware.scope')">
        <el-select v-model="query.scope" clearable class="middleware-filter">
          <el-option v-for="item in ['GLOBAL', 'SERVICE', 'ENDPOINT', 'CUSTOM']" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('baseMiddleware.routeStatus')">
        <el-select v-model="query.status" clearable class="middleware-filter">
          <el-option v-for="item in ['DRAFT', 'PUBLISHED', 'ARCHIVED']" :key="item" :label="item" :value="item" />
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
      <el-table-column prop="name" :label="t('baseMiddleware.name')" min-width="180" />
      <el-table-column prop="scope" :label="t('baseMiddleware.scope')" width="120" />
      <el-table-column prop="resource_pattern" :label="t('baseMiddleware.resourcePattern')" min-width="190" />
      <el-table-column :label="t('baseMiddleware.ruleTypes')" min-width="260">
        <template #default="{ row }">
          <el-space wrap>
            <el-tag v-for="rule in row.rules?.rules ?? []" :key="rule.type" type="info">
              {{ rule.type }}
            </el-tag>
          </el-space>
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
      <el-table-column prop="published_version" :label="t('baseMiddleware.publishedVersion')" width="120" />
      <el-table-column prop="version" :label="t('baseMiddleware.version')" width="90" />
      <el-table-column :label="t('crud.operation')" width="450" fixed="right">
        <template #default="{ row }">
          <el-button v-auth="['platform:middleware:configure']" type="primary" link @click="openEdit(row)">
            {{ t('crud.edit') }}
          </el-button>
          <el-button v-auth="['platform:middleware:configure']" link @click="validateRow(row)">
            {{ t('baseMiddleware.validate') }}
          </el-button>
          <el-button v-auth="['platform:middleware:publish']" type="success" link @click="publish(row)">
            {{ t('baseMiddleware.publish') }}
          </el-button>
          <el-button v-auth="['platform:middleware:list']" link @click="showVersions(row)">
            {{ t('baseMiddleware.versions') }}
          </el-button>
          <el-button v-auth="['platform:middleware:list']" link @click="showState(row)">
            {{ t('baseMiddleware.runtimeState') }}
          </el-button>
          <el-button
            v-auth="['platform:middleware:publish']"
            :type="row.enabled ? 'danger' : 'success'"
            link
            @click="toggle(row)"
          >
            {{ row.enabled ? t('baseMiddleware.disable') : t('baseMiddleware.enable') }}
          </el-button>
          <el-button
            v-if="!row.published_version"
            v-auth="['platform:middleware:configure']"
            type="danger"
            link
            @click="remove(row)"
          >
            {{ t('crud.delete') }}
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

    <el-drawer v-model="dialogVisible" :title="form.id ? t('crud.edit') : t('crud.add')" size="760px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="132px">
        <el-form-item :label="t('baseMiddleware.name')" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.scope')" prop="scope">
          <el-segmented
            v-model="form.scope"
            :options="['GLOBAL', 'SERVICE', 'ENDPOINT', 'CUSTOM']"
            @change="handleScopeChange"
          />
        </el-form-item>
        <el-form-item :label="t('baseMiddleware.resourcePattern')" prop="resource_pattern">
          <el-input v-model="form.resource_pattern" :disabled="form.scope === 'GLOBAL'" />
        </el-form-item>
        <el-form-item :label="t('crud.status')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-divider content-position="left">
          {{ t('baseMiddleware.rules') }}
        </el-divider>
        <div class="middleware-rule-actions">
          <el-dropdown trigger="click">
            <el-button type="primary" plain>
              {{ t('baseMiddleware.addRule') }}
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  v-for="item in ruleTypeOptions"
                  :key="item.value"
                  @click="addRule(item.value)"
                >
                  {{ item.label }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
        <div v-for="(rule, index) in form.rules.rules" :key="rule.type" class="middleware-rule-row">
          <div class="middleware-rule-header">
            <strong>{{ ruleTypeOptions.find(item => item.value === rule.type)?.label }}</strong>
            <el-button type="danger" link @click="form.rules.rules.splice(index, 1)">
              {{ t('crud.delete') }}
            </el-button>
          </div>
          <div v-if="rule.type === 'RATE_LIMIT'" class="middleware-rule-grid">
            <el-form-item :label="t('baseMiddleware.limit')">
              <el-input-number v-model="rule.limit" :min="1" :max="1000000000" />
            </el-form-item>
            <el-form-item :label="t('baseMiddleware.windowMs')">
              <el-input-number v-model="rule.window_ms" :min="100" :max="86400000" />
            </el-form-item>
          </div>
          <div v-else-if="rule.type === 'CONCURRENCY'" class="middleware-rule-grid">
            <el-form-item :label="t('baseMiddleware.maxConcurrency')">
              <el-input-number v-model="rule.max_concurrency" :min="1" :max="1000000" />
            </el-form-item>
          </div>
          <div v-else class="middleware-rule-grid">
            <el-form-item v-if="rule.type === 'SLOW_CALL_CIRCUIT'" :label="t('baseMiddleware.slowDurationMs')">
              <el-input-number v-model="rule.slow_call_duration_ms" :min="1" :max="86400000" />
            </el-form-item>
            <el-form-item :label="t('baseMiddleware.thresholdPercent')">
              <el-input-number v-model="rule.threshold_percent" :min="1" :max="100" />
            </el-form-item>
            <el-form-item :label="t('baseMiddleware.minimumRequests')">
              <el-input-number v-model="rule.minimum_requests" :min="1" :max="1000000" />
            </el-form-item>
            <el-form-item :label="t('baseMiddleware.statisticalWindowMs')">
              <el-input-number v-model="rule.statistical_window_ms" :min="1000" :max="86400000" />
            </el-form-item>
            <el-form-item :label="t('baseMiddleware.openDurationMs')">
              <el-input-number v-model="rule.open_duration_ms" :min="100" :max="86400000" />
            </el-form-item>
            <el-form-item :label="t('baseMiddleware.halfOpenProbes')">
              <el-input-number v-model="rule.half_open_probes" :min="1" :max="1000" />
            </el-form-item>
            <el-form-item :label="t('baseMiddleware.halfOpenSuccesses')">
              <el-input-number v-model="rule.half_open_successes" :min="1" :max="1000" />
            </el-form-item>
          </div>
        </div>
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

    <el-drawer v-model="versionsVisible" :title="t('baseMiddleware.versionHistory')" size="680px">
      <el-table :data="versionRows" border>
        <el-table-column prop="version" :label="t('baseMiddleware.version')" width="90" />
        <el-table-column prop="name" :label="t('baseMiddleware.name')" min-width="160" />
        <el-table-column prop="scope" :label="t('baseMiddleware.scope')" width="110" />
        <el-table-column prop="resource_pattern" :label="t('baseMiddleware.resourcePattern')" min-width="180" />
        <el-table-column prop="enabled" :label="t('crud.status')" width="100">
          <template #default="{ row }">
            {{ row.enabled ? t('baseMiddleware.enabled') : t('baseMiddleware.disabled') }}
          </template>
        </el-table-column>
        <el-table-column prop="published_at" :label="t('baseMiddleware.publishedAt')" width="180" />
        <el-table-column :label="t('crud.operation')" width="110">
          <template #default="{ row }">
            <el-button v-auth="['platform:middleware:publish']" type="warning" link @click="rollback(row)">
              {{ t('baseMiddleware.rollback') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>

    <el-drawer v-model="stateVisible" :title="t('baseMiddleware.runtimeState')" size="620px">
      <el-descriptions v-if="stateData" :column="2" border>
        <el-descriptions-item :label="t('baseMiddleware.version')">
          {{ stateData.version }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('baseMiddleware.currentConcurrency')">
          {{ stateData.concurrent }}
        </el-descriptions-item>
      </el-descriptions>
      <el-table v-if="stateData" :data="stateData.circuits ?? []" border class="middleware-state-table">
        <el-table-column prop="rule_type" :label="t('baseMiddleware.ruleTypes')" min-width="190" />
        <el-table-column prop="state" :label="t('baseMiddleware.runtimeState')" width="120" />
        <el-table-column prop="sample_count" :label="t('baseMiddleware.samples')" width="100" />
        <el-table-column prop="failure_count" :label="t('baseMiddleware.failures')" width="100" />
        <el-table-column prop="slow_count" :label="t('baseMiddleware.slowCalls')" width="100" />
      </el-table>
    </el-drawer>
  </div>
</template>
