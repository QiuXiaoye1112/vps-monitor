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
  const themeTitleMap = {
    auto: appStore.managedThemeMode === 'beijing'
      ? appStore.isBeijingDaytime ? '自动主题：北京时间日间' : '自动主题：北京时间夜间'
      : appStore.managedThemeMode === 'light' ? '自动主题：后台浅色' : '自动主题：后台深色',
    light: '浅色主题',
    dark: '深色主题',
  } as const
  const themeIconMap = {
    auto: appStore.isDark ? 'icon-park-outline:moon' : 'icon-park-outline:sun-one',
    light: 'icon-park-outline:sun-one',
    dark: 'icon-park-outline:moon',
  } as const

  return [
    {
      title: `${themeTitleMap[appStore.themeMode]}（点击切换）`,
      icon: themeIconMap[appStore.themeMode],
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
