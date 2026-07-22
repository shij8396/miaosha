<template>
  <div class="page-container">
    <h2 class="page-title">黑名单管理</h2>
    <CardGlass>
      <DataTable :data="blacklist" :loading="loading">
        <template #toolbar>
          <el-button type="danger" @click="showAdd">添加黑名单</el-button>
          <el-button @click="loadBlacklist">刷新</el-button>
        </template>
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="row.type === 'ip' ? 'warning' : 'danger'" size="small">{{ row.type === 'ip' ? 'IP' : '用户' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="value" label="值" />
        <el-table-column prop="reason" label="原因" />
        <el-table-column prop="created_at" label="添加时间" width="180">
          <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button type="danger" size="small" link @click="handleRemove(row)">移除</el-button>
          </template>
        </el-table-column>
      </DataTable>
    </CardGlass>

    <el-dialog v-model="dialogVisible" title="添加黑名单" width="400px">
      <el-form :model="form" label-width="80px">
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio value="ip">IP地址</el-radio>
            <el-radio value="user">用户ID</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="值">
          <el-input v-model="form.value" :placeholder="form.type === 'ip' ? '如 192.168.1.1' : '如 123'" />
        </el-form-item>
        <el-form-item label="原因">
          <el-input v-model="form.reason" placeholder="封禁原因" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="danger" @click="handleAdd">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { getBlacklist, addBlacklist, removeBlacklist } from '@/api/sentinelApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import DataTable from '@/components/Common/DataTable.vue'
import { ElMessage } from 'element-plus'

const blacklist = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const form = reactive({ type: 'ip', value: '', reason: '' })

async function loadBlacklist() {
  loading.value = true
  try { blacklist.value = await getBlacklist() || [] } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('加载黑名单失败:', e) } finally { loading.value = false }
}

function showAdd() { dialogVisible.value = true }

async function handleAdd() {
  try {
    await addBlacklist(form)
    ElMessage.success('添加成功')
    dialogVisible.value = false
    loadBlacklist()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('添加黑名单失败:', e) }
}

async function handleRemove(row) {
  try {
    await removeBlacklist(row.id)
    ElMessage.success('移除成功')
    loadBlacklist()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('移除黑名单失败:', e) }
}

onMounted(loadBlacklist)
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
</style>