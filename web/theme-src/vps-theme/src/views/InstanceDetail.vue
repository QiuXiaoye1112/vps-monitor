<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, defineAsyncComponent } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CardX } from '@/components/ui/card-x'
import { Empty } from '@/components/ui/empty'
import { ProgressThin } from '@/components/ui/progress-thin'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'
import { formatBytesPerSecondWithConfig, formatBytesWithConfig, formatDateTime, formatUptimeWithFormat, getStatus } from '@/utils/helper'
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
          <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
            <span class="detail-meta">
              <Icon icon="tabler:clock" :width="14" :height="14" />
              最后上报 {{ lastReportTime }}
            </span>
            <span class="detail-meta">
              <Icon icon="tabler:number-4" :width="14" :height="14" />
              IPv4 {{ data.ipv4 || '--' }}
            </span>
            <span class="detail-meta min-w-0">
              <Icon icon="tabler:number-6" :width="14" :height="14" class="shrink-0" />
              <span class="break-all">IPv6 {{ data.ipv6 || '--' }}</span>
            </span>
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

.detail-meta {
  display: inline-flex;
  align-items: center;
  gap: 0.25rem;
}
</style>
