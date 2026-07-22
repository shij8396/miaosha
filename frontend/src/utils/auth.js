/**
 * [修复] 权限工具函数
 * 问题：原 isAdmin 只检查 permissions 数组，未检查 userInfo.role 字段
 * 修复：统一检查逻辑，与 router/index.js 和 userStore 保持一致
 * 注意：此文件仅作为工具函数供非响应式场景使用，组件内优先使用 userStore.isAdmin
 */

export function hasPermission(permission) {
  const permissions = JSON.parse(localStorage.getItem('permissions') || '[]')
  const userInfo = JSON.parse(localStorage.getItem('userInfo') || 'null')
  return permissions.includes(permission) || permissions.includes('admin') || userInfo?.role === 'admin'
}

export function isAdmin() {
  return hasPermission('admin')
}