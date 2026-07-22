<template>
  <div class="countdown" :class="{ urgent: hours === 0 && minutes < 5 }">
    <span v-if="expired">已结束</span>
    <template v-else>
      <span class="num">{{ pad(hours) }}</span><span class="sep">:</span>
      <span class="num">{{ pad(minutes) }}</span><span class="sep">:</span>
      <span class="num">{{ pad(seconds) }}</span>
    </template>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'

const props = defineProps({
  endTime: { type: [String, Number, Date], required: true },
  startTime: { type: [String, Number, Date], default: null }
})

const now = ref(Date.now())
let timer = null

const diff = computed(() => {
  const start = props.startTime ? new Date(props.startTime).getTime() : 0
  const end = new Date(props.endTime).getTime()
  if (now.value < start) return start - now.value
  return end - now.value
})

const hours = computed(() => Math.floor(diff.value / 3600000))
const minutes = computed(() => Math.floor((diff.value % 3600000) / 60000))
const seconds = computed(() => Math.floor((diff.value % 60000) / 1000))
const expired = computed(() => diff.value <= 0)

function pad(n) { return String(n).padStart(2, '0') }

onMounted(() => { timer = setInterval(() => { now.value = Date.now() }, 1000) })
onUnmounted(() => { clearInterval(timer) })
</script>

<style scoped>
.countdown { display: inline-flex; align-items: center; gap: 2px; font-family: 'Courier New', monospace }
.num { background: rgba(0,0,0,.4); padding: 2px 6px; border-radius: 4px; font-size: 16px; font-weight: bold; color: #e94560; min-width: 32px; text-align: center }
.sep { color: #e94560; font-weight: bold }
.urgent .num { animation: pulse .5s infinite }
@keyframes pulse { 0%,100% { opacity: 1 } 50% { opacity: .5 } }
</style>