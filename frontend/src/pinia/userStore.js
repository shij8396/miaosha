import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { login as loginApi, register as registerApi } from '@/api/userApi'

export const useUserStore = defineStore('user', () => {
  // [修复] 从 localStorage 恢复角色权限，确保刷新页面不丢失
  const token = ref(localStorage.getItem('token') || '')
  const userInfo = ref(initUserInfo())
  const permissions = ref(initPermissions())

  // [修复] 初始化 userInfo：优先从 localStorage 恢复，确保刷新后角色不丢失
  function initUserInfo() {
    try {
      const stored = localStorage.getItem('userInfo')
      if (stored) {
        const parsed = JSON.parse(stored)
        // 确保 role 字段存在，如果后端未返回则从 token 中解析
        if (!parsed.role) {
          const token = localStorage.getItem('token')
          if (token) {
            try {
              const payload = JSON.parse(atob(token.split('.')[1]))
              parsed.role = payload.role || 'user'
            } catch (e) { parsed.role = 'user' }
          } else {
            parsed.role = 'user'
          }
        }
        return parsed
      }
    } catch (e) { /* 忽略解析错误 */ }
    return null
  }

  // [修复] 初始化 permissions：确保 admin/super_admin 角色拥有完整权限
  function initPermissions() {
    try {
      const stored = localStorage.getItem('permissions')
      if (stored) {
        const parsed = JSON.parse(stored)
        if (parsed.length > 0) return parsed
      }
    } catch (e) { /* 忽略解析错误 */ }
    // [修复] 根据 userInfo 的 role 自动生成默认权限
    const role = userInfo.value?.role || 'user'
    if (role === 'admin' || role === 'super_admin') {
      return ['admin']
    }
    return ['user']
  }

  const isLoggedIn = computed(() => !!token.value)
  // [修复] isAdmin 判断：同时检查 permissions 和 userInfo.role，确保 admin/super_admin 有完整权限
  const isAdmin = computed(() =>
    permissions.value.includes('admin') ||
    ['admin', 'super_admin'].includes(userInfo.value?.role)
  )
  const username = computed(() => userInfo.value?.username || '')

  /**
   * [修复] 登录逻辑 - 权限持久化策略
   * 1. 优先使用后端返回的 role（data.role）
   * 2. 后端未返回时，保留 localStorage 中已有的角色（防止刷新丢失）
   * 3. 都没有时才默认 'user'
   * 4. admin/super_admin 自动拥有完整菜单权限
   */
  async function login(credentials) {
    const data = await loginApi(credentials)

    // [修复] 构建 userInfo，强制保留 role 字段以保证刷新后权限不丢失
    userInfo.value = {
      user_id: data.user_id,
      username: data.username,
      nickname: data.nickname || '',
      role: data.role || userInfo.value?.role || 'user'
    }

    // [修复] 权限优先级：后端返回 > localStorage已有 > 根据role推断
    if (data.permissions && Array.isArray(data.permissions) && data.permissions.length > 0) {
      permissions.value = data.permissions
    } else {
      const storedPermissions = JSON.parse(localStorage.getItem('permissions') || '[]')
      if (storedPermissions.length > 0) {
        permissions.value = storedPermissions
      } else if (userInfo.value.role === 'admin' || userInfo.value.role === 'super_admin') {
        // [修复] admin/super_admin 自动拥有完整权限
        permissions.value = ['admin']
      } else {
        permissions.value = ['user']
      }
    }

    token.value = data.token
    // [修复] 持久化到 localStorage，确保刷新页面不丢失
    localStorage.setItem('token', data.token)
    localStorage.setItem('userInfo', JSON.stringify(userInfo.value))
    localStorage.setItem('permissions', JSON.stringify(permissions.value))
    return data
  }

  /**
   * [修复] 刷新权限：从 localStorage 重新读取角色信息
   * 用于页面刷新后或手动更新角色后同步状态
   */
  function refreshRole() {
    userInfo.value = initUserInfo()
    permissions.value = initPermissions()
  }

  async function register(form) {
    return await registerApi(form)
  }

  function logout() {
    token.value = ''
    userInfo.value = null
    permissions.value = []
    localStorage.clear()
  }

  return { token, userInfo, permissions, isLoggedIn, isAdmin, username, login, register, logout, refreshRole }
})