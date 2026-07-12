<script setup lang="ts">
import type { ModuleLifecycleAction, ModuleLifecycleRunVo } from '~/base/api/platformModuleLifecycle'
import type { EvidenceDialogExpose } from './sensitiveOperation'
import hasAuth from '@/utils/permission/hasAuth.ts'
import { useI18n } from 'vue-i18n'
import SensitiveOperationEvidenceDialog from '~/base/components/SensitiveOperationEvidenceDialog.vue'
import DiffTab from './components/DiffTab.vue'
import LifecycleActionPanel from './components/LifecycleActionPanel.vue'
import LifecycleResultPanel from './components/LifecycleResultPanel.vue'
import LocksTab from './components/LocksTab.vue'
import RunsTab from './components/RunsTab.vue'
import StateTab from './components/StateTab.vue'
import StepsTab from './components/StepsTab.vue'
import { statusLabel } from './presentation'
import { useLifecycleExecution } from './useLifecycleExecution'
import { useModuleLifecycleState } from './useModuleLifecycleState'
import { useStaleLockRelease } from './useStaleLockRelease'

defineOptions({ name: 'platform:moduleLifecycle' })

const { t } = useI18n()
const activeTab = ref('state')
const canViewStepLogs = computed(() => hasAuth('platform:moduleLifecycle:log'))
const evidenceDialogRef = ref<EvidenceDialogExpose>()
const {
  stateRows, runRows, stepRows, lockRows, diffRows,
  runTotal, stepTotal, runQuery, stepQuery,
  stateLoading, runLoading, stepLoading, lockLoading, diffLoading, refreshing,
  loadRuns, loadSteps, loadLocks, resetRunQuery, resetStepQuery,
  searchRuns, searchSteps, selectRunSteps, refreshAll,
} = useModuleLifecycleState({ canViewStepLogs })
const {
  form: actionForm,
  result,
  loading: actionLoading,
  expectedConfirmToken,
  moduleOptions,
  submit: submitAction,
} = useLifecycleExecution({ stateRows, evidenceDialog: evidenceDialogRef, refreshAll })
const {
  form: lockReleaseForm,
  loading: releaseLoading,
  submit: submitLockRelease,
} = useStaleLockRelease({ evidenceDialog: evidenceDialogRef, lockRows, loadLocks })

const actionOptions = computed<Array<{ label: string, value: ModuleLifecycleAction }>>(() => [
  { label: t('baseModuleLifecycle.actionInstall'), value: 'install' },
  { label: t('baseModuleLifecycle.actionUpgrade'), value: 'upgrade' },
  { label: t('baseModuleLifecycle.actionRollback'), value: 'rollback' },
  { label: t('baseModuleLifecycle.actionUninstall'), value: 'uninstall' },
])
const statusOptions = computed(() => [
  'planned', 'running', 'succeeded', 'failed', 'skipped', 'lock_blocked',
  'manual_required', 'reconciliation_required',
].map(value => ({ label: statusLabel({ status: value, t }), value })))
const stateSummary = computed(() => [
  { label: t('baseModuleLifecycle.totalModules'), value: stateRows.value.length },
  { label: t('baseModuleLifecycle.persistedModules'), value: stateRows.value.filter(item => item.persisted).length },
  { label: t('baseModuleLifecycle.disabledModules'), value: stateRows.value.filter(item => !item.enabled).length },
  { label: t('baseModuleLifecycle.failedModules'), value: stateRows.value.filter(item => item.persisted?.last_error).length },
])

async function viewRunSteps(row: ModuleLifecycleRunVo) {
  if (!canViewStepLogs.value) {
    return
  }
  activeTab.value = 'steps'
  await selectRunSteps(row)
}

function resizeRuns() {
  runQuery.page = 1
  return loadRuns()
}

function resizeSteps() {
  stepQuery.page = 1
  return loadSteps()
}

onMounted(refreshAll)
</script>

<template>
  <div v-loading="stateLoading" class="mine-layout module-lifecycle-page">
    <div class="module-lifecycle-toolbar">
      <div>
        <h2>{{ t('baseModuleLifecycle.title') }}</h2>
        <p>{{ t('baseModuleLifecycle.subtitle') }}</p>
      </div>
      <el-button type="primary" :loading="refreshing" @click="refreshAll">
        {{ t('baseModuleLifecycle.refresh') }}
      </el-button>
    </div>

    <div class="module-lifecycle-stats">
      <div v-for="item in stateSummary" :key="item.label" class="module-lifecycle-stat">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
      </div>
    </div>

    <LifecycleActionPanel
      :form="actionForm"
      :action-options="actionOptions"
      :module-options="moduleOptions"
      :expected-confirm-token="expectedConfirmToken"
      :loading="actionLoading"
      @submit="submitAction"
    />

    <el-tabs v-model="activeTab" class="module-lifecycle-tabs">
      <el-tab-pane :label="t('baseModuleLifecycle.stateTab')" name="state">
        <StateTab :rows="stateRows" />
      </el-tab-pane>
      <el-tab-pane :label="t('baseModuleLifecycle.runsTab')" name="runs">
        <RunsTab
          :query="runQuery"
          :rows="runRows"
          :total="runTotal"
          :loading="runLoading"
          :module-options="moduleOptions"
          :action-options="actionOptions"
          :status-options="statusOptions"
          :can-view-step-logs="canViewStepLogs"
          @search="searchRuns"
          @reset="resetRunQuery"
          @page-change="loadRuns"
          @size-change="resizeRuns"
          @view-steps="viewRunSteps"
        />
      </el-tab-pane>
      <el-tab-pane v-if="canViewStepLogs" :label="t('baseModuleLifecycle.stepsTab')" name="steps">
        <StepsTab
          :query="stepQuery"
          :rows="stepRows"
          :total="stepTotal"
          :loading="stepLoading"
          :module-options="moduleOptions"
          :action-options="actionOptions"
          :status-options="statusOptions"
          :can-view-step-logs="canViewStepLogs"
          @search="searchSteps"
          @reset="resetStepQuery"
          @page-change="loadSteps"
          @size-change="resizeSteps"
        />
      </el-tab-pane>
      <el-tab-pane :label="t('baseModuleLifecycle.locksTab')" name="locks">
        <LocksTab
          :form="lockReleaseForm"
          :rows="lockRows"
          :loading="lockLoading"
          :release-loading="releaseLoading"
          @submit="submitLockRelease"
        />
      </el-tab-pane>
      <el-tab-pane :label="t('baseModuleLifecycle.diffTab')" name="diff">
        <DiffTab :rows="diffRows" :loading="diffLoading" />
      </el-tab-pane>
    </el-tabs>

    <LifecycleResultPanel v-if="result" :result="result" />
    <SensitiveOperationEvidenceDialog ref="evidenceDialogRef" />
  </div>
</template>

<style src="./moduleLifecycle.scss" lang="scss"></style>
