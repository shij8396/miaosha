<template>
  <CardGlass title="实时PV/UV">
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
    legend: { data: ['PV', 'UV'], textStyle: { color: '#999' } },
    grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
    xAxis: { type: 'category', boundaryGap: false, data: [], axisLine: { lineStyle: { color: '#333' } } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,.06)' } } },
    series: [
      { name: 'PV', type: 'line', smooth: true, data: [], lineStyle: { color: '#e94560' }, areaStyle: { color: 'rgba(233,69,96,.1)' } },
      { name: 'UV', type: 'line', smooth: true, data: [], lineStyle: { color: '#409eff' }, areaStyle: { color: 'rgba(64,158,255,.1)' } }
    ]
  }
  chart.setOption(option)

  timer = setInterval(() => {
    const now = new Date().toLocaleTimeString()
    const pv = Math.floor(Math.random() * 500 + 200)
    const uv = Math.floor(Math.random() * 200 + 50)
    const xData = option.xAxis.data
    xData.push(now)
    if (xData.length > 20) xData.shift()
    option.series[0].data.push(pv)
    option.series[1].data.push(uv)
    if (option.series[0].data.length > 20) { option.series[0].data.shift(); option.series[1].data.shift() }
    chart.setOption(option)
  }, 3000)
})

onUnmounted(() => { clearInterval(timer); chart?.dispose() })
</script>