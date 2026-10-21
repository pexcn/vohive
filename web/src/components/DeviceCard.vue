<script setup lang="ts">
import { computed } from 'vue'
import type { DashboardDevice } from '../types/api'
import StatusLight from './StatusLight.vue'
import SvgIcon from './SvgIcon.vue'
import simIcon from '@material-symbols/svg-400/outlined/sim_card.svg?raw'
import wifiIcon from '@material-symbols/svg-400/outlined/wifi_calling.svg?raw'
import cellularIcon from '@material-symbols/svg-400/outlined/signal_cellular_alt.svg?raw'
import arrowIcon from '@material-symbols/svg-400/outlined/arrow_forward.svg?raw'

const props = defineProps<{ device: DashboardDevice }>()
const emit = defineEmits<{ 'open-device': [id: string] }>()
const networkMode = computed(() => [props.device.network_duplex, props.device.network_mode].filter(Boolean).join(' '))
const signal = computed(() => {
  const value = props.device.signal_dbm
  return typeof value === 'number' && Number.isFinite(value) && value !== 0 && value !== -999 ? `${value} dBm` : '暂无信号数据'
})
</script>

<template>
  <button
    type="button"
    class="device-card"
    :aria-label="`查看 ${device.name || device.id}`"
    @click="emit('open-device', device.id)"
  >
    <md-ripple />
    <div class="m3-row">
      <div class="device-identity">
        <span class="device-avatar"><SvgIcon :svg="simIcon" /></span>
        <h2>{{ device.name || device.id }}</h2>
      </div>
      <span class="device-health">
        <StatusLight :tone="device.healthy ? 'success' : 'danger'" :animated="false" />
        {{ device.healthy ? '在线' : '离线' }}
      </span>
    </div>
    <p class="device-connection">
      <SvgIcon :svg="device.vowifi_active ? wifiIcon : cellularIcon" />
      <span>{{ device.vowifi_active ? 'Wi-Fi Calling' : device.operator || '运营商待识别' }}</span>
    </p>
    <p class="m3-muted">
      {{
        device.vowifi_active
          ? '通过 Wi-Fi 网络连接'
          : [networkMode, signal].filter(Boolean).join(' · ')
      }}
    </p>
    <div class="device-ip">
      <span>
        <span class="m3-label">公网 IP</span>
        <strong>{{ device.public_ip || '暂无' }}</strong>
      </span>
      <SvgIcon :svg="arrowIcon" />
    </div>
  </button>
</template>

<style scoped>
.device-card {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 100%;
  padding: 16px;
  overflow: hidden;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 12px;
  background: var(--md-sys-color-surface);
  color: var(--md-sys-color-on-surface);
  text-align: left;
}
.device-card:focus-visible {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: 2px;
}
.device-identity {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.device-avatar {
  flex-shrink: 0;
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 20px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.device-health {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  line-height: 16px;
}
h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 400;
  line-height: 24px;
  overflow-wrap: anywhere;
}
.device-connection {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  line-height: 20px;
}
.device-connection span {
  min-width: 0;
  overflow-wrap: anywhere;
}
.device-ip {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  margin-top: auto;
  padding-top: 12px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
  color: var(--md-sys-color-primary);
}
.device-ip > span {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 8px;
}
.device-ip strong {
  font: 14px/20px var(--vh-typeface-mono);
  overflow-wrap: anywhere;
}
</style>
