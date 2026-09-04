import request from './request'

export const login = (data) => request.post('/api/v1/user/login', data)
export const register = (data) => request.post('/api/v1/user/register', data)
export const getUserInfo = () => request.get('/api/v1/user/info')
export const updateUserRole = (data) => request.put('/api/v1/user/role', data)
export const getUserList = (params) => request.get('/api/v1/user/list', { params })
export const changePassword = (data) => request.put('/api/v1/user/password', data)
export const forgotPassword = (data) => request.post('/api/v1/user/forgot-password', data)