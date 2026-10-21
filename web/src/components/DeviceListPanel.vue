<script setup lang="ts">
import EmptyState from './EmptyState.vue'
import ListSkeleton from './ListSkeleton.vue'
import StatusLight from './StatusLight.vue'
import type { DeviceMgmtListItem } from '../types/api'
import { isControlOnline, isRadioRegistered, lifecycleStatusLabel, primaryLifecycleStatus } from '../utils/deviceLifecycle'

defineProps<{
  loading: boolean
  selectedId: string
  devices: DeviceMgmtListItem[]
  deviceCount: number
  deviceLimit: number
}>()

const emit = defineEmits<{
  'select-device': [id: string]
}>()

const primaryStatus = primaryLifecycleStatus

const registrationText = (d: DeviceMgmtListItem) => {
  const phaseText = lifecycleStatusLabel(d.lifecycle_phase)
  if (phaseText && d.lifecycle_phase !== 'online' && d.lifecycle_phase !== 'offline') return phaseText
  if (isRadioRegistered(d)) {
    return `${d?.modem?.operator || '--'} · ${[d?.modem?.network_duplex, d?.modem?.network_mode].filter(Boolean).join(' ') || '--'}`
  }
  if (!isControlOnline(d)) return '控制面恢复中'
  if (d.registration_state_label === 'searching') return '搜索网络中'
  if (d.registration_state_label === 'denied') return '驻网被拒'
  return '未驻网'
}

const dataNetworkText = (d: DeviceMgmtListItem) => {
  if (d?.vowifi_enabled) return ''
  if (!d?.network_enabled) return '数据未开启'
  if (!d?.network_connected) return '数据网络未连接'
  return ''
}

const secondaryStatus = (d: DeviceMgmtListItem) => {
  if (d?.vowifi_enabled) return 'WiFi-Calling'
  return [registrationText(d), dataNetworkText(d)].filter(Boolean).join(' · ')
}
</script>

<template>
  <section class="device-picker m3-card" aria-label="选择设备">
    <div class="m3-row">
      <h2 class="m3-title">设备</h2>
      <span v-if="deviceLimit > 0" class="m3-badge">{{ deviceCount }} / {{ deviceLimit }}</span>
    </div>
    <ListSkeleton v-if="loading && devices.length === 0" :rows="4" />
    <EmptyState
      v-else-if="devices.length === 0"
      title="暂无设备"
      subtitle="添加设备开始管理"
    />
    <div v-else class="device-list" role="group" aria-label="设备列表">
      <button
        v-for="d in devices"
        :key="d.id"
        type="button"
        class="device-option"
        :aria-pressed="selectedId === d.id"
        @click="emit('select-device', d.id)"
      >
        <md-ripple />
        <span class="device-option-heading">
          <strong>{{ d.name || d.id }}</strong>
          <span class="device-status">
            <StatusLight :tone="primaryStatus(d).tone" :animated="primaryStatus(d).animated" />
            {{ primaryStatus(d).label }}
          </span>
        </span>
        <span class="device-option-meta">{{ d.id }} · {{ d.interface || '--' }}</span>
        <span class="device-option-meta">{{ secondaryStatus(d) }}</span>
      </button>
    </div>
  </section>
</template>

<style scoped>
.device-picker {
  display: flex;
  flex-direction: column;
  gap: 24px;
  align-self: start;
}
.device-list {
  display: grid;
  gap: 8px;
  max-height: 60vh;
  overflow-y: auto;
}
.device-option {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  padding: 16px;
  border: 0;
  border-radius: 12px;
  overflow: hidden;
  background: transparent;
  color: var(--md-sys-color-on-surface);
  text-align: left;
}
.device-option[aria-pressed='true'] {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.device-option:focus-visible {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: -3px;
}
.device-option-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.device-option-heading strong {
  min-width: 0;
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.device-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
  font-size: 12px;
}
.device-option-meta {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
}
</style>
