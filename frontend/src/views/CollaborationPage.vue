<template>
  <div class="collaboration-page" v-loading="loading">
    <div v-if="session" class="collaboration-shell">
      <section class="collaboration-hero">
        <div>
          <p class="eyebrow">Collaboration workspace</p>
          <h1>{{ session.title }}</h1>
          <p class="subtitle">
            {{ session.bucket }}<span v-if="session.prefix"> / {{ session.prefix }}</span>
            • Created by {{ session.creator }}
          </p>
          <div class="hero-meta">
            <el-tag type="info">{{ session.status }}</el-tag>
            <el-tag v-if="session.expiresAt" type="warning">Expires {{ formatDate(session.expiresAt) }}</el-tag>
            <el-tag>{{ onlineUsers.length }} online</el-tag>
            <el-badge :value="unreadCount" :hidden="!unreadCount">
              <el-tag type="danger">Unread</el-tag>
            </el-badge>
          </div>
        </div>
        <div class="hero-actions">
          <el-input :model-value="collaborationUrl" readonly>
            <template #append>
              <el-button @click="copyToClipboard(collaborationUrl)">Copy link</el-button>
            </template>
          </el-input>
          <div class="hero-action-row">
            <el-button type="primary" @click="refreshSession">Refresh</el-button>
            <el-button plain @click="markLatestRead" :disabled="!messages.length">Mark read</el-button>
            <el-dropdown>
              <el-button>Export transcript</el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item @click="exportTranscript('json')">JSON</el-dropdown-item>
                  <el-dropdown-item @click="exportTranscript('txt')">TXT</el-dropdown-item>
                  <el-dropdown-item @click="exportTranscript('pdf')">PDF</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-button v-if="canManage" @click="saveSessionSettings">Save access list</el-button>
            <el-button v-if="canManage && session.status === 'active'" type="warning" plain @click="closeSessionAction">Close session</el-button>
          </div>
        </div>
      </section>

      <section class="collaboration-grid">
        <aside class="side-panel">
          <el-card shadow="never">
            <template #header>
              <div class="card-header"><span>Members</span><el-tag size="small">{{ allowedUsersWithCreator.length }}</el-tag></div>
            </template>
            <div class="member-list">
              <div v-for="member in allowedUsersWithCreator" :key="member" class="member-item">
                <span>{{ member }}</span>
                <el-tag :type="onlineUsers.includes(member) ? 'success' : 'info'" size="small">
                  {{ onlineUsers.includes(member) ? 'online' : 'offline' }}
                </el-tag>
              </div>
            </div>
            <el-input
              v-if="canManage"
              v-model="allowedUsersDraft"
              type="textarea"
              :rows="5"
              placeholder="One username per line"
            />
          </el-card>

          <el-card shadow="never">
            <template #header>
              <div class="card-header"><span>Video</span><span class="caption">WebRTC signaling</span></div>
            </template>
            <div class="video-actions">
              <el-button type="primary" @click="startVideo">{{ localStream ? 'Reconnect camera' : 'Start camera' }}</el-button>
              <el-button v-if="localStream" @click="stopVideo" plain>Stop</el-button>
            </div>
            <video ref="localVideoRef" class="video-tile" autoplay playsinline muted />
            <div v-if="remoteStreams.length" class="remote-videos">
              <div v-for="remote in remoteStreams" :key="remote.username" class="remote-video-item">
                <p>{{ remote.username }}</p>
                <video :ref="(el) => bindRemoteVideo(el, remote.username)" class="video-tile" autoplay playsinline />
              </div>
            </div>
          </el-card>
        </aside>

        <main class="main-panel">
          <el-card class="chat-card" shadow="never">
            <template #header>
              <div class="card-header">
                <span>Chat</span>
                <div class="chat-header-meta">
                  <span class="caption">Markdown · reply · reactions</span>
                  <el-badge :value="unreadCount" :hidden="!unreadCount">
                    <span class="caption">Unread</span>
                  </el-badge>
                </div>
              </div>
            </template>

            <div class="quick-tools">
              <div class="chip-row">
                <el-button v-for="preset in quickReplies" :key="preset.label" size="small" plain @click="sendQuickReply(preset)">
                  {{ preset.label }}
                </el-button>
              </div>
              <div class="chip-row mention-row">
                <span class="caption">Mention</span>
                <el-button v-for="user in mentionableUsers" :key="user" size="small" text @click="insertMention(user)">
                  @{{ user }}
                </el-button>
              </div>
            </div>

            <el-scrollbar ref="chatScrollRef" class="chat-scroll">
              <div v-if="messages.length" class="message-list">
                <template v-for="(message, index) in messages" :key="message.id">
                  <div v-if="showUnreadMarker(index)" class="unread-marker">
                    <span>{{ unreadCount }} unread message<span v-if="unreadCount !== 1">s</span></span>
                    <el-button size="small" text @click="markLatestRead">Mark read</el-button>
                  </div>
                  <article :class="['message-row', isOwnMessage(message) ? 'is-own' : 'is-other']">
                    <div :class="['message-bubble', message.status === 'recalled' ? 'is-recalled' : '']">
                      <header class="message-header">
                        <strong>{{ message.author }}</strong>
                        <div class="message-meta">
                          <span>{{ formatDate(message.createdAt) }}</span>
                          <el-tag v-if="message.type === 'quick_reply'" size="small" type="success">Quick reply</el-tag>
                          <el-tag v-if="message.status === 'recalled'" size="small" type="warning">Recalled</el-tag>
                        </div>
                      </header>

                      <div v-if="message.replyTo" class="reply-preview">
                        <strong>{{ message.replyTo.author }}</strong>
                        <span>{{ message.replyTo.summary }}</span>
                      </div>

                      <div v-if="message.quickReply" class="quick-reply-pill">{{ message.quickReply }}</div>
                      <div v-if="message.status === 'recalled'" class="message-placeholder">This message was recalled.</div>
                      <div v-else class="message-markdown" v-html="renderMessage(message)" />

                      <div v-if="message.reactions?.length" class="reaction-row">
                        <button
                          v-for="reaction in message.reactions"
                          :key="`${message.id}-${reaction.emoji}`"
                          class="reaction-chip"
                          type="button"
                          @click="reactToMessage(message, reaction.emoji)"
                        >
                          <span>{{ reaction.emoji }}</span>
                          <span>{{ reaction.users?.length || 0 }}</span>
                        </button>
                      </div>

                      <div class="message-actions">
                        <el-button size="small" text @click="setReplyTarget(message)">Reply</el-button>
                        <el-button v-for="emoji in reactionChoices" :key="`${message.id}-${emoji}`" size="small" text @click="reactToMessage(message, emoji)">
                          {{ emoji }}
                        </el-button>
                        <el-button v-if="isOwnMessage(message) && message.status !== 'recalled'" size="small" text @click="recallMessage(message)">Recall</el-button>
                        <el-button v-if="isOwnMessage(message)" size="small" text type="danger" @click="removeMessage(message)">Delete</el-button>
                      </div>
                    </div>
                  </article>
                </template>
              </div>
              <el-empty v-else description="No messages yet" :image-size="72" />
            </el-scrollbar>

            <div class="composer">
              <div v-if="replyTarget" class="replying-banner">
                <div>
                  <strong>Replying to {{ replyTarget.author }}</strong>
                  <p>{{ replyTarget.summary || replyTarget.content }}</p>
                </div>
                <el-button text @click="replyTarget = null">Clear</el-button>
              </div>
              <el-input v-model="messageDraft" type="textarea" :rows="5" placeholder="Type Markdown, mention teammates with @name, or send a quick reply" />
              <div class="composer-actions">
                <div class="emoji-row">
                  <button v-for="emoji in composerEmojis" :key="emoji" class="emoji-chip" type="button" @click="appendEmoji(emoji)">{{ emoji }}</button>
                </div>
                <div class="composer-buttons">
                  <el-button :type="listening ? 'danger' : 'default'" @click="toggleVoiceInput">
                    {{ listening ? 'Stop voice input' : 'Voice input' }}
                  </el-button>
                  <el-button plain @click="markLatestRead" :disabled="!messages.length">Mark read</el-button>
                  <el-button type="primary" :disabled="!canSendMessage" @click="sendMessage">Send</el-button>
                </div>
              </div>
            </div>
          </el-card>
        </main>

        <aside class="side-panel">
          <el-card shadow="never">
            <template #header>
              <div class="card-header"><span>Attachments</span><span class="caption">Upload / download / delete</span></div>
            </template>
            <div class="upload-inline">
              <input ref="attachmentInputRef" type="file" hidden @change="onAttachmentChange" />
              <el-button type="primary" @click="attachmentInputRef?.click()">Upload attachment</el-button>
            </div>
            <div class="file-list">
              <div v-for="attachment in attachments" :key="attachment.id" class="file-item">
                <div>
                  <strong>{{ attachment.name }}</strong>
                  <p>{{ attachment.uploadedBy }} • {{ formatDate(attachment.createdAt) }}</p>
                </div>
                <div class="file-actions">
                  <el-button size="small" @click="downloadAttachment(attachment)">Download</el-button>
                  <el-button size="small" type="danger" plain @click="removeAttachment(attachment)">Delete</el-button>
                </div>
              </div>
              <el-empty v-if="!attachments.length" description="No attachments" :image-size="56" />
            </div>
          </el-card>

          <el-card shadow="never">
            <template #header>
              <div class="card-header"><span>Shared S3 files</span><span class="caption">Reference existing objects</span></div>
            </template>
            <div v-if="canManage" class="s3-browser">
              <el-select v-model="selectedBucket" placeholder="Bucket" style="width: 100%" @change="selectedPrefix = ''; loadBrowserObjects()">
                <el-option v-for="bucket in buckets" :key="bucket.name" :label="bucket.name" :value="bucket.name" />
              </el-select>
              <el-input v-model="selectedPrefix" placeholder="Prefix" @keyup.enter="loadBrowserObjects">
                <template #append><el-button @click="loadBrowserObjects">Load</el-button></template>
              </el-input>
              <el-scrollbar class="browser-scroll">
                <div v-for="item in browserObjects" :key="item.key" class="browser-item">
                  <span class="browser-name" @click="handleBrowserItem(item)">{{ item.name }}</span>
                  <el-button v-if="!item.isDir" size="small" @click="addSharedFile(item)">Add</el-button>
                </div>
              </el-scrollbar>
            </div>
            <div class="file-list">
              <div v-for="item in sharedFiles" :key="item.id" class="file-item">
                <div>
                  <strong>{{ item.name }}</strong>
                  <p>{{ item.bucket }}/{{ item.key }}</p>
                </div>
                <div class="file-actions">
                  <el-button size="small" @click="downloadSharedFile(item)">Download</el-button>
                  <el-button v-if="canManage" size="small" type="danger" plain @click="removeSharedFile(item)">Remove</el-button>
                </div>
              </div>
              <el-empty v-if="!sharedFiles.length" description="No shared files" :image-size="56" />
            </div>
          </el-card>
        </aside>
      </section>
    </div>
    <el-empty v-else-if="!loading" description="Collaboration session unavailable" :image-size="80" />
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  AUTH_TOKEN_STORAGE_KEY,
  closeCollaborationSession,
  createCollaborationAttachment,
  createCollaborationMessage,
  createCollaborationSharedFile,
  createCollaborationStreamToken,
  deleteCollaborationAttachment,
  deleteCollaborationMessage,
  deleteCollaborationSharedFile,
  exportCollaborationTranscript,
  getCollaborationSession,
  listBuckets,
  listObjects,
  markCollaborationRead,
  publishCollaborationSignal,
  recallCollaborationMessage,
  toggleCollaborationReaction,
  updateCollaborationSession
} from '../api'
import { useAuth } from '../auth'

const quickReplies = [
  { label: 'Acknowledged', content: 'Acknowledged.', quickReply: '✅ Acknowledged' },
  { label: 'On my way', content: 'On my way.', quickReply: '🚀 On my way' },
  { label: 'Need info', content: 'I need more details to proceed.', quickReply: '❓ Need info' }
]
const reactionChoices = ['👍', '🎯', '🔥', '✅']
const composerEmojis = ['😀', '👍', '🎉', '🤝', '🚀']

const route = useRoute()
const router = useRouter()
const { currentUser } = useAuth()

const loading = ref(true)
const session = ref(null)
const onlineUsers = ref([])
const messages = ref([])
const attachments = ref([])
const sharedFiles = ref([])
const allowedUsersDraft = ref('')
const messageDraft = ref('')
const replyTarget = ref(null)
const listening = ref(false)
const unreadCount = ref(0)
const lastReadMessageId = ref('')
const attachmentInputRef = ref(null)
const chatScrollRef = ref(null)
const buckets = ref([])
const browserObjects = ref([])
const selectedBucket = ref('')
const selectedPrefix = ref('')
const localVideoRef = ref(null)
const localStream = ref(null)
const remoteStreams = ref([])

let speechRecognition = null
let eventSource = null
const peerConnections = new Map()

const token = computed(() => route.params.token)
const canManage = computed(() => Boolean(session.value?.canManage))
const collaborationUrl = computed(() => `${window.location.origin}/collaboration/${token.value}`)
const currentUsername = computed(() => session.value?.currentUsername || currentUser.value?.username || '')
const allowedUsersWithCreator = computed(() => (session.value ? [session.value.creator, ...(session.value.allowedUsers || [])] : []))
const mentionableUsers = computed(() => session.value?.mentionableUsers || allowedUsersWithCreator.value)
const canSendMessage = computed(() => Boolean(messageDraft.value.trim() || replyTarget.value))
const firstUnreadIndex = computed(() => {
  if (!messages.value.length || unreadCount.value <= 0) return -1
  if (!lastReadMessageId.value) return Math.max(messages.value.length - unreadCount.value, 0)
  const index = messages.value.findIndex((item) => item.id === lastReadMessageId.value)
  return index < 0 ? Math.max(messages.value.length - unreadCount.value, 0) : Math.min(index + 1, messages.value.length - 1)
})

onMounted(async () => {
  await Promise.all([refreshSession(), loadBuckets()])
})

onUnmounted(() => {
  disconnectStream()
  stopVoiceRecognition()
  stopVideo()
})

watch(localVideoRef, () => attachLocalVideo())

async function refreshSession() {
  loading.value = true
  try {
    const { data } = await getCollaborationSession(token.value)
    applySession(data)
    await connectStream()
    if (!selectedBucket.value) {
      selectedBucket.value = data.bucket || ''
      selectedPrefix.value = data.prefix || ''
    }
    if (selectedBucket.value) await loadBrowserObjects()
    await nextTick()
    scrollChatToBottom()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
    session.value = null
  } finally {
    loading.value = false
  }
}

function applySession(data) {
  session.value = data
  onlineUsers.value = data.onlineUsers || []
  messages.value = data.messages || []
  attachments.value = data.attachments || []
  sharedFiles.value = data.sharedFiles || []
  allowedUsersDraft.value = (data.allowedUsers || []).join('\n')
  unreadCount.value = data.unreadCount || 0
  lastReadMessageId.value = data.lastReadMessageId || ''
}

async function connectStream() {
  disconnectStream()
  const { data } = await createCollaborationStreamToken(token.value)
  eventSource = new EventSource(`/api/v1/collaboration/sessions/${encodeURIComponent(token.value)}/stream?streamToken=${encodeURIComponent(data.streamToken)}`)
  eventSource.addEventListener('update', async (event) => {
    const payload = JSON.parse(event.data)
    await handleRealtimeEvent(payload)
  })
  eventSource.onerror = () => {
    if (eventSource?.readyState === EventSource.CLOSED) disconnectStream()
  }
}

function disconnectStream() {
  if (!eventSource) return
  eventSource.close()
  eventSource = null
}

async function handleRealtimeEvent(event) {
  const payload = event.payload || {}
  switch (event.type) {
    case 'presence':
      onlineUsers.value = payload.onlineUsers || []
      await maybeInitiateOffers()
      break
    case 'session.updated':
    case 'session.closed':
      applySession({ ...(session.value || {}), ...payload, onlineUsers: onlineUsers.value })
      break
    case 'session.deleted':
      ElMessage.warning('This collaboration session was deleted.')
      await router.replace({ name: 'browser' })
      break
    case 'message.created':
      messages.value = upsertMessage(payload)
      if (!isOwnAuthor(payload.author)) unreadCount.value += 1
      await nextTick()
      scrollChatToBottom()
      break
    case 'message.recalled':
    case 'reaction.changed':
      messages.value = upsertMessage(payload)
      break
    case 'message.deleted':
      if (payload.username === currentUsername.value) {
        messages.value = messages.value.filter((item) => item.id !== payload.messageId)
      }
      break
    case 'read.updated':
      if (payload.username === currentUsername.value) {
        unreadCount.value = payload.unreadCount || 0
        lastReadMessageId.value = payload.lastReadMessageId || latestMessageId()
      }
      break
    case 'attachment.created':
      attachments.value = [payload, ...attachments.value.filter((item) => item.id !== payload.id)]
      break
    case 'attachment.deleted':
      attachments.value = attachments.value.filter((item) => item.id !== payload.id)
      break
    case 'shared-file.created':
      sharedFiles.value = [payload, ...sharedFiles.value.filter((item) => item.id !== payload.id)]
      break
    case 'shared-file.deleted':
      sharedFiles.value = sharedFiles.value.filter((item) => item.id !== payload.id)
      break
    case 'signal':
      await handleSignal(payload)
      break
    default:
      break
  }
}

function upsertMessage(message) {
  return [...messages.value.filter((item) => item.id !== message.id), message].sort(
    (left, right) => new Date(left.createdAt) - new Date(right.createdAt)
  )
}

function isOwnAuthor(author) {
  return currentUsername.value && author === currentUsername.value
}

function isOwnMessage(message) {
  return isOwnAuthor(message.author)
}

function showUnreadMarker(index) {
  return unreadCount.value > 0 && index === firstUnreadIndex.value
}

async function saveSessionSettings() {
  if (!session.value) return
  try {
    const { data } = await updateCollaborationSession(token.value, {
      title: session.value.title,
      bucket: session.value.bucket,
      allowedUsers: parseAllowedUsers(allowedUsersDraft.value),
      expiresAt: session.value.expiresAt || ''
    })
    applySession({ ...data, onlineUsers: onlineUsers.value, unreadCount: unreadCount.value, lastReadMessageId: lastReadMessageId.value })
    ElMessage.success('Access list updated')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function closeSessionAction() {
  try {
    await ElMessageBox.confirm('Close this collaboration session?', 'Close Session', { type: 'warning' })
    const { data } = await closeCollaborationSession(token.value)
    applySession({ ...data, onlineUsers: onlineUsers.value })
    ElMessage.success('Session closed')
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function sendMessage() {
  const content = messageDraft.value.trim()
  if (!content && !replyTarget.value) return
  try {
    await createCollaborationMessage(token.value, {
      content,
      replyToId: replyTarget.value?.id || '',
      mentionedUsers: extractMentionedUsers(content),
      type: 'markdown'
    })
    messageDraft.value = ''
    replyTarget.value = null
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function sendQuickReply(preset) {
  try {
    await createCollaborationMessage(token.value, {
      content: preset.content,
      quickReply: preset.quickReply,
      replyToId: replyTarget.value?.id || '',
      mentionedUsers: extractMentionedUsers(preset.content),
      type: 'quick_reply'
    })
    replyTarget.value = null
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function reactToMessage(message, emoji) {
  try {
    await toggleCollaborationReaction(token.value, message.id, { emoji })
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function recallMessage(message) {
  try {
    await recallCollaborationMessage(token.value, message.id)
    ElMessage.success('Message recalled')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function removeMessage(message) {
  try {
    await ElMessageBox.confirm('Delete this message only from your view?', 'Delete Message', { type: 'warning' })
    await deleteCollaborationMessage(token.value, message.id)
    ElMessage.success('Message deleted')
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function markLatestRead() {
  if (!messages.value.length) return
  try {
    const { data } = await markCollaborationRead(token.value, { messageId: latestMessageId() })
    unreadCount.value = data.unreadCount || 0
    lastReadMessageId.value = data.lastReadMessageId || latestMessageId()
    ElMessage.success('Unread state updated')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

function latestMessageId() {
  return messages.value[messages.value.length - 1]?.id || ''
}

async function exportTranscript(format) {
  try {
    const { data, headers } = await exportCollaborationTranscript(token.value, format)
    const objectUrl = window.URL.createObjectURL(data)
    const link = document.createElement('a')
    const contentDisposition = headers['content-disposition'] || ''
    const match = /filename="?([^";]+)"?/i.exec(contentDisposition)
    link.href = objectUrl
    link.download = match?.[1] || `collaboration-transcript.${format}`
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(objectUrl)
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

function setReplyTarget(message) {
  replyTarget.value = message
}

function insertMention(username) {
  messageDraft.value = `${messageDraft.value}${messageDraft.value.endsWith(' ') || !messageDraft.value ? '' : ' '}@${username} `
}

function appendEmoji(emoji) {
  messageDraft.value = `${messageDraft.value}${emoji}`
}

function extractMentionedUsers(content) {
  return [...new Set(Array.from(content.matchAll(/(^|\s)@([A-Za-z0-9._-]{3,64})/g)).map((item) => item[2]))]
}

function renderMessage(message) {
  const source = message.content || ''
  const sections = source.split(/\n{2,}/).map((item) => item.trim()).filter(Boolean)
  return sections.map(renderBlock).join('')
}

function renderBlock(block) {
  if (block.startsWith('>')) {
    const quote = block.split('\n').map((line) => line.replace(/^>\s?/, '')).join('<br>')
    return `<blockquote>${renderInline(quote)}</blockquote>`
  }
  if (/^[-*]\s+/m.test(block)) {
    const items = block
      .split('\n')
      .filter(Boolean)
      .map((line) => `<li>${renderInline(line.replace(/^[-*]\s+/, ''))}</li>`)
      .join('')
    return `<ul>${items}</ul>`
  }
  const heading = block.match(/^(#{1,3})\s+(.+)$/)
  if (heading) {
    const level = Math.min(heading[1].length + 1, 4)
    return `<h${level}>${renderInline(heading[2])}</h${level}>`
  }
  return `<p>${renderInline(block.replace(/\n/g, '<br>'))}</p>`
}

function renderInline(value) {
  return value
    .replace(/\[(.+?)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/\*(.+?)\*/g, '<em>$1</em>')
    .replace(/~~(.+?)~~/g, '<del>$1</del>')
    .replace(/`(.+?)`/g, '<code>$1</code>')
    .replace(/(^|\s)@([A-Za-z0-9._-]{3,64})/g, '$1<span class="mention">@$2</span>')
}

function toggleVoiceInput() {
  const Recognition = window.SpeechRecognition || window.webkitSpeechRecognition
  if (!Recognition) {
    ElMessage.warning('Voice input is not supported in this browser')
    return
  }
  if (listening.value) return stopVoiceRecognition()
  speechRecognition = new Recognition()
  speechRecognition.lang = normalizeRecognitionLanguage(navigator.language)
  speechRecognition.interimResults = true
  speechRecognition.continuous = true
  speechRecognition.onresult = (event) => {
    const transcript = Array.from(event.results)
      .slice(event.resultIndex)
      .map((result) => result[0]?.transcript || '')
      .join(' ')
    messageDraft.value = `${messageDraft.value} ${transcript}`.trim()
  }
  speechRecognition.onend = () => {
    listening.value = false
  }
  speechRecognition.onerror = () => {
    listening.value = false
  }
  listening.value = true
  speechRecognition.start()
}

function stopVoiceRecognition() {
  if (speechRecognition) {
    speechRecognition.stop()
    speechRecognition = null
  }
  listening.value = false
}

async function onAttachmentChange(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  try {
    await createCollaborationAttachment(token.value, file)
    ElMessage.success('Attachment uploaded')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function removeAttachment(attachment) {
  try {
    await ElMessageBox.confirm(`Delete attachment "${attachment.name}"?`, 'Delete Attachment', { type: 'warning' })
    await deleteCollaborationAttachment(token.value, attachment.id)
    ElMessage.success('Attachment deleted')
  } catch (error) {
    if (error !== 'cancel') ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function downloadAttachment(attachment) {
  await downloadAuthorized(`/api/v1/collaboration/sessions/${encodeURIComponent(token.value)}/attachments/${encodeURIComponent(attachment.id)}/download`, attachment.name)
}

async function loadBuckets() {
  try {
    const { data } = await listBuckets()
    buckets.value = data || []
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function loadBrowserObjects() {
  if (!selectedBucket.value) return
  try {
    const { data } = await listObjects(selectedBucket.value, selectedPrefix.value)
    browserObjects.value = data || []
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

function handleBrowserItem(item) {
  if (!item.isDir) return
  selectedPrefix.value = item.key
  loadBrowserObjects()
}

async function addSharedFile(item) {
  try {
    await createCollaborationSharedFile(token.value, {
      bucket: selectedBucket.value,
      key: item.key,
      name: item.name
    })
    ElMessage.success('Shared file added')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function removeSharedFile(item) {
  try {
    await deleteCollaborationSharedFile(token.value, item.id)
    ElMessage.success('Shared file removed')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function downloadSharedFile(item) {
  await downloadAuthorized(`/api/v1/collaboration/sessions/${encodeURIComponent(token.value)}/files/${encodeURIComponent(item.id)}/download`, item.name)
}

async function downloadAuthorized(url, filename) {
  try {
    const authToken = window.localStorage.getItem(AUTH_TOKEN_STORAGE_KEY)
    const response = await fetch(url, {
      headers: authToken ? { Authorization: `Bearer ${authToken}` } : {}
    })
    if (!response.ok) {
      const data = await response.json().catch(() => ({}))
      throw new Error(data.error || `Download failed (HTTP ${response.status})`)
    }
    const blob = await response.blob()
    const objectUrl = window.URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = objectUrl
    link.download = filename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
    window.URL.revokeObjectURL(objectUrl)
  } catch (error) {
    ElMessage.error(error.message)
  }
}

function scrollChatToBottom() {
  const wrap = chatScrollRef.value?.wrapRef
  if (wrap) wrap.scrollTop = wrap.scrollHeight
}

async function startVideo() {
  try {
    await ensureLocalStream()
    await maybeInitiateOffers()
  } catch (error) {
    ElMessage.error(error.message || 'Unable to start video')
  }
}

function stopVideo() {
  for (const username of peerConnections.keys()) closePeer(username)
  if (localStream.value) {
    for (const track of localStream.value.getTracks()) track.stop()
    localStream.value = null
  }
  remoteStreams.value = []
  attachLocalVideo()
}

async function ensureLocalStream() {
  if (localStream.value) return localStream.value
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: true })
  localStream.value = stream
  attachLocalVideo()
  return stream
}

function attachLocalVideo() {
  if (localVideoRef.value) localVideoRef.value.srcObject = localStream.value || null
}

async function maybeInitiateOffers() {
  if (!localStream.value || !currentUser.value?.username) return
  for (const username of onlineUsers.value) {
    if (username === currentUser.value.username) continue
    if (!shouldInitiateOffer(username)) continue
    await createOffer(username)
  }
}

function shouldInitiateOffer(username) {
  return currentUser.value?.username && currentUser.value.username.localeCompare(username) < 0
}

async function createOffer(username) {
  const connection = await ensurePeerConnection(username)
  if (connection.signalingState !== 'stable') return
  const offer = await connection.createOffer()
  await connection.setLocalDescription(offer)
  await publishSignal({ to: username, description: connection.localDescription })
}

async function ensurePeerConnection(username) {
  if (peerConnections.has(username)) return peerConnections.get(username)
  const connection = new RTCPeerConnection({ iceServers: [{ urls: 'stun:stun.l.google.com:19302' }] })
  if (localStream.value) {
    for (const track of localStream.value.getTracks()) connection.addTrack(track, localStream.value)
  }
  connection.onicecandidate = (event) => {
    if (event.candidate) publishSignal({ to: username, candidate: event.candidate })
  }
  connection.ontrack = (event) => {
    setRemoteStream(username, event.streams[0])
  }
  connection.onconnectionstatechange = () => {
    if (['disconnected', 'failed', 'closed'].includes(connection.connectionState)) closePeer(username)
  }
  peerConnections.set(username, connection)
  return connection
}

function setRemoteStream(username, stream) {
  remoteStreams.value = [...remoteStreams.value.filter((item) => item.username !== username), { username, stream }]
}

function bindRemoteVideo(element, username) {
  if (!element) return
  const remote = remoteStreams.value.find((item) => item.username === username)
  if (remote) element.srcObject = remote.stream
}

function closePeer(username) {
  const connection = peerConnections.get(username)
  if (connection) {
    connection.onicecandidate = null
    connection.ontrack = null
    connection.close()
    peerConnections.delete(username)
  }
  remoteStreams.value = remoteStreams.value.filter((item) => item.username !== username)
}

async function handleSignal(payload) {
  const from = payload.from
  if (!from || from === currentUser.value?.username) return
  if (payload.to && payload.to !== currentUser.value?.username) return
  if (payload.type === 'hangup') {
    closePeer(from)
    return
  }
  const connection = await ensurePeerConnection(from)
  if (payload.description) {
    if (payload.description.type === 'offer') {
      await ensureLocalStream()
      if (!connection.getSenders().length && localStream.value) {
        for (const track of localStream.value.getTracks()) connection.addTrack(track, localStream.value)
      }
      await connection.setRemoteDescription(payload.description)
      const answer = await connection.createAnswer()
      await connection.setLocalDescription(answer)
      await publishSignal({ to: from, description: connection.localDescription })
    } else if (payload.description.type === 'answer') {
      await connection.setRemoteDescription(payload.description)
    }
  }
  if (payload.candidate) {
    try {
      await connection.addIceCandidate(payload.candidate)
    } catch (error) {
      console.debug('Ignoring transient ICE candidate issue', error)
    }
  }
}

async function publishSignal(payload) {
  try {
    await publishCollaborationSignal(token.value, payload)
  } catch (error) {
    ElMessage.error(error.response?.data?.error || error.message)
  }
}

async function copyToClipboard(value) {
  try {
    await navigator.clipboard.writeText(value)
    ElMessage.success('Copied to clipboard')
  } catch {
    ElMessage.error('Failed to copy')
  }
}

function parseAllowedUsers(value) {
  return value.split(/[,\n]/).map((item) => item.trim()).filter(Boolean)
}

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : ''
}

function normalizeRecognitionLanguage(value) {
  const language = typeof value === 'string' ? value.trim() : ''
  if (!language) return 'en-US'
  if (/^[a-z]{2}$/i.test(language)) {
    const defaults = { en: 'en-US', zh: 'zh-CN', ja: 'ja-JP', ko: 'ko-KR', fr: 'fr-FR', de: 'de-DE', es: 'es-ES' }
    return defaults[language.toLowerCase()] || 'en-US'
  }
  if (/^[a-z]{2}-[a-z]{2}$/i.test(language)) {
    const [base, region] = language.split('-')
    return `${base.toLowerCase()}-${region.toUpperCase()}`
  }
  return 'en-US'
}
</script>

<style scoped>
.collaboration-page { padding: 28px 0 40px; }
.collaboration-shell { display: flex; flex-direction: column; gap: 20px; }
.collaboration-hero, .collaboration-grid { display: grid; gap: 20px; }
.collaboration-hero { grid-template-columns: 1.4fr 1fr; align-items: start; }
.collaboration-grid { grid-template-columns: 280px minmax(0, 1fr) 320px; }
.eyebrow, .subtitle, .caption, .file-item p, .replying-banner p, .remote-video-item p { margin: 0; color: var(--kip-text-muted); }
h1 { margin: 8px 0 12px; font-size: 36px; line-height: 1.1; }
.hero-meta, .hero-action-row, .video-actions, .composer-actions, .file-actions, .chat-header-meta, .message-actions, .message-meta, .reaction-row, .chip-row, .composer-buttons, .emoji-row { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
.hero-actions, .side-panel, .main-panel, .chat-card, .s3-browser, .composer, .file-list, .member-list, .remote-videos, .quick-tools { display: flex; flex-direction: column; gap: 14px; }
.card-header { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.chat-scroll { height: 520px; }
.message-list { display: flex; flex-direction: column; gap: 14px; }
.message-row { display: flex; }
.message-row.is-own { justify-content: flex-end; }
.message-bubble { max-width: min(78%, 720px); padding: 14px 16px; border-radius: 24px; border: 1px solid var(--kip-border); background: rgba(255, 252, 245, 0.88); box-shadow: 0 18px 40px rgba(59, 43, 31, 0.06); }
.message-row.is-own .message-bubble { background: rgba(32, 25, 18, 0.92); color: #f9f3ea; }
.message-row.is-own .message-bubble :deep(a), .message-row.is-own .message-bubble :deep(code), .message-row.is-own .message-bubble :deep(blockquote), .message-row.is-own .message-bubble :deep(.mention) { color: inherit; }
.message-bubble.is-recalled { opacity: 0.82; }
.message-header, .member-item, .file-item, .browser-item { display: flex; justify-content: space-between; gap: 12px; align-items: center; }
.reply-preview, .replying-banner, .unread-marker { border: 1px solid var(--kip-border); background: rgba(237, 226, 210, 0.55); border-radius: 18px; padding: 10px 12px; }
.quick-reply-pill { display: inline-flex; align-items: center; padding: 6px 10px; border-radius: 999px; background: rgba(237, 226, 210, 0.85); color: var(--kip-text); font-size: 13px; font-weight: 600; }
.message-placeholder { font-style: italic; color: inherit; }
.message-markdown :deep(p), .message-markdown :deep(ul), .message-markdown :deep(blockquote), .message-markdown :deep(h2), .message-markdown :deep(h3), .message-markdown :deep(h4) { margin: 10px 0 0; }
.message-markdown :deep(ul) { padding-left: 20px; }
.message-markdown :deep(code) { padding: 2px 6px; border-radius: 8px; background: rgba(32, 25, 18, 0.08); }
.message-markdown :deep(blockquote) { margin-left: 0; padding-left: 12px; border-left: 3px solid rgba(32, 25, 18, 0.28); }
.message-markdown :deep(.mention) { font-weight: 700; color: #a84300; }
.reaction-chip, .emoji-chip { border: 1px solid var(--kip-border); background: rgba(255, 252, 245, 0.82); border-radius: 999px; padding: 6px 10px; cursor: pointer; }
.message-row.is-own .reaction-chip { background: rgba(255, 255, 255, 0.14); color: #f9f3ea; }
.browser-scroll { max-height: 180px; }
.browser-name { cursor: pointer; }
.file-item, .member-item, .browser-item, .unread-marker { padding: 10px 12px; border: 1px solid var(--kip-border); border-radius: 16px; }
.upload-inline { display: flex; }
@media (max-width: 1280px) {
  .collaboration-grid { grid-template-columns: 1fr; }
  .collaboration-hero { grid-template-columns: 1fr; }
  .message-bubble { max-width: 100%; }
}
</style>
