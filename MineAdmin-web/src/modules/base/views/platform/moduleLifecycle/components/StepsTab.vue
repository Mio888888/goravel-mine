<script setup lang="ts">
import type { ModuleLifecycleAction, ModuleLifecycleStepVo } from '~/base/api/platformModuleLifecycle'
import type { LifecycleSelectOption } from '../presentation'
import type { LifecycleStepQuery } from '../useModuleLifecycleState'
import { useI18n } from 'vue-i18n'
import { statusLabel, statusType } from '../presentation'
import LifecycleFilterActions from './LifecycleFilterActions.vue'
import LifecyclePagination from './LifecyclePagination.vue'
import LifecycleQueryFields from './LifecycleQueryFields.vue'

defineProps<{
  rows: ModuleLifecycleStepVo[]
  total: number
  loading: boolean
  moduleOptions: LifecycleSelectOption[]
  actionOptions: LifecycleSelectOption<ModuleLifecycleAction>[]
  statusOptions: LifecycleSelectOption[]
  canViewStepLogs: boolean
}>()
const emit = defineEmits<{ search: [], reset: [], pageChange: [], sizeChange: [] }>()
const query = defineModel<LifecycleStepQuery>('query', { required: true })
const { t } = useI18n()
const showStatus = (status?: string) => statusLabel({ status, t })
</script>

<template>
  <div class="module-lifecycle-section">
    <el-form :model="query" class="module-lifecycle-filter" label-position="top">
      <el-form-item :label="t('baseModuleLifecycle.runKey')">
        <el-input v-model="query.run_key" clearable />
      </el-form-item>
      <LifecycleQueryFields
        v-model:module-id="query.module_id"
        v-model:action="query.action"
        v-model:status="query.status"
        :module-options="moduleOptions"
        :action-options="actionOptions"
        :status-options="statusOptions"
      />
      <LifecycleFilterActions :loading="loading" @search="emit('search')" @reset="emit('reset')" />
    </el-form>

    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="id" label="ID" width="90" />
      <el-table-column prop="run_key" :label="t('baseModuleLifecycle.runKey')" min-width="220" show-overflow-tooltip />
      <el-table-column prop="module_id" :label="t('baseModuleLifecycle.moduleId')" min-width="170" show-overflow-tooltip />
      <el-table-column prop="step_name" :label="t('baseModuleLifecycle.stepName')" width="150" />
      <el-table-column :label="t('baseModuleLifecycle.statusLabel')" width="140">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">
            {{ showStatus(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="command" :label="t('baseModuleLifecycle.command')" min-width="240" show-overflow-tooltip />
      <el-table-column v-if="canViewStepLogs" prop="stdout" label="stdout" min-width="220" show-overflow-tooltip />
      <el-table-column v-if="canViewStepLogs" prop="stderr" label="stderr" min-width="220" show-overflow-tooltip />
      <el-table-column prop="error" :label="t('baseModuleLifecycle.error')" min-width="220" show-overflow-tooltip />
      <el-table-column prop="started_at" :label="t('baseModuleLifecycle.startedAt')" width="190" />
      <el-table-column prop="finished_at" :label="t('baseModuleLifecycle.finishedAt')" width="190" />
    </el-table>
    <LifecyclePagination v-model:query="query" :total="total" @page-change="emit('pageChange')" @size-change="emit('sizeChange')" />
  </div>
</template>
