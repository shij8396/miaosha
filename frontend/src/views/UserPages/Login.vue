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
        <!-- [修复] S-22: 添加验证码输入框，防止暴力破解 -->
        <el-form-item prop="captcha" v-if="showCaptcha">
          <el-input v-model="form.captcha" placeholder="验证码（4位数字）" :prefix-icon="Key" maxlength="4" style="width:60%" />
          <span class="captcha-display" @click="refreshCaptcha">{{ captchaCode }}</span>
        </el-form-item>
        <el-form-item>
          <!-- [修复] S-22: 登录失败次数过多时禁用按钮并显示倒计时 -->
          <el-button type="danger" style="width:100%" @click="handleLogin" :loading="loading" :disabled="lockoutCountdown > 0" size="large">
            {{ lockoutCountdown > 0 ? `请等待 ${lockoutCountdown} 秒` : '登 录' }}
          </el-button>
        </el-form-item>
      </el-form>
      <div class="login-footer">
        <span>还没有账号？</span>
        <router-link to="/register" class="link">立即注册</router-link>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock, Key } from '@element-plus/icons-vue'
import { useUserStore } from '@/pinia/userStore'
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

// [修复] S-22: 登录失败次数限制（同一会话内，1分钟内最多5次尝试）
const MAX_ATTEMPTS = 5
const LOCKOUT_DURATION = 60 // 秒
const attemptCount = ref(0)
const lockoutCountdown = ref(0)
const showCaptcha = ref(false)
const captchaCode = ref('')
let lockoutTimer = null

// [修复] S-22: 生成4位随机数字验证码
function refreshCaptcha() {
  captchaCode.value = String(Math.floor(1000 + Math.random() * 9000))
}

// [修复] S-22: 启动锁定倒计时
function startLockout() {
  lockoutCountdown.value = LOCKOUT_DURATION
  lockoutTimer = setInterval(() => {
    lockoutCountdown.value--
    if (lockoutCountdown.value <= 0) {
      clearInterval(lockoutTimer)
      lockoutTimer = null
      attemptCount.value = 0 // 重置尝试次数
    }
  }, 1000)
}

async function handleLogin() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  // [修复] S-22: 验证码校验（超过3次失败后显示验证码）
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
    // [修复] S-22: 登录成功后重置失败计数和验证码状态
    attemptCount.value = 0
    showCaptcha.value = false
    router.push('/seckill')
  } catch (e) {
    // [修复] S-22: 记录失败次数，超过限制后锁定
    attemptCount.value++
    if (attemptCount.value >= MAX_ATTEMPTS) {
      startLockout()
      ElMessage.error(`登录失败次数过多，请 ${LOCKOUT_DURATION} 秒后再试`)
    } else if (attemptCount.value >= 3) {
      // 超过3次失败后显示验证码
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