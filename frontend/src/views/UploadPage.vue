<template>
  <div class="upload-page">
    <div class="upload-card">
      <div class="upload-shell-copy">
        <p class="upload-shell-eyebrow">Kipup upload portal / Kipup 上传入口</p>
        <h1 class="upload-shell-title">Drop in a file and ship it with confidence.</h1>
        <p class="upload-shell-subtitle">A quieter upload flow, with calmer copy in both English and Chinese. / 用更统一的中英文文案，完成一次更从容的上传。</p>
      </div>
      <div class="upload-card-header">
        <el-icon :size="28" color="#201912"><UploadFilled /></el-icon>
        <span class="upload-card-title">File upload / 文件上传</span>
      </div>

      <p v-if="targetFilename" class="upload-hint">
        Upload destination / 上传目标：<strong>{{ targetFilename }}</strong>
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
          <el-icon :size="48" color="#201912"><UploadFilled /></el-icon>
          <p>Drop a file here or <strong>click</strong> to select / 拖拽文件到这里，或点击选择</p>
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
          Upload / 上传
        </el-button>
      </template>

      <!-- Success state -->
      <div v-if="done" class="result result--success">
        <el-icon :size="48" color="#67c23a"><CircleCheck /></el-icon>
        <p>File uploaded successfully. / 文件上传成功。</p>
      </div>

      <!-- Expired / invalid link state -->
      <div v-if="expired" class="result result--error">
        <el-icon :size="48" color="#f56c6c"><CircleClose /></el-icon>
        <p>This upload link is invalid or has expired. / 上传链接无效或已过期。</p>
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
      errorMsg.value = e.message || 'Upload failed. The link may have expired. / 上传失败，链接可能已过期。'
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
    // Use /api proxy route to avoid special-case nginx handling of /upload.
    xhr.open('POST', `/api/upload?${qs.toString()}`)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        progress.value = Math.round((e.loaded / e.total) * 100)
      }
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve()
      } else {
        const err = new Error(`Upload failed (HTTP ${xhr.status}) / 上传失败 (HTTP ${xhr.status})`)
        err.status = xhr.status
        reject(err)
      }
    }
    xhr.onerror = () => reject(new Error('Network error during upload / 上传过程中网络异常'))
    if (file.type) xhr.setRequestHeader('Content-Type', file.type)
    xhr.send(file)
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
  background:
    radial-gradient(circle at top, rgba(237, 226, 210, 0.82), transparent 42%),
    #f4efe6;
  padding: 32px;
  box-sizing: border-box;
}

.upload-card {
  background: rgba(255, 252, 245, 0.88);
  border: 1px solid rgba(69, 54, 42, 0.12);
  border-radius: 30px;
  box-shadow: 0 24px 80px rgba(59, 43, 31, 0.08);
  padding: 48px 52px;
  width: 100%;
  max-width: 660px;
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.upload-shell-copy {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.upload-shell-eyebrow {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #8b7f72;
}

.upload-shell-title {
  margin: 0;
  font-family: Iowan Old Style, Palatino Linotype, Book Antiqua, Georgia, serif;
  font-size: 42px;
  font-weight: 600;
  letter-spacing: -0.04em;
  line-height: 1.1;
  color: #201912;
}

.upload-shell-subtitle {
  max-width: 520px;
  margin: 2px 0 0;
  color: #6f6256;
  font-size: 16px;
  line-height: 1.7;
}

.upload-card-header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-top: 10px;
}

.upload-card-title {
  font-size: 20px;
  font-weight: 600;
  color: #201912;
}

.upload-hint {
  margin: 0;
  font-size: 14px;
  color: #5c5146;
}

.drop-zone {
  border: 1px dashed rgba(69, 54, 42, 0.22);
  border-radius: 24px;
  padding: 42px 24px;
  text-align: center;
  cursor: pointer;
  background: rgba(237, 226, 210, 0.24);
  transition: 0.2s ease;
}

.drop-zone:hover,
.drop-zone--over {
  border-color: rgba(32, 25, 18, 0.28);
  background: rgba(237, 226, 210, 0.46);
}

.drop-zone--disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.drop-zone p {
  margin: 8px 0 0;
  color: #5c5146;
  font-size: 15px;
}

.file-info {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #201912;
  padding: 12px 14px;
  background: rgba(237, 226, 210, 0.28);
  border: 1px solid rgba(69, 54, 42, 0.08);
  border-radius: 18px;
}

.file-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-size {
  color: #8b7f72;
  flex-shrink: 0;
}

.progress-bar {
  margin-top: 6px;
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
  color: #201912;
}

.error-msg {
  margin: 0;
  font-size: 13px;
  color: #f56c6c;
}

:deep(.el-progress-bar__inner) {
  background: linear-gradient(90deg, #201912, #5d4836);
}
</style>
