import { createRouter, createWebHashHistory } from 'vue-router'
import { routes } from './routes'
import { ElMessage } from 'element-plus'
import { connectWebSocket } from '@/composables/useWebSocket'

// [修复] S-20: 扩展 routes 数组，添加 404 兜底路由
const allRoutes = [
  ...routes,
  // [修复] S-20: 捕获所有未匹配路由，重定向到秒杀首页，避免 404 空白页
  { path: '/:pathMatch(.*)*', redirect: '/seckill' }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes: allRoutes
})

// [修复] 角色权限映射：每个角色可访问的菜单分组（route meta.role 使用分组名）
const roleAccess = {
  super_admin: ['operator', 'risk_control', 'monitor_readonly'],
  admin:       ['operator', 'risk_control', 'monitor_readonly'],
  operator:    ['operator'],
  risk_control:['risk_control'],
  monitor_readonly: ['monitor_readonly'],
  user: []
}

// [修复] 从多个来源获取用户角色（优先级：userInfo.role > token.payload.role > 默认'user'）
function getEffectiveRole() {
  try {
    const userInfo = JSON.parse(localStorage.getItem('userInfo') || '{}')
    if (userInfo.role) return userInfo.role
    const token = localStorage.getItem('token')
    if (token) {
      const payload = JSON.parse(atob(token.split('.')[1]))
      if (payload.role) return payload.role
    }
  } catch (e) { /* 忽略解析错误 */ }
  return 'user'
}

// [修复] S-20: 增强路由守卫 - 添加 token 过期处理和角色权限校验
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')

  // [修复] S-20: 检查 token 是否过期（JWT exp 字段）
  if (token) {
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      const now = Math.floor(Date.now() / 1000)
      if (payload.exp && payload.exp < now) {
        localStorage.clear()
        if (to.path !== '/login' && to.path !== '/register') {
          next('/login')
          return
        }
      }
    } catch (e) {
      localStorage.clear()
      if (to.path !== '/login' && to.path !== '/register') {
        next('/login')
        return
      }
    }
  }

  // 1. 未登录用户只能访问登录/注册页
  if (to.path !== '/login' && to.path !== '/register' && !token) {
    next('/login')
    return
  }

  // 2. 已登录用户访问登录/注册页，直接跳到秒杀首页
  if ((to.path === '/login' || to.path === '/register') && token) {
    next('/seckill')
    return
  }

  // [修复] 3. 角色权限校验：根据路由 meta.role（分组名）检查用户角色
  const requiredRole = to.meta?.role
  if (requiredRole && token) {
    const effectiveRole = getEffectiveRole()
    const allowedRoles = roleAccess[effectiveRole] || []

    if (!allowedRoles.includes(requiredRole)) {
      // [修复] 无权限时弹出提示弹窗，不再静默拦截
      console.warn(`[路由守卫] 角色 ${effectiveRole} 无权限访问 ${to.path}（需要 ${requiredRole}）`)
      ElMessage({
        type: 'warning',
        message: '当前账号无该页面访问权限',
        duration: 3000,
        showClose: true
      })
      // 保持在当前页面，不跳转
      next(false)
      return
    }
  }

  // [创新] 登录后自动建立 WebSocket 连接，接收实时推送
  if (token) {
    connectWebSocket()
  }

  next()
})

export default router