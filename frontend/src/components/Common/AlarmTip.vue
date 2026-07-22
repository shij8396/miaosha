<template>
  <el-popover placement="bottom-end" :width="360" trigger="click">
    <template #reference>
      <el-badge :value="globalStore.alarmMessages.length" :hidden="!globalStore.alarmMessages.length">
        <el-icon class="alarm-icon" :size="18"><Bell /></el-icon>
      </el-badge>
    </template>
    <div class="alarm-list">
      <div class="alarm-header">
        <span>告警消息</span>
        <el-button link type="danger" size="small" @click="globalStore.clearAlarms()">清空</el-button>
      </div>
      <el-empty v-if="!globalStore.alarmMessages.length" description="暂无告警" :image-size="60" />
      <div v-for="msg in globalStore.alarmMessages" :key="msg.id" class="alarm-item">
        <div class="alarm-level" :class="msg.level || 'warning'"></div>
        <div class="alarm-content">
          <div class="alarm-msg">{{ msg.msg }}</div>
          <div class="alarm-time">{{ msg.time }}</div>
        </div>
      </div>
    </div>
  </el-popover>
</template>

<script setup>
import { useGlobalStore } from '@/pinia/globalStore'
const globalStore = useGlobalStore()
</script>

<style scoped>
.alarm-icon { cursor: pointer; color: #999 }
.alarm-icon:hover { color: #e94560 }
.alarm-list { max-height: 400px; overflow-y: auto }
.alarm-header { display: flex; justify-content: space-between; align-items: center; padding-bottom: 8px; border-bottom: 1px solid rgba(255,255,255,.06); margin-bottom: 8px }
.alarm-item { display: flex; gap: 8px; padding: 8px 0; border-bottom: 1px solid rgba(255,255,255,.03) }
.alarm-level { width: 4px; border-radius: 2px; flex-shrink: 0 }
.alarm-level.error { background: #e94560 }
.alarm-level.warning { background: #e6a23c }
.alarm-level.info { background: #409eff }
.alarm-content { flex: 1 }
.alarm-msg { font-size: 13px; color: var(--text-primary) }
.alarm-time { font-size: 11px; color: #666; margin-top: 2px }
</style>