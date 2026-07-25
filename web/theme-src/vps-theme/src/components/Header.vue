<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed, inject, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { useAppStore } from '@/stores/app'

const router = useRouter()
const appStore = useAppStore()
const isScrolled = inject<ReturnType<typeof ref<boolean>>>('isScrolled', ref(false))
const avatarUrl = '/images/ethan-avatar.png'

const actionButtons = computed(() => {
  return [
    {
      title: appStore.isDark ? '深色主题（点击切换为亮色）' : '亮色主题（点击切换为暗色）',
      icon: appStore.isDark ? 'icon-park-outline:moon' : 'icon-park-outline:sun-one',
      action: 'toggleTheme',
    },
    { title: '后台管理', icon: 'icon-park-outline:setting', action: 'jumpToSetting' },
  ]
})

const sitename = computed(() => appStore.publicSettings?.sitename || 'VPS Monitor')

function handleButtonClick(action: string) {
  if (action === 'toggleTheme') {
    appStore.updateThemeMode()
    return
  }
  if (action === 'jumpToSetting')
    location.href = '/admin'
}
</script>

<template>
  <div
    class="transition-all duration-200 top-0 sticky z-10 border-b border-transparent"
    :class="isScrolled ? '!border-slate-500/10 backdrop-blur-lg' : 'bg-transparent'"
  >
    <div class="px-4 flex-between h-14 max-w-[1280px] mx-auto">
      <div class="flex items-center gap-3 cursor-pointer" @click="router.push('/')">
        <Avatar class="size-8">
          <AvatarImage :src="avatarUrl" :alt="sitename" />
          <AvatarFallback>{{ sitename.slice(0, 1) }}</AvatarFallback>
        </Avatar>
        <h3 class="m-0 text-lg font-semibold">
          {{ sitename }}
        </h3>
      </div>
      <TooltipProvider :delay-duration="200">
        <div class="flex items-center gap-2">
          <Tooltip v-for="button in actionButtons" :key="button.action">
            <TooltipTrigger as-child>
              <Button variant="ghost" size="icon-sm" :aria-label="button.title" @click="handleButtonClick(button.action)">
                <Icon :icon="button.icon" :width="18" :height="18" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{{ button.title }}</TooltipContent>
          </Tooltip>
        </div>
      </TooltipProvider>
    </div>
  </div>
</template>
