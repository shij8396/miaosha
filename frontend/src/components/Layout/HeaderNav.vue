<template>
  <div class="header-nav">
    <div class="header-left">
      <el-icon class="collapse-btn" @click="globalStore.toggleSidebar()" :size="20">
        <Fold v-if="!globalStore.sidebarCollapsed" /><Expand v-else />
      </el-icon>
      <el-breadcrumb separator="/">
        <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
        <el-breadcrumb-item v-if="route.meta.title">{{ route.meta.title }}</el-breadcrumb-item>
      </el-breadcrumb>
    </div>
    <div class="header-right">
      <el-tooltip content="主题切换">
        <el-icon class="action-btn" @click="globalStore.toggleTheme()" :size="18">
          <Sunny v-if="globalStore.theme === 'dark'" /><Moon v-else />
        </el-icon>
      </el-tooltip>
      <!-- [修复] 任务2: 告警铃铛 - 红点提示 + 弹出告警列表 -->
      <el-popover placement="bottom" :width="360" trigger="click" :visible="alarmPopVisible" @update:visible="onPopVisibleChange">
        <template #reference>
          <el-badge :value="globalStore.alarmMessages.length" :hidden="!globalStore.alarmMessages.length" class="alarm-badge">
            <el-icon class="action-btn" :size="18" @click="alarmPopVisible = true"><Bell /></el-icon>
          </el-badge>
        </template>
        <div class="alarm-popover">
          <div class="alarm-pop-header">
            <span class="alarm-pop-title">系统告警</span>
            <!-- [修复] 任务2: 告警音效开关 -->
            <el-tooltip :content="alarmSoundEnabled ? '关闭音效' : '开启音效'" placement="left">
              <el-icon class="sound-toggle" @click="toggleAlarmSound" :size="16">
                <VideoPlay v-if="alarmSoundEnabled" /><VideoPause v-else />
              </el-icon>
            </el-tooltip>
          </div>
          <div class="alarm-list" v-if="alarmMessagesSlice.length > 0">
            <div v-for="msg in alarmMessagesSlice" :key="msg.id" class="alarm-item">
              <span class="alarm-dot" :class="msg.level || 'info'"></span>
              <span class="alarm-text">{{ msg.msg || msg.message }}</span>
              <span class="alarm-item-time">{{ msg.time }}</span>
            </div>
          </div>
          <el-empty v-else description="暂无告警" :image-size="60" />
          <div class="alarm-pop-footer" v-if="globalStore.alarmMessages.length > 5">
            <el-button type="danger" link size="small" @click="viewAllAlarms">查看全部 ({{ globalStore.alarmMessages.length }})</el-button>
          </div>
        </div>
      </el-popover>
      <el-dropdown trigger="click">
        <div class="user-info">
          <el-avatar :size="32" style="background:#e94560">{{ userStore.username?.charAt(0)?.toUpperCase() }}</el-avatar>
          <span class="username">{{ userStore.username }}</span>
        </div>
        <template #dropdown>
          <el-dropdown-menu>
            <!-- [修复] F-07: 使用 router.push 替代 $router，避免模板中 $router 未定义导致退出登录崩溃 -->
            <el-dropdown-item @click="showPasswordDialog = true">修改密码</el-dropdown-item>
            <el-dropdown-item @click="handleLogout">退出登录</el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>
  </div>

  <!-- [修复] 修改密码弹窗 -->
  <el-dialog v-model="showPasswordDialog" title="修改密码" width="420px" :close-on-click-modal="false">
    <el-form :model="passwordForm" :rules="passwordRules" ref="passwordFormRef" label-width="80px">
      <el-form-item label="旧密码" prop="old_password">
        <el-input v-model="passwordForm.old_password" type="password" show-password placeholder="请输入旧密码" />
      </el-form-item>
      <el-form-item label="新密码" prop="new_password">
        <el-input v-model="passwordForm.new_password" type="password" show-password placeholder="请输入新密码（至少6位）" />
      </el-form-item>
      <el-form-item label="确认密码" prop="confirm_password">
        <el-input v-model="passwordForm.confirm_password" type="password" show-password placeholder="请再次输入新密码" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showPasswordDialog = false">取消</el-button>
      <el-button type="danger" @click="handleChangePassword" :loading="changingPassword">确认修改</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, computed, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGlobalStore } from '@/pinia/globalStore'
import { useUserStore } from '@/pinia/userStore'
import { changePassword } from '@/api/userApi'
import { ElMessage } from 'element-plus'
const route = useRoute()
// [修复] F-07: 添加 useRouter 实例，用于退出登录后跳转，避免模板中 $router 未定义
const router = useRouter()
const globalStore = useGlobalStore()
const userStore = useUserStore()

// [修复] 任务2: 告警弹窗控制
const alarmPopVisible = ref(false)
/* [修复] 任务2: 告警音效开关，从 localStorage 读取 */
const alarmSoundEnabled = ref(localStorage.getItem('alarmSoundEnabled') !== 'false')

/* [修复] 任务2: 显示最近 5 条告警 */
const alarmMessagesSlice = computed(() => globalStore.alarmMessages.slice(0, 5))

/* [修复] 任务2: 切换告警音效开关 */
function toggleAlarmSound() {
  alarmSoundEnabled.value = !alarmSoundEnabled.value
  localStorage.setItem('alarmSoundEnabled', alarmSoundEnabled.value.toString())
}

/* [修复] 任务2: 弹窗显示状态变化 */
function onPopVisibleChange(val) { alarmPopVisible.value = val }

/* [修复] 任务2: 查看全部告警，跳转到数据大屏 */
function viewAllAlarms() {
  alarmPopVisible.value = false
  router.push('/screen')
}

// [修复] F-07: 退出登录处理函数，清除所有状态并跳转登录页
function handleLogout() {
  userStore.logout()
  router.push('/login')
}

// [修复] 修改密码功能
const showPasswordDialog = ref(false)
const changingPassword = ref(false)
const passwordFormRef = ref(null)
const passwordForm = reactive({ old_password: '', new_password: '', confirm_password: '' })

const validateConfirm = (rule, value, callback) => {
  if (value !== passwordForm.new_password) { callback(new Error('两次输入的密码不一致')) } else { callback() }
}

const passwordRules = {
  old_password: [{ required: true, message: '请输入旧密码', trigger: 'blur' }],
  new_password: [{ required: true, min: 6, message: '新密码至少6位', trigger: 'blur' }],
  confirm_password: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    { validator: validateConfirm, trigger: 'blur' }
  ]
}

async function handleChangePassword() {
  const valid = await passwordFormRef.value?.validate().catch(() => false)
  if (!valid) return
  changingPassword.value = true
  try {
    await changePassword({
      old_password: passwordForm.old_password,
      new_password: passwordForm.new_password
    })
    ElMessage.success('密码修改成功，请重新登录')
    showPasswordDialog.value = false
    passwordForm.old_password = ''
    passwordForm.new_password = ''
    passwordForm.confirm_password = ''
    setTimeout(() => handleLogout(), 1500)
  } catch (e) {
    console.error('修改密码失败:', e)
  } finally {
    changingPassword.value = false
  }
}
</script>

<style scoped>
.header-nav {
  height: 60px; display: flex; align-items: center; justify-content: space-between;
  padding: 0 24px; background: var(--header-bg); border-bottom: 1px solid rgba(255,255,255,.06);
  position: relative; z-index: 99;
}
.header-left { display: flex; align-items: center; gap: 16px }
.collapse-btn { cursor: pointer; color: #999; transition: color .3s }
.collapse-btn:hover { color: #e94560 }
.header-right { display: flex; align-items: center; gap: 20px }
.action-btn { cursor: pointer; color: #999; transition: color .3s }
.action-btn:hover { color: #e94560 }
.user-info { display: flex; align-items: center; gap: 8px; cursor: pointer }
.username { font-size: 14px; color: var(--text-primary) }

/* [修复] 任务2: 告警弹窗样式 */
.alarm-badge { cursor: pointer }
.alarm-popover { max-height: 360px; overflow-y: auto }
.alarm-pop-header { display: flex; align-items: center; justify-content: space-between; padding-bottom: 8px; border-bottom: 1px solid rgba(255,255,255,.06); margin-bottom: 8px }
.alarm-pop-title { font-size: 14px; font-weight: bold; color: var(--text-primary) }
.sound-toggle { cursor: pointer; color: #999; transition: color .3s }
.sound-toggle:hover { color: #e94560 }
.alarm-list { display: flex; flex-direction: column; gap: 4px }
.alarm-item { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-radius: 6px; font-size: 13px; transition: background .2s }
.alarm-item:hover { background: rgba(255,255,255,.04) }
.alarm-item .alarm-dot { width: 6px; height: 6px; border-radius: 50%; flex-shrink: 0 }
.alarm-item .alarm-dot.error { background: #e94560 }
.alarm-item .alarm-dot.warning { background: #e6a23c }
.alarm-item .alarm-dot.info { background: #409eff }
.alarm-text { flex: 1; color: var(--text-primary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap }
.alarm-item-time { color: #666; font-size: 11px; flex-shrink: 0 }
.alarm-pop-footer { margin-top: 8px; padding-top: 8px; border-top: 1px solid rgba(255,255,255,.06); text-align: center }
</style>