<script setup lang="ts">
import type { ModuleLifecycleDiffVo } from '~/base/api/platformModuleLifecycle'
import { useI18n } from 'vue-i18n'
import { boolLabel } from '../presentation'

defineProps<{ rows: ModuleLifecycleDiffVo[], loading: boolean }>()
const { t } = useI18n()
const showBool = (value?: boolean) => boolLabel({ value, t })
</script>

<template>
  <div class="module-lifecycle-section">
    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="module_id" :label="t('baseModuleLifecycle.moduleId')" min-width="180" show-overflow-tooltip />
      <el-table-column prop="name" :label="t('baseModuleLifecycle.name')" min-width="180" show-overflow-tooltip />
      <el-table-column prop="manifest_version" :label="t('baseModuleLifecycle.manifestVersion')" width="150" />
      <el-table-column prop="persisted_version" :label="t('baseModuleLifecycle.persistedVersion')" width="150" />
      <el-table-column :label="t('baseModuleLifecycle.enabled')" width="160">
        <template #default="{ row }">
          {{ showBool(row.manifest_enabled) }} / {{ showBool(row.persisted_enabled) }}
        </template>
      </el-table-column>
      <el-table-column prop="persisted_status" :label="t('baseModuleLifecycle.persistedStatus')" width="150" />
      <el-table-column prop="last_action" :label="t('baseModuleLifecycle.lastAction')" width="140" />
      <el-table-column prop="drift" :label="t('baseModuleLifecycle.drift')" min-width="160" />
    </el-table>
  </div>
</template>
