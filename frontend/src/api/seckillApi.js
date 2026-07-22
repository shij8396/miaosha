import request from './request'

export const seckill = (data) => request.post('/api/v1/seckill', data)
export const getSeckillStats = () => request.get('/api/v1/seckill/stats')