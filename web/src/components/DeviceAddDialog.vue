<script setup lang="ts">
import { fieldValue, guardDialogCancel } from '../utils/material'
import { computed, watch } from 'vue'
import type { DeviceConfigDTO, DiscoveredDevice } from '../types/api'
import { isWwanQmiControlPath } from '../utils/deviceBackend'
import SvgIcon from './SvgIcon.vue'
import simIcon from '@material-symbols/svg-400/outlined/sim_card.svg?raw'
import usbIcon from '@material-symbols/svg-400/outlined/usb.svg?raw'
import checkIcon from '@material-symbols/svg-400/outlined/check_circle.svg?raw'
import circleIcon from '@material-symbols/svg-400/outlined/radio_button_unchecked.svg?raw'

const props = defineProps<{
  modelValue: boolean
  discovering: boolean
  unconfiguredDiscovered: DiscoveredDevice[]
  addSelected: DiscoveredDevice | null
  addConfig: DeviceConfigDTO
  addSaving: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'select-device': [device: DiscoveredDevice]
  save: []
}>()

function closeDialog() {
  emit('update:modelValue', false)
}

function discoveryIdentity(d: DiscoveredDevice | null | undefined): string {
  if (!d) return ''
  return String(d.discovery_key || `${d.usb_path || ''}|${d.at_port || ''}`)
}

function discoveryModeText(d: DiscoveredDevice | null | undefined): string {
  const mode = String(d?.mode || 'unknown').toLowerCase()
  if (mode === 'qmi') return 'QMI'
  if (mode === 'mbim') return 'MBIM'
  if (mode === 'ecm') return 'ECM'
  if (mode === 'rndis') return 'RNDIS'
  if (mode === 'ncm') return 'NCM'
  return 'UNKNOWN'
}

const isQMIBackendOnly = computed(() => isWwanQmiControlPath(props.addSelected?.control_path || props.addConfig?.control_device))
const isMBIMBackendOnly = computed(
  () => String(props.addSelected?.mode || '').toLowerCase() === 'mbim'
)

watch(
  isQMIBackendOnly,
  (locked) => {
    if (locked && props.addConfig) {
      props.addConfig.device_backend = 'qmi'
    }
  },
  { immediate: true }
)

watch(
  isMBIMBackendOnly,
  (locked) => {
    if (locked && props.addConfig) {
      props.addConfig.device_backend = 'mbim'
    }
  },
  { immediate: true }
)
</script>

<template>
  <md-dialog
    class="m3-dialog device-add-dialog"
    :class="{ 'device-add-compact': discovering || !unconfiguredDiscovered.length }"
    :open="modelValue"
    @cancel="guardDialogCancel($event, addSaving)"
    @closed="closeDialog"
  >
    <div slot="headline">添加设备</div>
    <div slot="content" class="device-add-content">
      <div v-if="discovering" class="device-add-state" role="status">
        <md-circular-progress indeterminate />
        <h3>正在探测设备…</h3>
        <p>请稍候，正在读取连接信息。</p>
      </div>
      <div v-else-if="!unconfiguredDiscovered.length" class="device-add-state">
        <span class="device-add-state-icon"><SvgIcon :svg="usbIcon" /></span>
        <h3>暂无可添加设备</h3>
        <p>已连接的设备可能已添加。<br>连接新设备后，请在设备页重新扫描。</p>
      </div>
      <template v-else>
        <p class="m3-muted">选择设备，确认信息后即可添加。</p>
        <div class="device-add-layout">
          <section class="device-add-picker" aria-label="可添加设备">
            <div class="device-add-heading">
              <h3>可添加设备</h3>
              <span>{{ unconfiguredDiscovered.length }} 台</span>
            </div>
            <div class="discovered-list" role="group" aria-label="未配置设备列表">
              <button
                v-for="d in unconfiguredDiscovered"
                :key="discoveryIdentity(d)"
                type="button"
                class="discovered-device"
                :disabled="!!d.degraded || addSaving"
                :aria-pressed="discoveryIdentity(addSelected) === discoveryIdentity(d)"
                @click="emit('select-device', d)"
              >
                <md-ripple />
                <span class="discovered-avatar"><SvgIcon :svg="simIcon" /></span>
                <span class="discovered-identity">
                  <strong>{{ d.net_interface || d.control_path || 'USB 设备' }} · {{ discoveryModeText(d) }}</strong>
                  <small>USB {{ d.usb_path || '--' }}</small>
                  <span v-if="d.degraded" class="m3-error">无法读取 IMEI，暂不可添加</span>
                </span>
                <SvgIcon
                  class="discovered-check"
                  :svg="discoveryIdentity(addSelected) === discoveryIdentity(d) ? checkIcon : circleIcon"
                />
              </button>
            </div>
          </section>
          <section v-if="addSelected" class="device-add-config m3-form" aria-label="设备配置">
            <h3>设备配置</h3>
            <md-outlined-text-field
              label="设备 ID"
              name="device-add-id"
              :value="addConfig.id"
              :disabled="addSaving"
              @input="addConfig.id = fieldValue($event)"
            />
            <md-outlined-text-field
              label="名称（可选）"
              name="device-add-name"
              :value="addConfig.name || ''"
              :disabled="addSaving"
              @input="addConfig.name = fieldValue($event)"
            />
            <md-outlined-select
              v-if="!isQMIBackendOnly && !isMBIMBackendOnly"
              label="设备后端"
              :value="addConfig.device_backend || 'at'"
              :disabled="addSaving"
              @change="addConfig.device_backend = fieldValue($event) as DeviceConfigDTO['device_backend']"
            >
              <md-select-option value="at" :selected="addConfig.device_backend === 'at'">
                <div slot="headline">AT</div>
              </md-select-option>
              <md-select-option
                value="qmi"
                :selected="addConfig.device_backend === 'qmi'"
                :disabled="!addConfig.control_device"
              >
                <div slot="headline">QMI</div>
              </md-select-option>
            </md-outlined-select>
            <div class="device-add-heading device-add-info-heading">
              <h3>自动识别信息</h3>
              <span v-if="isQMIBackendOnly || isMBIMBackendOnly" class="m3-badge">
                {{ isQMIBackendOnly ? 'QMI' : 'MBIM' }}
              </span>
            </div>
            <dl class="device-add-facts">
              <div><dt>IMEI</dt><dd>{{ addConfig.modem_imei || '--' }}</dd></div>
              <div><dt>USB 路径</dt><dd>{{ addConfig.usb_path || '--' }}</dd></div>
              <div><dt>网卡接口</dt><dd>{{ addConfig.interface || '--' }}</dd></div>
              <div><dt>AT 端口</dt><dd>{{ addConfig.at_port || '--' }}</dd></div>
              <div><dt>控制设备</dt><dd>{{ addConfig.control_device || '--' }}</dd></div>
            </dl>
            <p class="device-add-support">
              <template v-if="isQMIBackendOnly || isMBIMBackendOnly">
                此设备的后端固定为 {{ isQMIBackendOnly ? 'QMI' : 'MBIM' }}，AT 端口仍可用于终端。
              </template>
              <template v-else>连接信息由系统识别，无需手动填写。</template>
            </p>
          </section>
          <div v-else class="device-add-state device-add-placeholder">
            <span class="device-add-state-icon"><SvgIcon :svg="simIcon" /></span>
            <h3>选择要添加的设备</h3>
            <p>选择后将显示配置和连接信息。</p>
          </div>
        </div>
      </template>
    </div>
    <div slot="actions">
      <md-text-button :disabled="addSaving" @click="closeDialog">
        {{ !discovering && !unconfiguredDiscovered.length ? '关闭' : '取消' }}
      </md-text-button>
      <md-filled-button
        v-if="!discovering && unconfiguredDiscovered.length"
        :disabled="addSaving || !addSelected || addSelected.degraded"
        @click="emit('save')"
      >{{ addSaving ? '添加中…' : '添加设备' }}</md-filled-button>
    </div>
  </md-dialog>
</template>

<style scoped>
.device-add-dialog {
  width: min(800px, calc(100vw - 48px));
  max-height: calc(100dvh - 48px);
}
.device-add-dialog.device-add-compact {
  width: min(560px, calc(100vw - 48px));
}
.device-add-content {
  display: grid;
  gap: 24px;
}
.device-add-content h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
  color: var(--md-sys-color-on-surface);
}
.device-add-layout {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}
.device-add-picker {
  min-width: 0;
  padding-right: 24px;
  border-right: 1px solid var(--md-sys-color-outline-variant);
  align-self: stretch;
}
.device-add-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}
.device-add-heading > span:not(.m3-badge) {
  font-size: 12px;
  color: var(--md-sys-color-on-surface-variant);
}
.discovered-list {
  display: grid;
  gap: 8px;
}
.discovered-device {
  position: relative;
  display: flex;
  align-items: center;
  gap: 12px;
  width: 100%;
  min-width: 0;
  padding: 16px 12px;
  border: 0;
  border-radius: 16px;
  overflow: hidden;
  background: transparent;
  color: var(--md-sys-color-on-surface);
  text-align: left;
}
.discovered-device[aria-pressed='true'] {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.discovered-device:disabled {
  opacity: 0.38;
}
.discovered-device:focus-visible {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: -3px;
}
.discovered-avatar {
  display: grid;
  place-items: center;
  flex: none;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: var(--md-sys-color-surface-container-highest);
  color: var(--md-sys-color-on-surface-variant);
}
.discovered-device[aria-pressed='true'] .discovered-avatar {
  background: var(--md-sys-color-surface-container);
  color: var(--md-sys-color-primary);
}
.discovered-identity {
  display: grid;
  gap: 4px;
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
.discovered-identity strong {
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
}
.discovered-identity small {
  font-size: 12px;
  line-height: 16px;
  color: var(--md-sys-color-on-surface-variant);
}
.discovered-check.svg-icon {
  flex: none;
  width: 20px;
  height: 20px;
  color: var(--md-sys-color-on-surface-variant);
}
.discovered-device[aria-pressed='true'] .discovered-check {
  color: var(--md-sys-color-primary);
}
.device-add-config {
  display: grid;
  min-width: 0;
  gap: 16px;
}
.device-add-info-heading {
  margin: 8px 0 0;
}
.device-add-facts {
  display: grid;
  gap: 12px;
  margin: 0;
}
.device-add-facts > div {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 16px;
}
.device-add-facts dt {
  flex: none;
  color: var(--md-sys-color-on-surface-variant);
}
.device-add-facts dd {
  min-width: 0;
  margin: 0;
  font-family: var(--vh-typeface-mono);
  font-size: 13px;
  text-align: right;
  overflow-wrap: anywhere;
  color: var(--md-sys-color-on-surface);
}
.device-add-support {
  font-size: 12px;
  line-height: 20px;
  color: var(--md-sys-color-on-surface-variant);
}
.device-add-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 24px 8px;
  text-align: center;
  color: var(--md-sys-color-on-surface-variant);
}
.device-add-state-icon {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  margin-bottom: 16px;
  border-radius: 16px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.device-add-state h3 {
  font-size: 16px;
  line-height: 24px;
}
.device-add-state md-circular-progress {
  margin-bottom: 16px;
}
.device-add-placeholder {
  align-self: center;
}
@media (max-width: 719px) {
  .device-add-layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .device-add-picker {
    padding: 0 0 20px;
    border-right: 0;
    border-bottom: 1px solid var(--md-sys-color-outline-variant);
  }
}
</style>
