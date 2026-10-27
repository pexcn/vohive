<script setup lang="ts">
import { ref, computed } from 'vue'
import SvgIcon from './SvgIcon.vue'
import FieldRow from './FieldRow.vue'
import CountryFlag from './CountryFlag.vue'
import visibleIcon from '@material-symbols/svg-400/outlined/visibility.svg?raw'
import hiddenIcon from '@material-symbols/svg-400/outlined/visibility_off.svg?raw'
import settingsIcon from '@material-symbols/svg-400/outlined/tune.svg?raw'
import roamingIconSvg from '@material-symbols/svg-400/rounded/r_mobiledata.svg?raw'
import type { DeviceOverviewItem } from '../types/api'
import { useSensitiveVisibility } from '../composables/useSensitiveVisibility'
import { activeEsimProfileDisplayName } from './deviceOverviewActiveEsim'
import { isControlOnline, isRadioRegistered, isRecoveryPhase, lifecycleStatusLabel } from '../utils/deviceLifecycle'
import StatusLight from './StatusLight.vue'
import OperatorSelectionDialog from './OperatorSelectionDialog.vue'
import type { StatusLightTone } from './statusLight'
import { getVoWiFiReadiness, getVoWiFiStatus } from '../utils/vowifiStatus'

const props = defineProps<{
  device: DeviceOverviewItem | null
  simOperatorDisplay: string
  simOperatorCountry?: string
  trafficSpeedRx: string
  trafficSpeedTx: string
  trafficMinuteRx: string
  trafficMinuteTx: string
  e911Starting: boolean
}>()

const emit = defineEmits<{
  'setup-e911': []
  'refresh': []
}>()

const showSensitive = useSensitiveVisibility()
const showOperatorSelection = ref(false)
// The Material R sits in a signal canvas corner; crop its padding for inline use.
const roamingIcon = roamingIconSvg.replace('viewBox="0 -960 960 960"', 'viewBox="40 -920 360 400"')

const trafficStateLabel = computed(() => {
  const status = props.device?.traffic_meta?.status
  if (status === 'waiting_sample') return '等待采样'
  if (status === 'stale') return '采样中断'
  return ''
})

function trafficDisplay(value: string | undefined) {
  return trafficStateLabel.value || value
}

const trafficRxDisplay = computed(() => props.trafficMinuteRx || trafficDisplay(props.device?.traffic?.rx))
const trafficTxDisplay = computed(() => props.trafficMinuteTx || trafficDisplay(props.device?.traffic?.tx))
const trafficDownloadRateDisplay = computed(() => props.trafficSpeedRx || trafficDisplay(props.device?.traffic?.rate) || '--')
const trafficUploadRateDisplay = computed(() => props.trafficSpeedTx || trafficStateLabel.value || '--')

// ---- VoWiFi 模式计算属性 ----

const readinessItems = computed(() => getVoWiFiReadiness(props.device?.vowifi_runtime))

// 'ok' | 'partial' | 'off'
const vowifiStatus = computed(() => getVoWiFiStatus(props.device?.vowifi_runtime))

// 未就绪的环节名称列表
const notReadyNames = computed(() =>
  readinessItems.value.filter(i => !i.ready).map(i => i.key)
)

// ---- 窝蜂模式计算属性 ----

function hasValidSignalDbm(dbm: number | null | undefined): dbm is number {
  return typeof dbm === 'number' && Number.isFinite(dbm) && dbm !== 0 && dbm !== -999
}

// 0-5 格
const signalLevel = computed<number>(() => {
  const dbm = props.device?.modem?.signal_dbm
  if (!hasValidSignalDbm(dbm)) return 0
  if (dbm >= -75)  return 5
  if (dbm >= -85)  return 4
  if (dbm >= -95)  return 3
  if (dbm >= -105) return 2
  return 1
})

const signalColor = computed<'green' | 'amber' | 'red' | 'gray'>(() => {
  const dbm = props.device?.modem?.signal_dbm
  if (!hasValidSignalDbm(dbm)) return 'gray'
  if (dbm >= -85)  return 'green'
  if (dbm >= -100) return 'amber'
  return 'red'
})

const flightModeEnabled = computed(() => {
  if (props.device?.vowifi_active) return true
  const mode = props.device?.modem?.operating_mode
  return mode === 0 || mode === 4
})

const vowifiStatusTone = computed<StatusLightTone>(() => {
  if (vowifiStatus.value === 'partial') return 'warning'
  return vowifiStatus.value === 'ok' ? 'success' : 'danger'
})

const flightModeStatusText = computed(() => {
  return flightModeEnabled.value ? '是' : '否'
})

const activeEsimProfileName = computed(() => activeEsimProfileDisplayName(props.device))

const controlOnline = computed(() => isControlOnline(props.device))

const isRegistered = computed(() => isRadioRegistered(props.device))

const isRoaming = computed(() => props.device?.modem?.reg_status === 5)

const registrationStatusText = computed(() => {
  switch (props.device?.modem?.reg_status) {
    case 0: return '未注册'
    case 1:
    case 5: return '已注册'
    case 2: return '搜索中'
    case 3: return '注册被拒'
    case 4: return '未知'
    default: return props.device?.modem?.reg_status_text || '--'
  }
})

const cellularStatusTone = computed<StatusLightTone>(() => {
  if (isRecoveryPhase(props.device?.lifecycle_phase)) return 'warning'
  if (!controlOnline.value) return 'danger'
  return isRegistered.value ? 'success' : 'warning'
})

const cellularStatusText = computed(() => {
  const phaseText = lifecycleStatusLabel(props.device?.lifecycle_phase)
  if (phaseText && props.device?.lifecycle_phase !== 'online' && props.device?.lifecycle_phase !== 'offline') return phaseText
  if (!controlOnline.value) return props.device?.running ? '控制面恢复中' : '离线'
  if (isRegistered.value) return ''
  if (props.device?.registration_state_label === 'searching') return '搜索网络中'
  if (props.device?.registration_state_label === 'denied') return '驻网被拒'
  return '未驻网'
})

const networkPanelMessage = computed(() => {
  if (!props.device?.network_enabled) return '数据网络未开启'
  if (!props.device?.network_connected) return '数据网络未连接'
  return ''
})

</script>

<template>
  <div class="overview-panels">
    <section class="runtime-panel m3-inset" aria-label="运行状态">
      <div class="m3-row">
        <h2 class="m3-title">运行状态</h2>
        <md-icon-button
          v-if="!device?.vowifi_enabled"
          aria-label="网络选择设置"
          @click="showOperatorSelection = true"
        >
          <SvgIcon :svg="settingsIcon" />
        </md-icon-button>
      </div>
      <template v-if="device?.vowifi_enabled">
        <div class="runtime-status" :data-tone="vowifiStatusTone">
          <div class="runtime-status-content">
            <div class="runtime-status-label">
              <StatusLight :tone="vowifiStatusTone" :animated="vowifiStatus !== 'off'" />
              <span v-if="vowifiStatus === 'ok'">Wi-Fi Calling · 全部就绪</span>
              <span v-else-if="vowifiStatus === 'partial'">{{ notReadyNames.join(' · ') }} 未就绪</span>
              <span v-else>VoWiFi 未连接</span>
            </div>
            <p
              v-if="vowifiStatus === 'partial' && device.vowifi_runtime?.last_reason"
              class="runtime-reason"
            >
              {{ device.vowifi_runtime.last_reason }}
            </p>
          </div>
        </div>
        <ol class="readiness-steps" aria-label="VoWiFi 连接进度">
          <li
            v-for="item in readinessItems"
            :key="item.key"
            :data-state="
              item.ready === true ? 'ready' : item.ready === false ? 'pending' : 'unknown'
            "
            :aria-label="`${item.key}：${item.ready === true ? '已就绪' : '未就绪'}`"
          >
            <span class="readiness-track" />
            <span>{{ item.key }}</span>
          </li>
        </ol>
        <div id="vowifi-diagnostics" class="overview-fields">
          <FieldRow
            label="数据平面"
            :value="device.vowifi_runtime?.dataplane_mode || '--'"
            monospace
          />
          <FieldRow label="最后原因" :value="device.vowifi_runtime?.last_reason || '--'" />
          <FieldRow
            v-if="device.vowifi_runtime?.last_error_class"
            label="错误分类"
            :value="device.vowifi_runtime.last_error_class"
            monospace
            copyable
          />
        </div>
      </template>
      <template v-else>
        <div class="runtime-status" :data-tone="cellularStatusTone">
          <div class="runtime-status-label">
            <StatusLight :tone="cellularStatusTone" :animated="isRegistered" />
            <span>{{ isRegistered ? device?.modem?.operator || '--' : cellularStatusText }}</span>
          </div>
        </div>
        <div class="signal-reading" :data-signal="signalColor">
          <strong>
            {{ hasValidSignalDbm(device?.modem?.signal_dbm) ? device?.modem?.signal_dbm : '--' }}
          </strong>
          <span>dBm</span>
          <div class="signal-bars" aria-hidden="true">
            <span
              v-for="i in 5"
              :key="i"
              :class="{ active: i <= signalLevel }"
              :style="{ height: `${i * 20}%` }"
            />
          </div>
        </div>
        <dl class="signal-detail">
          <div>
            <dt>RSRP</dt>
            <dd>{{ device?.modem?.signal_rsrp ?? '--' }}</dd>
          </div>
          <div>
            <dt>RSRQ</dt>
            <dd>{{ device?.modem?.signal_rsrq ?? '--' }}</dd>
          </div>
          <div>
            <dt>SINR</dt>
            <dd>{{ device?.modem?.signal_sinr ?? '--' }}</dd>
          </div>
          <div v-if="device?.modem?.nr5g_signal_sinr !== undefined" class="signal-detail-extra">
            <dt>NR5G SINR</dt>
            <dd>{{ device.modem.nr5g_signal_sinr }}</dd>
          </div>
        </dl>
        <div class="overview-fields">
          <FieldRow
            label="网络模式"
            :value="
              [device?.modem?.network_duplex, device?.modem?.network_mode]
                .filter(Boolean)
                .join(' ') || '--'
            "
            monospace
          />
          <FieldRow label="频段" :value="device?.modem?.radio_band || '--'" monospace />
          <FieldRow
            label="信道"
            :value="device?.modem?.radio_channel ? String(device.modem.radio_channel) : '--'"
            monospace
          />
          <FieldRow label="注册状态" :value="registrationStatusText" monospace>
            <span class="registration-status">
              {{ registrationStatusText }}
              <span v-if="isRoaming" class="roaming-indicator" role="img" aria-label="漫游" title="漫游">
                <SvgIcon :svg="roamingIcon" />
              </span>
            </span>
          </FieldRow>
        </div>
      </template>
    </section>
    <section class="m3-inset">
      <div class="m3-row">
        <h2 class="m3-title">设备信息</h2>
        <md-icon-button
          :aria-label="showSensitive ? '隐藏敏感信息' : '显示敏感信息'"
          :aria-pressed="showSensitive"
          @click="showSensitive = !showSensitive"
        >
          <SvgIcon :svg="showSensitive ? visibleIcon : hiddenIcon" />
        </md-icon-button>
      </div>
      <div class="overview-fields">
        <FieldRow
          label="IMEI"
          :value="device?.modem?.imei"
          :sensitive="!showSensitive"
          monospace
          copyable
        />
        <FieldRow
          label="ICCID"
          :value="device?.modem?.iccid"
          :sensitive="!showSensitive"
          monospace
          copyable
        />
        <FieldRow
          label="IMSI"
          :value="device?.modem?.imsi"
          :sensitive="!showSensitive"
          monospace
          copyable
        />
        <FieldRow
          label="本机号码"
          :value="device?.local_phone || '--'"
          :sensitive="!showSensitive"
          monospace
          copyable
        />
        <div v-if="device?.e911_setup_available" class="m3-row">
          <span class="m3-muted">E911 地址</span>
          <md-text-button type="button" :disabled="e911Starting" @click="emit('setup-e911')">
            <md-circular-progress
              v-if="e911Starting"
              slot="icon"
              class="m3-progress"
              indeterminate
            />
            设置
          </md-text-button>
        </div>
        <FieldRow
          v-if="activeEsimProfileName"
          label="当前 eSIM"
          :value="activeEsimProfileName"
          monospace
        />
        <FieldRow label="原运营商" :value="simOperatorDisplay" copyable>
          <CountryFlag :iso="simOperatorCountry" />{{ simOperatorDisplay }}
        </FieldRow>
        <FieldRow label="固件版本" :value="device?.modem?.firmware" monospace copyable />
        <FieldRow label="飞行模式" :value="flightModeStatusText" />
        <FieldRow
          label="运行模式"
          :value="
            device?.backend_mode === 'qmi'
              ? 'QMI'
              : device?.backend_mode === 'mbim'
                ? 'MBIM'
                : device?.backend_mode === 'at'
                  ? 'AT'
                  : 'Auto'
          "
          monospace
        />
      </div>
    </section>
    <section class="network-panel m3-inset">
      <h2 class="m3-title">数据网络</h2>
      <p v-if="networkPanelMessage" class="network-empty m3-muted">{{ networkPanelMessage }}</p>
      <div v-else class="overview-fields">
        <FieldRow label="内网 IPv4" :value="device?.private_ip" monospace copyable />
        <FieldRow label="内网 IPv6" :value="device?.private_ipv6" monospace copyable />
        <FieldRow label="外网 IPv4" :value="device?.public_ip" monospace copyable />
        <FieldRow label="外网 IPv6" :value="device?.public_ipv6" monospace copyable />
        <FieldRow label="近 1 分钟上传" :value="trafficTxDisplay" monospace />
        <FieldRow label="近 1 分钟下载" :value="trafficRxDisplay" monospace />
        <FieldRow label="实时下载速率" :value="trafficDownloadRateDisplay" monospace />
        <FieldRow label="实时上传速率" :value="trafficUploadRateDisplay" monospace />
      </div>
    </section>
    <OperatorSelectionDialog
      v-if="device?.id"
      v-model="showOperatorSelection"
      :device-id="device.id"
      @updated="emit('refresh')"
    />
  </div>
</template>

<style scoped>
.overview-panels {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
}
.overview-panels > section {
  min-width: 0;
}
.overview-panels > section > .m3-row {
  min-height: 40px;
}
.network-panel > .m3-title {
  display: flex;
  align-items: center;
  min-height: 40px;
}
.overview-panels .m3-title {
  font-size: 18px;
  font-weight: 500;
  line-height: 24px;
}
@container device-panel (min-width: 600px) {
  .overview-panels {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .network-panel {
    grid-column: 1 / -1;
  }
}
@container device-panel (min-width: 900px) {
  .overview-panels {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
  .network-panel {
    grid-column: auto;
  }
}
.runtime-status {
  display: flex;
  align-items: center;
  justify-content: center;
  text-align: center;
  margin-top: 12px;
  padding: 10px 12px;
  border-radius: 12px;
  font-size: 14px;
  font-weight: 500;
  line-height: 20px;
  overflow-wrap: anywhere;
}
.runtime-status[data-tone='success'] {
  color: var(--vh-on-success-container);
  background: var(--vh-success-container);
}
.runtime-status[data-tone='warning'] {
  color: var(--vh-on-warning-container);
  background: var(--vh-warning-container);
}
.runtime-status[data-tone='danger'] {
  color: var(--md-sys-color-on-error-container);
  background: var(--md-sys-color-error-container);
}
.runtime-status-content {
  min-width: 0;
}
.runtime-status-label {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-width: 0;
}
.runtime-status-label > span {
  min-width: 0;
}
.runtime-reason {
  margin-top: 4px;
  font-size: 12px;
  font-weight: 400;
  line-height: 16px;
  overflow-wrap: anywhere;
}
.readiness-steps {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 8px;
  margin: 12px 0 4px;
  padding: 0;
  list-style: none;
}
.readiness-steps li {
  display: flex;
  flex-direction: column;
  gap: 8px;
  text-align: center;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
}
.readiness-track {
  height: 5px;
  border-radius: 3px;
  background: var(--md-sys-color-outline-variant);
}
[data-state='ready'] .readiness-track {
  background: var(--vh-success);
}
[data-state='pending'] .readiness-track {
  background: var(--vh-warning);
}
.overview-fields {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 12px;
  font-size: 14px;
  line-height: 20px;
}
.signal-reading {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin-top: 12px;
  color: var(--md-sys-color-on-surface-variant);
}
.signal-reading strong {
  font-size: 30px;
  font-weight: 400;
  line-height: 36px;
}
.signal-reading[data-signal='green'] {
  color: var(--vh-success);
}
.signal-reading[data-signal='amber'] {
  color: var(--vh-warning);
}
.signal-reading[data-signal='red'] {
  color: var(--md-sys-color-error);
}
.signal-bars {
  display: flex;
  align-items: flex-end;
  gap: 3px;
  height: 24px;
  margin-left: auto;
  flex-shrink: 0;
}
.signal-bars span {
  width: 5px;
  border-radius: 2px;
  background: var(--md-sys-color-outline-variant);
}
.signal-bars span.active {
  background: currentColor;
}
.signal-detail {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: 8px;
  text-align: center;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 14px;
  line-height: 20px;
}
.signal-detail > div {
  position: relative;
  display: flex;
  justify-content: center;
  align-items: baseline;
  gap: 4px;
}
.signal-detail > div:first-child {
  justify-content: flex-start;
}
.signal-detail > div:nth-child(3) {
  justify-content: flex-end;
}
.signal-detail > div:nth-child(2)::before,
.signal-detail > div:nth-child(3)::before {
  content: '·';
  position: absolute;
  left: -4px;
  transform: translateX(-50%);
}
.signal-detail dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
}
.signal-detail-extra {
  grid-column: 1 / -1;
}
.registration-status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  vertical-align: bottom;
}
.roaming-indicator {
  display: inline-flex;
}
.registration-status .roaming-indicator > .svg-icon {
  width: 16px;
  height: 16px;
}
.network-empty {
  margin-top: 12px;
  font-size: 14px;
  line-height: 20px;
}
</style>
