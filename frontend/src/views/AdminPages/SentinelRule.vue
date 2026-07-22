<template>
  <div class="page-container">
    <h2 class="page-title">Sentinel限流规则</h2>
    <CardGlass>
      <DataTable :data="rules" :loading="loading">
        <template #toolbar>
          <el-button type="danger" @click="showAdd">新增规则</el-button>
          <el-button @click="loadRules">刷新</el-button>
        </template>
        <el-table-column prop="resource" label="资源名" width="180" />
        <el-table-column prop="grade" label="类型" width="100">
          <template #default="{ row }">{{ row.grade === 0 ? 'QPS' : '线程数' }}</template>
        </el-table-column>
        <el-table-column prop="count" label="阈值" width="100" />
        <el-table-column prop="limitApp" label="来源应用" width="120" />
        <el-table-column prop="strategy" label="策略" width="100">
          <template #default="{ row }">{{ ['直接','关联','链路'][row.strategy] || '直接' }}</template>
        </el-table-column>
        <el-table-column prop="controlBehavior" label="效果" width="100">
          <template #default="{ row }">{{ ['快速失败','WarmUp','排队等待'][row.controlBehavior] || '快速失败' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150">
          <template #default="{ row }">
            <el-button size="small" link type="primary" @click="showEdit(row)">编辑</el-button>
            <el-button size="small" link type="danger" @click="handleDelete(row)">删除</el-button>
          </template>
        </el-table-column>
      </DataTable>
    </CardGlass>

    <el-dialog v-model="dialogVisible" :title="editingRule ? '编辑规则' : '新增规则'" width="500px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="资源名">
          <el-input v-model="form.resource" placeholder="如 seckill_api" />
        </el-form-item>
        <el-form-item label="阈值类型">
          <el-radio-group v-model="form.grade">
            <el-radio :value="0">QPS</el-radio>
            <el-radio :value="1">线程数</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="阈值">
          <el-input-number v-model="form.count" :min="1" />
        </el-form-item>
        <el-form-item label="限流效果">
          <el-select v-model="form.controlBehavior" style="width:100%">
            <el-option label="快速失败" :value="0" />
            <el-option label="Warm Up" :value="1" />
            <el-option label="排队等待" :value="2" />
          </el-select>
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
import { getRules, addRule, updateRule, deleteRule } from '@/api/sentinelApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import DataTable from '@/components/Common/DataTable.vue'
import { ElMessage } from 'element-plus'

const rules = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const editingRule = ref(null) // [修复] 编辑中的规则（null=新增模式）
const form = reactive({ resource: 'seckill_api', grade: 0, count: 100, controlBehavior: 0 })

async function loadRules() {
  loading.value = true
  try { rules.value = await getRules() || [] } catch (e) { console.error('加载限流规则失败:', e) } finally { loading.value = false }
}

function showAdd() { editingRule.value = null; dialogVisible.value = true }
function showEdit(row) { editingRule.value = row; Object.assign(form, row); dialogVisible.value = true }

async function handleAdd() {
  try {
    if (editingRule.value) {
      await updateRule(editingRule.value.id, form)
      ElMessage.success('更新成功')
    } else {
      await addRule(form)
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadRules()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('添加限流规则失败:', e) }
}

async function handleDelete(row) {
  try {
    await deleteRule(row.id)
    ElMessage.success('删除成功')
    loadRules()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('删除限流规则失败:', e) }
}

onMounted(loadRules)
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
</style>