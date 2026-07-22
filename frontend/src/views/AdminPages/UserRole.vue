<template>
  <div class="page-container">
    <h2 class="page-title">权限管理</h2>
    <CardGlass>
      <!-- [修复] 任务10: 角色说明卡片 -->
      <div class="role-info-cards">
        <el-row :gutter="12">
          <el-col :span="6">
            <div class="role-card role-super-admin">
              <div class="role-card-tag">super_admin</div>
              <div class="role-card-name">超级管理员</div>
              <div class="role-card-desc">全部菜单权限，可管理所有功能</div>
            </div>
          </el-col>
          <el-col :span="6">
            <div class="role-card role-operator">
              <div class="role-card-tag">operator</div>
              <div class="role-card-name">运营专员</div>
              <div class="role-card-desc">商品管理、订单管理、活动配置、库存对账</div>
            </div>
          </el-col>
          <el-col :span="6">
            <div class="role-card role-monitor">
              <div class="role-card-tag">monitor_readonly</div>
              <div class="role-card-name">监控只读</div>
              <div class="role-card-desc">数据大屏、服务监控（只读权限）</div>
            </div>
          </el-col>
          <el-col :span="6">
            <div class="role-card role-risk">
              <div class="role-card-tag">risk_control</div>
              <div class="role-card-name">风控专员</div>
              <div class="role-card-desc">限流规则、黑名单、权限管理</div>
            </div>
          </el-col>
        </el-row>
      </div>
      <DataTable :data="users" :loading="loading" :total="total" @page-change="onPageChange" @size-change="onSizeChange">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="username" label="用户名" />
        <el-table-column prop="phone" label="手机号" width="140" />
        <el-table-column prop="role" label="角色" width="160">
          <template #default="{ row }">
            <el-tag :type="getRoleTagType(row.role)" size="small">{{ getRoleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="注册时间" width="180">
          <template #default="{ row }">{{ new Date(row.created_at).toLocaleString() }}</template>
        </el-table-column>
        <el-table-column label="操作" width="180">
          <template #default="{ row }">
            <!-- [修复] 任务10: 四级角色选择 -->
            <el-select
              :model-value="row.role"
              size="small"
              style="width:140px"
              @change="(val) => changeRole(row, val)"
            >
              <el-option label="超级管理员" value="super_admin" />
              <el-option label="运营专员" value="operator" />
              <el-option label="监控只读" value="monitor_readonly" />
              <el-option label="风控专员" value="risk_control" />
              <el-option label="普通用户" value="user" />
            </el-select>
          </template>
        </el-table-column>
      </DataTable>
    </CardGlass>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { getUserList, updateUserRole } from '@/api/userApi'
import CardGlass from '@/components/Common/CardGlass.vue'
import DataTable from '@/components/Common/DataTable.vue'
import { ElMessage } from 'element-plus'

const users = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

/* [修复] 任务10: 角色标签映射 */
const roleMap = {
  super_admin: '超级管理员',
  operator: '运营专员',
  monitor_readonly: '监控只读',
  risk_control: '风控专员',
  admin: '管理员',
  user: '普通用户'
}

/* [修复] 任务10: 角色标签类型映射 */
function getRoleTagType(role) {
  const typeMap = {
    super_admin: '',
    operator: 'warning',
    monitor_readonly: 'info',
    risk_control: 'danger',
    admin: 'danger',
    user: 'info'
  }
  return typeMap[role] || 'info'
}

/* [修复] 任务10: 获取角色显示名称 */
function getRoleLabel(role) {
  return roleMap[role] || role || '普通用户'
}

async function loadUsers() {
  loading.value = true
  try {
    const data = await getUserList({ page: page.value, page_size: pageSize.value })
    users.value = data?.list || []
    total.value = data?.total || 0
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('加载用户列表失败:', e) } finally { loading.value = false }
}

function onPageChange(p) { page.value = p; loadUsers() }
function onSizeChange(s) { pageSize.value = s; loadUsers() }

/* [修复] 任务10: 更改用户角色 */
async function changeRole(row, newRole) {
  try {
    await updateUserRole({ user_id: row.id, role: newRole })
    ElMessage.success(`已更新为 ${getRoleLabel(newRole)}`)
    loadUsers()
  } catch (e) { /* [修复] 错误已在拦截器中处理，此处记录日志便于调试 */ console.error('更新用户角色失败:', e) }
}

onMounted(loadUsers)
</script>

<style scoped>
.page-title { font-size: 24px; color: var(--text-primary); margin-bottom: 24px }
/* [修复] 任务10: 角色说明卡片样式 */
.role-info-cards { margin-bottom: 20px }
.role-card { padding: 16px; border-radius: 10px; border: 1px solid rgba(255,255,255,.08); transition: all .3s }
.role-card:hover { transform: translateY(-2px) }
.role-card-tag { font-size: 11px; font-family: monospace; margin-bottom: 6px; opacity: .7 }
.role-card-name { font-size: 15px; font-weight: bold; margin-bottom: 4px; color: var(--text-primary) }
.role-card-desc { font-size: 12px; color: #999; line-height: 1.5 }
.role-super-admin { background: rgba(103,194,58,.08); border-color: rgba(103,194,58,.15) }
.role-super-admin .role-card-tag { color: #67c23a }
.role-operator { background: rgba(230,162,60,.08); border-color: rgba(230,162,60,.15) }
.role-operator .role-card-tag { color: #e6a23c }
.role-monitor { background: rgba(64,158,255,.08); border-color: rgba(64,158,255,.15) }
.role-monitor .role-card-tag { color: #409eff }
.role-risk { background: rgba(233,69,96,.08); border-color: rgba(233,69,96,.15) }
.role-risk .role-card-tag { color: #e94560 }
</style>