<template>
  <CardGlass title="库存实时监控">
    <div v-if="items.length === 0" class="empty-tip">暂无上架商品</div>
    <div v-else class="inv-scroll">
      <div v-for="p in items" :key="p.product_id" class="inv-row">
        <div class="inv-head">
          <span class="inv-name" :title="p.product_name">{{ p.product_name }}</span>
          <el-tag :type="tagType(p.warning_level)" size="small" effect="dark">
            {{ levelText(p.warning_level) }}
          </el-tag>
        </div>
        <div class="inv-body">
          <el-progress
            :percentage="Math.min(100, Math.max(0, Math.round(p.stock_percent || 0)))"
            :color="barColor(p.warning_level)"
            :stroke-width="10"
            :show-text="false"
          />
          <span class="inv-stock" :style="{ color: barColor(p.warning_level) }">
            {{ p.redis_stock }}/{{ p.total_stock }}
          </span>
        </div>
      </div>
    </div>
  </CardGlass>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import CardGlass from '@/components/Common/CardGlass.vue'
import { getInventory } from '@/api/monitorApi'

// [增强] 库存状态面板：Redis 实时库存 vs 总库存，告急库存排前（后端已按剩余率升序返回）
const items = ref([])
let timer = null

async function loadData() {
  try {
    const data = await getInventory()
    if (Array.isArray(data)) items.value = data
  } catch (e) { /* 接口异常时保留上一次数据 */ }
}

// warning_level: soldout(售罄) / danger(<10%) / warning(<30%) / normal
function levelText(level) {
  return { soldout: '已售罄', danger: '库存告急', warning: '库存偏低', normal: '正常' }[level] || '正常'
}
function tagType(level) {
  return { soldout: 'danger', danger: 'danger', warning: 'warning', normal: 'success' }[level] || 'success'
}
function barColor(level) {
  return { soldout: '#909399', danger: '#e94560', warning: '#e6a23c', normal: '#67c23a' }[level] || '#67c23a'
}

onMounted(() => {
  loadData()
  // [增强] 5 秒刷新，与后端 Redis 库存实时对账
  timer = setInterval(loadData, 5000)
})

onUnmounted(() => { clearInterval(timer) })
</script>

<style scoped>
.empty-tip { color: #666; font-size: 13px; text-align: center; padding: 40px 0 }
.inv-scroll { max-height: 300px; overflow-y: auto; display: flex; flex-direction: column; gap: 14px }
.inv-row { display: flex; flex-direction: column; gap: 6px }
.inv-head { display: flex; justify-content: space-between; align-items: center }
.inv-name { color: var(--text-primary); font-size: 13px; max-width: 70%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap }
.inv-body { display: flex; align-items: center; gap: 10px }
.inv-body :deep(.el-progress) { flex: 1 }
.inv-stock { font-size: 12px; font-weight: bold; min-width: 60px; text-align: right }
</style>
