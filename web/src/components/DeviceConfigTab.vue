<script setup lang="ts">
import { fieldValue } from '../utils/material'
import { computed, watch } from 'vue'
import type { DeviceConfigDTO, DeviceOverviewItem } from '../types/api'
import { isWwanQmiControlPath } from '../utils/deviceBackend'

const props = defineProps<{
  editConfig: DeviceConfigDTO | null
  deviceStatus?: DeviceOverviewItem | null
  saving: boolean
  deleting: boolean
}>()

const emit = defineEmits<{
  save: []
  delete: []
}>()

const activeControlDevice = computed(() => props.deviceStatus?.control_device || props.editConfig?.control_device)
const activeInterface = computed(() => props.deviceStatus?.interface || props.editConfig?.interface)
const activeATPort = computed(() => props.deviceStatus?.at_port || props.editConfig?.at_port)
const activeUsbPath = computed(() => props.deviceStatus?.usb_path || props.editConfig?.usb_path)

const isQMIBackendOnly = computed(() => isWwanQmiControlPath(activeControlDevice.value))
const isMBIMBackendOnly = computed(
  () => String(props.editConfig?.device_backend || '').toLowerCase() === 'mbim'
)

watch(
  isQMIBackendOnly,
  (locked) => {
    if (locked && props.editConfig) {
      props.editConfig.device_backend = 'qmi'
    }
  },
  { immediate: true }
)
</script>

<template>
  <section class="m3-stack">
    <div class="m3-row">
      <div>
        <h2 class="m3-title">设备配置</h2>
        <p class="m3-muted">部分变更需要重启后生效</p>
      </div>
      <div class="m3-actions">
        <md-text-button class="danger" :disabled="saving || deleting" @click="emit('delete')"
          >删除设备</md-text-button
        ><md-filled-button :disabled="saving || deleting" @click="emit('save')">{{
          saving ? '保存中…' : '保存配置'
        }}</md-filled-button>
      </div>
    </div>
    <div v-if="editConfig" class="m3-form">
      <div class="m3-form-grid">
        <md-outlined-text-field label="ID" :value="editConfig.id" disabled /><md-outlined-text-field
          label="名称"
          :value="editConfig.name"
          :disabled="saving || deleting"
          @input="editConfig.name = fieldValue($event)"
        />
        <md-outlined-text-field
          label="IMEI 绑定"
          :value="editConfig.modem_imei || ''"
          disabled
        /><md-outlined-text-field
          label="设备路径"
          :value="activeUsbPath || ''"
          disabled
        /><md-outlined-text-field
          label="网卡接口"
          :value="activeInterface || ''"
          disabled
        /><md-outlined-text-field
          label="AT 端口"
          :value="activeATPort || ''"
          disabled
        /><md-outlined-text-field label="控制设备" :value="activeControlDevice || ''" disabled />
        <md-outlined-select
          label="运行后端"
          :value="editConfig.device_backend || 'at'"
          :disabled="isQMIBackendOnly || isMBIMBackendOnly || saving || deleting"
          @change="editConfig.device_backend = fieldValue($event) as DeviceConfigDTO['device_backend']"
        >
          <md-select-option
            v-if="!isMBIMBackendOnly"
            value="at"
            :selected="editConfig.device_backend === 'at'"
            :disabled="isQMIBackendOnly"
            ><div slot="headline">AT</div></md-select-option
          >
          <md-select-option
            v-if="!isMBIMBackendOnly"
            value="qmi"
            :selected="editConfig.device_backend === 'qmi'"
            :disabled="!activeControlDevice && editConfig.device_backend !== 'qmi'"
            ><div slot="headline">QMI</div></md-select-option
          >
          <md-select-option v-if="isMBIMBackendOnly" value="mbim" selected
            ><div slot="headline">MBIM</div></md-select-option
          >
        </md-outlined-select>
      </div>
      <p class="m3-muted">
        {{
          isQMIBackendOnly
            ? '此类设备固定 QMI，AT 口仅用于终端'
            : isMBIMBackendOnly
              ? '此类设备固定 MBIM，AT 口仅用于终端'
              : 'AT 使用串口，QMI 使用控制设备'
        }}
      </p>
    </div>
  </section>
</template>
