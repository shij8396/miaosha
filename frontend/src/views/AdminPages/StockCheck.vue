<template>
  <div class="page-container">
    <h2 class="page-title">库存对账</h2>
    <CardGlass>
      <el-alert title="库存对账用于检测 Redis 缓存与 MySQL 数据库的库存一致性" type="info" :closable="false" style="margin-bottom:16px" />
      <!-- [修复] 任务5: 批量修复进度提示 -->
      <div v-if="batchFixing" class="batch-progress" style="margin-bottom:16px">
        <el-alert type="warning" :closable="false">
          <template #title>
            正在批量修复中... 已修复 {{ batchFixedCount }} / {{ batchTotalCount }}
          </template>
        </el-alert>
        <el-progress :percentage="batchProgress" :color="'#e94560'" style="margin-top:8px" />
      </div>
      <DataTable :data="diffs" :loading="loading" :total="total" @page-change="onPageChange" @size-change="onSizeChange">
        <template #toolbar>
          <el-button type="primary" @click="loadDiffs">刷新对账</el-button>
          <!-- [修复] 任务5: 一键批量修复按钮 -->
          <el-button type="danger" @click="batchFixAll" :disabled="batchFixing" :loading="batchFixing">一键批量修复</el-button>
          <!-- [修复] 任务5: 导出对账报告按钮 -->
          <el-button @click="exportReport">导出对账报告</el-button>
        </template>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="product_id" label="商品ID" width="100" />
        <el-table-column prop="redis_stock" label="Redis库存" width="120" />
        <el-table-column prop="mysql_stock" label="MySQL库存" width="120" />
        <el-table-column prop="diff" label="差异值" width="100">
          <template #default="{ row }">
            <el-tag :type="row.diff === 0 ? 'success' : 'danger'">{{ row.diff > 0 ? '+' + row.diff : row.diff }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="auto_corrected" label="已修正" width="100">
          <template #default="{ row }">
            <el-tag :type="row.auto_corrected ? 'success' : 'info'" size="small">{{ row.auto_corrected ? '是' : '否' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button v-if="!row.auto_corrected && row.diff !== 0" type="warning" size="small" link @click="fixDiff(row)">修复</el-button>
          </template>
        </el-table-column>
      </DataTable>
    </CardGlass>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { getReconDiff, fixReconDiff } from '@/api/orderApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import DataTable from '@/components/Common/DataTable.vue'
import { ElMessage } from 'element-plus'
import { exportToExcel } from '@/utils/download'

const diffs = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
/* [修复] 任务5: 批量修复状态 */
const batchFixing = ref(false)
const batchFixedCount = ref(0)
const batchTotalCount = ref(0)
/* [修复] 任务5: 批量修复进度百分比 */
const batchProgress = computed(() => {
  if (batchTotalCount.value === 0) return 0
  return Math.round(batchFixedCount.value / batchTotalCount.value * 100)
})

async function loadDiffs() {
  loading.value = true
  try {
    const data = await getReconDiff({ page: page.value, page_size: pageSize.value })
    diffs.value = data?.list || []
    total.value = data?.total || 0
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('加载库存对账失败:', e) } finally { loading.value = false }
}

function onPageChange(p) { page.value = p; loadDiffs() }
function onSizeChange(s) { pageSize.value = s; loadDiffs() }

async function fixDiff(row) {
  try {
    await fixReconDiff(row.id)
    ElMessage.success('修复成功')
    loadDiffs()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('修复库存差异失败:', e) }
}

/* [修复] 任务5: 一键批量修复所有未修复的差异记录 */
async function batchFixAll() {
  // 获取所有待修复的记录（差异不为0且未修正）
  const unfixedRows = diffs.value.filter(r => !r.auto_corrected && r.diff !== 0)
  if (unfixedRows.length === 0) {
    ElMessage.info('没有需要修复的差异记录')
    return
  }
  batchFixing.value = true
  batchTotalCount.value = unfixedRows.length
  batchFixedCount.value = 0

  for (const row of unfixedRows) {
    try {
      await fixReconDiff(row.id)
      batchFixedCount.value++
    } catch (e) {
      console.error('修复库存差异失败, ID:', row.id, e)
    }
    // 稍微延迟，避免请求过快
    await new Promise(r => setTimeout(r, 100))
  }

  batchFixing.value = false
  ElMessage.success(`批量修复完成，成功修复 ${batchFixedCount.value} / ${batchTotalCount.value} 条记录`)
  loadDiffs()
}

/* [修复] 任务5: 导出对账报告为 CSV */
function exportReport() {
  try {
    const headers = ['ID', '商品ID', 'Redis库存', 'MySQL库存', '差异值', '已修正', '创建时间']
    const rows = diffs.value.map(r => [
      r.id,
      r.product_id,
      r.redis_stock,
      r.mysql_stock,
      r.diff,
      r.auto_corrected ? '是' : '否',
      new Date(r.created_at).toLocaleString()
    ])
    // 添加 BOM 头确保 Excel 正确识别中文
    const bom = '\uFEFF'
    const csvContent = bom + [headers.join(','), ...rows.map(r => r.join(','))].join('\n')
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const url = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', `库存对账报告_${new Date().toISOString().slice(0, 10)}.csv`)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
    ElMessage.success('导出成功')
  } catch (e) {
    ElMessage.error('导出失败')
  }
}

onMounted(loadDiffs)
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
/* [修复] 任务5: 批量修复进度样式 */
.batch-progress { padding: 12px; background: rgba(233,69,96,.05); border-radius: 8px; border: 1px solid rgba(233,69,96,.15) }
</style>