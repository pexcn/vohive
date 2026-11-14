<script setup lang="ts">
import { notify, requestConfirmation } from '../stores/feedback'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { EsimChipInfo, EsimEUICCInfo, EsimEUICCProfiles, EsimNotificationItem, EsimProfileItem, EsimSpaceDelta } from '../types/api'
import { devicesService } from '../services/devices'
import { errorMessage } from '../services/http'
import { api } from '../stores/auth'
import { useSensitiveVisibility } from '../composables/useSensitiveVisibility'
import RefreshButton from './RefreshButton.vue'
import LoadingScreen from './LoadingScreen.vue'
import EmptyState from './EmptyState.vue'
import SvgIcon from './SvgIcon.vue'
import SensitiveText from './SensitiveText.vue'
import simIcon from '@material-symbols/svg-400/outlined/sim_card.svg?raw'
import notificationsIcon from '@material-symbols/svg-400/outlined/notifications.svg?raw'
import visibleIcon from '@material-symbols/svg-400/outlined/visibility.svg?raw'
import hiddenIcon from '@material-symbols/svg-400/outlined/visibility_off.svg?raw'
import moreIcon from '@material-symbols/svg-400/outlined/more_vert.svg?raw'
import infoIcon from '@material-symbols/svg-400/outlined/info.svg?raw'
import editIcon from '@material-symbols/svg-400/outlined/edit.svg?raw'
import deleteIcon from '@material-symbols/svg-400/outlined/delete.svg?raw'
import downloadIcon from '@material-symbols/svg-400/outlined/download.svg?raw'
import addIcon from '@material-symbols/svg-400/outlined/add.svg?raw'
import enableIcon from '@material-symbols/svg-400/outlined/toggle_on.svg?raw'
import disableIcon from '@material-symbols/svg-400/outlined/toggle_off.svg?raw'
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
const notificationIcons: Record<string, string> = {
  install: downloadIcon,
  enable: enableIcon,
  disable: disableIcon,
  delete: deleteIcon
}

const loading = ref(false)
const profilesRefreshing = ref(false)
const chipInfo = ref<EsimChipInfo | null>(null)
const profiles = ref<EsimEUICCProfiles[]>([])
const profileGroups = computed(() => profiles.value.map(group => {
  const chips = chipInfo.value?.eids || []
  const aid = normalizeAidHex(group.aid_hex)
  const entries = group.profiles || []
  const chip = chips.find(item => !!aid && normalizeAidHex(item.aid) === aid) ||
    chips.find(item => !!group.eid && item.eid === group.eid)
  return { ...group, profiles: entries, chip }
}))

// 操作状态
const switching = ref<string | null>(null)
const switchSubmitted = ref(false)
let switchRequestId = 0
const switchBusy = computed(() => switching.value !== null || switchSubmitted.value || props.esimSwitching === true)
watch(() => props.esimSwitching, (busy) => {
  if (!busy) switchSubmitted.value = false
})
const deleting = ref<string | null>(null)
const renaming = ref<{ iccid: string; aidHex: string } | null>(null)
const showSensitive = useSensitiveVisibility()
const renameValue = ref('')
const selectedChip = ref<EsimEUICCInfo | null>(null)
const selectedChipNumber = computed(() => profileGroups.value.findIndex(({ chip }) =>
  chip?.eid === selectedChip.value?.eid &&
  normalizeAidHex(chip?.aid) === normalizeAidHex(selectedChip.value?.aid)
) + 1)
const downloadDialogOpen = ref(false)
const profileMenuOpen = ref(false)
const profileMenu = ref<{ anchor: string; profile: EsimProfileItem; aidHex: string; deviceId: string } | null>(null)
let pendingProfileAction: 'rename' | 'delete' | null = null

function displayIdentifier(value: string) {
  return showSensitive.value || value.length <= 8 ? value : `···· ${value.slice(-4)}`
}

function hasChipDetails(chip?: EsimEUICCInfo) {
  return !!(chip && (chip.manufacturer || chip.certificates?.length || chip.default_smdp_address ||
    chip.root_ds_address || chip.sas_accreditation_number || chip.info_source))
}

function openProfileMenu(event: Event, profile: EsimProfileItem, aidHex: string) {
  profileMenu.value = { anchor: (event.currentTarget as HTMLElement).id, profile, aidHex, deviceId: props.deviceId }
  pendingProfileAction = null
  profileMenuOpen.value = true
}

function selectProfileAction(action: 'rename' | 'delete') {
  pendingProfileAction = action
  profileMenuOpen.value = false
}

function handleProfileMenuClosed() {
  const target = profileMenu.value
  const action = pendingProfileAction
  profileMenuOpen.value = false
  profileMenu.value = null
  pendingProfileAction = null
  if (!target || target.deviceId !== props.deviceId || switchBusy.value || deleting.value !== null) return
  if (action === 'rename') startRename(target.profile.iccid, target.profile.name, target.aidHex)
  if (action === 'delete' && !downloading.value) {
    void deleteProfile(target.profile.iccid, target.profile.name, target.aidHex)
  }
}

function closeTransientViews() {
  pendingProfileAction = null
  profileMenuOpen.value = false
  selectedChip.value = null
  downloadDialogOpen.value = false
}
const emit = defineEmits<{ editCardSettings: [profile: EsimProfileItem] }>()
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
const downloadTarget = computed(() => chipInfo.value?.eids.find(chip =>
  normalizeAidHex(chip.aid) === normalizeAidHex(downloadForm.value.aidHex)
))
const downloading = ref(false)
const downloadProgress = ref(0)
const downloadMsg = ref('')
const downloadError = ref('')
const downloadSessionId = ref(0)
const downloadEntryMessage = computed(() => downloadError.value || (downloading.value
  ? `${downloadMsg.value} · ${Math.round(downloadProgress.value)}%`
  : '使用 SM-DP+ 地址或 LPA 激活码'))
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

async function fetchProfiles() {
  const deviceId = props.deviceId
  profilesRefreshing.value = true
  try {
    // GetProfiles shares the invalidated overview's background load.
    const result = await devicesService.getEsimProfiles(deviceId)
    if (deviceId !== props.deviceId) return
    if (!result.ok) throw result.error
    profiles.value = result.data
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
    message: `确定要${action}此 eSIM 配置 (${iccid}) 吗？切换后设备会短暂断网。`,
    title: `${action} eSIM 配置`,
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
function startRename(iccid: string, currentName: string, aidHex: string) {
  renaming.value = { iccid, aidHex: normalizeAidHex(aidHex) }
  renameValue.value = currentName
}

// 保存名称
async function saveRename(iccid: string, aidHex: string) {
  const deviceId = props.deviceId
  const name = renameValue.value.trim()
  if (!name) {
    notify({ tone: 'warning', message: '名称不能为空' })
    return
  }
  try {
    const result = await devicesService.renameEsimProfile(deviceId, iccid, { name, aid_hex: aidHex })
    if (deviceId !== props.deviceId) return
    if (!result.ok) throw result.error
    const group = profiles.value.find(group => normalizeAidHex(group.aid_hex) === normalizeAidHex(aidHex))
    const profile = group?.profiles.find(profile => profile.iccid === iccid)
    if (profile) profile.name = name
    notify({ tone: 'success', message: '名称修改成功' })
    renaming.value = null
    try {
      await fetchProfiles()
    } catch (e: unknown) {
      if (deviceId === props.deviceId) {
        notify({ tone: 'warning', message: `名称已保存，但列表刷新失败：${errorMessage(e, '请稍后重试')}` })
      }
    }
  } catch (e: unknown) {
    if (deviceId === props.deviceId) {
      notify({ tone: 'error', message: errorMessage(e, '修改名称失败') })
    }
  }
}

// 取消编辑
function cancelRename() {
  renaming.value = null
  renameValue.value = ''
}

function focusRenameInput(element: Element) {
  if (props.isActive === false || !element.matches('.esim-rename-form')) return
  element.querySelector<HTMLElement>('md-outlined-text-field')?.focus()
}

function onRenameKeydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.key === 'Enter') {
    event.preventDefault()
    if (!event.repeat) (event.currentTarget as HTMLElement).closest('form')?.requestSubmit()
  } else if (event.key === 'Escape') {
    event.stopPropagation()
    cancelRename()
  }
}

function beforeProfileLeave(element: Element) {
  const row = element as HTMLElement
  row.style.setProperty('--esim-profile-height', `${row.getBoundingClientRect().height}px`)
}

// 删除 profile（需要输入 ICCID 后 4 位确认）
async function deleteProfile(iccid: string, name: string, aidHex: string) {
  const last4 = iccid.slice(-4)
  const { value: input } = await requestConfirmation({
    message: `此操作不可逆！请输入 ICCID 后 4 位「${last4}」以确认删除 eSIM 配置「${name}」`,
    title: '⚠️ 删除 eSIM 配置',
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
              ? 'eUICC 安装 eSIM 配置时空间不足，请删除未使用的 eSIM 配置后重试。'
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
            downloadDialogOpen.value = false
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
    if (!newId || !newActive) {
      closeTransientViews()
      return
    }

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
  closeTransientViews()
  switchRequestId++
  switching.value = null
  switchSubmitted.value = false
})

watch(() => props.deviceImei, () => {
  applyDeviceImeiDefault(false)
})

onBeforeUnmount(() => {
  closeTransientViews()
  switchRequestId++
  clearRecentSpaceDelta()
  if (fetchAbortController) {
    fetchAbortController.abort()
  }
})
</script>

<template>
  <section class="m3-stack esim-page" aria-label="eSIM 管理">
    <LoadingScreen v-if="loading" title="读取 eUICC…" subtitle="正在获取芯片和 eSIM 配置信息" />
    <template v-else>
      <div class="m3-row esim-toolbar">
        <div class="m3-heading">
          <h2 class="m3-title">{{ chipInfo?.sku_name || 'eSIM 管理' }}</h2>
          <p v-if="chipInfo" class="m3-muted">
            {{ chipInfo.firmware ? `固件 ${chipInfo.firmware}` : '' }}
            <SensitiveText :concealed="!showSensitive">{{
              chipInfo.serial_number ? `· SN ${chipInfo.serial_number}` : ''
            }}</SensitiveText>
          </p>
        </div>
        <div class="m3-actions esim-tools">
          <RefreshButton variant="text" :loading="profilesRefreshing" @click="fetchOverview(true)" />
          <md-text-button :disabled="notificationsLoading" aria-label="当前通知" @click="openNotificationsDialog">
            <SvgIcon slot="icon" :svg="notificationsIcon" />
            通知
          </md-text-button>
          <md-icon-button
            :aria-label="showSensitive ? '隐藏敏感信息' : '显示敏感信息'"
            :title="showSensitive ? '隐藏敏感信息' : '显示敏感信息'"
            :aria-pressed="showSensitive"
            @click="showSensitive = !showSensitive"
          >
            <SvgIcon :svg="showSensitive ? visibleIcon : hiddenIcon" />
          </md-icon-button>
        </div>
      </div>
      <p v-if="switchBusy" class="m3-notice" role="status">切卡处理中，请等待设备服务恢复。</p>
      <section
        v-for="(group, index) in profileGroups"
        :key="group.aid_hex || group.eid || index"
        class="esim-group"
        :aria-label="`eUICC #${index + 1}`"
      >
        <div class="m3-inset esim-chip-summary">
          <span class="esim-avatar chip-avatar" aria-hidden="true"><SvgIcon :svg="simIcon" /></span>
          <div class="esim-chip-identity">
            <div class="esim-chip-title">
              <h3>eUICC #{{ index + 1 }}</h3>
              <span class="m3-muted esim-count">{{ group.profiles.length }} 项 eSIM 配置</span>
            </div>
            <p class="m3-muted esim-eid" :title="showSensitive ? group.eid : undefined">
              <span class="esim-id-label">EID</span>
              <SensitiveText :concealed="!showSensitive">{{ group.eid }}</SensitiveText>
            </p>
          </div>
          <div v-if="group.chip" class="esim-free-space">
            <span class="m3-muted">可用空间</span>
            <strong>{{ group.chip.free_nvram || '--' }}</strong>
          </div>
          <md-icon-button
            v-if="hasChipDetails(group.chip)"
            aria-label="芯片与证书信息"
            title="芯片与证书信息"
            @click="selectedChip = group.chip || null"
          ><SvgIcon :svg="infoIcon" /></md-icon-button>
        </div>
        <p
          v-if="recentSpaceDelta && normalizeAidHex(group.aid_hex) === recentSpaceDelta.aidHex"
          class="m3-muted esim-space-delta"
          role="status"
        >{{ recentSpaceDelta.message }}</p>
        <Transition name="esim-collapse" mode="out-in">
          <div v-if="group.profiles.length" class="esim-collapse">
            <TransitionGroup name="esim-profile" tag="div" class="esim-profile-list" @before-leave="beforeProfileLeave">
              <div key="heading" class="esim-profile-list-heading">
                <span>eSIM 配置</span>
              </div>
              <article
                v-for="profile in group.profiles"
                :key="profile.iccid"
                class="esim-profile"
                :data-active="profile.state === 1"
              >
                <div class="esim-profile-content">
                  <Transition name="esim-rename" @after-enter="focusRenameInput">
                    <div v-if="renaming?.iccid !== profile.iccid || renaming?.aidHex !== normalizeAidHex(group.aid_hex)" class="esim-profile-row">
                      <span class="esim-avatar" aria-hidden="true"><SvgIcon :svg="simIcon" /></span>
                      <div class="esim-profile-identity">
                        <div class="esim-profile-heading">
                          <h4><SensitiveText :concealed="!profile.name && !showSensitive">{{ profile.name || profile.iccid }}</SensitiveText></h4>
                          <span class="esim-profile-state">
                            <span v-if="profile.state === 1" class="esim-state-dot" aria-hidden="true"></span>
                            {{ profile.state_text || (profile.state === 1 ? '已启用' : '未启用') }}
                          </span>
                        </div>
                        <p class="m3-muted esim-profile-meta">
                          <span v-if="profile.service_provider_name" class="esim-provider">{{ profile.service_provider_name }}</span>
                          <span v-if="profile.service_provider_name" class="esim-meta-separator" aria-hidden="true">·</span>
                          <span class="esim-profile-iccid" :title="showSensitive ? profile.iccid : undefined">
                            <span class="esim-id-label">ICCID</span>
                            <SensitiveText :concealed="!showSensitive">{{ profile.iccid }}</SensitiveText>
                          </span>
                        </p>
                      </div>
                      <div class="m3-actions esim-profile-actions">
                        <md-outlined-button
                          v-if="profile.state !== 1"
                          class="esim-switch"
                          :disabled="switchBusy || deleting !== null || downloading"
                          @click="switchProfile(profile.iccid, profile.state, group.aid_hex)"
                        >{{ switching === profile.iccid ? '切换中…' : '切换' }}</md-outlined-button>
                        <md-text-button
                          class="esim-policy"
                          aria-haspopup="dialog"
                          :disabled="switchBusy || deleting !== null"
                          @click="emit('editCardSettings', profile)"
                        >SIM 卡设置</md-text-button>
                        <md-icon-button
                          :id="`esim-profile-actions-${deviceId}-${index}-${profile.iccid}`"
                          class="esim-profile-more"
                          :aria-label="`更多操作：${profile.name || displayIdentifier(profile.iccid)}`"
                          aria-haspopup="menu"
                          :aria-expanded="profileMenuOpen && profileMenu?.profile.iccid === profile.iccid && profileMenu?.aidHex === group.aid_hex"
                          :disabled="switchBusy || deleting !== null"
                          @click="openProfileMenu($event, profile, group.aid_hex)"
                        ><SvgIcon :svg="moreIcon" /></md-icon-button>
                      </div>
                    </div>
                    <form v-else class="m3-form esim-rename-form" @submit.prevent="saveRename(profile.iccid, group.aid_hex)">
                      <md-outlined-text-field
                        label="eSIM 配置名称"
                        name="esim-profile-name"
                        :value="renameValue"
                        @input="renameValue = fieldValue($event)"
                        @keydown="onRenameKeydown"
                      />
                      <div class="m3-actions">
                        <md-text-button type="button" @click="cancelRename">取消</md-text-button>
                        <md-filled-button type="submit">保存</md-filled-button>
                      </div>
                    </form>
                  </Transition>
                </div>
              </article>
            </TransitionGroup>
          </div>
          <div v-else class="esim-collapse">
            <p class="m3-muted esim-empty-profiles">暂无 eSIM 配置</p>
          </div>
        </Transition>
      </section>
      <md-menu
        class="esim-profile-menu"
        :anchor="profileMenu?.anchor || ''"
        :open="profileMenuOpen"
        positioning="popover"
        anchor-corner="end-end"
        menu-corner="start-end"
        @closed="handleProfileMenuClosed"
      >
        <md-menu-item :disabled="switchBusy || deleting !== null" @click="selectProfileAction('rename')">
          <SvgIcon slot="start" :svg="editIcon" />
          <div slot="headline">改名</div>
        </md-menu-item>
        <md-menu-item class="danger" :disabled="switchBusy || deleting !== null || downloading" @click="selectProfileAction('delete')">
          <SvgIcon slot="start" :svg="deleteIcon" />
          <div slot="headline">{{ deleting ? '删除中…' : '删除' }}</div>
        </md-menu-item>
      </md-menu>
      <md-dialog class="m3-dialog esim-chip-dialog" :open="!!selectedChip" @closed="selectedChip = null">
        <div slot="headline">芯片与证书信息</div>
        <div slot="content" class="esim-chip-details">
          <section class="chip-details-card chip-details-identity">
            <span class="esim-avatar chip-avatar"><SvgIcon :svg="simIcon" /></span>
            <div>
              <h3>{{ selectedChipNumber ? `eUICC #${selectedChipNumber}` : 'eUICC' }}</h3>
              <p class="chip-details-caption">芯片标识</p>
            </div>
            <p class="esim-eid">
              <span class="esim-id-label">EID</span>
              <SensitiveText :concealed="!showSensitive">{{ selectedChip?.eid || '' }}</SensitiveText>
            </p>
          </section>
          <section
            v-if="selectedChip?.manufacturer || selectedChip?.sas_accreditation_number"
            class="chip-details-card"
          >
            <h3>芯片信息</h3>
            <dl class="chip-details-meta">
              <div v-if="selectedChip?.manufacturer"><dt>生产商</dt><dd>{{ selectedChip.manufacturer }}</dd></div>
              <div v-if="selectedChip?.sas_accreditation_number"><dt>SAS</dt><dd class="chip-details-code">{{ selectedChip.sas_accreditation_number }}</dd></div>
            </dl>
          </section>
          <section
            v-if="selectedChip?.certificates?.length || selectedChip?.default_smdp_address || selectedChip?.root_ds_address"
            class="chip-details-card"
          >
            <h3>证书与服务</h3>
            <dl class="chip-details-meta">
              <div v-if="selectedChip?.certificates?.length">
                <dt>证书</dt>
                <dd class="chip-details-certificates">
                  <p v-for="(certificate, index) in selectedChip.certificates" :key="index">{{ certificate }}</p>
                </dd>
              </div>
              <div v-if="selectedChip?.default_smdp_address"><dt>Default SM-DP+</dt><dd>{{ selectedChip.default_smdp_address }}</dd></div>
              <div v-if="selectedChip?.root_ds_address"><dt>Root SM-DS</dt><dd>{{ selectedChip.root_ds_address }}</dd></div>
            </dl>
          </section>
          <p v-if="selectedChip?.info_source" class="chip-details-source">
            <span>信息来源</span><span class="chip-details-code">{{ selectedChip.info_source }}</span>
          </p>
        </div>
        <div slot="actions"><md-text-button @click="selectedChip = null">关闭</md-text-button></div>
      </md-dialog>
      <md-dialog
        class="m3-dialog esim-notifications-dialog"
        :open="notificationsDialogOpen"
        @closed="notificationsDialogOpen = false"
      >
        <div slot="headline">当前通知</div>
        <div slot="content" class="esim-notifications-content">
          <div v-if="notificationsLoading" class="notification-state" role="status">
            <md-circular-progress indeterminate />
            <p>正在加载通知…</p>
          </div>
          <div v-else-if="!notifications.length" class="notification-state">
            <span class="notification-state-icon"><SvgIcon :svg="notificationsIcon" /></span>
            <h3>暂无通知</h3>
            <p>目前没有可展示的 eSIM 通知。</p>
          </div>
          <template v-else>
            <div class="notification-summary">
              <span>{{ notifications.length }} 条通知</span>
              <span>来自 eUICC</span>
            </div>
            <div class="notification-list">
              <article
                v-for="item in notifications"
                :key="item.sequence_number"
                class="notification-card"
              >
                <div class="notification-heading">
                  <div class="notification-identity">
                    <span class="esim-avatar notification-avatar">
                      <SvgIcon :svg="notificationIcons[item.event] || notificationsIcon" />
                    </span>
                    <h3>{{ formatEsimNotificationEvent(item.event) }}</h3>
                    <span class="notification-sequence">#{{ item.sequence_number }}</span>
                  </div>
                  <md-outlined-button
                    :aria-label="`重发通知 #${item.sequence_number}`"
                    :disabled="!item.can_retry || retryingNotificationSequence !== null"
                    @click="retryNotification(item)"
                  >
                    <md-circular-progress
                      v-if="retryingNotificationSequence === item.sequence_number"
                      slot="icon"
                      class="m3-progress"
                      indeterminate
                    />
                    {{ retryingNotificationSequence === item.sequence_number ? '重发中…' : '重发' }}
                  </md-outlined-button>
                </div>
                <dl v-if="item.iccid || item.address || item.aid_hex" class="notification-meta">
                  <div v-if="item.iccid"><dt>ICCID</dt><dd class="notification-identifier">{{ item.iccid }}</dd></div>
                  <div v-if="item.address"><dt>地址</dt><dd>{{ item.address }}</dd></div>
                  <div v-if="item.aid_hex" class="notification-aid"><dt>AID</dt><dd class="notification-identifier">{{ item.aid_hex }}</dd></div>
                </dl>
              </article>
            </div>
          </template>
        </div>
        <div slot="actions">
          <md-text-button @click="notificationsDialogOpen = false">关闭</md-text-button>
        </div>
      </md-dialog>
      <section v-if="chipInfo" class="m3-inset esim-download-entry">
        <span class="esim-avatar download-avatar" aria-hidden="true"><SvgIcon :svg="downloadIcon" /></span>
        <div class="esim-download-heading">
          <h3>下载新 eSIM 配置</h3>
          <p
            class="m3-muted"
            :class="{ 'm3-error': !!downloadError }"
            :role="downloadError ? 'alert' : downloading ? 'status' : undefined"
          >{{ downloadEntryMessage }}</p>
        </div>
        <md-filled-tonal-button @click="downloadDialogOpen = true">
          <SvgIcon slot="icon" :svg="addIcon" />
          {{ downloadError ? '查看详情' : downloading ? '查看进度' : '下载 eSIM 配置' }}
        </md-filled-tonal-button>
      </section>
      <md-dialog class="m3-dialog esim-download-dialog" :open="downloadDialogOpen" @closed="downloadDialogOpen = false">
        <div slot="headline">下载新 eSIM 配置</div>
        <form :id="`esim-download-form-${deviceId}`" slot="content" class="m3-form esim-download-form" @submit.prevent="downloadProfile">
          <div class="esim-download-layout">
            <md-outlined-text-field
              class="esim-download-activation"
              label="激活码或服务器地址"
              supporting-text="支持 SM-DP+ 地址或完整 LPA 激活码"
              name="esim-smdp"
              required
              :value="downloadForm.smdp"
              :disabled="downloading"
              @input="downloadForm.smdp = fieldValue($event)"
            />
            <aside class="esim-download-target">
              <div class="esim-download-target-heading">
                <span class="esim-avatar chip-avatar"><SvgIcon :svg="simIcon" /></span>
                <div>
                  <h3>下载目标</h3>
                  <p>选择接收配置的芯片</p>
                </div>
              </div>
              <md-outlined-select
                label="目标 eUICC"
                name="esim-download-target"
                :value="downloadForm.aidHex"
                :disabled="downloading"
                @change="downloadForm.aidHex = fieldValue($event)"
              >
                <md-select-option
                  v-for="(eid, index) in chipInfo?.eids || []"
                  :key="eid.aid"
                  :value="eid.aid"
                  :selected="downloadForm.aidHex === eid.aid"
                >
                  <div slot="headline">eUICC #{{ index + 1 }}（…{{ eid.eid.slice(-4) }}）</div>
                </md-select-option>
              </md-outlined-select>
              <div class="esim-download-space">
                <span>可用空间</span>
                <strong>{{ downloadTarget?.free_nvram || '--' }}</strong>
              </div>
            </aside>
            <section class="esim-download-extra">
              <h3>补充信息</h3>
              <md-outlined-text-field
                label="Matching ID（可选）"
                name="esim-matching-id"
                :value="downloadForm.matchingId"
                :disabled="downloading"
                @input="downloadForm.matchingId = fieldValue($event)"
              />
              <md-outlined-text-field
                label="确认码（可选）"
                name="esim-confirmation-code"
                :value="downloadForm.confirmationCode"
                :disabled="downloading"
                @input="downloadForm.confirmationCode = fieldValue($event)"
              />
              <md-outlined-text-field
                label="IMEI"
                name="esim-imei"
                maxlength="15"
                :supporting-text="deviceImei?.trim() ? '默认填入当前设备 IMEI，可按需修改' : '可按运营商要求填写 IMEI'"
                :value="downloadForm.imei"
                :disabled="downloading"
                @input="downloadForm.imei = fieldValue($event)"
              />
            </section>
          </div>
          <div v-if="downloading || downloadError" class="m3-stack">
            <md-linear-progress
              :key="downloadSessionId"
              :value="downloadProgress / 100"
              :indeterminate="downloading && downloadProgress === 0"
              aria-label="eSIM 配置下载进度"
            />
            <p
              :class="downloadError ? 'm3-error' : 'm3-muted'"
              :role="downloadError ? 'alert' : 'status'"
            >
              {{ downloadError || downloadMsg }} · {{ Math.round(downloadProgress) }}%
            </p>
          </div>
        </form>
        <div slot="actions">
          <md-text-button @click="downloadDialogOpen = false">关闭</md-text-button>
          <md-filled-button
            type="submit"
            :form="`esim-download-form-${deviceId}`"
            :disabled="downloading || switchBusy || deleting !== null"
          >{{ downloading ? '下载中…' : '开始下载' }}</md-filled-button>
        </div>
      </md-dialog>
      <EmptyState
        v-if="!profiles.length && !chipInfo"
        title="未检测到 eUICC"
        subtitle="此 SIM 卡可能不支持 eUICC 功能"
      />
    </template>
  </section>
</template>
<style scoped>
.esim-page {
  gap: 16px;
  container: esim-panel / inline-size;
}
h3,
h4 {
  margin: 0;
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  overflow-wrap: anywhere;
}
.esim-page p {
  overflow-wrap: anywhere;
}
.esim-toolbar {
  flex-wrap: wrap;
  gap: 12px;
}
.esim-tools {
  flex: none;
  gap: 4px;
  padding-right: 4px;
}
.esim-group {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-width: 0;
}
.esim-chip-summary {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
}
.esim-avatar {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  flex: none;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-on-surface-variant);
}
.chip-avatar {
  background: var(--md-sys-color-primary-container);
  color: var(--md-sys-color-on-primary-container);
}
.esim-chip-identity,
.esim-profile-identity {
  min-width: 0;
}
.esim-chip-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 12px;
}
.esim-count,
.esim-free-space > span {
  font-size: 12px;
  line-height: 16px;
}
.esim-eid {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font: 12px/20px var(--vh-typeface-mono);
}
.esim-id-label {
  flex: none;
}
.esim-chip-identity .esim-eid {
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.esim-free-space {
  display: grid;
  gap: 4px;
  text-align: right;
}
.esim-free-space strong {
  font-size: 18px;
  font-weight: 500;
  line-height: 24px;
  white-space: nowrap;
}
.esim-space-delta {
  font-size: 12px;
  line-height: 16px;
}
.esim-profile-list {
  overflow: hidden;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 12px;
}
.esim-profile-list-heading {
  padding: 12px 16px;
  font-size: 12px;
  line-height: 16px;
  color: var(--md-sys-color-on-surface-variant);
}
.esim-profile {
  min-width: 0;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.esim-profile[data-active='true'] {
  background: color-mix(in srgb, var(--vh-success) 6%, transparent);
}
.esim-profile-content {
  display: grid;
  align-items: start;
}
.esim-profile-content > * {
  grid-area: 1 / 1;
  min-width: 0;
}
.esim-collapse {
  display: grid;
  grid-template-rows: 1fr;
  min-width: 0;
}
.esim-collapse > * {
  min-height: 0;
  overflow: hidden;
}
@media (prefers-reduced-motion: no-preference) {
  .esim-collapse-enter-active,
  .esim-collapse-leave-active {
    transition: grid-template-rows 220ms cubic-bezier(0.2, 0, 0, 1), opacity 220ms;
  }
  .esim-collapse-enter-from,
  .esim-collapse-leave-to {
    grid-template-rows: 0fr;
    opacity: 0;
  }
  .esim-rename-enter-active,
  .esim-rename-leave-active {
    transition: opacity 180ms;
  }
  .esim-rename-enter-from,
  .esim-rename-leave-to {
    opacity: 0;
  }
  .esim-profile-leave-active {
    height: var(--esim-profile-height);
    overflow: hidden;
    transition: height 220ms cubic-bezier(0.2, 0, 0, 1), opacity 180ms;
  }
  .esim-profile-leave-to {
    height: 0;
    opacity: 0;
  }
}
.esim-collapse-leave-active,
.esim-rename-leave-active,
.esim-profile-leave-active {
  pointer-events: none;
}
.esim-profile-row {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  min-height: 88px;
  padding: 12px 16px;
}
.esim-profile-heading {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.esim-profile-identity h4 {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.esim-profile-meta {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-top: 4px;
  font-size: 13px;
  line-height: 20px;
}
.esim-provider,
.esim-profile-iccid {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.esim-provider {
  min-width: 0;
}
.esim-profile-iccid {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  flex-shrink: 0;
  max-width: 100%;
  font: 12px/20px var(--vh-typeface-mono);
}
.esim-meta-separator {
  color: var(--md-sys-color-outline);
}
.esim-profile-state {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--md-sys-color-surface-container-high);
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
  white-space: nowrap;
}
.esim-profile[data-active='true'] .esim-avatar,
.esim-profile[data-active='true'] .esim-profile-state {
  background: var(--vh-success-container);
  color: var(--vh-on-success-container);
}
.esim-state-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.esim-profile-actions {
  flex-wrap: nowrap;
  gap: 4px;
}
.esim-switch {
  --md-outlined-button-container-height: 36px;
  --md-outlined-button-leading-space: 20px;
  --md-outlined-button-trailing-space: 20px;
}
.esim-rename-form {
  padding: 16px;
}

.esim-empty-profiles {
  padding: 16px;
}
.esim-profile-menu {
  min-width: 152px;
}
.esim-profile-menu .danger {
  --md-menu-item-label-text-color: var(--md-sys-color-error);
  --md-menu-item-leading-icon-color: var(--md-sys-color-error);
}
.esim-chip-dialog {
  max-height: calc(100dvh - 48px);
}
.esim-chip-details {
  display: grid;
  gap: 12px;
  overflow-wrap: anywhere;
}
.chip-details-card {
  display: grid;
  gap: 16px;
  min-width: 0;
  padding: 16px;
  border-radius: 16px;
  background: var(--md-sys-color-surface-container);
}
.chip-details-card > h3 {
  font-size: 14px;
  line-height: 20px;
}
.chip-details-identity {
  grid-template-columns: 40px minmax(0, 1fr);
  align-items: center;
  gap: 8px 12px;
  margin-bottom: 4px;
}
.chip-details-caption {
  margin-top: 2px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
}
.chip-details-identity .esim-eid {
  grid-column: 1 / -1;
  min-width: 0;
  padding-top: 4px;
  white-space: normal;
}
.chip-details-meta {
  display: grid;
  gap: 12px;
  margin: 0;
}
.chip-details-meta > div {
  display: flex;
  align-items: baseline;
  gap: 16px;
  min-width: 0;
}
.chip-details-meta dt {
  width: 96px;
  flex: none;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 20px;
}
.chip-details-meta dd {
  min-width: 0;
  margin: 0;
  color: var(--md-sys-color-on-surface);
  font-size: 14px;
  line-height: 22px;
}
.chip-details-meta .chip-details-code,
.chip-details-code {
  font: 12px/20px var(--vh-typeface-mono);
}
.chip-details-certificates {
  display: grid;
  gap: 8px;
}
.chip-details-source {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 12px;
  padding: 4px 4px 0;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 20px;
}
@media (max-width: 600px) {
  .chip-details-meta > div {
    display: grid;
    gap: 4px;
  }
  .chip-details-meta dt {
    width: auto;
  }
  .chip-details-identity .esim-eid {
    display: grid;
    gap: 6px;
  }
}
.esim-download-dialog {
  width: min(860px, calc(100vw - 48px));
  max-height: calc(100dvh - 48px);
}
.esim-download-form {
  gap: 24px;
}
.esim-download-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 252px;
  align-items: start;
  gap: 24px;
}
.esim-download-activation {
  grid-column: 1;
  grid-row: 1;
}
.esim-download-target {
  display: grid;
  grid-column: 2;
  grid-row: 1 / span 2;
  gap: 24px;
  min-width: 0;
  padding: 16px;
  border-radius: 16px;
  background: var(--md-sys-color-surface-container);
}
.esim-download-target-heading {
  display: flex;
  align-items: center;
  gap: 12px;
}
.esim-download-target h3,
.esim-download-extra h3 {
  font-size: 14px;
  line-height: 20px;
}
.esim-download-target-heading p {
  margin-top: 4px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 20px;
}
.esim-download-space {
  display: grid;
  gap: 4px;
  padding-top: 16px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.esim-download-space > span {
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 20px;
}
.esim-download-space strong {
  font-size: 20px;
  font-weight: 500;
  line-height: 28px;
  overflow-wrap: anywhere;
}
.esim-download-extra {
  display: grid;
  grid-column: 1;
  grid-row: 2;
  gap: 16px;
  min-width: 0;
}
@media (max-width: 600px) {
  .esim-download-layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .esim-download-activation,
  .esim-download-target,
  .esim-download-extra {
    grid-column: 1;
    grid-row: auto;
  }
  .esim-download-target {
    gap: 16px;
  }
  .esim-download-space {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: 16px;
  }
  .esim-download-space strong {
    font-size: 18px;
    line-height: 24px;
  }
  .esim-download-extra {
    padding-top: 20px;
    border-top: 1px solid var(--md-sys-color-outline-variant);
  }
}
.esim-download-entry {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
}
.download-avatar {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.esim-download-heading {
  min-width: 0;
}
.esim-download-heading h3 {
  font-size: 14px;
  line-height: 20px;
}
.esim-download-heading p {
  margin-top: 4px;
  font-size: 12px;
  line-height: 16px;
}
.esim-notifications-dialog {
  max-height: calc(100dvh - 48px);
}
.esim-notifications-content {
  display: grid;
  gap: 16px;
}
.notification-summary,
.notification-heading,
.notification-identity {
  display: flex;
  align-items: center;
  gap: 12px;
}
.notification-summary,
.notification-heading {
  justify-content: space-between;
}
.notification-summary,
.notification-sequence {
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 20px;
}
.notification-list {
  display: grid;
  gap: 12px;
}
.notification-card {
  display: grid;
  gap: 12px;
  padding: 16px;
  border-radius: 16px;
  background: var(--md-sys-color-surface-container);
}
.notification-identity {
  min-width: 0;
  flex-wrap: wrap;
  gap: 8px;
}
.notification-identity h3 {
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
  overflow-wrap: anywhere;
}
.notification-avatar {
  width: 36px;
  height: 36px;
  margin-right: 4px;
  border-radius: 50%;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.notification-avatar > .svg-icon {
  width: 20px;
  height: 20px;
}
.notification-heading > md-outlined-button {
  flex: none;
}
.notification-sequence,
.notification-identifier {
  font-family: var(--vh-typeface-mono);
}
.notification-meta {
  display: grid;
  gap: 8px;
  min-width: 0;
  margin: 0;
}
.notification-meta > div {
  display: flex;
  align-items: baseline;
  gap: 12px;
  min-width: 0;
}
.notification-meta dt {
  width: 44px;
  flex: none;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 20px;
}
.notification-meta dd {
  min-width: 0;
  margin: 0;
  color: var(--md-sys-color-on-surface);
  overflow-wrap: anywhere;
}
.notification-meta .notification-identifier {
  font-size: 12px;
  line-height: 20px;
}
.notification-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 24px 8px 32px;
  text-align: center;
  color: var(--md-sys-color-on-surface-variant);
}
.notification-state-icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  margin-bottom: 16px;
  border-radius: 20px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.notification-state h3 {
  color: var(--md-sys-color-on-surface);
  font-size: 16px;
  line-height: 24px;
}
@media (max-width: 600px) {
  .notification-card {
    padding: 12px;
  }
  .notification-avatar {
    margin-right: 0;
  }
  .notification-heading > md-outlined-button {
    --md-outlined-button-container-height: 36px;
    --md-outlined-button-leading-space: 16px;
    --md-outlined-button-trailing-space: 16px;
  }
  .notification-meta > .notification-aid {
    display: grid;
    gap: 4px;
  }
  .notification-meta > div {
    gap: 8px;
  }
  .notification-meta dt {
    width: 40px;
  }
}
@container esim-panel (max-width: 639px) {
  .esim-chip-summary {
    grid-template-columns: 40px minmax(0, 1fr) auto;
    padding: 12px;
  }
  .esim-chip-summary > md-icon-button {
    grid-column: 3;
    grid-row: 1;
  }
  .esim-free-space {
    grid-column: 2;
    grid-row: 2;
    display: flex;
    align-items: baseline;
    gap: 8px;
    text-align: left;
  }
  .esim-profile-row {
    grid-template-columns: 40px minmax(0, 1fr);
    gap: 8px;
    padding: 12px;
  }
  .esim-profile-heading {
    flex-wrap: wrap;
  }
  .esim-profile-identity h4 {
    max-width: 100%;
  }
  .esim-profile-actions {
    grid-column: 1 / -1;
    justify-self: end;
    max-width: 100%;
    flex-wrap: wrap;
  }
  .esim-profile-meta,
  .esim-provider {
    display: block;
  }
  .esim-meta-separator {
    display: none;
  }
  .esim-profile-list-heading {
    padding: 12px;
  }
  .esim-download-entry {
    grid-template-columns: 40px minmax(0, 1fr);
  }
  .esim-download-entry > md-filled-tonal-button {
    grid-column: 2;
    justify-self: start;
  }
}
</style>
