<script setup lang="ts">
import type { ObservabilityPanelVo } from '~/base/api/platformObservability'
import { slowRequests } from '~/base/api/platformObservability'
import { ResultCode } from '@/utils/ResultCode.ts'
import { useMessage } from '@/hooks/useMessage.ts'
import { useI18n } from 'vue-i18n'

defineOptions({ name: 'platform:observability' })

const msg = useMessage()
const { t } = useI18n()
const loading = ref(false)
const limit = ref(20)
const data = ref<ObservabilityPanelVo>({
  summary: {
    total_requests: 0,
    inflight: 0,
    route_count: 0,
    slow_count: 0,
    uptime_seconds: 0,
    slow_routes: [],
  },
  slow_requests: [],
  slow_sql: [],
})

function normalizePanel(value?: Partial<ObservabilityPanelVo> | null): ObservabilityPanelVo {
  return {
    ...value,
    summary: {
      ...data.value.summary,
      ...value?.summary,
      slow_routes: Array.isArray(value?.summary?.slow_routes) ? value.summary.slow_routes : [],
    },
    slow_requests: Array.isArray(value?.slow_requests) ? value.slow_requests : [],
    slow_sql: Array.isArray(value?.slow_sql) ? value.slow_sql : [],
  }
}

const limitOptions = computed(() => [20, 50, 100].map(value => ({
  label: t('baseObservability.items', { count: value }),
  value,
})))

const statItems = computed(() => [
  { label: t('baseObservability.totalRequests'), value: data.value.summary.total_requests },
  { label: t('baseObservability.inflight'), value: data.value.summary.inflight },
  { label: t('baseObservability.routeCount'), value: data.value.summary.route_count },
  { label: t('baseObservability.slowCount'), value: data.value.summary.slow_count },
])

async function loadData() {
  loading.value = true
  try {
    const response = await slowRequests(limit.value)
    if (response.code === ResultCode.SUCCESS) {
      data.value = normalizePanel(response.data)
      return
    }
    msg.error(response.message)
  }
  finally {
    loading.value = false
  }
}

function statusType(status: number): 'success' | 'warning' | 'danger' {
  if (status >= 500) {
    return 'danger'
  }
  if (status >= 400) {
    return 'warning'
  }
  return 'success'
}

function durationType(duration: number, threshold: number): 'warning' | 'danger' {
  return duration >= threshold * 2 ? 'danger' : 'warning'
}

function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) {
    return t('baseObservability.uptimeDays', { days, hours })
  }
  if (hours > 0) {
    return t('baseObservability.uptimeHours', { hours, minutes })
  }
  return t('baseObservability.uptimeMinutes', { minutes })
}

onMounted(loadData)
</script>

<template>
  <div v-loading="loading" class="mine-layout observability-page">
    <div class="observability-toolbar">
      <div>
        <h2>{{ t('baseObservability.title') }}</h2>
        <p>{{ t('baseObservability.subtitle') }}</p>
      </div>
      <div class="observability-actions">
        <el-select v-model="limit" class="w-120px" @change="loadData">
          <el-option
            v-for="item in limitOptions"
            :key="item.value"
            :value="item.value"
            :label="item.label"
          />
        </el-select>
        <el-button type="primary" @click="loadData">
          {{ t('baseObservability.refresh') }}
        </el-button>
      </div>
    </div>

    <div class="observability-stats">
      <div v-for="item in statItems" :key="item.label" class="observability-stat">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </div>
      <div class="observability-stat">
        <span>{{ t('baseObservability.uptime') }}</span>
        <strong>{{ formatUptime(data.summary.uptime_seconds) }}</strong>
      </div>
    </div>

    <div class="observability-section">
      <div class="section-title">
        <h3>{{ t('baseObservability.slowRequests') }}</h3>
        <span>{{ t('baseObservability.retained', { count: data.slow_requests.length }) }}</span>
      </div>
      <el-table :data="data.slow_requests" border>
        <el-table-column prop="method" :label="t('baseObservability.method')" width="100" />
        <el-table-column prop="route" :label="t('baseObservability.route')" min-width="220" show-overflow-tooltip />
        <el-table-column prop="status" :label="t('baseObservability.status')" width="100">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="duration_ms" :label="t('baseObservability.duration')" width="130">
          <template #default="{ row }">
            <el-tag :type="durationType(row.duration_ms, row.threshold_ms)">
              {{ row.duration_ms }}ms
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="request_id" :label="t('baseObservability.requestId')" min-width="180" show-overflow-tooltip />
        <el-table-column prop="trace_id" :label="t('baseObservability.traceId')" min-width="220" show-overflow-tooltip />
        <el-table-column prop="ip" :label="t('baseObservability.ip')" width="150" />
        <el-table-column prop="recorded_at" :label="t('baseObservability.recordedAt')" width="210" />
      </el-table>
    </div>

    <div class="observability-grid">
      <div class="observability-section">
        <div class="section-title">
          <h3>{{ t('baseObservability.slowSql') }}</h3>
          <span>{{ t('baseObservability.retained', { count: data.slow_sql.length }) }}</span>
        </div>
        <el-table :data="data.slow_sql" border>
          <el-table-column prop="duration_ms" :label="t('baseObservability.duration')" width="120">
            <template #default="{ row }">
              <el-tag type="warning">
                {{ row.duration_ms }}ms
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="rows" :label="t('baseObservability.rows')" width="90" />
          <el-table-column prop="sql" :label="t('baseObservability.sql')" min-width="260" show-overflow-tooltip />
          <el-table-column prop="trace_id" :label="t('baseObservability.traceId')" min-width="220" show-overflow-tooltip />
        </el-table>
      </div>

      <div class="observability-section">
        <div class="section-title">
          <h3>{{ t('baseObservability.slowRoutes') }}</h3>
          <span>{{ t('baseObservability.routes', { count: data.summary.slow_routes.length }) }}</span>
        </div>
        <el-table :data="data.summary.slow_routes" border>
          <el-table-column prop="route" :label="t('baseObservability.route')" min-width="240" show-overflow-tooltip />
          <el-table-column prop="count" :label="t('baseObservability.count')" width="100" />
        </el-table>
      </div>
    </div>
  </div>
</template>

<style scoped>
.observability-page {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding-top: 12px;
}

.observability-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 16px 18px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
}

.observability-toolbar h2,
.section-title h3 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.observability-toolbar p,
.section-title span,
.observability-stat span {
  margin: 6px 0 0;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.observability-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.observability-stats {
  display: grid;
  grid-template-columns: repeat(5, minmax(120px, 1fr));
  gap: 12px;
}

.observability-stat {
  min-height: 82px;
  padding: 16px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
}

.observability-stat strong {
  display: block;
  margin-top: 10px;
  font-size: 24px;
  line-height: 1;
}

.observability-section {
  min-width: 0;
  padding: 16px;
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
}

.section-title {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.observability-grid {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(280px, 1fr);
  gap: 14px;
}

@media (max-width: 1100px) {
  .observability-stats,
  .observability-grid {
    grid-template-columns: 1fr;
  }

  .observability-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
