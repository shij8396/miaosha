<template>
  <div class="login-page">
    <div class="bg-grid"></div>
    <div class="login-card glass-card fade-in-up">
      <div class="login-header">
        <h1 class="gradient-text">注册账号</h1>
        <p>加入秒杀，抢购超值好货</p>
      </div>
      <el-form :model="form" :rules="rules" ref="formRef" label-width="0" size="large">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名（3-64位）" :prefix-icon="User" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" placeholder="密码（至少6位）" :prefix-icon="Lock" show-password />
        </el-form-item>
        <el-form-item prop="phone">
          <el-input v-model="form.phone" placeholder="手机号" :prefix-icon="Phone" />
        </el-form-item>
        <el-form-item>
          <el-button type="danger" style="width:100%" @click="handleRegister" :loading="loading" size="large">注 册</el-button>
        </el-form-item>
      </el-form>
      <div class="login-footer">
        <span>已有账号？</span>
        <router-link to="/login" class="link">去登录</router-link>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock, Phone } from '@element-plus/icons-vue'
import { useUserStore } from '@/pinia/userStore'
import { ElMessage } from 'element-plus'

const router = useRouter()
const userStore = useUserStore()
const formRef = ref(null)
const loading = ref(false)
const form = reactive({ username: '', password: '', phone: '' })
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }, { min: 3, max: 64, message: '3-64位', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }, { min: 6, max: 64, message: '至少6位', trigger: 'blur' }],
  // [修复] 添加手机号校验规则
  phone: [
    { required: true, message: '请输入手机号', trigger: 'blur' },
    { pattern: /^1[3-9]\d{9}$/, message: '请输入正确的手机号', trigger: 'blur' }
  ]
}

async function handleRegister() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    await userStore.register({ ...form, password: form.password })
    ElMessage.success('注册成功，请登录')
    router.push('/login')
  } catch (e) {
    // [修复] 错误已在拦截器中处理，此处记录日志便于调试
    console.error('注册失败:', e)
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
</style>