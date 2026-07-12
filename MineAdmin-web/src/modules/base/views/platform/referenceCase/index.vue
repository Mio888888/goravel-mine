<script setup lang="ts">
import type { FormInstance } from 'element-plus'
import type { ReferenceCaseVo } from '~/base/api/platformReferenceCase'
import { create, deleteByIds, page, save } from '~/base/api/platformReferenceCase'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

defineOptions({ name: 'platform:referenceCase' })

const t = useTrans().globalTrans
const msg = useMessage()
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)
const formRef = ref<FormInstance>()
const rows = ref<ReferenceCaseVo[]>([])
const total = ref(0)
const search = reactive({ code: '', title: '', status: undefined as number | undefined, page: 1, page_size: 10 })
const form = reactive<ReferenceCaseVo>({
  code: '',
  title: '',
  status: 1,
  version: '1.0.0',
  payload: { scenario: 'upgrade' },
  remark: '',
})
const payloadText = ref('{\n  "scenario": "upgrade"\n}')

const rules = {
  code: [{ required: true, message: t('baseReferenceCase.codeRequired'), trigger: 'blur' }],
  title: [{ required: true, message: t('baseReferenceCase.titleRequired'), trigger: 'blur' }],
}

async function fetchRows() {
  loading.value = true
  try {
    const res = await page(search)
    rows.value = res.data?.list ?? []
    total.value = res.data?.total ?? 0
  }
  catch {
    // 全局 HTTP 拦截器已反馈错误，此处仅收敛事件 Promise。
  }
  finally {
    loading.value = false
  }
}

function resetSearch() {
  search.code = ''
  search.title = ''
  search.status = undefined
  search.page = 1
  fetchRows()
}

function openCreate() {
  Object.assign(form, { id: undefined, code: '', title: '', status: 1, version: '1.0.0', payload: { scenario: 'upgrade' }, remark: '' })
  payloadText.value = JSON.stringify(form.payload, null, 2)
  dialogVisible.value = true
}

function openEdit(row: ReferenceCaseVo) {
  Object.assign(form, row)
  payloadText.value = JSON.stringify(row.payload ?? {}, null, 2)
  dialogVisible.value = true
}

async function submit() {
  try {
    if (!await formRef.value?.validate()) {
      return
    }
  }
  catch {
    return
  }
  let parsed: Record<string, any> | null = null
  try {
    parsed = payloadText.value.trim() ? JSON.parse(payloadText.value) : null
  }
  catch {
    msg.error(t('baseReferenceCase.payloadJsonError'))
    return
  }
  saving.value = true
  try {
    const payload = { ...form, payload: parsed }
    const res = form.id ? await save(form.id, payload) : await create(payload)
    if (res.code === ResultCode.SUCCESS) {
      msg.success(form.id ? t('crud.updateSuccess') : t('crud.createSuccess'))
      dialogVisible.value = false
      await fetchRows()
    }
    else {
      msg.error(res.message)
    }
  }
  catch {
    // 全局 HTTP 拦截器已反馈错误，此处仅恢复提交状态。
  }
  finally {
    saving.value = false
  }
}

async function remove(row: ReferenceCaseVo) {
  try {
    await msg.confirm(t('crud.delMessage'))
  }
  catch {
    return
  }
  try {
    const res = await deleteByIds([row.id as number])
    if (res.code === ResultCode.SUCCESS) {
      msg.success(t('crud.delSuccess'))
      await fetchRows()
    }
  }
  catch {
    // 全局 HTTP 拦截器已反馈错误，此处仅收敛事件 Promise。
  }
}

onMounted(fetchRows)
</script>

<template>
  <div class="mine-layout reference-case-page">
    <el-card shadow="never">
      <template #header>
        <div class="reference-case-header">
          <div>
            <h2>{{ t('baseReferenceCase.mainTitle') }}</h2>
            <p>{{ t('baseReferenceCase.subTitle') }}</p>
          </div>
          <el-button v-auth="['platform:referenceCase:save']" type="primary" @click="openCreate">
            {{ t('crud.add') }}
          </el-button>
        </div>
      </template>

      <el-form :model="search" inline label-width="80px">
        <el-form-item :label="t('baseReferenceCase.code')">
          <el-input v-model="search.code" clearable />
        </el-form-item>
        <el-form-item :label="t('baseReferenceCase.title')">
          <el-input v-model="search.title" clearable />
        </el-form-item>
        <el-form-item :label="t('baseModuleLifecycle.statusLabel')">
          <el-select v-model="search.status" clearable class="reference-case-status">
            <el-option :label="t('baseReferenceCase.enabled')" :value="1" />
            <el-option :label="t('baseReferenceCase.disabled')" :value="2" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="fetchRows">
            {{ t('crud.search') }}
          </el-button>
          <el-button @click="resetSearch">
            {{ t('crud.reset') }}
          </el-button>
        </el-form-item>
      </el-form>

      <el-table v-loading="loading" :data="rows" row-key="id" border>
        <el-table-column prop="code" :label="t('baseReferenceCase.code')" min-width="160" />
        <el-table-column prop="title" :label="t('baseReferenceCase.title')" min-width="220" />
        <el-table-column prop="version" :label="t('baseModuleLifecycle.version')" width="120" />
        <el-table-column prop="status" :label="t('baseModuleLifecycle.statusLabel')" width="120">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'">
              {{ row.status === 1 ? t('baseReferenceCase.enabled') : t('baseReferenceCase.disabled') }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="remark" :label="t('baseReferenceCase.remark')" min-width="180" />
        <el-table-column :label="t('crud.operation')" width="180" fixed="right">
          <template #default="{ row }">
            <el-button v-auth="['platform:referenceCase:update']" type="primary" link @click="openEdit(row)">
              {{ t('crud.edit') }}
            </el-button>
            <el-button v-auth="['platform:referenceCase:delete']" type="danger" link @click="remove(row)">
              {{ t('crud.delete') }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-pagination
        v-model:current-page="search.page"
        v-model:page-size="search.page_size"
        class="reference-case-pagination"
        layout="total, sizes, prev, pager, next"
        :total="total"
        @change="fetchRows"
      />
    </el-card>

    <el-dialog v-model="dialogVisible" :title="form.id ? t('crud.edit') : t('crud.add')" width="640px">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item :label="t('baseReferenceCase.code')" prop="code">
          <el-input v-model="form.code" />
        </el-form-item>
        <el-form-item :label="t('baseReferenceCase.title')" prop="title">
          <el-input v-model="form.title" />
        </el-form-item>
        <el-form-item :label="t('baseModuleLifecycle.version')">
          <el-input v-model="form.version" />
        </el-form-item>
        <el-form-item :label="t('baseModuleLifecycle.statusLabel')">
          <el-select v-model="form.status">
            <el-option :label="t('baseReferenceCase.enabled')" :value="1" />
            <el-option :label="t('baseReferenceCase.disabled')" :value="2" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('baseReferenceCase.payload')">
          <el-input v-model="payloadText" type="textarea" :rows="6" />
        </el-form-item>
        <el-form-item :label="t('baseReferenceCase.remark')">
          <el-input v-model="form.remark" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">
          {{ t('crud.cancel') }}
        </el-button>
        <el-button type="primary" :loading="saving" @click="submit">
          {{ t('crud.ok') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.reference-case-page {
  padding-top: 12px;
}

.reference-case-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.reference-case-header h2 {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
}

.reference-case-header p {
  margin: 6px 0 0;
  color: var(--el-text-color-secondary);
}

.reference-case-status {
  width: 140px;
}

.reference-case-pagination {
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
