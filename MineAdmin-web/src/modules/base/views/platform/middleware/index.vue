<script setup lang="ts">
import type {
  MiddlewareAdapterVo,
  MiddlewareRegistryVo,
} from '~/base/api/platformMiddleware'
import { adapters, registry } from '~/base/api/platformMiddleware'
import AdapterTab from './components/AdapterTab.vue'
import DeadLetterTab from './components/DeadLetterTab.vue'
import DeliveryTab from './components/DeliveryTab.vue'
import MetricsTab from './components/MetricsTab.vue'
import ProtectionTab from './components/ProtectionTab.vue'
import RouteTab from './components/RouteTab.vue'

defineOptions({ name: 'platform:middleware' })

const t = useTrans().globalTrans
const activeTab = ref('adapters')
const loading = ref(false)
const adapterRows = ref<MiddlewareAdapterVo[]>([])
const registryData = ref<MiddlewareRegistryVo>({ message_types: [], consumers: [] })

async function loadRegistry() {
  const response = await registry()
  registryData.value = response.data ?? { message_types: [], consumers: [] }
}

async function loadAdapters() {
  const response = await adapters()
  adapterRows.value = response.data ?? []
}

async function refreshShared() {
  loading.value = true
  try {
    await Promise.all([loadRegistry(), loadAdapters()])
  }
  finally {
    loading.value = false
  }
}

onMounted(refreshShared)
</script>

<template>
  <div class="mine-layout middleware-page">
    <div class="middleware-header">
      <div>
        <h2>{{ t('baseMiddleware.title') }}</h2>
        <p>{{ t('baseMiddleware.subtitle') }}</p>
      </div>
      <el-button :loading="loading" @click="refreshShared">
        {{ t('baseMiddleware.refresh') }}
      </el-button>
    </div>

    <el-tabs v-model="activeTab" class="middleware-tabs">
      <el-tab-pane :label="t('baseMiddleware.adapters')" name="adapters">
        <AdapterTab :rows="adapterRows" :loading="loading" @refresh="loadAdapters" />
      </el-tab-pane>
      <el-tab-pane :label="t('baseMiddleware.routes')" name="routes">
        <RouteTab :registry="registryData" :adapters="adapterRows" />
      </el-tab-pane>
      <el-tab-pane :label="t('baseMiddleware.deliveries')" name="deliveries">
        <DeliveryTab :registry="registryData" />
      </el-tab-pane>
      <el-tab-pane :label="t('baseMiddleware.deadLetters')" name="dead-letters">
        <DeadLetterTab :registry="registryData" />
      </el-tab-pane>
      <el-tab-pane :label="t('baseMiddleware.protectionRules')" name="protection">
        <ProtectionTab />
      </el-tab-pane>
      <el-tab-pane :label="t('baseMiddleware.metrics')" name="metrics">
        <MetricsTab />
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style src="./middleware.scss" lang="scss"></style>
