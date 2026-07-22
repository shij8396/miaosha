import request from './request'

export const getMetrics = () => request.get('/api/v1/monitor/metrics')
export const getQPS = () => request.get('/api/v1/monitor/qps')
export const getMiddlewareStatus = () => request.get('/api/v1/monitor/middleware')
export const getAlarms = () => request.get('/api/v1/monitor/alarms')
// [修复] 慢接口TOP排行
export const getSlowAPIs = () => request.get('/api/v1/monitor/slow-api')