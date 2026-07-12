<script setup lang="ts">
import type { ModuleLifecycleAction, ModuleLifecycleRunVo } from '~/base/api/platformModuleLifecycle'
import type { LifecycleSelectOption } from '../presentation'
import type { LifecycleRunQuery } from '../useModuleLifecycleState'
import { useI18n } from 'vue-i18n'
import { actionLabel, boolLabel, statusLabel, statusType } from '../presentation'
import LifecycleFilterActions from './LifecycleFilterActions.vue'
import LifecyclePagination from './LifecyclePagination.vue'
import LifecycleQueryFields from './LifecycleQueryFields.vue'

defineProps<{
  rows: ModuleLifecycleRunVo[]
  total: number
  loading: boolean
  moduleOptions: LifecycleSelectOption[]
  actionOptions: LifecycleSelectOption<ModuleLifecycleAction>[]
  statusOptions: LifecycleSelectOption[]
  canViewStepLogs: boolean
}>()
const emit = defineEmits<{
  search: []
  reset: []
  pageChange: []
  sizeChange: []
  viewSteps: [row: ModuleLifecycleRunVo]
}>()
const query = defineModel<LifecycleRunQuery>('query', { required: true })
const { t } = useI18n()
const showAction = (action?: string) => actionLabel({ action, t })
const showBool = (value?: boolean) => boolLabel({ value, t })
const showStatus = (status?: string) => statusLabel({ status, t })
</script>

<template>
  <div class="module-lifecycle-section">
    <el-form :model="query" class="module-lifecycle-filter" label-position="top">
      <LifecycleQueryFields
        v-model:module-id="query.module_id"
        v-model:action="query.action"
        v-model:status="query.status"
        :module-options="moduleOptions"
        :action-options="actionOptions"
        :status-options="statusOptions"
      />
      <el-form-item :label="t('baseModuleLifecycle.owner')">
        <el-input v-model="query.owner" clearable />
      </el-form-item>
      <LifecycleFilterActions :loading="loading" @search="emit('search')" @reset="emit('reset')" />
    </el-form>

    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="id" label="ID" width="90" />
      <el-table-column prop="module_id" :label="t('baseModuleLifecycle.moduleId')" min-width="170" show-overflow-tooltip />
      <el-table-column :label="t('baseModuleLifecycle.actionLabel')" width="120">
        <template #default="{ row }">
          {{ showAction(row.action) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('baseModuleLifecycle.statusLabel')" width="140">
        <template #default="{ row }">
          <el-tag :type="statusType(row.status)">
            {{ showStatus(row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('baseModuleLifecycle.dryRun')" width="100">
        <template #default="{ row }">
          {{ showBool(row.dry_run) }}
        </template>
      </el-table-column>
      <el-table-column prop="owner" :label="t('baseModuleLifecycle.owner')" min-width="130" show-overflow-tooltip />
      <el-table-column prop="reason" :label="t('baseModuleLifecycle.reason')" min-width="200" show-overflow-tooltip />
      <el-table-column prop="command" :label="t('baseModuleLifecycle.command')" min-width="260" show-overflow-tooltip />
      <el-table-column prop="error" :label="t('baseModuleLifecycle.error')" min-width="220" show-overflow-tooltip />
      <el-table-column prop="started_at" :label="t('baseModuleLifecycle.startedAt')" width="190" />
      <el-table-column prop="finished_at" :label="t('baseModuleLifecycle.finishedAt')" width="190" />
      <el-table-column v-if="canViewStepLogs" :label="t('baseModuleLifecycle.stepsTab')" width="110" fixed="right">
        <template #default="{ row }">
          <el-button link type="primary" @click="emit('viewSteps', row as ModuleLifecycleRunVo)">
            {{ t('baseModuleLifecycle.viewSteps') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
    <LifecyclePagination v-model:query="query" :total="total" @page-change="emit('pageChange')" @size-change="emit('sizeChange')" />
  </div>
</template>
