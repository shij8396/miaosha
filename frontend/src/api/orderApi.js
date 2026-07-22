import request from './request'

export const getOrders = (params) => request.get('/api/v1/order/list', { params })
export const getOrderDetail = (orderNo) => request.get(`/api/v1/order/${orderNo}`)
export const cancelOrder = (data) => request.post('/api/v1/order/cancel', data)
export const getAllOrders = (params) => request.get('/api/v1/order/all', { params })
export const exportOrders = (params) => request.get('/api/v1/order/export', { params, responseType: 'blob' })
export const getReconDiff = (params) => request.get('/api/v1/order/recon-diff', { params })
export const fixReconDiff = (id) => request.post('/api/v1/order/recon-fix', { id })
// [修复] 退款 + 批量导入
export const refundOrder = (data) => request.post('/api/v1/order/refund', data)
export const importOrders = (data) => request.post('/api/v1/order/import', data)