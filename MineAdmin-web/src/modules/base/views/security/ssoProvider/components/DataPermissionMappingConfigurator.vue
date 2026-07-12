<script setup lang="ts">
import type { DepartmentVo } from '~/base/api/department'
import type { DataPermissionMappingConfig, DataPermissionMappingItem } from './useDataPermissionMappingConfig'
import { Delete, Document, Folder, InfoFilled, Plus } from '@element-plus/icons-vue'
import { page as departmentPage } from '~/base/api/department'
import {
  dataPermissionMappingFromItems,
  dataPermissionMappingItems,
  defaultDataPermissionMappingConfig,
  parseDataPermissionMappingConfig,
  policyTypes,
  simplePolicyTypes,
} from './useDataPermissionMappingConfig'
import { dictLabel } from '@/utils/dict'

defineOptions({ name: 'DataPermissionMappingConfigurator' })

const props = defineProps<{
  modelValue?: string | Record<string, any> | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const departmentList = ref<DepartmentVo[]>([])
const mappingList = ref<DataPermissionMappingItem[]>([])
const parseError = ref('')
const lastEmitted = ref('')
const localConfig = ref<DataPermissionMappingConfig>(defaultDataPermissionMappingConfig())
const t = useTrans().globalTrans

const simplePolicyOptions = computed(() => policyTypes.filter(item => simplePolicyTypes.includes(item)))
const showCustomValueColumn = computed(() =>
  mappingList.value.some(item => !simplePolicyTypes.includes(item.valueType) || item.isConditional),
)
const jsonPreview = computed(() => JSON.stringify(localConfig.value, null, 2))

watch(
  () => props.modelValue,
  (value) => {
    if (value === lastEmitted.value) {
      return
    }
    try {
      parseError.value = ''
      localConfig.value = parseDataPermissionMappingConfig(value)
      syncMappingList()
    }
    catch (error) {
      parseError.value = error instanceof Error ? error.message : 'JSON 解析失败'
    }
  },
  { immediate: true },
)

onMounted(loadDepartments)

function syncMappingList() {
  mappingList.value = dataPermissionMappingItems(localConfig.value)
}

function addMappingItem(options?: { valueType?: DataPermissionMappingItem['valueType'], isConditional?: boolean }) {
  mappingList.value.push({
    key: '',
    valueType: options?.valueType ?? 'SELF',
    customValue: options?.valueType === 'CUSTOM_DEPT' ? [] : undefined,
    condition: '',
    isConditional: options?.isConditional ?? false,
  })
  handleMappingChange()
}

function removeMappingItem(index: number) {
  mappingList.value.splice(index, 1)
  handleMappingChange()
}

function handleTypeChange(row: DataPermissionMappingItem) {
  if (simplePolicyTypes.includes(row.valueType)) {
    row.customValue = undefined
  }
  else if (row.valueType === 'CUSTOM_DEPT' && !Array.isArray(row.customValue)) {
    row.customValue = []
  }
  else if (row.valueType === 'CUSTOM_FUNC' && typeof row.customValue !== 'string') {
    row.customValue = ''
  }
  handleMappingChange()
}

function handleRowTypeChange(row: unknown) {
  handleTypeChange(row as DataPermissionMappingItem)
}

function handleMappingChange() {
  localConfig.value.mapping = dataPermissionMappingFromItems(mappingList.value)
  emitConfigChange()
}

function emitConfigChange() {
  const value = jsonPreview.value
  lastEmitted.value = value
  emit('update:modelValue', value)
}

async function loadDepartments() {
  try {
    const res = await departmentPage()
    departmentList.value = res.data?.list || []
  }
  catch {
    departmentList.value = []
  }
}
</script>

<template>
  <div class="sso-mapping-configurator">
    <el-alert
      v-if="parseError"
      :title="`当前 JSON 无法解析：${parseError}`"
      type="error"
      show-icon
      :closable="false"
    />

    <section class="mapping-section">
      <div class="field-label">
        <el-icon><InfoFilled /></el-icon>
        <span>声明字段</span>
      </div>
      <el-input
        v-model="localConfig.claim"
        placeholder="从 OIDC / OAuth2 / SAML 用户信息读取的字段名，如 role、department、level"
        @input="emitConfigChange"
      />
      <div class="help-text">
        登录时会读取该字段的值，再按下方规则映射为用户数据权限策略。
      </div>
    </section>

    <section class="mapping-section">
      <div class="field-label">
        默认数据权限
      </div>
      <el-radio-group v-model="localConfig.default" @change="emitConfigChange">
        <el-radio-button
          v-for="option in simplePolicyOptions"
          :key="option"
          :label="option"
        >
          {{ dictLabel('data-scope', option, t) }}
        </el-radio-button>
      </el-radio-group>
    </section>

    <section class="mapping-section">
      <div class="section-header">
        <div class="field-label">
          映射规则
        </div>
        <div class="mapping-actions">
          <el-button type="primary" :icon="Plus" size="small" @click="addMappingItem()">
            添加映射
          </el-button>
          <el-button :icon="Folder" size="small" @click="addMappingItem({ valueType: 'CUSTOM_DEPT' })">
            自定义部门
          </el-button>
          <el-button :icon="Document" size="small" @click="addMappingItem({ isConditional: true })">
            条件映射
          </el-button>
        </div>
      </div>

      <el-table :data="mappingList" stripe border class="mapping-table" empty-text="暂无映射规则">
        <el-table-column label="声明值" min-width="160">
          <template #default="{ row }">
            <el-input v-model="row.key" placeholder="如 admin、manager、IT" @input="handleMappingChange" />
          </template>
        </el-table-column>
        <el-table-column label="数据权限类型" min-width="210">
          <template #default="{ row }">
            <el-select v-model="row.valueType" placeholder="选择类型" class="w-full" @change="handleRowTypeChange(row)">
              <el-option
                v-for="option in policyTypes"
                :key="option"
                :label="dictLabel('data-scope', option, t)"
                :value="option"
              />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column v-if="showCustomValueColumn" label="自定义值 / 条件" min-width="260">
          <template #default="{ row }">
            <div class="condition-cell">
              <el-tree-select
                v-if="row.valueType === 'CUSTOM_DEPT'"
                v-model="row.customValue"
                :data="departmentList"
                multiple
                filterable
                clearable
                check-strictly
                node-key="id"
                :props="{ label: 'name' }"
                placeholder="选择部门"
                class="w-full"
                @change="handleMappingChange"
              />
              <el-input
                v-else-if="row.valueType === 'CUSTOM_FUNC'"
                v-model="row.customValue"
                placeholder="输入自定义函数名"
                @input="handleMappingChange"
              />
              <el-input
                v-if="row.isConditional"
                v-model="row.condition"
                placeholder="条件表达式，如 level >= 5"
                @input="handleMappingChange"
              />
            </div>
          </template>
        </el-table-column>
        <el-table-column label="条件" width="90">
          <template #default="{ row }">
            <el-checkbox v-model="row.isConditional" @change="handleMappingChange">
              启用
            </el-checkbox>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ $index }">
            <el-button type="danger" :icon="Delete" size="small" link @click="removeMappingItem($index)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="mapping-section">
      <div class="field-label">
        JSON 预览
      </div>
      <el-input :model-value="jsonPreview" type="textarea" :rows="7" readonly class="json-preview" />
    </section>
  </div>
</template>

<style scoped lang="scss" src="./mappingConfigurator.scss"></style>
