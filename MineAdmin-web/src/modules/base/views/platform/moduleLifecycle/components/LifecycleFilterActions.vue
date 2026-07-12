<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{ loading: boolean }>()
const emit = defineEmits<{ search: [], reset: [] }>()
const { t } = useI18n()
</script>

<template>
  <el-form-item class="module-lifecycle-filter-actions">
    <el-button type="primary" :loading="loading" @click="emit('search')">
      {{ t('crud.search') }}
    </el-button>
    <el-button @click="emit('reset')">
      {{ t('crud.reset') }}
    </el-button>
  </el-form-item>
</template>
