export interface MTabsOptionItems<T> {
  label: string | (() => string)
  value: T
  icon?: string
  [key: string]: any
}

export interface MTabsProps<T> {
  options: MTabsOptionItems<T>[]
  direction?: 'horizontal' | 'vertical'
  align?: 'start' | 'center' | 'end'
}

export interface MTabsEmits {
  (event: 'change', value: any, optionItem: MTabsOptionItems<any>): void
}
