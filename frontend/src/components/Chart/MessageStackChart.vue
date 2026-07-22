<template>
  <CardGlass title="MQ消息堆积">
    <div ref="chartRef" style="width:100%;height:300px"></div>
  </CardGlass>
</template>

<script setup>
import { ref, onMounted, onUnmounted, getCurrentInstance } from 'vue'
import CardGlass from '@/components/Common/CardGlass.vue'

const { proxy } = getCurrentInstance()
const chartRef = ref(null)
let chart = null
let timer = null

onMounted(() => {
  chart = proxy.$echarts.init(chartRef.value)
  const option = {
    tooltip: { trigger: 'axis' },
    legend: { data: ['订单队列', '延迟队列', '死信队列'], textStyle: { color: '#999' } },
    grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
    xAxis: { type: 'category', boundaryGap: false, data: [], axisLine: { lineStyle: { color: '#333' } } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,.06)' } } },
    series: [
      { name: '订单队列', type: 'line', data: [], smooth: true, lineStyle: { color: '#67c23a' } },
      { name: '延迟队列', type: 'line', data: [], smooth: true, lineStyle: { color: '#e6a23c' } },
      { name: '死信队列', type: 'line', data: [], smooth: true, lineStyle: { color: '#e94560' } }
    ]
  }
  chart.setOption(option)

  timer = setInterval(() => {
    const now = new Date().toLocaleTimeString()
    option.xAxis.data.push(now)
    if (option.xAxis.data.length > 20) option.xAxis.data.shift()
    option.series.forEach(s => {
      s.data.push(Math.floor(Math.random() * 50))
      if (s.data.length > 20) s.data.shift()
    })
    chart.setOption(option)
  }, 3000)
})

onUnmounted(() => { clearInterval(timer); chart?.dispose() })
</script>