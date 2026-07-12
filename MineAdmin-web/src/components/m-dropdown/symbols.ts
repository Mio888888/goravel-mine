import type { InjectionKey } from 'vue'

const DropdownContextInjectionKey: InjectionKey<{
  hide: () => void
}> = Symbol('dropdown-context')

export default DropdownContextInjectionKey
