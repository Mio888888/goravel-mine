<script setup lang="tsx">
import type { MaProTableExpose, MaProTableOptions, MaProTableSchema } from '@mineadmin/pro-table'
import type { Ref } from 'vue'
import type { ScheduledTaskVo } from '~/base/api/platformScheduledTask'
import type { TransType } from '@/hooks/auto-imports/useTrans.ts'
import type { UseDialogExpose } from '@/hooks/useDialog.ts'
import { ElTag } from 'element-plus'
import { deleteByIds, logs, page } from '~/base/api/platformScheduledTask'
import getSearchItems from './data/getSearchItems.tsx'
import getTableColumns from './data/getTableColumns.tsx'
import { logStatusMeta, taskTypeLabel } from './data/options.ts'
import ScheduledTaskForm from './form.vue'
import useDialog from '@/hooks/useDialog.ts'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

defineOptions({ name: 'platform:scheduledTask' })

const proTableRef = ref<MaProTableExpose>() as Ref<MaProTableExpose>
const formRef = ref()
const selections = ref<any[]>([])
const i18n = useTrans() as TransType
const t = i18n.globalTrans
const msg = useMessage()
const logDrawerVisible = ref(false)
const activeTask = ref<ScheduledTaskVo | null>(null)
const logLoading = ref(false)
const logRows = ref<any[]>([])

const maDialog: UseDialogExpose = useDialog({
  lgWidth: '860px',
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
    mainTitle: () => t('baseScheduledTaskManage.mainTitle'),
    subTitle: () => t('baseScheduledTaskManage.subTitle'),
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
  tableColumns: getTableColumns(maDialog, openLogs, t),
})

function handleDelete() {
  const ids = selections.value.map((item: any) => item.id)
  msg.confirm(t('crud.delMessage')).then(async () => {
    const response = await deleteByIds(ids)
    if (response.code === ResultCode.SUCCESS) {
      msg.success(t('crud.delSuccess'))
      await proTableRef.value.refresh()
    }
  })
}

async function openLogs(row: ScheduledTaskVo) {
  activeTask.value = row
  logDrawerVisible.value = true
  logLoading.value = true
  try {
    const response = await logs({ task_id: row.id, page: 1, page_size: 20 })
    logRows.value = response.data.list ?? []
  }
  finally {
    logLoading.value = false
  }
}
</script>

<template>
  <div class="mine-layout pt-3">
    <MaProTable ref="proTableRef" :options="options" :schema="schema">
      <template #actions>
        <el-button
          v-auth="['platform:scheduledTask:save']"
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
          v-auth="['platform:scheduledTask:delete']"
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
            v-auth="['platform:scheduledTask:save']"
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
        <ScheduledTaskForm v-if="['add', 'edit'].includes(formType)" ref="formRef" :form-type="formType" :data="data" />
      </template>
    </component>

    <el-drawer v-model="logDrawerVisible" :title="`${t('baseScheduledTaskManage.logs')} - ${activeTask?.name ?? ''}`" size="70%">
      <el-table v-loading="logLoading" :data="logRows" border>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="trigger_mode" :label="t('baseScheduledTaskManage.triggerMode')" width="110" />
        <el-table-column :label="t('baseScheduledTaskManage.taskType')" width="110">
          <template #default="{ row }">
            <ElTag>{{ taskTypeLabel(row.task_type) }}</ElTag>
          </template>
        </el-table-column>
        <el-table-column :label="t('baseScheduledTaskManage.logStatus')" width="100">
          <template #default="{ row }">
            <ElTag :type="logStatusMeta(row.status).type">
              {{ logStatusMeta(row.status).label }}
            </ElTag>
          </template>
        </el-table-column>
        <el-table-column prop="node_ip" :label="t('baseScheduledTaskManage.nodeIp')" width="140" />
        <el-table-column prop="started_at" :label="t('baseScheduledTaskManage.startedAt')" width="170" />
        <el-table-column prop="duration_ms" :label="t('baseScheduledTaskManage.duration')" width="110" />
        <el-table-column prop="http_status" label="HTTP" width="90" />
        <el-table-column prop="exit_code" :label="t('baseScheduledTaskManage.exitCode')" width="100" />
        <el-table-column prop="error_message" :label="t('baseScheduledTaskManage.errorMessage')" min-width="220" show-overflow-tooltip />
        <el-table-column prop="stdout" label="STDOUT" min-width="220" show-overflow-tooltip />
        <el-table-column prop="stderr" label="STDERR" min-width="220" show-overflow-tooltip />
      </el-table>
    </el-drawer>
  </div>
</template>
