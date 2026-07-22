<template>
  <div class="app-shell" :data-theme="globalStore.theme">
    <!-- 动态背景粒子 -->
    <div class="bg-particles">
      <div v-for="i in 20" :key="i" class="particle" :style="particleStyle(i)"></div>
    </div>
    <router-view />
  </div>
</template>

<script setup>
import { useGlobalStore } from '@/pinia/globalStore'
const globalStore = useGlobalStore()

function particleStyle(i) {
  const size = Math.random() * 3 + 1
  return {
    width: size + 'px',
    height: size + 'px',
    left: Math.random() * 100 + '%',
    top: Math.random() * 100 + '%',
    animationDelay: Math.random() * 5 + 's',
    animationDuration: (Math.random() * 10 + 10) + 's'
  }
}
</script>

<style>
/* [修复] 使用 height:100% 继承 #app 高度，保持布局链完整 */
.app-shell {
  width: 100%;
  height: 100%;
  background: var(--bg-primary);
  position: relative;
  overflow: hidden;
}
.bg-particles {
  position: fixed;
  inset: 0;
  pointer-events: none;
  z-index: 0;
}
.particle {
  position: absolute;
  background: rgba(233,69,96,.15);
  border-radius: 50%;
  animation: floatUp linear infinite;
}
@keyframes floatUp {
  0% { transform: translateY(100vh) scale(0); opacity: 0; }
  10% { opacity: 1; }
  90% { opacity: 1; }
  100% { transform: translateY(-10vh) scale(1); opacity: 0; }
}
</style>