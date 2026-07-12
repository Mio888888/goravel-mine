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
import { ElTag } from 'element-plus'

export default function getColumns(t: any): MaProTableColumns[] {
  const dictStore = useDictStore()

  return [
    // 索引序号列
    { type: 'index' },
    // 普通列
    { label: () => t('baseLoginLog.username'), prop: 'username' },
    // { label: () => t('baseLoginLog.os'), prop: 'os' },
    { label: () => t('baseLoginLog.ip'), prop: 'ip' },
    { label: () => t('baseLoginLog.browser'), prop: 'browser' },
    { label: () => t('baseLoginLog.status'), prop: 'status',
      cellRender: ({ row }) => (
        <ElTag type={dictStore.t('system-state', row.status, 'color')}>
          {t(dictStore.t('system-state', row.status, 'i18n'))}
        </ElTag>
      ),
    },
    { label: () => t('baseLoginLog.login_time'), prop: 'login_time' },
  ]
}
