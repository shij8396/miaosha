<template>
  <div class="stock-num">
    <span class="label">剩余</span>
    <span class="num" :class="colorClass">{{ displayNum }}</span>
    <span class="label">/ {{ total }}</span>
  </div>
</template>

<script setup>
import { ref, watch, computed } from 'vue'

const props = defineProps({
  remain: { type: Number, default: 0 },
  total: { type: Number, default: 0 }
})

const displayNum = ref(props.remain)
let animTimer = null

watch(() => props.remain, (newVal, oldVal) => {
  const duration = 500
  const steps = 20
  const stepVal = (newVal - oldVal) / steps
  let i = 0
  clearInterval(animTimer)
  animTimer = setInterval(() => {
    i++
    displayNum.value = Math.round(oldVal + stepVal * i)
    if (i >= steps) { displayNum.value = newVal; clearInterval(animTimer) }
  }, duration / steps)
})

const colorClass = computed(() => {
  const pct = props.remain / props.total
  if (pct > 0.5) return 'color-safe'
  if (pct > 0.2) return 'color-warn'
  return 'color-danger'
})
</script>

<style scoped>
.stock-num { display: inline-flex; align-items: baseline; gap: 4px }
.label { font-size: 12px; color: #999 }
.num { font-size: 24px; font-weight: bold; transition: color .3s }
.num.color-safe { color: #67c23a }
.num.color-warn { color: #e6a23c }
.num.color-danger { color: #e94560 }
</style>