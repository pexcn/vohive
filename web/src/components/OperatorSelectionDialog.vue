<script setup lang="ts">
import { guardDialogCancel } from '../utils/material'
import { notify } from '../stores/feedback'
import { computed, ref, watch } from 'vue'
import { devicesService } from '../services/devices'
import type { OperatorCandidate, OperatorSelection, OperatorSelectionRAT } from '../types/api'
import { useDevicesStore } from '../stores/devices'
import SvgIcon from './SvgIcon.vue'
import antennaIcon from '@material-symbols/svg-400/outlined/settings_input_antenna.svg?raw'
import networkIcon from '@material-symbols/svg-400/outlined/cell_tower.svg?raw'
import searchIcon from '@material-symbols/svg-400/outlined/search.svg?raw'
import exploreIcon from '@material-symbols/svg-400/outlined/travel_explore.svg?raw'
import refreshIcon from '@material-symbols/svg-400/outlined/refresh.svg?raw'
import checkIcon from '@material-symbols/svg-400/outlined/check_circle.svg?raw'
import chevronIcon from '@material-symbols/svg-400/outlined/chevron_right.svg?raw'
import errorIcon from '@material-symbols/svg-400/outlined/error.svg?raw'
import warningIcon from '@material-symbols/svg-400/outlined/warning.svg?raw'
const props = defineProps<{
  modelValue: boolean
  deviceId: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'updated': []
}>()

const devicesStore = useDevicesStore()
const loading = ref(false)
const currentSelection = ref<OperatorSelection | null>(null)
const lastNotifiedScanKey = ref('')

const scanState = computed(() => {
  if (!props.deviceId) return null
  return devicesStore.getOperatorScan(props.deviceId)
})
const scanning = computed(() => scanState.value?.status === 'running')
const candidates = computed<OperatorCandidate[]>(() => scanState.value?.candidates || [])
const scanMessage = computed(() => scanState.value?.message || '')
const scanError = computed(() => (scanState.value?.retryable ? '' : (scanState.value?.error || '')))
const scanRetryable = computed(() => !!scanState.value?.retryable)
const scanFailed = computed(() => scanState.value?.status === 'failed')
const scanFeedback = computed(() => scanError.value || scanMessage.value || (
  scanning.value ? '正在扫描网络…'
    : scanFailed.value ? '扫描失败，请重试'
      : scanState.value?.status === 'complete' ? '扫描完成' : ''
))
const scanTone = computed(() => {
  if (scanError.value || (scanFailed.value && !scanRetryable.value)) return 'error'
  if (scanFailed.value && scanRetryable.value) return 'warning'
  return scanState.value?.status === 'complete' ? 'success' : ''
})
const scanFeedbackIcon = computed(() => (
  scanTone.value === 'error' ? errorIcon
    : scanTone.value === 'warning' ? warningIcon
      : scanTone.value === 'success' ? checkIcon : ''
))

function firstCandidateRAT(candidate: OperatorCandidate): OperatorSelectionRAT | undefined {
  return candidate.rats?.find(rat => !!rat)
}

function ratDisplay(candidate: OperatorCandidate) {
  const rats = candidate.rats?.filter(Boolean) || []
  return rats.length > 0 ? rats.map(rat => rat.toUpperCase()).join(' / ') : '--'
}

const loadCurrent = async () => {
  if (!props.deviceId) return
  loading.value = true
  try {
    const res = await devicesService.getOperatorSelection(props.deviceId)
    if (!res.ok) throw new Error(res.error.message)
    currentSelection.value = res.data
  } catch (e: any) {
    notify({ tone: 'error', message: e.message || '加载当前配置失败' })
  } finally {
    loading.value = false
  }
}

const doScan = async () => {
  if (!props.deviceId || scanning.value) return
  try {
    await devicesStore.startOperatorScan(props.deviceId)
  } catch (e: any) {
    notify({ tone: 'error', message: e.message || '扫描网络失败' })
  }
}

const setModeAuto = async () => {
  loading.value = true
  try {
    const res = await devicesService.setOperatorSelection(props.deviceId, { mode: 'automatic' })
    if (!res.ok) throw new Error(res.error.message)
    notify({ tone: 'success', message: '已恢复自动选网' })
    emit('updated')
    await loadCurrent()
  } catch (e: any) {
    notify({ tone: 'error', message: e.message || '设置失败' })
  } finally {
    loading.value = false
  }
}

const setModeManual = async (candidate: OperatorCandidate) => {
  loading.value = true
  try {
    const rat = firstCandidateRAT(candidate)
    const res = await devicesService.setOperatorSelection(props.deviceId, {
      mode: 'manual',
      plmn: candidate.plmn,
      includes_pcs_digit: candidate.includes_pcs_digit,
      rat
    })
    if (!res.ok) throw new Error(res.error.message)
    notify({ tone: 'success', message: `已锁定网络 ${candidate.plmn}` })
    emit('updated')
    await loadCurrent()
  } catch (e: any) {
    notify({ tone: 'error', message: e.message || '设置失败' })
  } finally {
    loading.value = false
  }
}

watch(() => props.modelValue, (val) => {
  if (val) {
    loadCurrent()
    if (props.deviceId) {
      void devicesStore.resumeOperatorScan(props.deviceId)
    }
  }
})

watch(scanState, (next) => {
  if (!next || !props.modelValue) return
  const notifyKey = `${next.scan_id}:${next.status}`
  if (notifyKey === lastNotifiedScanKey.value) return
  if (next.status === 'complete') {
    notify({ tone: 'success', message: '运营商扫描完成' })
  } else if (next.status === 'failed' && next.retryable) {
    notify({
      tone: 'warning',
      message: next.message || '扫描超时或模组忙，请稍后重试'
    })
  }
  lastNotifiedScanKey.value = notifyKey
})
</script>

<template>
  <md-dialog
    class="m3-dialog operator-dialog"
    :open="modelValue"
    @cancel="guardDialogCancel($event, loading)"
    @closed="emit('update:modelValue', false)"
  >
    <div slot="headline">运营商网络选择</div>
    <div slot="content" class="operator-content">
      <div class="operator-mode">
        <div class="operator-mode-main">
          <SvgIcon :svg="antennaIcon" />
          <div class="operator-mode-value">
            <span class="operator-caption">当前模式</span>
            <strong>{{
              currentSelection?.mode === 'automatic'
                ? '自动选网'
                : currentSelection
                  ? '手动锁定'
                  : '读取中'
            }}</strong>
            <p v-if="currentSelection?.mode === 'manual'" class="operator-mode-plmn">
              PLMN {{ currentSelection.plmn || '--' }}
            </p>
          </div>
        </div>
        <md-text-button
          :disabled="loading || currentSelection?.mode === 'automatic'"
          @click="setModeAuto"
        >恢复自动选网</md-text-button>
      </div>
      <section v-if="!scanState" class="operator-empty">
        <div class="operator-empty-icon"><SvgIcon :svg="exploreIcon" /></div>
        <h3>查找可用网络</h3>
        <p>扫描附近运营商，选择要锁定的网络</p>
        <md-filled-tonal-button :disabled="loading" @click="doScan">
          <SvgIcon slot="icon" :svg="searchIcon" />扫描可用网络
        </md-filled-tonal-button>
      </section>
      <div v-else class="operator-results-heading">
        <div class="operator-results-summary">
          <h3>可用网络 <span class="operator-count">{{ candidates.length }}</span></h3>
          <p v-if="scanFeedback" class="operator-feedback" :class="scanTone" role="status">
            <md-circular-progress v-if="scanning" indeterminate aria-hidden="true" />
            <SvgIcon v-else-if="scanFeedbackIcon" :svg="scanFeedbackIcon" />
            <span>{{ scanFeedback }}</span>
          </p>
        </div>
        <md-outlined-button :disabled="scanning || loading" @click="doScan">
          <SvgIcon slot="icon" :svg="refreshIcon" />{{ scanning ? '扫描中…' : '重新扫描' }}
        </md-outlined-button>
      </div>
      <div v-if="candidates.length" class="operator-list">
        <button
          v-for="candidate in candidates"
          :key="`${candidate.plmn}-${ratDisplay(candidate)}`"
          type="button"
          class="operator-network"
          :class="{ current: candidate.status === 'current' }"
          :disabled="loading"
          @click="setModeManual(candidate)"
        >
          <md-ripple :disabled="loading" />
          <SvgIcon class="operator-network-icon" :svg="networkIcon" />
          <span class="operator-identity">
            <strong>{{ candidate.operator_name || candidate.short_name || '未知网络' }}</strong>
            <small>{{ candidate.plmn }} · {{ ratDisplay(candidate) }}</small>
          </span>
          <span class="operator-status" :class="candidate.status">
            <SvgIcon v-if="candidate.status === 'current'" :svg="checkIcon" />{{
            candidate.status === 'current'
              ? '当前网络'
              : candidate.status === 'forbidden'
                ? '禁止'
                : '可选择'
            }}
          </span>
          <SvgIcon v-if="candidate.status !== 'current'" class="operator-chevron" :svg="chevronIcon" />
        </button>
      </div>
      <p v-else-if="scanState" class="operator-empty-message">{{
        scanning ? '正在查找可用网络…'
          : scanFailed ? '暂时没有可用结果，请重新扫描'
            : '未发现可用网络，可尝试重新扫描'
      }}</p>
    </div>
    <div slot="actions">
      <md-text-button :disabled="loading" @click="emit('update:modelValue', false)"
        >关闭</md-text-button
      >
    </div>
  </md-dialog>
</template>

<style scoped>
.operator-content {
  display: grid;
  gap: 20px;
}
.operator-content h3,
.operator-content p {
  margin: 0;
}
.operator-content h3 {
  color: var(--md-sys-color-on-surface);
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}
.operator-mode {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px 8px 16px;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container);
  color: var(--md-sys-color-on-surface);
}
.operator-mode-main {
  display: flex;
  flex: 1;
  align-items: center;
  gap: 12px;
  min-width: 0;
}
.operator-mode-main > .svg-icon {
  flex: none;
  color: var(--md-sys-color-primary);
}
.operator-mode-value {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 12px;
  min-width: 0;
}
.operator-caption,
.operator-count {
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  font-weight: 400;
  line-height: 16px;
}
.operator-mode-value strong {
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
  white-space: nowrap;
}
.operator-mode-plmn {
  flex-basis: 100%;
  color: var(--md-sys-color-on-surface-variant);
  font: 12px/20px var(--vh-typeface-mono);
  overflow-wrap: anywhere;
}
.operator-mode md-text-button {
  flex: none;
  margin-inline-start: auto;
}
.operator-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px 8px 16px;
  text-align: center;
}
.operator-empty-icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  margin-bottom: 16px;
  border-radius: 16px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.operator-empty-icon > .svg-icon {
  width: 32px;
  height: 32px;
}
.operator-empty p {
  margin-top: 8px;
  color: var(--md-sys-color-on-surface-variant);
}
.operator-empty md-filled-tonal-button {
  margin-top: 20px;
}
.operator-results-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.operator-results-summary {
  min-width: 0;
}
.operator-results-heading h3 {
  display: flex;
  align-items: center;
  gap: 8px;
}
.operator-results-heading md-outlined-button {
  flex: none;
}
.operator-content .operator-feedback {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 4px;
  font-size: 12px;
  line-height: 16px;
  overflow-wrap: anywhere;
}
.operator-feedback > .svg-icon,
.operator-feedback md-circular-progress {
  flex: none;
  width: 14px;
  height: 14px;
  --md-circular-progress-size: 14px;
}
.operator-feedback.success,
.operator-status.current,
.operator-network.current .operator-network-icon {
  color: var(--vh-success);
}
.operator-feedback.warning {
  color: var(--vh-warning);
}
.operator-feedback.error,
.operator-status.forbidden {
  color: var(--md-sys-color-error);
}
.operator-list {
  overflow: hidden;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 12px;
}
.operator-network {
  position: relative;
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  min-height: 72px;
  padding: 12px 16px;
  background: transparent;
  color: var(--md-sys-color-on-surface);
  text-align: left;
}
.operator-network + .operator-network {
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.operator-network.current {
  background: color-mix(in srgb, var(--vh-success) 6%, transparent);
}
.operator-network:disabled {
  opacity: 0.55;
}
.operator-network:focus-visible {
  outline: 2px solid var(--md-sys-color-primary);
  outline-offset: -2px;
}
.operator-network-icon.svg-icon {
  flex: none;
  width: 20px;
  height: 20px;
  color: var(--md-sys-color-on-surface-variant);
}
.operator-identity {
  display: grid;
  flex: 1;
  gap: 4px;
  min-width: 0;
  overflow-wrap: anywhere;
}
.operator-identity strong {
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
}
.operator-identity small {
  color: var(--md-sys-color-on-surface-variant);
  font: 12px/16px var(--vh-typeface-mono);
}
.operator-status {
  display: inline-flex;
  flex: none;
  align-items: center;
  gap: 4px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
}
.operator-status > .svg-icon {
  width: 16px;
  height: 16px;
}
.operator-chevron.svg-icon {
  flex: none;
  width: 18px;
  height: 18px;
  color: var(--md-sys-color-on-surface-variant);
}
.operator-empty-message {
  padding: 24px 8px;
  text-align: center;
}
@media (max-width: 420px) {
  .operator-mode {
    flex-wrap: wrap;
    padding: 12px;
  }
  .operator-mode-main {
    flex-basis: 120px;
    gap: 8px;
  }
  .operator-network {
    gap: 8px;
    padding: 12px;
  }
  .operator-status {
    font-size: 11px;
  }
}
</style>
