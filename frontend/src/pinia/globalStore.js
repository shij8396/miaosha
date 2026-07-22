import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useGlobalStore = defineStore('global', () => {
  const theme = ref(localStorage.getItem('theme') || 'dark')
  const sidebarCollapsed = ref(localStorage.getItem('sidebarCollapsed') === 'true')
  const alarmMessages = ref([])

  function toggleTheme() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
    localStorage.setItem('theme', theme.value)
  }

  function toggleSidebar() {
    sidebarCollapsed.value = !sidebarCollapsed.value
    localStorage.setItem('sidebarCollapsed', sidebarCollapsed.value)
  }

  function addAlarm(msg) {
    alarmMessages.value.unshift({ id: Date.now(), ...msg, time: new Date().toLocaleString() })
    if (alarmMessages.value.length > 100) alarmMessages.value.pop()
  }

  function clearAlarms() {
    alarmMessages.value = []
  }

  return { theme, sidebarCollapsed, alarmMessages, toggleTheme, toggleSidebar, addAlarm, clearAlarms }
})