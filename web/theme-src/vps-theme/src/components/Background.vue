<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()

const isLoaded = ref(false)
const hasError = ref(false)

const showBackground = computed(() => appStore.backgroundEnabled)
const currentUrl = computed(() => showBackground.value ? appStore.currentBackgroundUrl : '')
const hasCustomBackground = computed(() => showBackground.value && !!currentUrl.value)
const showBackgroundOverlay = computed(() => appStore.backgroundOverlay > 0)

const backgroundStyle = computed(() => {
  const blur = appStore.backgroundBlur
  return {
    filter: blur > 0 ? `blur(${blur}px)` : 'none',
    opacity: 1,
  }
})

const backgroundContainerStyle = computed(() => {
  const overlay = appStore.backgroundOverlay
  if (overlay >= 0)
    return {}

  return { opacity: 1 - Math.abs(overlay) / 100 }
})

const overlayStyle = computed(() => {
  const overlay = appStore.backgroundOverlay
  if (overlay <= 0)
    return {}

  return { backgroundColor: `rgba(0, 0, 0, ${overlay / 100})` }
})

const showLoadedBackground = computed(() =>
  hasCustomBackground.value && isLoaded.value && !hasError.value,
)

const showMediaBackground = computed(() =>
  showLoadedBackground.value,
)

const showDefaultBackground = computed(() =>
  !hasCustomBackground.value || hasError.value,
)

let imageLoader: HTMLImageElement | null = null

function clearImageLoader() {
  if (imageLoader) {
    imageLoader.onload = null
    imageLoader.onerror = null
    imageLoader = null
  }
}

function loadImage(url: string) {
  const preloadedUrl = (window as Window & { __VPS_BACKGROUND_PRELOADED__?: string }).__VPS_BACKGROUND_PRELOADED__
  if (preloadedUrl === url) {
    isLoaded.value = true
    hasError.value = false
    return
  }

  isLoaded.value = false
  hasError.value = false

  clearImageLoader()

  imageLoader = new Image()
  imageLoader.onload = () => {
    isLoaded.value = true
    hasError.value = false
  }
  imageLoader.onerror = () => {
    isLoaded.value = false
    hasError.value = true
  }
  imageLoader.src = url
}

function resetBackgroundState() {
  clearImageLoader()
  isLoaded.value = false
  hasError.value = false
}

watch([showBackground, currentUrl], ([enabled, url]) => {
  if (!enabled || !url) {
    resetBackgroundState()
    return
  }

  loadImage(url)
}, { immediate: true })

onUnmounted(() => {
  resetBackgroundState()
})
</script>

<template>
  <div class="background-container" :style="backgroundContainerStyle">
    <Transition name="fade">
      <div v-if="showDefaultBackground" class="default-background">
        <div class="default-background__spotlight">
          <div class="default-background__emerald-surface">
            <svg aria-hidden="true" class="default-background__pattern">
              <defs>
                <pattern id="glassmorphism-emerald-grid" width="72" height="56" patternUnits="userSpaceOnUse" x="-12" y="4">
                  <path d="M.5 56V.5H72" fill="none" />
                </pattern>
              </defs>
              <rect width="100%" height="100%" stroke-width="0" fill="url(#glassmorphism-emerald-grid)" />
              <svg x="-12" y="4" class="default-background__pattern-blocks">
                <rect stroke-width="0" width="73" height="57" x="288" y="168" />
                <rect stroke-width="0" width="73" height="57" x="144" y="56" />
                <rect stroke-width="0" width="73" height="57" x="504" y="168" />
                <rect stroke-width="0" width="73" height="57" x="720" y="336" />
              </svg>
            </svg>
          </div>
        </div>
      </div>
    </Transition>
    <Transition name="fade">
      <div v-if="showMediaBackground" class="background-media" :style="backgroundStyle">
        <div
          class="background-image"
          :style="{ backgroundImage: `url(${currentUrl})` }"
        />
      </div>
    </Transition>
    <div v-if="showBackgroundOverlay" class="background-overlay" :style="overlayStyle" />
  </div>
</template>

<style scoped>
.background-container {
  position: fixed;
  inset: 0;
  z-index: -1;
  overflow: hidden;
}

.default-background {
  position: absolute;
  inset: 0;
  overflow: hidden;
  background: #f8fafc;
  transform: scale(1.5);
  transform-origin: top center;
}

.dark .default-background {
  background: #0f172a;
}

.default-background__spotlight,
.default-background__emerald-surface,
.default-background__pattern,
.default-background__pattern-blocks {
  position: absolute;
}

.default-background__spotlight {
  top: 0;
  left: 50%;
  width: 81.25rem;
  height: 25rem;
  margin-left: -38rem;
  pointer-events: none;
  mask-image: linear-gradient(white, transparent);
}

.default-background__emerald-surface {
  inset: 0;
  overflow: hidden;
  background: linear-gradient(90deg, rgb(16 185 129 / 40%), rgb(190 242 100 / 40%));
  opacity: 0.4;
  mask-image: radial-gradient(farthest-side at top, white, transparent);
}

.dark .default-background__emerald-surface {
  background: linear-gradient(90deg, rgb(16 185 129 / 30%), rgb(190 242 100 / 30%));
  opacity: 1;
}

.default-background__pattern {
  inset-inline: 0;
  top: -50%;
  width: 100%;
  height: 200%;
  fill: rgb(0 0 0 / 40%);
  stroke: rgb(0 0 0 / 50%);
  mix-blend-mode: overlay;
  transform: skewY(-18deg);
}

.dark .default-background__pattern {
  fill: rgb(255 255 255 / 2.4%);
  stroke: rgb(255 255 255 / 5.1%);
}

.default-background__pattern-blocks {
  overflow: visible;
}

@media (max-width: 768px) {
  .default-background {
    transform: scale(1.25);
  }

  .default-background__spotlight {
    left: 50%;
    width: 60rem;
    height: 22rem;
    margin-left: -30rem;
  }
}

.background-loading {
  position: absolute;
  inset: 0;
  background-color: rgb(15 23 42);
}

:root:not(.dark) .background-loading {
  background:
    radial-gradient(circle at 50% 0%, rgb(16 185 129 / 0.08), transparent 36%),
    linear-gradient(180deg, rgb(203 213 225), rgb(148 163 184));
}

.background-media {
  position: absolute;
  inset: 0;
  transition: opacity 0.8s ease;
}

.background-image {
  width: 100%;
  height: 100%;
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
}

.background-overlay {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.8s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
