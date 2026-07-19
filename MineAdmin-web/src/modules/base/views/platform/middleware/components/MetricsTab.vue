<script setup lang="ts">
import type { MiddlewareMetricsVo } from '~/base/api/platformMiddleware'
import { metrics } from '~/base/api/platformMiddleware'

const t = useTrans().globalTrans
const loading = ref(false)
const data = ref<MiddlewareMetricsVo>({
  message: { adapters: [], deliveries: [], dead_letters: [] },
  outbox: {
    FailedJobs: 0,
    OutboxPending: 0,
    OutboxProcessing: 0,
    OutboxFailed: 0,
    OutboxSent: 0,
    Classes: [],
  },
  protection: [],
})

const summaries = computed(() => [
  { label: t('baseMiddleware.adapterInstances'), value: data.value.message.adapters.reduce((sum, item) => sum + item.count, 0) },
  { label: t('baseMiddleware.deliveryCount'), value: data.value.message.deliveries.reduce((sum, item) => sum + item.count, 0) },
  { label: t('baseMiddleware.deadLetterCount'), value: data.value.message.dead_letters.reduce((sum, item) => sum + item.count, 0) },
  { label: t('baseMiddleware.outboxPending'), value: data.value.outbox.OutboxPending },
  { label: t('baseMiddleware.outboxFailed'), value: data.value.outbox.OutboxFailed },
  { label: t('baseMiddleware.protectionRuleCount'), value: data.value.protection.length },
])

async function loadData() {
  loading.value = true
  try {
    const response = await metrics()
    data.value = response.data ?? data.value
  }
  finally {
    loading.value = false
  }
}

function averageDuration(duration: number, count: number) {
  return count > 0 ? `${(duration / count).toFixed(2)}ms` : '-'
}

onMounted(loadData)
</script>

<template>
  <div v-loading="loading" class="middleware-section">
    <div class="middleware-section-toolbar">
      <div>
        <h3>{{ t('baseMiddleware.metrics') }}</h3>
        <p>{{ t('baseMiddleware.metricsHint') }}</p>
      </div>
      <el-button type="primary" @click="loadData">
        {{ t('baseMiddleware.refresh') }}
      </el-button>
    </div>

    <el-alert
      :title="t('baseMiddleware.metricsSourceHint')"
      type="info"
      :closable="false"
      show-icon
    />
    <div class="middleware-metric-grid">
      <div v-for="item in summaries" :key="item.label" class="middleware-metric">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </div>
    </div>

    <div class="middleware-table-grid">
      <section>
        <div class="middleware-subtitle">
          {{ t('baseMiddleware.adapterHealthMetrics') }}
        </div>
        <el-table :data="data.message.adapters" border>
          <el-table-column prop="adapter_type" :label="t('baseMiddleware.adapterType')" min-width="160" />
          <el-table-column prop="health_status" :label="t('baseMiddleware.health')" width="120" />
          <el-table-column prop="count" :label="t('baseMiddleware.count')" width="100" />
        </el-table>
      </section>
      <section>
        <div class="middleware-subtitle">
          {{ t('baseMiddleware.outbox') }}
        </div>
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="t('baseMiddleware.outboxPending')">
            {{ data.outbox.OutboxPending }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.outboxProcessing')">
            {{ data.outbox.OutboxProcessing }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.outboxFailed')">
            {{ data.outbox.OutboxFailed }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.outboxSent')">
            {{ data.outbox.OutboxSent }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.failedJobs')" :span="2">
            {{ data.outbox.FailedJobs }}
          </el-descriptions-item>
        </el-descriptions>
      </section>
    </div>

    <div class="middleware-subtitle">
      {{ t('baseMiddleware.deliveryMetrics') }}
    </div>
    <el-table :data="data.message.deliveries" border>
      <el-table-column prop="message_type" :label="t('baseMiddleware.messageType')" min-width="190" />
      <el-table-column prop="consumer_key" :label="t('baseMiddleware.consumer')" min-width="180" />
      <el-table-column prop="status" :label="t('baseMiddleware.deliveryStatus')" width="150" />
      <el-table-column prop="count" :label="t('baseMiddleware.count')" width="100" />
      <el-table-column :label="t('baseMiddleware.averageDuration')" width="150">
        <template #default="{ row }">
          {{ averageDuration(row.duration_sum_ms, row.count) }}
        </template>
      </el-table-column>
    </el-table>

    <div class="middleware-subtitle">
      {{ t('baseMiddleware.protectionMetrics') }}
    </div>
    <el-table :data="data.protection" border>
      <el-table-column prop="rule_set_id" label="ID" width="80" />
      <el-table-column prop="resource_pattern" :label="t('baseMiddleware.resourcePattern')" min-width="180" />
      <el-table-column prop="passed" :label="t('baseMiddleware.passed')" width="100" />
      <el-table-column prop="rate_limited" :label="t('baseMiddleware.rateLimited')" width="120" />
      <el-table-column prop="circuit_rejected" :label="t('baseMiddleware.circuitRejected')" width="130" />
      <el-table-column prop="concurrency_rejected" :label="t('baseMiddleware.concurrencyRejected')" width="150" />
      <el-table-column prop="calls" :label="t('baseMiddleware.calls')" width="100" />
      <el-table-column prop="failures" :label="t('baseMiddleware.failures')" width="100" />
      <el-table-column :label="t('baseMiddleware.averageDuration')" width="150">
        <template #default="{ row }">
          {{ averageDuration(row.duration_sum_ms, row.calls) }}
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>
