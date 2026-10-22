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
      <h2 class="m3-title">{{ device.name || device.id }}</h2>
    </div>
    <div class="m3-actions">
      <md-text-button
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
      </md-text-button>
      <md-text-button
        v-else
        type="button"
        :disabled="rotating || !device.network_connected"
        :aria-busy="rotating"
        @click="emit('rotate-ip')"
      >
        <md-circular-progress v-if="rotating" slot="icon" class="m3-progress" indeterminate />
        <SvgIcon v-else slot="icon" :svg="refreshIcon" />
        切换 IP
      </md-text-button>
      <md-text-button
        type="button"
        :disabled="rebooting"
        :aria-busy="rebooting"
        @click="emit('reboot-modem')"
      >
        <md-circular-progress v-if="rebooting" slot="icon" class="m3-progress" indeterminate />
        <SvgIcon v-else slot="icon" :svg="rebootIcon" />
        重启模组
      </md-text-button>
      <md-filled-tonal-button type="button" @click="emit('open-sms')">
        <SvgIcon slot="icon" :svg="smsIcon" />
        短信
      </md-filled-tonal-button>
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
.device-identity > .m3-title {
  min-width: 0;
  overflow-wrap: anywhere;
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
</style>
