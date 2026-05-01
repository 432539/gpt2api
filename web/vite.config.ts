import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import path from 'node:path'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiBase = env.VITE_API_BASE || 'http://localhost:18080'
  return {
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src'),
      },
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      proxy: {
        // 开发期统一经本地代理,避免 CORS;生产由 nginx / ingress 承担
        '/api': { target: apiBase, changeOrigin: true },
        '/v1': { target: apiBase, changeOrigin: true },
        '/healthz': { target: apiBase, changeOrigin: true },
      },
    },
    plugins: [
      vue(),
      AutoImport({
        imports: ['vue', 'vue-router', 'pinia', '@vueuse/core'],
        resolvers: [ElementPlusResolver()],
        dts: 'src/auto-imports.d.ts',
      }),
      Components({
        resolvers: [ElementPlusResolver()],
        dts: 'src/components.d.ts',
      }),
    ],
    build: {
      outDir: 'dist',
      sourcemap: false,
      chunkSizeWarningLimit: 700,
      rollupOptions: {
        output: {
          /**
           * 手工拆包,避免把 Element Plus 全量塞进 index.js。
           * - element-plus:UI 组件库 + 图标,业务里几乎每页都用,拆出独立 chunk 以便浏览器长期缓存。
           * - vue-core:vue / vue-router / pinia / @vueuse,运行时核心。
           * - vendor:其它 node_modules(axios、dayjs 等)。
           */
          manualChunks(id) {
            if (!id.includes('node_modules')) return
            if (id.includes('element-plus') || id.includes('@element-plus')) return 'element-plus'
            if (
              id.includes('/vue/') ||
              id.includes('/@vue/') ||
              id.includes('/vue-router/') ||
              id.includes('/pinia') ||
              id.includes('/@vueuse/')
            ) return 'vue-core'
            return 'vendor'
          },
        },
      },
    },
  }
})
