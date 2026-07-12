<script setup lang="ts">
import type { RoleVo } from '~/base/api/role'
import type { RoleMappingConfig, RoleMappingItem } from './useRoleMappingConfig'
import { Delete, Document, InfoFilled, Plus } from '@element-plus/icons-vue'
import { page as rolePage } from '~/base/api/role'
import {
  defaultRoleMappingConfig,
  parseRoleMappingConfig,
  roleMappingFromItems,
  roleMappingItems,
} from './useRoleMappingConfig'

defineOptions({ name: 'RoleMappingConfigurator' })

const props = defineProps<{
  modelValue?: string | Record<string, any> | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const roleList = ref<RoleVo[]>([])
const mappingList = ref<RoleMappingItem[]>([])
const parseError = ref('')
const lastEmitted = ref('')
const localConfig = ref<RoleMappingConfig>(defaultRoleMappingConfig())

const roleOptions = computed(() =>
  roleList.value
    .filter((role): role is RoleVo & { code: string } => typeof role.code === 'string' && role.code.length > 0)
    .map(role => ({
      label: `${role.name || role.code} (${role.code})`,
      value: role.code,
    })),
)
const showConditionColumn = computed(() => mappingList.value.some(item => item.isConditional))
const jsonPreview = computed(() => JSON.stringify(localConfig.value, null, 2))

watch(
  () => props.modelValue,
  (value) => {
    if (value === lastEmitted.value) {
      return
    }
    try {
      parseError.value = ''
      localConfig.value = parseRoleMappingConfig(value)
      syncMappingList()
    }
    catch (error) {
      parseError.value = error instanceof Error ? error.message : 'JSON 解析失败'
    }
  },
  { immediate: true },
)

onMounted(loadRoles)

function syncMappingList() {
  mappingList.value = roleMappingItems(localConfig.value)
}

function addMappingItem(isConditional = false) {
  mappingList.value.push({
    key: '',
    roles: [],
    condition: '',
    isConditional,
  })
  handleMappingChange()
}

function removeMappingItem(index: number) {
  mappingList.value.splice(index, 1)
  handleMappingChange()
}

function handleConditionalToggle(row: RoleMappingItem) {
  if (!row.isConditional) {
    row.condition = ''
  }
  handleMappingChange()
}

function handleRowConditionalToggle(row: unknown) {
  handleConditionalToggle(row as RoleMappingItem)
}

function handleMappingChange() {
  localConfig.value.mapping = roleMappingFromItems(mappingList.value)
  emitConfigChange()
}

function emitConfigChange() {
  const value = jsonPreview.value
  lastEmitted.value = value
  emit('update:modelValue', value)
}

async function loadRoles() {
  try {
    const res = await rolePage({ page: 1, page_size: 999 })
    roleList.value = res.data?.list || []
  }
  catch {
    roleList.value = []
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
        placeholder="从 OIDC / OAuth2 / SAML 用户信息读取的字段名，如 role、groups"
        @input="emitConfigChange"
      />
      <div class="help-text">
        登录时会读取该字段的值，再按下方规则映射为系统角色编码。
      </div>
    </section>

    <section class="mapping-section">
      <div class="field-label">
        默认角色
      </div>
      <el-select
        v-model="localConfig.default"
        multiple
        filterable
        clearable
        placeholder="未匹配任何规则时分配的角色"
        class="w-full"
        @change="emitConfigChange"
      >
        <el-option
          v-for="role in roleOptions"
          :key="role.value"
          :label="role.label"
          :value="role.value"
        />
      </el-select>
    </section>

    <section class="mapping-section">
      <div class="section-header">
        <div class="field-label">
          映射规则
        </div>
        <div class="mapping-actions">
          <el-button type="primary" :icon="Plus" size="small" @click="addMappingItem(false)">
            添加映射
          </el-button>
          <el-button :icon="Document" size="small" @click="addMappingItem(true)">
            添加条件映射
          </el-button>
        </div>
      </div>

      <el-table :data="mappingList" stripe border class="mapping-table" empty-text="暂无映射规则">
        <el-table-column label="声明值" min-width="160">
          <template #default="{ row }">
            <el-input v-model="row.key" placeholder="如 admin、manager、IT" @input="handleMappingChange" />
          </template>
        </el-table-column>
        <el-table-column label="系统角色" min-width="240">
          <template #default="{ row }">
            <el-select
              v-model="row.roles"
              multiple
              filterable
              clearable
              placeholder="选择角色"
              class="w-full"
              @change="handleMappingChange"
            >
              <el-option
                v-for="role in roleOptions"
                :key="role.value"
                :label="role.label"
                :value="role.value"
              />
            </el-select>
          </template>
        </el-table-column>
        <el-table-column v-if="showConditionColumn" label="条件表达式" min-width="220">
          <template #default="{ row }">
            <div class="condition-cell">
              <el-input
                v-if="row.isConditional"
                v-model="row.condition"
                placeholder="如 level >= 5"
                @input="handleMappingChange"
              />
              <el-checkbox v-model="row.isConditional" @change="handleRowConditionalToggle(row)">
                启用条件
              </el-checkbox>
            </div>
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
