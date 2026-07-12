import type { MaFormItem } from '@mineadmin/form'
import type { MenuVo } from '~/base/api/menu.ts'
import type { TenantPlanVo } from '~/base/api/platformTenantPlan.ts'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'
import MaTree from '@/components/ma-tree/index.vue'
import { allPermissionNames, includePermissionAncestors } from '../../tenant/permissionTree.ts'

function ensureDefaults(model: TenantPlanVo) {
  model.status = model.status || 1
  model.sort = model.sort || 0
  model.billing = model.billing ?? {}
  model.quotas = model.quotas ?? {}
  model.features = model.features ?? {}
  model.billing.subscription_status = model.billing.subscription_status ?? 'active'
  model.billing.currency = model.billing.currency ?? 'CNY'
  model.quotas.api_rate_per_minute = model.quotas.api_rate_per_minute ?? 600
  model.quotas.max_users = model.quotas.max_users ?? 0
  model.quotas.max_roles = model.quotas.max_roles ?? 0
  model.quotas.max_storage_mb = model.quotas.max_storage_mb ?? 0
}

export default function getFormItems(
  t: any,
  model: TenantPlanVo,
  menus: MenuVo[] = [],
  externalPermissionTreeRef?: { value: any },
): MaFormItem[] {
  ensureDefaults(model)
  const permissionTreeRef = externalPermissionTreeRef ?? ref<any>()
  const defaultPermissions = allPermissionNames(menus)
  const checkedPermissions = computed(() => model.features?.permissions?.allowed ?? defaultPermissions)

  if (!Array.isArray(model.features?.permissions?.allowed)) {
    model.features = model.features ?? {}
    model.features.permissions = {
      ...(model.features.permissions ?? {}),
      allowed: defaultPermissions,
    }
  }

  setTimeout(() => {
    permissionTreeRef.value?.elTree?.setCheckedKeys?.(checkedPermissions.value)
  }, 50)

  const syncPermissions = () => {
    const checked = permissionTreeRef.value?.elTree?.getCheckedKeys?.() as string[] ?? []
    const halfChecked = permissionTreeRef.value?.elTree?.getHalfCheckedKeys?.() as string[] ?? []
    model.features = model.features ?? {}
    model.features.permissions = {
      ...(model.features.permissions ?? {}),
      allowed: includePermissionAncestors(menus, [...checked, ...halfChecked]),
    }
  }

  return [
    {
      label: () => t('baseTenantPlanManage.code'),
      prop: 'code',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantPlanManage.code') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('baseTenantPlanManage.code') }) }],
      },
    },
    {
      label: () => t('baseTenantPlanManage.name'),
      prop: 'name',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantPlanManage.name') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('baseTenantPlanManage.name') }) }],
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
      renderProps: {
        class: 'w-full',
        min: 0,
      },
    },
    {
      label: () => t('baseTenantManage.apiRatePerMinute'),
      prop: 'quotas.api_rate_per_minute',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: {
        class: 'w-full',
        min: 0,
      },
    },
    {
      label: () => t('baseTenantManage.maxUsers'),
      prop: 'quotas.max_users',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: {
        class: 'w-full',
        min: 0,
      },
    },
    {
      label: () => t('baseTenantManage.maxRoles'),
      prop: 'quotas.max_roles',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: {
        class: 'w-full',
        min: 0,
      },
    },
    {
      label: () => t('baseTenantManage.maxStorageMb'),
      prop: 'quotas.max_storage_mb',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: {
        class: 'w-full',
        min: 0,
      },
    },
    {
      label: () => t('baseTenantManage.subscriptionStatus'),
      prop: 'billing.subscription_status',
      render: () => MaDictSelect,
      cols: { md: 12, xs: 24 },
      renderProps: {
        clearable: false,
        dictName: 'tenant-subscription-status',
      },
    },
    {
      label: () => t('baseTenantManage.currency'),
      prop: 'billing.currency',
      render: 'input',
      cols: { md: 12, xs: 24 },
    },
    {
      label: () => t('baseTenantManage.allowedPermissions'),
      prop: 'features.permissions.allowed',
      render: () => MaTree,
      cols: { xs: 24 },
      renderProps: {
        ref: (el: any) => permissionTreeRef.value = el,
        class: 'w-full',
        showCheckbox: true,
        treeKey: 'meta.title',
        nodeKey: 'name',
        data: menus,
        onCheck: syncPermissions,
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
    },
    {
      label: () => t('crud.remark'),
      prop: 'remark',
      render: 'input',
      cols: { xs: 24 },
      renderProps: {
        type: 'textarea',
        placeholder: t('form.pleaseInput', { msg: t('crud.remark') }),
      },
    },
  ]
}
