import axios from 'axios'

export const AUTH_TOKEN_STORAGE_KEY = 'kipup-auth-token'

const api = axios.create({ baseURL: '/api/v1' })

api.interceptors.request.use((config) => {
  if (typeof window !== 'undefined') {
    const token = window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)
    if (token) {
      config.headers = config.headers || {}
      config.headers.Authorization = `Bearer ${token}`
    }
  }
  return config
})

export const signUp = (payload) => api.post('/auth/sign-up', payload)
export const signIn = (payload) => api.post('/auth/sign-in', payload)
export const signOut = () => api.post('/auth/sign-out')
export const getCurrentUser = () => api.get('/auth/me')
export const listUsers = () => api.get('/users')
export const createTemporaryUser = (payload) => api.post('/users/temp', payload)
export const updateUser = (username, payload) => api.put(`/users/${encodeURIComponent(username)}`, payload)
export const deleteUserAccount = (username) => api.delete(`/users/${encodeURIComponent(username)}`)

export const listBuckets = () => api.get('/buckets')
export const createBucket = (name, region = 'us-east-1') => api.post('/buckets', { name, region })
export const deleteBucket = (bucket) => api.delete(`/buckets/${encodeURIComponent(bucket)}`)

export const listObjects = (bucket, prefix = '') =>
  api.get(`/objects/${encodeURIComponent(bucket)}`, { params: { prefix } })

export const searchObjects = (bucket, filters = {}) =>
  api.get(`/search/${encodeURIComponent(bucket)}`, { params: filters })

export const downloadObject = (bucket, key) =>
  api.get(`/objects/${encodeURIComponent(bucket)}/${encodeURIComponent(key)}`, { responseType: 'blob' })

export const uploadObjects = (bucket, files, prefix = '', onProgress, taskId) => {
  const form = new FormData()
  for (const file of files) {
    form.append('file', file)
  }
  return api.post(`/objects/${encodeURIComponent(bucket)}`, form, {
    params: { prefix },
    headers: {
      'Content-Type': 'multipart/form-data',
      ...(taskId ? { 'X-Task-ID': taskId } : {})
    },
    onUploadProgress: onProgress
  })
}

export const initResumableUpload = (bucket, payload, prefix = '') =>
  api.post(`/uploads/${encodeURIComponent(bucket)}/resumable/init`, payload, { params: { prefix } })

export const getResumableUploadStatus = (bucket, key, uploadId, prefix = '') =>
  api.get(`/uploads/${encodeURIComponent(bucket)}/resumable/status`, {
    params: { key, uploadId, prefix }
  })

export const uploadResumablePart = (bucket, key, uploadId, partNumber, chunk, prefix = '', onProgress) =>
  api.put(`/uploads/${encodeURIComponent(bucket)}/resumable/part`, chunk, {
    params: { key, uploadId, partNumber, prefix },
    headers: { 'Content-Type': 'application/octet-stream' },
    onUploadProgress: onProgress
  })

export const completeResumableUpload = (bucket, payload, prefix = '') =>
  api.post(`/uploads/${encodeURIComponent(bucket)}/resumable/complete`, payload, { params: { prefix } })

export const abortResumableUpload = (bucket, key, uploadId, prefix = '') =>
  api.delete(`/uploads/${encodeURIComponent(bucket)}/resumable`, {
    params: { key, uploadId, prefix }
  })

export const deleteObject = (bucket, key) =>
  api.delete(`/objects/${encodeURIComponent(bucket)}/${encodeURIComponent(key)}`)

export const batchDelete = (bucket, keys, taskId) =>
  api.post(`/operations/${encodeURIComponent(bucket)}/delete`, { keys, taskId })

export const batchMove = (bucket, items, taskId) =>
  api.post(`/operations/${encodeURIComponent(bucket)}/move`, { items, taskId })

export const batchRename = (bucket, items, taskId) =>
  api.post(`/operations/${encodeURIComponent(bucket)}/rename`, { items, taskId })

export const batchDownload = (bucket, keys) =>
  api.post(`/operations/${encodeURIComponent(bucket)}/download`, { keys }, { responseType: 'blob' })

export const listTasks = (params = {}) => api.get('/tasks', { params })
export const listHistory = (params = {}) => api.get('/history', { params })

export const listCleanupPolicies = () => api.get('/cleanup-policies')
export const createCleanupPolicy = (payload) => api.post('/cleanup-policies', payload)
export const updateCleanupPolicy = (id, payload) => api.put(`/cleanup-policies/${encodeURIComponent(id)}`, payload)
export const deleteCleanupPolicy = (id) => api.delete(`/cleanup-policies/${encodeURIComponent(id)}`)
export const runCleanupPolicy = (id) => api.post(`/cleanup-policies/${encodeURIComponent(id)}/run`)

export const listWebhooks = () => api.get('/webhooks')
export const createWebhook = (payload) => api.post('/webhooks', payload)
export const updateWebhook = (id, payload) => api.put(`/webhooks/${encodeURIComponent(id)}`, payload)
export const deleteWebhook = (id) => api.delete(`/webhooks/${encodeURIComponent(id)}`)
export const listWebhookDeliveries = () => api.get('/webhook-deliveries')

export const generateDownloadLink = (bucket, key, expirySeconds = 86400) =>
  api.get(`/presign/download/${encodeURIComponent(bucket)}/${encodeURIComponent(key)}`, {
    params: { expiry: expirySeconds }
  })

export const generateUploadLink = (bucket, key, expirySeconds = 86400) =>
  api.get(`/presign/upload/${encodeURIComponent(bucket)}/${encodeURIComponent(key)}`, {
    params: { expiry: expirySeconds }
  })

export const listCollaborationSessions = () => api.get('/collaboration/sessions')

export const createCollaborationSession = (payload) => api.post('/collaboration/sessions', payload)

export const getCollaborationSession = (token) => api.get(`/collaboration/sessions/${encodeURIComponent(token)}`)

export const updateCollaborationSession = (token, payload) =>
  api.put(`/collaboration/sessions/${encodeURIComponent(token)}`, payload)

export const closeCollaborationSession = (token) =>
  api.post(`/collaboration/sessions/${encodeURIComponent(token)}/close`)

export const deleteCollaborationSession = (token) =>
  api.delete(`/collaboration/sessions/${encodeURIComponent(token)}`)

export const createCollaborationMessage = (token, payload) =>
  api.post(`/collaboration/sessions/${encodeURIComponent(token)}/messages`, payload)

export const createCollaborationAttachment = (token, file) => {
  const form = new FormData()
  form.append('file', file)
  return api.post(`/collaboration/sessions/${encodeURIComponent(token)}/attachments`, form, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
}

export const deleteCollaborationAttachment = (token, attachmentId) =>
  api.delete(`/collaboration/sessions/${encodeURIComponent(token)}/attachments/${encodeURIComponent(attachmentId)}`)

export const createCollaborationSharedFile = (token, payload) =>
  api.post(`/collaboration/sessions/${encodeURIComponent(token)}/files`, payload)

export const deleteCollaborationSharedFile = (token, fileId) =>
  api.delete(`/collaboration/sessions/${encodeURIComponent(token)}/files/${encodeURIComponent(fileId)}`)

export const createCollaborationStreamToken = (token) =>
  api.post(`/collaboration/sessions/${encodeURIComponent(token)}/stream-token`)

export const publishCollaborationSignal = (token, payload) =>
  api.post(`/collaboration/sessions/${encodeURIComponent(token)}/signal`, payload)
