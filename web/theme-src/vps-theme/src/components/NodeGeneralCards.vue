<script setup lang="ts">
import type { GeneralCardKey } from '@/stores/app'
import type { NodeData } from '@/stores/nodes'
import { Icon } from '@iconify/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import NodeEarthGlobe from '@/components/NodeEarthGlobe.vue'
import { CardX } from '@/components/ui/card-x'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'
import { formatBytesPerSecondSplit, formatBytesSplit } from '@/utils/helper'

interface GeneralMetricCard {
  key: GeneralCardKey
  label: string
  icon: string
  value: string
  unit?: string
}

const props = defineProps<{
  nodes?: NodeData[]
  globeNodes?: NodeData[]
  transitionKey?: string
}>()

const appStore = useAppStore()
const nodesStore = useNodesStore()
const summaryNodes = computed(() => props.nodes ?? nodesStore.visibleNodes)
const summaryTransitionKey = computed(() => props.transitionKey ?? nodesStore.visibleNodes.length)
const onlineNodes = computed(() => summaryNodes.value.filter(node => node.online))
const totalCount = computed(() => summaryNodes.value.length)

const totalSpeed = computed(() => onlineNodes.value.reduce(
  (result, node) => {
    result.up += node.net_out || 0
    result.down += node.net_in || 0
    return result
  },
  { up: 0, down: 0 },
))

// 多个节点的 status 事件并不会同时到达。总览卡不直接消费每个事件，
// 而是每秒采样一次所有节点的最新速度，避免节点分批上报时总览数字连续跳动。
const displayedTotalSpeed = ref({ ...totalSpeed.value })
let totalSpeedRefreshTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  totalSpeedRefreshTimer = window.setInterval(() => {
    displayedTotalSpeed.value = { ...totalSpeed.value }
  }, 1000)
})

onBeforeUnmount(() => {
  if (totalSpeedRefreshTimer)
    clearInterval(totalSpeedRefreshTimer)
  totalSpeedRefreshTimer = null
})

const totalTraffic = computed(() => summaryNodes.value.reduce(
  (result, node) => {
    result.up += node.net_total_up || 0
    result.down += node.net_total_down || 0
    return result
  },
  { up: 0, down: 0 },
))

const formattedTraffic = computed(() => formatBytesSplit(
  totalTraffic.value.up + totalTraffic.value.down,
  appStore.byteDecimals,
))
const formattedSpeedUp = computed(() => formatBytesPerSecondSplit(displayedTotalSpeed.value.up, appStore.byteDecimals))
const formattedSpeedDown = computed(() => formatBytesPerSecondSplit(displayedTotalSpeed.value.down, appStore.byteDecimals))

function formatCount(value: number): string {
  return Math.round(value).toLocaleString('zh-CN')
}

function resolveMetricCard(key: GeneralCardKey): GeneralMetricCard {
  switch (key) {
    case 'onlineNodes':
      return {
        key,
        label: '在线节点',
        icon: 'tabler:activity-heartbeat',
        value: formatCount(onlineNodes.value.length),
        unit: `/ ${formatCount(totalCount.value)}`,
      }
    case 'totalTraffic':
      return {
        key,
        label: '累计流量',
        icon: 'tabler:download',
        value: formattedTraffic.value.value,
        unit: formattedTraffic.value.unit,
      }
    case 'uploadSpeed':
      return {
        key,
        label: '实时上行',
        icon: 'tabler:chevrons-up',
        value: formattedSpeedUp.value.value,
        unit: formattedSpeedUp.value.unit,
      }
    default:
      return {
        key: 'downloadSpeed',
        label: '实时下行',
        icon: 'tabler:chevrons-down',
        value: formattedSpeedDown.value.value,
        unit: formattedSpeedDown.value.unit,
      }
  }
}

const metricCards = computed(() => (
  ['onlineNodes', 'totalTraffic', 'uploadSpeed', 'downloadSpeed'] as GeneralCardKey[]
).map(resolveMetricCard))

const showEarth = computed(() => !appStore.hideEarth)
const tiledEarth = computed(() => showEarth.value && appStore.earthRenderer === 'tiled')
const showGeneral = computed(() => showEarth.value || metricCards.value.length > 0)
const wrapperClass = computed(() => {
  if (!showEarth.value)
    return 'p-4 grid grid-cols-1 gap-2 h-auto'
  if (tiledEarth.value)
    return 'p-3 sm:p-4 grid grid-cols-12 gap-2 sm:gap-3 h-auto min-h-[40rem] sm:min-h-[30rem] md:min-h-[36rem] lg:min-h-[40rem]'
  return 'p-4 grid grid-cols-12 grid-rows-1 gap-2 h-auto md:h-58'
})
const earthClass = computed(() => tiledEarth.value
  ? 'col-span-12 row-start-2 min-h-[18rem] h-[18rem] sm:h-[20rem] md:h-[24rem] lg:h-[28rem]'
  : 'col-span-12 col-start-1 md:col-span-6 md:col-start-7 md:row-start-1')
const cardsClass = computed(() => {
  if (!showEarth.value)
    return 'col-span-1 grid grid-cols-2 md:grid-cols-4 gap-2'
  if (tiledEarth.value)
    return 'col-span-12 row-start-1 z-9 grid grid-cols-12 auto-rows-[4.75rem] sm:auto-rows-[5rem] md:auto-rows-[5.8rem] gap-2 sm:gap-3'
  return 'h-42 -mt-42 md:mt-0 col-span-12 row-start-3 z-9 md:h-auto md:col-span-6 md:row-start-1 grid grid-cols-12 grid-rows-2 gap-2'
})

const regularCardClasses = [
  'col-span-6 row-span-1 col-start-1 row-start-1',
  'col-span-6 row-span-1 col-start-7 row-start-1',
  'col-span-6 row-span-1 col-start-1 row-start-2',
  'col-span-6 row-span-1 col-start-7 row-start-2',
]
const tiledCardClasses = [
  'col-span-6 sm:col-span-3 row-span-1 sm:col-start-1 row-start-1',
  'col-span-6 sm:col-span-3 row-span-1 sm:col-start-4 row-start-1',
  'col-span-6 sm:col-span-3 row-span-1 sm:col-start-7 row-start-2 sm:row-start-1',
  'col-span-6 sm:col-span-3 row-span-1 sm:col-start-10 row-start-2 sm:row-start-1',
]

function getCardClass(index: number): string {
  if (!showEarth.value)
    return 'col-span-1 min-h-18 md:min-h-28'
  return tiledEarth.value
    ? tiledCardClasses[index] ?? 'col-span-6 sm:col-span-3 row-span-1'
    : regularCardClasses[index] ?? 'col-span-6 row-span-1'
}

function getMetricStyle(index: number): Record<string, string> {
  return { '--metric-switch-delay': `${index * 35}ms` }
}
</script>

<template>
  <div v-if="showGeneral" :class="wrapperClass">
    <NodeEarthGlobe
      v-if="showEarth"
      :nodes="globeNodes"
      :class="earthClass"
    />

    <div v-if="metricCards.length" :class="cardsClass">
      <CardX
        v-for="(metric, index) in metricCards"
        :key="metric.key"
        hoverable
        class="group relative z-10 h-full bg-background/50 border-none hover:bg-background backdrop-blur-sm md:backdrop-blur-none transition-all" :class="[
          getCardClass(index),
        ]"
        content-class="h-full !p-3"
      >
        <div class="flex h-full flex-col justify-between gap-1">
          <div class="flex items-start justify-between gap-2">
            <span class="text-xs font-semibold tracking-wider text-[var(--glass-light-text)] dark:text-[var(--glass-dark-muted-text)] truncate">{{ metric.label }}</span>
            <Icon
              :icon="metric.icon"
              :width="20"
              :height="20"
              class="shrink-0 text-[var(--glass-light-text)] opacity-60 group-hover:opacity-90 dark:text-[var(--glass-dark-muted-text)] transition-opacity"
            />
          </div>
          <div class="flex min-w-0 flex-col gap-1.5">
            <div
              :key="`${metric.key}-${summaryTransitionKey}`"
              class="flex items-baseline gap-1 min-w-0"
              :style="getMetricStyle(index)"
            >
              <span class="text-md md:text-2xl font-bold leading-none tracking-tight truncate">{{ metric.value }}</span>
              <span v-if="metric.unit" class="text-[11px] md:text-xs font-semibold text-[var(--glass-light-text)] dark:text-[var(--glass-dark-muted-text)] truncate">
                {{ metric.unit }}
              </span>
            </div>
          </div>
        </div>
      </CardX>
    </div>
  </div>
</template>
