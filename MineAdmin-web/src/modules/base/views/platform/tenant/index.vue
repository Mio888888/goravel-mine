<script setup lang="tsx">
import type { MaProTableExpose, MaProTableOptions, MaProTableSchema } from '@mineadmin/pro-table'
import type { Ref } from 'vue'
import type { TransType } from '@/hooks/auto-imports/useTrans.ts'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { destroy, governance, page } from '~/base/api/tenant'
import getSearchItems from './data/getSearchItems.tsx'
import getTableColumns from './data/getTableColumns.tsx'
import useDialog from '@/hooks/useDialog.ts'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import TenantForm from './form.vue'
import SetPermissionForm from './setPermissionForm.vue'
import SensitiveOperationEvidenceDialog from '~/base/components/SensitiveOperationEvidenceDialog.vue'
import TenantExportDialog from './components/TenantExportDialog.vue'

defineOptions({ name: 'platform:tenant' })

const proTableRef = ref<MaProTableExpose>() as Ref<MaProTableExpose>
const formRef = ref()
const selections = ref<any[]>([])
const i18n = useTrans() as TransType
const t = i18n.globalTrans
const msg = useMessage()
const evidenceDialogRef = ref<InstanceType<typeof SensitiveOperationEvidenceDialog>>()
const exportDialogRef = ref<InstanceType<typeof TenantExportDialog>>()
const requestSensitiveEvidence = (input: any) => evidenceDialogRef.value!.open(input)
provide('requestSensitiveEvidence', requestSensitiveEvidence)

const maDialog: UseDialogExpose = useDialog({
  lgWidth: '720px',
  ok: ({ formType }, okLoadingState: (state: boolean) => void) => {
    okLoadingState(true)
    if (['add', 'edit'].includes(formType)) {
      formRef.value.validate().then(() => {
        const submit = formType === 'add' ? formRef.value.add : formRef.value.edit
        submit().then((res: any) => {
          const message = formType === 'add' ? t('crud.createSuccess') : t('crud.updateSuccess')
          res.code === ResultCode.SUCCESS ? msg.success(message) : msg.error(res.message)
          maDialog.close()
          proTableRef.value.refresh()
        }).catch((err: any) => {
          msg.alertError(err)
        })
      }).catch()
    }
    if (formType === 'permission') {
      formRef.value.saveTenantPermissions().then((res: any) => {
        res.code === ResultCode.SUCCESS ? msg.success(t('baseTenantManage.permissionSuccess')) : msg.error(res.message)
        maDialog.close()
        proTableRef.value.refresh()
      }).catch((err: any) => {
        msg.alertError(err)
      })
    }
    okLoadingState(false)
  },
})

const options = ref<MaProTableOptions>({
  adaptionOffsetBottom: 161,
  header: {
    mainTitle: () => t('baseTenantManage.mainTitle'),
    subTitle: () => t('baseTenantManage.subTitle'),
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
  requestOptions: {
    api: page,
  },
})

const schema = ref<MaProTableSchema>({
  searchItems: getSearchItems(t),
  tableColumns: getTableColumns(maDialog, t, requestTenantDeletionEvidence, requestSensitiveEvidence, row => exportDialogRef.value?.open(row)),
})

function tenantDeletionResource(ids: number[]) {
  return `tenant-data:delete:${[...ids].sort((a, b) => a - b).join(',')}:metadata`
}

async function requestTenantDeletionEvidence(ids: number[]) {
  const policies = await Promise.all(ids.map(async (id) => {
    const response = await governance(id)
    if (response.code !== ResultCode.SUCCESS) {
      throw response
    }
    return response.data
  }))
  return evidenceDialogRef.value!.open({
    scope: 'tenant.data.delete',
    resource: tenantDeletionResource(ids),
    reason: `Destroy tenant metadata: ${ids.join(',')}`,
    approval_required: policies.some(policy => policy.data_deletion.requires_approval),
  })
}

async function handleDestroy() {
  const ids = selections.value.map((item: any) => item.id)
  try {
    const evidence = await requestTenantDeletionEvidence(ids)
    await msg.delConfirm(t('baseTenantManage.destroyConfirm'))
    const response = await destroy({
      ids,
      confirm_code: selections.value.length === 1 ? selections.value[0].code : undefined,
      ...evidence,
    })
    if (response.code === ResultCode.SUCCESS) {
      msg.success(t('baseTenantManage.destroySuccess'))
      await proTableRef.value.refresh()
    }
  }
  catch {}
}
</script>

<template>
  <div class="mine-layout pt-3">
    <MaProTable ref="proTableRef" :options="options" :schema="schema">
      <template #actions>
        <el-button
          v-auth="['platform:tenant:save']"
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
          v-auth="['platform:tenant:destroy']"
          type="danger"
          plain
          :disabled="selections.length < 1"
          @click="handleDestroy"
        >
          {{ t('baseTenantManage.destroy') }}
        </el-button>
      </template>
      <template #empty>
        <el-empty>
          <el-button
            v-auth="['platform:tenant:save']"
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
        <TenantForm v-if="['add', 'edit'].includes(formType)" ref="formRef" :form-type="formType" :data="data" />
        <SetPermissionForm v-if="formType === 'permission'" ref="formRef" :data="data" />
      </template>
    </component>
    <SensitiveOperationEvidenceDialog ref="evidenceDialogRef" />
    <TenantExportDialog ref="exportDialogRef" />
  </div>
</template>
