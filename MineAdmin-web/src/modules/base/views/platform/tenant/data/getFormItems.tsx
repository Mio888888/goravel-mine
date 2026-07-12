import type { MaFormItem } from '@mineadmin/form'
import type { TenantPlanOptionVo, TenantVo } from '~/base/api/tenant.ts'
import MaDictRadio from '@/components/ma-dict-picker/ma-dict-radio.vue'
import MaDictSelect from '@/components/ma-dict-picker/ma-dict-select.vue'

export type TenantFormTab
  = | 'basic'
    | 'database'
    | 'billing'
    | 'quota'
    | 'branding'
    | 'other'

export interface TenantFormTabOption {
  label: string
  name: TenantFormTab
}

export function getTenantFormTabs(t: any): TenantFormTabOption[] {
  return [
    { label: t('baseTenantManage.basicInfo'), name: 'basic' },
    { label: t('baseTenantManage.databaseInfo'), name: 'database' },
    { label: t('baseTenantManage.billingInfo'), name: 'billing' },
    { label: t('baseTenantManage.quotaInfo'), name: 'quota' },
    { label: t('baseTenantManage.brandingInfo'), name: 'branding' },
    { label: t('baseTenantManage.otherInfo'), name: 'other' },
  ]
}

function ensureEnterpriseDefaults(model: TenantVo) {
  model.branding = model.branding ?? {}
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
  model: TenantVo,
  activeTab: TenantFormTab = 'basic',
  plans: TenantPlanOptionVo[] = [],
  onPlanChange?: (planCode?: string) => void,
): MaFormItem[] {
  model.status = model.status || 1
  model.plan = model.plan || plans[0]?.code || 'standard'
  model.db_port = model.db_port || 5432
  model.db_schema = model.db_schema || 'public'
  model.initialize = model.initialize || false
  ensureEnterpriseDefaults(model)

  const groups: Record<TenantFormTab, MaFormItem[]> = {
    basic: [{
      label: () => t('baseTenantManage.code'),
      prop: 'code',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.code') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('baseTenantManage.code') }) }],
      },
    },
    {
      label: () => t('baseTenantManage.name'),
      prop: 'name',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.name') }),
      },
      itemProps: {
        rules: [{ required: true, message: t('form.requiredInput', { msg: t('baseTenantManage.name') }) }],
      },
    },
    {
      label: () => t('baseTenantManage.customDomain'),
      prop: 'custom_domain',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.customDomain') }),
      },
    },
    {
      label: () => t('baseTenantManage.plan'),
      prop: 'plan',
      render: () => MaDictSelect,
      cols: { md: 12, xs: 24 },
      renderProps: {
        clearable: false,
        data: plans.map(item => ({ label: item.name || item.code, value: item.code })),
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.plan') }),
        onChange: onPlanChange,
      },
    },
    {
      label: () => t('crud.status'),
      prop: 'status',
      render: () => MaDictRadio,
      cols: { md: 12, xs: 24 },
      renderProps: {
        dictName: 'tenant-status',
      },
    }],
    database: [{
      label: () => t('baseTenantManage.dbHost'),
      prop: 'db_host',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.dbHost') }),
      },
    },
    {
      label: () => t('baseTenantManage.dbPort'),
      prop: 'db_port',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: {
        class: 'w-full',
        min: 1,
        max: 65535,
      },
    },
    {
      label: () => t('baseTenantManage.dbDatabase'),
      prop: 'db_database',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.dbDatabase') }),
      },
    },
    {
      label: () => t('baseTenantManage.dbUsername'),
      prop: 'db_username',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.dbUsername') }),
      },
    },
    {
      label: () => t('baseTenantManage.dbPassword'),
      prop: 'db_password',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        type: 'password',
        showPassword: true,
        autocomplete: 'new-password',
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.dbPassword') }),
      },
    },
    {
      label: () => t('baseTenantManage.dbSchema'),
      prop: 'db_schema',
      render: 'input',
      cols: { md: 12, xs: 24 },
      renderProps: {
        placeholder: t('form.pleaseInput', { msg: t('baseTenantManage.dbSchema') }),
      },
    },
    {
      label: () => t('baseTenantManage.initialize'),
      prop: 'initialize',
      render: 'switch',
      renderProps: {
        activeValue: true,
        inactiveValue: false,
      },
    }],
    billing: [{
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
      renderProps: {
        placeholder: 'CNY',
      },
    },
    {
      label: () => t('baseTenantManage.amountCents'),
      prop: 'billing.amount_cents',
      render: 'inputNumber',
      cols: { md: 12, xs: 24 },
      renderProps: {
        class: 'w-full',
        min: 0,
        step: 100,
      },
    },
    {
      label: () => t('baseTenantManage.expiresAt'),
      prop: 'billing.expires_at',
      render: 'datePicker',
      cols: { md: 12, xs: 24 },
      renderProps: {
        class: 'w-full',
        type: 'datetime',
        valueFormat: 'YYYY-MM-DDTHH:mm:ss[Z]',
      },
    }],
    quota: [{
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
    }],
    branding: [{
      label: () => t('baseTenantManage.appName'),
      prop: 'branding.app_name',
      render: 'input',
      cols: { md: 12, xs: 24 },
    },
    {
      label: () => t('baseTenantManage.logoUrl'),
      prop: 'branding.logo_url',
      render: 'input',
      cols: { md: 12, xs: 24 },
    },
    {
      label: () => t('baseTenantManage.primaryColor'),
      prop: 'branding.primary_color',
      render: 'colorPicker',
      cols: { md: 12, xs: 24 },
    },
    {
      label: () => t('baseTenantManage.mailFromName'),
      prop: 'branding.mail_from_name',
      render: 'input',
      cols: { md: 12, xs: 24 },
    }],
    other: [{
      label: () => t('crud.remark'),
      prop: 'remark',
      render: 'input',
      cols: { xs: 24 },
      renderProps: {
        type: 'textarea',
        placeholder: t('form.pleaseInput', { msg: t('crud.remark') }),
      },
    }],
  }

  return groups[activeTab]
}
