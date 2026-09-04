<template>
  <CardGlass title="实时PV/UV">
    <!-- [增强] 当日累计 PV/UV 指标（真实埋点数据） -->
    <div class="pvuv-badges">
      <div class="badge">
        <span class="badge-value pv">{{ pv }}</span>
        <span class="badge-label">当日PV</span>
      </div>
      <div class="badge">
        <span class="badge-value uv">{{ uv }}</span>
        <span class="badge-label">当日UV</span>
      </div>
      <div class="badge">
        <span class="badge-value qps">{{ qps }}</span>
        <span class="badge-label">当前QPS</span>
      </div>
    </div>
    <div ref="chartRef" style="width:100%;height:260px"></div>
  </CardGlass>
</template>

<script setup>
import { ref, onMounted, onUnmounted, getCurrentInstance } from 'vue'
import CardGlass from '@/components/Common/CardGlass.vue'
import { getPVUV } from '@/api/monitorApi'

const { proxy } = getCurrentInstance()
const chartRef = ref(null)
let chart = null
let timer = null

const pv = ref(0)
const uv = ref(0)
const qps = ref(0)

// [增强] 拉取真实 PV/UV 数据并刷新图表（最近60秒每秒请求序列）
async function loadData() {
  try {
    const data = await getPVUV()
    if (!data) return
    pv.value = data.pv || 0
    uv.value = data.uv || 0
    qps.value = data.qps || 0
    const series = data.series || []
    const xData = series.map(p => p.time || '')
    const pvData = series.map(p => p.pv || 0)
    chart.setOption({
      xAxis: { data: xData },
      series: [{ data: pvData }]
    })
  } catch (e) { /* 接口异常时保留上一次数据 */ }
}

onMounted(() => {
  chart = proxy.$echarts.init(chartRef.value)
  chart.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['请求量(次/秒)'], textStyle: { color: '#999' } },
    grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
    xAxis: { type: 'category', boundaryGap: false, data: [], axisLine: { lineStyle: { color: '#333' } } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,.06)' } } },
    series: [
      { name: '请求量(次/秒)', type: 'line', smooth: true, data: [], showSymbol: false,
        lineStyle: { color: '#e94560' }, areaStyle: { color: 'rgba(233,69,96,.1)' } }
    ]
  })
  loadData()
  // [增强] 3 秒刷新，与后端统计引擎秒级采样对齐
  timer = setInterval(loadData, 3000)
})

onUnmounted(() => { clearInterval(timer); chart?.dispose() })
</script>

<style scoped>
.pvuv-badges { display: flex; gap: 24px; margin-bottom: 12px }
.badge { display: flex; flex-direction: column; align-items: center }
.badge-value { font-size: 24px; font-weight: bold }
.badge-value.pv { color: #e94560 }
.badge-value.uv { color: #409eff }
.badge-value.qps { color: #67c23a }
.badge-label { font-size: 12px; color: #999; margin-top: 2px }
</style>
