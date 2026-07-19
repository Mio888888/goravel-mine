<script setup lang="ts">
import type { MessageDeadLetterVo, MiddlewareRegistryVo } from '~/base/api/platformMiddleware'
import {
  deadLetterDetail,
  deadLetters,
  replayDeadLetter,
  resolveDeadLetter,
} from '~/base/api/platformMiddleware'
import hasAuth from '@/utils/permission/hasAuth.ts'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

const props = defineProps<{ registry: MiddlewareRegistryVo }>()
const t = useTrans().globalTrans
const msg = useMessage()
const loading = ref(false)
const detailLoading = ref(false)
const detailVisible = ref(false)
const activeDetail = ref<MessageDeadLetterVo | null>(null)
const rows = ref<MessageDeadLetterVo[]>([])
const total = ref(0)
const query = reactive({
  message_id: '',
  message_type: '',
  consumer_key: '',
  failure_class: '',
  resolution_status: '',
  page: 1,
  page_size: 15,
})

async function loadRows() {
  loading.value = true
  try {
    const response = await deadLetters(query)
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
    failure_class: '',
    resolution_status: '',
    page: 1,
  })
  loadRows()
}

async function showDetail(row: any) {
  detailVisible.value = true
  activeDetail.value = null
  detailLoading.value = true
  try {
    const response = await deadLetterDetail(row.id)
    if (response.code === ResultCode.SUCCESS) {
      activeDetail.value = response.data
    }
  }
  finally {
    detailLoading.value = false
  }
}

async function replay(row: any) {
  try {
    await msg.confirm(t('baseMiddleware.replayConfirm'))
  }
  catch {
    return
  }
  const key = globalThis.crypto?.randomUUID?.()
    ?? `dead-letter-${row.id}-${Date.now()}-${Math.random().toString(16).slice(2)}`
  const response = await replayDeadLetter(row.id, key)
  if (response.code === ResultCode.SUCCESS) {
    msg.success(t('baseMiddleware.replayQueued'))
    await loadRows()
  }
}

async function resolve(row: any) {
  try {
    await msg.confirm(t('baseMiddleware.resolveConfirm'))
  }
  catch {
    return
  }
  const response = await resolveDeadLetter(row.id)
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
        <h3>{{ t('baseMiddleware.deadLetters') }}</h3>
        <p>{{ t('baseMiddleware.deadLettersHint') }}</p>
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
      <el-form-item :label="t('baseMiddleware.failureClass')">
        <el-select v-model="query.failure_class" clearable class="middleware-filter">
          <el-option v-for="item in ['RETRYABLE', 'NON_RETRYABLE', 'UNKNOWN_RESULT']" :key="item" :label="item" :value="item" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('baseMiddleware.resolutionStatus')">
        <el-select v-model="query.resolution_status" clearable class="middleware-filter">
          <el-option label="OPEN" value="OPEN" />
          <el-option label="RESOLVED" value="RESOLVED" />
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
      <el-table-column prop="consumer_key" :label="t('baseMiddleware.consumer')" min-width="180" />
      <el-table-column prop="failure_class" :label="t('baseMiddleware.failureClass')" width="160" />
      <el-table-column prop="resolution_status" :label="t('baseMiddleware.resolutionStatus')" width="140">
        <template #default="{ row }">
          <el-tag :type="row.resolution_status === 'OPEN' ? 'danger' : 'success'">
            {{ row.resolution_status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="replay_count" :label="t('baseMiddleware.replayCount')" width="110" />
      <el-table-column prop="last_failed_at" :label="t('baseMiddleware.lastFailedAt')" width="180" />
      <el-table-column prop="error_summary" :label="t('baseMiddleware.errorSummary')" min-width="240" show-overflow-tooltip />
      <el-table-column :label="t('crud.operation')" width="250" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="hasAuth('platform:middleware:payload')"
            type="primary"
            link
            @click="showDetail(row)"
          >
            {{ t('crud.detail') }}
          </el-button>
          <el-button
            v-auth="['platform:middleware:replay']"
            type="warning"
            link
            @click="replay(row)"
          >
            {{ t('baseMiddleware.replay') }}
          </el-button>
          <el-button
            v-if="row.resolution_status === 'OPEN'"
            v-auth="['platform:middleware:replay']"
            type="success"
            link
            @click="resolve(row)"
          >
            {{ t('baseMiddleware.resolve') }}
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

    <el-drawer v-model="detailVisible" :title="t('baseMiddleware.deadLetterDetail')" size="700px">
      <div v-loading="detailLoading">
        <el-descriptions v-if="activeDetail" :column="2" border>
          <el-descriptions-item :label="t('baseMiddleware.messageId')" :span="2">
            {{ activeDetail.message_id }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.messageType')">
            {{ activeDetail.message_type }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.consumer')">
            {{ activeDetail.consumer_key }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.failureClass')">
            {{ activeDetail.failure_class }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.resolutionStatus')">
            {{ activeDetail.resolution_status }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('baseMiddleware.errorSummary')" :span="2">
            {{ activeDetail.error_summary }}
          </el-descriptions-item>
        </el-descriptions>
        <div v-if="activeDetail" class="middleware-payload">
          <div class="middleware-subtitle">
            {{ t('baseMiddleware.redactedEnvelope') }}
          </div>
          <pre>{{ JSON.stringify(activeDetail.envelope ?? {}, null, 2) }}</pre>
        </div>
      </div>
    </el-drawer>
  </div>
</template>
