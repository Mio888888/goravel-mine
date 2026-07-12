<script setup lang="ts">
import type { MaFormExpose } from '@mineadmin/form'
import type { DictItemVo, DictTypeVo } from '~/base/api/dictionary'
import { items, saveItem, saveType } from '~/base/api/dictionary'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

defineOptions({ name: 'dataCenter:dictionary:form' })

const { data = null } = defineProps<{
  data?: DictTypeVo | null
}>()

const t = useTrans().globalTrans
const dictForm = ref<MaFormExpose>()
const dictModel = ref<DictTypeVo>({})
const itemList = ref<DictItemVo[]>([])

useForm('tenantDictForm').then(async (form: MaFormExpose) => {
  if (data) {
    dictModel.value = { ...data }
    const response = await items(data.id as number)
    itemList.value = response.data.map(item => ({ ...item }))
  }
  form.setItems([
    {
      label: () => t('baseDictionaryManage.code'),
      prop: 'code',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: { disabled: true },
    },
    {
      label: () => t('baseDictionaryManage.name'),
      prop: 'name',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseDictionaryManage.name') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('baseDictionaryManage.name') }) }],
      },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictRadio,
      cols: { md: 12, xs: 24 },
      renderProps: {
        dictName: 'system-status',
      },
    },
    {
      label: () => t('crud.sort'),
      prop: 'sort',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: { class: 'w-full', min: 0 },
    },
    {
      label: () => t('crud.remark'),
      prop: 'remark',
      render: 'input',
      cols: { xs: 24 },
      renderProps: { type: 'textarea' },
    },
  ])
  form.setOptions({ labelWidth: '110px' })
})

async function edit(): Promise<any> {
  const typeResponse = await saveType(dictModel.value.id as number, dictModel.value)
  if (typeResponse.code !== ResultCode.SUCCESS) {
    throw typeResponse
  }
  for (const item of itemList.value) {
    const response = await saveItem(item.id as number, item)
    if (response.code !== ResultCode.SUCCESS) {
      throw response
    }
  }
  await useDictStore().load(dictModel.value.code, true)
  return typeResponse
}

defineExpose({
  edit,
  maForm: dictForm,
})
</script>

<template>
  <div class="dictionary-form">
    <ma-form ref="dictForm" v-model="dictModel" />

    <div class="mb-3 mt-2 text-sm font-medium">
      {{ t('baseDictionaryManage.items') }}
    </div>

    <el-table :data="itemList" border size="small">
      <el-table-column :label="t('baseDictionaryManage.itemLabel')" min-width="150">
        <template #default="{ row }">
          <el-input v-model="row.label" />
        </template>
      </el-table-column>
      <el-table-column :label="t('baseDictionaryManage.itemValue')" min-width="130">
        <template #default="{ row }">
          <el-input v-model="row.value" disabled />
        </template>
      </el-table-column>
      <el-table-column label="I18n" min-width="180">
        <template #default="{ row }">
          <el-input v-model="row.i18n" />
        </template>
      </el-table-column>
      <el-table-column :label="t('baseDictionaryManage.color')" width="120">
        <template #default="{ row }">
          <el-input v-model="row.color" />
        </template>
      </el-table-column>
      <el-table-column :label="t('crud.status')" width="110">
        <template #default="{ row }">
          <el-switch v-model="row.status" :active-value="1" :inactive-value="2" />
        </template>
      </el-table-column>
      <el-table-column :label="t('crud.sort')" width="110">
        <template #default="{ row }">
          <el-input-number v-model="row.sort" :min="0" controls-position="right" class="w-full" />
        </template>
      </el-table-column>
      <el-table-column :label="t('crud.remark')" min-width="160">
        <template #default="{ row }">
          <el-input v-model="row.remark" />
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>
