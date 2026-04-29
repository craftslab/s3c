<template>
  <div class="browser-layout">
    <aside class="sidebar">
      <div class="sidebar-intro">
        <p class="sidebar-eyebrow">Storage index</p>
        <p class="sidebar-copy">A curated view of every bucket, ready for upload, cleanup, and sharing.</p>
      </div>
      <div class="sidebar-header">
        <span class="sidebar-title">Buckets</span>
        <el-button circle type="primary" :icon="Plus" size="small" title="Create bucket" @click="openCreateBucket" />
      </div>
      <el-scrollbar class="sidebar-scroll">
        <ul class="bucket-list">
          <li
            v-for="b in buckets"
            :key="b.name"
            class="bucket-item"
            :class="{ active: b.name === currentBucket }"
            @click="selectBucket(b.name)"
          >
            <el-icon><Coin /></el-icon>
            <span class="bucket-name">{{ b.name }}</span>
          </li>
        </ul>
        <el-empty v-if="!buckets.length" description="No buckets" :image-size="60" />
      </el-scrollbar>
      <div class="sidebar-footer">
        <span>{{ buckets.length }} bucket{{ buckets.length === 1 ? '' : 's' }}</span>
      </div>
    </aside>

    <div class="main-area">
      <section class="workspace-intro">
        <div class="workspace-copy">
          <p class="workspace-eyebrow">{{ workspaceEyebrow }}</p>
          <h2 class="workspace-title">{{ workspaceTitle }}</h2>
          <p class="workspace-subtitle">{{ workspaceSubtitle }}</p>
          <p class="workspace-description">
            {{ workspaceDescription }}
          </p>
        </div>
        <div class="workspace-stats">
          <article class="stat-card">
            <span class="stat-label">Buckets</span>
            <strong class="stat-value">{{ buckets.length }}</strong>
          </article>
          <article class="stat-card">
            <span class="stat-label">Visible files</span>
            <strong class="stat-value">{{ visibleFileCount }}</strong>
          </article>
          <article class="stat-card">
            <span class="stat-label">Visible folders</span>
            <strong class="stat-value">{{ visibleFolderCount }}</strong>
          </article>
          <article class="stat-card">
            <span class="stat-label">Visible data</span>
            <strong class="stat-value">{{ formatSize(visibleObjectBytes) }}</strong>
          </article>
        </div>
      </section>

      <section class="workspace-panel">
        <div class="toolbar">
          <el-breadcrumb separator="/" class="breadcrumb">
            <el-breadcrumb-item><span class="breadcrumb-link" @click="goBucketRoot">Home</span></el-breadcrumb-item>
            <el-breadcrumb-item v-if="currentBucket">
              <span class="breadcrumb-link" @click="goBucketRoot">{{ currentBucket }}</span>
            </el-breadcrumb-item>
            <el-breadcrumb-item v-for="(part, i) in prefixParts" :key="i">
              <span class="breadcrumb-link" @click="navigateToDepth(i)">{{ part }}</span>
            </el-breadcrumb-item>
          </el-breadcrumb>

          <div class="toolbar-actions">
            <el-button v-if="currentBucket" :icon="Search" @click="searchVisible = !searchVisible">Search</el-button>
            <el-button v-if="currentBucket" :icon="Clock" @click="openHistoryDrawer">History</el-button>
            <el-button v-if="currentBucket" :icon="Finished" @click="openTaskDrawer">Tasks</el-button>
            <el-button v-if="currentBucket" :icon="Brush" @click="openCleanupDrawer">Cleanup</el-button>
            <el-button v-if="currentBucket" :icon="Connection" @click="openWebhookDrawer">Webhooks</el-button>
            <el-button v-if="currentBucket" type="primary" :icon="UploadFilled" @click="showUploadDialog = true">Upload</el-button>
            <el-button v-if="currentBucket" :icon="Share" @click="openUploadLinkDialog">Upload Link</el-button>
            <el-button
              v-if="currentBucket && !currentPrefix"
              type="danger"
              :icon="Delete"
              plain
              @click="confirmDeleteBucket"
            >Delete Bucket</el-button>
          </div>
        </div>

        <div v-if="currentBucket && searchVisible" class="search-panel">
          <el-form :inline="true" class="search-form">
            <el-form-item label="Name">
              <el-input v-model="searchForm.name" placeholder="contains..." clearable />
            </el-form-item>
            <el-form-item label="Min Size">
              <el-input-number v-model="searchForm.minSize" :min="0" :controls="false" />
            </el-form-item>
            <el-form-item label="Max Size">
              <el-input-number v-model="searchForm.maxSize" :min="0" :controls="false" />
            </el-form-item>
            <el-form-item label="Modified">
              <el-date-picker
                v-model="searchDateRange"
                type="datetimerange"
                range-separator="to"
                start-placeholder="Start"
                end-placeholder="End"
                value-format="YYYY-MM-DDTHH:mm:ss[Z]"
              />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" @click="applySearch">Search</el-button>
              <el-button @click="resetSearch">Reset</el-button>
            </el-form-item>
          </el-form>
        </div>

        <div v-if="selectedRows.length" class="batch-toolbar">
          <span>{{ selectedRows.length }} selected</span>
          <div class="batch-toolbar-actions">
            <el-button size="small" :icon="Download" @click="downloadSelected">Download ZIP</el-button>
            <el-button size="small" :icon="FolderOpened" @click="openMoveDialog">Move</el-button>
            <el-button size="small" :icon="Edit" @click="openRenameDialog">Rename</el-button>
            <el-button size="small" type="danger" :icon="Delete" plain @click="confirmBatchDelete">Delete</el-button>
          </div>
        </div>

        <div class="objects-table-wrap">
          <el-table
            v-loading="loading"
            :data="objects"
            class="objects-table"
            style="width: 100%"
            height="100%"
            empty-text="No objects — select a bucket or upload files"
            @selection-change="onSelectionChange"
          >
            <el-table-column type="selection" width="44" />
            <el-table-column label="Name" min-width="320" show-overflow-tooltip>
              <template #default="{ row }">
                <div class="file-row" @click="handleRowClick(row)">
                  <el-icon class="file-icon" :color="row.isDir ? '#ad7f45' : '#7d7063'">
                    <Folder v-if="row.isDir" />
                    <Document v-else />
                  </el-icon>
                  <span :class="row.isDir ? 'folder-name' : ''">{{ row.name }}</span>
                </div>
              </template>
            </el-table-column>
            <el-table-column label="Size" width="120" align="right">
              <template #default="{ row }">
                <span v-if="!row.isDir">{{ formatSize(row.size) }}</span>
                <span v-else style="color:#b8aa99">—</span>
              </template>
            </el-table-column>
            <el-table-column label="Last Modified" width="190">
              <template #default="{ row }">
                <span v-if="!row.isDir">{{ formatDate(row.lastModified) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="Actions" width="270" fixed="right">
              <template #default="{ row }">
                <el-button v-if="!row.isDir" type="primary" :icon="Download" size="small" @click.stop="downloadFile(row)">
                  Download
                </el-button>
                <el-button v-if="!row.isDir" :icon="Share" size="small" @click.stop="openDownloadLinkDialog(row)">
                  Copy Link
                </el-button>
                <el-button type="danger" :icon="Delete" size="small" plain @click.stop="confirmDeleteObject(row)">
                  Delete
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </section>
    </div>

    <el-dialog v-model="showUploadDialog" title="Batch Upload" width="640px" @closed="resetUpload">
      <div
        class="drop-zone"
        :class="{ 'drop-zone--over': isDragging }"
        @dragover.prevent="isDragging = true"
        @dragleave="isDragging = false"
        @drop.prevent="onDrop"
        @click="triggerFileInput"
      >
        <el-icon :size="48" color="#201912"><UploadFilled /></el-icon>
        <p>Drop files here or <strong>click</strong> to select files</p>
        <p class="hint">Select a folder to keep its relative paths. Re-select the same files to resume unfinished uploads.</p>
      </div>
      <div class="upload-picker-actions">
        <el-button @click.stop="triggerFileInput">Select Files</el-button>
        <el-button @click.stop="triggerFolderInput">Select Folder</el-button>
      </div>
      <input ref="fileInputRef" type="file" multiple style="display:none" @change="onFileInputChange" />
      <input ref="folderInputRef" type="file" webkitdirectory multiple style="display:none" @change="onFolderInputChange" />
      <div v-if="uploadFiles.length" class="upload-list">
        <div class="upload-summary">
          <div class="small-text">{{ uploadStats.completed }}/{{ uploadStats.total }} completed · {{ formatSize(uploadStats.loadedBytes) }} / {{ formatSize(uploadStats.totalBytes) }}</div>
          <el-progress :percentage="uploadProgress" :status="uploadStats.failed ? 'exception' : uploadProgress === 100 ? 'success' : undefined" class="upload-progress" />
        </div>
        <div v-for="f in uploadFiles" :key="f.id" class="upload-item upload-item--stacked">
          <div class="upload-item-main">
            <div class="upload-item-meta">
              <el-icon><Document /></el-icon>
              <span class="upload-filename">{{ f.relativePath }}</span>
            </div>
            <span class="upload-size">{{ formatSize(f.size) }}</span>
            <el-tag v-if="f.status === 'done'" type="success" size="small">Done</el-tag>
            <el-tag v-else-if="f.status === 'uploading'" type="primary" size="small">Uploading</el-tag>
            <el-tag v-else-if="f.status === 'paused'" type="warning" size="small">Paused</el-tag>
            <el-tag v-else-if="f.status === 'error'" type="danger" size="small">Error</el-tag>
            <el-tag v-else-if="f.status === 'resumable'" type="info" size="small">Resumable</el-tag>
            <el-tag v-else size="small">Pending</el-tag>
          </div>
          <el-progress :percentage="fileProgress(f)" :status="f.status === 'error' ? 'exception' : f.status === 'done' ? 'success' : undefined" />
          <div class="small-text upload-item-detail">
            <span>{{ formatSize(f.uploadedBytes) }} / {{ formatSize(f.size) }}</span>
            <span v-if="f.error">{{ toEnglishText(f.error) }}</span>
          </div>
        </div>
      </div>
      <template #footer>
        <el-button @click="showUploadDialog = false">Close</el-button>
        <el-button :disabled="!uploadFiles.length || uploading" @click="clearUploadQueue">Clear</el-button>
        <el-button v-if="uploading" @click="pauseUpload">Pause</el-button>
        <el-button type="primary" :disabled="!uploadFiles.length || uploading || uploadStats.completed === uploadStats.total" :loading="uploading" @click="startUpload">
          {{ uploadStats.completed ? 'Resume Upload' : 'Start Upload' }}{{ uploadFiles.length ? ` (${uploadFiles.length})` : '' }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showCreateDialog" title="Create Bucket" width="400px">
      <el-form :model="newBucket" label-width="80px" @submit.prevent="createBucketAction">
        <el-form-item label="Name"><el-input v-model="newBucket.name" placeholder="my-bucket" autofocus /></el-form-item>
        <el-form-item label="Region"><el-input v-model="newBucket.region" placeholder="us-east-1" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">Cancel</el-button>
        <el-button type="primary" @click="createBucketAction">Create</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showMoveDialog" title="Move Selected Items" width="480px">
      <el-form label-width="120px">
        <el-form-item label="Target Prefix">
          <el-input v-model="moveTargetPrefix" placeholder="archive/2026" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showMoveDialog = false">Cancel</el-button>
        <el-button type="primary" @click="submitBatchMove">Move</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showRenameDialog" title="Rename Selected Items" width="640px">
      <div class="rename-list">
        <div v-for="item in renameItems" :key="item.sourceKey" class="rename-item">
          <span class="rename-source">{{ item.sourceKey }}</span>
          <el-input v-model="item.newName" placeholder="New name" />
        </div>
      </div>
      <template #footer>
        <el-button @click="showRenameDialog = false">Cancel</el-button>
        <el-button type="primary" @click="submitBatchRename">Rename</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showDownloadLinkDialog" title="Generate Download Link" width="540px">
      <el-form label-width="100px">
        <el-form-item label="File"><span class="link-meta">{{ downloadLinkTarget?.key }}</span></el-form-item>
        <el-form-item label="Expires in">
          <el-select v-model="downloadLinkExpiry" style="width:100%">
            <el-option label="1 hour" :value="3600" />
            <el-option label="6 hours" :value="21600" />
            <el-option label="24 hours" :value="86400" />
            <el-option label="3 days" :value="259200" />
            <el-option label="7 days" :value="604800" />
          </el-select>
        </el-form-item>
      </el-form>
      <div v-if="downloadLinkUrl" class="generated-link">
        <el-input v-model="downloadLinkUrl" readonly>
          <template #append><el-button :icon="CopyDocument" @click="copyToClipboard(downloadLinkUrl)">Copy</el-button></template>
        </el-input>
      </div>
      <template #footer>
        <el-button @click="showDownloadLinkDialog = false">Close</el-button>
        <el-button type="primary" :loading="generatingDownloadLink" @click="generateDownloadLinkAction">Generate Link</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showUploadLinkDialog" title="Generate Upload Link" width="540px" @closed="resetUploadLink">
      <el-form label-width="100px">
        <el-form-item label="Destination">
          <el-input v-model="uploadLinkKey" placeholder="folder/filename.ext" />
          <div class="field-hint">Full object key for the upload destination.</div>
        </el-form-item>
        <el-form-item label="Expires in">
          <el-select v-model="uploadLinkExpiry" style="width:100%">
            <el-option label="1 hour" :value="3600" />
            <el-option label="6 hours" :value="21600" />
            <el-option label="24 hours" :value="86400" />
            <el-option label="3 days" :value="259200" />
            <el-option label="7 days" :value="604800" />
          </el-select>
        </el-form-item>
      </el-form>
      <div v-if="uploadPageUrl" class="generated-link">
        <el-input v-model="uploadPageUrl" readonly>
          <template #append><el-button :icon="CopyDocument" @click="copyToClipboard(uploadPageUrl)">Copy</el-button></template>
        </el-input>
      </div>
      <template #footer>
        <el-button @click="showUploadLinkDialog = false">Close</el-button>
        <el-button type="primary" :loading="generatingUploadLink" @click="generateUploadLinkAction">Generate Link</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="showTaskDrawer" title="Tasks" size="50%">
      <div class="drawer-actions"><el-button :icon="Refresh" @click="refreshTasks">Refresh</el-button></div>
      <el-table :data="tasks" size="small">
        <el-table-column prop="type" label="Type" width="130" />
        <el-table-column prop="bucket" label="Bucket" width="120" />
        <el-table-column label="Progress" min-width="220">
          <template #default="{ row }">
            <el-progress :percentage="taskProgress(row)" :status="row.status === 'failed' ? 'exception' : row.status === 'completed' ? 'success' : undefined" />
            <div class="small-text">{{ row.completedItems }}/{{ row.totalItems }} · {{ taskMessage(row) }}</div>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="Status" width="110" />
        <el-table-column label="Updated" width="180">
          <template #default="{ row }">{{ formatDate(row.updatedAt) }}</template>
        </el-table-column>
      </el-table>
    </el-drawer>

    <el-drawer v-model="showHistoryDrawer" title="Operation History" size="55%">
      <div class="drawer-actions"><el-button :icon="Refresh" @click="refreshHistory">Refresh</el-button></div>
      <el-table :data="historyEntries" size="small">
        <el-table-column prop="type" label="Type" width="170" />
        <el-table-column prop="actor" label="Actor" width="110" />
        <el-table-column prop="status" label="Status" width="90" />
        <el-table-column label="Keys" min-width="260">
          <template #default="{ row }">{{ (row.keys || []).join(', ') }}</template>
        </el-table-column>
        <el-table-column label="Created" width="180">
          <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
        </el-table-column>
      </el-table>
    </el-drawer>

    <el-drawer v-model="showCleanupDrawer" title="Cleanup Policies" size="55%">
      <el-form label-width="130px" class="drawer-form">
        <el-form-item label="Policy Name"><el-input v-model="cleanupForm.name" /></el-form-item>
        <el-form-item label="Bucket"><el-input v-model="cleanupForm.bucket" /></el-form-item>
        <el-form-item label="Prefix"><el-input v-model="cleanupForm.prefix" placeholder="logs/" /></el-form-item>
        <el-form-item label="Name Contains"><el-input v-model="cleanupForm.nameContains" /></el-form-item>
        <el-form-item label="Older Than Days"><el-input-number v-model="cleanupForm.olderThanDays" :min="0" /></el-form-item>
        <el-form-item label="Keep Latest"><el-input-number v-model="cleanupForm.keepLatest" :min="0" /></el-form-item>
        <el-form-item label="Min Size"><el-input-number v-model="cleanupForm.minSize" :min="0" :controls="false" /></el-form-item>
        <el-form-item label="Max Size"><el-input-number v-model="cleanupForm.maxSize" :min="0" :controls="false" /></el-form-item>
        <el-form-item label="Enabled"><el-switch v-model="cleanupForm.enabled" /></el-form-item>
        <el-form-item>
          <el-button type="primary" @click="createCleanupPolicyAction">Save Policy</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="cleanupPolicies" size="small">
        <el-table-column prop="name" label="Name" min-width="150" />
        <el-table-column prop="bucket" label="Bucket" width="120" />
        <el-table-column prop="prefix" label="Prefix" min-width="140" />
        <el-table-column label="Last Run" width="180">
          <template #default="{ row }">{{ formatDate(row.lastRunAt) }}</template>
        </el-table-column>
        <el-table-column label="Actions" width="180">
          <template #default="{ row }">
            <el-button size="small" @click="runCleanupPolicyAction(row)">Run</el-button>
            <el-button size="small" type="danger" plain @click="deleteCleanupPolicyAction(row)">Delete</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>

    <el-drawer v-model="showWebhookDrawer" title="Webhooks" size="60%">
      <el-form label-width="110px" class="drawer-form">
        <el-form-item label="Name"><el-input v-model="webhookForm.name" /></el-form-item>
        <el-form-item label="URL"><el-input v-model="webhookForm.url" placeholder="https://example.com/webhook" /></el-form-item>
        <el-form-item label="Events">
          <el-select v-model="webhookForm.events" multiple style="width:100%">
            <el-option v-for="event in webhookEvents" :key="event" :label="event" :value="event" />
          </el-select>
        </el-form-item>
        <el-form-item label="Secret"><el-input v-model="webhookForm.secret" type="password" show-password /></el-form-item>
        <el-form-item label="Enabled"><el-switch v-model="webhookForm.enabled" /></el-form-item>
        <el-form-item><el-button type="primary" @click="createWebhookAction">Save Webhook</el-button></el-form-item>
      </el-form>
      <el-table :data="webhooks" size="small">
        <el-table-column prop="name" label="Name" min-width="140" />
        <el-table-column prop="url" label="URL" min-width="220" show-overflow-tooltip />
        <el-table-column label="Events" min-width="180">
          <template #default="{ row }">{{ (row.events || []).join(', ') }}</template>
        </el-table-column>
        <el-table-column label="Actions" width="120">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="deleteWebhookAction(row)">Delete</el-button>
          </template>
        </el-table-column>
      </el-table>
      <h4 class="drawer-subtitle">Recent Deliveries</h4>
      <el-table :data="deliveries" size="small">
        <el-table-column prop="webhook" label="Webhook / Webhook" min-width="130" />
        <el-table-column prop="event" label="Event" min-width="150" />
        <el-table-column prop="status" label="Status" width="100" />
        <el-table-column prop="statusCode" label="HTTP" width="80" />
        <el-table-column prop="error" label="Error" min-width="180" show-overflow-tooltip />
      </el-table>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Plus,
  UploadFilled,
  Delete,
  Download,
  Folder,
  Document,
  Coin,
  Share,
  CopyDocument,
  Search,
  Clock,
  Finished,
  Brush,
  Connection,
  FolderOpened,
  Edit,
  Refresh
} from '@element-plus/icons-vue'
import {
  listBuckets,
  createBucket,
  deleteBucket,
  listObjects,
  searchObjects,
  downloadUrl,
  deleteObject,
  batchDelete,
  batchMove,
  batchRename,
  batchDownload,
  listTasks,
  listHistory,
  listCleanupPolicies,
  createCleanupPolicy,
  deleteCleanupPolicy,
  runCleanupPolicy,
  listWebhooks,
  createWebhook,
  deleteWebhook,
  listWebhookDeliveries,
  generateDownloadLink,
  generateUploadLink,
  initResumableUpload,
  getResumableUploadStatus,
  uploadResumablePart,
  completeResumableUpload,
  abortResumableUpload
} from '../api'

const route = useRoute()
const router = useRouter()

const buckets = ref([])
const objects = ref([])
const loading = ref(false)
const selectedRows = ref([])
const searchVisible = ref(false)
const searchForm = ref({ name: '', minSize: null, maxSize: null })
const searchDateRange = ref([])
const searchActive = ref(false)

const showUploadDialog = ref(false)
const uploadFiles = ref([])
const uploading = ref(false)
const uploadProgress = ref(0)
const isDragging = ref(false)
const fileInputRef = ref(null)
const folderInputRef = ref(null)
const pauseUploadRequested = ref(false)
const uploadBatchTaskId = ref('')

const showCreateDialog = ref(false)
const newBucket = ref({ name: '', region: 'us-east-1' })

const showMoveDialog = ref(false)
const moveTargetPrefix = ref('')
const showRenameDialog = ref(false)
const renameItems = ref([])

const showDownloadLinkDialog = ref(false)
const downloadLinkTarget = ref(null)
const downloadLinkExpiry = ref(86400)
const downloadLinkUrl = ref('')
const generatingDownloadLink = ref(false)

const showUploadLinkDialog = ref(false)
const uploadLinkKey = ref('')
const uploadLinkExpiry = ref(86400)
const uploadPageUrl = ref('')
const generatingUploadLink = ref(false)

const showTaskDrawer = ref(false)
const tasks = ref([])
const showHistoryDrawer = ref(false)
const historyEntries = ref([])

const showCleanupDrawer = ref(false)
const cleanupPolicies = ref([])
const cleanupForm = ref({
  name: '',
  bucket: '',
  prefix: '',
  nameContains: '',
  olderThanDays: 0,
  keepLatest: 0,
  minSize: 0,
  maxSize: 0,
  enabled: true
})

const showWebhookDrawer = ref(false)
const webhooks = ref([])
const deliveries = ref([])
const webhookEvents = [
  'object.uploaded',
  'object.deleted',
  'object.moved',
  'object.renamed',
  'object.downloaded',
  'object.batch_downloaded',
  'cleanup.completed'
]
const webhookForm = ref({ name: '', url: '', events: ['object.uploaded'], secret: '', enabled: true })
const PENDING_STATUS_LABEL = 'Pending'

const workspaceCopy = {
  default: {
    eyebrow: 'Storage workspace',
    title: 'Select a bucket to start managing storage',
    subtitle: 'Editorial hierarchy, warm surfaces, and clearer language for every storage task.',
    description: 'Choose a bucket on the left to browse objects, share links, and run cleanup flows without leaving the workspace.'
  },
  active(bucket, prefix) {
    return {
      eyebrow: 'Active bucket',
      title: bucket,
      subtitle: `A calmer place to browse ${bucket}.`,
      description: `Manage ${bucket} from one warm, focused surface${prefix ? ` — ${prefix}` : '.'}`
    }
  }
}

const currentBucket = computed(() => route.params.bucket || '')
const currentPrefix = computed(() => {
  const match = route.params.pathMatch
  if (!match) return ''
  const raw = Array.isArray(match) ? match.join('/') : match
  return raw ? `${raw}/` : ''
})
const workspaceContent = computed(() => (currentBucket.value
  ? workspaceCopy.active(currentBucket.value, currentPrefix.value)
  : workspaceCopy.default))
const prefixParts = computed(() => currentPrefix.value.split('/').filter(Boolean))
const uploadStats = computed(() => {
  const total = uploadFiles.value.length
  const completed = uploadFiles.value.filter((item) => item.status === 'done').length
  const failed = uploadFiles.value.some((item) => item.status === 'error')
  const totalBytes = uploadFiles.value.reduce((sum, item) => sum + (item.size || 0), 0)
  const loadedBytes = uploadFiles.value.reduce((sum, item) => sum + Math.min(item.uploadedBytes || 0, item.size || 0), 0)
  return { total, completed, failed, totalBytes, loadedBytes }
})
const workspaceEyebrow = computed(() => workspaceContent.value.eyebrow)
const workspaceTitle = computed(() => workspaceContent.value.title)
const workspaceSubtitle = computed(() => workspaceContent.value.subtitle)
const workspaceDescription = computed(() => workspaceContent.value.description)
const visibleFolderCount = computed(() => objects.value.filter((item) => item.isDir).length)
const visibleFileCount = computed(() => objects.value.filter((item) => !item.isDir).length)
const visibleObjectBytes = computed(() => objects.value.reduce((sum, item) => sum + (item.isDir ? 0 : item.size || 0), 0))

const RESUMABLE_PART_SIZE = 8 * 1024 * 1024
const RESUMABLE_UPLOAD_STORAGE_KEY = 'kipup-resumable-upload-sessions-v1'
const LEGACY_RESUMABLE_UPLOAD_STORAGE_KEY = 's3c-resumable-upload-sessions-v1'

let poller = null

onMounted(async () => {
  await fetchBuckets()
  startPolling()
})

onUnmounted(() => stopPolling())

watch(
  () => [currentBucket.value, currentPrefix.value],
  async ([bucket]) => {
    cleanupForm.value.bucket = bucket || cleanupForm.value.bucket
    moveTargetPrefix.value = currentPrefix.value
    if (!bucket) {
      objects.value = []
      return
    }
    await fetchObjects()
  },
  { immediate: true }
)

async function fetchBuckets() {
  try {
    const { data } = await listBuckets()
    buckets.value = data || []
  } catch (error) {
    ElMessage.error('Failed to load buckets: ' + (error.response?.data?.error || error.message))
  }
}

async function fetchObjects() {
  if (!currentBucket.value) return
  loading.value = true
  selectedRows.value = []
  try {
    const params = buildSearchParams()
    const request = searchActive.value ? searchObjects(currentBucket.value, params) : listObjects(currentBucket.value, currentPrefix.value)
    const { data } = await request
    objects.value = data || []
  } catch (error) {
    objects.value = []
    ElMessage.error('Failed to load objects: ' + (error.response?.data?.error || error.message))
  } finally {
    loading.value = false
  }
}

function buildSearchParams() {
  const params = {
    prefix: currentPrefix.value,
    name: searchForm.value.name || undefined,
    minSize: searchForm.value.minSize ?? undefined,
    maxSize: searchForm.value.maxSize ?? undefined,
    modifiedAfter: searchDateRange.value?.[0] || undefined,
    modifiedBefore: searchDateRange.value?.[1] || undefined
  }
  return params
}

function applySearch() {
  searchActive.value = true
  fetchObjects()
}

function resetSearch() {
  searchForm.value = { name: '', minSize: null, maxSize: null }
  searchDateRange.value = []
  searchActive.value = false
  fetchObjects()
}

function onSelectionChange(rows) {
  selectedRows.value = rows
}

function selectBucket(name) {
  router.push({ name: 'bucket', params: { bucket: name } })
}

function goBucketRoot() {
  if (currentBucket.value) router.push({ name: 'bucket', params: { bucket: currentBucket.value } })
  else router.push({ name: 'browser' })
}

function navigateToDepth(index) {
  router.push({ name: 'folder', params: { bucket: currentBucket.value, pathMatch: prefixParts.value.slice(0, index + 1).join('/') } })
}

function handleRowClick(row) {
  if (!row.isDir) return
  router.push({ name: 'folder', params: { bucket: currentBucket.value, pathMatch: row.key.replace(/\/$/, '') } })
}

function openCreateBucket() {
  newBucket.value = { name: '', region: 'us-east-1' }
  showCreateDialog.value = true
}

async function createBucketAction() {
  const name = newBucket.value.name.trim()
  if (!name) return ElMessage.warning('Bucket name is required')
  try {
    await createBucket(name, newBucket.value.region || 'us-east-1')
    showCreateDialog.value = false
    ElMessage.success(`Bucket "${name}" created`)
    await fetchBuckets()
  } catch (error) {
    ElMessage.error('Failed to create bucket: ' + (error.response?.data?.error || error.message))
  }
}

async function confirmDeleteBucket() {
  try {
    await ElMessageBox.confirm(`Delete bucket "${currentBucket.value}"? All objects must be removed first.`, 'Delete Bucket', { type: 'warning' })
    await deleteBucket(currentBucket.value)
    ElMessage.success('Bucket deleted')
    router.push({ name: 'browser' })
    await fetchBuckets()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error('Action failed: ' + (error.response?.data?.error || error.message))
  }
}

function downloadFile(row) {
  const a = document.createElement('a')
  a.href = downloadUrl(currentBucket.value, row.key)
  a.download = row.name
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

async function confirmDeleteObject(row) {
  try {
    await ElMessageBox.confirm(`Delete ${row.isDir ? 'folder' : 'file'} "${row.name}"?`, 'Confirm Delete', { type: 'warning' })
    await deleteObject(currentBucket.value, row.key)
    ElMessage.success('Deleted')
    await fetchObjects()
    await refreshHistory()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error('Action failed: ' + (error.response?.data?.error || error.message))
  }
}

function triggerFileInput() {
  fileInputRef.value?.click()
}

function triggerFolderInput() {
  folderInputRef.value?.click()
}

function onFileInputChange(event) {
  addFiles(Array.from(event.target.files || []))
  event.target.value = ''
}

function onFolderInputChange(event) {
  addFiles(Array.from(event.target.files || []))
  event.target.value = ''
}

function onDrop(event) {
  isDragging.value = false
  addFiles(Array.from(event.dataTransfer.files || []))
}

function addFiles(files) {
  for (const file of files) {
    const relativePath = normalizeUploadPath(file.webkitRelativePath || file.name)
    if (!relativePath) continue
    const prefix = normalizePrefix(currentPrefix.value).replace(/\/$/, '')
    const key = buildUploadObjectKey(relativePath, prefix)
    const id = buildUploadEntryId(key, file)
    const existing = uploadFiles.value.find((item) => item.id === id)
    const persisted = getPersistedUploadSession(id)
    if (existing) {
      existing.file = file
      existing.status = resolveUploadStatus(existing.status, persisted)
      existing.error = persisted?.error || ''
      continue
    }
    uploadFiles.value.push({
      id,
      key,
      prefix: persisted?.prefix || prefix,
      name: file.name,
      relativePath,
      size: file.size,
      lastModified: file.lastModified,
      contentType: file.type || 'application/octet-stream',
      file,
      status: resolveUploadStatus('pending', persisted),
      error: persisted?.error || '',
      uploadedBytes: persisted?.uploadedBytes || 0,
      uploadId: persisted?.uploadId || '',
      partSize: persisted?.partSize || RESUMABLE_PART_SIZE,
      parts: Array.isArray(persisted?.parts) ? persisted.parts : [],
      taskId: persisted?.taskId || ''
    })
  }
  refreshUploadProgress()
}

function resetUpload() {
  isDragging.value = false
  if (!uploadFiles.value.some((item) => item.status !== 'done')) {
    void clearUploadQueue()
  }
}

async function startUpload() {
  if (!uploadFiles.value.length) return
  const pendingItems = uploadFiles.value.filter((item) => item.status !== 'done')
  if (!pendingItems.length) return
  const taskId = uploadBatchTaskId.value || pendingItems.find((item) => item.taskId)?.taskId || createTaskId('upload')
  uploadBatchTaskId.value = taskId
  pauseUploadRequested.value = false
  uploading.value = true
  try {
    for (const item of pendingItems) {
      if (pauseUploadRequested.value) break
      item.taskId = taskId
      await uploadFileInParts(item, taskId)
    }
    if (pauseUploadRequested.value) {
      uploadFiles.value.forEach((item) => {
        if (item.status !== 'done') item.status = 'paused'
      })
      ElMessage.info('Upload paused')
      return
    }
    if (uploadFiles.value.every((item) => item.status === 'done')) {
      ElMessage.success(`${uploadFiles.value.length} file(s) uploaded`)
      showUploadDialog.value = false
      await clearUploadQueue()
      await Promise.all([fetchObjects(), refreshTasks(), refreshHistory()])
    }
  } catch (error) {
    ElMessage.error('Upload failed: ' + (error.response?.data?.error || error.message))
  } finally {
    uploading.value = false
    refreshUploadProgress()
  }
}

function pauseUpload() {
  pauseUploadRequested.value = true
}

async function clearUploadQueue() {
  await Promise.allSettled(
    uploadFiles.value
      .filter((item) => item.uploadId && item.status !== 'done')
      .map((item) => abortResumableUpload(currentBucket.value, item.relativePath, item.uploadId, item.prefix || ''))
  )
  for (const item of uploadFiles.value) {
    clearPersistedUploadSession(item.id)
  }
  uploadFiles.value = []
  uploadBatchTaskId.value = ''
  uploadProgress.value = 0
  uploading.value = false
  pauseUploadRequested.value = false
}

async function uploadFileInParts(item, taskId) {
  try {
    if (!item.file) {
      throw new Error(`Please re-select "${item.relativePath}" to continue.`)
    }
    item.status = 'uploading'
    item.error = ''
    const prefix = item.prefix || ''
    await ensureResumableSession(item, taskId, prefix)
    const { data: status } = await getResumableUploadStatus(currentBucket.value, item.relativePath, item.uploadId, prefix)
    item.parts = Array.isArray(status.parts) ? status.parts : []
    item.uploadedBytes = calculateUploadedBytes(item)
    refreshUploadProgress()
    const totalParts = Math.max(1, Math.ceil(item.size / item.partSize))
    for (let partNumber = 1; partNumber <= totalParts; partNumber += 1) {
      if (pauseUploadRequested.value) {
        item.status = 'paused'
        persistUploadSession(item)
        return
      }
      if (item.parts.some((part) => part.partNumber === partNumber)) continue
      const start = (partNumber - 1) * item.partSize
      const end = Math.min(item.size, start + item.partSize)
      const chunk = item.file.slice(start, end)
      const uploadedBeforePart = calculateUploadedBytes(item)
      const { data } = await uploadResumablePart(
        currentBucket.value,
        item.relativePath,
        item.uploadId,
        partNumber,
        chunk,
        prefix,
        (event) => {
          item.uploadedBytes = Math.min(item.size, uploadedBeforePart + (event.loaded || 0))
          refreshUploadProgress()
        }
      )
      item.parts = [...item.parts, { partNumber: data.partNumber, etag: data.etag, size: data.size || chunk.size }].sort(
        (a, b) => a.partNumber - b.partNumber
      )
      item.uploadedBytes = calculateUploadedBytes(item)
      persistUploadSession(item)
      refreshUploadProgress()
    }
    await completeResumableUpload(
      currentBucket.value,
      {
        key: item.relativePath,
        uploadId: item.uploadId,
        contentType: item.contentType,
        taskId,
        totalItems: uploadFiles.value.length,
        completedItems: uploadFiles.value.filter((entry) => entry.status === 'done').length + 1,
        parts: item.parts.map((part) => ({ partNumber: part.partNumber, etag: part.etag, size: part.size }))
      },
      prefix
    )
    item.status = 'done'
    item.uploadedBytes = item.size
    item.error = ''
    clearPersistedUploadSession(item.id)
  } catch (error) {
    item.status = 'error'
    item.error = error.response?.data?.error || error.message
    persistUploadSession(item)
    throw error
  }
}

async function ensureResumableSession(item, taskId, prefix) {
  if (item.uploadId) {
    persistUploadSession(item)
    return
  }
  const { data } = await initResumableUpload(
    currentBucket.value,
    {
      key: item.relativePath,
      size: item.size,
      contentType: item.contentType,
      taskId,
      totalItems: uploadFiles.value.length
    },
    prefix
  )
  item.uploadId = data.uploadId
  item.partSize = data.partSize || RESUMABLE_PART_SIZE
  item.taskId = data.taskId || taskId
  persistUploadSession(item)
}

function buildUploadEntryId(key, file) {
  return [currentBucket.value, key, file.size, file.lastModified].join('::')
}

function resolveUploadStatus(currentStatus, persisted) {
  if (currentStatus === 'done') return 'done'
  if (!persisted?.uploadId) return currentStatus
  return persisted.status === 'error' ? 'error' : 'resumable'
}

function normalizeUploadPath(value) {
  return (value || '')
    .replace(/\\/g, '/')
    .replace(/^\/+/, '')
    .split('/')
    .filter(Boolean)
    .join('/')
}

function buildUploadObjectKey(relativePath, prefix = currentPrefix.value) {
  const path = normalizeUploadPath(relativePath)
  return `${normalizePrefix(prefix)}${path}`
}

function fileProgress(file) {
  if (!file?.size) return file?.status === 'done' ? 100 : 0
  return Math.min(100, Math.round(((file.uploadedBytes || 0) / file.size) * 100))
}

function calculateUploadedBytes(item) {
  return (item.parts || []).reduce((sum, part) => sum + (part.size || 0), 0)
}

function refreshUploadProgress() {
  if (!uploadStats.value.totalBytes) {
    if (!uploadStats.value.total) {
      uploadProgress.value = 0
      return
    }
    uploadProgress.value = uploadStats.value.completed === uploadStats.value.total ? 100 : 0
    return
  }
  uploadProgress.value = Math.min(100, Math.round((uploadStats.value.loadedBytes / uploadStats.value.totalBytes) * 100))
}

function readPersistedUploadSessions() {
  try {
    const current = window.localStorage.getItem(RESUMABLE_UPLOAD_STORAGE_KEY)
    if (current) {
      return JSON.parse(current)
    }
    return JSON.parse(window.localStorage.getItem(LEGACY_RESUMABLE_UPLOAD_STORAGE_KEY) || '{}')
  } catch {
    return {}
  }
}

function writePersistedUploadSessions(sessions) {
  window.localStorage.setItem(RESUMABLE_UPLOAD_STORAGE_KEY, JSON.stringify(sessions))
  window.localStorage.removeItem(LEGACY_RESUMABLE_UPLOAD_STORAGE_KEY)
}

function getPersistedUploadSession(id) {
  return readPersistedUploadSessions()[id] || null
}

function persistUploadSession(item) {
  const sessions = readPersistedUploadSessions()
  sessions[item.id] = {
    id: item.id,
    key: item.key,
    prefix: item.prefix,
    relativePath: item.relativePath,
    size: item.size,
    lastModified: item.lastModified,
    uploadedBytes: calculateUploadedBytes(item),
    uploadId: item.uploadId,
    partSize: item.partSize,
    parts: item.parts,
    taskId: item.taskId,
    status: item.status,
    error: item.error,
    updatedAt: new Date().toISOString()
  }
  writePersistedUploadSessions(sessions)
}

function clearPersistedUploadSession(id) {
  const sessions = readPersistedUploadSessions()
  if (!sessions[id]) return
  delete sessions[id]
  writePersistedUploadSessions(sessions)
}

function selectedKeys() {
  return selectedRows.value.map((row) => row.key)
}

async function downloadSelected() {
  if (!selectedRows.value.length) return
  try {
    const { data } = await batchDownload(currentBucket.value, selectedKeys())
    const blob = new Blob([data], { type: 'application/zip' })
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${currentBucket.value}-batch.zip`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    window.URL.revokeObjectURL(url)
    await refreshHistory()
  } catch (error) {
    ElMessage.error('Batch download failed: ' + (error.response?.data?.error || error.message))
  }
}

function openMoveDialog() {
  moveTargetPrefix.value = currentPrefix.value
  showMoveDialog.value = true
}

async function submitBatchMove() {
  const prefix = normalizePrefix(moveTargetPrefix.value)
  const items = selectedRows.value.map((row) => ({
    sourceKey: row.key,
    targetKey: `${prefix}${row.name}${row.isDir ? '/' : ''}`
  }))
  try {
    await batchMove(currentBucket.value, items, createTaskId('move'))
    showMoveDialog.value = false
    ElMessage.success('Move started/completed')
    await Promise.all([fetchObjects(), refreshTasks(), refreshHistory()])
  } catch (error) {
    ElMessage.error('Move failed: ' + (error.response?.data?.error || error.message))
  }
}

function openRenameDialog() {
  renameItems.value = selectedRows.value.map((row) => ({ sourceKey: row.key, newName: row.name }))
  showRenameDialog.value = true
}

async function submitBatchRename() {
  try {
    await batchRename(currentBucket.value, renameItems.value, createTaskId('rename'))
    showRenameDialog.value = false
    ElMessage.success('Rename started/completed')
    await Promise.all([fetchObjects(), refreshTasks(), refreshHistory()])
  } catch (error) {
    ElMessage.error('Rename failed: ' + (error.response?.data?.error || error.message))
  }
}

async function confirmBatchDelete() {
  try {
    await ElMessageBox.confirm(`Delete ${selectedRows.value.length} selected item(s)?`, 'Batch Delete', { type: 'warning' })
    await batchDelete(currentBucket.value, selectedKeys(), createTaskId('delete'))
    ElMessage.success('Delete started/completed')
    await Promise.all([fetchObjects(), refreshTasks(), refreshHistory()])
  } catch (error) {
    if (error !== 'cancel') ElMessage.error('Delete failed: ' + (error.response?.data?.error || error.message))
  }
}

function openDownloadLinkDialog(row) {
  downloadLinkTarget.value = row
  downloadLinkExpiry.value = 86400
  downloadLinkUrl.value = ''
  showDownloadLinkDialog.value = true
}

async function generateDownloadLinkAction() {
  if (!downloadLinkTarget.value) return
  generatingDownloadLink.value = true
  try {
    const key = downloadLinkTarget.value.key
    const [downloadResult, uploadResult] = await Promise.allSettled([
      generateDownloadLink(currentBucket.value, key, downloadLinkExpiry.value),
      generateUploadLink(currentBucket.value, key, downloadLinkExpiry.value)
    ])
    if (downloadResult.status === 'rejected') {
      throw new Error(
        `Failed to create download link: ${downloadResult.reason?.response?.data?.error || downloadResult.reason?.message || 'unknown error'}`
      )
    }
    if (uploadResult.status === 'rejected') {
      throw new Error(
        `Failed to create upload link: ${uploadResult.reason?.response?.data?.error || uploadResult.reason?.message || 'unknown error'}`
      )
    }
    const downloadData = downloadResult.value.data
    const uploadData = uploadResult.value.data
    const filename = key.split('/').pop() || key
    const params = new URLSearchParams({
      url: uploadData.url,
      downloadUrl: downloadData.url,
      filename
    })
    downloadLinkUrl.value = `${window.location.origin}/upload?${params.toString()}`
  } catch (error) {
    ElMessage.error(error.message || 'Failed to generate link')
  } finally {
    generatingDownloadLink.value = false
  }
}

function openUploadLinkDialog() {
  uploadLinkKey.value = currentPrefix.value
  uploadPageUrl.value = ''
  uploadLinkExpiry.value = 86400
  showUploadLinkDialog.value = true
}

async function generateUploadLinkAction() {
  const key = uploadLinkKey.value.trim()
  if (!key) return ElMessage.warning('Destination key is required')
  generatingUploadLink.value = true
  try {
    const { data } = await generateUploadLink(currentBucket.value, key, uploadLinkExpiry.value)
    const filename = key.split('/').pop() || key
    const params = new URLSearchParams({ url: data.url, filename })
    uploadPageUrl.value = `${window.location.origin}/upload?${params.toString()}`
  } catch (error) {
    ElMessage.error('Failed to generate link: ' + (error.response?.data?.error || error.message))
  } finally {
    generatingUploadLink.value = false
  }
}

function resetUploadLink() {
  uploadPageUrl.value = ''
}

function openTaskDrawer() {
  showTaskDrawer.value = true
  refreshTasks()
}

function openHistoryDrawer() {
  showHistoryDrawer.value = true
  refreshHistory()
}

function openCleanupDrawer() {
  cleanupForm.value.bucket = currentBucket.value
  showCleanupDrawer.value = true
  refreshCleanupPolicies()
}

function openWebhookDrawer() {
  showWebhookDrawer.value = true
  refreshWebhooks()
}

async function refreshTasks() {
  try {
    const { data } = await listTasks({ bucket: currentBucket.value || undefined })
    tasks.value = data || []
  } catch (error) {
    ElMessage.error('Failed to load tasks: ' + (error.response?.data?.error || error.message))
  }
}

async function refreshHistory() {
  try {
    const { data } = await listHistory({ bucket: currentBucket.value || undefined })
    historyEntries.value = data || []
  } catch (error) {
    ElMessage.error('Failed to load history: ' + (error.response?.data?.error || error.message))
  }
}

async function refreshCleanupPolicies() {
  try {
    const { data } = await listCleanupPolicies()
    cleanupPolicies.value = (data || []).filter((policy) => !currentBucket.value || policy.bucket === currentBucket.value)
  } catch (error) {
    ElMessage.error('Failed to load cleanup policies: ' + (error.response?.data?.error || error.message))
  }
}

async function createCleanupPolicyAction() {
  if (!cleanupForm.value.name || !cleanupForm.value.bucket) {
    return ElMessage.warning('Policy name and bucket are required')
  }
  try {
    await createCleanupPolicy({ ...cleanupForm.value })
    cleanupForm.value = {
      name: '',
      bucket: currentBucket.value,
      prefix: currentPrefix.value,
      nameContains: '',
      olderThanDays: 0,
      keepLatest: 0,
      minSize: 0,
      maxSize: 0,
      enabled: true
    }
    ElMessage.success('Cleanup policy saved')
    await refreshCleanupPolicies()
  } catch (error) {
    ElMessage.error('Failed to save cleanup policy: ' + (error.response?.data?.error || error.message))
  }
}

async function runCleanupPolicyAction(policy) {
  try {
    const { data } = await runCleanupPolicy(policy.id)
    ElMessage.success(`Cleanup removed ${(data.deleted || []).length} object(s)`) 
    await Promise.all([fetchObjects(), refreshCleanupPolicies(), refreshHistory(), refreshTasks()])
  } catch (error) {
    ElMessage.error('Cleanup failed: ' + (error.response?.data?.error || error.message))
  }
}

async function deleteCleanupPolicyAction(policy) {
  try {
    await deleteCleanupPolicy(policy.id)
    ElMessage.success('Policy deleted')
    await refreshCleanupPolicies()
  } catch (error) {
    ElMessage.error('Failed to delete policy: ' + (error.response?.data?.error || error.message))
  }
}

async function refreshWebhooks() {
  try {
    const [{ data: hooks }, { data: deliveryItems }] = await Promise.all([listWebhooks(), listWebhookDeliveries()])
    webhooks.value = hooks || []
    deliveries.value = deliveryItems || []
  } catch (error) {
    ElMessage.error('Failed to load webhooks: ' + (error.response?.data?.error || error.message))
  }
}

async function createWebhookAction() {
  if (!webhookForm.value.name || !webhookForm.value.url) return ElMessage.warning('Webhook name and URL are required')
  try {
    await createWebhook({ ...webhookForm.value })
    webhookForm.value = { name: '', url: '', events: ['object.uploaded'], secret: '', enabled: true }
    ElMessage.success('Webhook saved')
    await refreshWebhooks()
  } catch (error) {
    ElMessage.error('Failed to save webhook: ' + (error.response?.data?.error || error.message))
  }
}

async function deleteWebhookAction(webhook) {
  try {
    await deleteWebhook(webhook.id)
    ElMessage.success('Webhook deleted')
    await refreshWebhooks()
  } catch (error) {
    ElMessage.error('Failed to delete webhook: ' + (error.response?.data?.error || error.message))
  }
}

function startPolling() {
  stopPolling()
  poller = window.setInterval(() => {
    if (currentBucket.value) {
      refreshTasks()
    }
    if (showWebhookDrawer.value) {
      refreshWebhooks()
    }
  }, 5000)
}

function stopPolling() {
  if (poller) {
    window.clearInterval(poller)
    poller = null
  }
}

function taskProgress(task) {
  if (!task?.totalItems) return task?.status === 'completed' ? 100 : 0
  return Math.min(100, Math.round((task.completedItems / task.totalItems) * 100))
}

async function copyToClipboard(text) {
  if (!text) return
  try {
    if (window.isSecureContext && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }
    ElMessage.success('Copied to clipboard')
  } catch {
    ElMessage.error('Failed to copy')
  }
}

function normalizePrefix(prefix) {
  const value = (prefix || '').trim().replace(/^\/+/, '').replace(/\/+/g, '/')
  if (!value) return ''
  return value.endsWith('/') ? value : `${value}/`
}

function createTaskId(prefix) {
  if (window.crypto?.randomUUID) return `${prefix}-${window.crypto.randomUUID()}`
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function taskMessage(row) {
  if (row.currentKey) return row.currentKey
  if (row.message) return toEnglishText(row.message)
  return PENDING_STATUS_LABEL
}

function toEnglishText(text) {
  if (!text) return ''
  const [english] = text.split(' / ')
  return english || text
}

function formatSize(bytes) {
  if (bytes == null) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let value = bytes
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i += 1
  }
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function formatDate(value) {
  if (!value) return '—'
  return new Date(value).toLocaleString()
}
</script>

<style scoped>
.browser-layout {
  display: flex;
  gap: 24px;
  height: calc(100vh - 162px);
  padding-top: 28px;
  box-sizing: border-box;
}

.sidebar {
  width: 260px;
  min-width: 260px;
  display: flex;
  flex-direction: column;
  background: rgba(255, 252, 245, 0.82);
  border: 1px solid rgba(69, 54, 42, 0.12);
  border-radius: 28px;
  box-shadow: 0 24px 80px rgba(59, 43, 31, 0.08);
  overflow: hidden;
}

.sidebar-intro,
.sidebar-footer {
  padding: 22px 22px 0;
}

.sidebar-eyebrow {
  margin: 0 0 10px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #8b7f72;
}

.sidebar-copy,
.sidebar-footer {
  color: #6f6256;
  font-size: 14px;
  line-height: 1.6;
}

.sidebar-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 22px 14px;
}

.sidebar-title {
  font-weight: 600;
  font-size: 12px;
  color: #8b7f72;
  text-transform: uppercase;
  letter-spacing: 0.12em;
}

.sidebar-scroll {
  flex: 1;
  padding: 0 10px 12px;
}

.bucket-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.bucket-item {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
  padding: 12px 14px;
  border: 1px solid transparent;
  border-radius: 18px;
  cursor: pointer;
  font-size: 14px;
  color: #2b241d;
  transition: 0.18s ease;
}

.bucket-item:hover {
  background: rgba(237, 226, 210, 0.52);
  border-color: rgba(69, 54, 42, 0.08);
}

.bucket-item.active {
  background: #201912;
  color: #f9f3ea;
  box-shadow: 0 18px 36px rgba(32, 25, 18, 0.18);
}

.bucket-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-footer {
  padding: 0 22px 22px;
  border-top: 1px solid rgba(69, 54, 42, 0.08);
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
}

.workspace-intro {
  display: flex;
  align-items: stretch;
  justify-content: space-between;
  gap: 20px;
}

.workspace-copy,
.workspace-stats {
  background: rgba(255, 252, 245, 0.82);
  border: 1px solid rgba(69, 54, 42, 0.12);
  border-radius: 28px;
  box-shadow: 0 24px 80px rgba(59, 43, 31, 0.08);
}

.workspace-copy {
  flex: 1.6;
  padding: 34px 34px 32px;
}

.workspace-eyebrow {
  margin: 0 0 12px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: #8b7f72;
}

.workspace-title {
  margin: 0;
  font-family: Iowan Old Style, Palatino Linotype, Book Antiqua, Georgia, serif;
  max-width: 840px;
  font-size: 42px;
  font-weight: 600;
  letter-spacing: -0.04em;
  line-height: 1.1;
  color: #201912;
}

.workspace-subtitle {
  max-width: 720px;
  margin: 14px 0 0;
  color: #6f6256;
  font-size: 16px;
  line-height: 1.7;
}

.workspace-description {
  max-width: 760px;
  margin: 12px 0 0;
  color: #5c5146;
  font-size: 15px;
  line-height: 1.7;
}

.workspace-stats {
  flex: 1;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  padding: 16px;
}

.stat-card {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  min-height: 116px;
  padding: 18px 20px;
  border-radius: 22px;
  background: rgba(237, 226, 210, 0.38);
  border: 1px solid rgba(69, 54, 42, 0.08);
}

.stat-label {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: #8b7f72;
}

.stat-value {
  margin-top: 18px;
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -0.03em;
  color: #201912;
}

.workspace-panel {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background: rgba(255, 252, 245, 0.82);
  border: 1px solid rgba(69, 54, 42, 0.12);
  border-radius: 28px;
  box-shadow: 0 24px 80px rgba(59, 43, 31, 0.08);
}

.toolbar,
.batch-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 18px 22px;
  border-bottom: 1px solid rgba(69, 54, 42, 0.08);
  gap: 12px;
  flex-wrap: wrap;
}

.toolbar-actions,
.batch-toolbar-actions,
.drawer-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.toolbar-actions :deep(.el-button span),
.batch-toolbar-actions :deep(.el-button span),
.drawer-actions :deep(.el-button span) {
  letter-spacing: -0.01em;
}

.breadcrumb {
  flex: 1;
}

.breadcrumb-link {
  cursor: pointer;
  color: #5c5146;
  transition: color 0.2s ease;
}

.breadcrumb-link:hover {
  color: #201912;
}

.search-panel {
  padding: 18px 22px 0;
  border-bottom: 1px solid rgba(69, 54, 42, 0.08);
}

.search-form,
.drawer-form {
  margin-bottom: 16px;
}

.batch-toolbar {
  background: rgba(237, 226, 210, 0.34);
}

.objects-table {
  width: 100%;
}

.objects-table-wrap {
  flex: 1;
  min-height: 0;
  margin: 0 22px 22px;
}

.file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
}

.file-icon {
  width: 22px;
  height: 22px;
  padding: 8px;
  border-radius: 14px;
  background: rgba(237, 226, 210, 0.52);
}

.folder-name {
  font-weight: 600;
}

.drop-zone {
  border: 1px dashed rgba(69, 54, 42, 0.22);
  border-radius: 24px;
  padding: 36px 24px;
  text-align: center;
  cursor: pointer;
  background: rgba(237, 226, 210, 0.22);
  transition: 0.2s ease;
}

.drop-zone:hover,
.drop-zone--over {
  border-color: rgba(32, 25, 18, 0.28);
  background: rgba(237, 226, 210, 0.42);
}

.hint,
.small-text,
.field-hint {
  color: #8b7f72;
  font-size: 12px;
}

.upload-list,
.rename-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin-top: 16px;
}

.upload-picker-actions,
.upload-summary,
.upload-item-detail {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
}

.upload-picker-actions {
  margin-top: 12px;
}

.upload-item,
.rename-item {
  display: flex;
  align-items: center;
  gap: 10px;
}

.upload-item--stacked {
  align-items: stretch;
  flex-direction: column;
  padding: 14px 16px;
  border: 1px solid rgba(69, 54, 42, 0.08);
  border-radius: 20px;
  background: rgba(237, 226, 210, 0.24);
}

.upload-item-main,
.upload-item-meta {
  display: flex;
  align-items: center;
  gap: 10px;
}

.upload-item-main {
  justify-content: space-between;
}

.upload-item-meta {
  flex: 1;
  min-width: 0;
}

.upload-filename,
.rename-source {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.upload-progress {
  margin-top: 12px;
}

.generated-link {
  margin-top: 16px;
}

.link-meta {
  word-break: break-all;
}

.drawer-subtitle {
  margin: 20px 0 12px;
  color: #2b241d;
}

.rename-item {
  padding: 10px 0;
}

:deep(.el-empty) {
  padding: 24px 0;
}

:deep(.el-drawer__header span),
:deep(.el-dialog__title) {
  font-family: Iowan Old Style, Palatino Linotype, Book Antiqua, Georgia, serif;
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -0.02em;
  color: #201912;
}

:deep(.el-form-item__label) {
  color: #6f6256;
}

:deep(.el-table .cell) {
  line-height: 1.45;
}

:deep(.el-progress-bar__inner) {
  background: linear-gradient(90deg, #201912, #5d4836);
}

:deep(.el-tag--success),
:deep(.el-tag--primary),
:deep(.el-tag--warning),
:deep(.el-tag--danger),
:deep(.el-tag--info) {
  border: none;
}

:deep(.el-drawer__body) {
  padding-top: 12px;
}
</style>
