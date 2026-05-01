<template>
  <div class="mobile-download-page" v-loading="loading">
    <el-card class="download-card" shadow="never">
      <template v-if="link">
        <p class="eyebrow">Mobile download</p>
        <h1>{{ link.release.title }}</h1>
        <p class="subtitle">
          {{ link.release.platform }} · {{ link.release.version }}
          <span v-if="link.release.collaborationTitle">· {{ link.release.collaborationTitle }}</span>
        </p>

        <div class="meta-grid">
          <div class="meta-item">
            <span>Package</span>
            <strong>{{ link.release.fileName }}</strong>
          </div>
          <div class="meta-item">
            <span>App expires</span>
            <strong>{{ formatDate(link.release.expiresAt) }}</strong>
          </div>
          <div class="meta-item">
            <span>Link expires</span>
            <strong>{{ formatDate(link.expiresAt) }}</strong>
          </div>
          <div class="meta-item">
            <span>Offline grace</span>
            <strong>{{ Math.round((link.release.offlineGraceSeconds || 0) / 3600) }} h</strong>
          </div>
        </div>

        <el-alert type="info" :closable="false" show-icon>
          <template #title>
            Android can usually install directly after download. iOS normally requires TestFlight, enterprise signing, or MDM.
          </template>
        </el-alert>

        <div class="actions">
          <el-button type="primary" @click="downloadBinary">Download app</el-button>
          <el-button @click="copyToClipboard(link.token)">Copy activation code</el-button>
        </div>

        <el-form label-width="130px" class="activation-form">
          <el-form-item label="Activation code">
            <el-input :model-value="link.token" readonly>
              <template #append><el-button @click="copyToClipboard(link.token)">Copy</el-button></template>
            </el-input>
          </el-form-item>
        </el-form>

        <ol class="steps">
          <li>Download and install the app package for your platform.</li>
          <li>Open the Flutter app and paste the activation code above on first launch.</li>
          <li>The app revalidates startup access on every launch and clears local data after expiry or revocation.</li>
        </ol>
      </template>
      <el-empty v-else-if="!loading" description="Mobile download link unavailable" :image-size="80" />
    </el-card>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { getMobileAppDownloadLink } from '../api'

const route = useRoute()
const loading = ref(true)
const link = ref(null)

onMounted(() => {
  void loadLink()
})

async function loadLink() {
  loading.value = true
  try {
    const { data } = await getMobileAppDownloadLink(route.params.token)
    link.value = data
  } catch (error) {
    link.value = null
    ElMessage.error(error.response?.data?.error || error.message)
  } finally {
    loading.value = false
  }
}

function downloadBinary() {
  if (!link.value) return
  window.location.href = `/api/v1/mobile/download-links/${encodeURIComponent(link.value.token)}/file`
}

async function copyToClipboard(value) {
  try {
    await navigator.clipboard.writeText(value)
    ElMessage.success('Copied')
  } catch {
    ElMessage.error('Copy failed')
  }
}

function formatDate(value) {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString()
}
</script>

<style scoped>
.mobile-download-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  box-sizing: border-box;
}

.download-card {
  width: min(760px, 100%);
}

.eyebrow,
.subtitle,
.steps,
.meta-item span {
  color: var(--kip-text-muted);
}

.eyebrow {
  margin: 0 0 8px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  font-size: 12px;
}

h1 {
  margin: 0;
}

.subtitle {
  margin: 10px 0 0;
}

.meta-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin: 20px 0;
}

.meta-item {
  border: 1px solid var(--kip-border);
  border-radius: 16px;
  padding: 16px;
  display: grid;
  gap: 6px;
}

.actions {
  display: flex;
  gap: 12px;
  margin: 20px 0 8px;
}

.activation-form {
  margin-top: 12px;
}

.steps {
  line-height: 1.8;
}

@media (max-width: 720px) {
  .meta-grid {
    grid-template-columns: 1fr;
  }

  .actions {
    flex-direction: column;
  }
}
</style>
