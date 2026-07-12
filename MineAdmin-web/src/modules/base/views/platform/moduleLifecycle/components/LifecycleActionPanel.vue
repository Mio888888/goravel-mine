<script setup lang="ts">
import type { ModuleLifecycleAction } from '~/base/api/platformModuleLifecycle'
import type { LifecycleSelectOption } from '../presentation'
import type { LifecycleActionForm } from '../useLifecycleExecution'
import { useI18n } from 'vue-i18n'

defineProps<{
  actionOptions: LifecycleSelectOption<ModuleLifecycleAction>[]
  moduleOptions: LifecycleSelectOption[]
  expectedConfirmToken: string
  loading: boolean
}>()
const emit = defineEmits<{ submit: [] }>()
const form = defineModel<LifecycleActionForm>('form', { required: true })
const { t } = useI18n()
</script>

<template>
  <div class="module-lifecycle-section">
    <el-form :model="form" class="module-lifecycle-action" label-position="top">
      <el-form-item :label="t('baseModuleLifecycle.actionLabel')">
        <el-select v-model="form.action">
          <el-option v-for="item in actionOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('baseModuleLifecycle.moduleLabel')">
        <el-select v-model="form.module_id" clearable filterable :placeholder="t('baseModuleLifecycle.allModules')">
          <el-option v-for="item in moduleOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('baseModuleLifecycle.executeMode')">
        <el-switch v-model="form.execute" :active-text="t('baseModuleLifecycle.execute')" :inactive-text="t('baseModuleLifecycle.dryRun')" />
      </el-form-item>
      <el-form-item :label="t('baseModuleLifecycle.owner')">
        <el-input v-model="form.owner" clearable />
      </el-form-item>
      <el-form-item :label="t('baseModuleLifecycle.reason')">
        <el-input v-model="form.reason" clearable />
      </el-form-item>
      <el-form-item :label="t('baseModuleLifecycle.confirmToken')">
        <el-input v-model="form.confirm_token" clearable :placeholder="expectedConfirmToken" />
      </el-form-item>
      <el-form-item class="module-lifecycle-submit">
        <el-button v-auth="['platform:moduleLifecycle:execute']" type="primary" :loading="loading" @click="emit('submit')">
          {{ form.execute ? t('baseModuleLifecycle.execute') : t('baseModuleLifecycle.dryRun') }}
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>
