import type { MaybeRefOrGetter } from 'vue'
import type { PingRecord, PingTaskInfo } from '@/utils/rpc'
import { computed, onScopeDispose, ref, shallowRef, toValue, watch } from 'vue'
import { loadPingRecordsWithTasks } from '@/services/history.service'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/helper'
import { subscribeRealtimeEvents } from '@/utils/realtime'

export interface PingTaskBar {
  key: string
  className: string
  tooltip: string
}

export interface PingTaskRow {
  id: string
  name: string
  color: string
  latencyDisplay: string
  lossDisplay: string
  latencyBars: PingTaskBar[]
  lossBars: PingTaskBar[]
}

interface PingHistoryPoint {
  time: string
  latency: number | null
  loss: number | null
}

interface SharedPingTaskState {
  recordsByClient: Map<string, PingRecord[]>
  tasks: PingTaskInfo[]
}

interface SharedPingTaskEntry {
  data: ReturnType<typeof shallowRef<SharedPingTaskState | null>>
  promise: Promise<void> | null
  unsubscribeRealtime: (() => void) | null
  subscribers: number
  lastFetchedAt: number
}

const MAX_BAR_COUNT = 20
const TASK_COLORS = ['#fb7185', '#60a5fa', '#34d399', '#a78bfa', '#f59e0b', '#22d3ee']
const sharedEntries = new Map<number, SharedPingTaskEntry>()

function getEntry(hours: number): SharedPingTaskEntry {
  const cached = sharedEntries.get(hours)
  if (cached)
    return cached

  const entry: SharedPingTaskEntry = {
    data: shallowRef(null),
    promise: null,
    unsubscribeRealtime: null,
    subscribers: 0,
    lastFetchedAt: 0,
  }
  sharedEntries.set(hours, entry)
  return entry
}

function groupRecordsByClient(records: PingRecord[]): Map<string, PingRecord[]> {
  const grouped = new Map<string, PingRecord[]>()
  for (const record of records) {
    if (!record.client)
      continue
    const clientRecords = grouped.get(record.client) ?? []
    clientRecords.push(record)
    grouped.set(record.client, clientRecords)
  }
  for (const recordsByClient of grouped.values())
    recordsByClient.sort((left, right) => new Date(left.time).getTime() - new Date(right.time).getTime())
  return grouped
}

async function refreshEntry(entry: SharedPingTaskEntry, hours: number): Promise<void> {
  if (entry.promise)
    return entry.promise

  entry.promise = (async () => {
    try {
      const result = await loadPingRecordsWithTasks(hours)
      entry.data.value = {
        recordsByClient: groupRecordsByClient(result.records),
        tasks: result.tasks,
      }
      entry.lastFetchedAt = Date.now()
    }
    finally {
      entry.promise = null
    }
  })()
  return entry.promise
}

function appendPingRecord(entry: SharedPingTaskEntry, hours: number, event: { uuid: string, task_id?: number, time?: string, value?: number }): void {
  if (!entry.data.value || typeof event.task_id !== 'number' || typeof event.value !== 'number' || !event.time)
    return

  const record: PingRecord = {
    client: event.uuid,
    task_id: event.task_id,
    time: event.time,
    value: event.value,
  }
  const cutoff = Date.now() - hours * 60 * 60 * 1000
  const nextRecords = [
    ...(entry.data.value.recordsByClient.get(event.uuid) ?? []),
    record,
  ].filter((item) => {
    const timestamp = new Date(item.time).getTime()
    return !Number.isFinite(timestamp) || timestamp >= cutoff
  })
  nextRecords.sort((left, right) => new Date(left.time).getTime() - new Date(right.time).getTime())

  const recordsByClient = new Map(entry.data.value.recordsByClient)
  recordsByClient.set(event.uuid, nextRecords)
  entry.data.value = { ...entry.data.value, recordsByClient }
  entry.lastFetchedAt = Date.now()
}

function retainEntry(hours: number): () => void {
  const entry = getEntry(hours)
  const wasInactive = entry.subscribers === 0
  entry.subscribers += 1

  if (wasInactive) {
    entry.lastFetchedAt = 0
    entry.unsubscribeRealtime = subscribeRealtimeEvents((event) => {
      if (event.kind === 'ping')
        appendPingRecord(entry, hours, event)
    })
  }

  let released = false
  return () => {
    if (released)
      return
    released = true
    entry.subscribers = Math.max(0, entry.subscribers - 1)
    if (entry.subscribers === 0) {
      entry.unsubscribeRealtime?.()
      entry.unsubscribeRealtime = null
    }
  }
}

function average(values: number[]): number {
  return values.length ? values.reduce((sum, value) => sum + value, 0) / values.length : 0
}

function buildHistory(records: PingRecord[], maxBars = MAX_BAR_COUNT): PingHistoryPoint[] {
  const sortedRecords = records
    .map(record => ({ ...record, timestamp: new Date(record.time).getTime() }))
    .filter(record => Number.isFinite(record.timestamp))
    .sort((left, right) => left.timestamp - right.timestamp)
  if (!sortedRecords.length)
    return []

  const firstTime = sortedRecords[0]?.timestamp ?? 0
  const lastTime = sortedRecords.at(-1)?.timestamp ?? firstTime
  const bucketCount = Math.min(maxBars, sortedRecords.length)
  const bucketSize = Math.max(1, (lastTime - firstTime) / bucketCount)
  const history: PingHistoryPoint[] = []
  let recordIndex = 0

  for (let index = 0; index < bucketCount; index++) {
    const startTime = firstTime + bucketSize * index
    const endTime = index === bucketCount - 1 ? lastTime + 1 : startTime + bucketSize
    let total = 0
    let lost = 0
    let latencyTotal = 0
    let latencyCount = 0

    while (recordIndex < sortedRecords.length) {
      const record = sortedRecords[recordIndex]
      if (!record || record.timestamp >= endTime)
        break
      if (record.timestamp >= startTime) {
        total += 1
        if (record.value >= 0) {
          latencyTotal += record.value
          latencyCount += 1
        }
        else {
          lost += 1
        }
      }
      recordIndex += 1
    }

    history.push({
      time: new Date(startTime).toISOString(),
      latency: latencyCount ? latencyTotal / latencyCount : null,
      loss: total ? lost / total * 100 : null,
    })
  }
  return history
}

function latencyClass(value: number): string {
  if (value <= 60)
    return 'bg-emerald-600/90'
  if (value <= 100)
    return 'bg-green-400/80'
  if (value <= 160)
    return 'bg-lime-400/80'
  if (value <= 200)
    return 'bg-yellow-400/80'
  return 'bg-rose-500/80'
}

function lossClass(value: number): string {
  if (value <= 1)
    return 'bg-emerald-600/90'
  if (value <= 3)
    return 'bg-green-400/90'
  if (value <= 6)
    return 'bg-lime-400/90'
  if (value <= 9)
    return 'bg-yellow-400/90'
  return 'bg-rose-500/80'
}

function buildBars(records: PingRecord[], metric: 'latency' | 'loss', taskId: string, barCount: number): PingTaskBar[] {
  const bars = buildHistory(records, barCount).map((point, index) => {
    const value = point[metric]
    return {
      key: `${taskId}-${metric}-${point.time}-${index}`,
      className: value === null ? 'bg-muted-foreground/15' : metric === 'latency' ? latencyClass(value) : lossClass(value),
      tooltip: value === null
        ? `${formatDateTime(point.time, 'HH:mm:ss')} N/A`
        : metric === 'latency'
          ? `${formatDateTime(point.time, 'HH:mm:ss')}\n${Math.round(value)} ms`
          : `${formatDateTime(point.time, 'HH:mm:ss')}\n${value.toFixed(1)}%`,
    }
  })

  if (bars.length >= barCount)
    return bars.slice(-barCount)
  return [
    ...Array.from({ length: barCount - bars.length }, (_, index) => ({
      key: `${taskId}-${metric}-empty-${index}`,
      className: 'bg-muted-foreground/10',
      tooltip: '无采样数据',
    })),
    ...bars,
  ]
}

function taskBarCount(interval: number | undefined): number {
  const normalized = Number(interval)
  if (!Number.isFinite(normalized) || normalized <= 0)
    return MAX_BAR_COUNT
  return Math.max(1, Math.min(MAX_BAR_COUNT, Math.floor(3600 / normalized)))
}

function buildTaskRows(records: PingRecord[], tasks: PingTaskInfo[], taskOrder: number[]): PingTaskRow[] {
  const recordsByTask = new Map<string, PingRecord[]>()
  for (const record of records) {
    const taskId = String(record.task_id ?? '')
    if (!taskId)
      continue
    const taskRecords = recordsByTask.get(taskId) ?? []
    taskRecords.push(record)
    recordsByTask.set(taskId, taskRecords)
  }

  const taskMap = new Map(tasks.map(task => [String(task.id), task]))
  const orderedTaskIds = taskOrder.length
    ? taskOrder.map(String).filter(taskId => taskMap.has(taskId))
    : [...taskMap.keys()].filter(taskId => recordsByTask.has(taskId))

  return orderedTaskIds.map((taskId, index) => {
    const taskRecords = recordsByTask.get(taskId) ?? []
    const validLatencies = taskRecords.map(record => record.value).filter(value => value >= 0)
    const latency = average(validLatencies)
    const loss = taskRecords.length ? (taskRecords.length - validLatencies.length) / taskRecords.length * 100 : 0
    const task = taskMap.get(taskId)
    const barCount = taskBarCount(task?.interval)
    return {
      id: taskId,
      name: task?.name?.trim() || `任务 ${taskId}`,
      color: TASK_COLORS[index % TASK_COLORS.length] ?? '#94a3b8',
      latencyDisplay: validLatencies.length ? `${Math.round(latency)} ms` : '-',
      lossDisplay: taskRecords.length ? `${loss.toFixed(1)}%` : '-',
      latencyBars: buildBars(taskRecords, 'latency', taskId, barCount),
      lossBars: buildBars(taskRecords, 'loss', taskId, barCount),
    }
  })
}

export function useNodePingTaskDisplay(
  uuid: MaybeRefOrGetter<string>,
  options?: {
    hours?: MaybeRefOrGetter<number>
    enabled?: MaybeRefOrGetter<boolean>
    taskOrder?: MaybeRefOrGetter<number[]>
  },
) {
  const loading = ref(false)
  const resolved = computed(() => ({
    uuid: toValue(uuid),
    hours: Math.max(1, Math.floor(toValue(options?.hours) ?? 1)),
    enabled: toValue(options?.enabled) ?? true,
    taskOrder: Array.isArray(toValue(options?.taskOrder)) ? toValue(options?.taskOrder) : [],
  }))

  let activeHours: number | null = null
  let releaseEntry: (() => void) | null = null

  function syncSubscription(hours: number | null) {
    if (activeHours === hours)
      return
    releaseEntry?.()
    releaseEntry = null
    activeHours = null
    if (hours !== null) {
      releaseEntry = retainEntry(hours)
      activeHours = hours
    }
  }

  onScopeDispose(() => syncSubscription(null))

  const taskRows = computed(() => {
    const { uuid: nodeUuid, hours, enabled, taskOrder } = resolved.value
    if (!enabled || !nodeUuid.trim())
      return []
    const state = getEntry(hours).data.value
    return state ? buildTaskRows(state.recordsByClient.get(nodeUuid) ?? [], state.tasks, taskOrder ?? []) : []
  })

  watch(resolved, async ({ uuid: nodeUuid, hours, enabled }, _previous, onCleanup) => {
    let cancelled = false
    onCleanup(() => {
      cancelled = true
    })
    if (!enabled || !nodeUuid.trim()) {
      syncSubscription(null)
      loading.value = false
      return
    }

    syncSubscription(hours)
    const entry = getEntry(hours)
    if (entry.data.value && entry.lastFetchedAt > 0) {
      loading.value = false
      return
    }

    loading.value = !entry.data.value
    try {
      await refreshEntry(entry, hours)
    }
    catch {
    }
    finally {
      if (!cancelled)
        loading.value = false
    }
  }, { immediate: true })

  return { loading, taskRows }
}

export function useNodeCardPingTasks(
  uuid: MaybeRefOrGetter<string>,
  options?: { taskOrder?: MaybeRefOrGetter<number[]> },
) {
  const appStore = useAppStore()
  const enabled = computed(() => appStore.publicSettings?.record_enabled !== false && appStore.publicSettings?.ping_record_preserve_time !== 0)
  const hours = computed(() => {
    const preserveHours = appStore.publicSettings?.ping_record_preserve_time
    return typeof preserveHours === 'number' && preserveHours > 0 ? Math.min(preserveHours, 1) : 1
  })
  const state = useNodePingTaskDisplay(uuid, { hours, enabled, taskOrder: options?.taskOrder })
  const taskRows = computed<PingTaskRow[]>(() => {
    if (state.taskRows.value.length)
      return state.taskRows.value.slice(0, 3)
    const message = state.loading.value ? '加载中' : '暂无任务'
    return [{
      id: 'empty',
      name: message,
      color: '#94a3b8',
      latencyDisplay: '-',
      lossDisplay: '-',
      latencyBars: Array.from({ length: MAX_BAR_COUNT }, (_, index) => ({ key: `empty-latency-${index}`, className: 'bg-muted-foreground/10', tooltip: message })),
      lossBars: Array.from({ length: MAX_BAR_COUNT }, (_, index) => ({ key: `empty-loss-${index}`, className: 'bg-muted-foreground/10', tooltip: message })),
    }]
  })
  return { taskRows }
}
