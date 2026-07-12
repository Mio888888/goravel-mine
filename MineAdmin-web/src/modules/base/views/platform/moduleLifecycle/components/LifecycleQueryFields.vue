<script setup lang="ts">
import type { ModuleLifecycleAction } from '~/base/api/platformModuleLifecycle'
import type { LifecycleSelectOption } from '../presentation'
import { useI18n } from 'vue-i18n'

defineProps<{
  moduleOptions: LifecycleSelectOption[]
  actionOptions: LifecycleSelectOption<ModuleLifecycleAction>[]
  statusOptions: LifecycleSelectOption[]
}>()
const moduleID = defineModel<string>('moduleId', { required: true })
const action = defineModel<string>('action', { required: true })
const status = defineModel<string>('status', { required: true })
const { t } = useI18n()
</script>

<template>
  <el-form-item :label="t('baseModuleLifecycle.moduleLabel')">
    <el-select v-model="moduleID" clearable filterable :placeholder="t('baseModuleLifecycle.allModules')">
      <el-option v-for="item in moduleOptions" :key="item.value" :label="item.label" :value="item.value" />
    </el-select>
  </el-form-item>
  <el-form-item :label="t('baseModuleLifecycle.actionLabel')">
    <el-select v-model="action" clearable>
      <el-option v-for="item in actionOptions" :key="item.value" :label="item.label" :value="item.value" />
    </el-select>
  </el-form-item>
  <el-form-item :label="t('baseModuleLifecycle.statusLabel')">
    <el-select v-model="status" clearable>
      <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
    </el-select>
  </el-form-item>
</template>
