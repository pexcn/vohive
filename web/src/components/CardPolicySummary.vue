<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
import type { CardPolicy } from '../types/api'
import { cardsService } from '../services/cards'
import { cardConnectionMode, connectionModeLabels, policyIPLabels } from '../utils/cardPolicy'
import { useSensitiveVisibility } from '../composables/useSensitiveVisibility'
import SensitiveText from './SensitiveText.vue'
import SvgIcon from './SvgIcon.vue'
import simIcon from '@material-symbols/svg-400/outlined/sim_card.svg?raw'
import settingsIcon from '@material-symbols/svg-400/outlined/tune.svg?raw'

const props = defineProps<{
  iccid?: string
  cardName?: string
  isVisible: boolean
  revision?: number
  busy?: boolean
}>()
const emit = defineEmits<{ edit: [] }>()
const id = useId()
const showSensitive = useSensitiveVisibility()
const policy = ref<CardPolicy | null>(null)
const loading = ref(false)
const loadError = ref('')
let generation = 0
const connectionLabel = computed(() => policy.value ? connectionModeLabels[cardConnectionMode(policy.value)] : '—')

async function load() {
  const request = ++generation
  policy.value = null
  loadError.value = ''
  loading.value = false
  if (!props.iccid || !props.isVisible) return
  loading.value = true
  const result = await cardsService.getPolicy(props.iccid)
  if (request !== generation) return
  loading.value = false
  if (result.ok) policy.value = result.data
  else loadError.value = result.error.message || 'SIM 卡设置加载失败'
}
watch(() => [props.iccid, props.isVisible, props.revision], load, { immediate: true })
onBeforeUnmount(() => { generation++ })
</script>

<template>
  <section class="card-policy-summary" :aria-labelledby="`${id}-title`">
    <header class="summary-heading"><h3 :id="`${id}-title`">当前 SIM 卡</h3><span class="m3-badge">跟随 SIM 卡保存</span></header>
    <p v-if="!iccid" class="m3-notice">尚未识别到 SIM 卡，识别后可设置连接方式。</p>
    <template v-else>
      <div class="summary-identity">
        <span class="summary-avatar" aria-hidden="true"><SvgIcon :svg="simIcon" /></span>
        <div class="summary-name"><h4>{{ cardName || 'SIM 卡' }}</h4><p>ICCID <SensitiveText :concealed="!showSensitive">{{ iccid }}</SensitiveText></p></div>
        <span class="m3-badge summary-current">当前使用</span>
      </div>
      <div v-if="loading" class="summary-loading" role="status"><md-circular-progress class="m3-progress" indeterminate />正在加载 SIM 卡设置…</div>
      <div v-else-if="loadError" class="m3-row" role="alert"><p class="m3-error">{{ loadError }}</p><md-text-button @click="load">重试</md-text-button></div>
      <dl v-else-if="policy" class="summary-values">
        <div><dt>连接方式</dt><dd>{{ connectionLabel }}</dd></div>
        <div><dt>APN</dt><dd>{{ policy.apn || '自动识别' }}</dd></div>
        <div><dt>IP 版本</dt><dd>{{ policyIPLabels[policy.ip_version] || 'IPv4' }}</dd></div>
      </dl>
      <footer class="summary-footer"><p class="m3-muted">切换卡后自动应用对应设置</p><md-filled-button class="summary-edit" :disabled="busy" aria-haspopup="dialog" @click="emit('edit')"><SvgIcon slot="icon" :svg="settingsIcon" />编辑 SIM 卡设置</md-filled-button></footer>
    </template>
  </section>
</template>

<style scoped>
.card-policy-summary {
  container-type: inline-size;
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
  padding: 24px;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 16px;
  background: var(--md-sys-color-surface-container-low);
}
.summary-heading, .summary-identity { display: flex; align-items: center; gap: 16px; }
.summary-heading { flex-wrap: wrap; gap: 12px; }
h3 { font-size: 20px; font-weight: 500; line-height: 28px; }
h4 { font-size: 16px; font-weight: 500; overflow-wrap: anywhere; }
.summary-avatar { display: grid; place-items: center; width: 44px; height: 44px; flex-shrink: 0; border-radius: 12px; background: var(--md-sys-color-secondary-container); color: var(--md-sys-color-on-secondary-container); }
.summary-name { flex: 1; min-width: 0; }
.summary-name p { margin-top: 4px; color: var(--md-sys-color-on-surface-variant); font: 13px/20px var(--vh-typeface-mono); overflow-wrap: anywhere; }
.summary-current { background: var(--vh-success-container); color: var(--vh-on-success-container); flex-shrink: 0; }
.summary-values { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; margin: 0; }
dt { margin-bottom: 6px; color: var(--md-sys-color-on-surface-variant); font-size: 13px; }
dd { margin: 0; overflow-wrap: anywhere; }
.summary-footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 16px; padding-top: 20px; border-top: 1px solid var(--md-sys-color-outline-variant); }
.summary-footer p { font-size: 13px; }
.summary-edit { margin-left: auto; }
.summary-loading { display: flex; align-items: center; gap: 12px; color: var(--md-sys-color-on-surface-variant); }
@container (max-width: 380px) {
  .summary-avatar { display: none; }
  .summary-identity { gap: 10px; }
}
@media (max-width: 640px) {
  .card-policy-summary { padding: 16px; }
}
</style>
