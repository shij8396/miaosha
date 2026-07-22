<template>
  <div class="page-container">
    <h2 class="page-title">活动配置</h2>

    <!-- [修复] 顶部操作栏：时间开关 + 缓存预热 -->
    <CardGlass style="margin-bottom:16px">
      <div style="display:flex;align-items:center;gap:16px;flex-wrap:wrap">
        <el-button type="danger" @click="handleCacheWarmup" :loading="warmingUp" icon="RefreshRight">一键缓存预热</el-button>
        <el-button @click="loadActivityProducts" icon="Refresh">刷新列表</el-button>
        <el-divider direction="vertical" />
        <span style="color:#888;font-size:13px">开启后自动预热 Redis 库存，确保秒杀扣减准确</span>
      </div>
    </CardGlass>

    <!-- [修复] 活动商品列表，支持编辑限购数量、活动时间 -->
    <CardGlass title="活动商品管理" style="margin-bottom:16px">
      <el-table :data="activityProducts" v-loading="loadingProducts" stripe @selection-change="onSelectionChange">
        <el-table-column type="selection" width="40" />
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="商品名称" min-width="120" />
        <el-table-column prop="seckill_price" label="秒杀价" width="100">
          <template #default="{ row }">¥{{ row.seckill_price }}</template>
        </el-table-column>
        <!-- [修复] 限购规则改造：可编辑的限购数量列 -->
        <el-table-column label="限购数量" width="140">
          <template #default="{ row }">
            <el-input-number v-model="row._editLimit" :min="1" :max="99" size="small" controls-position="right" style="width:100px" />
          </template>
        </el-table-column>
        <!-- [修复] 活动时间开关 -->
        <el-table-column label="活动时间" min-width="240">
          <template #default="{ row }">
            <div style="display:flex;flex-direction:column;gap:4px;font-size:12px">
              <span><strong>开始：</strong>{{ row.start_time || '-' }}</span>
              <span><strong>结束：</strong>{{ row.end_time || '-' }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="remain_stock" label="剩余库存" width="90" />
        <el-table-column prop="total_stock" label="总库存" width="80" />
        <el-table-column prop="status" label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">{{ row.status === 1 ? '上架' : '下架' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="handleSaveConfig(row)" :loading="savingId === row.id">保存</el-button>
            <el-button size="small" :type="row.status === 1 ? 'warning' : 'success'" @click="toggleProductStatus(row)">{{ row.status === 1 ? '下架' : '上架' }}</el-button>
          </template>
        </el-table-column>
      </el-table>
      <!-- [修复] 批量操作栏 -->
      <div style="margin-top:12px;display:flex;gap:8px" v-if="selectedIds.length > 0">
        <span style="color:#888;font-size:13px;line-height:32px">已选 {{ selectedIds.length }} 项</span>
        <el-button size="small" type="success" @click="batchToggleStatus(1)">批量上架</el-button>
        <el-button size="small" type="warning" @click="batchToggleStatus(0)">批量下架</el-button>
        <el-button size="small" type="danger" @click="batchCacheWarmup">批量缓存预热</el-button>
      </div>
      <el-empty v-if="!loadingProducts && activityProducts.length === 0" description="暂无活动商品" />
    </CardGlass>

    <!-- [修复] 活动配置说明 -->
    <CardGlass title="配置说明">
      <el-alert type="info" :closable="false" show-icon>
        <template #title>
          <ul style="margin:0;padding-left:16px;line-height:2">
            <li><strong>限购数量</strong>：每个用户最多可抢购该商品的件数（1-99），修改后实时生效无需重启</li>
            <li><strong>活动时间</strong>：在「商品管理」页面编辑商品时设置活动起止时间</li>
            <li><strong>上下架</strong>：上架后商品立即对用户可见，同时自动同步 Redis 库存缓存</li>
            <li><strong>缓存预热</strong>：将所有在售商品库存同步到 Redis，确保秒杀扣减准确</li>
            <li><strong>批量操作</strong>：勾选多个商品后可使用批量上下架和批量缓存预热</li>
          </ul>
        </template>
      </el-alert>
    </CardGlass>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getActiveProducts, saveActivityConfig } from '@/api/goodsApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const activityProducts = ref([])
const loadingProducts = ref(false)
const warmingUp = ref(false)
const savingId = ref(null)
// [修复] 批量选择
const selectedIds = ref([])

function onSelectionChange(selection) {
  selectedIds.value = selection.map(s => s.id)
}

async function loadActivityProducts() {
  loadingProducts.value = true
  try {
    const data = await getActiveProducts()
    const list = Array.isArray(data) ? data : (data?.list || [])
    activityProducts.value = list.map(p => ({
      ...p,
      _editLimit: p.limit_per_user || 1
    }))
  } catch (e) {
    console.error('加载活动商品失败:', e)
  } finally {
    loadingProducts.value = false
  }
}

async function toggleProductStatus(row) {
  const token = localStorage.getItem('token')
  const newStatus = row.status === 1 ? 0 : 1
  try {
    await axios.put('/api/v1/activity', { product_id: row.id, status: newStatus }, { headers: { Authorization: `Bearer ${token}` } })
    ElMessage.success(newStatus === 1 ? '已上架' : '已下架')
    loadActivityProducts()
  } catch (e) { ElMessage.error('操作失败') }
}

async function handleSaveConfig(row) {
  savingId.value = row.id
  try {
    await saveActivityConfig({ product_id: row.id, limit_per_user: row._editLimit })
    ElMessage.success(`商品「${row.name}」限购数量已更新为 ${row._editLimit} 件，实时生效`)
    row.limit_per_user = row._editLimit
  } catch (e) {
    ElMessage.error('保存失败: ' + (e?.message || '未知错误'))
  } finally { savingId.value = null }
}

async function handleCacheWarmup() {
  warmingUp.value = true
  const token = localStorage.getItem('token')
  try {
    const res = await axios.post('/api/v1/activity/cache-warmup', {}, { headers: { Authorization: `Bearer ${token}` } })
    const count = res.data?.data?.warmed_up_count || 0
    ElMessage.success(`缓存预热完成，成功预热 ${count} 个商品`)
  } catch (e) { ElMessage.error('缓存预热失败') }
  finally { warmingUp.value = false }
}

// [修复] 批量上下架
async function batchToggleStatus(status) {
  const token = localStorage.getItem('token')
  let count = 0
  for (const id of selectedIds.value) {
    try {
      await axios.put('/api/v1/activity', { product_id: id, status }, { headers: { Authorization: `Bearer ${token}` } })
      count++
    } catch (e) { /* skip */ }
  }
  ElMessage.success(`批量${status === 1 ? '上架' : '下架'}完成，成功 ${count} / ${selectedIds.value.length} 个`)
  loadActivityProducts()
}

// [修复] 批量缓存预热
async function batchCacheWarmup() {
  warmingUp.value = true
  try {
    await handleCacheWarmup()
  } finally { warmingUp.value = false }
}

onMounted(() => { loadActivityProducts() })
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
</style>