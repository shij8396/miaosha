// [创新] WebSocket 实时推送客户端
// 自动重连 + 心跳保持 + 消息分发
// 秒杀结果/订单状态/系统告警实时推送
import { ref, onUnmounted } from 'vue'
import { ElNotification } from 'element-plus'

const WS_URL = `ws://localhost:8080/ws`
const HEARTBEAT_INTERVAL = 30000
const RECONNECT_DELAY = 3000
const MAX_RECONNECT = 5

/** 连接状态 */
export const wsConnected = ref(false)
/** 最新秒杀结果 */
export const lastSeckillResult = ref(null)
/** 最新订单状态 */
export const lastOrderStatus = ref(null)
/** 系统告警列表 */
export const alerts = ref([])

let ws = null
let reconnectCount = 0
let heartbeatTimer = null
let reconnectTimer = null

/** 获取 JWT Token */
function getToken() {
  const token = localStorage.getItem('token')
  return token ? token.replace('Bearer ', '') : ''
}

/** 建立 WebSocket 连接 */
export function connectWebSocket() {
  const token = getToken()
  if (!token) {
    console.warn('[WebSocket] 未登录，跳过连接')
    return
  }

  try {
    ws = new WebSocket(`${WS_URL}?token=${token}`)
  } catch (e) {
    console.error('[WebSocket] 连接失败:', e)
    scheduleReconnect()
    return
  }

  ws.onopen = () => {
    console.log('[WebSocket] 已连接')
    wsConnected.value = true
    reconnectCount = 0
    startHeartbeat()
  }

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data)
      handleMessage(msg)
    } catch (e) {
      console.error('[WebSocket] 消息解析失败:', e)
    }
  }

  ws.onclose = (event) => {
    console.log('[WebSocket] 已断开:', event.code, event.reason)
    wsConnected.value = false
    stopHeartbeat()
    scheduleReconnect()
  }

  ws.onerror = (error) => {
    console.error('[WebSocket] 错误:', error)
    wsConnected.value = false
  }
}

/** 消息处理 */
function handleMessage(msg) {
  switch (msg.type) {
    case 'seckill_result':
      lastSeckillResult.value = { ...msg.data, timestamp: msg.timestamp }
      ElNotification({
        title: '秒杀结果',
        message: msg.data.message || '秒杀结果已更新',
        type: msg.data.status === 'success' ? 'success' : 'warning',
        duration: 5000,
      })
      break

    case 'order_status':
      lastOrderStatus.value = { ...msg.data, timestamp: msg.timestamp }
      ElNotification({
        title: '订单状态变更',
        message: `订单 ${msg.data.order_no} 状态变更为: ${msg.data.status}`,
        type: 'info',
        duration: 4000,
      })
      break

    case 'system_alert':
      alerts.value.unshift({ ...msg.data, timestamp: msg.timestamp })
      if (alerts.value.length > 50) alerts.value.pop()
      ElNotification({
        title: '系统告警',
        message: msg.data.message || '系统状态异常',
        type: 'warning',
        duration: 10000,
      })
      break

    case 'seckill_start':
      ElNotification({
        title: '秒杀活动',
        message: msg.data.message || '秒杀活动已开始',
        type: 'success',
        duration: 5000,
      })
      break

    case 'heartbeat':
      // 心跳响应，无需处理
      break

    default:
      console.log('[WebSocket] 未知消息类型:', msg.type)
  }
}

/** 心跳 */
function startHeartbeat() {
  stopHeartbeat()
  heartbeatTimer = setInterval(() => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: 'heartbeat' }))
    }
  }, HEARTBEAT_INTERVAL)
}

function stopHeartbeat() {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

/** 自动重连 */
function scheduleReconnect() {
  if (reconnectCount >= MAX_RECONNECT) {
    console.warn('[WebSocket] 重连次数已达上限，停止重连')
    return
  }
  reconnectCount++
  console.log(`[WebSocket] ${RECONNECT_DELAY / 1000}s 后第 ${reconnectCount} 次重连...`)
  reconnectTimer = setTimeout(() => {
    connectWebSocket()
  }, RECONNECT_DELAY * reconnectCount)
}

/** 断开连接 */
export function disconnectWebSocket() {
  stopHeartbeat()
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (ws) {
    ws.close(1000, '用户主动断开')
    ws = null
  }
  wsConnected.value = false
}

/** 组件卸载时自动清理 */
export function useWebSocket() {
  onUnmounted(() => {
    disconnectWebSocket()
  })
  return {
    wsConnected,
    lastSeckillResult,
    lastOrderStatus,
    alerts,
    connectWebSocket,
    disconnectWebSocket,
  }
}