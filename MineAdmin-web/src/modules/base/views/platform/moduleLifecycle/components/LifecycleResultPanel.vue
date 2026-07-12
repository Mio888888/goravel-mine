<script setup lang="ts">
import type { ModuleLifecycleResultVo } from '~/base/api/platformModuleLifecycle'
import { useI18n } from 'vue-i18n'
import { actionLabel, statusLabel, statusType } from '../presentation'

defineProps<{ result: ModuleLifecycleResultVo }>()
const { t } = useI18n()
const showAction = (action?: string) => actionLabel({ action, t })
const showStatus = (status?: string) => statusLabel({ status, t })
</script>

<template>
  <div class="module-lifecycle-section">
    <div class="section-title">
      <h3>{{ result.dry_run ? t('baseModuleLifecycle.dryRunResult') : t('baseModuleLifecycle.executeResult') }}</h3>
      <span>{{ showAction(result.action) }}</span>
    </div>
    <el-table :data="result.items" border>
      <el-table-column prop="module_id" :label="t('baseModuleLifecycle.moduleId')" min-width="170" show-overflow-tooltip />
      <el-table-column prop="name" :label="t('baseModuleLifecycle.name')" min-width="170" show-overflow-tooltip />
      <el-table-column :label="t('baseModuleLifecycle.statusLabel')" width="140">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">
            {{ showStatus(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="command" :label="t('baseModuleLifecycle.command')" min-width="260" show-overflow-tooltip />
      <el-table-column prop="destructive_check" :label="t('baseModuleLifecycle.destructiveCheck')" min-width="240" show-overflow-tooltip />
      <el-table-column prop="idempotency_key" :label="t('baseModuleLifecycle.idempotencyKey')" min-width="220" show-overflow-tooltip />
      <el-table-column prop="error" :label="t('baseModuleLifecycle.error')" min-width="220" show-overflow-tooltip />
    </el-table>
  </div>
</template>
