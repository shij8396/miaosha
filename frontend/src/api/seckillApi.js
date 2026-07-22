import request from './request'

export const seckill = (data) => request.post('/api/v1/seckill', data)
export const getSeckillStats = () => request.get('/api/v1/seckill/stats')
// [创新] 秒杀地址隐藏：获取动态 Path Token
export const getSeckillPath = (productId) => request.get('/api/v1/seckill/path', { params: { product_id: productId } })
// [创新] 数学验证码：获取随机算式
export const getCaptcha = (productId) => request.get('/api/v1/seckill/captcha', { params: { product_id: productId } })