export type RoleMappingValue = string | string[] | {
  condition?: string
  roles?: string | string[]
}

export interface RoleMappingConfig {
  claim: string
  mapping: Record<string, RoleMappingValue>
  default: string[]
}

export interface RoleMappingItem {
  key: string
  roles: string[]
  condition: string
  isConditional: boolean
}

export function defaultRoleMappingConfig(): RoleMappingConfig {
  return {
    claim: 'role',
    mapping: {},
    default: [],
  }
}

export function toStringArray(value: any): string[] {
  if (Array.isArray(value)) {
    return value.map(item => String(item)).filter(Boolean)
  }
  if (typeof value === 'string' && value) {
    return [value]
  }
  return []
}

export function parseRoleMappingConfig(value?: string | Record<string, any> | null): RoleMappingConfig {
  if (!value) {
    return defaultRoleMappingConfig()
  }
  const source = typeof value === 'string' ? JSON.parse(value || '{}') : value
  return {
    claim: typeof source.claim === 'string' && source.claim ? source.claim : 'role',
    mapping: typeof source.mapping === 'object' && source.mapping ? source.mapping : {},
    default: toStringArray(source.default),
  }
}

export function roleMappingItems(config: RoleMappingConfig): RoleMappingItem[] {
  return Object.entries(config.mapping || {}).map(([key, value]) => {
    if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
      return {
        key,
        roles: toStringArray(value.roles),
        condition: value.condition || '',
        isConditional: Object.prototype.hasOwnProperty.call(value, 'condition'),
      }
    }
    return {
      key,
      roles: toStringArray(value),
      condition: '',
      isConditional: false,
    }
  })
}

export function roleMappingFromItems(items: RoleMappingItem[]): Record<string, RoleMappingValue> {
  const mapping: Record<string, RoleMappingValue> = {}
  items.forEach((item) => {
    const key = item.key.trim()
    const roles = item.roles.filter(Boolean)
    if (!key || roles.length === 0) {
      return
    }
    if (item.isConditional && item.condition.trim()) {
      mapping[key] = { condition: item.condition.trim(), roles }
      return
    }
    mapping[key] = roles.length === 1 ? roles[0] : roles
  })
  return mapping
}
