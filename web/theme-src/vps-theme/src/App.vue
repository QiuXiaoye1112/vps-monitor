<script setup lang="ts">
import { nextTick, onMounted, onUnmounted } from 'vue'
import { Toaster } from '@/components/ui/sonner'
import { useAppStore } from '@/stores/app'
import { destroyInitManager, initApp } from '@/utils/init'
import Background from './components/Background.vue'
import Header from './components/Header.vue'
import LoadingCover from './components/LoadingCover.vue'
import Provider from './components/Provider.vue'

const appStore = useAppStore()

onMounted(async () => {
  try {
    const backgroundReady = (window as Window & {
      __VPS_BACKGROUND_READY__?: Promise<unknown>
    }).__VPS_BACKGROUND_READY__
    if (backgroundReady)
      await backgroundReady

    await initApp()
    await nextTick()
  }
  catch (error) {
    console.error('[App] Initialization failed:', error)
  }
})

onUnmounted(() => {
  destroyInitManager()
})
</script>

<template>
  <Provider>
    <Background />
    <LoadingCover v-if="appStore.loading" />
    <Header />
    <main v-if="!appStore.loading" class="min-h-screen overflow-hidden">
      <div class="max-w-[1280px] mx-auto">
        <RouterView v-slot="{ Component }">
          <component :is="Component" v-if="Component" />
        </RouterView>
      </div>
    </main>
    <Toaster rich-colors close-button position="top-center" />
  </Provider>
</template>
