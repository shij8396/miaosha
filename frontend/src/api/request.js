import axios from 'axios'
import { ElMessage } from 'element-plus'
import CryptoJS from 'crypto-js'

// [修复] API 签名配置
const SIGN_SECRET = import.meta.env.VITE_SIGN_SECRET || 'miaosha-sign-secret-2026'
const SIGN_ENABLED = import.meta.env.VITE_SIGN_ENABLED !== 'false'

// [修复] 生成 HMAC-SHA256 签名：Sign = HMAC-SHA256(Timestamp + Method + Path + Body, Secret)
function generateSign(timestamp, method, path, body) {
  const payload = timestamp + method + path + (body || '')
  return CryptoJS.HmacSHA256(payload, SIGN_SECRET).toString(CryptoJS.enc.Hex)
}

// [修复] S-21: baseURL 从环境变量读取，开发环境为空（使用 Vite proxy），生产环境为 /api（Nginx 代理）
const request = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '',
  timeout: 15000
})

// [修复] 任务3: 全局 loading 计数器，用于 LayoutView 显示进度条
let loadingCount = 0
// [修复] 任务3: 全局 loading 状态变更回调，由 LayoutView 注册
let onLoadingChange = null
export function setLoadingChangeCallback(cb) { onLoadingChange = cb }
function showLoading() { loadingCount++; if (onLoadingChange) onLoadingChange(true) }
function hideLoading() { loadingCount = Math.max(0, loadingCount - 1); if (loadingCount === 0 && onLoadingChange) onLoadingChange(false) }

request.interceptors.request.use(config => {
  /* [修复] 任务3: 排除不需要 loading 的请求类型（导出下载等） */
  if (config.responseType !== 'blob') {
    showLoading()
  }
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  // [修复] API 签名：为所有非白名单请求添加 HMAC-SHA256 签名头
  // [修复] FormData 请求跳过签名（multipart body 无法正确序列化）
  if (SIGN_ENABLED && !(config.data instanceof FormData)) {
    const timestamp = Math.floor(Date.now() / 1000).toString()
    const method = config.method.toUpperCase()
    const path = (config.url || '').replace(config.baseURL || '', '').split('?')[0]
    const body = config.data ? (typeof config.data === 'string' ? config.data : JSON.stringify(config.data)) : ''
    config.headers['X-Timestamp'] = timestamp
    config.headers['X-Sign'] = generateSign(timestamp, method, path, body)
  }
  return config
})

request.interceptors.response.use(
  response => {
    if (response.config.responseType !== 'blob') {
      hideLoading()
    }
    if (response.config.responseType === 'blob') {
      return response
    }
    const { code, message, data } = response.data
    if (code === 200) return data
    if (code === 401 && (message.includes('Token') || message.includes('token'))) {
      localStorage.clear()
      window.location.hash = '#/login'
      ElMessage.error('登录已过期，请重新登录')
      return Promise.reject(new Error(message))
    }
    return Promise.reject(new Error(message))
  },
  error => {
    if (error.config?.responseType !== 'blob') {
      hideLoading()
    }
    if (error.response?.status === 401) {
      localStorage.clear()
      window.location.hash = '#/login'
      ElMessage.error('登录已过期')
      return Promise.reject(error)
    }
    const businessMsg = error.response?.data?.message
    const displayMsg = businessMsg || error.message || '网络错误'
    return Promise.reject(new Error(displayMsg))
  }
)

export default request