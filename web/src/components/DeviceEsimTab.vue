<script setup lang="ts">
import { notify, requestConfirmation } from '../stores/feedback'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { EsimChipInfo, EsimEUICCProfiles, EsimNotificationItem, EsimSpaceDelta } from '../types/api'
import { devicesService } from '../services/devices'
import { errorMessage } from '../services/http'
import { api } from '../stores/auth'
import { useSensitiveVisibility } from '../composables/useSensitiveVisibility'
import EsimCardPolicyInline from './EsimCardPolicyInline.vue'
import RefreshButton from './RefreshButton.vue'
import LoadingScreen from './LoadingScreen.vue'
import EmptyState from './EmptyState.vue'
import { fieldValue } from '../utils/material'
import { applyOptimisticActiveState } from './deviceEsimOptimistic'
import { pickNextDownloadAid } from './deviceEsimOverviewRefresh'
import { describeDeleteResultNotice, describeDownloadTerminalNotice, describeSpaceDelta } from './deviceEsimOperationNotice'
import { formatEsimNotificationEvent, reconcileEsimNotificationDialogState } from './deviceEsimNotifications'

const props = defineProps<{
  deviceId: string
  deviceImei?: string
  isActive?: boolean
  deviceOnline?: boolean
  esimSwitching?: boolean
}>()

// 数据状态
const loading = ref(false)
const profilesRefreshing = ref(false)
const chipInfo = ref<EsimChipInfo | null>(null)
const profiles = ref<EsimEUICCProfiles[]>([])

// 操作状态
const switching = ref<string | null>(null)
const switchSubmitted = ref(false)
let switchRequestId = 0
const switchBusy = computed(() => switching.value !== null || switchSubmitted.value || props.esimSwitching === true)
watch(() => props.esimSwitching, (busy) => {
  if (!busy) switchSubmitted.value = false
})
const deleting = ref<string | null>(null)
const renaming = ref<string | null>(null)
const showSensitive = useSensitiveVisibility()
const renameValue = ref('')
// 行内卡策略展开态（手风琴，一次只展开一行，按 iccid 记）
const expandedPolicyIccid = ref<string | null>(null)
function togglePolicyPanel(iccid: string) {
  expandedPolicyIccid.value = expandedPolicyIccid.value === iccid ? null : iccid
}
const notifications = ref<EsimNotificationItem[]>([])
const notificationsLoading = ref(false)
const notificationsDialogOpen = ref(false)
const retryingNotificationSequence = ref<number | null>(null)

// 下载表单
const downloadForm = ref({
  smdp: '',
  matchingId: '',
  confirmationCode: '',
  aidHex: '',
  imei: ''
})
const downloading = ref(false)
const downloadProgress = ref(0)
const downloadMsg = ref('')
const downloadError = ref('')
const downloadSessionId = ref(0)
const recentSpaceDelta = ref<{ aidHex: string; message: string } | null>(null)
let recentSpaceDeltaTimer: number | null = null
let lastDeviceImeiDefault = ''

function defaultDeviceImei() {
  return (props.deviceImei || '').trim()
}

function applyDeviceImeiDefault(force = false) {
  const next = defaultDeviceImei()
  if (force || !downloadForm.value.imei || downloadForm.value.imei === lastDeviceImeiDefault) {
    downloadForm.value.imei = next
  }
  lastDeviceImeiDefault = next
}

// 智能解析完整的 LPA 激活码或移除 URL 前缀
watch(() => downloadForm.value.smdp, (newVal) => {
  if (!newVal) return

  if (newVal.startsWith('LPA:')) {
    const parts = newVal.split('$')
    if (parts.length >= 3) {
      downloadForm.value.smdp = parts[1] // SM-DP+
      downloadForm.value.matchingId = parts[2] // Matching ID
      notify({ tone: 'success', message: '已自动解析完整的 LPA 激活码' })
    }
  } else if (newVal.startsWith('http://') || newVal.startsWith('https://')) {
    downloadForm.value.smdp = newVal.replace(/^https?:\/\//i, '')
  }
})

let fetchAbortController: AbortController | null = null
let fetchOverviewRequestId = 0

function normalizeAidHex(aidHex: string | undefined | null): string {
  return (aidHex || '').trim().toUpperCase()
}

function clearRecentSpaceDelta() {
  if (recentSpaceDeltaTimer !== null) {
    window.clearTimeout(recentSpaceDeltaTimer)
    recentSpaceDeltaTimer = null
  }
  recentSpaceDelta.value = null
}

function showRecentSpaceDelta(aidHex: string, spaceDelta?: EsimSpaceDelta) {
  const normalizedAidHex = normalizeAidHex(aidHex)
  const message = describeSpaceDelta(spaceDelta)
  if (!normalizedAidHex || !message) return
  clearRecentSpaceDelta()
  recentSpaceDelta.value = { aidHex: normalizedAidHex, message }
  recentSpaceDeltaTimer = window.setTimeout(() => {
    recentSpaceDelta.value = null
    recentSpaceDeltaTimer = null
  }, 75000)
}

async function fetchNotifications() {
  notificationsLoading.value = true
  const result = await devicesService.getEsimNotifications(props.deviceId)
  try {
    if (!result.ok) throw result.error
    notifications.value = result.data
  } catch (e: unknown) {
    notify({ tone: 'error', message: errorMessage(e, '获取当前通知列表失败') })
  } finally {
    notificationsLoading.value = false
  }
}

async function openNotificationsDialog() {
  notificationsDialogOpen.value = true
  await fetchNotifications()
}

async function retryNotification(item: EsimNotificationItem) {
  if (!item.can_retry || retryingNotificationSequence.value !== null) return
  retryingNotificationSequence.value = item.sequence_number
  const result = await devicesService.retryEsimNotification(props.deviceId, item.sequence_number, item.aid_hex)
  try {
    if (!result.ok) throw result.error
    retryingNotificationSequence.value = null
    notify({ tone: 'success', message: result.data.message })
    const refreshed = await devicesService.getEsimNotifications(props.deviceId)
    if (!refreshed.ok) {
      notify({
        tone: 'warning',
        message: refreshed.error.message || '通知已发送，但刷新通知列表失败'
      })
      return
    }
    const nextState = reconcileEsimNotificationDialogState({
      isOpen: notificationsDialogOpen.value,
      items: notifications.value,
      refreshedItems: refreshed.data,
      retriedSequenceNumber: item.sequence_number
    })
    notificationsDialogOpen.value = nextState.isOpen
    notifications.value = nextState.items
    retryingNotificationSequence.value = nextState.retryingSequenceNumber
  } catch (e: unknown) {
    const nextState = reconcileEsimNotificationDialogState({
      isOpen: notificationsDialogOpen.value,
      items: notifications.value,
      refreshedItems: notifications.value,
      retriedSequenceNumber: null
    })
    notificationsDialogOpen.value = nextState.isOpen
    notifications.value = nextState.items
    retryingNotificationSequence.value = nextState.retryingSequenceNumber
    notify({ tone: 'error', message: errorMessage(e, '通知重试发送失败') })
  }
}

// 获取 eSIM 总览数据
async function fetchOverview(refresh = false) {
  fetchOverviewRequestId += 1
  const requestId = fetchOverviewRequestId

  if (fetchAbortController) {
    fetchAbortController.abort()
  }
  const controller = new AbortController()
  fetchAbortController = controller

  if (refresh) {
    profilesRefreshing.value = true
  } else {
    loading.value = true
  }

  const currentAidHex = downloadForm.value.aidHex
  const result = await devicesService.getEsimOverview(props.deviceId, {
    refresh,
    signal: controller.signal
  })
  let shouldResetLoading = true
  try {
    if (requestId !== fetchOverviewRequestId) {
      shouldResetLoading = false
      return
    }
    if (!result.ok) throw result.error
    chipInfo.value = result.data.chipInfo
    profiles.value = result.data.profiles
    downloadForm.value.aidHex = pickNextDownloadAid(chipInfo.value, currentAidHex)
  } catch (e: unknown) {
    if (result.ok === false && result.error.code === 'ERR_CANCELED') {
      return
    }
    notify({ tone: 'error', message: errorMessage(e, '获取 eSIM 信息失败') })
  } finally {
    if (shouldResetLoading) {
      if (refresh) {
        profilesRefreshing.value = false
      } else {
        loading.value = false
      }
    }
  }
}

async function fetchProfiles(refresh = false) {
  profilesRefreshing.value = true
  const result = await devicesService.getEsimProfiles(props.deviceId, { refresh })
  try {
    if (!result.ok) throw result.error
    profiles.value = result.data
  } catch (e: unknown) {
    notify({
      tone: 'error',
      message: errorMessage(e, '获取 eSIM Profiles 失败')
    })
  } finally {
    profilesRefreshing.value = false
  }
}

function applyOptimisticActive(targetICCID: string, aidHex: string) {
  profiles.value = applyOptimisticActiveState(profiles.value, targetICCID, aidHex)
}

// 切换 profile（启用/禁用）
async function switchProfile(iccid: string, currentState: number, aidHex: string) {
  if (switchBusy.value) return
  const deviceId = props.deviceId
  const requestId = ++switchRequestId
  const isCurrent = () => props.deviceId === deviceId && requestId === switchRequestId
  const action = currentState === 1 ? '禁用' : '启用'
  const confirmed = await requestConfirmation({
    message: `确定要${action}此 Profile (${iccid}) 吗？切换后设备会短暂断网。`,
    title: `${action} Profile`,
    confirmLabel: action,
    cancelLabel: '取消',
    danger: true
  }).then(() => true).catch(() => false)
  if (!confirmed || switchBusy.value || !isCurrent()) return

  switching.value = iccid
  try {
    const result = await devicesService.switchEsimProfile(deviceId, {
      iccid,
      aid_hex: aidHex
    })
    if (!isCurrent()) return
    if (!result.ok) throw new Error(result.error.message || `${action}失败`)
    switchSubmitted.value = true
    notify({ tone: 'success', message: '切卡指令已提交，正在恢复设备服务' })
    applyOptimisticActive(iccid, aidHex)
    // Covers completion before the SSE busy transition reaches this component.
    const overview = await devicesService.getOverviewLite(deviceId)
    if (isCurrent() && (!overview.ok || !overview.data?.esim_switching)) {
      switchSubmitted.value = false
    }
  } catch (e: unknown) {
    if (isCurrent()) notify({ tone: 'error', message: errorMessage(e, `${action}失败`) })
  } finally {
    if (isCurrent()) switching.value = null
  }
}

// 开始编辑名称
function startRename(iccid: string, currentName: string) {
  renaming.value = iccid
  renameValue.value = currentName
}

// 保存名称
async function saveRename(iccid: string, aidHex: string) {
  const name = renameValue.value.trim()
  if (!name) {
    notify({ tone: 'warning', message: '名称不能为空' })
    return
  }
  try {
    const result = await devicesService.renameEsimProfile(props.deviceId, iccid, { name, aid_hex: aidHex })
    if (!result.ok) throw new Error(result.error.message || '修改名称失败')
    notify({ tone: 'success', message: '名称修改成功' })
    renaming.value = null
    await fetchProfiles(true)
  } catch (e: unknown) {
    notify({ tone: 'error', message: errorMessage(e, '修改名称失败') })
  }
}

// 取消编辑
function cancelRename() {
  renaming.value = null
  renameValue.value = ''
}

// 删除 profile（需要输入 ICCID 后 4 位确认）
async function deleteProfile(iccid: string, name: string, aidHex: string) {
  const last4 = iccid.slice(-4)
  const { value: input } = await requestConfirmation({
    message: `此操作不可逆！请输入 ICCID 后 4 位「${last4}」以确认删除 Profile「${name}」`,
    title: '⚠️ 删除 Profile',
    confirmLabel: '确认删除',
    cancelLabel: '取消',
    danger: true,
    input: {
      pattern: new RegExp(`^${last4}$`),
      placeholder: `输入 ${last4}`,
      errorMessage: `请输入 ${last4} 以确认`
    }
  }).then(value => ({ value })).catch(() => ({ value: '' }))
  if (input !== last4) return

  deleting.value = iccid
  try {
    const result = await devicesService.deleteEsimProfile(props.deviceId, iccid, aidHex)
    if (!result.ok) throw new Error(result.error.message || '删除失败')
    showRecentSpaceDelta(aidHex, result.data.space_delta)
    const notice = describeDeleteResultNotice(result.data)
    if (notice.tone === 'warning') {
      notify({ tone: 'warning', message: notice.message })
    } else {
      notify({ tone: 'success', message: notice.message })
    }
    await fetchOverview(true)
  } catch (e: unknown) {
    notify({ tone: 'error', message: errorMessage(e, '删除失败') })
  } finally {
    deleting.value = null
  }
}

// 下载新 profile（SSE 流式进度）
async function downloadProfile() {
  if (downloading.value) return
  const { smdp, matchingId, confirmationCode, aidHex, imei } = downloadForm.value
  const targetAidHex = aidHex || pickNextDownloadAid(chipInfo.value, '')
  if (!smdp) {
    notify({ tone: 'warning', message: '请输入 SM-DP+ 地址' })
    return
  }

  downloadSessionId.value++
  downloading.value = true
  downloadProgress.value = 0
  downloadMsg.value = '正在连接...'
  downloadError.value = ''

  const params = new URLSearchParams({ smdp })
  if (matchingId) params.set('matching_id', matchingId)
  if (confirmationCode) params.set('confirmation_code', confirmationCode)
  if (targetAidHex) params.set('aid_hex', targetAidHex)
  if (imei.trim()) params.set('imei', imei.trim())

  const base = api.defaults.baseURL || ''
  const url = `${base}/devices/${props.deviceId}/esim/actions/download?${params}`
  const token = localStorage.getItem('token') || ''
  const controller = new AbortController()

  try {
    const res = await fetch(url, {
      method: 'GET',
      headers: { Authorization: `Bearer ${token}`, Accept: 'text/event-stream' },
      signal: controller.signal
    })
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || `HTTP ${res.status}`)
    }
    if (!res.body) throw new Error('No stream body')

    const reader = res.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''

    outer: while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      while (true) {
        const nl = buffer.indexOf('\n')
        if (nl < 0) break
        let line = buffer.slice(0, nl)
        buffer = buffer.slice(nl + 1)
        if (line.endsWith('\r')) line = line.slice(0, -1)
        if (!line.startsWith('data:')) continue

        const payload = line.slice('data:'.length).trim()
        try {
          const evt = JSON.parse(payload) as { step: string; msg: string; pct: number; code?: string; warning?: string; space_delta?: EsimSpaceDelta }
          if (evt.step === 'error') {
            downloadError.value = evt.code === 'euicc_insufficient_memory'
              ? 'eUICC 安装 profile 时空间不足，请删除未使用的 profile 后重试。'
              : evt.msg
            break outer
          }
          downloadProgress.value = evt.pct
          downloadMsg.value = evt.msg
          if (evt.step === 'done') {
            showRecentSpaceDelta(targetAidHex, evt.space_delta)
            const notice = describeDownloadTerminalNotice(evt)
            if (notice.tone === 'warning') {
              notify({ tone: 'warning', message: notice.message })
            } else {
              notify({ tone: 'success', message: notice.message })
            }
            downloadForm.value = { smdp: '', matchingId: '', confirmationCode: '', aidHex: targetAidHex, imei }
            await fetchOverview(true)
            break outer
          }
        } catch { /* 非 JSON 行，忽略 */ }
      }
    }
  } catch (e: unknown) {
    if (!downloadError.value) {
      downloadError.value = errorMessage(e, '下载失败')
    }
  } finally {
    downloading.value = false
  }
}

// 切换设备或改换 tab 时重新获取数据
watch(
  [() => props.deviceId, () => props.isActive],
  ([newId, newActive]) => {
    if (fetchAbortController) {
      fetchAbortController.abort()
    }
    expandedPolicyIccid.value = null
    if (!newId || !newActive) return

    clearRecentSpaceDelta()
    chipInfo.value = null
    profiles.value = []
    downloadForm.value.aidHex = ''
    applyDeviceImeiDefault(true)
    fetchOverview()
  },
  { immediate: true }
)

watch(() => props.deviceId, () => {
  switchRequestId++
  switching.value = null
  switchSubmitted.value = false
})

watch(() => props.deviceImei, () => {
  applyDeviceImeiDefault(false)
})

onBeforeUnmount(() => {
  switchRequestId++
  clearRecentSpaceDelta()
  if (fetchAbortController) {
    fetchAbortController.abort()
  }
})
</script>

<template>
  <section class="m3-stack esim-page" aria-label="eSIM 管理">
    <LoadingScreen v-if="loading" title="读取 eUICC…" subtitle="正在获取芯片和 Profile 信息" />
    <template v-else>
      <div class="m3-row">
        <div>
          <h2 class="m3-title">{{ chipInfo?.sku_name || 'eSIM 管理' }}</h2>
          <p v-if="chipInfo" class="m3-muted">
            {{ chipInfo.firmware ? `固件 ${chipInfo.firmware}` : '' }}
            <span :class="{ 'blur-sm select-none': !showSensitive }">{{
              chipInfo.serial_number ? `· SN ${chipInfo.serial_number}` : ''
            }}</span>
          </p>
        </div>
        <div class="m3-actions">
          <RefreshButton
            :loading="profilesRefreshing"
            @click="fetchOverview(true)"
          /><md-outlined-button :disabled="notificationsLoading" @click="openNotificationsDialog"
            >当前通知</md-outlined-button
          ><md-text-button @click="showSensitive = !showSensitive">{{
            showSensitive ? '隐藏敏感信息' : '显示敏感信息'
          }}</md-text-button>
        </div>
      </div>
      <p v-if="switchBusy" class="m3-notice" role="status">切卡处理中，请等待设备服务恢复。</p>
      <section
        v-for="(group, index) in profiles"
        :key="group.aid_hex || group.eid || index"
        class="m3-inset m3-stack"
      >
        <div>
          <h3>eUICC #{{ index + 1 }}</h3>
          <p class="esim-eid m3-muted" :class="{ 'blur-sm select-none': !showSensitive }">
            {{ group.eid }}
          </p>
        </div>
        <template v-for="eid in chipInfo?.eids || []" :key="eid.eid"
          ><div v-if="eid.eid === group.eid" class="m3-stack">
            <p class="m3-muted">
              可用空间 {{ eid.free_nvram
              }}<span
                v-if="
                  recentSpaceDelta && normalizeAidHex(group.aid_hex) === recentSpaceDelta.aidHex
                "
              >
                · {{ recentSpaceDelta.message }}</span
              >
            </p>
            <details
              v-if="
                eid.manufacturer ||
                eid.certificates?.length ||
                eid.default_smdp_address ||
                eid.root_ds_address ||
                eid.sas_accreditation_number ||
                eid.info_source
              "
              class="m3-muted"
            >
              <summary>芯片与证书信息</summary>
              <p v-if="eid.manufacturer">生产商：{{ eid.manufacturer }}</p>
              <p v-if="eid.certificates?.length">证书：{{ eid.certificates.join(' · ') }}</p>
              <p v-if="eid.default_smdp_address">Default SM-DP+：{{ eid.default_smdp_address }}</p>
              <p v-if="eid.root_ds_address">Root SM-DS：{{ eid.root_ds_address }}</p>
              <p v-if="eid.sas_accreditation_number">SAS：{{ eid.sas_accreditation_number }}</p>
              <p v-if="eid.info_source">来源：{{ eid.info_source }}</p>
            </details>
          </div></template
        >
        <p v-if="!group.profiles?.length" class="m3-muted">暂无 Profile</p>
        <article
          v-for="profile in group.profiles"
          :key="profile.iccid"
          class="esim-profile m3-stack"
        >
          <div v-if="renaming !== profile.iccid" class="m3-stack">
            <div class="m3-row">
              <h4>{{ profile.name || profile.iccid }}</h4>
              <span class="m3-badge">{{
                profile.state_text || (profile.state === 1 ? '已启用' : '未启用')
              }}</span>
            </div>
            <p class="m3-muted">
              {{ profile.service_provider_name }}
              <span :class="{ 'blur-sm select-none': !showSensitive }">{{ profile.iccid }}</span>
            </p>
            <div class="m3-actions">
              <md-outlined-button
                v-if="profile.state !== 1"
                :disabled="switchBusy || deleting !== null || downloading"
                @click="switchProfile(profile.iccid, profile.state, group.aid_hex)"
                >{{ switching === profile.iccid ? '切换中…' : '切换' }}</md-outlined-button
              ><md-text-button
                :disabled="switchBusy || deleting !== null"
                @click="startRename(profile.iccid, profile.name)"
                >改名</md-text-button
              ><md-text-button
                :aria-expanded="expandedPolicyIccid === profile.iccid"
                @click="togglePolicyPanel(profile.iccid)"
                >卡策略</md-text-button
              ><md-text-button
                class="danger"
                :disabled="switchBusy || deleting !== null || downloading"
                @click="deleteProfile(profile.iccid, profile.name, group.aid_hex)"
                >{{ deleting === profile.iccid ? '删除中…' : '删除' }}</md-text-button
              >
            </div>
          </div>
          <form v-else class="m3-form" @submit.prevent="saveRename(profile.iccid, group.aid_hex)">
            <md-outlined-text-field
              label="Profile 名称"
              name="esim-profile-name"
              autofocus
              :value="renameValue"
              @input="renameValue = fieldValue($event)"
              @keydown.escape.stop="cancelRename"
            />
            <div class="m3-actions">
              <md-text-button type="button" @click="cancelRename">取消</md-text-button
              ><md-filled-button type="submit">保存</md-filled-button>
            </div>
          </form>
          <EsimCardPolicyInline
            v-if="expandedPolicyIccid === profile.iccid"
            :device-id="deviceId"
            :iccid="profile.iccid"
            :is-active-card="profile.state === 1"
            :device-online="deviceOnline === true"
            @policy-changed="fetchOverview(true)"
          />
        </article>
      </section>
      <md-dialog
        class="m3-dialog"
        :open="notificationsDialogOpen"
        @closed="notificationsDialogOpen = false"
        ><div slot="headline">当前通知列表</div>
        <div slot="content" class="m3-stack">
          <p v-if="notificationsLoading" role="status">正在加载通知…</p>
          <p v-else-if="!notifications.length" class="m3-muted">当前没有可展示的通知</p>
          <article
            v-for="item in notifications"
            v-else
            :key="item.sequence_number"
            class="m3-inset m3-stack"
          >
            <div class="m3-row">
              <h3>#{{ item.sequence_number }} · {{ formatEsimNotificationEvent(item.event) }}</h3>
              <md-outlined-button
                :disabled="!item.can_retry || retryingNotificationSequence !== null"
                @click="retryNotification(item)"
                >{{
                  retryingNotificationSequence === item.sequence_number ? '重发中…' : '重发'
                }}</md-outlined-button
              >
            </div>
            <p v-if="item.iccid" class="m3-muted">ICCID {{ item.iccid }}</p>
            <p v-if="item.address" class="m3-muted">地址 {{ item.address }}</p>
            <p v-if="item.aid_hex" class="m3-muted">AID {{ item.aid_hex }}</p>
          </article>
        </div>
        <div slot="actions">
          <md-text-button @click="notificationsDialogOpen = false">关闭</md-text-button>
        </div></md-dialog
      >
      <section v-if="chipInfo" class="m3-inset m3-stack">
        <h3>下载新 Profile</h3>
        <form class="m3-form" @submit.prevent="downloadProfile">
          <div class="m3-form-grid">
            <md-outlined-text-field
              label="SM-DP+ 地址或 LPA 激活码"
              name="esim-smdp"
              required
              :value="downloadForm.smdp"
              :disabled="downloading"
              @input="downloadForm.smdp = fieldValue($event)"
            /><md-outlined-text-field
              label="Matching ID（可选）"
              :value="downloadForm.matchingId"
              :disabled="downloading"
              @input="downloadForm.matchingId = fieldValue($event)"
            /><md-outlined-text-field
              label="确认码（可选）"
              :value="downloadForm.confirmationCode"
              :disabled="downloading"
              @input="downloadForm.confirmationCode = fieldValue($event)"
            /><md-outlined-text-field
              label="IMEI"
              maxlength="15"
              :value="downloadForm.imei"
              :disabled="downloading"
              @input="downloadForm.imei = fieldValue($event)"
            />
            <md-outlined-select
              label="目标 eUICC"
              :value="downloadForm.aidHex"
              :disabled="downloading"
              @change="downloadForm.aidHex = fieldValue($event)"
              ><md-select-option
                v-for="(eid, index) in chipInfo.eids || []"
                :key="eid.aid"
                :value="eid.aid"
                :selected="downloadForm.aidHex === eid.aid"
                ><div slot="headline">
                  eUICC #{{ index + 1 }} (…{{ eid.eid.slice(-4) }}) · {{ eid.free_nvram }} 可用
                </div></md-select-option
              ></md-outlined-select
            >
          </div>
          <div v-if="downloading || downloadError" class="m3-stack">
            <md-linear-progress
              :key="downloadSessionId"
              :value="downloadProgress / 100"
              :indeterminate="downloading && downloadProgress === 0"
              aria-label="Profile 下载进度"
            />
            <p
              :class="downloadError ? 'm3-error' : 'm3-muted'"
              :role="downloadError ? 'alert' : 'status'"
            >
              {{ downloadError || downloadMsg }} · {{ Math.round(downloadProgress) }}%
            </p>
          </div>
          <div class="m3-actions">
            <md-filled-button
              type="submit"
              :disabled="downloading || switchBusy || deleting !== null"
              >{{ downloading ? '下载中…' : '开始下载' }}</md-filled-button
            >
          </div>
        </form>
      </section>
      <EmptyState
        v-if="!profiles.length && !chipInfo"
        title="未检测到 eUICC"
        subtitle="此 SIM 卡可能不支持 eUICC 功能"
      />
    </template>
  </section>
</template>
<style scoped>
h3,
h4 {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  overflow-wrap: anywhere;
}
.esim-eid {
  font-family: var(--vh-typeface-mono);
}
.esim-page p {
  overflow-wrap: anywhere;
}
.esim-profile {
  padding-top: 16px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.esim-profile .m3-badge {
  flex-shrink: 0;
}
details {
  overflow-wrap: anywhere;
}
summary {
  cursor: pointer;
  padding-block: 8px;
}
</style>
