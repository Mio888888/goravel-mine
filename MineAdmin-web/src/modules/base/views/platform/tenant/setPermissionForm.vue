<script setup lang="tsx">
import type { MaFormExpose } from '@mineadmin/form'
import type { MenuVo } from '~/base/api/menu.ts'
import type { TenantPermissionPayload, TenantVo } from '~/base/api/tenant.ts'
import { permissionCatalog, permissions, savePermissions } from '~/base/api/tenant.ts'
import useForm from '@/hooks/useForm.ts'
import MaTree from '@/components/ma-tree/index.vue'
import { ResultCode } from '@/utils/ResultCode.ts'
import { includePermissionAncestors } from './permissionTree.ts'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'
import { tenantChangeResource } from '~/base/utils/sensitiveOperation'

defineOptions({ name: 'platform:tenant:setPermissionForm' })

const { data = null } = defineProps<{
  data?: TenantVo | null
}>()

const t = useTrans().globalTrans
const permissionForm = ref<MaFormExpose>()
const model = ref<{ id?: number }>({})
const permissionTreeRef = ref<any>()
let permissionPayload: TenantPermissionPayload = { allowed: [] }
let menus: MenuVo[] = []
const requestEvidence = inject<SensitiveEvidenceRequester>('requestSensitiveEvidence')!

function treeItem(prop: string, label: string, treeRef: typeof permissionTreeRef) {
  return {
    label: () => label,
    prop,
    render: () => MaTree,
    renderProps: {
      ref: (el: any) => treeRef.value = el,
      class: 'w-full tenant-permission-tree',
      showCheckbox: true,
      treeKey: 'meta.title',
      nodeKey: 'name',
      data: menus,
    },
    renderSlots: {
      default: ({ data }) => (
        <div class="mine-tree-node">
          <div class="label">
            { data.meta?.icon && <ma-svg-icon name={data.meta?.icon} size={16} /> }
            { data.meta?.i18n ? t(data.meta?.i18n) : data.meta?.title ?? data.name }
          </div>
        </div>
      ),
    },
  }
}

useForm('permissionForm').then(async (form: MaFormExpose) => {
  if (data?.id) {
    model.value.id = data.id
    const [permissionRes, menuRes] = await Promise.all([
      permissions(data.id),
      permissionCatalog(),
    ])
    if (permissionRes.code === ResultCode.SUCCESS && permissionRes.data) {
      permissionPayload = {
        allowed: permissionRes.data.allowed ?? [],
      }
    }
    menus = menuRes.data ?? []
  }

  form.setItems([
    treeItem('allowed', t('baseTenantManage.allowedPermissions'), permissionTreeRef),
  ])
  form.setOptions({ labelWidth: '110px' })
  await nextTick(() => {
    setTimeout(() => {
      permissionTreeRef.value?.elTree?.setCheckedKeys?.(permissionPayload.allowed)
    }, 50)
  })
})

async function saveTenantPermissions(): Promise<any> {
  const checked = permissionTreeRef.value?.elTree?.getCheckedKeys?.() as string[] ?? []
  const halfChecked = permissionTreeRef.value?.elTree?.getHalfCheckedKeys?.() as string[] ?? []
  const payload = { allowed: includePermissionAncestors(menus, [...checked, ...halfChecked]) }
  const id = model.value.id as number
  const resource = tenantChangeResource('permissions', id, payload)
  const evidence = await requestEvidence({ policy_key: 'tenant.permissions.sync', scope: 'tenant.permissions.sync', resource, reason: `Sync tenant ${id} permissions` })
  const response = await savePermissions(id, payload, evidence)
  if (response.code !== ResultCode.SUCCESS) {
    throw response
  }
  return response
}

defineExpose({
  saveTenantPermissions,
  maForm: permissionForm,
})
</script>

<template>
  <ma-form ref="permissionForm" v-model="model" />
</template>

<style scoped lang="scss">
.tenant-permission-tree {
  max-height: 360px;
}
</style>
