<template>
  <!--
    [修复] 布局高度链：
    #app(100%) → .app-shell(100%) → .layout-container(100%) → .main-area(flex:1) → .main-content(flex:1, overflow-y:auto)
    每一层都使用百分比高度继承，最终滚动由 main-content 处理
    底部状态栏固定在 main-area 底部，始终可见
  -->
  <div class="layout-container" :class="{ 'mobile-open': globalStore.isMobile && globalStore.mobileSidebarOpen }">
    <!-- [修复] 移动端遮罩：抽屉打开时点击遮罩关闭侧边栏 -->
    <div class="mobile-mask" v-show="globalStore.isMobile && globalStore.mobileSidebarOpen" @click="globalStore.closeMobileSidebar()"></div>
    <SideBar />
    <div class="main-area" :class="{ collapsed: !globalStore.isMobile && globalStore.sidebarCollapsed }">
      <HeaderNav />
      <div class="main-content">
        <!-- [修复] 任务3: 全局加载进度条，当有 API 请求时显示 -->
        <div class="global-loading-bar" v-show="globalLoading">
          <el-progress :percentage="100" :show-text="false" :stroke-width="3" color="#e94560" :indeterminate="true" />
        </div>
        <router-view />
      </div>
      <!-- [修复] 底部状态栏：始终固定可见，显示系统运行信息 -->
      <div class="status-bar">
        <div class="status-left">
          <span class="status-dot online"></span>
          <span>系统运行中</span>
          <span class="status-sep">|</span>
          <span>后端: {{ backendStatus }}</span>
          <span class="status-sep">|</span>
          <span>用户: {{ userStore.username }}</span>
        </div>
        <div class="status-right">
          <span>{{ currentTime }}</span>
          <span class="status-sep">|</span>
          <span>v1.0.0</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import SideBar from './SideBar.vue'
import HeaderNav from './HeaderNav.vue'
import { useGlobalStore } from '@/pinia/globalStore'
import { useUserStore } from '@/pinia/userStore'
import { setLoadingChangeCallback } from '@/api/request'

const globalStore = useGlobalStore()
const userStore = useUserStore()

const currentTime = ref(new Date().toLocaleString())
const backendStatus = ref('在线')
/* [修复] 任务3: 全局 loading 状态 */
const globalLoading = ref(false)

let timer = null
/* [修复] 移动端：resize 监听，跨过 768px 断点时同步 isMobile */
function onResize() {
  globalStore.setMobile(window.innerWidth <= 768)
}
onMounted(() => {
  timer = setInterval(() => {
    currentTime.value = new Date().toLocaleString()
  }, 1000)
  /* [修复] 任务3: 注册全局 loading 回调，当 API 请求时显示进度条 */
  setLoadingChangeCallback((loading) => {
    globalLoading.value = loading
  })
  onResize()
  window.addEventListener('resize', onResize)
})
onUnmounted(() => {
  clearInterval(timer)
  window.removeEventListener('resize', onResize)
})
</script>

<style scoped>
.layout-container {
  display: flex;
  height: 100%;
  overflow: hidden;
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  transition: margin-left .3s;
  margin-left: 220px;
  min-width: 0;
  height: 100%;
}

.main-area.collapsed {
  margin-left: 64px;
}

.main-content {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  position: relative;
  z-index: 1;
  min-height: 0;
}

.global-loading-bar {
  position: sticky;
  top: 0;
  z-index: 999;
  width: 100%;
}

.status-bar {
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  background: rgba(0, 0, 0, .4);
  border-top: 1px solid rgba(255, 255, 255, .06);
  font-size: 12px;
  color: #888;
  flex-shrink: 0;
  z-index: 10;
}

.status-left, .status-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-sep {
  color: rgba(255, 255, 255, .15);
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #888;
  flex-shrink: 0;
}

.status-dot.online {
  background: #67c23a;
  box-shadow: 0 0 4px rgba(103, 194, 58, .5);
}

.mobile-mask {
  display: none;
}
</style>

<style>
/* 全局响应式样式 —— 必须非 scoped 才能跨组件生效 */
@media (max-width: 768px) {
  .layout-container .main-area,
  .layout-container .main-area.collapsed {
    margin-left: 0 !important;
    width: 100% !important;
  }

  .layout-container .sidebar {
    position: fixed !important;
    top: 0 !important;
    left: 0 !important;
    bottom: 0 !important;
    width: 220px !important;
    transform: translateX(-100%) !important;
    transition: transform .3s ease !important;
    z-index: 1000 !important;
    flex-shrink: 0 !important;
    box-shadow: none !important;
  }

  .layout-container.mobile-open .sidebar {
    transform: translateX(0) !important;
    box-shadow: 2px 0 12px rgba(0, 0, 0, .45) !important;
  }

  .layout-container .mobile-mask {
    display: block !important;
    position: fixed !important;
    inset: 0 !important;
    background: rgba(0, 0, 0, .5) !important;
    z-index: 999 !important;
  }

  .status-right {
    display: none !important;
  }
}
</style>