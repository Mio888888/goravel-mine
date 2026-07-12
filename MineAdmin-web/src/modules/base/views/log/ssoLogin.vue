<script setup lang="ts">
import type { MaProTableExpose, MaProTableOptions, MaProTableSchema } from '@mineadmin/pro-table'
import type { Ref } from 'vue'
import type { SSOLoginLogSearchVo, SSOLoginStatsVo } from '~/base/api/ssoLoginLog'
import { page, stats } from '~/base/api/ssoLoginLog'
import getSearchItems from './ssoLoginData/getSearchItems'
import getTableColumns from './ssoLoginData/getTableColumns'

defineOptions({ name: 'security:ssoLoginAudit' })

const t = useTrans().globalTrans
const proTableRef = ref<MaProTableExpose>() as Ref<MaProTableExpose>
const logStats = ref<SSOLoginStatsVo>({
  total: 0,
  success_count: 0,
  fail_count: 0,
  success_rate: 0,
  providers: [],
})

async function loadStats(form: SSOLoginLogSearchVo = {}) {
  const response = await stats(form)
  if (response.data) {
    logStats.value = response.data
  }
}

onMounted(() => {
  loadStats()
})

const options = ref<MaProTableOptions>({
  adaptionOffsetBottom: 225,
  header: {
    mainTitle: () => t('baseSsoLoginLog.mainTitle'),
    subTitle: () => t('baseSsoLoginLog.subTitle'),
  },
  searchOptions: {
    fold: true,
    text: {
      searchBtn: () => t('crud.search'),
      resetBtn: () => t('crud.reset'),
      isFoldBtn: () => t('crud.searchFold'),
      notFoldBtn: () => t('crud.searchUnFold'),
    },
  },
  searchFormOptions: { labelWidth: '100px' },
  requestOptions: {
    api: page,
  },
  onSearchSubmit: (form) => {
    loadStats(form)
    return form
  },
  onSearchReset: (form) => {
    loadStats(form)
    return form
  },
})

const schema = ref<MaProTableSchema>({
  searchItems: getSearchItems(t),
  tableColumns: getTableColumns(t),
})
</script>

<template>
  <div class="mine-layout pt-3">
    <div class="sso-login-stat-grid">
      <div class="sso-login-stat">
        <span>{{ t('baseSsoLoginLog.total') }}</span>
        <strong>{{ logStats.total }}</strong>
      </div>
      <div class="sso-login-stat">
        <span>{{ t('baseSsoLoginLog.successCount') }}</span>
        <strong>{{ logStats.success_count }}</strong>
      </div>
      <div class="sso-login-stat">
        <span>{{ t('baseSsoLoginLog.failCount') }}</span>
        <strong>{{ logStats.fail_count }}</strong>
      </div>
      <div class="sso-login-stat">
        <span>{{ t('baseSsoLoginLog.successRate') }}</span>
        <strong>{{ logStats.success_rate }}%</strong>
      </div>
    </div>
    <MaProTable ref="proTableRef" :options="options" :schema="schema" />
  </div>
</template>

<style scoped lang="scss">
.sso-login-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.sso-login-stat {
  display: flex;
  min-height: 72px;
  flex-direction: column;
  justify-content: center;
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
  background: var(--el-bg-color);
  padding: 12px 16px;
}

.sso-login-stat span {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.sso-login-stat strong {
  margin-top: 6px;
  color: var(--el-text-color-primary);
  font-size: 24px;
  font-weight: 600;
}

@media (max-width: 768px) {
  .sso-login-stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
