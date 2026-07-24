<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, defineAsyncComponent, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardX } from '@/components/ui/card-x'
import { Empty } from '@/components/ui/empty'
import { ProgressThin } from '@/components/ui/progress-thin'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'
import { formatBytesPerSecondWithConfig, formatBytesWithConfig, formatDateTime, formatUptimeWithFormat, getStatus } from '@/utils/helper'
import { message } from '@/utils/message'
import { getDiskPercentage, getMemoryPercentage, getTrafficUsed, getTrafficUsedPercentage, hasTrafficLimit } from '@/utils/nodeMetricsHelper'
import { getRegionCode, getRegionDisplayName } from '@/utils/regionHelper'

const PingChart = defineAsyncComponent(() => import('@/components/PingChart.vue'))
const LoadChart = defineAsyncComponent(() => import('@/components/LoadChart.vue'))
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const nodesStore = useNodesStore()

const data = computed(() => nodesStore.nodes.find(node => node.uuid === route.params.id))
const formatBytes = (bytes: number) => formatBytesWithConfig(bytes, appStore.byteDecimals)
const formatBytesPerSecond = (bytes: number) => formatBytesPerSecondWithConfig(bytes, appStore.byteDecimals)
const formatUptime = (seconds: number) => formatUptimeWithFormat(seconds, 'minute')
const getRegionAltText = (region: string) => getRegionDisplayName(region) || getRegionCode(region)
const lastReportTime = computed(() => {
  const timestamp = data.value?.status_updated_at || data.value?.time
  return timestamp ? formatDateTime(timestamp) : '--'
})
const copiedIp = ref<'ipv4' | 'ipv6' | null>(null)
let copyFeedbackTimer: ReturnType<typeof setTimeout> | null = null

async function copyIp(kind: 'ipv4' | 'ipv6', value?: string) {
  if (!value)
    return
  try {
    await navigator.clipboard.writeText(value)
  }
  catch {
    const textarea = document.createElement('textarea')
    textarea.value = value
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    const copied = document.execCommand('copy')
    textarea.remove()
    if (!copied) {
      message.error('复制失败，请手动复制')
      return
    }
  }

  copiedIp.value = kind
  message.success(`${kind === 'ipv4' ? 'IPv4' : 'IPv6'} 已复制`)
  if (copyFeedbackTimer)
    clearTimeout(copyFeedbackTimer)
  copyFeedbackTimer = setTimeout(() => {
    copiedIp.value = null
    copyFeedbackTimer = null
  }, 1600)
}

onBeforeUnmount(() => {
  if (copyFeedbackTimer)
    clearTimeout(copyFeedbackTimer)
})

const memPercentage = computed(() => data.value ? getMemoryPercentage(data.value) : 0)
const diskPercentage = computed(() => data.value ? getDiskPercentage(data.value) : 0)
const trafficUsed = computed(() => data.value ? getTrafficUsed(data.value) : 0)
const trafficUsedPercentage = computed(() => data.value ? getTrafficUsedPercentage(data.value) : 0)
const trafficStatus = computed(() => {
  if (!data.value || !hasTrafficLimit(data.value))
    return 'success'
  if (trafficUsedPercentage.value >= 95)
    return 'error'
  if (trafficUsedPercentage.value >= 80)
    return 'warning'
  if (trafficUsedPercentage.value >= 60)
    return 'info'
  return 'success'
})

const trafficResetText = computed(() => {
  if (!data.value?.traffic_reset_enabled)
    return '无'
  const day = data.value.traffic_reset_day || 1
  const hour = String(data.value.traffic_reset_hour ?? 0).padStart(2, '0')
  return `每月 ${day} 日 ${hour} 时重置`
})
</script>

<template>
  <div class="instance-detail space-y-4">
    <div v-if="!data" class="p-4">
      <CardX>
        <Empty description="节点不存在或已被删除">
          <template #extra>
            <Button @click="router.push('/')">
              返回首页
            </Button>
          </template>
        </Empty>
      </CardX>
    </div>

    <template v-else>
      <div class="px-4 flex gap-3 items-start sm:items-center">
        <Button variant="ghost" size="icon-sm" class="bg-background/50 hover:bg-background" aria-label="返回首页" @click="router.push('/')">
          <Icon icon="tabler:arrow-left" :width="16" :height="16" />
        </Button>
        <div class="min-w-0 flex-1 space-y-2">
          <div class="flex flex-wrap gap-2 items-center">
            <div class="text-lg font-bold flex gap-2 items-center">
              <img :src="`/images/flags/${getRegionCode(data.region)}.svg`" :alt="getRegionAltText(data.region)" class="size-6">
              <span>{{ data.name }}</span>
            </div>
            <Badge :variant="data.online ? 'default' : 'destructive'" class="text-xs !rounded">
              {{ data.online ? '在线' : '离线' }}
            </Badge>
          </div>
          <div class="detail-meta-grid">
            <div class="detail-meta-card">
              <span class="detail-meta-icon detail-meta-icon-clock">
                <Icon icon="tabler:clock" :width="16" :height="16" />
              </span>
              <span class="min-w-0">
                <span class="detail-meta-label">最后上报</span>
                <span class="detail-meta-value">{{ lastReportTime }}</span>
              </span>
            </div>
            <button
              type="button"
              class="detail-meta-card detail-meta-copy"
              :disabled="!data.ipv4"
              :aria-label="data.ipv4 ? `复制 IPv4 ${data.ipv4}` : '无 IPv4 地址'"
              @click="copyIp('ipv4', data.ipv4)"
            >
              <span class="detail-meta-icon detail-meta-icon-v4">
                <Icon icon="tabler:number-4" :width="16" :height="16" />
              </span>
              <span class="min-w-0 flex-1 text-left">
                <span class="detail-meta-label">IPv4</span>
                <span class="detail-meta-value font-mono">{{ data.ipv4 || '--' }}</span>
              </span>
              <Icon :icon="copiedIp === 'ipv4' ? 'tabler:check' : 'tabler:copy'" :width="15" :height="15" class="detail-copy-icon" />
            </button>
            <button
              type="button"
              class="detail-meta-card detail-meta-copy min-w-0"
              :disabled="!data.ipv6"
              :aria-label="data.ipv6 ? `复制 IPv6 ${data.ipv6}` : '无 IPv6 地址'"
              @click="copyIp('ipv6', data.ipv6)"
            >
              <span class="detail-meta-icon detail-meta-icon-v6">
                <Icon icon="tabler:number-6" :width="16" :height="16" />
              </span>
              <span class="min-w-0 flex-1 text-left">
                <span class="detail-meta-label">IPv6</span>
                <span class="detail-meta-value break-all font-mono">{{ data.ipv6 || '--' }}</span>
              </span>
              <Icon :icon="copiedIp === 'ipv6' ? 'tabler:check' : 'tabler:copy'" :width="15" :height="15" class="detail-copy-icon" />
            </button>
          </div>
        </div>
      </div>

      <div class="px-4 grid grid-cols-1 gap-4 lg:grid-cols-2">
        <CardX title="系统状态" size="small" class="bg-background/50 border-none rounded-md">
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div class="status-tile">
              <div class="flex justify-between text-sm">
                <span class="text-muted-foreground">CPU</span>
                <span class="tabular-nums font-medium">{{ (data.cpu ?? 0).toFixed(1) }}%</span>
              </div>
              <ProgressThin :percentage="data.cpu ?? 0" :status="getStatus(data.cpu ?? 0)" :height="4" />
            </div>

            <div class="status-tile">
              <div class="flex justify-between text-sm">
                <span class="text-muted-foreground">内存</span>
                <span class="tabular-nums font-medium">{{ memPercentage.toFixed(1) }}%</span>
              </div>
              <ProgressThin :percentage="memPercentage" :status="getStatus(memPercentage)" :height="4" />
              <div class="text-xs text-muted-foreground">
                {{ formatBytes(data.ram ?? 0) }} / {{ formatBytes(data.mem_total ?? 0) }}
              </div>
            </div>

            <div class="status-tile">
              <div class="flex justify-between text-sm">
                <span class="text-muted-foreground">硬盘</span>
                <span class="tabular-nums font-medium">{{ diskPercentage.toFixed(1) }}%</span>
              </div>
              <ProgressThin :percentage="diskPercentage" :status="getStatus(diskPercentage)" :height="4" />
              <div class="text-xs text-muted-foreground">
                {{ formatBytes(data.disk ?? 0) }} / {{ formatBytes(data.disk_total ?? 0) }}
              </div>
            </div>

            <div class="status-tile">
              <div class="flex justify-between text-sm">
                <span class="text-muted-foreground">累计流量</span>
                <span class="tabular-nums font-medium">{{ data.traffic_limit ? `${trafficUsedPercentage.toFixed(1)}%` : '∞' }}</span>
              </div>
              <ProgressThin :percentage="trafficUsedPercentage" :status="trafficStatus" :height="4" />
              <div class="text-xs text-muted-foreground">
                {{ formatBytes(trafficUsed) }}
                <template v-if="hasTrafficLimit(data)">
                  / {{ formatBytes(data.traffic_limit) }}
                </template>
                <template v-else>
                  / ∞
                </template>
              </div>
            </div>
          </div>
        </CardX>

        <CardX title="流量" size="small" class="bg-background/50 border-none rounded-md">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div class="grid grid-cols-1 gap-3">
              <div class="traffic-tile">
                <div class="text-xs text-muted-foreground">
                  运行时间
                </div>
                <div class="text-lg font-bold">
                  {{ formatUptime(data.uptime ?? 0) }}
                </div>
              </div>
              <div class="traffic-tile">
                <div class="text-xs text-muted-foreground">
                  流量重置时间
                </div>
                <div class="text-lg font-bold">
                  {{ trafficResetText }}
                </div>
              </div>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <div class="traffic-tile">
                <div class="text-xs text-muted-foreground">
                  实时上行
                </div>
                <div class="text-lg font-bold text-green-600">
                  {{ formatBytesPerSecond(data.net_out ?? 0) }}
                </div>
              </div>
              <div class="traffic-tile">
                <div class="text-xs text-muted-foreground">
                  实时下行
                </div>
                <div class="text-lg font-bold text-blue-600">
                  {{ formatBytesPerSecond(data.net_in ?? 0) }}
                </div>
              </div>
              <div class="traffic-tile">
                <div class="text-xs text-muted-foreground">
                  总上行
                </div>
                <div class="text-lg font-bold">
                  {{ formatBytes(data.net_total_up ?? 0) }}
                </div>
              </div>
              <div class="traffic-tile">
                <div class="text-xs text-muted-foreground">
                  总下行
                </div>
                <div class="text-lg font-bold">
                  {{ formatBytes(data.net_total_down ?? 0) }}
                </div>
              </div>
            </div>
          </div>
        </CardX>
      </div>

      <div class="px-4">
        <LoadChart :uuid="data.uuid" />
      </div>

      <div class="px-4">
        <CardX size="small" class="bg-background/50 border-none rounded-md">
          <PingChart :uuid="data.uuid" />
        </CardX>
      </div>
    </template>
  </div>
</template>

<style scoped>
.status-tile,
.traffic-tile {
  height: 82px;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  padding: 0.75rem;
  border: 1px solid rgb(100 116 139 / 0.1);
  border-radius: 0.5rem;
  background: rgb(100 116 139 / 0.05);
}

.detail-meta-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.detail-meta-card {
  min-height: 44px;
  display: inline-flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.45rem 0.65rem;
  border: 1px solid rgb(148 163 184 / 0.14);
  border-radius: 0.65rem;
  background: linear-gradient(135deg, rgb(255 255 255 / 0.08), rgb(148 163 184 / 0.035));
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.05);
  backdrop-filter: blur(8px);
}

.detail-meta-icon {
  width: 28px;
  height: 28px;
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
}

.detail-meta-icon-clock {
  color: rgb(56 189 248);
  background: rgb(14 165 233 / 0.12);
}

.detail-meta-icon-v4 {
  color: rgb(52 211 153);
  background: rgb(16 185 129 / 0.12);
}

.detail-meta-icon-v6 {
  color: rgb(167 139 250);
  background: rgb(139 92 246 / 0.12);
}

.detail-meta-label,
.detail-meta-value {
  display: block;
}

.detail-meta-label {
  margin-bottom: 0.1rem;
  color: rgb(148 163 184 / 0.85);
  font-size: 0.65rem;
  line-height: 1;
}

.detail-meta-value {
  color: rgb(226 232 240 / 0.9);
  font-size: 0.75rem;
  line-height: 1.15rem;
  font-variant-numeric: tabular-nums;
}

.detail-meta-copy {
  cursor: pointer;
  transition:
    border-color 160ms ease,
    background-color 160ms ease,
    transform 160ms ease;
}

.detail-meta-copy:hover {
  border-color: rgb(34 197 94 / 0.28);
  background: rgb(255 255 255 / 0.1);
  transform: translateY(-1px);
}

.detail-meta-copy:focus-visible {
  outline: 2px solid rgb(34 197 94 / 0.45);
  outline-offset: 2px;
}

.detail-meta-copy:disabled {
  cursor: default;
  opacity: 0.55;
  transform: none;
}

.detail-copy-icon {
  flex: 0 0 auto;
  color: rgb(148 163 184 / 0.75);
}

@media (max-width: 640px) {
  .detail-meta-card {
    width: 100%;
  }
}
</style>
