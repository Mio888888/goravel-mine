import type { Dictionary } from '#/global'

export function dictItem(dictName: string, value: any): Dictionary | null {
  const normalizedValue = String(value ?? '')
  return useDictStore().find(dictName)?.find(item => String(item.value ?? '') === normalizedValue) ?? null
}

export function dictLabel(dictName: string, value: any, t?: (key: string) => string): string {
  const item = dictItem(dictName, value)
  if (!item) {
    return String(value ?? '')
  }
  return item.i18n && t ? t(item.i18n) : item.label ?? String(value ?? '')
}

export function dictColor(dictName: string, value: any): string {
  return dictItem(dictName, value)?.color ?? ''
}
