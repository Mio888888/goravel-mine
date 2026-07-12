<script setup lang="ts">
import type { MaFormExpose } from '@mineadmin/form'
import type { PlatformUserVo } from '~/base/api/platformUser'
import { create, save } from '~/base/api/platformUser'
import getFormItems from './data/getFormItems.tsx'
import useForm from '@/hooks/useForm.ts'
import { ResultCode } from '@/utils/ResultCode.ts'

defineOptions({ name: 'platform:user:form' })

const { formType = 'add', data = null } = defineProps<{
  formType?: 'add' | 'edit'
  data?: PlatformUserVo | null
}>()

const t = useTrans().globalTrans
const userForm = ref<MaFormExpose>()
const userModel = ref<PlatformUserVo>({})

useForm('platformUserForm').then((form: MaFormExpose) => {
  if (formType === 'edit' && data) {
    Object.keys(data).map((key: string) => {
      userModel.value[key] = data[key]
    })
  }
  form.setItems(getFormItems(formType, t, userModel.value))
  form.setOptions({
    labelWidth: '90px',
  })
})

function add(): Promise<any> {
  return new Promise((resolve, reject) => {
    create(userModel.value).then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch(reject)
  })
}

function edit(): Promise<any> {
  return new Promise((resolve, reject) => {
    save(userModel.value.id as number, userModel.value).then((res: any) => {
      res.code === ResultCode.SUCCESS ? resolve(res) : reject(res)
    }).catch(reject)
  })
}

defineExpose({
  add,
  edit,
  maForm: userForm,
})
</script>

<template>
  <ma-form ref="userForm" v-model="userModel" />
</template>
