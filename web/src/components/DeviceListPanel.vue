<script setup lang="ts">
import { computed, ref } from 'vue'
import SvgIcon from './SvgIcon.vue'
import searchIcon from '@material-symbols/svg-400/outlined/search.svg?raw'
import EmptyState from './EmptyState.vue'
import ListSkeleton from './ListSkeleton.vue'
import StatusLight from './StatusLight.vue'
import type { DeviceMgmtListItem } from '../types/api'
import { isControlOnline, isRadioRegistered, lifecycleStatusLabel, primaryLifecycleStatus } from '../utils/deviceLifecycle'

const props = defineProps<{
  loading: boolean
  query: string
  statusFilter: 'all' | 'online' | 'offline'
  sortKey: 'name' | 'signal'
  sortDir: 'asc' | 'desc'
  selectedId: string
  filteredDevices: DeviceMgmtListItem[]
  deviceCount: number
  deviceLimit: number
}>()

const emit = defineEmits<{
  'update:query': [value: string]
  'update:statusFilter': [value: 'all' | 'online' | 'offline']
  'update:sortKey': [value: 'name' | 'signal']
  'update:sortDir': [value: 'asc' | 'desc']
  'select-device': [id: string]
}>()

const filtersOpen = ref(false)

const modelQuery = computed({
  get: () => props.query,
  set: (value: string) => emit('update:query', value)
})

const modelStatusFilter = computed({
  get: () => props.statusFilter,
  set: (value: 'all' | 'online' | 'offline') => emit('update:statusFilter', value)
})



const modelSortKey = computed({
  get: () => props.sortKey,
  set: (value: 'name' | 'signal') => emit('update:sortKey', value)
})

const modelSortDir = computed({
  get: () => props.sortDir,
  set: (value: 'asc' | 'desc') => emit('update:sortDir', value)
})

function fieldValue(event: Event) {
  return (event.target as HTMLInputElement).value
}

function changeSort(event: Event) {
  const [key, direction] = fieldValue(event).split(':')
  if ((key === 'name' || key === 'signal') && (direction === 'asc' || direction === 'desc')) {
    modelSortKey.value = key
    modelSortDir.value = direction
  }
}

const primaryStatus = primaryLifecycleStatus

const registrationText = (d: DeviceMgmtListItem) => {
  const phaseText = lifecycleStatusLabel(d.lifecycle_phase)
  if (phaseText && d.lifecycle_phase !== 'online' && d.lifecycle_phase !== 'offline') return phaseText
  if (isRadioRegistered(d)) {
    return `${d?.modem?.operator || '--'} · ${[d?.modem?.network_duplex, d?.modem?.network_mode].filter(Boolean).join(' ') || '--'}`
  }
  if (!isControlOnline(d)) return '控制面恢复中'
  if (d.registration_state_label === 'searching') return '搜索网络中'
  if (d.registration_state_label === 'denied') return '驻网被拒'
  return '未驻网'
}

const dataNetworkText = (d: DeviceMgmtListItem) => {
  if (d?.vowifi_enabled) return ''
  if (!d?.network_enabled) return '数据未开启'
  if (!d?.network_connected) return '数据网络未连接'
  return ''
}

const secondaryStatus = (d: DeviceMgmtListItem) => {
  if (d?.vowifi_enabled) return 'WiFi-Calling'
  return [registrationText(d), dataNetworkText(d)].filter(Boolean).join(' · ')
}
</script>

<template>
  <section class="device-picker m3-card" aria-label="选择设备">
    <div class="m3-row">
      <h2 class="m3-title">设备</h2>
      <span v-if="deviceLimit > 0" class="m3-badge">{{ deviceCount }} / {{ deviceLimit }}</span>
    </div>
    <md-outlined-text-field
      label="搜索设备"
      supporting-text="名称、ICCID、IMEI 或网卡"
      :value="modelQuery"
      @input="modelQuery = fieldValue($event)"
    >
      <SvgIcon :svg="searchIcon" slot="leading-icon" />
    </md-outlined-text-field>
    <md-text-button type="button" :aria-expanded="filtersOpen" aria-controls="device-filters" @click="filtersOpen = !filtersOpen">
      {{ statusFilter === 'all' ? '筛选与排序' : '已筛选 · 筛选与排序' }}
    </md-text-button>
    <div v-show="filtersOpen" id="device-filters" class="device-filters">
      <md-outlined-select
        label="状态"
        :value="modelStatusFilter"
        @change="modelStatusFilter = fieldValue($event) as typeof statusFilter"
      >
        <md-select-option value="all" :selected="modelStatusFilter === 'all'">
          <div slot="headline">全部状态</div>
        </md-select-option>
        <md-select-option value="online" :selected="modelStatusFilter === 'online'">
          <div slot="headline">仅在线</div>
        </md-select-option>
        <md-select-option value="offline" :selected="modelStatusFilter === 'offline'">
          <div slot="headline">仅离线</div>
        </md-select-option>
      </md-outlined-select>
      <md-outlined-select
        label="排序"
        :value="`${modelSortKey}:${modelSortDir}`"
        @change="changeSort"
      >
        <md-select-option
          v-for="option in [
            { value: 'name:asc', label: '名称升序' },
            { value: 'name:desc', label: '名称降序' },
            { value: 'signal:desc', label: '信号最强' },
            { value: 'signal:asc', label: '信号最弱' },
          ]"
          :key="option.value"
          :value="option.value"
          :selected="`${modelSortKey}:${modelSortDir}` === option.value"
        >
          <div slot="headline">{{ option.label }}</div>
        </md-select-option>
      </md-outlined-select>
    </div>
    <ListSkeleton v-if="loading && filteredDevices.length === 0" :rows="4" />
    <EmptyState
      v-else-if="filteredDevices.length === 0"
      :title="query || statusFilter !== 'all' ? '没有匹配的设备' : '暂无设备'"
      subtitle="调整筛选条件，或添加设备开始管理"
    />
    <div v-else class="device-list" role="group" aria-label="设备列表">
      <button
        v-for="d in filteredDevices"
        :key="d.id"
        type="button"
        class="device-option"
        :aria-pressed="selectedId === d.id"
        @click="emit('select-device', d.id)"
      >
        <md-ripple />
        <span class="device-option-heading">
          <strong>{{ d.name || d.id }}</strong>
          <span class="device-status">
            <StatusLight :tone="primaryStatus(d).tone" :animated="primaryStatus(d).animated" />
            {{ primaryStatus(d).label }}
          </span>
        </span>
        <span class="device-option-meta">{{ d.id }} · {{ d.interface || '--' }}</span>
        <span class="device-option-meta">{{ secondaryStatus(d) }}</span>
      </button>
    </div>
  </section>
</template>

<style scoped>
.device-picker {
  display: flex;
  flex-direction: column;
  gap: 24px;
  align-self: start;
}
.device-filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 200px), 1fr));
  gap: 12px;
}
.device-list {
  display: grid;
  gap: 8px;
  max-height: 60vh;
  overflow-y: auto;
}
.device-option {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  padding: 16px;
  border: 0;
  border-radius: 12px;
  overflow: hidden;
  background: transparent;
  color: var(--md-sys-color-on-surface);
  text-align: left;
}
.device-option[aria-pressed='true'] {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.device-option:focus-visible {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: -3px;
}
.device-option-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.device-option-heading strong {
  min-width: 0;
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.device-status {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
  font-size: 12px;
}
.device-option-meta {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
}
</style>
