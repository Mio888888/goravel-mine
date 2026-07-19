<script setup lang="ts">
import type { MessageDeliveryVo, MiddlewareRegistryVo } from '~/base/api/platformMiddleware'
import { deliveries } from '~/base/api/platformMiddleware'

const props = defineProps<{ registry: MiddlewareRegistryVo }>()
const t = useTrans().globalTrans
const loading = ref(false)
const rows = ref<MessageDeliveryVo[]>([])
const total = ref(0)
const query = reactive({
  message_id: '',
  message_type: '',
  consumer_key: '',
  status: '',
  page: 1,
  page_size: 15,
})

async function loadRows() {
  loading.value = true
  try {
    const response = await deliveries(query)
    rows.value = response.data?.list ?? []
    total.value = response.data?.total ?? 0
  }
  finally {
    loading.value = false
  }
}

function resetQuery() {
  Object.assign(query, {
    message_id: '',
    message_type: '',
    consumer_key: '',
    status: '',
    page: 1,
  })
  loadRows()
}

function statusType(status: string) {
  return ({
    SUCCEEDED: 'success',
    PROCESSING: 'primary',
    RETRY_SCHEDULED: 'warning',
    DEAD_LETTERED: 'danger',
    IGNORED: 'info',
  } as const)[status] ?? 'info'
}

onMounted(loadRows)
</script>

<template>
  <div class="middleware-section">
    <div class="middleware-section-toolbar">
      <div>
        <h3>{{ t('baseMiddleware.deliveries') }}</h3>
        <p>{{ t('baseMiddleware.deliveriesHint') }}</p>
      </div>
      <el-button @click="loadRows">
        {{ t('baseMiddleware.refresh') }}
      </el-button>
    </div>
    <el-form :model="query" inline>
      <el-form-item :label="t('baseMiddleware.messageId')">
        <el-input v-model="query.message_id" clearable />
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
      <el-form-item :label="t('baseMiddleware.consumer')">
        <el-select v-model="query.consumer_key" clearable filterable class="middleware-filter">
          <el-option
            v-for="item in props.registry.consumers"
            :key="item.consumer_key"
            :label="item.consumer_key"
            :value="item.consumer_key"
          />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('baseMiddleware.deliveryStatus')">
        <el-select v-model="query.status" clearable class="middleware-filter">
          <el-option v-for="item in ['PROCESSING', 'SUCCEEDED', 'RETRY_SCHEDULED', 'DEAD_LETTERED', 'IGNORED']" :key="item" :label="item" :value="item" />
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
      <el-table-column prop="message_id" :label="t('baseMiddleware.messageId')" min-width="240" show-overflow-tooltip />
      <el-table-column prop="message_type" :label="t('baseMiddleware.messageType')" min-width="200" />
      <el-table-column prop="consumer_key" :label="t('baseMiddleware.consumer')" min-width="190" />
      <el-table-column prop="status" :label="t('baseMiddleware.deliveryStatus')" width="150">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">
            {{ row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="attempt" :label="t('baseMiddleware.attempt')" width="90" />
      <el-table-column prop="duration_ms" :label="t('baseMiddleware.duration')" width="110" />
      <el-table-column prop="correlation_id" :label="t('baseMiddleware.correlationId')" min-width="220" show-overflow-tooltip />
      <el-table-column prop="received_at" :label="t('baseMiddleware.receivedAt')" width="180" />
      <el-table-column prop="error_summary" :label="t('baseMiddleware.errorSummary')" min-width="240" show-overflow-tooltip />
    </el-table>

    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      class="middleware-pagination"
      layout="total, sizes, prev, pager, next"
      :total="total"
      @change="loadRows"
    />
  </div>
</template>
