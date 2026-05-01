<template>
  <div class="mobile-apps-page">
    <section class="page-hero">
      <div>
        <p class="eyebrow">Mobile distribution</p>
        <h2>Kipup mobile app releases</h2>
        <p class="subtitle">Create expiring Android / iOS release records, copy hosted download pages, and revoke installed clients.</p>
      </div>
      <el-button @click="refreshAll">Refresh</el-button>
    </section>

    <el-alert
      v-if="!isAdmin"
      title="Only administrators can manage mobile app releases."
      type="warning"
      show-icon
      :closable="false"
    />

    <template v-else>
      <section class="page-grid">
        <el-card shadow="never">
          <template #header><span>Create mobile app release</span></template>
          <el-form label-width="140px">
            <el-form-item label="Title">
              <el-input v-model="form.title" placeholder="Kipup Collaboration App" />
            </el-form-item>
            <el-form-item label="Version">
              <el-input v-model="form.version" placeholder="1.0.0" />
            </el-form-item>
            <el-form-item label="Platform">
              <el-select v-model="form.platform" style="width: 100%">
                <el-option label="Android APK" value="android" />
                <el-option label="iOS IPA" value="ios" />
              </el-select>
            </el-form-item>
            <el-form-item label="Bucket">
              <el-select v-model="form.bucket" filterable style="width: 100%">
                <el-option v-for="bucket in buckets" :key="bucket.name" :label="bucket.name" :value="bucket.name" />
              </el-select>
            </el-form-item>
            <el-form-item label="Object key">
              <el-input v-model="form.objectKey" placeholder="mobile/kipup.apk" />
            </el-form-item>
            <el-form-item label="Collaboration room">
              <el-select v-model="form.collaborationToken" clearable filterable style="width: 100%" placeholder="Optional linked room">
                <el-option
                  v-for="item in availableCollaborations"
                  :key="item.token"
                  :label="`${item.title} (${item.token})`"
                  :value="item.token"
                />
              </el-select>
            </el-form-item>
            <el-form-item label="App expires at">
              <el-date-picker
                v-model="form.expiresAt"
                type="datetime"
                style="width: 100%"
                placeholder="Select expiry time"
              />
            </el-form-item>
            <el-form-item label="Offline grace (h)">
              <el-input-number v-model="form.offlineGraceHours" :min="1" :max="168" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :loading="creating" @click="createReleaseAction">Create release</el-button>
            </el-form-item>
          </el-form>
        </el-card>

        <el-card shadow="never">
          <template #header><span>Release guidance</span></template>
          <ul class="guidance-list">
            <li>Install package files stay in S3/MinIO and are exposed only through signed release links.</li>
            <li>Each release must expire, and installed clients will be blocked after expiry or revocation.</li>
            <li>Link a release to a collaboration room when mobile access should stop after the room closes.</li>
            <li>Use the activation code on the download page when opening the Flutter app for the first time.</li>
          </ul>
        </el-card>
      </section>

      <el-card shadow="never" class="release-card">
        <template #header><span>Published releases</span></template>
        <el-table v-loading="loading" :data="releases" size="small">
          <el-table-column prop="title" label="Title" min-width="180" />
          <el-table-column label="Platform / version" width="150">
            <template #default="{ row }">
              <div>{{ row.platform }}</div>
              <div class="small-text">{{ row.version }}</div>
            </template>
          </el-table-column>
          <el-table-column label="Package" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">
              <div>{{ row.bucket }}/{{ row.objectKey }}</div>
              <div class="small-text">{{ formatSize(row.size) }}</div>
            </template>
          </el-table-column>
          <el-table-column label="Room" min-width="170" show-overflow-tooltip>
            <template #default="{ row }">
              <span>{{ row.collaborationTitle || '—' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="Status" width="130">
            <template #default="{ row }">
              <el-tag :type="row.status === 'revoked' || row.expired ? 'danger' : 'success'">
                {{ row.status === 'revoked' ? 'revoked' : row.expired ? 'expired' : 'active' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Expires" width="190">
            <template #default="{ row }">{{ formatDate(row.expiresAt) }}</template>
          </el-table-column>
          <el-table-column label="Actions" width="290" fixed="right">
            <template #default="{ row }">
              <el-button size="small" @click="openLinkDialog(row)">Create link</el-button>
              <el-button size="small" @click="openInstallationsDialog(row)">Installs</el-button>
              <el-button v-if="row.status !== 'revoked'" size="small" type="danger" plain @click="revokeReleaseAction(row)">Revoke</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <el-dialog v-model="showLinkDialog" title="Generate download link" width="560px">
      <el-form label-width="120px">
        <el-form-item label="Release">
          <span class="link-meta">{{ selectedRelease?.title }} · {{ selectedRelease?.version }}</span>
        </el-form-item>
        <el-form-item label="Link expires at">
          <el-date-picker v-model="linkExpiresAt" type="datetime" style="width: 100%" />
        </el-form-item>
      </el-form>
      <div v-if="generatedLink" class="generated-link">
        <el-form label-width="120px">
          <el-form-item label="Download page">
            <el-input :model-value="generatedLink.downloadPageUrl" readonly>
              <template #append><el-button @click="copyToClipboard(generatedLink.downloadPageUrl)">Copy</el-button></template>
            </el-input>
          </el-form-item>
          <el-form-item label="Activation code">
            <el-input :model-value="generatedLink.token" readonly>
              <template #append><el-button @click="copyToClipboard(generatedLink.token)">Copy</el-button></template>
            </el-input>
          </el-form-item>
        </el-form>
      </div>
      <template #footer>
        <el-button @click="showLinkDialog = false">Close</el-button>
        <el-button type="primary" :loading="generatingLink" @click="generateLinkAction">Generate</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showInstallationsDialog" title="Installed clients" width="820px">
      <el-table :data="installations" size="small">
        <el-table-column prop="deviceName" label="Device" min-width="180">
          <template #default="{ row }">
            <div>{{ row.deviceName || 'Unnamed device' }}</div>
            <div class="small-text">{{ row.deviceId }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="appVersion" label="App version" width="120" />
        <el-table-column prop="status" label="Status" width="120" />
        <el-table-column label="Activated" width="180">
          <template #default="{ row }">{{ formatDate(row.activatedAt) }}</template>
        </el-table-column>
        <el-table-column label="Last validated" width="180">
          <template #default="{ row }">{{ formatDate(row.lastValidatedAt) }}</template>
        </el-table-column>
        <el-table-column label="Actions" width="120">
          <template #default="{ row }">
            <el-button v-if="row.status !== 'revoked'" size="small" type="danger" plain @click="revokeInstallationAction(row)">Revoke</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createMobileAppDownloadLink,
  createMobileAppRelease,
  listBuckets,
  listCollaborationSessions,
  listMobileAppInstallations,
  listMobileAppReleases,
  revokeMobileAppInstallation,
  revokeMobileAppRelease
} from '../api'
import { useAuth } from '../auth'

const { isAdmin } = useAuth()

const loading = ref(false)
const creating = ref(false)
const generatingLink = ref(false)
const buckets = ref([])
const releases = ref([])
const collaborationSessions = ref([])
const installations = ref([])
const selectedRelease = ref(null)
const showLinkDialog = ref(false)
const showInstallationsDialog = ref(false)
const generatedLink = ref(null)
const linkExpiresAt = ref(new Date(Date.now() + 24 * 60 * 60 * 1000))

const form = ref({
  title: 'Kipup Collaboration App',
  version: '1.0.0',
  platform: 'android',
  bucket: '',
  objectKey: 'mobile/kipup.apk',
  collaborationToken: '',
  expiresAt: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000),
  offlineGraceHours: 24
})

const availableCollaborations = computed(() =>
  (collaborationSessions.value || []).filter((item) => !form.value.bucket || item.bucket === form.value.bucket)
)

onMounted(() => {
  if (isAdmin.value) {
    void refreshAll()
  }
})

async function refreshAll() {
  loading.value = true
  try {
    const [{ data: releaseItems }, { data: bucketItems }, { data: collaborationItems }] = await Promise.all([
      listMobileAppReleases(),
      listBuckets(),
      listCollaborationSessions()
    ])
    releases.value = releaseItems || []
    buckets.value = bucketItems || []
    collaborationSessions.value = collaborationItems || []
    if (!form.value.bucket && buckets.value.length) {
      form.value.bucket = buckets.value[0].name
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  } finally {
    loading.value = false
  }
}

async function createReleaseAction() {
  if (!form.value.title.trim()) return ElMessage.warning('Title is required')
  if (!form.value.version.trim()) return ElMessage.warning('Version is required')
  if (!form.value.bucket) return ElMessage.warning('Bucket is required')
  if (!form.value.objectKey.trim()) return ElMessage.warning('Object key is required')
  if (!form.value.expiresAt) return ElMessage.warning('Expiry time is required')
  creating.value = true
  try {
    await createMobileAppRelease({
      title: form.value.title,
      version: form.value.version,
      platform: form.value.platform,
      bucket: form.value.bucket,
      objectKey: form.value.objectKey,
      collaborationToken: form.value.collaborationToken,
      expiresAt: new Date(form.value.expiresAt).toISOString(),
      offlineGraceSeconds: Number(form.value.offlineGraceHours || 24) * 3600
    })
    ElMessage.success('Mobile app release created')
    await refreshAll()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  } finally {
    creating.value = false
  }
}

function openLinkDialog(release) {
  selectedRelease.value = release
  generatedLink.value = null
  linkExpiresAt.value = new Date(Math.min(new Date(release.expiresAt).getTime(), Date.now() + 24 * 60 * 60 * 1000))
  showLinkDialog.value = true
}

async function generateLinkAction() {
  if (!selectedRelease.value) return
  generatingLink.value = true
  try {
    const { data } = await createMobileAppDownloadLink(selectedRelease.value.id, {
      expiresAt: new Date(linkExpiresAt.value).toISOString()
    })
    generatedLink.value = data
    ElMessage.success('Download link generated')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  } finally {
    generatingLink.value = false
  }
}

async function openInstallationsDialog(release) {
  selectedRelease.value = release
  try {
    const { data } = await listMobileAppInstallations(release.id)
    installations.value = data || []
    showInstallationsDialog.value = true
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function revokeReleaseAction(release) {
  try {
    await ElMessageBox.confirm(`Revoke release "${release.title}"?`, 'Revoke Release', { type: 'warning' })
    await revokeMobileAppRelease(release.id)
    ElMessage.success('Release revoked')
    await refreshAll()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.error || error.message)
    }
  }
}

async function revokeInstallationAction(installation) {
  try {
    await ElMessageBox.confirm(`Revoke device ${installation.deviceId}?`, 'Revoke Installation', { type: 'warning' })
    await revokeMobileAppInstallation(installation.id)
    ElMessage.success('Installation revoked')
    if (selectedRelease.value) {
      await openInstallationsDialog(selectedRelease.value)
    }
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error(error.response?.data?.error || error.message)
    }
  }
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

function formatSize(value) {
  const size = Number(value || 0)
  if (!size) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let current = size
  let index = 0
  while (current >= 1024 && index < units.length - 1) {
    current /= 1024
    index += 1
  }
  return `${current.toFixed(current >= 10 || index === 0 ? 0 : 1)} ${units[index]}`
}
</script>

<style scoped>
.mobile-apps-page {
  padding: 28px 0 40px;
  display: grid;
  gap: 20px;
}

.page-hero,
.page-grid {
  display: grid;
  gap: 20px;
}

.page-hero {
  grid-template-columns: 1fr auto;
  align-items: center;
}

.page-grid {
  grid-template-columns: minmax(0, 1.2fr) minmax(280px, 0.8fr);
}

.eyebrow,
.subtitle,
.small-text,
.guidance-list {
  color: var(--kip-text-muted);
}

.eyebrow {
  margin: 0 0 8px;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  font-size: 12px;
}

.page-hero h2,
.guidance-list {
  margin: 0;
}

.subtitle {
  margin: 8px 0 0;
}

.release-card,
.generated-link {
  margin-top: 4px;
}

.guidance-list {
  padding-left: 18px;
  line-height: 1.8;
}

.link-meta {
  word-break: break-all;
}

@media (max-width: 960px) {
  .page-hero,
  .page-grid {
    grid-template-columns: 1fr;
  }
}
</style>
