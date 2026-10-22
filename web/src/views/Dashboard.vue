<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { storeToRefs } from 'pinia'
import { useRouter } from 'vue-router'
import DeviceCard from '../components/DeviceCard.vue'
import PageHeader from '../components/PageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import ErrorState from '../components/ErrorState.vue'
import RefreshButton from '../components/RefreshButton.vue'
import TrafficAnalysisPanel from '../components/TrafficAnalysisPanel.vue'
import { usePollingScheduler } from '../composables/usePollingScheduler'
import { useDashboardStore } from '../stores/dashboard'
import type { TrafficRange } from '../services/traffic'

const dashboard = useDashboardStore()
const router = useRouter()
const {
  devices,
  devicesLoading: loading,
  devicesLastOkAt,
  devicesError,
  analysis,
  analysisLoading,
  analysisLastOkAt,
  analysisError
} = storeToRefs(dashboard)

const lastUpdatedAt = ref<number | null>(null)

const analysisRange = ref<TrafficRange>('day')

const totalCount = computed(() => devices.value.length)
const onlineCount = computed(() => devices.value.filter(d => d?.healthy).length)
const offlineCount = computed(() => Math.max(0, totalCount.value - onlineCount.value))

async function fetchDevices() {
  await dashboard.fetchDevices()
  lastUpdatedAt.value = Date.now()
}

async function fetchTrafficAnalysis() {
  await dashboard.fetchAnalysis(analysisRange.value)
}

function handleAnalysisRangeChange(range: TrafficRange) {
  if (analysisRange.value === range) return
  analysisRange.value = range
  void fetchTrafficAnalysis()
}

function openDeviceOverview(id: string) {
  const deviceID = String(id || '').trim()
  if (!deviceID) return
  void router.push({
    name: 'Devices',
    query: {
      device: deviceID,
      tab: 'overview'
    }
  })
}

usePollingScheduler(fetchDevices, 5000, {
  immediate: true,
  maxIntervalMs: 30000,
  backgroundIntervalMs: 15000
})
usePollingScheduler(fetchTrafficAnalysis, 60000, {
  immediate: false,
  maxIntervalMs: 300000,
  backgroundIntervalMs: 120000
})

onMounted(() => {
  const win = window as Window & {
    requestIdleCallback?: (cb: IdleRequestCallback, opts?: IdleRequestOptions) => number
  }
  if (typeof win.requestIdleCallback === 'function') {
    win.requestIdleCallback(() => fetchTrafficAnalysis(), { timeout: 1500 })
  } else {
    setTimeout(fetchTrafficAnalysis, 800)
  }
})
</script>

<template>
  <div class="m3-page">
    <PageHeader title="设备监控" subtitle="查看所有设备的连接状态与流量">
      <template #actions><RefreshButton :loading="loading" @click="fetchDevices" /></template>
    </PageHeader>
    <section class="dashboard-summary" aria-label="设备概况">
      <div class="m3-card summary-total">
        <p class="m3-label">设备总数</p>
        <strong>{{ totalCount }}</strong>
      </div>
      <div class="m3-card">
        <p class="m3-label">在线设备</p>
        <strong class="summary-online">{{ onlineCount }}</strong>
      </div>
      <div class="m3-card">
        <p class="m3-label">离线设备</p>
        <strong>{{ offlineCount }}</strong>
      </div>
      <div class="m3-card">
        <p class="m3-label">最近刷新</p>
        <strong class="summary-time">
          {{ lastUpdatedAt ? new Date(lastUpdatedAt).toLocaleTimeString() : '--:--:--' }}
        </strong>
      </div>
    </section>
    <ErrorState
      v-if="devicesError"
      title="设备列表加载失败"
      :message="devicesError.message"
      :status-code="devicesError.status"
      :request-method="devicesError.method"
      :request-url="devicesError.url"
      :last-success-at="devicesLastOkAt"
      retry-text="重试"
      @retry="fetchDevices"
    />
    <ListSkeleton v-if="loading && devices.length === 0" :rows="4" />
    <EmptyState
      v-else-if="devices.length === 0"
      title="暂无设备接入"
      subtitle="添加设备后，可在这里查看连接状态和流量"
    >
      <template #actions>
        <md-filled-button type="button" @click="router.push('/devices')">管理设备</md-filled-button>
      </template>
    </EmptyState>
    <section v-else class="dashboard-devices" aria-label="设备列表">
      <DeviceCard
        v-for="dev in devices"
        :key="dev.id"
        :device="dev"
        @open-device="openDeviceOverview"
      />
    </section>
    <TrafficAnalysisPanel
      v-if="devices.length > 0 || !loading"
      :analysis="analysis"
      :loading="analysisLoading"
      :error="analysisError"
      :last-ok-at="analysisLastOkAt"
      :range="analysisRange"
      mode="global"
      @update:range="handleAnalysisRangeChange"
      @refresh="fetchTrafficAnalysis"
    />
  </div>
</template>

<style scoped>
.m3-page {
  container-type: inline-size;
}
.dashboard-summary,
.dashboard-devices {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 16px;
}
.dashboard-summary .m3-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.dashboard-summary strong {
  font-size: 36px;
  line-height: 44px;
  font-weight: 400;
}
.dashboard-summary .summary-time {
  font-size: 24px;
  line-height: 44px;
}
.summary-total {
  background: var(--md-sys-color-primary-container);
  color: var(--md-sys-color-on-primary-container);
}
.summary-total .m3-label {
  color: inherit;
}
.summary-online {
  color: var(--vh-success);
}
@container (max-width: 959px) {
  .dashboard-summary,
  .dashboard-devices {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@container (max-width: 599px) {
  .dashboard-devices {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
