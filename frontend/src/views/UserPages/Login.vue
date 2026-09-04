<template>
  <div class="login-page">
    <div class="bg-grid"></div>
    <div class="login-card glass-card fade-in-up">
      <div class="login-header">
        <h1 class="gradient-text">秒杀系统</h1>
        <p>企业级分布式高可用秒杀平台</p>
      </div>
      <el-form :model="form" :rules="rules" ref="formRef" label-width="0" size="large">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" :prefix-icon="User" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" placeholder="密码" :prefix-icon="Lock" show-password @keyup.enter="handleLogin" />
        </el-form-item>
        <el-form-item prop="captcha" v-if="showCaptcha">
          <el-input v-model="form.captcha" placeholder="验证码（4位数字）" :prefix-icon="Key" maxlength="4" style="width:60%" />
          <span class="captcha-display" @click="refreshCaptcha">{{ captchaCode }}</span>
        </el-form-item>
        <el-form-item>
          <el-button type="danger" style="width:100%" @click="handleLogin" :loading="loading" :disabled="lockoutCountdown > 0" size="large">
            {{ lockoutCountdown > 0 ? `请等待 ${lockoutCountdown} 秒` : '登 录' }}
          </el-button>
        </el-form-item>
      </el-form>
      <div class="login-footer">
        <span>还没有账号？</span>
        <router-link to="/register" class="link">立即注册</router-link>
        <el-divider direction="vertical" />
        <a class="link" @click="showForgotDialog = true">忘记密码？</a>
      </div>
    </div>

    <el-dialog v-model="showForgotDialog" title="找回密码" width="420px" :close-on-click-modal="false" @closed="resetForgotForm">
      <el-form :model="forgotForm" :rules="forgotRules" ref="forgotFormRef" label-width="0" size="large">
        <el-form-item prop="username">
          <el-input v-model="forgotForm.username" placeholder="请输入用户名" :prefix-icon="User" />
        </el-form-item>
        <el-form-item prop="phone">
          <el-input v-model="forgotForm.phone" placeholder="请输入注册时绑定的手机号" :prefix-icon="Phone" />
        </el-form-item>
        <el-form-item prop="newPassword">
          <el-input v-model="forgotForm.newPassword" type="password" placeholder="请输入新密码（至少6位）" :prefix-icon="Lock" show-password />
        </el-form-item>
        <el-form-item prop="confirmPassword">
          <el-input v-model="forgotForm.confirmPassword" type="password" placeholder="请再次输入新密码" :prefix-icon="Lock" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showForgotDialog = false">取消</el-button>
        <el-button type="danger" @click="handleForgotPassword" :loading="forgotLoading">重置密码</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock, Key, Phone } from '@element-plus/icons-vue'
import { useUserStore } from '@/pinia/userStore'
import { forgotPassword } from '@/api/userApi'
import { ElMessage } from 'element-plus'

const router = useRouter()
const userStore = useUserStore()
const formRef = ref(null)
const loading = ref(false)
const form = reactive({ username: '', password: '', captcha: '' })
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

const MAX_ATTEMPTS = 5
const LOCKOUT_DURATION = 60
const attemptCount = ref(0)
const lockoutCountdown = ref(0)
const showCaptcha = ref(false)
const captchaCode = ref('')
let lockoutTimer = null

const showForgotDialog = ref(false)
const forgotLoading = ref(false)
const forgotFormRef = ref(null)
const forgotForm = reactive({ username: '', phone: '', newPassword: '', confirmPassword: '' })
const forgotRules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  phone: [{ required: true, message: '请输入手机号', trigger: 'blur' }],
  newPassword: [{ required: true, min: 6, message: '密码至少6位', trigger: 'blur' }],
  confirmPassword: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    {
      validator: (rule, value, callback) => {
        if (value !== forgotForm.newPassword) {
          callback(new Error('两次输入的密码不一致'))
        } else {
          callback()
        }
      },
      trigger: 'blur'
    }
  ]
}

function resetForgotForm() {
  forgotForm.username = ''
  forgotForm.phone = ''
  forgotForm.newPassword = ''
  forgotForm.confirmPassword = ''
  forgotFormRef.value?.clearValidate()
}

async function handleForgotPassword() {
  const valid = await forgotFormRef.value.validate().catch(() => false)
  if (!valid) return
  forgotLoading.value = true
  try {
    await forgotPassword({
      username: forgotForm.username,
      phone: forgotForm.phone,
      new_password: forgotForm.newPassword
    })
    ElMessage.success('密码重置成功，请使用新密码登录')
    showForgotDialog.value = false
  } catch (e) {
    ElMessage.error(e?.message || '密码重置失败，请稍后重试')
  } finally {
    forgotLoading.value = false
  }
}

function refreshCaptcha() {
  captchaCode.value = String(Math.floor(1000 + Math.random() * 9000))
}

function startLockout() {
  lockoutCountdown.value = LOCKOUT_DURATION
  lockoutTimer = setInterval(() => {
    lockoutCountdown.value--
    if (lockoutCountdown.value <= 0) {
      clearInterval(lockoutTimer)
      lockoutTimer = null
      attemptCount.value = 0
    }
  }, 1000)
}

async function handleLogin() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  if (showCaptcha.value && form.captcha !== captchaCode.value) {
    ElMessage.error('验证码错误')
    refreshCaptcha()
    form.captcha = ''
    return
  }

  loading.value = true
  try {
    await userStore.login({ ...form, password: form.password })
    ElMessage.success('登录成功')
    attemptCount.value = 0
    showCaptcha.value = false
    router.push('/seckill')
  } catch (e) {
    const errMsg = e?.message || '登录失败，请检查用户名和密码'
    ElMessage.error(errMsg)
    attemptCount.value++
    if (attemptCount.value >= MAX_ATTEMPTS) {
      startLockout()
      ElMessage.error(`登录失败次数过多，请 ${LOCKOUT_DURATION} 秒后再试`)
    } else if (attemptCount.value >= 3) {
      showCaptcha.value = true
      refreshCaptcha()
    }
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  width: 100vw; height: 100vh; display: flex; align-items: center; justify-content: center;
  position: relative; z-index: 1;
}
.bg-grid {
  position: fixed; inset: 0; z-index: 0;
  background-image: linear-gradient(rgba(233,69,96,.03) 1px, transparent 1px), linear-gradient(90deg, rgba(233,69,96,.03) 1px, transparent 1px);
  background-size: 60px 60px;
}
.login-card {
  width: 420px; padding: 48px 40px; position: relative; z-index: 1;
}
.login-header { text-align: center; margin-bottom: 36px }
.login-header h1 { font-size: 32px; margin-bottom: 8px }
.login-header p { color: #999; font-size: 14px }
.login-footer { text-align: center; color: #999; font-size: 14px; margin-top: 16px }
.link { color: #e94560; text-decoration: none; margin-left: 4px }
.link:hover { text-decoration: underline }
/* [修复] S-22: 验证码显示样式 */
.captcha-display {
  display: inline-block; margin-left: 12px; padding: 4px 16px;
  background: rgba(233,69,96,.15); color: #e94560; font-size: 20px;
  letter-spacing: 6px; font-weight: bold; cursor: pointer; border-radius: 4px;
  user-select: none; vertical-align: middle;
}
.captcha-display:hover { background: rgba(233,69,96,.25) }
</style>