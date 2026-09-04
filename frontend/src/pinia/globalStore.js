import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useGlobalStore = defineStore('global', () => {
  const theme = ref(localStorage.getItem('theme') || 'dark')
  const sidebarCollapsed = ref(localStorage.getItem('sidebarCollapsed') === 'true')
  const alarmMessages = ref([])
  // [修复] 移动端响应式：isMobile 标记 + 抽屉开关（移动端侧边栏为抽屉模式）
  const isMobile = ref(typeof window !== 'undefined' && window.innerWidth <= 768)
  const mobileSidebarOpen = ref(false)

  function toggleTheme() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
    localStorage.setItem('theme', theme.value)
  }

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
    localStorage.setItem('sidebarCollapsed', sidebarCollapsed.value)
  }

  // [修复] 窗口尺寸变化时同步 isMobile，切到移动端时强制收起抽屉
  function setMobile(v) {
    if (v !== isMobile.value) {
      isMobile.value = v
      if (v) mobileSidebarOpen.value = false
    }
  }

  function toggleMobileSidebar() {
    mobileSidebarOpen.value = !mobileSidebarOpen.value
  }

  function closeMobileSidebar() {
    mobileSidebarOpen.value = false
  }

  function addAlarm(msg) {
    alarmMessages.value.unshift({ id: Date.now(), ...msg, time: new Date().toLocaleString() })
    if (alarmMessages.value.length > 100) alarmMessages.value.pop()
  }

  function clearAlarms() {
    alarmMessages.value = []
  }

  return { theme, sidebarCollapsed, alarmMessages, isMobile, mobileSidebarOpen, toggleTheme, toggleSidebar, setMobile, toggleMobileSidebar, closeMobileSidebar, addAlarm, clearAlarms }
})