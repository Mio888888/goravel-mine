import type { PageList, ResponseStruct } from '#/global'
import type {
  ModuleLifecycleDiffVo,
  ModuleLifecycleLockVo,
  ModuleLifecycleRunVo,
  ModuleLifecycleStateVo,
  ModuleLifecycleStepVo,
} from '~/base/api/platformModuleLifecycle'
import type { Ref } from 'vue'
import { locks, runs, stateDiff, states, steps } from '~/base/api/platformModuleLifecycle'
import { useMessage } from '@/hooks/useMessage.ts'
import { ResultCode } from '@/utils/ResultCode.ts'
import { computed, reactive, ref } from 'vue'

export interface LifecycleRunQuery {
  run_key: string
  module_id: string
  action: string
  status: string
  owner: string
  page: number
  page_size: number
}

export interface LifecycleStepQuery {
  run_key: string
  module_id: string
  action: string
  status: string
  page: number
  page_size: number
}

interface UseModuleLifecycleStateOptions {
  canViewStepLogs: Readonly<Ref<boolean>>
}

interface ReadRequest<T> {
  loading: Ref<boolean>
  request: () => Promise<ResponseStruct<PageList<T>>>
  apply: (page: PageList<T>) => void
}

const queryDefaults = { page: 1, page_size: 15 }

function createLifecycleStateStore() {
  const stateLoading = ref(false)
  const runLoading = ref(false)
  const stepLoading = ref(false)
  const lockLoading = ref(false)
  const diffLoading = ref(false)
  return {
    stateLoading, runLoading, stepLoading, lockLoading, diffLoading,
    stateRows: ref<ModuleLifecycleStateVo[]>([]),
    runRows: ref<ModuleLifecycleRunVo[]>([]),
    stepRows: ref<ModuleLifecycleStepVo[]>([]),
    lockRows: ref<ModuleLifecycleLockVo[]>([]),
    diffRows: ref<ModuleLifecycleDiffVo[]>([]),
    runTotal: ref(0),
    stepTotal: ref(0),
    runQuery: reactive<LifecycleRunQuery>({
      run_key: '', module_id: '', action: '', status: '', owner: '', ...queryDefaults,
    }),
    stepQuery: reactive<LifecycleStepQuery>({
      run_key: '', module_id: '', action: '', status: '', ...queryDefaults,
    }),
    refreshing: computed(() => [
      stateLoading.value, runLoading.value, stepLoading.value,
      lockLoading.value, diffLoading.value,
    ].some(Boolean)),
  }
}

type LifecycleStateStore = ReturnType<typeof createLifecycleStateStore>
type LifecycleMessage = ReturnType<typeof useMessage>

interface LifecycleReadContext {
  store: LifecycleStateStore
  canViewStepLogs: Readonly<Ref<boolean>>
  msg: LifecycleMessage
}

async function loadReadModel<T>(request: ReadRequest<T>, msg: LifecycleMessage) {
  request.loading.value = true
  try {
    const response = await request.request()
    if (response.code !== ResultCode.SUCCESS) {
      msg.error(response.message)
      return
    }
    request.apply({
      list: response.data.list ?? [],
      total: response.data.total ?? 0,
    })
  }
  finally {
    request.loading.value = false
  }
}

function loadState(context: LifecycleReadContext) {
  return loadReadModel({
    loading: context.store.stateLoading,
    request: states,
    apply: page => context.store.stateRows.value = page.list,
  }, context.msg)
}

function loadRuns(context: LifecycleReadContext) {
  const { store } = context
  return loadReadModel({
    loading: store.runLoading,
    request: () => runs({ ...store.runQuery }),
    apply: (page) => {
      store.runRows.value = page.list
      store.runTotal.value = page.total
    },
  }, context.msg)
}

function loadSteps(context: LifecycleReadContext) {
  const { store } = context
  if (!context.canViewStepLogs.value) {
    store.stepRows.value = []
    store.stepTotal.value = 0
    return Promise.resolve()
  }
  return loadReadModel({
    loading: store.stepLoading,
    request: () => steps({ ...store.stepQuery }),
    apply: (page) => {
      store.stepRows.value = page.list
      store.stepTotal.value = page.total
    },
  }, context.msg)
}

function loadLocks(context: LifecycleReadContext) {
  return loadReadModel({
    loading: context.store.lockLoading,
    request: locks,
    apply: page => context.store.lockRows.value = page.list,
  }, context.msg)
}

function loadDiff(context: LifecycleReadContext) {
  return loadReadModel({
    loading: context.store.diffLoading,
    request: stateDiff,
    apply: page => context.store.diffRows.value = page.list,
  }, context.msg)
}

function resetRunQuery(context: LifecycleReadContext) {
  Object.assign(context.store.runQuery, {
    run_key: '', module_id: '', action: '', status: '', owner: '', ...queryDefaults,
  })
  return loadRuns(context)
}

function resetStepQuery(context: LifecycleReadContext) {
  Object.assign(context.store.stepQuery, {
    run_key: '', module_id: '', action: '', status: '', ...queryDefaults,
  })
  return loadSteps(context)
}

function searchRuns(context: LifecycleReadContext) {
  context.store.runQuery.page = 1
  return loadRuns(context)
}

function searchSteps(context: LifecycleReadContext) {
  context.store.stepQuery.page = 1
  return loadSteps(context)
}

function selectRunSteps(context: LifecycleReadContext, row: ModuleLifecycleRunVo) {
  Object.assign(context.store.stepQuery, {
    run_key: row.idempotency_key,
    module_id: row.module_id,
    action: row.action,
    status: '',
    page: 1,
  })
  return loadSteps(context)
}

async function refreshAll(context: LifecycleReadContext) {
  const requests = [loadState(context), loadRuns(context), loadLocks(context), loadDiff(context)]
  if (context.canViewStepLogs.value) {
    requests.push(loadSteps(context))
  }
  await Promise.all(requests)
}

export function useModuleLifecycleState(options: UseModuleLifecycleStateOptions) {
  const store = createLifecycleStateStore()
  const context = { store, canViewStepLogs: options.canViewStepLogs, msg: useMessage() }
  return {
    ...store,
    loadState: () => loadState(context),
    loadRuns: () => loadRuns(context),
    loadSteps: () => loadSteps(context),
    loadLocks: () => loadLocks(context),
    loadDiff: () => loadDiff(context),
    resetRunQuery: () => resetRunQuery(context),
    resetStepQuery: () => resetStepQuery(context),
    searchRuns: () => searchRuns(context),
    searchSteps: () => searchSteps(context),
    selectRunSteps: (row: ModuleLifecycleRunVo) => selectRunSteps(context, row),
    refreshAll: () => refreshAll(context),
  }
}
