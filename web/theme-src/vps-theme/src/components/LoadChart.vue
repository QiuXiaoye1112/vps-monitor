<script setup lang="ts">
import type { ComputedRef } from 'vue'
import type { ChartDashboardCardKey } from '@/stores/app'
import type { RecordFormat } from '@/utils/recordHelper'
import type { StatusRecord } from '@/utils/rpc'
import { Icon } from '@iconify/vue'
import dayjs from 'dayjs'
import { computed, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch, watchEffect } from 'vue'
import VChart from 'vue-echarts'
import MetricChartHeader from '@/components/MetricChartHeader.vue'
import MetricSeriesChartCard from '@/components/MetricSeriesChartCard.vue'
import { AppDialog } from '@/components/ui/app-dialog'
import { Button } from '@/components/ui/button'
import { CardX } from '@/components/ui/card-x'
import { Empty } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useNodeLoadStats } from '@/composables/useNodeLoadStats'
import { LOAD_RECORD_MAX_COUNT } from '@/constants/load'
import { normalizeStatusRecordsPayload } from '@/services/history.service'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'
import { getLoadChartPalette } from '@/utils/chartPalette'
import { formatBytes, formatBytesSplit } from '@/utils/helper'
import { subscribeRealtimeEvents } from '@/utils/realtime'
import { getSharedRpc } from '@/utils/rpc'
import '@/utils/echarts' // 共享 ECharts 配置

const props = defineProps<{
  uuid: string
}>()

const appStore = useAppStore()
const nodesStore = useNodesStore()

// 从 publicSettings 获取记录保留时间
const maxRecordPreserveTime = computed(() => Math.min(appStore.publicSettings?.record_preserve_time || 168, 168))

const detailLoadStatsHours = computed(() => maxRecordPreserveTime.value)

// 使用 store 中的 isDark computed
const isDark = computed(() => appStore.isDark)

const chartColors = reactive(getLoadChartPalette(appStore.colorVisionFriendly))

watchEffect(() => {
  Object.assign(chartColors, getLoadChartPalette(appStore.colorVisionFriendly))
})

const CUSTOM_VIEW_LABEL = '自定义'

interface MetricChartSeriesData {
  name: string
  color: string
  kind: 'bytes' | 'bytesPerSecond' | 'count' | 'milliseconds' | 'percent' | 'temperature'
  data: Array<[string, number | null]>
  dashed?: boolean
}

interface CustomRange {
  start: dayjs.Dayjs
  end: dayjs.Dayjs
  hours: number
}

// 图表主题相关颜色
const chartThemeColors = computed(() => ({
  text: isDark.value ? 'rgba(255, 255, 255, 0.85)' : 'rgba(0, 0, 0, 0.85)',
  textSecondary: isDark.value ? 'rgba(255, 255, 255, 0.55)' : 'rgba(0, 0, 0, 0.55)',
  textTertiary: isDark.value ? 'rgba(255, 255, 255, 0.35)' : 'rgba(0, 0, 0, 0.35)',
  borderColor: isDark.value ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)',
  splitLineColor: isDark.value ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)',
  tooltipBg: isDark.value ? 'rgba(40, 40, 40, 0.95)' : 'rgba(255, 255, 255, 0.8)',
  tooltipShadow: isDark.value ? 'rgba(0, 0, 0, 0.4)' : 'rgba(0, 0, 0, 0.06)',
  crosshairColor: isDark.value ? 'rgba(255, 255, 255, 0.15)' : 'rgba(0, 0, 0, 0.1)',
}))

// 通用 Tooltip 配置
const baseTooltipConfig = computed(() => ({
  trigger: 'axis' as const,
  confine: false,
  backgroundColor: chartThemeColors.value.tooltipBg,
  borderColor: 'transparent',
  borderWidth: 0,
  borderRadius: 6,
  textStyle: {
    color: chartThemeColors.value.text,
    fontSize: 12,
    lineHeight: 20,
  },
  extraCssText: `backdrop-filter: blur(5px);z-index:9;box-shadow:0 0 0 1px ${chartThemeColors.value.tooltipShadow}, 0 0 16px ${chartThemeColors.value.tooltipShadow}`,
  axisPointer: {
    type: 'cross' as const,
    crossStyle: {
      color: chartThemeColors.value.textTertiary,
    },
    lineStyle: {
      color: chartThemeColors.value.crosshairColor,
      width: 1,
      type: 'dashed' as const,
    },
    shadowStyle: {
      color: chartThemeColors.value.crosshairColor,
    },
  },
}))

// 图表边距配置
const chartMargin = { top: 30, right: 24, bottom: 32, left: 56 }
const chartMarginWithLegend = { top: 30, right: 24, bottom: 52, left: 56 }

// 视图选项
const presetViews = [
  { label: '1 小时', hours: 1 },
  { label: '6 小时', hours: 6 },
  { label: '12 小时', hours: 12 },
  { label: '1 天', hours: 24 },
  { label: '3 天', hours: 72 },
  { label: '5 天', hours: 120 },
  { label: '7 天', hours: 168 },
]

// 可用视图列表
const availableViews = computed(() => {
  const views: { label: string, hours?: number }[] = []
  const maxHours = maxRecordPreserveTime.value

  for (const v of presetViews) {
    if (maxHours >= v.hours) {
      views.push({ label: v.label, hours: v.hours })
    }
  }

  const maxPreset = presetViews.at(-1)
  if (maxPreset && maxHours > maxPreset.hours) {
    const label = maxHours % 24 === 0
      ? `${Math.floor(maxHours / 24)} 天`
      : `${maxHours} 小时`
    views.push({ label, hours: maxHours })
  }
  else if (maxHours > 1 && !presetViews.some(v => v.hours === maxHours)) {
    const label = maxHours % 24 === 0
      ? `${Math.floor(maxHours / 24)} 天`
      : `${maxHours} 小时`
    views.push({ label, hours: maxHours })
  }

  views.push({ label: CUSTOM_VIEW_LABEL })
  return views
})

// 当前选中的视图
const selectedView = ref<string>('1 小时')
const customStartInput = ref('')
const customEndInput = ref('')
const selectedHours = computed(() => {
  const view = availableViews.value.find(v => v.label === selectedView.value)
  return view?.hours
})
const isRealtime = computed(() => false)
const isCustomRange = computed(() => selectedView.value === CUSTOM_VIEW_LABEL)
const customRange = computed<CustomRange | null>(() => {
  if (!customStartInput.value || !customEndInput.value)
    return null

  const start = dayjs(customStartInput.value)
  const end = dayjs(customEndInput.value)
  if (!start.isValid() || !end.isValid() || !end.isAfter(start))
    return null
  const now = dayjs()
  if (end.isAfter(now.add(1, 'minute')) || start.isBefore(now.subtract(maxRecordPreserveTime.value, 'hour')))
    return null
  if (end.diff(start, 'hour', true) > maxRecordPreserveTime.value)
    return null

  return {
    start,
    end,
    hours: Math.max(1, Math.ceil(end.diff(start, 'hour', true))),
  }
})
const customRangeError = computed(() => {
  if (!isCustomRange.value || (!customStartInput.value && !customEndInput.value))
    return ''
  if (!customStartInput.value || !customEndInput.value)
    return '请选择开始和结束时间'
  return customRange.value ? '' : '只能查看最近 7 天内的数据，且结束时间必须晚于开始时间'
})
const effectiveHistoryHours = computed(() => isCustomRange.value ? customRange.value?.hours ?? 1 : selectedHours.value ?? 1)

// 数据状态：小卡片固定使用最近 1 小时，详情弹窗使用独立的时间范围。
const cardRemoteData = shallowRef<StatusRecord[]>([])
const remoteData = shallowRef<StatusRecord[]>([])
const cardLoading = ref(false)
const cardError = ref<string | null>(null)
const loading = ref(false)
const isInitialLoad = ref(true) // 是否为首次加载（用于控制实时模式下的 NSpin 显示）
const error = ref<string | null>(null)
let fetchInFlight = false
let pendingVisibleFetch = false
let cardFetchInFlight = false
let pendingCardFetch = false

// 节点信息
const nodeInfo = computed(() => nodesStore.nodesByUuid.get(props.uuid))
const { diskPrediction, diskPredictionState } = useNodeLoadStats(
  () => props.uuid,
  {
    hours: () => detailLoadStatsHours.value,
    enabled: () => appStore.diskPredictionEnabled && appStore.privateFeaturesAllowed,
    diskTotal: () => nodeInfo.value?.disk_total ?? 0,
    online: () => nodeInfo.value?.online ?? false,
    permission: 'diskPrediction',
  },
)
const diskPredictionSummary = computed(() => {
  if (!appStore.diskPredictionEnabled || !appStore.privateFeaturesAllowed)
    return ''

  const prediction = diskPrediction.value
  if (prediction) {
    const days = Math.max(0, Math.ceil(prediction.daysUntilFull))
    const growth = formatBytesSplit(prediction.dailyGrowthBytes, appStore.byteDecimals)
    return days <= 0
      ? `按最近 ${prediction.sampleDays.toFixed(1)} 天趋势，磁盘预计已满`
      : `按最近 ${prediction.sampleDays.toFixed(1)} 天趋势，预计 ${days} 天后满 · 日增 ${growth.value} ${growth.unit}`
  }

  const state = diskPredictionState.value
  if (state.reason === 'no_samples')
    return '磁盘预测数据积累中：暂无可用历史样本'
  if (state.reason === 'insufficient_samples')
    return '磁盘预测数据积累中：样本数量不足'
  if (state.reason === 'insufficient_duration')
    return `磁盘预测数据积累中：历史跨度 ${state.sampleDays.toFixed(1)} 天，至少需要约 2 天`
  return ''
})

// RPC 客户端
const rpc = getSharedRpc()

// ==================== 数据获取 ====================

function statusToRecordFormat(records: StatusRecord[]): RecordFormat[] {
  return records.map(r => ({
    client: r.client,
    time: r.time,
    cpu: r.cpu ?? null,
    ram: r.ram ?? null,
    ram_total: r.ram_total ?? null,
    swap: r.swap ?? null,
    swap_total: r.swap_total ?? null,
    load: r.load ?? null,
    temp: r.temp ?? null,
    disk: r.disk ?? null,
    disk_total: r.disk_total ?? null,
    net_in: r.net_in ?? null,
    net_out: r.net_out ?? null,
    net_total_up: r.net_total_up ?? null,
    net_total_down: r.net_total_down ?? null,
    traffic_up: r.traffic_up ?? null,
    traffic_down: r.traffic_down ?? null,
    process: r.process ?? null,
    connections: r.connections ?? null,
    connections_udp: r.connections_udp ?? null,
  }))
}

function metricValue(value: number | null | undefined): number | null {
  return typeof value === 'number' && Number.isFinite(value) ? value : null
}

async function fetchRecentData() {
  if (!props.uuid)
    return

  // 只在首次加载时显示 loading
  if (isInitialLoad.value) {
    loading.value = true
  }
  error.value = null

  try {
    const result = await rpc.getNodeRecentStatus(props.uuid)
    const records = result?.records || []
    records.sort((a, b) => dayjs(a.time).valueOf() - dayjs(b.time).valueOf())
    const maxLength = 150
    remoteData.value = records.slice(-maxLength)
  }
  catch (err) {
    error.value = err instanceof Error ? err.message : '获取数据失败'
    remoteData.value = []
  }
  finally {
    loading.value = false
    isInitialLoad.value = false
  }
}

async function fetchHistoryData(silent = false) {
  if (!props.uuid)
    return

  if (isCustomRange.value && !customRange.value) {
    remoteData.value = []
    error.value = customRangeError.value || '请选择有效的自定义时间范围'
    return
  }

  const range = customRange.value
  const hours = effectiveHistoryHours.value

  if (!silent)
    loading.value = true
  error.value = null

  try {
    const loadParams = isCustomRange.value && range
      ? {
          uuid: props.uuid,
          start: range.start.toDate().toISOString(),
          end: range.end.toDate().toISOString(),
          maxCount: LOAD_RECORD_MAX_COUNT,
        }
      : { uuid: props.uuid, hours, maxCount: LOAD_RECORD_MAX_COUNT }
    const historyResult = await rpc.getLoadRecordsRange(loadParams)
    remoteData.value = normalizeStatusRecordsPayload(historyResult.records)
  }
  catch (err) {
    error.value = err instanceof Error ? err.message : '获取数据失败'
    remoteData.value = []
  }
  finally {
    if (!silent)
      loading.value = false
  }
}

async function fetchCardData(silent = false) {
  if (!props.uuid)
    return
  if (cardFetchInFlight) {
    pendingCardFetch = true
    return
  }

  cardFetchInFlight = true
  if (!silent)
    cardLoading.value = true
  cardError.value = null

  try {
    const historyResult = await rpc.getLoadRecordsRange({
      uuid: props.uuid,
      hours: 1,
      maxCount: LOAD_RECORD_MAX_COUNT,
    })
    cardRemoteData.value = normalizeStatusRecordsPayload(historyResult.records)
  }
  catch (err) {
    cardError.value = err instanceof Error ? err.message : '获取数据失败'
    if (cardRemoteData.value.length === 0)
      cardRemoteData.value = []
  }
  finally {
    cardFetchInFlight = false
    if (!silent)
      cardLoading.value = false
    if (pendingCardFetch) {
      pendingCardFetch = false
      void fetchCardData(true)
    }
  }
}

async function fetchData(silent = false) {
  if (fetchInFlight) {
    if (!silent)
      pendingVisibleFetch = true
    return
  }
  fetchInFlight = true
  try {
    if (isRealtime.value) {
      await fetchRecentData()
    }
    else {
      await fetchHistoryData(silent)
    }
  }
  finally {
    fetchInFlight = false
    if (pendingVisibleFetch) {
      pendingVisibleFetch = false
      void fetchData()
    }
  }
}

// ==================== 数据处理 ====================

const cardChartData = computed(() => {
  return statusToRecordFormat(cardRemoteData.value)
})

const chartData = computed(() => {
  return statusToRecordFormat(remoteData.value)
})

const cardHistoryHours = computed(() => 1)

function createChartOptions(
  chartData: ComputedRef<RecordFormat[]>,
  effectiveHistoryHours: ComputedRef<number>,
) {
  function seriesHasData(series: MetricChartSeriesData): boolean {
    return series.data.some(([, value]) => value !== null && Number.isFinite(value))
  }

  function recordMetricSeries(
    name: string,
    color: string,
    kind: MetricChartSeriesData['kind'],
    getter: (record: RecordFormat) => number | null | undefined,
    dashed = false,
  ): MetricChartSeriesData {
    return {
      name,
      color,
      kind,
      dashed,
      data: chartData.value.map(record => [record.time, metricValue(getter(record))]),
    }
  }

  const trafficChartSeries = computed<MetricChartSeriesData[]>(() => [
    recordMetricSeries('累计下载', chartColors.quinary, 'bytes', record => record.net_total_down),
    recordMetricSeries('累计上传', chartColors.quaternary, 'bytes', record => record.net_total_up),
    recordMetricSeries('周期下载', chartColors.tertiary, 'bytes', record => record.traffic_down, true),
    recordMetricSeries('周期上传', chartColors.secondary, 'bytes', record => record.traffic_up, true),
  ].filter(seriesHasData))

  const temperatureChartSeries = computed<MetricChartSeriesData[]>(() => [
    recordMetricSeries('系统温度', chartColors.secondary, 'temperature', record => record.temp),
  ].filter(seriesHasData))

  const hasTrafficData = computed(() => trafficChartSeries.value.length > 0)
  const hasTemperatureData = computed(() => temperatureChartSeries.value.length > 0)

  // ==================== 工具函数 ====================

  function formatTime(time: string, showDate: boolean): string {
    const date = dayjs(time)
    if (showDate) {
      return date.format('M/D HH:mm')
    }
    return date.format('HH:mm')
  }

  function formatTimeForTooltip(time: string, hours: number): string {
    const date = dayjs(time)
    if (hours < 24) {
      return date.format('HH:mm:ss')
    }
    return date.format('MM/DD HH:mm')
  }

  const showDateInAxis = computed(() => (effectiveHistoryHours.value) >= 24)

  // 通用 X 轴配置
  const baseXAxisConfig = computed(() => ({
    type: 'category' as const,
    data: chartData.value.map(r => formatTime(r.time, showDateInAxis.value)),
    axisLabel: {
      fontSize: 11,
      color: chartThemeColors.value.textSecondary,
      margin: 12,
    },
    axisLine: {
      show: true,
      lineStyle: { color: chartThemeColors.value.borderColor, width: 1 },
    },
    axisTick: { show: false },
    boundaryGap: false,
  }))

  // 通用 Y 轴配置
  const baseYAxisConfig = computed(() => ({
    type: 'value' as const,
    axisLabel: {
      fontSize: 11,
      color: chartThemeColors.value.textSecondary,
    },
    axisLine: { show: false },
    axisTick: { show: false },
    splitLine: {
      lineStyle: {
        color: chartThemeColors.value.splitLineColor,
        type: 'dashed' as const,
      },
    },
  }))

  // ==================== 图表配置 ====================

  // CPU 图表
  const cpuChartOption = computed(() => ({
    animation: false,
    // 全局颜色配置（确保 Tooltip 圆点颜色与线条一致）
    color: [chartColors.primary, chartColors.secondary],
    tooltip: {
      ...baseTooltipConfig.value,
      formatter: (params: unknown) => {
        const p = params as Array<{ dataIndex: number, seriesName: string, value: number, color: string }>
        if (!p.length)
          return ''
        const firstParam = p[0]
        if (!firstParam)
          return ''
        const record = chartData.value[firstParam.dataIndex]
        if (!record)
          return ''

        const timeStr = formatTimeForTooltip(record.time, effectiveHistoryHours.value)
        let html = `<div style="font-weight:600;margin-bottom:6px;color:${chartThemeColors.value.textSecondary}">${timeStr}</div>`
        html += '<div style="display:flex;flex-direction:column;gap:4px">'

        for (const item of p) {
          const colorDot = `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${item.color};margin-right:8px;flex-shrink:0"></span>`
          if (item.seriesName === 'CPU') {
            html += `<div style="display:flex;align-items:center">${colorDot}<span>CPU</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${item.value?.toFixed(1) ?? '-'}%</span></div>`
          }
          else if (item.seriesName === '负载') {
            html += `<div style="display:flex;align-items:center">${colorDot}<span>系统负载</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${item.value?.toFixed(2) ?? '-'}</span></div>`
          }
        }
        html += '</div>'
        return html
      },
    },
    grid: chartMargin,
    xAxis: baseXAxisConfig.value,
    yAxis: [
      {
        ...baseYAxisConfig.value,
        name: 'CPU %',
        nameTextStyle: { color: chartThemeColors.value.textSecondary, padding: [0, 40, 0, 0] },
        min: 0,
        max: 100,
        axisLabel: { ...baseYAxisConfig.value.axisLabel, formatter: '{value}%' },
      },
      {
        ...baseYAxisConfig.value,
        name: '负载',
        nameTextStyle: { color: chartThemeColors.value.textSecondary, padding: [0, 0, 0, 40] },
        min: 0,
        splitLine: { show: false },
      },
    ],
    series: [
      {
        name: 'CPU',
        type: 'line',
        data: chartData.value.map(r => r.cpu),

        showSymbol: false,
        yAxisIndex: 0,
        lineStyle: { width: 1.5, color: chartColors.primary, cap: 'round' as const },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: chartColors.primaryAreaStrong },
              { offset: 1, color: chartColors.primaryAreaFaint },
            ],
          },
        },
      },
      {
        name: '负载',
        type: 'line',
        data: chartData.value.map(r => r.load),

        showSymbol: false,
        yAxisIndex: 1,
        lineStyle: { width: 1.5, color: chartColors.secondary, cap: 'round' as const },
      },
    ],
  }))

  // 内存图表
  const memoryChartOption = computed(() => ({
    animation: false,
    color: [chartColors.primary, chartColors.quinary, chartColors.secondary, chartColors.quaternary],
    tooltip: {
      ...baseTooltipConfig.value,
      formatter: (params: unknown) => {
        const p = params as Array<{ dataIndex: number, seriesName: string, value: number, color: string }>
        if (!p.length)
          return ''
        const firstParam = p[0]
        if (!firstParam)
          return ''
        const record = chartData.value[firstParam.dataIndex]
        if (!record)
          return ''

        const ramUsed = record.ram ?? 0
        const ramTotal = record.ram_total ?? nodeInfo.value?.mem_total ?? 0
        const swapUsed = record.swap ?? 0
        const swapTotal = record.swap_total ?? nodeInfo.value?.swap_total ?? 0
        const ramPercent = ramTotal > 0 ? ((ramUsed / ramTotal) * 100).toFixed(1) : '0'
        const swapPercent = swapTotal > 0 ? ((swapUsed / swapTotal) * 100).toFixed(1) : '0'

        const timeStr = formatTimeForTooltip(record.time, effectiveHistoryHours.value)
        let html = `<div style="font-weight:600;margin-bottom:6px;color:${chartThemeColors.value.textSecondary}">${timeStr}</div>`
        html += '<div style="display:flex;flex-direction:column;gap:4px">'

        for (const item of p) {
          const colorDot = `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${item.color};margin-right:8px;flex-shrink:0"></span>`
          if (item.seriesName === 'RAM') {
            html += `<div style="display:flex;align-items:center">${colorDot}<span>RAM</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${formatBytes(ramUsed)} (${ramPercent}%)</span></div>`
          }
          else if (item.seriesName === 'Swap') {
            html += `<div style="display:flex;align-items:center">${colorDot}<span>Swap</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${formatBytes(swapUsed)} (${swapPercent}%)</span></div>`
          }
          else if (item.seriesName === 'RAM 总量') {
            html += `<div style="display:flex;align-items:center">${colorDot}<span>RAM 总量</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${formatBytes(ramTotal)}</span></div>`
          }
          else if (item.seriesName === 'Swap 总量') {
            html += `<div style="display:flex;align-items:center">${colorDot}<span>Swap 总量</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${formatBytes(swapTotal)}</span></div>`
          }
        }
        html += '</div>'
        return html
      },
    },
    legend: {
      data: ['RAM', 'RAM 总量', 'Swap', 'Swap 总量'],
      bottom: 4,
      itemWidth: 10,
      itemHeight: 8,
      textStyle: { fontSize: 10, color: chartThemeColors.value.textSecondary },
    },
    grid: chartMarginWithLegend,
    xAxis: baseXAxisConfig.value,
    yAxis: {
      ...baseYAxisConfig.value,
      name: '内存',
      nameTextStyle: { color: chartThemeColors.value.textSecondary, padding: [0, 40, 0, 0] },
      axisLabel: {
        ...baseYAxisConfig.value.axisLabel,
        formatter: (val: number) => formatBytes(val),
      },
    },
    series: [
      {
        name: 'RAM',
        type: 'line',
        data: chartData.value.map(r => r.ram),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.primary, cap: 'round' as const },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: chartColors.primaryAreaStrong },
              { offset: 1, color: chartColors.primaryAreaFaint },
            ],
          },
        },
      },
      {
        name: 'RAM 总量',
        type: 'line',
        data: chartData.value.map(r => r.ram_total ?? nodeInfo.value?.mem_total ?? null),
        showSymbol: false,
        lineStyle: { width: 1.2, type: 'dashed' as const, color: chartColors.quinary, cap: 'round' as const },
      },
      {
        name: 'Swap',
        type: 'line',
        data: chartData.value.map(r => r.swap),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.secondary, cap: 'round' as const },
      },
      {
        name: 'Swap 总量',
        type: 'line',
        data: chartData.value.map(r => r.swap_total ?? nodeInfo.value?.swap_total ?? null),
        showSymbol: false,
        lineStyle: { width: 1.2, type: 'dashed' as const, color: chartColors.quaternary, cap: 'round' as const },
      },
    ],
  }))

  // 磁盘图表
  const diskChartOption = computed(() => ({
    animation: false,
    color: [chartColors.tertiary, chartColors.quinary],
    tooltip: {
      ...baseTooltipConfig.value,
      formatter: (params: unknown) => {
        const p = params as Array<{ dataIndex: number, seriesName: string, value: number, color: string }>
        if (!p.length)
          return ''
        const firstParam = p[0]
        if (!firstParam)
          return ''
        const record = chartData.value[firstParam.dataIndex]
        if (!record)
          return ''

        const diskUsed = record.disk ?? 0
        const diskTotal = record.disk_total ?? nodeInfo.value?.disk_total ?? 0
        const diskPercent = diskTotal > 0 ? ((diskUsed / diskTotal) * 100).toFixed(1) : '0'

        const timeStr = formatTimeForTooltip(record.time, effectiveHistoryHours.value)
        let html = `<div style="font-weight:600;margin-bottom:6px;color:${chartThemeColors.value.textSecondary}">${timeStr}</div>`
        html += '<div style="display:flex;flex-direction:column;gap:4px">'
        for (const item of p) {
          const colorDot = `<span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:${item.color};margin-right:8px;flex-shrink:0"></span>`
          const text = item.seriesName === '磁盘总量' ? formatBytes(diskTotal) : `${formatBytes(diskUsed)} (${diskPercent}%)`
          html += `<div style="display:flex;align-items:center">${colorDot}<span>${item.seriesName}</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${text}</span></div>`
        }
        html += '</div>'
        return html
      },
    },
    legend: {
      data: ['磁盘已用', '磁盘总量'],
      bottom: 4,
      itemWidth: 10,
      itemHeight: 8,
      textStyle: { fontSize: 10, color: chartThemeColors.value.textSecondary },
    },
    grid: chartMarginWithLegend,
    xAxis: baseXAxisConfig.value,
    yAxis: {
      ...baseYAxisConfig.value,
      name: '磁盘',
      nameTextStyle: { color: chartThemeColors.value.textSecondary, padding: [0, 40, 0, 0] },
      axisLabel: {
        ...baseYAxisConfig.value.axisLabel,
        formatter: (val: number) => formatBytes(val),
      },
    },
    series: [
      {
        name: '磁盘已用',
        type: 'line',
        data: chartData.value.map(r => r.disk),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.tertiary, cap: 'round' as const },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: chartColors.tertiaryAreaStrong },
              { offset: 1, color: chartColors.tertiaryAreaFaint },
            ],
          },
        },
      },
      {
        name: '磁盘总量',
        type: 'line',
        data: chartData.value.map(r => r.disk_total ?? nodeInfo.value?.disk_total ?? null),
        showSymbol: false,
        lineStyle: { width: 1.2, type: 'dashed' as const, color: chartColors.quinary, cap: 'round' as const },
      },
    ],
  }))

  // 网络图表
  const networkChartOption = computed(() => ({
    animation: false,
    color: [chartColors.quinary, chartColors.quaternary],
    tooltip: {
      ...baseTooltipConfig.value,
      formatter: (params: unknown) => {
        const p = params as Array<{ dataIndex: number, seriesName: string, value: number, color: string }>
        if (!p.length)
          return ''
        const firstParam = p[0]
        if (!firstParam)
          return ''
        const record = chartData.value[firstParam.dataIndex]
        if (!record)
          return ''

        const timeStr = formatTimeForTooltip(record.time, effectiveHistoryHours.value)
        let html = `<div style="font-weight:600;margin-bottom:6px;color:${chartThemeColors.value.textSecondary}">${timeStr}</div>`
        html += '<div style="display:flex;flex-direction:column;gap:4px">'

        for (const item of p) {
          const colorDot = `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${item.color};margin-right:8px;flex-shrink:0"></span>`
          const label = item.seriesName === '下载' ? '↓ 下载' : '↑ 上传'
          html += `<div style="display:flex;align-items:center">${colorDot}<span>${label}</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${formatBytes(item.value)}/s</span></div>`
        }
        html += '</div>'
        return html
      },
    },
    legend: {
      data: ['下载', '上传'],
      bottom: 4,
      itemWidth: 12,
      itemHeight: 12,
      itemGap: 20,
      icon: 'roundRect',
      textStyle: { fontSize: 11, color: chartThemeColors.value.textSecondary },
    },
    grid: chartMarginWithLegend,
    xAxis: baseXAxisConfig.value,
    yAxis: {
      ...baseYAxisConfig.value,
      name: '速度',
      nameTextStyle: { color: chartThemeColors.value.textSecondary, padding: [0, 40, 0, 0] },
      axisLabel: {
        ...baseYAxisConfig.value.axisLabel,
        formatter: (val: number) => formatBytes(val),
      },
    },
    series: [
      {
        name: '下载',
        type: 'line',
        data: chartData.value.map(r => r.net_in ?? 0),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.quinary, cap: 'round' as const },
      },
      {
        name: '上传',
        type: 'line',
        data: chartData.value.map(r => r.net_out ?? 0),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.quaternary, cap: 'round' as const },
      },
    ],
  }))

  // 连接数图表
  const connectionsChartOption = computed(() => ({
    animation: false,
    color: [chartColors.primary, chartColors.tertiary],
    tooltip: {
      ...baseTooltipConfig.value,
      formatter: (params: unknown) => {
        const p = params as Array<{ dataIndex: number, seriesName: string, value: number, color: string }>
        if (!p.length)
          return ''
        const firstParam = p[0]
        if (!firstParam)
          return ''
        const record = chartData.value[firstParam.dataIndex]
        if (!record)
          return ''

        const timeStr = formatTimeForTooltip(record.time, effectiveHistoryHours.value)
        let html = `<div style="font-weight:600;margin-bottom:6px;color:${chartThemeColors.value.textSecondary}">${timeStr}</div>`
        html += '<div style="display:flex;flex-direction:column;gap:4px">'

        for (const item of p) {
          const colorDot = `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${item.color};margin-right:8px;flex-shrink:0"></span>`
          const displayValue = item.value != null ? Math.round(item.value) : '-'
          html += `<div style="display:flex;align-items:center">${colorDot}<span>${item.seriesName}</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${displayValue}</span></div>`
        }
        html += '</div>'
        return html
      },
    },
    legend: {
      data: ['TCP', 'UDP'],
      bottom: 4,
      itemWidth: 12,
      itemHeight: 12,
      itemGap: 20,
      icon: 'roundRect',
      textStyle: { fontSize: 11, color: chartThemeColors.value.textSecondary },
    },
    grid: chartMarginWithLegend,
    xAxis: baseXAxisConfig.value,
    yAxis: {
      ...baseYAxisConfig.value,
      name: '连接数',
      nameTextStyle: { color: chartThemeColors.value.textSecondary, padding: [0, 40, 0, 0] },
      min: 0,
      axisLabel: {
        ...baseYAxisConfig.value.axisLabel,
        formatter: (val: number) => Math.round(val).toString(),
      },
    },
    series: [
      {
        name: 'TCP',
        type: 'line',
        data: chartData.value.map(r => r.connections ?? 0),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.primary, cap: 'round' as const },
      },
      {
        name: 'UDP',
        type: 'line',
        data: chartData.value.map(r => r.connections_udp ?? 0),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.tertiary, cap: 'round' as const },
      },
    ],
  }))

  // 进程数图表
  const processChartOption = computed(() => ({
    animation: false,
    color: [chartColors.quaternary],
    tooltip: {
      ...baseTooltipConfig.value,
      formatter: (params: unknown) => {
        const p = params as Array<{ dataIndex: number, value: number, color: string }>
        if (!p.length)
          return ''
        const firstParam = p[0]
        if (!firstParam)
          return ''
        const record = chartData.value[firstParam.dataIndex]
        if (!record)
          return ''

        const timeStr = formatTimeForTooltip(record.time, effectiveHistoryHours.value)
        const colorDot = `<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:${firstParam.color};margin-right:8px;flex-shrink:0"></span>`
        const displayValue = firstParam.value != null ? Math.round(firstParam.value) : '-'

        let html = `<div style="font-weight:600;margin-bottom:6px;color:${chartThemeColors.value.textSecondary}">${timeStr}</div>`
        html += '<div style="display:flex;flex-direction:column;gap:4px">'
        html += `<div style="display:flex;align-items:center">${colorDot}<span>进程数</span><span style="margin-left:auto;font-weight:600;margin-left:16px">${displayValue}</span></div>`
        html += '</div>'
        return html
      },
    },
    grid: chartMargin,
    xAxis: baseXAxisConfig.value,
    yAxis: {
      ...baseYAxisConfig.value,
      name: '进程',
      nameTextStyle: { color: chartThemeColors.value.textSecondary, padding: [0, 40, 0, 0] },
      min: 0,
      axisLabel: {
        ...baseYAxisConfig.value.axisLabel,
        formatter: (val: number) => Math.round(val).toString(),
      },
    },
    series: [
      {
        name: '进程数',
        type: 'line',
        data: chartData.value.map(r => r.process ?? 0),

        showSymbol: false,
        lineStyle: { width: 1.5, color: chartColors.quaternary, cap: 'round' as const },
        areaStyle: {
          color: {
            type: 'linear',
            x: 0,
            y: 0,
            x2: 0,
            y2: 1,
            colorStops: [
              { offset: 0, color: 'rgba(167, 139, 250, 0.25)' },
              { offset: 1, color: 'rgba(167, 139, 250, 0.02)' },
            ],
          },
        },
      },
    ],
  }))

  return {
    chartData,
    trafficChartSeries,
    temperatureChartSeries,
    hasTrafficData,
    hasTemperatureData,
    cpuChartOption,
    memoryChartOption,
    diskChartOption,
    networkChartOption,
    connectionsChartOption,
    processChartOption,
  }
}

const cardCharts = reactive(createChartOptions(cardChartData, cardHistoryHours))
const detailCharts = createChartOptions(chartData, effectiveHistoryHours)

const chartDashboardCards = computed(() => appStore.chartDashboardTemplate.cards)

function isChartCardEnabled(key: ChartDashboardCardKey): boolean {
  if (!chartDashboardCards.value.includes(key))
    return false

  switch (key) {
    case 'traffic':
      return cardCharts.hasTrafficData
    case 'temperature':
      return cardCharts.hasTemperatureData
    default:
      return true
  }
}

function getChartCardOrder(key: ChartDashboardCardKey): number {
  const index = chartDashboardCards.value.indexOf(key)
  return index < 0 ? 99 : index
}

function getChartCardStyle(key: ChartDashboardCardKey): Record<string, string> {
  return { order: String(getChartCardOrder(key)) }
}

type ExpandableChartKey = 'cpu' | 'memory' | 'disk' | 'network' | 'connections' | 'process'

const expandedChart = ref<ExpandableChartKey | null>(null)
const expandedChartMeta: Record<ExpandableChartKey, { title: string, description: string }> = {
  cpu: { title: 'CPU 与负载', description: 'CPU 使用率与系统负载历史' },
  memory: { title: '内存与 Swap', description: '内存与交换空间使用历史' },
  disk: { title: '磁盘', description: '磁盘使用历史' },
  network: { title: '实时网络', description: '网络上传与下载速率历史' },
  connections: { title: '网络连接', description: 'TCP 与 UDP 连接数历史' },
  process: { title: '进程', description: '系统进程数量历史' },
}
const expandedChartOption = computed(() => {
  switch (expandedChart.value) {
    case 'cpu': return detailCharts.cpuChartOption.value
    case 'memory': return detailCharts.memoryChartOption.value
    case 'disk': return detailCharts.diskChartOption.value
    case 'network': return detailCharts.networkChartOption.value
    case 'connections': return detailCharts.connectionsChartOption.value
    case 'process': return detailCharts.processChartOption.value
    default: return {}
  }
})
const expandedChartTitle = computed(() => expandedChart.value ? expandedChartMeta[expandedChart.value].title : '')
const expandedChartDescription = computed(() => expandedChart.value ? expandedChartMeta[expandedChart.value].description : '')

function openExpandedChart(key: ExpandableChartKey) {
  selectedView.value = '1 小时'
  expandedChart.value = key
}

function setExpandedOpen(open: boolean) {
  if (open)
    return
  expandedChart.value = null
  if (selectedView.value !== '1 小时')
    selectedView.value = '1 小时'
}

function ensureDefaultCustomRange() {
  if (customStartInput.value && customEndInput.value)
    return
  const end = dayjs()
  customStartInput.value = end.subtract(24, 'hour').format('YYYY-MM-DDTHH:mm')
  customEndInput.value = end.format('YYYY-MM-DDTHH:mm')
}

// ==================== 实时更新 ====================

// 历史图表只在服务端收到新的历史采样后更新，不按固定间隔重复查询。
const unsubscribeRealtime = subscribeRealtimeEvents((event) => {
  if (event.kind === 'history' && event.uuid === props.uuid) {
    void fetchData(true)
    void fetchCardData(true)
  }
})

// 生命周期 ====================

watch(selectedView, () => {
  if (isCustomRange.value)
    ensureDefaultCustomRange()
  isInitialLoad.value = true // 切换视图时重置首次加载状态
  if (!isCustomRange.value || customRange.value)
    fetchData()
})

watch(() => props.uuid, () => {
  cardRemoteData.value = []
  remoteData.value = []
  cardError.value = null
  isInitialLoad.value = true // 切换节点时重置首次加载状态
  fetchData()
  fetchCardData()
})

onMounted(() => {
  fetchData()
  fetchCardData()
})

onBeforeUnmount(() => {
  unsubscribeRealtime()
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <!-- 内容区域 -->
    <div :aria-busy="loading">
      <div v-if="cardError" class="text-red-500 py-8 text-center">
        {{ cardError }}
      </div>
      <div v-else-if="cardCharts.chartData.length === 0 && !cardLoading" class="py-8">
        <Empty description="暂无负载数据" />
      </div>

      <!-- 图表网格 -->
      <div v-else class="gap-4 grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3">
        <!-- CPU 卡片 -->
        <CardX v-if="isChartCardEnabled('cpu')" size="small" class="cursor-pointer bg-background/50 border-none hover:bg-background transition-all rounded-md" :style="getChartCardStyle('cpu')" role="button" tabindex="0" @click="openExpandedChart('cpu')" @keydown.enter="openExpandedChart('cpu')">
          <template #header>
            <MetricChartHeader title="CPU 与负载" icon="tabler:cpu" tone="rose">
              <div v-if="nodeInfo?.cpu != null" class="text-xs flex gap-0.5 items-baseline">
                <span>{{ nodeInfo.cpu.toFixed(1) }}</span>
                <span>%</span>
              </div>
              <span v-else>-</span>
            </MetricChartHeader>
          </template>
          <div class="h-48">
            <VChart :option="cardCharts.cpuChartOption" autoresize />
          </div>
        </CardX>

        <!-- 内存卡片 -->
        <CardX v-if="isChartCardEnabled('memory')" size="small" class="cursor-pointer bg-background/50 border-none hover:bg-background transition-all rounded-md" :style="getChartCardStyle('memory')" role="button" tabindex="0" @click="openExpandedChart('memory')" @keydown.enter="openExpandedChart('memory')">
          <template #header>
            <MetricChartHeader title="内存与 Swap" icon="tabler:database" tone="violet">
              <div class="text-xs flex gap-1 items-baseline">
                <template v-if="nodeInfo?.ram != null">
                  <span>{{ formatBytesSplit(nodeInfo.ram).value }}</span>
                  <span>{{ formatBytesSplit(nodeInfo.ram).unit }}</span>
                </template>
                <span v-else>-</span>
                <span>·</span>
                <template v-if="nodeInfo?.mem_total">
                  <span>{{
                    formatBytesSplit(nodeInfo.mem_total).value }}</span>
                  <span>{{ formatBytesSplit(nodeInfo.mem_total).unit }}</span>
                </template>
                <span v-else>-</span>
              </div>
            </MetricChartHeader>
          </template>
          <div class="h-48">
            <VChart :option="cardCharts.memoryChartOption" autoresize />
          </div>
        </CardX>

        <!-- 磁盘卡片 -->
        <CardX v-if="isChartCardEnabled('disk')" size="small" class="cursor-pointer bg-background/50 border-none hover:bg-background transition-all rounded-md" :style="getChartCardStyle('disk')" role="button" tabindex="0" @click="openExpandedChart('disk')" @keydown.enter="openExpandedChart('disk')">
          <template #header>
            <MetricChartHeader title="磁盘" icon="tabler:device-floppy" tone="emerald" :subtitle="diskPredictionSummary">
              <div class="text-xs flex gap-1 items-baseline shrink-0">
                <template v-if="nodeInfo?.disk != null">
                  <span>{{ formatBytesSplit(nodeInfo.disk).value }}</span>
                  <span>{{ formatBytesSplit(nodeInfo.disk).unit }}</span>
                </template>
                <span v-else>-</span>
                <span>·</span>
                <template v-if="nodeInfo?.disk_total">
                  <span>{{ formatBytesSplit(nodeInfo.disk_total).value }}</span>
                  <span>{{ formatBytesSplit(nodeInfo.disk_total).unit }}</span>
                </template>
                <span v-else>-</span>
              </div>
            </MetricChartHeader>
          </template>
          <div class="h-48">
            <VChart :option="cardCharts.diskChartOption" autoresize />
          </div>
        </CardX>

        <!-- 网络卡片 -->
        <CardX v-if="isChartCardEnabled('network')" size="small" class="cursor-pointer bg-background/50 border-none hover:bg-background transition-all rounded-md" :style="getChartCardStyle('network')" role="button" tabindex="0" @click="openExpandedChart('network')" @keydown.enter="openExpandedChart('network')">
          <template #header>
            <MetricChartHeader title="实时网络" icon="tabler:network" tone="sky">
              <div class="text-xs flex gap-2 items-baseline">
                <span class="flex flex-row items-center justify-center gap-0.5">
                  <Icon icon="tabler:chevron-up" width="12" height="12" />
                  <template v-if="nodeInfo?.net_out != null">
                    {{ formatBytesSplit(nodeInfo.net_out).value }}
                    {{ formatBytesSplit(nodeInfo.net_out).unit }}/s
                  </template>
                  <template v-else>-</template>
                </span>
                <span class="flex flex-row items-center justify-center gap-0.5">
                  <Icon icon="tabler:chevron-down" width="12" height="12" />
                  <template v-if="nodeInfo?.net_in != null">
                    {{ formatBytesSplit(nodeInfo.net_in).value }}
                    {{ formatBytesSplit(nodeInfo.net_in).unit }}/s
                  </template>
                  <template v-else>-</template>
                </span>
              </div>
            </MetricChartHeader>
          </template>
          <div class="h-48">
            <VChart :option="cardCharts.networkChartOption" autoresize />
          </div>
        </CardX>

        <MetricSeriesChartCard
          v-if="isChartCardEnabled('traffic')"
          title="累计与周期流量"
          icon="tabler:arrows-transfer-up-down"
          tone="sky"
          :series="cardCharts.trafficChartSeries"
          :order="getChartCardOrder('traffic')"
        />

        <MetricSeriesChartCard
          v-if="isChartCardEnabled('temperature')"
          title="温度"
          icon="tabler:temperature"
          tone="orange"
          :series="cardCharts.temperatureChartSeries"
          :order="getChartCardOrder('temperature')"
        />

        <!-- 连接数卡片 -->
        <CardX v-if="isChartCardEnabled('connections')" size="small" class="cursor-pointer bg-background/50 border-none hover:bg-background transition-all rounded-md" :style="getChartCardStyle('connections')" role="button" tabindex="0" @click="openExpandedChart('connections')" @keydown.enter="openExpandedChart('connections')">
          <template #header>
            <MetricChartHeader title="网络连接" icon="tabler:binary-tree" tone="amber">
              <div class="text-xs flex gap-1 items-baseline">
                <span>TCP: {{ nodeInfo?.connections ?? '-' }}</span>
                <span>·</span>
                <span>UDP: {{ nodeInfo?.connections_udp ?? '-' }}</span>
              </div>
            </MetricChartHeader>
          </template>
          <div class="h-48">
            <VChart :option="cardCharts.connectionsChartOption" autoresize />
          </div>
        </CardX>

        <!-- 进程卡片 -->
        <CardX v-if="isChartCardEnabled('process')" size="small" class="cursor-pointer bg-background/50 border-none hover:bg-background transition-all rounded-md" :style="getChartCardStyle('process')" role="button" tabindex="0" @click="openExpandedChart('process')" @keydown.enter="openExpandedChart('process')">
          <template #header>
            <MetricChartHeader title="进程" icon="tabler:activity" tone="slate">
              <span class="text-xs">
                {{ nodeInfo?.process ?? '-' }}
              </span>
            </MetricChartHeader>
          </template>
          <div class="h-48">
            <VChart :option="cardCharts.processChartOption" autoresize />
          </div>
        </CardX>
      </div>
    </div>

    <AppDialog
      :open="expandedChart !== null"
      :title="expandedChartTitle"
      :description="expandedChartDescription"
      content-class="max-w-6xl"
      @update:open="setExpandedOpen"
    >
      <div class="flex flex-col gap-4">
        <div class="flex flex-col gap-2">
          <Tabs v-model="selectedView" class="w-full items-center">
            <div class="min-w-0 flex-1 overflow-x-auto rounded-sm pointer-events-auto">
              <TabsList class="w-max h-8 bg-background/50 backdrop-blur-xl rounded-md">
                <TabsTrigger
                  v-for="view in availableViews" :key="view.label" :value="view.label"
                  class="h-6.5 flex-none shrink-0 text-xs border-none data-[state=active]:text-green-600 shadow-none rounded-sm"
                >
                  {{ view.label }}
                </TabsTrigger>
              </TabsList>
            </div>
          </Tabs>

          <div v-if="isCustomRange" class="flex w-full flex-col items-center gap-2 sm:flex-row sm:justify-center">
            <div class="grid w-full gap-2 sm:w-auto sm:grid-cols-[minmax(0,13rem)_minmax(0,13rem)_auto]">
              <Input
                v-model="customStartInput"
                type="datetime-local"
                aria-label="负载图开始时间"
                class="h-8 bg-background/50 text-xs"
              />
              <Input
                v-model="customEndInput"
                type="datetime-local"
                aria-label="负载图结束时间"
                class="h-8 bg-background/50 text-xs"
              />
              <Button
                type="button"
                size="sm"
                variant="outline"
                :disabled="!customRange"
                class="h-8 text-xs"
                @click="fetchData"
              >
                应用
              </Button>
            </div>
            <div v-if="customRangeError" class="text-[11px] text-orange-500">
              {{ customRangeError }}
            </div>
          </div>
        </div>

        <Spinner :show="loading">
          <div v-if="error" class="py-8 text-center text-red-500">
            {{ error }}
          </div>
          <div v-else class="h-[min(62vh,34rem)] min-h-80 rounded-md bg-background/35 p-2">
            <VChart :option="expandedChartOption" autoresize />
          </div>
        </Spinner>
      </div>
    </AppDialog>
  </div>
</template>
