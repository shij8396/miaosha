import request from './request'

export const getMetrics = () => request.get('/api/v1/monitor/metrics')
export const getQPS = () => request.get('/api/v1/monitor/qps')
export const getMiddlewareStatus = () => request.get('/api/v1/monitor/middleware')
export const getAlarms = () => request.get('/api/v1/monitor/alarms')
// [修复] 慢接口TOP排行
export const getSlowAPIs = () => request.get('/api/v1/monitor/slow-api')
// [增强] 数据大屏补全：实时流量/热销排行/库存状态
export const getPVUV = () => request.get('/api/v1/monitor/pvuv')
export const getHotProducts = (top = 10) => request.get(`/api/v1/monitor/hot-products?top=${top}`)
export const getInventory = () => request.get('/api/v1/monitor/inventory')