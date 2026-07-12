<script setup lang="ts">
import type { ModuleLifecycleStateVo } from '~/base/api/platformModuleLifecycle'
import { useI18n } from 'vue-i18n'
import { actionLabel, boolLabel, statusType } from '../presentation'

defineProps<{ rows: ModuleLifecycleStateVo[] }>()
const { t } = useI18n()
const showAction = (action?: string) => actionLabel({ action, t })
const showBool = (value?: boolean) => boolLabel({ value, t })
</script>

<template>
  <div class="module-lifecycle-section">
    <el-table :data="rows" border>
      <el-table-column prop="id" :label="t('baseModuleLifecycle.moduleId')" min-width="180" show-overflow-tooltip />
      <el-table-column prop="name" :label="t('baseModuleLifecycle.name')" min-width="180" show-overflow-tooltip />
      <el-table-column prop="version" :label="t('baseModuleLifecycle.version')" width="110" />
      <el-table-column :label="t('baseModuleLifecycle.enabled')" width="110">
        <template #default="{ row }">
          <el-tag :type="row.enabled ? 'success' : 'info'">
            {{ showBool(row.enabled) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('baseModuleLifecycle.persistedStatus')" width="140">
        <template #default="{ row }">
          <el-tag :type="statusType(row.persisted?.status)">
            {{ row.persisted?.status || '-' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('baseModuleLifecycle.owner')" min-width="140" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.persisted?.owner || '-' }}
        </template>
      </el-table-column>
      <el-table-column :label="t('baseModuleLifecycle.lastAction')" width="120">
        <template #default="{ row }">
          {{ showAction(row.persisted?.last_action) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('baseModuleLifecycle.lastError')" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">
          {{ row.persisted?.last_error || row.reason || '-' }}
        </template>
      </el-table-column>
      <el-table-column prop="lifecycle.upgrade" :label="t('baseModuleLifecycle.command')" min-width="280" show-overflow-tooltip />
    </el-table>
  </div>
</template>
