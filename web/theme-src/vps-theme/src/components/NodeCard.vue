<script setup lang="ts">
import type { PingTaskRow } from '@/composables/useNodePingTaskDisplay'
import type { NodeData } from '@/stores/nodes'
import { Icon } from '@iconify/vue'
import { computed } from 'vue'
import { CardX } from '@/components/ui/card-x'
import { DataTooltip } from '@/components/ui/data-tooltip'
import { ProgressThin } from '@/components/ui/progress-thin'
import { useNodeCardPingTasks } from '@/composables/useNodePingTaskDisplay'
import { useAppStore } from '@/stores/app'
import { formatBytesPerSecondWithConfig, formatBytesWithConfig, formatDateTime, getStatus, getUptimeDays } from '@/utils/helper'
import { getDiskPercentage, getMemoryPercentage, getTrafficUsed, getTrafficUsedPercentage, hasTrafficLimit } from '@/utils/nodeMetricsHelper'
import { getOSImage, getOSName } from '@/utils/osImageHelper'
import { getRegionCode, getRegionDisplayName } from '@/utils/regionHelper'

const props = withDefaults(defineProps<{
  node: NodeData
  reduceMotion?: boolean
}>(), {
  reduceMotion: false,
})
const emit = defineEmits<{
  click: []
  pingClick: []
}>()
const appStore = useAppStore()

const nodeCardXSize = computed(() => appStore.nodeCardSize === 'large' ? 'large' : 'medium')
const nodeCardContentClass = computed(() => appStore.nodeCardSize === 'large' ? 'gap-4' : 'gap-3')
const nodeCardMetricBoxClass = computed(() => appStore.nodeCardSize === 'compact' ? 'px-1.5 py-1.5' : 'px-2 py-1.5')

const formatBytes = (bytes: number) => formatBytesWithConfig(bytes, appStore.byteDecimals)
const formatBytesPerSecond = (bytes: number) => formatBytesPerSecondWithConfig(bytes, appStore.byteDecimals)
const offlineTime = computed(() => formatDateTime(props.node.time))

const cpuStatus = computed(() => getStatus(props.node.cpu ?? 0))
const memPercentage = computed(() => getMemoryPercentage(props.node))
const memStatus = computed(() => getStatus(memPercentage.value))
const diskPercentage = computed(() => getDiskPercentage(props.node))
const diskStatus = computed(() => getStatus(diskPercentage.value))
const trafficUsedPercentage = computed(() => getTrafficUsedPercentage(props.node))
const trafficUsed = computed(() => getTrafficUsed(props.node))

const { taskRows } = useNodeCardPingTasks(
  () => props.node.uuid,
  { taskOrder: () => props.node.ping_task_order ?? [] },
)

const trafficStatus = computed(() => {
  if (!hasTrafficLimit(props.node))
    return 'success'
  if (trafficUsedPercentage.value >= 95)
    return 'error'
  if (trafficUsedPercentage.value >= 80)
    return 'warning'
  if (trafficUsedPercentage.value >= 60)
    return 'info'
  return 'success'
})

const trafficPercentageClass = computed(() => {
  if (!hasTrafficLimit(props.node))
    return 'text-muted-foreground'
  if (trafficUsedPercentage.value >= 95)
    return 'text-red-500'
  if (trafficUsedPercentage.value >= 80)
    return 'text-orange-500'
  if (trafficUsedPercentage.value >= 60)
    return 'text-yellow-500'
  return 'text-green-600'
})

const uptimeDaysText = computed(() => {
  const days = getUptimeDays(props.node.uptime)
  return appStore.lang === 'zh-CN' ? `在线 ${days} 天` : `${days} days online`
})

function getRegionAltText(region: string): string {
  return getRegionDisplayName(region) || getRegionCode(region)
}

function hasRegion(region: string | null | undefined): boolean {
  return Boolean(region?.trim())
}

function taskSummary(): string {
  const count = taskRows.value[0]?.id === 'empty' ? 0 : taskRows.value.length
  if (count === 3)
    return '三网'
  return count > 0 ? `${count} 项` : ''
}

function barsFor(row: PingTaskRow, metric: 'latency' | 'loss') {
  return metric === 'latency' ? row.latencyBars : row.lossBars
}

function displayFor(row: PingTaskRow, metric: 'latency' | 'loss') {
  return metric === 'latency' ? row.latencyDisplay : row.lossDisplay
}
</script>

<template>
  <CardX
    hoverable
    :size="nodeCardXSize"
    class="node-card w-full cursor-pointer border-none shadow-[0_0_0_3px] shadow-transparent transition-all duration-200 rounded-xl"
    :class="[!props.node.online && '!shadow-red-500/30']"
    role="button"
    tabindex="0"
    :aria-label="`查看节点 ${props.node.name} 详情`"
    @click="emit('click')"
  >
    <template #header>
      <div class="flex items-center gap-2 min-w-0">
        <div class="relative size-2.5 shrink-0">
          <span class="size-2.5 rounded-full block" :class="props.node.online ? 'bg-green-500' : 'bg-red-500'" />
          <span
            v-if="!props.reduceMotion"
            class="animate-ping absolute inset-0 rounded-full opacity-60"
            :class="props.node.online ? 'bg-green-500' : 'bg-red-500'"
          />
        </div>
        <span class="text-sm font-bold flex-1 min-w-0 truncate">{{ props.node.name }}</span>
      </div>
    </template>

    <template #header-extra>
      <div class="flex gap-1.5 items-center shrink-0">
        <img :src="getOSImage(props.node.os)" :alt="getOSName(props.node.os)" class="size-4">
        <img
          v-if="hasRegion(props.node.region)"
          :src="`/images/flags/${getRegionCode(props.node.region)}.svg`"
          :alt="getRegionAltText(props.node.region)"
          class="size-5 shrink-0"
        >
      </div>
    </template>

    <template #default>
      <div class="flex flex-col relative" :class="nodeCardContentClass">
        <div class="relative z-20 flex items-center gap-1.5 -mt-1 h-[19px] overflow-hidden">
          <span class="shrink-0 text-[11px] px-2 py-0.5 rounded-full bg-slate-500/10 text-muted-foreground leading-tight">
            {{ uptimeDaysText }}
          </span>
        </div>

        <div class="grid grid-cols-2 gap-x-4 gap-y-2.5">
          <div class="flex flex-col gap-1">
            <div class="flex justify-between text-xs">
              <span class="text-muted-foreground">CPU</span>
              <span class="tabular-nums font-medium">{{ (props.node.cpu ?? 0).toFixed(1) }}%</span>
            </div>
            <ProgressThin :percentage="props.node.cpu ?? 0" :status="cpuStatus" :height="4" />
          </div>

          <div class="flex flex-col gap-1">
            <div class="flex justify-between text-xs">
              <span class="text-muted-foreground">内存</span>
              <span class="tabular-nums font-medium">{{ memPercentage.toFixed(1) }}%</span>
            </div>
            <ProgressThin :percentage="memPercentage" :status="memStatus" :height="4" />
            <div class="text-[11px] text-muted-foreground truncate">
              {{ formatBytes(props.node.ram ?? 0) }} / {{ formatBytes(props.node.mem_total ?? 0) }}
            </div>
          </div>

          <div class="flex flex-col gap-1">
            <div class="flex justify-between text-xs">
              <span class="text-muted-foreground">硬盘</span>
              <span class="tabular-nums font-medium">{{ diskPercentage.toFixed(1) }}%</span>
            </div>
            <ProgressThin :percentage="diskPercentage" :status="diskStatus" :height="4" />
            <div class="text-[11px] text-muted-foreground truncate">
              {{ formatBytes(props.node.disk ?? 0) }} / {{ formatBytes(props.node.disk_total ?? 0) }}
            </div>
          </div>

          <div class="flex flex-col gap-1">
            <div class="flex justify-between text-xs">
              <span class="text-muted-foreground">流量</span>
              <span class="tabular-nums font-medium" :class="trafficPercentageClass">
                {{ hasTrafficLimit(props.node) ? `${trafficUsedPercentage.toFixed(1)}%` : '∞' }}
              </span>
            </div>
            <ProgressThin :percentage="trafficUsedPercentage" :status="trafficStatus" :height="4" />
            <div class="text-[11px] truncate" :class="trafficUsedPercentage >= 95 ? 'text-red-500' : 'text-muted-foreground'">
              {{ (trafficUsed / 1073741824).toFixed(2) }} GB
              <template v-if="hasTrafficLimit(props.node)">
                / {{ (props.node.traffic_limit / 1073741824).toFixed(2) }} GB
              </template>
              <template v-else>
                / ∞
              </template>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-1.5">
          <div class="flex flex-col gap-0.5 rounded-lg bg-slate-500/5 min-w-0 overflow-hidden" :class="nodeCardMetricBoxClass">
            <div class="text-[11px] text-green-600 flex items-center gap-1">
              <Icon icon="tabler:chevron-up" width="11" height="11" />
              <span class="truncate min-w-0 overflow-hidden">{{ formatBytesPerSecond(props.node.net_out ?? 0) }}</span>
            </div>
            <div class="text-[11px] text-blue-600 flex items-center gap-1">
              <Icon icon="tabler:chevron-down" width="11" height="11" />
              <span class="truncate min-w-0 overflow-hidden">{{ formatBytesPerSecond(props.node.net_in ?? 0) }}</span>
            </div>
          </div>

          <div class="flex flex-col gap-0.5 rounded-lg bg-slate-500/5 min-w-0 overflow-hidden" :class="nodeCardMetricBoxClass">
            <div class="text-[11px] text-muted-foreground flex items-center gap-1">
              <Icon icon="tabler:upload" width="11" height="11" />
              <span class="truncate min-w-0 overflow-hidden">{{ formatBytes(props.node.net_total_up ?? 0) }}</span>
            </div>
            <div class="text-[11px] text-muted-foreground flex items-center gap-1">
              <Icon icon="tabler:download" width="11" height="11" />
              <span class="truncate min-w-0 overflow-hidden">{{ formatBytes(props.node.net_total_down ?? 0) }}</span>
            </div>
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div
            v-for="metric in (['latency', 'loss'] as const)"
            :key="metric"
            class="group/panel relative flex flex-col gap-2 min-w-0 overflow-hidden"
            :class="!props.node.online ? 'blur-xs opacity-50' : ''"
            :title="metric === 'latency' ? '各 Ping 任务延迟' : '各 Ping 任务丢包'"
            @click.stop="emit('pingClick')"
          >
            <div class="flex items-center justify-between px-0.5 text-xs leading-none">
              <span class="text-muted-foreground">{{ metric === 'latency' ? '延迟' : '丢包' }}</span>
              <span class="text-muted-foreground">{{ taskSummary() }}</span>
            </div>
            <div class="flex flex-col gap-2.5">
              <div v-for="row in taskRows" :key="`${metric}-${row.id}`" class="flex flex-col gap-1 min-w-0 px-0.5">
                <div class="flex items-center justify-between gap-1 text-xs leading-none">
                  <span class="flex items-center gap-1 min-w-0 overflow-hidden">
                    <span class="block shrink-0 rounded-full size-2" :style="{ backgroundColor: row.color }" />
                    <span class="truncate min-w-0">{{ row.name }}</span>
                  </span>
                  <span class="font-semibold tabular-nums shrink-0">{{ displayFor(row, metric) }}</span>
                </div>
                <div
                  class="grid items-end gap-[1px] opacity-80 group-hover/panel:opacity-100"
                  :style="{ height: '9px', gridTemplateColumns: `repeat(${barsFor(row, metric).length}, minmax(0, 1fr))` }"
                >
                  <DataTooltip
                    v-for="bar in barsFor(row, metric)"
                    :key="bar.key"
                    placement="top"
                    :content="bar.tooltip"
                    class="h-full w-full"
                  >
                    <span
                      class="block h-full w-full rounded-[1px] transition-transform duration-150 group-hover/data-tooltip:scale-y-160 group-hover/panel:opacity-60 group-hover/data-tooltip:!opacity-100"
                      :class="bar.className"
                    />
                  </DataTooltip>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="!props.node.online"
          class="absolute inset-0 flex flex-col items-center justify-center z-10 rounded-xl bg-white/20 dark:bg-black/20 backdrop-blur-[2px]"
        >
          <div class="text-sm font-semibold text-destructive">
            离线
          </div>
          <div class="text-[11px] text-muted-foreground mt-1">
            {{ offlineTime }}
          </div>
        </div>
      </div>
    </template>
  </CardX>
</template>

<style scoped>
.node-card {
  position: relative;
  overflow: hidden;
}
</style>
