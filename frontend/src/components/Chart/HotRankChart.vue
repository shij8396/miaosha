<template>
  <CardGlass title="热销商品排行">
    <div v-if="empty" class="empty-tip">暂无秒杀成交数据（有秒杀成功后此处显示真实排行）</div>
    <div v-show="!empty" ref="chartRef" style="width:100%;height:300px"></div>
  </CardGlass>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, getCurrentInstance } from 'vue'
import CardGlass from '@/components/Common/CardGlass.vue'
import { getHotProducts } from '@/api/monitorApi'

const { proxy } = getCurrentInstance()
const chartRef = ref(null)
let chart = null
let timer = null

const items = ref([])
const empty = computed(() => items.value.length === 0)

// [增强] 拉取真实热销排行（累计销售件数 + 实时库存），5 秒刷新
async function loadData() {
  try {
    const data = await getHotProducts(10)
    if (!Array.isArray(data)) return
    items.value = data
    if (data.length === 0) return
    // 后端已按销量降序返回，横向条形图需倒序绘制（最大值在顶部）
    const reversed = [...data].reverse()
    chart.setOption({
      yAxis: { data: reversed.map(p => p.product_name || `商品${p.product_id}`) },
      series: [{
        data: reversed.map(p => p.sold_quantity || 0)
      }]
    })
  } catch (e) { /* 接口异常时保留上一次数据 */ }
}

onMounted(() => {
  chart = proxy.$echarts.init(chartRef.value)
  chart.setOption({
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      formatter: (params) => {
        const idx = params[0].dataIndex
        const p = [...items.value].reverse()[idx] || {}
        return `<b>${p.product_name || ''}</b><br/>已售：${p.sold_quantity || 0} 件<br/>` +
          `Redis库存：${p.redis_stock ?? '-'} / 总库存 ${p.total_stock ?? '-'}<br/>` +
          `秒杀价：￥${(p.seckill_price || 0).toFixed(2)}`
      }
    },
    grid: { left: '3%', right: '8%', bottom: '3%', containLabel: true },
    xAxis: { type: 'value', splitLine: { lineStyle: { color: 'rgba(255,255,255,.06)' } } },
    yAxis: { type: 'category', data: [], axisLabel: { color: '#999' } },
    series: [{
      name: '已售件数', type: 'bar', data: [],
      label: { show: true, position: 'right', color: '#e94560' },
      itemStyle: { color: new proxy.$echarts.graphic.LinearGradient(0, 0, 1, 0, [
        { offset: 0, color: '#e94560' }, { offset: 1, color: '#ff6b6b' }
      ])}
    }]
  })
  loadData()
  timer = setInterval(loadData, 5000)
})

onUnmounted(() => { clearInterval(timer); chart?.dispose() })
</script>

<style scoped>
.empty-tip { color: #666; font-size: 13px; text-align: center; padding: 40px 0 }
</style>
