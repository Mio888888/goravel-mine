<script setup lang="ts">
import type { ModuleLifecycleLockVo } from '~/base/api/platformModuleLifecycle'
import type { StaleLockReleaseForm } from '../useStaleLockRelease'
import { useI18n } from 'vue-i18n'
import { timeLabel } from '../presentation'

defineProps<{ rows: ModuleLifecycleLockVo[], loading: boolean, releaseLoading: boolean }>()
const emit = defineEmits<{ submit: [] }>()
const form = defineModel<StaleLockReleaseForm>('form', { required: true })
const { t } = useI18n()
</script>

<template>
  <div class="module-lifecycle-section">
    <el-form :model="form" class="module-lifecycle-filter" label-position="top">
      <el-form-item :label="t('baseModuleLifecycle.lockKey')">
        <el-input v-model="form.key" clearable />
      </el-form-item>
      <el-form-item :label="t('baseModuleLifecycle.dryRun')">
        <el-switch v-model="form.dry_run" />
      </el-form-item>
      <el-form-item :label="t('baseModuleLifecycle.confirmToken')">
        <el-input v-model="form.confirm_token" clearable placeholder="release-stale-locks" />
      </el-form-item>
      <el-form-item class="module-lifecycle-filter-actions">
        <el-button v-auth="['platform:moduleLifecycle:execute']" type="primary" :loading="releaseLoading" @click="emit('submit')">
          {{ t('baseModuleLifecycle.releaseLocks') }}
        </el-button>
      </el-form-item>
    </el-form>
    <el-table v-loading="loading" :data="rows" border>
      <el-table-column prop="id" label="ID" width="90" />
      <el-table-column prop="key" :label="t('baseModuleLifecycle.lockKey')" min-width="220" show-overflow-tooltip />
      <el-table-column prop="owner" :label="t('baseModuleLifecycle.owner')" width="160" show-overflow-tooltip />
      <el-table-column prop="run_key" :label="t('baseModuleLifecycle.runKey')" min-width="220" show-overflow-tooltip />
      <el-table-column :label="t('baseModuleLifecycle.expiresAt')" width="190">
        <template #default="{ row }">
          {{ timeLabel(row.expires_at) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('baseModuleLifecycle.updatedAt')" width="190">
        <template #default="{ row }">
          {{ timeLabel(row.updated_at) }}
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>
