import type { MenuVo } from '~/base/api/menu.ts'

export function allPermissionNames(menus: MenuVo[]): string[] {
  const names = new Set<string>()
  walkPermissionTree(menus, (item) => {
    if (item.name) {
      names.add(item.name)
    }
  })
  return Array.from(names).sort()
}

export function includePermissionAncestors(menus: MenuVo[], permissions: string[]): string[] {
  const selected = new Set(permissions.filter(Boolean))
  const parentByName = permissionParentByName(menus)

  Array.from(selected).forEach((name) => {
    let parent = parentByName.get(name)
    while (parent) {
      selected.add(parent)
      parent = parentByName.get(parent)
    }
  })

  return Array.from(selected).sort()
}

function permissionParentByName(menus: MenuVo[]): Map<string, string> {
  const byID = new Map<number, MenuVo>()
  walkPermissionTree(menus, (item) => {
    if (typeof item.id === 'number') {
      byID.set(item.id, item)
    }
  })

  const parentByName = new Map<string, string>()
  walkPermissionTree(menus, (item) => {
    if (!item.name || typeof item.parent_id !== 'number' || item.parent_id === 0) {
      return
    }
    const parent = byID.get(item.parent_id)
    if (parent?.name) {
      parentByName.set(item.name, parent.name)
    }
  })
  return parentByName
}

function walkPermissionTree(items: MenuVo[], visitor: (item: MenuVo) => void) {
  items.forEach((item) => {
    visitor(item)
    if (Array.isArray(item.children)) {
      walkPermissionTree(item.children, visitor)
    }
    if (Array.isArray(item.btnPermission)) {
      walkPermissionTree(item.btnPermission, visitor)
    }
  })
}
