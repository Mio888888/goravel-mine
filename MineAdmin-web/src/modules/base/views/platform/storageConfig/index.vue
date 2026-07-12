<script setup lang="tsx">
import type { MaProTableExpose, MaProTableOptions, MaProTableSchema } from '@mineadmin/pro-table'
import type { Ref } from 'vue'
import type { TransType } from '@/hooks/auto-imports/useTrans.ts'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { deleteByIds, page } from '~/base/api/platformStorageConfig'
import getSearchItems from './data/getSearchItems.tsx'
import getTableColumns from './data/getTableColumns.tsx'
import useDialog from '@/hooks/useDialog.ts'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import StorageConfigForm from './form.vue'
import SensitiveOperationEvidenceDialog from '~/base/components/SensitiveOperationEvidenceDialog.vue'
import { secretResource } from '~/base/utils/sensitiveOperation'

defineOptions({ name: 'platform:storageConfig' })

const proTableRef = ref<MaProTableExpose>() as Ref<MaProTableExpose>
const formRef = ref()
const selections = ref<any[]>([])
const i18n = useTrans() as TransType
const t = i18n.globalTrans
const msg = useMessage()
const evidenceDialogRef = ref<InstanceType<typeof SensitiveOperationEvidenceDialog>>()
const requestSensitiveEvidence = (input: any) => evidenceDialogRef.value!.open(input)
provide('requestSensitiveEvidence', requestSensitiveEvidence)

const maDialog: UseDialogExpose = useDialog({
  lgWidth: '780px',
  ok: ({ formType }, okLoadingState: (state: boolean) => void) => {
    okLoadingState(true)
    if (['add', 'edit'].includes(formType)) {
      const elForm = formRef.value.maForm.getElFormRef()
      elForm.validate().then(() => {
        const submit = formType === 'add' ? formRef.value.add : formRef.value.edit
        submit().then((res: any) => {
          const message = formType === 'add' ? t('crud.createSuccess') : t('crud.updateSuccess')
          res.code === ResultCode.SUCCESS ? msg.success(message) : msg.error(res.message)
          maDialog.close()
          proTableRef.value.refresh()
        }).catch((err: any) => msg.alertError(err))
      }).catch()
    }
    okLoadingState(false)
  },
})

const options = ref<MaProTableOptions>({
  adaptionOffsetBottom: 161,
  header: {
    mainTitle: () => t('baseStorageConfigManage.mainTitle'),
    subTitle: () => t('baseStorageConfigManage.subTitle'),
  },
  tableOptions: {
    on: {
      onSelectionChange: (selection: any[]) => selections.value = selection,
    },
  },
  searchOptions: {
    fold: true,
    text: {
      searchBtn: () => t('crud.search'),
      resetBtn: () => t('crud.reset'),
      isFoldBtn: () => t('crud.searchFold'),
      notFoldBtn: () => t('crud.searchUnFold'),
    },
  },
  searchFormOptions: { labelWidth: '90px' },
  requestOptions: { api: page },
})

const schema = ref<MaProTableSchema>({
  searchItems: getSearchItems(t),
  tableColumns: getTableColumns(maDialog, t, requestDeleteEvidence),
})

function requestDeleteEvidence(ids: number[]) {
  return requestSensitiveEvidence({
    policy_key: 'storage.secret.change',
    scope: 'storage.secret.change',
    resource: secretResource('storage-config', 'delete', ids),
    reason: `Delete storage configs: ${ids.join(',')}`,
  })
}

function handleDelete() {
  const ids = selections.value.map((item: any) => item.id)
  msg.confirm(t('crud.delMessage')).then(async () => {
    const evidence = await requestDeleteEvidence(ids)
    const response = await deleteByIds(ids, evidence)
    if (response.code === ResultCode.SUCCESS) {
      msg.success(t('crud.delSuccess'))
      await proTableRef.value.refresh()
    }
  })
}
</script>

<template>
  <div class="mine-layout pt-3">
    <MaProTable ref="proTableRef" :options="options" :schema="schema">
      <template #actions>
        <el-button
          v-auth="['platform:storageConfig:save']"
          type="primary"
          @click="() => {
            maDialog.setTitle(t('crud.add'))
            maDialog.open({ formType: 'add' })
          }"
        >
          {{ t('crud.add') }}
        </el-button>
      </template>
      <template #toolbarLeft>
        <el-button
          v-auth="['platform:storageConfig:delete']"
          type="danger"
          plain
          :disabled="selections.length < 1"
          @click="handleDelete"
        >
          {{ t('crud.delete') }}
        </el-button>
      </template>
      <template #empty>
        <el-empty>
          <el-button
            v-auth="['platform:storageConfig:save']"
            type="primary"
            @click="() => {
              maDialog.setTitle(t('crud.add'))
              maDialog.open({ formType: 'add' })
            }"
          >
            {{ t('crud.add') }}
          </el-button>
        </el-empty>
      </template>
    </MaProTable>

    <component :is="maDialog.Dialog">
      <template #default="{ formType, data }">
        <StorageConfigForm v-if="['add', 'edit'].includes(formType)" ref="formRef" :form-type="formType" :data="data" />
      </template>
    </component>
    <SensitiveOperationEvidenceDialog ref="evidenceDialogRef" />
  </div>
</template>
