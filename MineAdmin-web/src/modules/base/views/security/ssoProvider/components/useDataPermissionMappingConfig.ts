export type PolicyType = 'ALL' | 'DEPT_TREE' | 'DEPT_SELF' | 'SELF' | 'CUSTOM_DEPT' | 'CUSTOM_FUNC'
export type DataPermissionMappingValue = PolicyType | {
  condition?: string
  policy_type?: PolicyType
  value?: any
}

export interface DataPermissionMappingConfig {
  claim: string
  mapping: Record<string, DataPermissionMappingValue>
  default: PolicyType
}

export interface DataPermissionMappingItem {
  key: string
  valueType: PolicyType
  customValue?: any
  condition: string
  isConditional: boolean
}

export const simplePolicyTypes: PolicyType[] = ['ALL', 'DEPT_TREE', 'DEPT_SELF', 'SELF']
export const policyTypes: PolicyType[] = ['ALL', 'DEPT_TREE', 'DEPT_SELF', 'SELF', 'CUSTOM_DEPT', 'CUSTOM_FUNC']

export function defaultDataPermissionMappingConfig(): DataPermissionMappingConfig {
  return {
    claim: 'role',
    mapping: {},
    default: 'SELF',
  }
}

export function parsePolicyType(value: any, fallback: PolicyType = 'SELF'): PolicyType {
  return policyTypes.includes(value) ? value : fallback
}

export function parseDataPermissionMappingConfig(value?: string | Record<string, any> | null): DataPermissionMappingConfig {
  if (!value) {
    return defaultDataPermissionMappingConfig()
  }
  const source = typeof value === 'string' ? JSON.parse(value || '{}') : value
  return {
    claim: typeof source.claim === 'string' && source.claim ? source.claim : 'role',
    mapping: typeof source.mapping === 'object' && source.mapping ? source.mapping : {},
    default: parsePolicyType(source.default),
  }
}

export function dataPermissionMappingItems(config: DataPermissionMappingConfig): DataPermissionMappingItem[] {
  return Object.entries(config.mapping || {}).map(([key, value]) => {
    if (typeof value === 'object' && value !== null) {
      return {
        key,
        valueType: parsePolicyType(value.policy_type),
        customValue: value.value,
        condition: value.condition || '',
        isConditional: Object.prototype.hasOwnProperty.call(value, 'condition'),
      }
    }
    return {
      key,
      valueType: parsePolicyType(value),
      customValue: undefined,
      condition: '',
      isConditional: false,
    }
  })
}

export function dataPermissionMappingFromItems(
  items: DataPermissionMappingItem[],
): Record<string, DataPermissionMappingValue> {
  const mapping: Record<string, DataPermissionMappingValue> = {}
  items.forEach((item) => {
    const key = item.key.trim()
    if (!key) {
      return
    }
    if (item.isConditional) {
      mapping[key] = {
        condition: item.condition.trim(),
        policy_type: item.valueType,
        value: item.customValue,
      }
      return
    }
    if (simplePolicyTypes.includes(item.valueType)) {
      mapping[key] = item.valueType
      return
    }
    mapping[key] = {
      policy_type: item.valueType,
      value: item.customValue,
    }
  })
  return mapping
}
