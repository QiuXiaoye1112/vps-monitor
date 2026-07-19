<script setup lang="ts">
import type { NodeData } from '@/stores/nodes'
import { computed, defineAsyncComponent, nextTick, onActivated, onDeactivated } from 'vue'
import { useRouter } from 'vue-router'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Empty } from '@/components/ui/empty'
import { useAppStore } from '@/stores/app'
import { useNodesStore } from '@/stores/nodes'

defineOptions({ name: 'HomeView' })

const NodeCard = defineAsyncComponent(() => import('@/components/NodeCard.vue'))
const NodeGeneralCards = defineAsyncComponent(() => import('@/components/NodeGeneralCards.vue'))

const nodeItemStaggerMs = 35
const nodeItemStaggerLimit = 12
const appStore = useAppStore()
const nodesStore = useNodesStore()
const router = useRouter()

onActivated(() => {
  if (appStore.homeScrollPosition <= 0)
    return
  nextTick(() => window.scrollTo({ top: appStore.homeScrollPosition, behavior: 'instant' }))
})

onDeactivated(() => {
  appStore.homeScrollPosition = window.scrollY
})

const nodeList = computed(() => nodesStore.visibleNodes)

const nodeGridClass = computed(() => [
  'grid grid-cols-1',
  {
    mini: 'gap-3 sm:grid-cols-[repeat(auto-fill,minmax(300px,1fr))]',
    compact: 'gap-3 sm:grid-cols-[repeat(auto-fill,minmax(300px,1fr))]',
    comfortable: 'gap-4 sm:grid-cols-[repeat(auto-fill,minmax(360px,1fr))]',
    large: 'gap-5 sm:grid-cols-[repeat(auto-fill,minmax(420px,1fr))]',
  }[appStore.nodeCardSize],
])

function handleNodeClick(node: NodeData) {
  router.push({ name: 'instance-detail', params: { id: node.uuid } })
}

function getNodeItemTransitionStyle(index: number): Record<string, string> {
  return {
    '--node-item-delay': `${Math.min(index, nodeItemStaggerLimit) * nodeItemStaggerMs}ms`,
  }
}
</script>

<template>
  <div class="home-view">
    <div v-if="appStore.connectionError" class="alert px-4">
      <Alert variant="destructive" class="border-none backdrop-blur-xs bg-red-400/10 rounded-md">
        <AlertTitle>RPC 服务错误</AlertTitle>
        <AlertDescription>连接服务器失败，请检查网络设置或刷新页面后再试。</AlertDescription>
      </Alert>
    </div>

    <NodeGeneralCards
      v-if="!appStore.hideGeneralCard"
      :nodes="nodeList"
      :globe-nodes="nodeList"
      transition-key="all"
    />

    <div
      class="node-info p-4 pt-0 flex flex-col gap-4 relative z-1 pointer-events-none"
      :class="appStore.hideGeneralCard && 'pt-4'"
    >
      <div v-if="nodeList.length" :class="nodeGridClass">
        <div
          v-for="(node, index) in nodeList"
          :key="node.uuid"
          class="min-w-0 pointer-events-auto"
          :style="getNodeItemTransitionStyle(index)"
        >
          <NodeCard :node="node" @click="handleNodeClick(node)" />
        </div>
      </div>
      <div v-else class="pointer-events-auto text-muted-foreground text-center py-8">
        <Empty description="暂无节点" />
      </div>
    </div>
  </div>
</template>
