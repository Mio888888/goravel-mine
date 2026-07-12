/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import type { MenuVo } from './menu'
import type { RoleVo } from './role'
import type { ResponseStruct } from '#/global'
import type { OperationResponse } from '@/generated/admin-api'
import { operations } from '@/generated/admin-api'

type SuccessData<T extends { data: unknown }> = Extract<T, { code: 200 }>['data']
type PermissionMenusData = SuccessData<OperationResponse<'adminPermissionMenus'>>
type PermissionRolesData = SuccessData<OperationResponse<'adminPermissionRoles'>>

/**
 * Get Current User's Menu
 */
export function getMenus(): Promise<ResponseStruct<MenuVo[] & PermissionMenusData>> {
  return useHttp().get(operations.adminPermissionMenus.path)
}

/**
 * Get Current User's Roles
 */
export function getRoles(): Promise<ResponseStruct<RoleVo[] & PermissionRolesData>> {
  return useHttp().get(operations.adminPermissionRoles.path)
}

export {
  MenuVo,
  RoleVo,
}
