<script setup lang="ts">
import { guardDialogCancel } from '../utils/material'
import { notify } from '../stores/feedback'
import { computed, ref, watch } from 'vue'
import { devicesService } from '../services/devices'
import type { OperatorCandidate, OperatorSelection, OperatorSelectionRAT } from '../types/api'
import { useDevicesStore } from '../stores/devices'
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
    class="m3-dialog"
    :open="modelValue"
    @cancel="guardDialogCancel($event, loading)"
    @closed="emit('update:modelValue', false)"
  >
    <div slot="headline">运营商网络选择</div>
    <div slot="content" class="m3-stack">
      <div class="m3-inset">
        <div class="m3-row">
          <span>当前模式</span
          ><span class="m3-badge">{{
            currentSelection?.mode === 'automatic'
              ? '自动选网'
              : currentSelection
                ? '手动锁定'
                : '读取中'
          }}</span>
        </div>
        <p v-if="currentSelection?.mode === 'manual'" class="m3-muted">
          PLMN {{ currentSelection.plmn || '--' }}
        </p>
      </div>
      <div class="m3-actions">
        <md-outlined-button :disabled="scanning || loading" @click="doScan">{{
          scanning ? '扫描中…' : '扫描可用网络'
        }}</md-outlined-button
        ><md-text-button
          :disabled="loading || currentSelection?.mode === 'automatic'"
          @click="setModeAuto"
          >恢复自动选网</md-text-button
        >
      </div>
      <p
        v-if="scanning || scanMessage || scanError"
        :class="scanError && !scanRetryable ? 'm3-error' : 'm3-notice'"
        role="status"
      >
        {{ scanError || scanMessage || '正在扫描网络…' }}
      </p>
      <div class="operator-list m3-stack">
        <button
          v-for="candidate in candidates"
          :key="`${candidate.plmn}-${ratDisplay(candidate)}`"
          type="button"
          class="m3-option"
          :disabled="loading"
          @click="setModeManual(candidate)"
        >
          <md-ripple /><span
            ><strong>{{ candidate.operator_name || candidate.short_name || '未知网络' }}</strong
            ><span class="m3-muted block"
              >{{ candidate.plmn }} · {{ ratDisplay(candidate) }}</span
            ></span
          ><span class="m3-badge">{{
            candidate.status === 'current'
              ? '当前'
              : candidate.status === 'forbidden'
                ? '禁止'
                : '可选择'
          }}</span>
        </button>
        <p v-if="!candidates.length && !scanning" class="m3-muted">点击扫描查看可用网络</p>
      </div>
    </div>
    <div slot="actions">
      <md-text-button :disabled="loading" @click="emit('update:modelValue', false)"
        >关闭</md-text-button
      >
    </div>
  </md-dialog>
</template><style scoped>
.operator-list {
  max-height: 300px;
  overflow: auto;
}
</style>
