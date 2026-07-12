<script setup lang="ts">
import type { MaFormExpose } from '@mineadmin/form'
import type { TenantPlanOptionVo, TenantVo } from '~/base/api/tenant'
import { create, planOptions, save } from '~/base/api/tenant'
import getFormItems, { getTenantFormTabs } from './data/getFormItems.tsx'
import type { TenantFormTab } from './data/getFormItems.tsx'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

defineOptions({ name: 'platform:tenant:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: TenantVo | null
}>()

const t = useTrans().globalTrans
const tenantForm = ref<MaFormExpose>()
const tenantModel = ref<TenantVo>({})
const plans = ref<TenantPlanOptionVo[]>([])
const activeTab = ref<TenantFormTab>('basic')
const tabOptions = computed(() => getTenantFormTabs(t))
let formExpose: MaFormExpose | null = null

function setTabItems(tab: TenantFormTab) {
  formExpose?.setItems(getFormItems(t, tenantModel.value, tab, plans.value, applyPlanDefaults))
}

function applyPlanDefaults(planCode = tenantModel.value.plan) {
  const plan = plans.value.find(item => item.code === planCode)
  if (!plan) {
    return
  }
  tenantModel.value.plan = plan.code
  tenantModel.value.quotas = {
    ...(tenantModel.value.quotas ?? {}),
    ...(plan.quotas ?? {}),
  }
  tenantModel.value.billing = {
    ...(tenantModel.value.billing ?? {}),
    ...(plan.billing ?? {}),
  }
  tenantModel.value.features = {
    ...(tenantModel.value.features ?? {}),
    ...(plan.features ?? {}),
  }
}

Promise.all([
  useForm('tenantForm'),
  planOptions(),
]).then(([form, response]: [MaFormExpose, any]) => {
  plans.value = response.data ?? []
  if (formType === 'edit' && data) {
    Object.keys(data).map((key: string) => {
      tenantModel.value[key] = data[key]
    })
  }
  if (formType === 'add') {
    applyPlanDefaults(tenantModel.value.plan)
  }
  formExpose = form
  setTabItems(activeTab.value)
  form.setOptions({
    labelWidth: '110px',
  })
})

watch(activeTab, tab => setTabItems(tab))

function add(): Promise<any> {
  return new Promise((resolve, reject) => {
    create(tenantModel.value).then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch((err) => {
      reject(err)
    })
  })
}

function edit(): Promise<any> {
  return new Promise((resolve, reject) => {
    save(tenantModel.value.id as number, tenantModel.value).then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch((err) => {
      reject(err)
    })
  })
}

async function validate(): Promise<void> {
  activeTab.value = 'basic'
  await nextTick()
  const elForm = tenantForm.value?.getElFormRef()
  await elForm?.validate()
}

defineExpose({
  add,
  edit,
  validate,
  maForm: tenantForm,
})
</script>

<template>
  <div class="tenant-form">
    <el-tabs v-model="activeTab" class="tenant-form-tabs">
      <el-tab-pane
        v-for="item in tabOptions"
        :key="item.name"
        :label="item.label"
        :name="item.name"
      />
    </el-tabs>
    <ma-form ref="tenantForm" v-model="tenantModel" />
  </div>
</template>

<style scoped lang="scss">
.tenant-form-tabs {
  margin-bottom: 16px;
}
</style>
