<script setup lang="ts">
import { syncMaterialSwitch as vSyncSwitch } from '../directives/materialSwitch'
import { fieldSelected } from '../utils/material'
import { computed, onMounted, ref } from 'vue'
import type { CardPolicy } from '../types/api'
import { cardsService } from '../services/cards'
import { devicesService } from '../services/devices'
import { useCardPolicyToggles, type PolicyMirror } from '../composables/useCardPolicyToggles'

const props = defineProps<{
  deviceId: string
  iccid: string
  isActiveCard: boolean
  deviceOnline: boolean
}>()

const emit = defineEmits<{
  policyChanged: []
}>()

const policy = ref<CardPolicy | null>(null)
const loadFailed = ref(false)
const loading = ref(false)

// 激活卡 + 设备在线 → live 热切换；否则 stored 存储（激活/上线后生效）
const mode = computed<'live' | 'stored'>(() =>
  props.isActiveCard && props.deviceOnline ? 'live' : 'stored'
)

const hint = computed(() => {
  if (mode.value === 'live') return ''
  if (!props.deviceOnline) return '设备离线，改动已保存，激活/上线后生效'
  return '改动将在此卡激活后生效'
})

const mirror = computed<PolicyMirror | null>(() =>
  policy.value
    ? {
        network_enabled: policy.value.network_enabled,
        vowifi_enabled: policy.value.vowifi_enabled,
        airplane_enabled: policy.value.airplane_enabled
      }
    : null
)

async function loadPolicy() {
  loading.value = true
  loadFailed.value = false
  const r = await cardsService.getPolicy(props.iccid)
  loading.value = false
  if (r.ok) {
    policy.value = r.data
  } else {
    loadFailed.value = true
  }
}

onMounted(loadPolicy)

// stored 执行器：PUT 互斥后的完整三元组
async function putTriple(next: PolicyMirror): Promise<{ ok: boolean }> {
  const r = await cardsService.putPolicy(props.iccid, {
    network_enabled: next.network_enabled,
    vowifi_enabled: next.vowifi_enabled,
    airplane_enabled: next.airplane_enabled
  })
  return { ok: r.ok }
}

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
  async applyNetwork(enabled, next) {
    if (mode.value === 'stored') return putTriple(next)
    const r = enabled
      ? await devicesService.startNetwork(props.deviceId, {
          ip_version: policy.value?.ip_version || 'v4',
          apn: policy.value?.apn || ''
        })
      : await devicesService.stopNetwork(props.deviceId)
    return { ok: r.ok }
  },
  async applyVoWiFi(enabled, next) {
    if (mode.value === 'stored') return putTriple(next)
    const r = enabled
      ? await devicesService.enableVoWiFi(props.deviceId)
      : await devicesService.disableVoWiFi(props.deviceId)
    return { ok: r.ok }
  },
  async applyAirplane(enabled, next) {
    if (mode.value === 'stored') return putTriple(next)
    const r = await devicesService.setFlightMode(props.deviceId, enabled)
    return { ok: r.ok }
  },
  onChanged() {
    emit('policyChanged')
  }
})

function changePolicy(field: keyof PolicyMirror, event: Event) {
  const enabled = fieldSelected(event)
  local.value[field] = enabled
  if (field === 'network_enabled') void onNetworkToggle(enabled)
  else if (field === 'vowifi_enabled') void onVoWiFiToggle(enabled)
  else void onAirplaneToggle(enabled)
}
</script>

<template>
  <section class="m3-inset m3-stack" aria-label="此 Profile 的卡策略">
    <p v-if="loading" class="m3-muted" role="status">正在加载策略…</p>
    <div v-else-if="loadFailed" class="m3-row">
      <span class="m3-error">策略加载失败</span
      ><md-text-button @click="loadPolicy">重试</md-text-button>
    </div>
    <template v-else
      ><p v-if="hint" class="m3-notice">{{ hint }}</p>
      <label class="m3-switch-row"
        ><span>开启网络 <small v-if="networkFailed" class="m3-error">未生效，请重试</small></span
        ><md-switch
          v-sync-switch
          aria-label="此卡开启网络"
          :selected="local.network_enabled"
          :disabled="local.vowifi_enabled || local.airplane_enabled || networkPending"
          @change="changePolicy('network_enabled', $event)"
      /></label>
      <label class="m3-switch-row"
        ><span>VoWiFi <small v-if="vowifiFailed" class="m3-error">未生效，请重试</small></span
        ><md-switch
          v-sync-switch
          aria-label="此卡 VoWiFi"
          :selected="local.vowifi_enabled"
          :disabled="vowifiPending"
          @change="changePolicy('vowifi_enabled', $event)"
      /></label>
      <label class="m3-switch-row"
        ><span>飞行模式 <small v-if="airplaneFailed" class="m3-error">未生效，请重试</small></span
        ><md-switch
          v-sync-switch
          aria-label="此卡飞行模式"
          :selected="local.airplane_enabled"
          :disabled="local.vowifi_enabled || airplanePending"
          @change="changePolicy('airplane_enabled', $event)"
      /></label>
    </template>
  </section>
</template>
