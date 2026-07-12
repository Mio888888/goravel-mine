import type { MaFormItem } from '@mineadmin/form'
import type { PlatformUserVo } from '~/base/api/platformUser.ts'
import MaUploadImage from '@/components/ma-upload-image/index.vue'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'

export default function getFormItems(
  formType: 'add' | 'edit' = 'add',
  t: any,
  model: PlatformUserVo,
): MaFormItem[] {
  if (formType === 'add') {
    model.password = '123456'
    model.status = 1
    model.user_type = 900
    model.dashboard = 'platform:tenant'
  }

  model.backend_setting = []

  return [
    {
      label: () => t('basePlatformUserManage.avatar'),
      prop: 'avatar',
      render: () => MaUploadImage,
      renderProps: {
        actionUrl: '/admin/platform/attachment/upload',
      },
    },
    {
      label: () => t('basePlatformUserManage.username'),
      prop: 'username',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        disabled: formType === 'edit',
        placeholder: t('form.pleaseInput', { msg: t('basePlatformUserManage.username') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('basePlatformUserManage.username') }) }],
      },
    },
    {
      label: () => t('basePlatformUserManage.nickname'),
      prop: 'nickname',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('basePlatformUserManage.nickname') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('basePlatformUserManage.nickname') }) }],
      },
    },
    {
      label: () => t('basePlatformUserManage.password'),
      prop: 'password',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        disabled: formType === 'edit',
        placeholder: t('form.pleaseInput', { msg: t('basePlatformUserManage.password') }),
      },
      itemProps: {
        rules: formType === 'add' ? [{ required: true, message: t('form.requiredInput', { msg: t('basePlatformUserManage.password') }) }] : [],
      },
    },
    {
      label: () => t('basePlatformUserManage.phone'),
      prop: 'phone',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('basePlatformUserManage.phone') }),
      },
    },
    {
      label: () => t('basePlatformUserManage.email'),
      prop: 'email',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('basePlatformUserManage.email') }),
      },
    },
    {
      label: () => t('basePlatformUserManage.dashboard'),
      prop: 'dashboard',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('basePlatformUserManage.dashboard') }),
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
      label: () => t('crud.remark'),
      prop: 'remark',
      render: 'input',
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('crud.remark') }),
        type: 'textarea',
      },
    },
  ]
}
