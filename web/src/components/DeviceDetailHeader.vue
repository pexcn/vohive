<script setup lang="ts">
import type { DeviceOverviewItem } from '../types/api'
import SvgIcon from './SvgIcon.vue'
import simIcon from '@material-symbols/svg-400/outlined/sim_card.svg?raw'
import refreshIcon from '@material-symbols/svg-400/outlined/refresh.svg?raw'
import rebootIcon from '@material-symbols/svg-400/outlined/restart_alt.svg?raw'
import smsIcon from '@material-symbols/svg-400/outlined/sms.svg?raw'

defineProps<{
  device: DeviceOverviewItem
  rotating: boolean
  rebooting: boolean
  reconnectingVoWiFi: boolean
}>()

const emit = defineEmits<{
  'copy-text': [value: string]
  'rotate-ip': []
  'reboot-modem': []
  'reconnect-vowifi': []
  'open-sms': []
}>()
</script>

<template>
  <section class="device-header m3-card" aria-label="当前设备">
    <div class="device-identity">
      <span class="device-avatar"><SvgIcon :svg="simIcon" /></span>
      <div>
        <h2 class="m3-title">{{ device.name || device.id }}</h2>
        <div class="device-identifiers">
          <button type="button" @click="emit('copy-text', device.id)">{{ device.id }}</button>
          <span>公网 IP</span>
          <button
            type="button"
            :disabled="!device.public_ip"
            @click="emit('copy-text', device.public_ip || '')"
          >
            {{ device.public_ip || '暂无' }}
          </button>
        </div>
      </div>
    </div>
    <div class="m3-actions">
      <md-outlined-button
        v-if="device.vowifi_enabled"
        type="button"
        :disabled="reconnectingVoWiFi"
        :aria-busy="reconnectingVoWiFi"
        @click="emit('reconnect-vowifi')"
      >
        <md-circular-progress
          v-if="reconnectingVoWiFi"
          slot="icon"
          class="m3-progress"
          indeterminate
        />
        <SvgIcon v-else slot="icon" :svg="refreshIcon" />
        重连 VoWiFi
      </md-outlined-button>
      <md-outlined-button
        v-else
        type="button"
        :disabled="rotating || !device.network_connected"
        :aria-busy="rotating"
        @click="emit('rotate-ip')"
      >
        <md-circular-progress v-if="rotating" slot="icon" class="m3-progress" indeterminate />
        <SvgIcon v-else slot="icon" :svg="refreshIcon" />
        切换 IP
      </md-outlined-button>
      <md-outlined-button
        type="button"
        :disabled="rebooting"
        :aria-busy="rebooting"
        @click="emit('reboot-modem')"
      >
        <md-circular-progress v-if="rebooting" slot="icon" class="m3-progress" indeterminate />
        <SvgIcon v-else slot="icon" :svg="rebootIcon" />
        重启模组
      </md-outlined-button>
      <md-filled-button type="button" @click="emit('open-sms')">
        <SvgIcon slot="icon" :svg="smsIcon" />
        短信
      </md-filled-button>
    </div>
  </section>
</template>

<style scoped>
.device-header {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
}
@container (min-width: 1060px) {
  .device-header {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
  }
}
.device-identity {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}
.device-identity > div {
  min-width: 0;
}
.device-avatar {
  flex: none;
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: 24px;
  background: var(--md-sys-color-primary-container);
  color: var(--md-sys-color-on-primary-container);
}
.device-identifiers {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 8px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
}
.device-identifiers button {
  font-family: var(--vh-typeface-mono);
  overflow-wrap: anywhere;
}
.device-identifiers button:enabled:hover {
  text-decoration: underline;
}
.device-identifiers button:focus-visible {
  outline: 2px solid var(--md-sys-color-primary);
}
</style>
