<template>
  <div class="page-container">
    <h2 class="page-title">我的订单</h2>
    <CardGlass>
      <!-- [修复] 添加订单状态筛选栏 -->
      <div class="filter-bar">
        <el-radio-group v-model="statusFilter" @change="loadOrders">
          <el-radio-button value="all">全部</el-radio-button>
          <el-radio-button value="pending">待支付</el-radio-button>
          <el-radio-button value="paid">已支付</el-radio-button>
          <el-radio-button value="cancelled">已取消</el-radio-button>
          <el-radio-button value="refunded">已退款</el-radio-button>
          <el-radio-button value="timeout">超时取消</el-radio-button>
        </el-radio-group>
      </div>
      <DataTable :data="orders" :loading="loading" :total="total" @page-change="onPageChange" @size-change="onSizeChange">
        <el-table-column prop="order_no" label="订单号" width="220" />
        <el-table-column prop="product_name" label="商品名称" />
        <el-table-column prop="seckill_price" label="秒杀价" width="100">
          <template #default="{ row }">¥{{ row.seckill_price }}</template>
        </el-table-column>
        <el-table-column prop="quantity" label="数量" width="60" />
        <el-table-column prop="total_amount" label="总金额" width="100">
          <template #default="{ row }">¥{{ row.total_amount }}</template>
        </el-table-column>
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
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button v-if="row.status === 0" type="danger" size="small" link @click="handleCancel(row)">取消</el-button>
          </template>
        </el-table-column>
      </DataTable>
    </CardGlass>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getOrders, cancelOrder } from '@/api/orderApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import DataTable from '@/components/Common/DataTable.vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const orders = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
// [修复] 订单状态筛选：all=全部, pending=待支付, paid=已支付, cancelled=已取消, refunded=已退款, timeout=超时取消
const statusFilter = ref('all')

async function loadOrders() {
  loading.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    // [修复] 非"全部"时传递状态筛选参数
    if (statusFilter.value !== 'all') {
      params.status = statusFilter.value
    }
    const data = await getOrders(params)
    orders.value = data?.list || []
    total.value = data?.total || 0
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('加载订单列表失败:', e) } finally { loading.value = false }
}

function onPageChange(p) { page.value = p; loadOrders() }
function onSizeChange(s) { pageSize.value = s; loadOrders() }

async function handleCancel(row) {
  try {
    await ElMessageBox.confirm('确定取消此订单吗？', '取消订单', { type: 'warning' })
    await cancelOrder({ order_no: row.order_no, reason: '用户主动取消' })
    ElMessage.success('取消成功')
    loadOrders()
  } catch (e) {
    // [修复] ElMessageBox 取消操作会抛出 'cancel' 错误，需区分处理
    if (e !== 'cancel' && e !== 'close') {
      console.error('取消订单失败:', e)
    }
  }
}

onMounted(loadOrders)
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
/* [修复] 订单状态筛选栏样式 */
.filter-bar { margin-bottom: 16px }
</style>