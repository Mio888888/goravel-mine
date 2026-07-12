<script setup lang="ts">
import type { MaFormExpose } from '@mineadmin/form'
import type { MenuVo } from '~/base/api/menu'
import type { TenantPlanVo } from '~/base/api/platformTenantPlan'
import { permissionCatalog } from '~/base/api/tenant'
import { create, save } from '~/base/api/platformTenantPlan'
import getFormItems from './data/getFormItems.tsx'
import { includePermissionAncestors } from '../tenant/permissionTree.ts'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

defineOptions({ name: 'platform:tenantPlan:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: TenantPlanVo | null
}>()

const t = useTrans().globalTrans
const planForm = ref<MaFormExpose>()
const planModel = ref<TenantPlanVo>({})
const permissionTreeRef = ref<any>()
const permissionMenus = ref<MenuVo[]>([])

Promise.all([
  useForm('planForm'),
  permissionCatalog(),
]).then(([form, menuRes]: [MaFormExpose, any]) => {
  const menus: MenuVo[] = menuRes.data ?? []
  permissionMenus.value = menus
  if (formType === 'edit' && data) {
    Object.keys(data).map((key: string) => {
      planModel.value[key] = data[key]
    })
  }
  form.setItems(getFormItems(t, planModel.value, menus, permissionTreeRef))
  form.setOptions({
    labelWidth: '110px',
  })
})

function syncPermissionTree() {
  const checked = permissionTreeRef.value?.elTree?.getCheckedKeys?.() as string[] | undefined
  if (!checked) {
    return
  }
  const halfChecked = permissionTreeRef.value?.elTree?.getHalfCheckedKeys?.() as string[] ?? []
  planModel.value.features = planModel.value.features ?? {}
  planModel.value.features.permissions = {
    ...(planModel.value.features.permissions ?? {}),
    allowed: includePermissionAncestors(permissionMenus.value, [...checked, ...halfChecked]),
  }
}

function add(): Promise<any> {
  return new Promise((resolve, reject) => {
    syncPermissionTree()
    create(planModel.value).then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch((err) => {
      reject(err)
    })
  })
}

function edit(): Promise<any> {
  return new Promise((resolve, reject) => {
    syncPermissionTree()
    save(planModel.value.id as number, planModel.value).then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch((err) => {
      reject(err)
    })
  })
}

defineExpose({
  add,
  edit,
  maForm: planForm,
})
</script>

<template>
  <ma-form ref="planForm" v-model="planModel" />
</template>
