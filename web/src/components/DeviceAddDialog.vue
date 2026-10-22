<script setup lang="ts">
import { fieldValue, guardDialogCancel } from '../utils/material'
import { computed, watch } from 'vue'
import type { DeviceConfigDTO, DiscoveredDevice } from '../types/api'
import { isWwanQmiControlPath } from '../utils/deviceBackend'

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
    class="m3-dialog"
    :open="modelValue"
    @cancel="guardDialogCancel($event, addSaving)"
    @closed="closeDialog"
  >
    <div slot="headline">添加设备</div>
    <div slot="content" class="m3-stack">
      <p class="m3-muted">选择未配置设备，识别信息将自动填入。</p>
      <div class="discovered-list m3-stack">
        <p v-if="discovering" role="status">正在探测设备…</p>
        <template v-else
          ><button
            v-for="d in unconfiguredDiscovered"
            :key="discoveryIdentity(d)"
            type="button"
            class="m3-option"
            :disabled="!!d.degraded || addSaving"
            :aria-pressed="discoveryIdentity(addSelected) === discoveryIdentity(d)"
            @click="emit('select-device', d)"
          >
            <md-ripple /><span
              ><strong>{{ d.net_interface || '--' }} · {{ discoveryModeText(d) }}</strong
              ><span class="m3-muted block">{{ d.control_path }} · AT {{ d.at_port || '--' }}</span
              ><span class="m3-muted block"
                >IMEI {{ d.imei || '--' }} · {{ d.usb_path || '--' }}</span
              ><span v-if="d.degraded" class="m3-error block">无法读取 IMEI，暂不可添加</span></span
            >
          </button>
          <p v-if="!unconfiguredDiscovered.length" class="m3-muted">暂无可添加设备</p></template
        >
      </div>
      <div class="m3-form">
        <div class="m3-form-grid">
          <md-outlined-text-field
            label="ID"
            :value="addConfig.id"
            :disabled="addSaving"
            @input="addConfig.id = fieldValue($event)"
          /><md-outlined-text-field
            label="名称"
            :value="addConfig.name || ''"
            :disabled="addSaving"
            @input="addConfig.name = fieldValue($event)"
          />
          <md-outlined-text-field
            label="IMEI"
            :value="addConfig.modem_imei || ''"
            disabled
          /><md-outlined-text-field
            label="USB 路径"
            :value="addConfig.usb_path || ''"
            disabled
          /><md-outlined-text-field
            label="网卡接口"
            :value="addConfig.interface || ''"
            disabled
          /><md-outlined-text-field
            label="AT 端口"
            :value="addConfig.at_port || ''"
            disabled
          /><md-outlined-text-field
            label="控制设备"
            :value="addConfig.control_device || ''"
            disabled
          />
          <md-outlined-select
            label="设备后端"
            :value="addConfig.device_backend || 'at'"
            :disabled="isQMIBackendOnly || isMBIMBackendOnly || addSaving"
            @change="addConfig.device_backend = fieldValue($event) as DeviceConfigDTO['device_backend']"
          >
            <md-select-option
              v-if="!isMBIMBackendOnly"
              value="at"
              :selected="addConfig.device_backend === 'at'"
              :disabled="isQMIBackendOnly"
              ><div slot="headline">AT</div></md-select-option
            ><md-select-option
              v-if="!isMBIMBackendOnly"
              value="qmi"
              :selected="addConfig.device_backend === 'qmi'"
              :disabled="!addConfig.control_device"
              ><div slot="headline">QMI</div></md-select-option
            ><md-select-option v-if="isMBIMBackendOnly" value="mbim" selected
              ><div slot="headline">MBIM</div></md-select-option
            >
          </md-outlined-select>
        </div>
        <p v-if="isQMIBackendOnly || isMBIMBackendOnly" class="m3-notice">
          此设备的后端固定为 {{ isQMIBackendOnly ? 'QMI' : 'MBIM' }}，AT 口保留给终端。
        </p>
      </div>
    </div>
    <div slot="actions">
      <md-text-button :disabled="addSaving" @click="closeDialog">取消</md-text-button
      ><md-filled-button
        :disabled="addSaving || !addSelected || addSelected.degraded"
        @click="emit('save')"
        >{{ addSaving ? '保存中…' : '保存' }}</md-filled-button
      >
    </div>
  </md-dialog>
</template>
<style scoped>
.discovered-list {
  max-height: 260px;
  overflow: auto;
}
.m3-option span {
  overflow-wrap: anywhere;
  min-width: 0;
}
</style>
