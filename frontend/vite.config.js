import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { resolve } from 'path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src')
    }
  },
  server: {
    // [修复] 开发服务器端口从 3000 改为 5173，避免与 Grafana 端口冲突
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
      // [修复] 图片静态资源代理：后端通过 router.Static("/uploads") 提供服务，前端需代理到 8080
      '/uploads': 'http://localhost:8080',
      // [修复] WebSocket 代理：支持 ws 协议转发，解决开发环境 /ws 连接失败
      '/ws': {
        target: 'http://localhost:8080',
        ws: true
      }
    }
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets',
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/element-plus')) return 'element-plus'
          if (id.includes('node_modules/echarts')) return 'echarts'
        }
      }
    }
  }
})