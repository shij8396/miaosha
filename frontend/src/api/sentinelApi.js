import request from './request'

export const getRules = () => request.get('/api/v1/sentinel/rules')
export const addRule = (data) => request.post('/api/v1/sentinel/rule', data)
export const updateRule = (id, data) => request.put(`/api/v1/sentinel/rule/${id}`, data)
export const deleteRule = (id) => request.delete(`/api/v1/sentinel/rule/${id}`)
export const getBlacklist = () => request.get('/api/v1/sentinel/blacklist')
export const addBlacklist = (data) => request.post('/api/v1/sentinel/blacklist', data)
export const removeBlacklist = (id) => request.delete(`/api/v1/sentinel/blacklist/${id}`)