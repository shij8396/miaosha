<template>
  <CardGlass title="热销商品排行">
    <div ref="chartRef" style="width:100%;height:300px"></div>
  </CardGlass>
</template>

<script setup>
import { ref, onMounted, onUnmounted, getCurrentInstance } from 'vue'
import CardGlass from '@/components/Common/CardGlass.vue'

const { proxy } = getCurrentInstance()
const chartRef = ref(null)
let chart = null

onMounted(() => {
  chart = proxy.$echarts.init(chartRef.value)
  chart.setOption({
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
    xAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,.06)' } } },
    yAxis: { type: 'category', data: ['iPhone 16', '华为Mate70', '小米15', 'iPad Pro', 'AirPods'], axisLabel: { color: '#999' } },
    series: [{
      type: 'bar', data: [320, 280, 200, 150, 120],
      itemStyle: { color: new proxy.$echarts.graphic.LinearGradient(0, 0, 1, 0, [
        { offset: 0, color: '#e94560' }, { offset: 1, color: '#ff6b6b' }
      ])}
    }]
  })
})

onUnmounted(() => { chart?.dispose() })
</script>