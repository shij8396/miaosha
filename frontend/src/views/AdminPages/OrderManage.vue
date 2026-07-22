<template>
  <div class="page-container">
    <h2 class="page-title">订单管理</h2>
    <CardGlass>
      <DataTable :data="orders" :loading="loading" :total="total" @page-change="onPageChange" @size-change="onSizeChange">
        <template #toolbar>
          <el-select v-model="statusFilter" placeholder="订单状态" clearable style="width:140px" @change="loadOrders">
            <el-option label="全部" value="" />
            <el-option label="待支付" :value="0" />
            <el-option label="已支付" :value="1" />
            <el-option label="已取消" :value="2" />
            <el-option label="已退款" :value="3" />
          </el-select>
          <!-- [修复] 任务7: 导入订单按钮 -->
          <el-upload
            :auto-upload="false"
            :show-file-list="false"
            accept=".xlsx,.xls,.csv"
            :on-change="handleImportOrders"
          >
            <el-button>导入订单</el-button>
          </el-upload>
          <el-button @click="exportOrders">导出Excel</el-button>
        </template>
        <el-table-column prop="order_no" label="订单号" width="220" />
        <el-table-column prop="user_id" label="用户ID" width="80" />
        <el-table-column prop="product_name" label="商品名称" />
        <el-table-column prop="seckill_price" label="秒杀价" width="100" />
        <el-table-column prop="total_amount" label="总金额" width="100" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="['warning','success','info','danger'][row.status]" size="small">
              {{ ['待支付','已支付','已取消','已退款'][row.status] }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
        </el-table-column>
      </DataTable>
    </CardGlass>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { getAllOrders } from '@/api/orderApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import DataTable from '@/components/Common/DataTable.vue'
import { ElMessage } from 'element-plus'
import * as XLSX from 'xlsx'

const orders = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const statusFilter = ref('')

async function loadOrders() {
  loading.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (statusFilter.value !== '') params.status = statusFilter.value
    const data = await getAllOrders(params)
    orders.value = data?.list || []
    total.value = data?.total || 0
  } catch (e) { /* 错误已在拦截器中处理 */ } finally { loading.value = false }
}

function onPageChange(p) { page.value = p; loadOrders() }
function onSizeChange(s) { pageSize.value = s; loadOrders() }

// 导出订单为 CSV 文件（直接下载）
async function exportOrders() {
  try {
    const token = localStorage.getItem('token')
    const params = new URLSearchParams()
    params.set('page', '1')
    params.set('page_size', '10000')
    if (statusFilter.value !== '') params.set('status', statusFilter.value)

    const response = await axios.get('/api/v1/order/export', {
      params,
      headers: { Authorization: `Bearer ${token}` },
      responseType: 'blob'
    })

    // 创建下载链接
    const url = window.URL.createObjectURL(new Blob([response.data]))
    const link = document.createElement('a')
    link.href = url
    link.setAttribute('download', `orders_${new Date().toISOString().slice(0, 10)}.csv`)
    document.body.appendChild(link)
    link.click()
    link.remove()
    window.URL.revokeObjectURL(url)
    ElMessage.success('导出成功')
  } catch (e) {
    ElMessage.error('导出失败')
  }
}

/* [修复] 任务7: 导入订单 - 读取 Excel/CSV 文件 */
async function handleImportOrders(file) {
  try {
    const data = await file.raw.arrayBuffer()
    const workbook = XLSX.read(data, { type: 'array' })
    const sheetName = workbook.SheetNames[0]
    const worksheet = workbook.Sheets[sheetName]
    const jsonData = XLSX.utils.sheet_to_json(worksheet)

    if (!jsonData || jsonData.length === 0) {
      ElMessage.warning('文件中没有数据')
      return
    }

    const token = localStorage.getItem('token')
    // 逐条导入订单数据
    let successCount = 0
    for (const row of jsonData) {
      try {
        await axios.post('/api/v1/order/import', row, {
          headers: { Authorization: `Bearer ${token}` }
        })
        successCount++
      } catch (e) {
        console.error('导入订单失败:', row, e)
      }
    }
    ElMessage.success(`导入完成，成功 ${successCount} / ${jsonData.length} 条`)
    loadOrders()
  } catch (e) {
    ElMessage.error('文件解析失败，请检查文件格式')
    console.error('导入订单异常:', e)
  }
}

onMounted(loadOrders)
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
</style>