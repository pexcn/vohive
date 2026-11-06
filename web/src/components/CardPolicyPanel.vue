<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
import type { CardPolicy } from '../types/api'
import { cardsService } from '../services/cards'
import { notify } from '../stores/feedback'
import { fieldValue } from '../utils/material'
import SvgIcon from './SvgIcon.vue'
import SensitiveText from './SensitiveText.vue'
import simIcon from '@material-symbols/svg-400/outlined/sim_card.svg?raw'
import settingsIcon from '@material-symbols/svg-400/outlined/tune.svg?raw'
import expandIcon from '@material-symbols/svg-400/outlined/keyboard_arrow_down.svg?raw'

type ConnectionMode = 'standby' | 'network' | 'vowifi' | 'airplane'
type PolicyDraft = Pick<
  CardPolicy,
  'network_enabled' | 'vowifi_enabled' | 'airplane_enabled' | 'ip_version' | 'apn'
>
const props = withDefaults(
  defineProps<{
    deviceId?: string
    iccid?: string
    cardName?: string
    deviceOnline: boolean
    isActiveCard?: boolean
    isVisible?: boolean
    esimSwitching?: boolean
    concealIccid?: boolean
    embedded?: boolean
  }>(),
  { isActiveCard: true, isVisible: true }
)
const emit = defineEmits<{ policyChanged: []; savingChanged: [saving: boolean] }>()
const id = useId()
const modes: { value: ConnectionMode; label: string; description: string }[] = [
  { value: 'standby', label: '仅驻网', description: '收短信，保持蜂窝注册' },
  { value: 'network', label: '蜂窝数据', description: '开启移动数据连接' },
  { value: 'vowifi', label: 'VoWiFi', description: '通过 Wi-Fi 连接' },
  { value: 'airplane', label: '飞行模式', description: '关闭蜂窝射频与数据' }
]
const policy = ref<CardPolicy | null>(null)
const draft = ref<PolicyDraft>({
  network_enabled: false,
  vowifi_enabled: false,
  airplane_enabled: false,
  ip_version: 'v4',
  apn: ''
})
const baseline = ref('')
const loading = ref(false)
const saving = ref(false)
const loadError = ref('')
const saveError = ref('')
const dataExpanded = ref(true)
let generation = 0
const dirty = computed(() => !!policy.value && JSON.stringify(draft.value) !== baseline.value)
const locked = computed(() => !policy.value || loading.value || saving.value || props.esimSwitching)
const mode = computed<ConnectionMode>(() =>
  draft.value.vowifi_enabled
    ? 'vowifi'
    : draft.value.airplane_enabled
      ? 'airplane'
      : draft.value.network_enabled
        ? 'network'
        : 'standby'
)
const modeHelp = computed(
  () =>
    ({
      standby: '保持蜂窝注册，不开启移动数据连接。',
      network: '使用此卡的 APN 和 IP 版本建立移动数据连接。',
      vowifi: 'VoWiFi 模式会关闭蜂窝射频与数据网络，通过 Wi-Fi 连接。',
      airplane: '关闭蜂窝射频、移动数据和 VoWiFi。'
    })[mode.value]
)

function readDraft(p: CardPolicy): PolicyDraft {
  return {
    network_enabled: p.network_enabled,
    vowifi_enabled: p.vowifi_enabled,
    airplane_enabled: p.airplane_enabled,
    ip_version: p.ip_version || 'v4',
    apn: p.apn || ''
  }
}
function resetDraft() {
  if (!policy.value) return
  draft.value = readDraft(policy.value)
  baseline.value = JSON.stringify(draft.value)
}
function selectMode(value: ConnectionMode) {
  if (locked.value || mode.value === value) return
  // Only explicit mode changes replace the saved triple. Editing APN/IP alone
  // preserves existing fallback preferences for leaving VoWiFi.
  Object.assign(draft.value, {
    network_enabled: value === 'network',
    vowifi_enabled: value === 'vowifi',
    airplane_enabled: value === 'airplane'
  })
}
async function loadPolicy() {
  const request = ++generation
  policy.value = null
  baseline.value = ''
  loadError.value = ''
  saveError.value = ''
  saving.value = false
  emit('savingChanged', false)
  if (!props.iccid) {
    loading.value = false
    return
  }
  loading.value = true
  const result = await cardsService.getPolicy(props.iccid)
  if (request !== generation) return
  loading.value = false
  if (!result.ok) {
    loadError.value = result.error.message || 'SIM 卡设置加载失败'
    return
  }
  policy.value = result.data
  resetDraft()
}
async function save() {
  if (locked.value || !props.iccid || (!dirty.value && !saveError.value)) return
  const request = generation
  const iccid = props.iccid
  saving.value = true
  saveError.value = ''
  emit('savingChanged', true)
  const result = await cardsService.putPolicy(iccid, {
    ...draft.value,
    apn: draft.value.apn.trim(),
    ...(props.isActiveCard && props.deviceOnline && props.deviceId
      ? { apply_device_id: props.deviceId }
      : {})
  })
  if (request !== generation) return
  saving.value = false
  emit('savingChanged', false)
  if (!result.ok) {
    saveError.value = result.error.message || 'SIM 卡设置保存失败'
    return
  }
  policy.value = result.data
  resetDraft()
  saveError.value = result.data.apply_error || ''
  emit('policyChanged')
  if (!saveError.value)
    notify({
      message:
        props.isActiveCard && props.deviceOnline
          ? 'SIM 卡设置已保存，正在应用'
          : 'SIM 卡设置已保存，将在激活或上线后应用',
      tone: 'success'
    })
}
watch(
  () => [props.deviceId, props.iccid],
  () => {
    void loadPolicy()
  },
  { immediate: true }
)
watch(
  () => props.isVisible,
  (visible) => {
    if (visible && !dirty.value && !saving.value) void loadPolicy()
  }
)
onBeforeUnmount(() => {
  generation++
  emit('savingChanged', false)
})
</script>

<template>
  <section class="policy-panel" :class="{ embedded }" :aria-labelledby="`${id}-title`">
    <header class="policy-heading" :class="{ 'sr-only': embedded }">
      <div class="policy-heading-row">
        <h3 :id="`${id}-title`">{{ isActiveCard ? '当前 SIM 卡设置' : 'SIM 卡设置' }}</h3>
        <span class="m3-badge">跟随 SIM 卡保存</span>
      </div>
      <p class="m3-muted">切换卡后自动应用对应设置</p>
    </header>
    <p v-if="!iccid" class="m3-notice">尚未识别到 SIM 卡，识别后可设置连接方式。</p>
    <div v-else-if="loading" class="policy-loading" role="status">
      <md-circular-progress class="m3-progress" indeterminate />正在加载 SIM 卡设置…
    </div>
    <div v-else-if="loadError" class="m3-row" role="alert">
      <p class="m3-error">{{ loadError }}</p>
      <md-text-button @click="loadPolicy">重试</md-text-button>
    </div>
    <template v-else-if="policy">
      <div class="policy-identity">
        <span class="policy-avatar" aria-hidden="true"><SvgIcon :svg="simIcon" /></span>
        <div class="policy-identity-text">
          <h4>{{ cardName || 'SIM 卡' }}</h4>
          <p class="policy-iccid">
            ICCID <SensitiveText :concealed="concealIccid">{{ iccid }}</SensitiveText>
          </p>
        </div>
        <span class="m3-badge" :class="{ 'policy-current': isActiveCard }">{{
          isActiveCard ? '当前使用' : '未启用'
        }}</span>
      </div>
      <p v-if="!deviceOnline" class="m3-notice">设备离线，保存后将在此卡激活、设备上线时应用。</p>
      <p v-else-if="!isActiveCard" class="m3-notice">设置将在此卡启用后应用。</p>
      <p v-if="esimSwitching" class="m3-notice" role="status">设备正在切卡，请在切换完成后保存。</p>
      <fieldset class="policy-mode-group" :disabled="locked">
        <legend>连接方式</legend>
        <div class="policy-modes">
          <label
            v-for="item in modes"
            :key="item.value"
            class="policy-mode"
            :class="{ selected: mode === item.value }"
          >
            <input
              type="radio"
              :name="`${id}-connection-mode`"
              :value="item.value"
              :checked="mode === item.value"
              @change="selectMode(item.value)"
            />
            <span
              ><strong>{{ item.label }}</strong
              ><small>{{ item.description }}</small></span
            >
          </label>
        </div>
      </fieldset>
      <p class="policy-mode-help">{{ modeHelp }}</p>
      <div class="policy-data">
        <button
          class="policy-data-toggle"
          type="button"
          :aria-expanded="dataExpanded"
          :aria-controls="`${id}-data`"
          @click="dataExpanded = !dataExpanded"
        >
          <SvgIcon :svg="settingsIcon" />
          <span><strong>蜂窝数据设置</strong><small>下次开启蜂窝数据时生效</small></span>
          <SvgIcon class="policy-chevron" :class="{ expanded: dataExpanded }" :svg="expandIcon" />
        </button>
        <Transition name="policy-reveal">
          <div v-if="dataExpanded" :id="`${id}-data`" class="policy-reveal">
            <div>
              <div class="policy-data-fields">
                <md-outlined-select
                  name="card-ip-version"
                  label="IP 版本"
                  :value="draft.ip_version"
                  :disabled="locked"
                  @change="draft.ip_version = fieldValue($event) as PolicyDraft['ip_version']"
                >
                  <md-select-option value="v4" :selected="draft.ip_version === 'v4'"
                    ><div slot="headline">IPv4</div></md-select-option
                  >
                  <md-select-option value="v6" :selected="draft.ip_version === 'v6'"
                    ><div slot="headline">IPv6</div></md-select-option
                  >
                  <md-select-option value="v4v6" :selected="draft.ip_version === 'v4v6'"
                    ><div slot="headline">IPv4 + IPv6</div></md-select-option
                  >
                </md-outlined-select>
                <md-outlined-text-field
                  name="card-apn"
                  label="APN（可选）"
                  placeholder="自动识别"
                  supporting-text="留空自动识别"
                  :value="draft.apn"
                  :disabled="locked"
                  @input="draft.apn = fieldValue($event)"
                />
              </div>
            </div>
          </div>
        </Transition>
      </div>
      <p v-if="saveError" class="m3-error policy-save-error" role="alert">{{ saveError }}</p>
      <footer class="policy-footer">
        <p class="m3-muted">修改仅应用于此 SIM 卡{{ dirty ? ' · 有未保存的更改' : '' }}</p>
        <div class="m3-actions">
          <md-text-button v-if="dirty" :disabled="locked" @click="resetDraft">重置</md-text-button>
          <md-filled-button
            class="policy-save"
            :disabled="locked || (!dirty && !saveError)"
            @click="save"
            >{{
              saving ? '保存并应用中…' : saveError ? '重新保存并应用' : '保存 SIM 卡设置'
            }}</md-filled-button
          >
        </div>
      </footer>
    </template>
  </section>
</template>

<style scoped>
.policy-panel {
  container-type: inline-size;
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-width: 0;
  padding: 24px;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 16px;
}
.policy-panel.embedded {
  border: 0;
  padding: 0;
}
.policy-heading-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
h3 {
  font-size: 20px;
  font-weight: 500;
  line-height: 28px;
}
.policy-heading > p {
  margin-top: 4px;
  font-size: 14px;
}
.policy-identity {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 16px;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container);
}
.policy-avatar {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  border-radius: 12px;
  flex-shrink: 0;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.policy-identity-text {
  flex: 1;
  min-width: 0;
}
h4 {
  font-size: 16px;
  font-weight: 500;
  overflow-wrap: anywhere;
}
.policy-iccid {
  color: var(--md-sys-color-on-surface-variant);
  font: 13px/20px var(--vh-typeface-mono);
  overflow-wrap: anywhere;
  margin-top: 4px;
}
.policy-current {
  background: var(--vh-success-container);
  color: var(--vh-on-success-container);
}
.policy-mode-group {
  border: 0;
  padding: 0;
  margin: 0;
  min-width: 0;
}
legend {
  margin-bottom: 12px;
  font-weight: 500;
  font-size: 16px;
}
.policy-modes {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.policy-mode {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 16px;
  border: 1px solid var(--md-sys-color-outline);
  border-radius: 10px;
  cursor: pointer;
}
.policy-mode.selected {
  border-color: var(--md-sys-color-primary);
  box-shadow: inset 0 0 0 1px var(--md-sys-color-primary);
  background: color-mix(in srgb, var(--md-sys-color-primary) 12%, transparent);
}
.policy-mode:hover {
  background-color: color-mix(in srgb, var(--md-sys-color-on-surface) 8%, transparent);
}
.policy-mode:focus-within {
  outline: 2px solid var(--md-sys-color-primary);
  outline-offset: 3px;
}
.policy-mode input {
  width: 20px;
  height: 20px;
  margin: 1px 0 0;
  flex-shrink: 0;
  accent-color: var(--md-sys-color-primary);
}
.policy-mode strong {
  font-weight: 500;
  line-height: 24px;
}
.policy-mode small,
.policy-data-toggle small {
  display: block;
  margin-top: 4px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 13px;
  line-height: 20px;
}
.policy-mode-group:disabled .policy-mode {
  opacity: 0.55;
  cursor: default;
}
.policy-mode-help {
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
  line-height: 20px;
  margin-top: -8px;
}
.policy-data {
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 12px;
}
.policy-data-toggle {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 16px;
  padding: 16px 20px;
  text-align: left;
  border: 0;
  color: inherit;
  background: transparent;
  cursor: pointer;
  border-radius: 12px;
}
.policy-data-toggle:focus-visible {
  outline: 2px solid var(--md-sys-color-primary);
  outline-offset: 2px;
}
.policy-data-toggle > span:not(.svg-icon) {
  flex: 1;
}
.policy-data-toggle strong {
  font-weight: 500;
  font-size: 16px;
}
.policy-data-toggle > .svg-icon:first-child {
  color: var(--md-sys-color-primary);
}
.policy-chevron.expanded {
  transform: rotate(180deg);
}
.policy-reveal {
  display: grid;
  grid-template-rows: 1fr;
}
.policy-reveal > div {
  min-height: 0;
  overflow: hidden;
}
.policy-data-fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  align-items: start;
  gap: 20px;
  margin: 0 20px;
  padding: 20px 0;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.policy-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}
.policy-footer > p {
  font-size: 13px;
}
.policy-loading {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--md-sys-color-on-surface-variant);
}
.policy-save-error {
  font-size: 14px;
  white-space: pre-wrap;
}
@container (max-width: 680px) {
  .policy-modes {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@container (max-width: 380px) {
  .policy-data-fields {
    grid-template-columns: minmax(0, 1fr);
  }
  .policy-identity {
    gap: 10px;
    padding: 12px;
  }
  .policy-avatar {
    display: none;
  }
  .policy-mode {
    padding: 12px;
    gap: 8px;
  }
}
@media (max-width: 640px) {
  .policy-panel {
    padding: 16px;
  }
}
@media (prefers-reduced-motion: no-preference) {
  .policy-mode {
    transition:
      background-color 150ms,
      border-color 150ms;
  }
  .policy-chevron {
    transition: transform 220ms;
  }
  .policy-reveal-enter-active,
  .policy-reveal-leave-active {
    transition:
      grid-template-rows 220ms cubic-bezier(0.2, 0, 0, 1),
      opacity 220ms;
  }
  .policy-reveal-enter-from,
  .policy-reveal-leave-to {
    grid-template-rows: 0fr;
    opacity: 0;
  }
}
.policy-reveal-leave-active {
  pointer-events: none;
}
</style>
