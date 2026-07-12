<script setup lang="ts">
import type { MaFormExpose } from '@mineadmin/form'
import type { DictItemVo } from '~/base/api/dictionary'
import type { PlatformDictTypeVo } from '~/base/api/platformDictionary'
import { create, save } from '~/base/api/platformDictionary'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

defineOptions({ name: 'platform:dictionary:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: PlatformDictTypeVo | null
}>()

const t = useTrans().globalTrans
const dictForm = ref<MaFormExpose>()
const dictModel = ref<PlatformDictTypeVo>({
  status: 1,
  sort: 0,
  is_system: true,
  items: [],
})

useForm('dictForm').then((form: MaFormExpose) => {
  if (formType === 'edit' && data) {
    dictModel.value = {
      ...data,
      items: (data.items ?? []).map(item => ({ ...item })),
    }
  }
  if (!dictModel.value.items) {
    dictModel.value.items = []
  }
  form.setItems([
    {
      label: () => t('baseDictionaryManage.code'),
      prop: 'code',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        disabled: formType === 'edit',
        placeholder: t('form.pleaseInput', { msg: t('baseDictionaryManage.code') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('baseDictionaryManage.code') }) }],
      },
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

function addItem() {
  dictModel.value.items?.push({
    label: '',
    value: '',
    status: 1,
    sort: (dictModel.value.items?.length ?? 0) * 10 + 10,
  })
}

function removeItem(index: number) {
  dictModel.value.items?.splice(index, 1)
}

function submit(): Promise<any> {
  return new Promise((resolve, reject) => {
    const request = formType === 'add'
      ? create(dictModel.value)
      : save(dictModel.value.id as number, dictModel.value)
    request.then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch(reject)
  })
}

defineExpose({
  add: submit,
  edit: submit,
  maForm: dictForm,
})
</script>

<template>
  <div class="dictionary-form">
    <ma-form ref="dictForm" v-model="dictModel" />

    <div class="mb-3 mt-2 flex items-center justify-between">
      <span class="text-sm font-medium">{{ t('baseDictionaryManage.items') }}</span>
      <el-button type="primary" link @click="addItem">
        {{ t('crud.add') }}
      </el-button>
    </div>

    <el-table :data="dictModel.items" border size="small">
      <el-table-column :label="t('baseDictionaryManage.itemLabel')" min-width="140">
        <template #default="{ row }">
          <el-input v-model="row.label" />
        </template>
      </el-table-column>
      <el-table-column :label="t('baseDictionaryManage.itemValue')" min-width="130">
        <template #default="{ row }">
          <el-input v-model="row.value" :disabled="formType === 'edit' && !!row.id" />
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
      <el-table-column :label="t('crud.operation')" width="90" fixed="right">
        <template #default="{ $index, row }: { $index: number, row: DictItemVo }">
          <el-button type="danger" link :disabled="formType === 'edit' && !!row.id" @click="removeItem($index)">
            {{ t('crud.delete') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>
