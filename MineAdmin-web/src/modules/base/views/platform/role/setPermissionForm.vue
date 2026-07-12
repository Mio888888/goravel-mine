<!--
 - MineAdmin is committed to providing solutions for quickly building web applications
 - Please view the LICENSE file that was distributed with this source code,
 - For the full copyright and license information.
 - Thank you very much for using MineAdmin.
 -
 - @Author X.Mo<root@imoi.cn>
 - @Link https://github.com/mineadmin
-->

<script setup lang="tsx">
import type { MaFormExpose } from '@mineadmin/form'
import type { RoleVo } from '~/base/api/platformRole.ts'
import { getRolePermission, setRolePermission } from '~/base/api/platformRole.ts'
import { page } from '~/base/api/platformMenu.ts'

import useForm from '@/hooks/useForm.ts'

import MaTree from '@/components/ma-tree/index.vue'
import { ResultCode } from '@/utils/ResultCode.ts'
import type { SensitiveEvidenceRequester } from '~/base/utils/sensitiveOperation'
import { rbacPermissionsResource } from '~/base/utils/sensitiveOperation'

const { data = null } = defineProps<{
  data?: RoleVo | null
}>()

const t = useTrans().globalTrans
const userRoleForm = ref<MaFormExpose>()
const userModel = ref<{ id?: number }>({})
const checkStrictly = ref<boolean>(false)

const permissionTreeRef = ref<any>()
const requestEvidence = inject<SensitiveEvidenceRequester>('requestSensitiveEvidence')!

useForm('userRoleForm').then(async (form: MaFormExpose) => {
  const names: string[] = []
  if (data?.id) {
    userModel.value.id = data.id
    const response: any = await getRolePermission(data?.id)
    if (response.code === ResultCode.SUCCESS && response.data) {
      response.data.map((item: any) => {
        names.push(item.name)
      })
      checkStrictly.value = true
    }
  }

  const menuRes = await page()
  form.setItems([
    {
      label: () => t('basePlatformRoleManage.permission'),
      prop: 'permission_id',
      render: () => MaTree,
      renderProps: {
        ref: (el: any) => permissionTreeRef.value = el,
        class: 'w-full',
        showCheckbox: true,
        treeKey: 'meta.title',
        placeholder: t('form.pleaseSelect', { msg: t('basePlatformRoleManage.permission') }),
        nodeKey: 'name',
        data: menuRes.data,
      },
      renderSlots: {
        default: ({ data }) => {
          return (
            <div class="mine-tree-node">
              <div class="label">
                { data.meta?.icon && <ma-svg-icon name={data.meta?.icon} size={16} />}
                { data.meta?.i18n ? t(data.meta?.i18n) : data.meta.title ?? 'unknown' }
              </div>
            </div>
          )
        },
      },
    },
  ])
  form.setOptions({
    labelWidth: '80px',
  })

  await nextTick(() => {
    permissionTreeRef.value?.setCheckStrictly(!!userModel.value?.id)
    setTimeout(() => {
      permissionTreeRef.value?.elTree?.setCheckedKeys?.(names)
    }, 50)
  })
})

// 保存用户角色
async function saveUserRole(): Promise<any> {
  const id = userModel.value.id as number
  const permissions = permissionTreeRef.value.elTree.getCheckedKeys() as string[]
  const resource = rbacPermissionsResource(id, permissions)
  const evidence = await requestEvidence({ policy_key: 'role.permissions.sync', scope: 'role.permissions.sync', resource, reason: `Sync platform role ${id} permissions` })
  const response = await setRolePermission(id, permissions, evidence)
  if (response.code !== ResultCode.SUCCESS) {
    throw response
  }
  return response
}

defineExpose({
  saveUserRole,
  maForm: userRoleForm,
})
</script>

<template>
  <ma-form ref="userRoleForm" v-model="userModel" />
</template>

<style scoped lang="scss">

</style>
