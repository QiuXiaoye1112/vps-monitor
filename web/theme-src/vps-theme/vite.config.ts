import type { Plugin } from 'vite'
import { execSync } from 'node:child_process'
import { existsSync, readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'
import process from 'node:process'
import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

import vueDevTools from 'vite-plugin-vue-devtools'

const require = createRequire(import.meta.url)
const fs = require('node:fs')
const archiver = require('archiver')

interface ThemeManifest {
  preview?: unknown
  version?: unknown
}

const themeJsonPath = resolve(__dirname, 'vps-monitor-theme.json')
const devApiTarget = process.env.VITE_API_TARGET || 'http://127.0.0.1:25774'

function readThemeManifest(): ThemeManifest {
  if (!existsSync(themeJsonPath))
    throw new Error('vps-monitor-theme.json not found')

  return JSON.parse(readFileSync(themeJsonPath, 'utf-8')) as ThemeManifest
}

function getThemeVersion(): string {
  const version = readThemeManifest().version

  if (typeof version !== 'string' || !version.trim())
    throw new TypeError('vps-monitor-theme.json does not contain a top-level string version field')

  return version.trim()
}

function getCommitHash(): string {
  try {
    return execSync('git rev-parse --short HEAD', { encoding: 'utf-8' }).trim()
  }
  catch {
    return 'unknown'
  }
}

/**
 * Vite 插件：构建后打包 VPS Monitor 主题 Zip
 * theme.zip
 * ├── vps-monitor-theme.json
 * ├── preview.png
 * └── dist/
 */
function vpsMonitorThemeZip(): Plugin {
  return {
    name: 'vps-monitor-theme-zip',
    apply: 'build',
    closeBundle: async () => {
      const commitHash = getCommitHash()
      const zipFileName = `vps-monitor-theme-Glassmorphism-build-${commitHash}.zip`
      const distDir = resolve(__dirname, 'dist')
      const previewPath = resolve(__dirname, 'docs/preview.png')
      const outputPath = resolve(__dirname, zipFileName)
      const themeManifest = readThemeManifest()
      const manifestPreviewName = typeof themeManifest.preview === 'string' && themeManifest.preview.trim()
        ? themeManifest.preview.trim()
        : 'preview.png'

      if (!existsSync(distDir)) {
        console.log('[vps-monitor-theme-zip] dist directory not found, skipping zip creation')
        return
      }

      const output = fs.createWriteStream(outputPath)
      const archive = archiver('zip', { zlib: { level: 9 } })

      return new Promise((resolve, reject) => {
        output.on('close', () => {
          const sizeMB = (archive.pointer() / 1024 / 1024).toFixed(2)
          console.log(`[vps-monitor-theme-zip] Created ${zipFileName} (${sizeMB} MB)`)
          resolve(undefined)
        })

        archive.on('error', (err: Error) => {
          console.error('[vps-monitor-theme-zip] Error:', err)
          reject(err)
        })

        archive.pipe(output)

        archive.file(themeJsonPath, { name: 'vps-monitor-theme.json' })

        if (existsSync(previewPath)) {
          archive.file(previewPath, { name: 'preview.png' })
          if (manifestPreviewName !== 'preview.png') {
            archive.file(previewPath, { name: manifestPreviewName })
          }
        }

        archive.directory(distDir, 'dist')

        archive.finalize()
      })
    },
  }
}

export default defineConfig({
  define: {
    __BUILD_VERSION__: JSON.stringify(getThemeVersion()),
    __BUILD_GIT_HASH__: JSON.stringify(getCommitHash()),
  },
  plugins: [
    vue(),
    process.env.VPS_EMBED_BUILD === '1' ? null : vueDevTools(),
    tailwindcss(),
    process.env.VPS_EMBED_BUILD === '1' ? null : vpsMonitorThemeZip(),
  ].filter(Boolean) as Plugin[],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    host: '0.0.0.0',
    proxy: {
      // Management routes are served by the monitor backend. Without these
      // explicit proxies Vite falls back to the public theme index, so the
      // login page and admin application appear as an empty public page.
      '/admin': {
        target: devApiTarget,
        changeOrigin: true,
        headers: { Origin: devApiTarget },
      },
      '/admin-app': {
        target: devApiTarget,
        changeOrigin: true,
        headers: { Origin: devApiTarget },
      },
      '/manage': {
        target: devApiTarget,
        changeOrigin: true,
        headers: { Origin: devApiTarget },
      },
      '/terminal': {
        target: devApiTarget,
        changeOrigin: true,
        headers: { Origin: devApiTarget },
        rewriteWsOrigin: true,
        ws: true,
      },
      '/assets': {
        target: devApiTarget,
        changeOrigin: true,
      },
      '/vps-admin-clean.js': {
        target: devApiTarget,
        changeOrigin: true,
      },
      '/favicon.ico': {
        target: devApiTarget,
        changeOrigin: true,
      },
      '/api': {
        target: devApiTarget,
        changeOrigin: true,
        headers: { Origin: devApiTarget },
        rewriteWsOrigin: true,
        ws: true,
      },
      '/themes': {
        target: devApiTarget,
        changeOrigin: true,
        headers: { Origin: devApiTarget },
      },
    },
  },
  build: {
    target: ['es2018', 'safari15.4'],
    cssTarget: 'safari15.4',
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks: {
          'vue-vendor': ['vue', 'vue-router', 'pinia'],
          'echarts': ['echarts', 'vue-echarts'],
          'globe': ['globe.gl', 'three'],
          'reka-ui': ['reka-ui'],
          'vueuse': ['@vueuse/core'],
          'v3-services': [
            './src/services/history.service.ts',
            './src/services/request.service.ts',
            './src/services/cache.service.ts',
            './src/utils/osImageHelper.ts',
            './src/composables/useNodePingDisplay.ts',
          ],
        },
      },
    },
  },
})
