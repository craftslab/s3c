<template>
  <div class="upload-page">
    <div class="upload-card">
      <div class="upload-card-header">
        <el-icon :size="28" color="#409eff"><UploadFilled /></el-icon>
        <span class="upload-card-title">File Upload</span>
      </div>

      <p v-if="targetFilename" class="upload-hint">
        Upload destination: <strong>{{ targetFilename }}</strong>
      </p>

      <template v-if="!done && !expired">
        <!-- Drop zone -->
        <div
          class="drop-zone"
          :class="{ 'drop-zone--over': isDragging, 'drop-zone--disabled': uploading }"
          @dragover.prevent="isDragging = true"
          @dragleave="isDragging = false"
          @drop.prevent="onDrop"
          @click="triggerFileInput"
        >
          <el-icon :size="48" color="#409eff"><UploadFilled /></el-icon>
          <p>Drop a file here or <strong>click</strong> to select</p>
        </div>
        <input ref="fileInputRef" type="file" style="display:none" @change="onFileChange" />

        <!-- Selected file -->
        <div v-if="selectedFile" class="file-info">
          <el-icon><Document /></el-icon>
          <span class="file-name">{{ selectedFile.name }}</span>
          <span class="file-size">{{ formatSize(selectedFile.size) }}</span>
        </div>

        <!-- Progress -->
        <el-progress
          v-if="uploading"
          :percentage="progress"
          :status="progress === 100 ? 'success' : undefined"
          class="progress-bar"
        />

        <el-button
          type="primary"
          :disabled="!selectedFile || uploading"
          :loading="uploading"
          class="upload-btn"
          @click="startUpload"
        >
          Upload
        </el-button>
      </template>

      <!-- Success state -->
      <div v-if="done" class="result result--success">
        <el-icon :size="48" color="#67c23a"><CircleCheck /></el-icon>
        <p>File uploaded successfully!</p>
      </div>

      <!-- Expired / invalid link state -->
      <div v-if="expired" class="result result--error">
        <el-icon :size="48" color="#f56c6c"><CircleClose /></el-icon>
        <p>This upload link is invalid or has expired.</p>
      </div>

      <!-- Error message -->
      <p v-if="errorMsg" class="error-msg">{{ errorMsg }}</p>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { UploadFilled, Document, CircleCheck, CircleClose } from '@element-plus/icons-vue'

const route = useRoute()

const presignedUrl = ref('')
const targetFilename = ref('')

const selectedFile = ref(null)
const isDragging = ref(false)
const uploading = ref(false)
const progress = ref(0)
const done = ref(false)
const expired = ref(false)
const errorMsg = ref('')
const fileInputRef = ref(null)

onMounted(() => {
  presignedUrl.value = route.query.url || ''
  targetFilename.value = route.query.filename || ''
  if (!presignedUrl.value) {
    expired.value = true
  }
})

function triggerFileInput() {
  if (!uploading.value) fileInputRef.value?.click()
}

function onFileChange(e) {
  const f = e.target.files[0]
  if (f) selectedFile.value = f
  e.target.value = ''
}

function onDrop(e) {
  isDragging.value = false
  if (uploading.value) return
  const f = e.dataTransfer.files[0]
  if (f) selectedFile.value = f
}

async function startUpload() {
  if (!selectedFile.value || !presignedUrl.value) return
  uploading.value = true
  progress.value = 0
  errorMsg.value = ''

  try {
    await uploadWithProgress(presignedUrl.value, selectedFile.value, targetFilename.value)
    done.value = true
  } catch (e) {
    if (e.status === 403 || e.status === 401) {
      expired.value = true
    } else {
      errorMsg.value = e.message || 'Upload failed. The link may have expired.'
    }
  } finally {
    uploading.value = false
  }
}

function uploadWithProgress(url, file, filename) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const qs = new URLSearchParams({ url })
    if (filename) qs.set('filename', filename)
    // POST to /upload/ so nginx can proxy it while keeping GET /upload as SPA route.
    xhr.open('POST', `/upload/?${qs.toString()}`)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        progress.value = Math.round((e.loaded / e.total) * 100)
      }
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve()
      } else {
        const err = new Error(`Upload failed: HTTP ${xhr.status}`)
        err.status = xhr.status
        reject(err)
      }
    }
    xhr.onerror = () => reject(new Error('Network error during upload'))
    const form = new FormData()
    form.append('file', file, file.name)
    xhr.send(form)
  })
}

function formatSize(bytes) {
  if (bytes === null || bytes === undefined) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let n = bytes
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}
</script>

<style scoped>
.upload-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f5f7fa;
  padding: 24px;
  box-sizing: border-box;
}

.upload-card {
  background: #fff;
  border-radius: 12px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.08);
  padding: 40px 48px;
  width: 100%;
  max-width: 520px;
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.upload-card-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.upload-card-title {
  font-size: 20px;
  font-weight: 600;
  color: #303133;
}

.upload-hint {
  margin: 0;
  font-size: 14px;
  color: #606266;
}

.drop-zone {
  border: 2px dashed #c0c4cc;
  border-radius: 8px;
  padding: 36px 24px;
  text-align: center;
  cursor: pointer;
  transition: border-color 0.2s, background 0.2s;
}

.drop-zone:hover,
.drop-zone--over {
  border-color: #409eff;
  background: #ecf5ff;
}

.drop-zone--disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.drop-zone p {
  margin: 8px 0 0;
  color: #606266;
  font-size: 14px;
}

.file-info {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #303133;
  padding: 8px 12px;
  background: #f5f7fa;
  border-radius: 6px;
}

.file-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-size {
  color: #909399;
  flex-shrink: 0;
}

.progress-bar {
  margin-top: 4px;
}

.upload-btn {
  width: 100%;
}

.result {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 16px 0;
}

.result p {
  margin: 0;
  font-size: 15px;
  font-weight: 500;
  color: #303133;
}

.error-msg {
  margin: 0;
  font-size: 13px;
  color: #f56c6c;
}
</style>
