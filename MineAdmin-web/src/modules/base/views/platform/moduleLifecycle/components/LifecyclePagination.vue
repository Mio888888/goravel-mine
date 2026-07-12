<script setup lang="ts">
interface PaginationQuery {
  page: number
  page_size: number
}

defineProps<{ total: number }>()
const emit = defineEmits<{ pageChange: [], sizeChange: [] }>()
const query = defineModel<PaginationQuery>('query', { required: true })
</script>

<template>
  <div class="module-lifecycle-pagination">
    <el-pagination
      v-model:current-page="query.page"
      v-model:page-size="query.page_size"
      background
      layout="total, prev, pager, next, sizes"
      :total="total"
      :page-sizes="[15, 30, 50]"
      @current-change="emit('pageChange')"
      @size-change="emit('sizeChange')"
    />
  </div>
</template>
