<script setup lang="ts">
import { fieldValue } from '../utils/material'
import { computed, ref, useId, watch } from 'vue'
import CardPolicyPanel from './CardPolicyPanel.vue'
import SvgIcon from './SvgIcon.vue'
import deviceIcon from '@material-symbols/svg-400/outlined/memory.svg?raw'
import expandIcon from '@material-symbols/svg-400/outlined/keyboard_arrow_down.svg?raw'
import type { DeviceConfigDTO, DeviceOverviewItem } from '../types/api'
import { isWwanQmiControlPath } from '../utils/deviceBackend'

const props = defineProps<{
  editConfig: DeviceConfigDTO | null
  deviceStatus?: DeviceOverviewItem | null
  saving: boolean
  deleting: boolean
  cardName?: string
  isActive?: boolean
}>()

const emit = defineEmits<{
  save: []
  delete: []
  policyChanged: []
}>()

const id = useId()
const deviceExpanded = ref(false)
const policySaving = ref(false)
const busy = computed(() => props.saving || props.deleting || policySaving.value)
watch(
  () => props.deviceStatus?.id,
  () => {
    deviceExpanded.value = false
  }
)

const activeControlDevice = computed(
  () => props.deviceStatus?.control_device || props.editConfig?.control_device
)
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
  <section class="device-config m3-stack">
    <header class="m3-heading">
      <h2 class="m3-title">配置</h2>
      <p class="m3-muted">管理设备与当前 SIM 卡</p>
    </header>
    <CardPolicyPanel
      :device-id="deviceStatus?.id"
      :iccid="deviceStatus?.modem?.iccid"
      :card-name="cardName"
      :device-online="deviceStatus?.running === true"
      :esim-switching="deviceStatus?.esim_switching === true"
      :is-visible="isActive === true"
      @policy-changed="emit('policyChanged')"
      @saving-changed="policySaving = $event"
    />
    <section class="device-settings" :aria-labelledby="`${id}-device-title`">
      <header class="device-settings-heading">
        <h3 :id="`${id}-device-title`">设备设置</h3>
        <span class="m3-badge">跟随设备保存</span>
      </header>
      <button
        type="button"
        class="device-settings-toggle"
        :aria-expanded="deviceExpanded"
        :aria-controls="`${id}-device-fields`"
        @click="deviceExpanded = !deviceExpanded"
      >
        <SvgIcon :svg="deviceIcon" />
        <span><strong>模组与接口</strong><small>查看设备信息与接口配置</small></span>
        <SvgIcon
          class="device-settings-chevron"
          :class="{ expanded: deviceExpanded }"
          :svg="expandIcon"
        />
      </button>
      <Transition name="device-settings-reveal">
        <div v-if="deviceExpanded" :id="`${id}-device-fields`" class="device-settings-reveal">
          <div>
            <div class="device-settings-content">
              <p class="m3-muted">部分变更需要重启后生效</p>
              <div v-if="editConfig" class="m3-form">
                <div class="m3-form-grid">
                  <md-outlined-text-field
                    label="ID"
                    :value="editConfig.id"
                    disabled
                  /><md-outlined-text-field
                    label="名称"
                    :value="editConfig.name"
                    :disabled="busy"
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
                  /><md-outlined-text-field
                    label="控制设备"
                    :value="activeControlDevice || ''"
                    disabled
                  />
                  <md-outlined-select
                    label="运行后端"
                    :value="editConfig.device_backend || 'at'"
                    :disabled="isQMIBackendOnly || isMBIMBackendOnly || busy"
                    @change="
                      editConfig.device_backend = fieldValue(
                        $event
                      ) as DeviceConfigDTO['device_backend']
                    "
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
              <p v-if="!editConfig" class="m3-muted" role="status">正在加载设备配置…</p>
              <footer class="m3-actions device-settings-actions">
                <md-text-button
                  class="danger"
                  :disabled="busy || !editConfig"
                  @click="emit('delete')"
                  >删除设备</md-text-button
                >
                <md-filled-button :disabled="busy || !editConfig" @click="emit('save')">{{
                  saving ? '保存中…' : '保存设备设置'
                }}</md-filled-button>
              </footer>
            </div>
          </div>
        </div>
      </Transition>
    </section>
  </section>
</template>

<style scoped>
.device-config {
  gap: 24px;
}
.device-settings {
  min-width: 0;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 16px;
  padding: 24px;
}
.device-settings-heading {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}
.device-settings-heading h3 {
  font-size: 20px;
  font-weight: 500;
  line-height: 28px;
}
.device-settings-toggle {
  display: flex;
  align-items: center;
  text-align: left;
  gap: 16px;
  width: 100%;
  padding: 16px;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 12px;
  background: var(--md-sys-color-surface-container);
  color: inherit;
  cursor: pointer;
}
.device-settings-toggle:focus-visible {
  outline: 2px solid var(--md-sys-color-primary);
  outline-offset: 2px;
}
.device-settings-toggle > span:not(.svg-icon) {
  flex: 1;
}
.device-settings-toggle strong {
  font-size: 16px;
  font-weight: 500;
}
.device-settings-toggle small {
  display: block;
  margin-top: 4px;
  font-size: 14px;
  color: var(--md-sys-color-on-surface-variant);
}
.device-settings-chevron.expanded {
  transform: rotate(180deg);
}
.device-settings-reveal {
  display: grid;
  grid-template-rows: 1fr;
}
.device-settings-reveal > div {
  min-height: 0;
  overflow: hidden;
}
.device-settings-content {
  padding-top: 24px;
}
.device-settings-content > p {
  margin-bottom: 20px;
}
.device-settings-actions {
  justify-content: flex-end;
  margin-top: 24px;
}
@media (max-width: 640px) {
  .device-settings {
    padding: 16px;
  }
}
@media (prefers-reduced-motion: no-preference) {
  .device-settings-chevron {
    transition: transform 220ms;
  }
  .device-settings-reveal-enter-active,
  .device-settings-reveal-leave-active {
    transition:
      grid-template-rows 220ms cubic-bezier(0.2, 0, 0, 1),
      opacity 220ms;
  }
  .device-settings-reveal-enter-from,
  .device-settings-reveal-leave-to {
    grid-template-rows: 0fr;
    opacity: 0;
  }
}
.device-settings-reveal-leave-active {
  pointer-events: none;
}
</style>
