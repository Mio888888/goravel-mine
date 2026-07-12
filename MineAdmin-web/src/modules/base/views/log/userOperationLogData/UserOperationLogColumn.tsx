/**
 * MineAdmin is committed to providing solutions for quickly building web applications
 * Please view the LICENSE file that was distributed with this source code,
 * For the full copyright and license information.
 * Thank you very much for using MineAdmin.
 *
 * @Author X.Mo<root@imoi.cn>
 * @Link   https://github.com/mineadmin
 */
import type { MaProTableColumns } from '@mineadmin/pro-table'

export default function getColumns(t: any): MaProTableColumns[] {
  return [
    // 索引序号列
    { type: 'index' },
    // 普通列
    { label: () => t('baseOperationLog.username'), prop: 'username' },
    { label: () => t('baseOperationLog.method'), prop: 'method' },
    { label: () => t('baseOperationLog.router'), prop: 'router' },
    { label: () => t('baseOperationLog.service_name'), prop: 'service_name' },
    { label: () => t('baseOperationLog.ip'), prop: 'ip' },
    { label: () => t('baseOperationLog.created_at'), prop: 'created_at' },
  ]
}
