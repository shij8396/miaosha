<template>
  <div class="page-container">
    <h2 class="page-title">服务监控面板</h2>
    <div class="monitor-grid">
      <CardGlass title="QPS趋势">
        <div ref="qpsChart" style="width:100%;height:300px"></div>
      </CardGlass>
      <CardGlass title="限流统计">
        <div class="metric-cards">
          <div class="metric-card">
            <div class="metric-value" style="color:#e94560">{{ metrics.rejectCount }}</div>
            <div class="metric-label">被拒绝请求</div>
          </div>
          <div class="metric-card">
            <div class="metric-value" style="color:#e6a23c">{{ metrics.passCount }}</div>
            <div class="metric-label">通过请求</div>
          </div>
          <div class="metric-card">
            <div class="metric-value" style="color:#67c23a">{{ metrics.avgRt }}ms</div>
            <div class="metric-label">平均响应时间</div>
          </div>
        </div>
      </CardGlass>
      <CardGlass title="中间件健康">
        <el-table :data="mwList" style="width:100%">
          <el-table-column prop="name" label="服务" />
          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'up' ? 'success' : 'danger'" size="small" effect="dark">{{ row.status === 'up' ? '正常' : '异常' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="addr" label="地址" />
          <el-table-column prop="uptime" label="运行时间" width="140" />
        </el-table>
      </CardGlass>
      <CardGlass title="系统资源">
        <div class="metric-cards">
          <div class="metric-card">
            <div class="metric-value">{{ metrics.cpuUsage }}%</div>
            <div class="metric-label">CPU使用率</div>
          </div>
          <div class="metric-card">
            <div class="metric-value">{{ metrics.memUsage }}%</div>
            <div class="metric-label">内存使用率</div>
          </div>
          <div class="metric-card">
            <div class="metric-value">{{ metrics.activeConns }}</div>
            <div class="metric-label">活跃连接数</div>
          </div>
        </div>
      </CardGlass>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, getCurrentInstance } from 'vue'
import { getMetrics, getQPS, getMiddlewareStatus } from '@/api/monitorApi'
import CardGlass from '@/components/Common/CardGlass.vue'

const { proxy } = getCurrentInstance()
const qpsChart = ref(null)
let chart = null
let dataTimer = null

const metrics = ref({ reject_count: 0, pass_count: 0, avg_rt: 0, cpu_usage: 0, mem_usage: 0, active_conns: 0 })
const mwList = ref([])

// 加载监控指标
async function loadMetrics() {
  try {
    const data = await getMetrics()
    if (data) {
      metrics.value = {
        rejectCount: data.reject_count || 0,
        passCount: data.pass_count || 0,
        avgRt: data.avg_rt || 0,
        cpuUsage: data.cpu_usage || 0,
        memUsage: data.mem_usage || 0,
        activeConns: data.active_conns || 0
      }
    }
  } catch (e) { /* 使用默认值 */ }
}

// 加载 QPS 历史数据
async function loadQPSHistory() {
  try {
    const data = await getQPS()
    if (data && Array.isArray(data) && chart) {
      const opt = chart.getOption()
      opt.xAxis[0].data = data.map(d => d.time || '')
      opt.series[0].data = data.map(d => d.value || 0)
      chart.setOption(opt)
    }
  } catch (e) { /* 使用随机数据作为后备 */ }
}

// 加载中间件状态
async function loadMWStatus() {
  try {
    const data = await getMiddlewareStatus()
    if (data && Array.isArray(data)) {
      mwList.value = data.map(m => ({
        name: m.name || '',
        status: m.status || 'up',
        addr: m.address || m.addr || '',
        uptime: m.uptime || ''
      }))
    }
  } catch (e) { /* 使用默认值 */ }
}

onMounted(() => {
  chart = proxy.$echarts.init(qpsChart.value)
  chart.setOption({
    tooltip: { trigger: 'axis' },
    grid: { left: '3%', right: '4%', bottom: '3%', containLabel: true },
    xAxis: { type: 'category', boundaryGap: false, data: [], axisLine: { lineStyle: { color: '#333' } } },
    yAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,.06)' } } },
    series: [{
      type: 'line', smooth: true, data: [], lineStyle: { color: '#e94560' },
      areaStyle: { color: new proxy.$echarts.graphic.LinearGradient(0, 0, 0, 1, [
        { offset: 0, color: 'rgba(233,69,96,.3)' }, { offset: 1, color: 'rgba(233,69,96,.02)' }
      ])}
    }]
  })

  // 初始化加载
  loadMetrics()
  loadQPSHistory()
  loadMWStatus()

  // 每 5 秒刷新数据
  dataTimer = setInterval(() => {
    loadMetrics()
    loadQPSHistory()
    loadMWStatus()
  }, 5000)
})

onUnmounted(() => {
  if (dataTimer) clearInterval(dataTimer)
  chart?.dispose()
})
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
.monitor-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px }
.metric-cards { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px }
.metric-card { text-align: center; padding: 16px; background: rgba(255,255,255,.02); border-radius: 8px }
.metric-value { font-size: 28px; font-weight: bold }
.metric-label { font-size: 13px; color: #999; margin-top: 4px }
:deep(.el-table) {
  --el-table-bg-color: transparent; --el-table-tr-bg-color: transparent;
  --el-table-header-bg-color: rgba(255,255,255,.04); --el-table-border-color: rgba(255,255,255,.06);
  --el-table-text-color: var(--text-primary); --el-table-header-text-color: var(--text-secondary);
}
</style>