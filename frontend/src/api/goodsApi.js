import request from './request'

export const createProduct = (data) => request.post('/api/v1/product', data)
export const updateProduct = (id, data) => request.put(`/api/v1/product/${id}`, data)
export const getProductList = (params) => request.get('/api/v1/product/list', { params })
export const getActiveProducts = () => request.get('/api/v1/product/active')
export const getProductDetail = (id) => request.get(`/api/v1/product/${id}`)
export const batchImportProducts = (data) => request.post('/api/v1/product/batch', data)
// [修复] 活动配置保存 API（限购数量、价格、时间等）
export const saveActivityConfig = (data) => request.post('/api/v1/activity/config', data)
// [修复] 活动配置查询 + 更新 + 缓存预热
export const getActivity = () => request.get('/api/v1/activity')
export const updateActivity = (data) => request.put('/api/v1/activity', data)
export const cacheWarmUp = () => request.post('/api/v1/activity/cache-warmup')
// [修复] 商品图片上传 API
export const uploadImage = (file) => {
  const formData = new FormData()
  formData.append('file', file)
  return request.post('/api/v1/product/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}
// [修复] 审计日志查询
export const getAuditLogs = (params) => request.get('/api/v1/audit/list', { params })