import type { Composer } from 'vue-i18n'

type Translator = Composer['t']

export interface LifecycleSelectOption<T = string> {
  label: string
  value: T
}

interface StatusLabelOptions {
  status?: string
  t: Translator
}

interface ActionLabelOptions {
  action?: string
  t: Translator
}

interface BoolLabelOptions {
  value?: boolean
  t: Translator
}

export function statusType(status?: string): 'success' | 'warning' | 'danger' | 'info' {
  switch (status) {
    case 'succeeded':
      return 'success'
    case 'failed':
    case 'lock_blocked':
    case 'manual_required':
    case 'reconciliation_required':
      return 'danger'
    case 'running':
    case 'planned':
      return 'warning'
    default:
      return 'info'
  }
}

export function statusLabel(options: StatusLabelOptions) {
  if (!options.status) {
    return options.t('baseModuleLifecycle.statusUnknown')
  }
  return options.t(`baseModuleLifecycle.status.${options.status}`)
}

export function actionLabel(options: ActionLabelOptions) {
  if (!options.action) {
    return '-'
  }
  return options.t(`baseModuleLifecycle.action.${options.action}`)
}

export function boolLabel(options: BoolLabelOptions) {
  return options.value
    ? options.t('baseModuleLifecycle.yes')
    : options.t('baseModuleLifecycle.no')
}

export function timeLabel(value?: string) {
  return value || '-'
}
