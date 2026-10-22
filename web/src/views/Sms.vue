<script setup lang="ts">
import { notify, requestConfirmation } from '../stores/feedback'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSMSStore } from '../stores/sms'
import { usePollingScheduler } from '../composables/usePollingScheduler'
import { useSmsThreadReadState } from '../composables/useSmsThreadReadState'
import { toAppError } from '../services/http'
import type { SmsThreadQueryParams } from '../services/sms'
import PageHeader from '../components/PageHeader.vue'
import SvgIcon from '../components/SvgIcon.vue'
import deleteIcon from '@material-symbols/svg-400/outlined/delete.svg?raw'
import sendIcon from '@material-symbols/svg-400/outlined/send.svg?raw'
import { fieldValue, guardDialogCancel } from '../utils/material'
import EmptyState from '../components/EmptyState.vue'
import ErrorState from '../components/ErrorState.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import RefreshButton from '../components/RefreshButton.vue'
import type { DeviceMgmtListItem, SMSMessage } from '../types/api'
import { RecycleScroller } from 'vue-virtual-scroller'
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'

type SmsThread = {
  key: string
  imsi: string
  peer: string
  deviceId?: string
  lastTs: number
  lastSmsId: number
  lastMessage: string
  lastDeviceName?: string
  localPhone?: string
  peerLower: string
  lastMessageLower: string
}

const route = useRoute()
const router = useRouter()
const smsStore = useSMSStore()
const { getLastSeen, setLastSeen } = useSmsThreadReadState()

const devices = ref<DeviceMgmtListItem[]>([])
const devicesLastOkAt = ref<number | null>(null)
const devicesError = ref<{ message: string; status?: number; method?: string; url?: string } | null>(null)

const loading = ref(false)
const threads = ref<SmsThread[]>([])
const messagesLastOkAt = ref<number | null>(null)
const messagesError = ref<{ message: string; status?: number; method?: string; url?: string } | null>(null)

const selectedDevice = ref<string>(typeof route.query.device === 'string' ? route.query.device : 'all')
const selectedThreadKey = ref<string>(typeof route.query.contact === 'string' ? route.query.contact : '')
const searchQuery = ref('')

const smsPageRef = ref<HTMLElement | null>(null)
const smsPageWidth = ref(0)
const SMS_NARROW_BREAKPOINT = 980
let smsPageResizeObserver: ResizeObserver | null = null

function syncSmsPageWidth() {
  smsPageWidth.value = smsPageRef.value?.clientWidth || 0
}

function parseTs(s: string) {
  const ms = new Date(s).getTime()
  return Number.isFinite(ms) ? ms : 0
}

function formatClock(ms: number) {
  return ms ? new Date(ms).toLocaleTimeString() : ''
}

function dateKey(ms: number) {
  const t = new Date(ms)
  if (!Number.isFinite(t.getTime())) return '未知日期'
  return `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`
}

const filteredThreads = computed(() => {
  const q = String(searchQuery.value || '').trim().toLowerCase()
  if (!q) return threads.value
  return threads.value.filter(t => {
    if (t.peerLower.includes(q)) return true
    return t.lastMessageLower.includes(q)
  })
})

const selectedThread = computed(() => {
  return threads.value.find(t => t.key === selectedThreadKey.value) || null
})

const loadingHistoryMore = ref(false)
const threadLoading = ref(false)
const threadMessages = ref<SMSMessage[]>([])
const threadHasMore = ref(false)

const canLoadMoreHistory = computed(() => {
  return !!selectedThread.value && threadHasMore.value
})

const selectedThreadGroups = computed(() => {
  if (!selectedThread.value) return []
  const out: Array<{ date: string; items: SMSMessage[] }> = []
  let last = ''
  for (const m of threadMessages.value) {
    const key = dateKey(parseTs(m.timestamp))
    if (!out.length || key !== last) {
      out.push({ date: key, items: [m] })
      last = key
    } else {
      out[out.length - 1].items.push(m)
    }
  }
  return out
})

const isNarrowLayout = computed(() => smsPageWidth.value > 0 && smsPageWidth.value < SMS_NARROW_BREAKPOINT)
const showDeviceSidebar = computed(() => !isNarrowLayout.value)
const showListPane = computed(() => !isNarrowLayout.value || !selectedThreadKey.value)
const showDetailPane = computed(() => !isNarrowLayout.value || !!selectedThreadKey.value)

const showSendModal = ref(false)
const sending = ref(false)
const deletingMessageId = ref<number | null>(null)
const deletingThreadKey = ref<string | null>(null)
const showActionSheet = ref(false)
const actionSheetTarget = ref<{ type: 'thread'; thread: SmsThread } | { type: 'message'; message: SMSMessage } | null>(null)
const composer = ref('')
const GSM7_BASIC_CHARS = new Set(Array.from(`@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ !"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà`))
const GSM7_EXT_CHARS = new Set(Array.from(`^{}\\[~]|€`))

function estimateSegments(text: string) {
  const raw = String(text || '')
  let gsm7Units = 0
  let isGSM7 = true
  for (const ch of Array.from(raw)) {
    if (GSM7_BASIC_CHARS.has(ch)) {
      gsm7Units += 1
      continue
    }
    if (GSM7_EXT_CHARS.has(ch)) {
      gsm7Units += 2
      continue
    }
    isGSM7 = false
    break
  }
  if (isGSM7) {
    const single = 160
    const multi = 153
    const parts = gsm7Units <= single ? 1 : Math.ceil(gsm7Units / multi)
    return { encoding: 'GSM7', parts, units: gsm7Units, unitName: 'septets' }
  }
  const ucs2Units = Array.from(raw).length
  const single = 70
  const multi = 67
  const parts = ucs2Units <= single ? 1 : Math.ceil(ucs2Units / multi)
  return { encoding: 'UCS2', parts, units: ucs2Units, unitName: 'chars' }
}

const composerLen = computed(() => Array.from(String(composer.value || '')).length)
const composerEstimate = computed(() => estimateSegments(String(composer.value || '')))
const detailScrollbar = ref<HTMLElement | null>(null)
const composerInput = ref<unknown>(null)

const sendForm = ref({
  device_id: '',
  phone: '',
  message: ''
})
const sendEstimate = computed(() => estimateSegments(String(sendForm.value.message || '')))

const selectedSendDeviceId = ref('')
const sendDeviceOptions = computed(() => devices.value.map(d => ({ label: `${d.name || d.id}`, value: d.id })))

const deviceSidebarItems = computed(() => {
  return [
    { id: 'all', label: '全部设备', healthy: true },
    ...devices.value.map(d => ({ id: d.id, label: d.name || d.id, healthy: !!d.running && (d.control_online ?? d.healthy) === true }))
  ]
})

function normalizeQueryDevice(device: string) {
  const v = String(device || '').trim()
  if (!v || v === 'all') return undefined
  return v
}

function buildSmsQuery(device: string, contact?: string) {
  const nextContact = String(contact || '').trim()
  return {
    ...route.query,
    device: normalizeQueryDevice(device),
    contact: nextContact || undefined
  }
}

function markThreadSeen(t: SmsThread | null) {
  if (!t) return
  setLastSeen(t.key, t.lastTs)
}

function isUnread(t: SmsThread) {
  return t.lastTs > getLastSeen(t.key)
}

function backToList() {
  if (!selectedThreadKey.value) return
  clearSelectedThread(true)
}

function scrollThreadToBottom() {
  const doScroll = () => {
    const wrap = detailScrollbar.value
    if (!wrap) return
    wrap.scrollTop = wrap.scrollHeight
  }
  // 第一次：DOM 更新后立即滚动
  nextTick(() => {
    requestAnimationFrame(doScroll)
    // 第二次：延迟 150ms 补偿大量消息渲染延迟
    setTimeout(doScroll, 150)
  })
}

function getDetailWrap() {
  return detailScrollbar.value
}

function isNearBottom(thresholdPx = 160) {
  const wrap = detailScrollbar.value
  if (!wrap) return true
  const distance = wrap.scrollHeight - (wrap.scrollTop + wrap.clientHeight)
  return distance <= thresholdPx
}

async function loadMoreHistory() {
  if (!canLoadMoreHistory.value) return
  if (loadingHistoryMore.value) return
  if (!selectedThread.value) return
  if (threadMessages.value.length === 0) return
  const wrap = getDetailWrap()
  const prevTop = wrap?.scrollTop || 0
  const prevHeight = wrap?.scrollHeight || 0
  loadingHistoryMore.value = true
  try {
    const oldest = threadMessages.value[0]
    const params: SmsThreadQueryParams = {
      peer: selectedThread.value.peer,
      limit: 80,
      before_ts: oldest.timestamp,
      before_id: oldest.id
    }
    if (selectedDevice.value && selectedDevice.value !== 'all') {
      params.device_id = selectedDevice.value
    } else {
      params.device_id = 'all'
      params.imsi = selectedThread.value.imsi
    }
    const result = await smsStore.fetchThread(params)
    if (!result.ok) throw new Error(result.error.message)
    const list = (result.data || []) as SMSMessage[]
    const merged = list.slice().sort((a, b) => parseTs(a.timestamp) - parseTs(b.timestamp) || a.id - b.id).concat(threadMessages.value)
    threadMessages.value = merged
    threadHasMore.value = list.length === params.limit
    await nextTick()
    requestAnimationFrame(() => {
      const w = getDetailWrap()
      if (!w) return
      const nextHeight = w.scrollHeight
      const delta = Math.max(0, nextHeight - prevHeight)
      w.scrollTop = prevTop + delta
    })
  } catch {
    // Ignore history load errors to keep current thread state unchanged.
  } finally {
    loadingHistoryMore.value = false
  }
}

function onDetailScroll(e: Event) {
  clearLongPress()
  const target = e.target as HTMLElement
  if (target && target.scrollTop <= 80) {
    loadMoreHistory()
  }
}

let devicesFetchSeq = 0
let messagesFetchSeq = 0
let threadFetchSeq = 0
let longPressTimer: ReturnType<typeof setTimeout> | null = null
let longPressStartX = 0
let longPressStartY = 0

function clearLongPress() {
  if (longPressTimer != null) {
    clearTimeout(longPressTimer)
    longPressTimer = null
  }
}

function startLongPress(target: { type: 'thread'; thread: SmsThread } | { type: 'message'; message: SMSMessage }, e: PointerEvent) {
  if (!isNarrowLayout.value) return
  if (e.pointerType === 'mouse') return
  clearLongPress()
  longPressStartX = e.clientX
  longPressStartY = e.clientY
  longPressTimer = setTimeout(() => {
    longPressTimer = null
    actionSheetTarget.value = target
    showActionSheet.value = true
    if (typeof navigator !== 'undefined' && typeof navigator.vibrate === 'function') {
      navigator.vibrate(20)
    }
  }, 450)
}

function moveLongPress(e: PointerEvent) {
  if (longPressTimer == null) return
  if (Math.abs(e.clientX - longPressStartX) > 10 || Math.abs(e.clientY - longPressStartY) > 10) {
    clearLongPress()
  }
}

function openThreadActionSheet(thread: SmsThread, e: PointerEvent) {
  startLongPress({ type: 'thread', thread }, e)
}

function openMessageActionSheet(message: SMSMessage, e: PointerEvent) {
  startLongPress({ type: 'message', message }, e)
}

function closeActionSheet() {
  showActionSheet.value = false
  actionSheetTarget.value = null
}

async function onActionSheetDelete() {
  const target = actionSheetTarget.value
  closeActionSheet()
  if (!target) return
  if (target.type === 'thread') {
    await confirmDeleteThread(target.thread)
    return
  }
  await confirmDeleteMessage(target.message)
}

function clearSelectedThread(syncRoute = false) {
  threadFetchSeq += 1
  selectedThreadKey.value = ''
  threadMessages.value = []
  threadHasMore.value = false
  threadLoading.value = false
  if (syncRoute) {
    void router.replace({ query: buildSmsQuery(selectedDevice.value) })
  }
}

async function fetchDevices() {
  const seq = ++devicesFetchSeq
  devicesError.value = null
  const result = await smsStore.fetchDevices()
  if (seq !== devicesFetchSeq) return false
  if (result.ok) {
    devices.value = (result.data || []) as DeviceMgmtListItem[]
    devicesLastOkAt.value = Date.now()
    if (selectedDevice.value !== 'all' && !devices.value.some(d => d.id === selectedDevice.value)) {
      selectedDevice.value = 'all'
      clearSelectedThread(false)
      void router.replace({ query: buildSmsQuery('all') })
    }
    return true
  }
  devicesError.value = result.error
  return false
}

async function fetchMessages(silent = false) {
  const seq = ++messagesFetchSeq
  if (!silent) loading.value = true
  messagesError.value = null
  const wasNearBottom = isNearBottom()
  const result = await smsStore.fetchThreads(selectedDevice.value)
  if (seq !== messagesFetchSeq) return false
  if (result.ok) {
    threads.value = (result.data || []) as SmsThread[]
    messagesLastOkAt.value = Date.now()
  } else {
    messagesError.value = result.error
  }
  if (!silent) loading.value = false
  if (result.ok && selectedThreadKey.value && wasNearBottom) {
    scrollThreadToBottom()
  }
  return result.ok
}

async function fetchThreadLatest(silent = false) {
  const t = selectedThread.value
  if (!t) {
    threadMessages.value = []
    threadHasMore.value = false
    return false
  }
  const seq = ++threadFetchSeq
  if (!silent) threadLoading.value = true
  const params: SmsThreadQueryParams = { peer: t.peer, limit: 80 }
  if (selectedDevice.value && selectedDevice.value !== 'all') {
    params.device_id = selectedDevice.value
  } else {
    params.device_id = 'all'
    params.imsi = t.imsi
  }
  const result = await smsStore.fetchThread(params)
  if (seq !== threadFetchSeq) return false
  if (result.ok) {
    threadMessages.value = (result.data || []) as SMSMessage[]
    threadHasMore.value = threadMessages.value.length === params.limit
  } else {
    messagesError.value = result.error
  }
  if (!silent) threadLoading.value = false
  return result.ok
}

async function ensureThreadSelection(options: { syncRoute?: boolean; silent?: boolean; scrollToBottom?: boolean } = {}) {
  const syncRoute = options.syncRoute === true
  const silent = options.silent === true
  const scrollToBottom = options.scrollToBottom === true

  const current = selectedThread.value
  if (current) {
    const ok = await fetchThreadLatest(silent)
    if (ok) {
      markThreadSeen(current)
      if (scrollToBottom) scrollThreadToBottom()
    }
    return
  }

  threadMessages.value = []
  threadHasMore.value = false
  if (selectedThreadKey.value) {
    selectedThreadKey.value = ''
    if (syncRoute) {
      void router.replace({ query: buildSmsQuery(selectedDevice.value) })
    }
  }
  if (isNarrowLayout.value || filteredThreads.value.length === 0) return
  await selectThread(filteredThreads.value[0].key, { syncRoute, silent, scrollToBottom })
}

async function selectThread(key: string, options: { syncRoute?: boolean; silent?: boolean; scrollToBottom?: boolean } = {}) {
  if (!key) return
  if (selectedThreadKey.value === key && threadMessages.value.length > 0) return

  const syncRoute = options.syncRoute !== false
  const silent = options.silent === true
  const scrollToBottom = options.scrollToBottom !== false

  selectedThreadKey.value = key
  if (syncRoute) {
    void router.replace({ query: buildSmsQuery(selectedDevice.value, key) })
  }

  const t = threads.value.find(x => x.key === key) || null
  if (!t) {
    threadMessages.value = []
    threadHasMore.value = false
    return
  }

  const ok = await fetchThreadLatest(silent)
  if (!ok) return
  markThreadSeen(t)
  if (scrollToBottom) scrollThreadToBottom()
}

async function handleSelectDevice(deviceId: string, options: { syncRoute?: boolean; silent?: boolean } = {}) {
  const nextDevice = String(deviceId || 'all').trim() || 'all'
  const syncRoute = options.syncRoute !== false
  const silent = options.silent === true

  selectedDevice.value = nextDevice
  clearSelectedThread(false)
  if (syncRoute) {
    void router.replace({ query: buildSmsQuery(nextDevice) })
  }

  const ok = await fetchMessages(silent)
  if (!ok || selectedDevice.value !== nextDevice) return
  await ensureThreadSelection({ syncRoute, silent, scrollToBottom: false })
}

function handleNarrowDeviceChange(value: unknown) {
  void handleSelectDevice(String(value || 'all'))
}

async function fetchMessagesAndThread(silent = false) {
  const ok = await fetchMessages(silent)
  if (!ok) return
  await ensureThreadSelection({ syncRoute: false, silent, scrollToBottom: !silent })
}

async function refreshAll() {
  await fetchDevices()
  await fetchMessagesAndThread()
}

async function pollRefresh() {
  if (loading.value || threadLoading.value || loadingHistoryMore.value) return
  try {
    await fetchMessagesAndThread(true)
  } catch {
    // Ignore polling failures; scheduler keeps retrying.
  }
}

usePollingScheduler(pollRefresh, 5000, {
  maxIntervalMs: 60000,
  backgroundIntervalMs: 15000
})

watch(
  () => isNarrowLayout.value,
  async (isNarrow) => {
    if (isNarrow) return
    if (selectedThreadKey.value) return
    if (filteredThreads.value.length === 0) return
    await selectThread(filteredThreads.value[0].key, { syncRoute: true, scrollToBottom: false })
  }
)

onMounted(async () => {
  syncSmsPageWidth()
  if (typeof ResizeObserver !== 'undefined') {
    smsPageResizeObserver = new ResizeObserver(() => {
      syncSmsPageWidth()
    })
    if (smsPageRef.value) {
      smsPageResizeObserver.observe(smsPageRef.value)
    }
  } else {
    window.addEventListener('resize', syncSmsPageWidth, { passive: true })
  }
  const initialDevice = selectedDevice.value
  const [, messagesOk] = await Promise.all([fetchDevices(), fetchMessages()])
  if (!messagesOk && selectedDevice.value !== initialDevice) {
    await fetchMessages()
  }
  await ensureThreadSelection({ syncRoute: false, silent: false, scrollToBottom: false })
})

onUnmounted(() => {
  clearLongPress()
  smsPageResizeObserver?.disconnect()
  smsPageResizeObserver = null
  window.removeEventListener('resize', syncSmsPageWidth)
})

function openSendModal() {
  sendForm.value.phone = ''
  sendForm.value.message = ''
  selectedSendDeviceId.value = selectedDevice.value !== 'all' ? selectedDevice.value : (devices.value[0]?.id || '')
  showSendModal.value = true
}

async function handleSendModal() {
  if (sending.value) return
  if (!selectedSendDeviceId.value || !sendForm.value.phone || !sendForm.value.message) {
    notify({ tone: 'warning', message: '请填写完整信息' })
    return
  }
  sending.value = true
  try {
    const result = await smsStore.send({
      device_id: selectedSendDeviceId.value,
      phone: sendForm.value.phone,
      message: sendForm.value.message
    })
    if (!result.ok) throw new Error(result.error.message || '发送失败')
    const parts = result.data.partsTotal
    notify({
      tone: 'success',
      message: `短信已发送${parts > 1 ? `（${parts}段）` : ''}`
    })
    showSendModal.value = false
    setTimeout(async () => {
      await fetchMessagesAndThread()
    }, 800)
  } catch (e: unknown) {
    const err = toAppError(e)
    notify({
      tone: 'error',
      message: '发送失败：' + (err.message || '未知错误')
    })
  } finally {
    sending.value = false
  }
}

async function sendToCurrentThread() {
  if (sending.value) return
  const t = selectedThread.value
  if (!t) return
  const text = String(composer.value || '').trim()
  if (!text) return
  const resolvedDeviceId = selectedDevice.value !== 'all' ? selectedDevice.value : (t.deviceId || devices.value[0]?.id || '')
  if (!resolvedDeviceId) {
    notify({ tone: 'warning', message: '暂无可用设备' })
    return
  }
  sending.value = true
  try {
    if (selectedDevice.value === 'all') {
      const result = await smsStore.send({ imsi: t.imsi, phone: t.peer, message: text })
      if (!result.ok) throw new Error(result.error.message || '发送失败')
    } else {
      const result = await smsStore.send({ device_id: resolvedDeviceId, phone: t.peer, message: text })
      if (!result.ok) throw new Error(result.error.message || '发送失败')
    }
    composer.value = ''
    scrollThreadToBottom()
    setTimeout(async () => {
      await fetchMessagesAndThread()
    }, 800)
  } catch (e: unknown) {
    const err = toAppError(e)
    notify({
      tone: 'error',
      message: '发送失败：' + (err.message || '未知错误')
    })
  } finally {
    sending.value = false
  }
}

async function confirmDeleteMessage(message: SMSMessage) {
  if (!message.id || deletingMessageId.value === message.id) return
  try {
    await requestConfirmation({
      message: '删除后无法恢复。仅删除短信中心历史记录。',
      title: '删除这条短信？',
      confirmLabel: '删除',
      cancelLabel: '取消',
      danger: true
    })
  } catch {
    return
  }

  deletingMessageId.value = message.id
  try {
    const result = await smsStore.deleteMessage(message.id)
    if (!result.ok) throw new Error(result.error.message || '删除失败')
    notify({ tone: 'success', message: '已删除短信' })
    await fetchMessagesAndThread()
    if (result.data.thread_empty) {
      clearSelectedThread(true)
    }
  } catch (e: unknown) {
    const err = toAppError(e)
    notify({
      tone: 'error',
      message: '删除失败：' + (err.message || '未知错误')
    })
  } finally {
    deletingMessageId.value = null
  }
}

async function confirmDeleteThread(thread: SmsThread) {
  if (deletingThreadKey.value === thread.key) return
  try {
    await requestConfirmation({
      message: `将删除与 ${thread.peer} 的全部短信历史，无法恢复。仅删除短信中心历史记录。`,
      title: '永久删除整个对话？',
      confirmLabel: '删除对话',
      cancelLabel: '取消',
      danger: true
    })
  } catch {
    return
  }

  deletingThreadKey.value = thread.key
  try {
    const payload = selectedDevice.value !== 'all'
      ? { device_id: selectedDevice.value, peer: thread.peer }
      : { device_id: 'all', imsi: thread.imsi, peer: thread.peer }
    const result = await smsStore.deleteThread(payload)
    if (!result.ok) throw new Error(result.error.message || '删除失败')
    notify({ tone: 'success', message: '已删除对话' })
    const deletingCurrent = selectedThreadKey.value === thread.key
    if (deletingCurrent) clearSelectedThread(true)
    await fetchMessages(false)
  } catch (e: unknown) {
    const err = toAppError(e)
    notify({
      tone: 'error',
      message: '删除失败：' + (err.message || '未知错误')
    })
  } finally {
    deletingThreadKey.value = null
  }
}


function onComposerKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.isComposing && !event.shiftKey && !event.ctrlKey && !event.altKey && !event.metaKey) {
    event.preventDefault()
    void sendToCurrentThread()
  }
}
</script>

<template>
  <div ref="smsPageRef" class="m3-page sms-page">
    <PageHeader title="短信中心" subtitle="按联系人聚合，查看与管理短信会话">
      <template #actions
        ><RefreshButton :loading="loading" @click="refreshAll" /><md-filled-button
          :disabled="sending"
          @click="openSendModal"
          ><SvgIcon slot="icon" :svg="sendIcon" />新建短信</md-filled-button
        ></template
      >
    </PageHeader>
    <ErrorState
      v-if="devicesError"
      title="设备列表加载失败"
      :message="devicesError.message"
      :status-code="devicesError.status"
      :request-method="devicesError.method"
      :request-url="devicesError.url"
      :last-success-at="devicesLastOkAt"
      retry-text="重试"
      @retry="fetchDevices"
    />
    <ErrorState
      v-if="messagesError"
      title="短信加载失败"
      :message="messagesError.message"
      :status-code="messagesError.status"
      :request-method="messagesError.method"
      :request-url="messagesError.url"
      :last-success-at="messagesLastOkAt"
      retry-text="重试"
      @retry="refreshAll"
    />
    <div class="sms-workspace" :class="{ narrow: isNarrowLayout }">
      <aside v-if="showDeviceSidebar" class="sms-devices">
        <h2 class="m3-label">设备</h2>
        <button
          v-for="device in deviceSidebarItems"
          :key="device.id"
          type="button"
          class="m3-option"
          :aria-pressed="selectedDevice === device.id"
          @click="handleSelectDevice(device.id)"
        >
          <md-ripple /><span>{{ device.label }}</span
          ><span
            v-if="device.id !== 'all'"
            class="sms-device-dot"
            :class="{ online: device.healthy }"
          />
        </button>
      </aside>
      <section v-if="showListPane" class="sms-list-pane" aria-label="短信会话列表">
        <div class="sms-search m3-form">
          <md-outlined-select
            v-if="isNarrowLayout"
            label="设备"
            :value="selectedDevice"
            @change="handleNarrowDeviceChange(fieldValue($event))"
            ><md-select-option
              v-for="device in deviceSidebarItems"
              :key="device.id"
              :value="device.id"
              :selected="selectedDevice === device.id"
              ><div slot="headline">{{ device.label }}</div></md-select-option
            ></md-outlined-select
          ><md-outlined-text-field
            label="搜索联系人或内容"
            name="sms-search"
            :value="searchQuery"
            @input="searchQuery = fieldValue($event)"
          />
        </div>
        <ListSkeleton v-if="loading && !threads.length" :rows="4" />
        <EmptyState
          v-else-if="!filteredThreads.length"
          :title="searchQuery ? '没有匹配的会话' : '暂无短信'"
          subtitle="收到短信后会显示在这里"
        />
        <RecycleScroller
          v-else
          :items="filteredThreads"
          :item-size="88"
          key-field="key"
          class="sms-thread-list"
        >
          <template #default="{ item: thread }"
            ><div
              class="sms-thread"
              :class="{ selected: selectedThreadKey === thread.key }"
              @pointerdown="openThreadActionSheet(thread, $event)"
              @pointermove="moveLongPress"
              @pointerup="clearLongPress"
              @pointercancel="clearLongPress"
            >
              <button
                type="button"
                class="sms-thread-main"
                :aria-pressed="selectedThreadKey === thread.key"
                @click="selectThread(thread.key)"
              >
                <md-ripple /><span class="sms-thread-heading"
                  ><strong>{{ thread.peer }}</strong
                  ><span v-if="isUnread(thread)" class="sms-unread" aria-label="未读" /><time>{{
                    formatClock(thread.lastTs)
                  }}</time></span
                ><span class="sms-preview">{{ thread.lastMessage }}</span
                ><span class="sms-local-phone">{{
                  thread.localPhone || thread.lastDeviceName || ''
                }}</span>
              </button>
              <md-icon-button
                class="danger"
                :aria-label="`删除与 ${thread.peer} 的对话`"
                :disabled="deletingThreadKey === thread.key"
                @click.stop="confirmDeleteThread(thread)"
                ><SvgIcon :svg="deleteIcon"
              /></md-icon-button></div
          ></template>
        </RecycleScroller>
      </section>
      <section v-if="showDetailPane" class="sms-detail" aria-label="会话详情">
        <header class="sms-detail-heading">
          <md-text-button v-if="isNarrowLayout && selectedThreadKey" @click="backToList"
            >返回</md-text-button
          >
          <div class="sms-peer">
            <h2>{{ selectedThread?.peer || '会话详情' }}</h2>
            <p v-if="selectedThread?.localPhone" class="m3-muted">
              本机：{{ selectedThread.localPhone }}
            </p>
          </div>
          <md-text-button v-if="selectedThread" @click="scrollThreadToBottom">最新</md-text-button>
        </header>
        <EmptyState
          v-if="!selectedThread"
          title="请选择一个会话"
          subtitle="从联系人列表进入短信明细"
        />
        <div
          v-else
          ref="detailScrollbar"
          class="sms-detail-scroll"
          :aria-busy="threadLoading"
          @scroll="onDetailScroll"
        >
          <div class="sms-message-groups">
            <div v-if="canLoadMoreHistory" class="sms-history-action">
              <md-text-button :disabled="loadingHistoryMore" @click="loadMoreHistory">{{
                loadingHistoryMore ? '加载中…' : '加载更多历史'
              }}</md-text-button>
            </div>
            <section v-for="group in selectedThreadGroups" :key="group.date" class="sms-date-group">
              <h3 class="sms-date">{{ group.date }}</h3>
              <article
                v-for="message in group.items"
                :key="message.id"
                class="sms-message"
                :class="{ outgoing: message.type === 2 }"
                @pointerdown="openMessageActionSheet(message, $event)"
                @pointermove="moveLongPress"
                @pointerup="clearLongPress"
                @pointercancel="clearLongPress"
              >
                <div class="sms-message-meta">
                  <span v-if="message.type === 1">{{ message.sender }}</span
                  ><span v-if="message.device_name" class="m3-badge">{{ message.device_name }}</span
                  ><time :datetime="message.timestamp">{{
                    new Date(message.timestamp).toLocaleString()
                  }}</time
                  ><span v-if="message.type === 2 && message.status === 2" class="sms-sent"
                    >发送成功</span
                  ><span v-else-if="message.type === 2 && message.status === 3" class="m3-error"
                    >发送失败</span
                  ><md-icon-button
                    class="danger"
                    :aria-label="`删除短信 ${message.id}`"
                    :disabled="deletingMessageId === message.id"
                    @click="confirmDeleteMessage(message)"
                    ><SvgIcon :svg="deleteIcon"
                  /></md-icon-button>
                </div>
                <div class="sms-bubble">{{ message.content }}</div>
              </article>
            </section>
          </div>
        </div>
        <form
          v-if="selectedThread"
          class="sms-composer m3-form"
          @submit.prevent="sendToCurrentThread"
        >
          <p class="m3-label">
            {{ composerEstimate.encoding }} · 预计 {{ composerEstimate.parts }} 段 ·
            {{ composerLen }} 字
          </p>
          <div class="sms-compose-row">
            <md-outlined-text-field
              ref="composerInput"
              name="sms-reply"
              type="textarea"
              rows="2"
              label="回复短信"
              supporting-text="Enter 发送，Shift + Enter 换行"
              :value="composer"
              :disabled="sending"
              @input="composer = fieldValue($event)"
              @keydown="onComposerKeydown"
            /><md-filled-button type="submit" :disabled="sending || !composer.trim()">{{
              sending ? '发送中…' : '发送'
            }}</md-filled-button>
          </div>
        </form>
      </section>
    </div>
    <md-dialog class="m3-dialog sms-action-sheet" :open="showActionSheet" @closed="closeActionSheet"
      ><div slot="headline">
        {{ actionSheetTarget?.type === 'thread' ? '会话操作' : '短信操作' }}
      </div>
      <div slot="content">
        <md-text-button class="danger" @click="onActionSheetDelete">{{
          actionSheetTarget?.type === 'thread' ? '删除对话' : '删除短信'
        }}</md-text-button>
      </div>
      <div slot="actions">
        <md-text-button @click="closeActionSheet">取消</md-text-button>
      </div></md-dialog
    >
    <md-dialog
      class="m3-dialog"
      :open="showSendModal"
      @cancel="guardDialogCancel($event, sending)"
      @closed="showSendModal = false"
    >
      <div slot="headline">发送短信</div>
      <form id="sms-send-form" slot="content" class="m3-form" @submit.prevent="handleSendModal">
        <md-outlined-select
          label="发送设备"
          name="sms-device"
          required
          :value="selectedSendDeviceId"
          :disabled="sending"
          @change="selectedSendDeviceId = fieldValue($event)"
          ><md-select-option
            v-for="device in sendDeviceOptions"
            :key="device.value"
            :value="device.value"
            :selected="selectedSendDeviceId === device.value"
            ><div slot="headline">{{ device.label }}</div></md-select-option
          ></md-outlined-select
        >
        <md-outlined-text-field
          label="目标号码"
          name="sms-phone"
          type="tel"
          required
          :value="sendForm.phone"
          :disabled="sending"
          @input="sendForm.phone = fieldValue($event)"
        />
        <md-outlined-text-field
          label="短信内容"
          name="sms-body"
          type="textarea"
          rows="5"
          required
          :value="sendForm.message"
          :disabled="sending"
          @input="sendForm.message = fieldValue($event)"
        />
        <p class="m3-muted">
          {{ sendEstimate.encoding }} · 预计 {{ sendEstimate.parts }} 段 ·
          {{ Array.from(sendForm.message).length }} 字
        </p>
      </form>
      <div slot="actions">
        <md-text-button :disabled="sending" @click="showSendModal = false">取消</md-text-button
        ><md-filled-button type="submit" form="sms-send-form" :disabled="sending">{{
          sending ? '发送中…' : '发送'
        }}</md-filled-button>
      </div>
    </md-dialog>
  </div>
</template>

<style scoped>
.sms-page {
  height: calc(100dvh - 112px);
  min-height: 480px;
  gap: 16px;
}
.sms-workspace {
  flex: 1;
  min-height: 320px;
  min-width: 0;
  display: grid;
  grid-template-columns: 180px 300px minmax(0, 1fr);
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 16px;
  overflow: hidden;
  background: var(--md-sys-color-surface-container-low);
}
.sms-workspace.narrow {
  grid-template-columns: minmax(0, 1fr);
}
.sms-devices {
  padding: 16px 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border-right: 1px solid var(--md-sys-color-outline-variant);
  overflow: auto;
}
.sms-devices h2 {
  padding: 0 8px 8px;
}
.sms-devices .m3-option {
  border: 0;
  min-height: 48px;
}
.sms-device-dot,
.sms-unread {
  display: inline-block;
  flex-shrink: 0;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--md-sys-color-outline);
}
.sms-device-dot.online {
  background: var(--vh-success);
}
.sms-unread {
  background: var(--md-sys-color-primary);
}
.sms-list-pane,
.sms-detail {
  display: flex;
  flex-direction: column;
  min-height: 0;
  min-width: 0;
}
.sms-list-pane {
  border-right: 1px solid var(--md-sys-color-outline-variant);
}
.narrow .sms-list-pane {
  border-right: 0;
}
.sms-search {
  padding: 16px;
  gap: 12px;
}
.sms-thread-list {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.sms-thread {
  display: flex;
  align-items: center;
  height: 88px;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
}
.sms-thread.selected {
  background: var(--md-sys-color-secondary-container);
}
.sms-thread-main {
  position: relative;
  flex: 1;
  min-width: 0;
  height: 100%;
  padding: 10px 12px;
  overflow: hidden;
  text-align: left;
}
.sms-thread-heading {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 16px;
  line-height: 24px;
}
.sms-thread-heading strong {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}
.sms-thread-heading time {
  margin-left: auto;
  flex-shrink: 0;
  font-size: 11px;
  color: var(--md-sys-color-on-surface-variant);
}
.sms-preview,
.sms-local-phone {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--md-sys-color-on-surface-variant);
}
.sms-preview {
  font-size: 13px;
  line-height: 20px;
}
.sms-local-phone {
  font-size: 11px;
  line-height: 16px;
}
.sms-detail-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 16px;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
}
.sms-peer {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.sms-peer h2 {
  font-size: 18px;
  font-weight: 500;
}
.sms-detail-scroll {
  flex: 1;
  min-height: 0;
  overflow: auto;
}
.sms-message-groups {
  padding: 20px;
}
.sms-history-action {
  text-align: center;
}
.sms-date-group {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-bottom: 20px;
}
.sms-date {
  align-self: center;
  padding: 4px 12px;
  border-radius: 8px;
  background: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  font-weight: 500;
}
.sms-message {
  max-width: 90%;
  align-self: flex-start;
}
.sms-message.outgoing {
  align-self: flex-end;
}
.sms-message-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  font-size: 11px;
  color: var(--md-sys-color-on-surface-variant);
}
.sms-message-meta md-icon-button {
  --md-icon-button-state-layer-height: 32px;
  --md-icon-button-state-layer-width: 32px;
}
.sms-bubble {
  padding: 12px 16px;
  border-radius: 16px;
  background: var(--md-sys-color-surface-container-high);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 14px;
  line-height: 24px;
}
.outgoing .sms-bubble {
  background: var(--md-sys-color-primary-container);
  color: var(--md-sys-color-on-primary-container);
}
.sms-sent {
  color: var(--vh-success);
}
.sms-composer {
  padding: 16px;
  gap: 8px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.sms-compose-row {
  display: flex;
  align-items: flex-end;
  gap: 8px;
}
.sms-compose-row md-outlined-text-field {
  flex: 1;
}
.sms-composer md-filled-button {
  flex-shrink: 0;
}
.sms-action-sheet {
  margin: auto auto 0;
  width: min(560px, 100vw);
  max-width: 100vw;
  --md-dialog-container-shape-end-start: 0px;
  --md-dialog-container-shape-end-end: 0px;
}
@media (max-width: 599px) {
  .sms-message-groups {
    padding: 12px;
  }
  .sms-message {
    max-width: 100%;
  }
  .sms-compose-row {
    flex-wrap: wrap;
  }
  .sms-compose-row md-outlined-text-field {
    flex-basis: 100%;
  }
}
</style>
