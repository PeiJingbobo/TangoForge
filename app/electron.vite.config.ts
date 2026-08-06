import { fileURLToPath, URL } from 'node:url'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig, externalizeDepsPlugin } from 'electron-vite'

// electron-vite 统一构建 main / preload / renderer 三端（docs/TECHNICAL.md §4.1）。
// 输出目录约定：out/main、out/preload、out/renderer。
// 注意：配置文件必须命名为 electron.vite.config.ts（electron-vite 禁止 vite.config.ts 命名）。
export default defineConfig({
  main: {
    plugins: [externalizeDepsPlugin()],
    build: {
      outDir: 'out/main',
      rollupOptions: {
        input: { index: fileURLToPath(new URL('./electron/main.ts', import.meta.url)) },
      },
    },
  },
  preload: {
    plugins: [externalizeDepsPlugin()],
    build: {
      outDir: 'out/preload',
      rollupOptions: {
        input: { index: fileURLToPath(new URL('./electron/preload.ts', import.meta.url)) },
      },
    },
  },
  renderer: {
    root: fileURLToPath(new URL('.', import.meta.url)),
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    build: {
      outDir: 'out/renderer',
      rollupOptions: {
        input: { index: fileURLToPath(new URL('./index.html', import.meta.url)) },
      },
    },
  },
})
