<template>
  <div class="big-screen">
    <div class="screen-header">
      <h1 class="gradient-text neon-text">秒杀实时数据大屏</h1>
      <div class="header-time">{{ currentTime }}</div>
      <!-- [修复] 任务6: 右上角操作按钮组 -->
      <div class="header-actions">
        <!-- [修复] 任务6: 导出营销报表按钮 -->
        <el-button size="small" @click="exportReport" circle>
          <el-icon><Download /></el-icon>
        </el-button>
        <!-- [修复] 任务6: 全屏切换按钮 -->
        <el-button size="small" @click="toggleFullscreen" circle>
          <el-icon><FullScreen v-if="!isFullscreen" /><Aim v-else /></el-icon>
        </el-button>
      </div>
    </div>
    <div class="screen-grid">
      <div class="col-left">
        <RealTimePV />
        <MessageStackChart />
      </div>
      <div class="col-center">
        <CardGlass title="实时抢购概览">
          <div class="stats-row">
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#e94560">{{ stats.totalOrders }}</div>
              <div class="stat-label">总订单数</div>
            </div>
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#409eff">{{ stats.successRate }}%</div>
              <div class="stat-label">成功率</div>
            </div>
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#67c23a">{{ stats.qps }}</div>
              <div class="stat-label">当前QPS</div>
            </div>
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#e6a23c">{{ stats.mqBacklog }}</div>
              <div class="stat-label">MQ堆积</div>
            </div>
          </div>
          <!-- [增强] 大屏指标补全：转化率/PV/UV/秒杀请求漏斗 -->
          <div class="stats-row stats-row-second">
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#b37feb">{{ stats.conversionRate }}%</div>
              <div class="stat-label">UV转化率</div>
            </div>
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#e94560">{{ stats.pv }}</div>
              <div class="stat-label">当日PV</div>
            </div>
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#409eff">{{ stats.uv }}</div>
              <div class="stat-label">当日UV</div>
            </div>
            <div class="stat-item count-up">
              <div class="stat-value" style="color:#67c23a">{{ stats.seckillRequests }}</div>
              <div class="stat-label">秒杀请求</div>
            </div>
          </div>
        </CardGlass>
        <HotRankChart />
      </div>
      <div class="col-right">
        <CardGlass title="告警实时滚动">
          <div class="alarm-scroll">
            <div v-for="(alarm, i) in alarms" :key="i" class="alarm-row">
              <span class="alarm-dot" :class="alarm.level"></span>
              <span class="alarm-msg">{{ alarm.msg }}</span>
              <span class="alarm-time">{{ alarm.time }}</span>
            </div>
          </div>
        </CardGlass>
        <CardGlass title="中间件状态">
          <div class="mw-status">
            <div v-for="mw in middleware" :key="mw.name" class="mw-row">
              <span class="mw-name">{{ mw.name }}</span>
              <el-tag :type="mw.status === 'up' ? 'success' : 'danger'" size="small" effect="dark">
                {{ mw.status === 'up' ? '正常' : '异常' }}
              </el-tag>
            </div>
          </div>
        </CardGlass>
        <!-- [增强] 库存实时监控面板（Redis 库存 vs 总库存，告急排前） -->
        <InventoryPanel />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { getSeckillStats } from '@/api/seckillApi'
import { getAlarms, getMiddlewareStatus } from '@/api/monitorApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import RealTimePV from '@/components/Chart/RealTimePV.vue'
import HotRankChart from '@/components/Chart/HotRankChart.vue'
import MessageStackChart from '@/components/Chart/MessageStackChart.vue'
import InventoryPanel from '@/components/Chart/InventoryPanel.vue'
import { ElMessage } from 'element-plus'

const currentTime = ref(new Date().toLocaleString())
let timeTimer = null
let dataTimer = null

const stats = ref({ totalOrders: 0, successRate: 0, qps: 0, mqBacklog: 0, conversionRate: 0, pv: 0, uv: 0, seckillRequests: 0 })
const alarms = ref([])
const middleware = ref([])
/* [修复] 任务6: 全屏状态 */
const isFullscreen = ref(false)

// [修复] 任务6: 告警音效 - 使用 Web Audio API 生成提示音
let audioCtx = null
function playAlarmSound() {
  /* [修复] 任务6: 检查音效开关 */
  const soundEnabled = localStorage.getItem('alarmSoundEnabled') !== 'false'
  if (!soundEnabled) return
  try {
    if (!audioCtx) audioCtx = new (window.AudioContext || window.webkitAudioContext)()
    const oscillator = audioCtx.createOscillator()
    const gainNode = audioCtx.createGain()
    oscillator.connect(gainNode)
    gainNode.connect(audioCtx.destination)
    oscillator.frequency.value = 800
    oscillator.type = 'square'
    gainNode.gain.setValueAtTime(0.3, audioCtx.currentTime)
    gainNode.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.3)
    oscillator.start(audioCtx.currentTime)
    oscillator.stop(audioCtx.currentTime + 0.3)
  } catch (e) {
    // 浏览器不支持 Web Audio API，静默处理
  }
}

// 加载秒杀统计数据
async function loadStats() {
  try {
    const data = await getSeckillStats()
    if (data) {
      stats.value = {
        totalOrders: data.total_orders || 0,
        successRate: data.success_rate || 0,
        qps: data.qps || 0,
        mqBacklog: data.mq_backlog || 0,
        // [增强] 大屏指标补全：转化率/PV/UV/秒杀请求（真实统计引擎数据）
        conversionRate: (data.conversion_rate || 0).toFixed(2),
        pv: data.pv || 0,
        uv: data.uv || 0,
        seckillRequests: data.seckill_requests || 0
      }
    }
  } catch (e) { /* 使用默认值 */ }
}

// 加载告警和中间件状态
async function loadAlarmsAndMW() {
  try {
    const [alarmData, mwData] = await Promise.all([
      getAlarms().catch(() => []),
      getMiddlewareStatus().catch(() => [])
    ])
    alarms.value = (alarmData || []).map(a => ({
      level: a.level || 'info',
      msg: a.message || a.msg || '',
      time: a.created_at ? new Date(a.created_at).toLocaleTimeString() : ''
    }))
    // 如果没有告警，显示默认提示
    if (alarms.value.length === 0) {
      alarms.value = [{ level: 'info', msg: '当前无告警信息', time: '' }]
    }
    middleware.value = (mwData || []).map(m => ({
      name: m.name || '',
      status: m.status || 'up'
    }))
    // 如果没有中间件数据，显示默认列表
    if (middleware.value.length === 0) {
      middleware.value = [
        { name: 'MySQL', status: 'up' },
        { name: 'Redis', status: 'up' },
        { name: 'RabbitMQ', status: 'up' },
        { name: 'Kafka', status: 'up' },
        { name: 'Etcd', status: 'up' },
        { name: 'Sentinel', status: 'up' }
      ]
    }
  } catch (e) { /* 使用默认值 */ }
}

/* [修复] 任务6: 监听告警变化，当有 critical/error 级别告警时播放音效 */
watch(alarms, (newAlarms) => {
  const hasErrorAlarm = newAlarms.some(a => a.level === 'error' || a.level === 'critical')
  if (hasErrorAlarm && newAlarms.length > 0) {
    playAlarmSound()
  }
}, { deep: true })

/* [修复] 任务6: 全屏切换 */
function toggleFullscreen() {
  if (!isFullscreen.value) {
    const el = document.documentElement
    if (el.requestFullscreen) {
      el.requestFullscreen()
    } else if (el.webkitRequestFullscreen) {
      el.webkitRequestFullscreen()
    } else if (el.msRequestFullscreen) {
      el.msRequestFullscreen()
    }
  } else {
    if (document.exitFullscreen) {
      document.exitFullscreen()
    } else if (document.webkitExitFullscreen) {
      document.webkitExitFullscreen()
    } else if (document.msExitFullscreen) {
      document.msExitFullscreen()
    }
  }
}

/* [修复] 任务6: 监听全屏状态变化 */
function onFullscreenChange() {
  isFullscreen.value = !!(document.fullscreenElement || document.webkitFullscreenElement || document.msFullscreenElement)
}

/* [修复] 任务6: 导出营销报表 CSV */
function exportReport() {
  try {
    const headers = ['指标', '数值']
    const rows = [
      ['总订单数', stats.value.totalOrders],
      ['成功率', stats.value.successRate + '%'],
      ['当前QPS', stats.value.qps],
      ['MQ堆积量', stats.value.mqBacklog],
      ['UV转化率', stats.value.conversionRate + '%'],
      ['当日PV', stats.value.pv],
      ['当日UV', stats.value.uv],
      ['秒杀请求总数', stats.value.seckillRequests],
      ['告警总数', alarms.value.filter(a => a.level !== 'info').length],
      ['中间件正常数', middleware.value.filter(m => m.status === 'up').length],
      ['中间件异常数', middleware.value.filter(m => m.status !== 'up').length],
      ['导出时间', new Date().toLocaleString()]
    ]
    const bom = '\uFEFF'
    const csvContent = bom + [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', `营销报表_${new Date().toISOString().slice(0, 10)}.csv`)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
    ElMessage.success('导出成功')
  } catch (e) {
    ElMessage.error('导出失败')
  }
}

onMounted(() => {
  timeTimer = setInterval(() => { currentTime.value = new Date().toLocaleString() }, 1000)
  loadStats()
  loadAlarmsAndMW()
  // 每 5 秒刷新数据
  dataTimer = setInterval(() => { loadStats(); loadAlarmsAndMW() }, 5000)
  /* [修复] 任务6: 监听全屏状态变化 */
  document.addEventListener('fullscreenchange', onFullscreenChange)
  document.addEventListener('webkitfullscreenchange', onFullscreenChange)
  document.addEventListener('msfullscreenchange', onFullscreenChange)
})

onUnmounted(() => {
  clearInterval(timeTimer)
  if (dataTimer) clearInterval(dataTimer)
  /* [修复] 任务6: 移除全屏监听 */
  document.removeEventListener('fullscreenchange', onFullscreenChange)
  document.removeEventListener('webkitfullscreenchange', onFullscreenChange)
  document.removeEventListener('msfullscreenchange', onFullscreenChange)
})
</script>

<style scoped>
.big-screen { padding: 20px; height: 100%; overflow-y: auto }
.screen-header { text-align: center; margin-bottom: 24px; position: relative }
.screen-header h1 { font-size: 32px }
.header-time { color: #999; font-size: 16px; margin-top: 8px }
/* [修复] 任务6: 右上角操作按钮组 */
.header-actions { position: absolute; right: 0; top: 0; display: flex; gap: 8px }
.screen-grid { display: grid; grid-template-columns: 1fr 1.2fr 1fr; gap: 16px }
.col-left, .col-center, .col-right { display: flex; flex-direction: column; gap: 16px }
.stats-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px }
.stats-row-second { margin-top: 16px; padding-top: 16px; border-top: 1px solid rgba(255,255,255,.06) }
.stat-item { text-align: center }
.stat-value { font-size: 32px; font-weight: bold }
.stat-label { font-size: 13px; color: #999; margin-top: 4px }
.alarm-scroll { max-height: 200px; overflow-y: auto }
.alarm-row { display: flex; align-items: center; gap: 8px; padding: 8px 0; border-bottom: 1px solid rgba(255,255,255,.03); font-size: 13px }
.alarm-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0 }
.alarm-dot.error, .alarm-dot.critical { background: #e94560 }
.alarm-dot.warning { background: #e6a23c }
.alarm-dot.info { background: #409eff }
.alarm-msg { flex: 1; color: var(--text-primary) }
.alarm-time { color: #666; font-size: 12px }
.mw-status { display: flex; flex-direction: column; gap: 12px }
.mw-row { display: flex; justify-content: space-between; align-items: center }
.mw-name { color: var(--text-primary); font-size: 14px }
</style>