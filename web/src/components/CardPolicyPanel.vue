<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { CardPolicy } from '../types/api'
import { devicesService } from '../services/devices'
import { useCardPolicyToggles, type PolicyMirror } from '../composables/useCardPolicyToggles'
import { syncMaterialSwitch as vSyncSwitch } from '../directives/materialSwitch'

const props = defineProps<{
  deviceId: string | undefined
  iccid: string | undefined
  policy: CardPolicy | null
  deviceOnline: boolean
}>()

const emit = defineEmits<{
  policyChanged: []
}>()

// ip/apn 仍由本组件独立持有（不进 composable）
const ipVersion = ref<'v4' | 'v6' | 'v4v6'>('v4')
const apn = ref('')

const canToggle = computed(() => props.deviceOnline && !!props.iccid)

// 上游 policy → 三开关镜像（喂给 composable）
const mirror = computed<PolicyMirror | null>(() =>
  props.policy
    ? {
        network_enabled: props.policy.network_enabled,
        vowifi_enabled: props.policy.vowifi_enabled,
        airplane_enabled: props.policy.airplane_enabled
      }
    : null
)

// 同步 ip/apn（这两项不参与 composable）
watch(
  () => props.policy,
  (p) => {
    if (!p) return
    ipVersion.value = p.ip_version || 'v4'
    apn.value = p.apn || ''
  },
  { immediate: true }
)

// live 执行器：调设备动作端点，即时生效。network 携带本组件的 ip/apn。
const {
  local,
  networkPending,
  networkFailed,
  vowifiPending,
  vowifiFailed,
  airplanePending,
  airplaneFailed,
  onNetworkToggle,
  onVoWiFiToggle,
  onAirplaneToggle
} = useCardPolicyToggles(mirror, {
  async applyNetwork(enabled) {
    if (!props.deviceId) return { ok: false }
    const r = enabled
      ? await devicesService.startNetwork(props.deviceId, { ip_version: ipVersion.value, apn: apn.value })
      : await devicesService.stopNetwork(props.deviceId)
    return { ok: r.ok }
  },
  async applyVoWiFi(enabled) {
    if (!props.deviceId) return { ok: false }
    const r = enabled
      ? await devicesService.enableVoWiFi(props.deviceId)
      : await devicesService.disableVoWiFi(props.deviceId)
    return { ok: r.ok }
  },
  async applyAirplane(enabled) {
    if (!props.deviceId) return { ok: false }
    const r = await devicesService.setFlightMode(props.deviceId, enabled)
    return { ok: r.ok }
  },
  onChanged() {
    emit('policyChanged')
  }
})

function fieldValue(event: Event) {
  return (event.target as HTMLInputElement).value
}

function switchValue(event: Event) {
  return (event.target as HTMLElement & { selected: boolean }).selected
}

function changePolicy(field: keyof PolicyMirror, event: Event) {
  const enabled = switchValue(event)
  local.value[field] = enabled
  const apply = {
    network_enabled: onNetworkToggle,
    vowifi_enabled: onVoWiFiToggle,
    airplane_enabled: onAirplaneToggle
  }[field]
  return apply(enabled)
}

const sourceLabel = computed(() => {
  if (!props.policy) return ''
  return props.policy.source === 'user' ? '手动设置' : '自动默认'
})
</script>

<template>
  <section class="policy-panel" aria-labelledby="policy-title">
    <header>
      <h2 id="policy-title" class="m3-title">卡策略</h2>
      <p class="m3-muted">策略跟随 SIM 卡保存，切卡后即时生效</p>
    </header>
    <p v-if="!iccid" class="m3-notice">尚未识别到 SIM 卡 ICCID，暂时无法设置策略。</p>
    <template v-else>
      <p v-if="!deviceOnline" class="m3-notice">设备离线，当前策略仅供查看。</p>
      <div class="policy-identity m3-inset">
        <div>
          <p class="m3-label">当前卡 ICCID</p>
          <p class="policy-iccid">{{ iccid }}</p>
        </div>
        <span v-if="sourceLabel" class="m3-badge">{{ sourceLabel }}</span>
      </div>
      <div class="policy-settings">
        <md-outlined-select
          label="IP 版本"
          supporting-text="下次开启网络时生效"
          :value="ipVersion"
          :disabled="!canToggle"
          @change="ipVersion = fieldValue($event) as typeof ipVersion"
        >
          <md-select-option value="v4" :selected="ipVersion === 'v4'">
            <div slot="headline">IPv4</div>
          </md-select-option>
          <md-select-option value="v6" :selected="ipVersion === 'v6'">
            <div slot="headline">IPv6</div>
          </md-select-option>
          <md-select-option value="v4v6" :selected="ipVersion === 'v4v6'">
            <div slot="headline">IPv4 + IPv6</div>
          </md-select-option>
        </md-outlined-select>
        <md-outlined-text-field
          label="APN（可选）"
          supporting-text="留空自动识别；下次开启网络时生效"
          :value="apn"
          :disabled="!canToggle"
          @input="apn = fieldValue($event)"
        />
      </div>
      <div class="policy-options">
        <div class="policy-option">
          <div>
            <h3 id="network-policy-label">开启网络</h3>
            <p id="network-policy-help">VoWiFi 或飞行模式开启时不可用</p>
            <span v-if="networkFailed" class="policy-failure" role="status">未生效，请重试</span>
          </div>
          <div class="policy-control">
            <md-circular-progress
              v-if="networkPending"
              class="m3-progress"
              indeterminate
              aria-label="正在设置网络"
            />
            <md-switch
              v-sync-switch
              aria-labelledby="network-policy-label"
              aria-describedby="network-policy-help"
              :selected="local.network_enabled"
              :disabled="
                !canToggle || local.vowifi_enabled || local.airplane_enabled || networkPending
              "
              @change="changePolicy('network_enabled', $event)"
            />
          </div>
        </div>
        <div class="policy-option">
          <div>
            <h3 id="vowifi-policy-label">VoWiFi</h3>
            <p id="vowifi-policy-help">启用后关闭射频，通过 Wi-Fi 连接；不支持国内运营商</p>
            <span v-if="vowifiFailed" class="policy-failure" role="status">未生效，请重试</span>
          </div>
          <div class="policy-control">
            <md-circular-progress
              v-if="vowifiPending"
              class="m3-progress"
              indeterminate
              aria-label="正在设置 VoWiFi"
            />
            <md-switch
              v-sync-switch
              aria-labelledby="vowifi-policy-label"
              aria-describedby="vowifi-policy-help"
              :selected="local.vowifi_enabled"
              :disabled="!canToggle || vowifiPending"
              @change="changePolicy('vowifi_enabled', $event)"
            />
          </div>
        </div>
        <div class="policy-option">
          <div>
            <h3 id="airplane-policy-label">飞行模式</h3>
            <p id="airplane-policy-help">关闭射频和数据网络；VoWiFi 开启时由其接管</p>
            <span v-if="airplaneFailed" class="policy-failure" role="status">未生效，请重试</span>
          </div>
          <div class="policy-control">
            <md-circular-progress
              v-if="airplanePending"
              class="m3-progress"
              indeterminate
              aria-label="正在设置飞行模式"
            />
            <md-switch
              v-sync-switch
              aria-labelledby="airplane-policy-label"
              aria-describedby="airplane-policy-help"
              :selected="local.airplane_enabled"
              :disabled="!canToggle || local.vowifi_enabled || airplanePending"
              @change="changePolicy('airplane_enabled', $event)"
            />
          </div>
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.policy-panel {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
header p {
  margin-top: 8px;
}
.policy-identity {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.policy-iccid {
  margin-top: 4px;
  font: 14px/20px var(--vh-typeface-mono);
  overflow-wrap: anywhere;
}
.policy-settings {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr));
  gap: 24px;
}
.policy-options {
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 12px;
  overflow: hidden;
}
.policy-option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 20px 16px;
}
.policy-option + .policy-option {
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
h3 {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
}
.policy-option p {
  margin-top: 4px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
  line-height: 20px;
}
.policy-control {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.policy-failure {
  display: block;
  margin-top: 4px;
  color: var(--md-sys-color-error);
  font-size: 12px;
}
</style>
