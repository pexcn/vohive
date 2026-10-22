<script setup lang="ts">
import { computed, inject, ref, shallowRef, watchEffect, type Ref } from 'vue'
import ErrorState from './ErrorState.vue'
import RefreshButton from './RefreshButton.vue'
import SvgIcon from './SvgIcon.vue'
import checkIcon from '@material-symbols/svg-400/outlined/check.svg?raw'
import type { AppError } from '../types/domain'
import type { TrafficAnalysis, TrafficRange } from '../services/traffic'

type TrafficAnalysisMode = 'global' | 'device'
type TooltipParam = {
  axisValue?: string | number
  dataIndex?: number
  seriesName?: string
  value?: string | number
  color?: string
}

const props = withDefaults(defineProps<{
  analysis: TrafficAnalysis
  loading?: boolean
  error?: AppError | null
  lastOkAt?: number | null
  range: TrafficRange
  mode: TrafficAnalysisMode
  title?: string
  subtitle?: string
  disabled?: boolean
  deviceLabel?: string
}>(), {
  title: '流量分析',
  subtitle: '数据每分钟采样一次，按日/周/月聚合',
  disabled: false
})

const emit = defineEmits<{
  (e: 'update:range', value: TrafficRange): void
  (e: 'refresh'): void
}>()

const VChartComp = shallowRef<unknown>(null)
const chartLoadError = ref<string | null>(null)
const chartLoading = ref(false)
let chartInitPromise: Promise<void> | null = null

function formatChartLoadError(err: unknown) {
  if (err instanceof Error) return `${err.name}: ${err.message}`
  if (typeof err === 'string') return err
  try {
    return JSON.stringify(err) || '图表模块加载失败'
  } catch {
    return '图表模块加载失败'
  }
}

// Vite dev 模式下依赖预打包重新优化会让旧的 ?v=hash 立即失效；若图表的懒加载 import()
// 恰好与那次重新优化撞车，会报这类错误。这是瞬时竞态，重试一次通常就能拿到新哈希成功。
// 生产构建（无依赖再优化）不会触发，因此这里不需要也不应该全局处理。
const CHUNK_LOAD_ERROR_PATTERN =
  /Loading chunk|ChunkLoadError|dynamically imported module|Importing a module script failed|Failed to fetch dynamically imported module/i

function isChunkLoadLikeError(err: unknown) {
  const msg = err instanceof Error ? err.message : typeof err === 'string' ? err : ''
  return CHUNK_LOAD_ERROR_PATTERN.test(msg)
}

async function ensureChartLoaded() {
  if (VChartComp.value) return

  const maxAttempts = 2
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    if (!chartInitPromise) {
      chartLoading.value = true
      chartLoadError.value = null
      chartInitPromise = (async () => {
        const { default: VChart } = await import('./trafficChart')
        VChartComp.value = VChart
      })()
    }

    try {
      await chartInitPromise
      chartLoading.value = false
      return
    } catch (err) {
      chartInitPromise = null
      if (attempt < maxAttempts && isChunkLoadLikeError(err)) {
        await new Promise((resolve) => setTimeout(resolve, 400))
        continue
      }
      chartLoadError.value = formatChartLoadError(err)
      chartLoading.value = false
      return
    }
  }
}

const analysisBuckets = computed(() => props.analysis.buckets || [])
const recentBuckets = computed(() => [...analysisBuckets.value].reverse())
const analysisChartData = computed(() => props.analysis.chart)

function formatBytes(bytes: unknown) {
  const v = Number(bytes) || 0
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let val = v
  let i = 0
  while (val >= 1024 && i < units.length - 1) {
    val /= 1024
    i++
  }
  return `${val.toFixed(i === 0 ? 0 : 2)} ${units[i]}`
}

function parsePeriodStart(value: unknown): Date | null {
  if (typeof value !== 'string' || !value.trim()) return null
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date : null
}

function pad2(value: number) {
  return String(value).padStart(2, '0')
}

function formatLocalDate(date: Date) {
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

function formatLocalHour(date: Date) {
  return `${pad2(date.getHours())}:00`
}

function formatLocalDateHour(date: Date) {
  return `${formatLocalDate(date)} ${formatLocalHour(date)}`
}

function formatTrafficAxisTime(periodStart: unknown, fallback: string) {
  const date = parsePeriodStart(periodStart)
  if (!date) return fallback
  if (props.range === 'day') return formatLocalHour(date)
  return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
}

function formatTrafficTooltipTime(periodStart: unknown, fallback: string | number | undefined) {
  const date = parsePeriodStart(periodStart)
  if (!date) return fallback ?? ''
  if (props.range === 'day') return formatLocalDateHour(date)
  return formatLocalDate(date)
}

function formatTrafficBucketTime(bucket: { period_start?: string; bucket?: string }) {
  const date = parsePeriodStart(bucket?.period_start)
  if (!date) return bucket?.bucket || ''
  if (props.range === 'day') return formatLocalDateHour(date)
  return formatLocalDate(date)
}

const analysisTotal = computed(() => {
  const rx = analysisBuckets.value.reduce((sum, bucket) => sum + (Number(bucket.rx_bytes) || 0), 0)
  const tx = analysisBuckets.value.reduce((sum, bucket) => sum + (Number(bucket.tx_bytes) || 0), 0)
  return { rx, tx, total: rx + tx }
})

const rangeText = computed(() => {
  const map: Record<TrafficRange, string> = {
    day: '本日',
    week: '本周',
    month: '本月'
  }
  return map[props.range] || '本周期'
})

function pickUnit(maxBytes: number) {
  const gb = 1024 * 1024 * 1024
  const mb = 1024 * 1024
  const kb = 1024
  if (maxBytes >= gb) return { label: 'GB', divisor: gb, decimals: 2 }
  if (maxBytes >= mb) return { label: 'MB', divisor: mb, decimals: 2 }
  if (maxBytes >= kb) return { label: 'KB', divisor: kb, decimals: 2 }
  return { label: 'B', divisor: 1, decimals: 0 }
}

const chartSeriesSnapshot = computed(() => {
  const chart = analysisChartData.value
  if (!chart) return null

  const timestamps = Array.isArray(chart.timestamps) ? chart.timestamps : []
  const periodStarts = Array.isArray(chart.period_starts) ? chart.period_starts : []
  const displayTimestamps = timestamps.map((label, idx) =>
    formatTrafficAxisTime(periodStarts[idx], String(label || ''))
  )

  const totalBytesByTs = timestamps.map((_, idx) =>
    chart.devices.reduce((sum, dev) => sum + Number(chart.series[dev]?.[idx] || 0), 0)
  )

  return {
    timestamps: displayTimestamps,
    periodStarts,
    devices: chart.devices,
    series: chart.series,
    totalBytesByTs
  }
})

const hasChartData = computed(() => {
  const snapshot = chartSeriesSnapshot.value
  if (!snapshot) return false
  if (snapshot.timestamps.length === 0) return false
  if (props.mode === 'device') {
    return snapshot.totalBytesByTs.some(v => v > 0)
  }
  return snapshot.devices.length > 0 && snapshot.totalBytesByTs.some(v => v > 0)
})

const deviceSeriesName = computed(() => {
  const label = String(props.deviceLabel || '').trim()
  if (label) return label
  return analysisChartData.value?.devices?.[0] || '当前设备'
})

const isDark = inject<Ref<boolean>>('isDark', ref(false))
const chartColors = computed(() => {
  // Re-read the CSS roles after the app switches its theme class.
  void isDark.value
  const style = getComputedStyle(document.documentElement)
  const color = (role: string) => style.getPropertyValue(`--md-sys-color-${role}`).trim()
  return {
    primary: color('primary'),
    onPrimary: color('on-primary'),
    tertiary: color('tertiary'),
    secondary: color('secondary'),
    text: color('on-surface'),
    muted: color('on-surface-variant'),
    outline: color('outline-variant'),
    surface: color('surface-container-high')
  }
})

const chartOption = computed(() => {
  const snapshot = chartSeriesSnapshot.value
  if (!snapshot || !hasChartData.value) return null

  const { timestamps, devices, series, totalBytesByTs } = snapshot
  const maxBytes = Math.max(0, ...totalBytesByTs)
  const unit = pickUnit(maxBytes)
  const colors = chartColors.value
  const chartStyle = {
    color: [colors.primary, colors.tertiary, colors.secondary],
    textStyle: { color: colors.muted, fontFamily: 'Roboto, system-ui, sans-serif' }
  }

  if (props.mode === 'device') {
    return {
      ...chartStyle,
      tooltip: {
        backgroundColor: colors.surface,
        borderColor: colors.outline,
        textStyle: { color: colors.text },
        trigger: 'axis',
        axisPointer: { type: 'cross', label: { backgroundColor: colors.primary, color: colors.onPrimary } },
        formatter: (params: unknown) => {
          const list: TooltipParam[] = Array.isArray(params)
            ? params.filter((item): item is TooltipParam => !!item && typeof item === 'object')
            : []
          const axisLabel = list[0]?.axisValue ?? ''
          const dataIndex = Number(list[0]?.dataIndex ?? -1)
          const timeLabel = formatTrafficTooltipTime(snapshot.periodStarts[dataIndex], axisLabel)
          const point = list[0]
          const value = Number(point?.value) || 0
          return `<div class="font-bold mb-1">${timeLabel}</div>
            <div class="flex justify-between gap-4 text-xs">
              <span>${deviceSeriesName.value}</span>
              <span class="font-mono">${value.toFixed(unit.decimals)} ${unit.label}</span>
            </div>`
        }
      },
      grid: {
        left: '3%',
        right: '4%',
        bottom: 28,
        top: 24,
        containLabel: true
      },
      xAxis: [
        {
          type: 'category',
          axisLabel: { color: colors.muted },
          boundaryGap: false,
          data: timestamps,
          axisLine: { lineStyle: { color: colors.outline } }
        }
      ],
      yAxis: [
        {
          type: 'value',
          axisLabel: { color: colors.muted },
          nameTextStyle: { color: colors.muted },
          name: `流量 (${unit.label})`,
          splitLine: { lineStyle: { color: colors.outline, type: 'dashed', opacity: 0.3 } },
          axisLine: { lineStyle: { color: colors.outline } }
        }
      ],
      dataZoom: [
        {
          type: 'inside',
          filterMode: 'none'
        }
      ],
      series: [
        {
          name: deviceSeriesName.value,
          type: 'line',
          symbol: 'none',
          smooth: true,
          areaStyle: { opacity: 0.18 },
          lineStyle: { width: 2.5, opacity: 0.95 },
          emphasis: { focus: 'series' },
          data: totalBytesByTs.map(v => v / unit.divisor)
        }
      ],
      backgroundColor: 'transparent'
    }
  }

  const stackedSeries = devices.map(dev => ({
    name: dev,
    type: 'line',
    stack: 'Total',
    areaStyle: {},
    symbol: 'none',
    smooth: true,
    emphasis: { focus: 'series' },
    data: (series[dev] || []).map(v => Number(v || 0) / unit.divisor)
  }))

  const totalSeries = {
    name: '总流量',
    type: 'line',
    symbol: 'none',
    smooth: true,
    lineStyle: { width: 2, opacity: 0.9 },
    emphasis: { focus: 'series' },
    data: totalBytesByTs.map(v => v / unit.divisor)
  }

  return {
    ...chartStyle,
    tooltip: {
      backgroundColor: colors.surface,
      borderColor: colors.outline,
      textStyle: { color: colors.text },
      trigger: 'axis',
      axisPointer: { type: 'cross', label: { backgroundColor: colors.primary, color: colors.onPrimary } },
      formatter: (params: unknown) => {
        const list: TooltipParam[] = Array.isArray(params)
          ? params.filter((item): item is TooltipParam => !!item && typeof item === 'object')
          : []
        const axisLabel = list[0]?.axisValue ?? ''
        const dataIndex = Number(list[0]?.dataIndex ?? -1)
        const timeLabel = formatTrafficTooltipTime(snapshot.periodStarts[dataIndex], axisLabel)
        const totalItem = list.find(item => item?.seriesName === '总流量')
        const deviceItems = list.filter(item => item?.seriesName !== '总流量')
        deviceItems.sort((a, b) => (Number(b?.value) || 0) - (Number(a?.value) || 0))

        const topItems = deviceItems.slice(0, 6)
        const otherItems = deviceItems.slice(6)
        const otherSum = otherItems.reduce((sum, item) => sum + (Number(item?.value) || 0), 0)

        let res = `<div class="font-bold mb-1">${timeLabel}</div>`
        const totalVal = Number(totalItem?.value) || 0
        res += `<div class="flex justify-between gap-4 text-xs font-bold">
          <span>总流量</span>
          <span class="font-mono">${totalVal.toFixed(unit.decimals)} ${unit.label}</span>
        </div>`
        res += `<div class="mt-1">`
        topItems.forEach(item => {
          const val = Number(item?.value) || 0
          res += `<div class="flex justify-between gap-4 text-xs">
            <span style="color:${item.color}">● ${item.seriesName}</span>
            <span class="font-mono">${val.toFixed(unit.decimals)} ${unit.label}</span>
          </div>`
        })
        if (otherItems.length > 0) {
          res += `<div class="flex justify-between gap-4 text-xs text-gray-500">
            <span>其他（${otherItems.length}）</span>
            <span class="font-mono">${otherSum.toFixed(unit.decimals)} ${unit.label}</span>
          </div>`
        }
        res += `</div>`
        return res
      }
    },
    legend: {
      type: 'scroll',
      data: ['总流量', ...devices],
      textStyle: { color: colors.muted },
      top: 0,
      left: 10,
      right: 10,
      height: 44
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: 28,
      top: 56,
      containLabel: true
    },
    xAxis: [
      {
        type: 'category',
        axisLabel: { color: colors.muted },
        boundaryGap: false,
        data: timestamps,
        axisLine: { lineStyle: { color: colors.outline } }
      }
    ],
    yAxis: [
      {
        type: 'value',
        axisLabel: { color: colors.muted },
        nameTextStyle: { color: colors.muted },
        name: `流量 (${unit.label})`,
        splitLine: { lineStyle: { color: colors.outline, type: 'dashed', opacity: 0.3 } },
        axisLine: { lineStyle: { color: colors.outline } }
      }
    ],
    dataZoom: [
      {
        type: 'inside',
        filterMode: 'none'
      }
    ],
    series: [totalSeries, ...stackedSeries],
    backgroundColor: 'transparent'
  }
})

watchEffect(() => {
  if (chartOption.value && !VChartComp.value && !chartLoadError.value) {
    void ensureChartLoaded()
  }
})

function retryChartLoad() {
  chartLoadError.value = null
  chartInitPromise = null
  void ensureChartLoaded()
}

function handleRangeChange(value: string | number | boolean | undefined) {
  if (value === 'day' || value === 'week' || value === 'month') {
    emit('update:range', value)
  }
}
</script>

<template>
  <section class="traffic-panel m3-card" :aria-label="title">
    <header class="traffic-heading">
      <div>
        <h2 class="m3-title">{{ title }}</h2>
        <p class="m3-muted">{{ subtitle }}</p>
      </div>
      <div class="m3-actions">
        <div class="range-buttons" role="group" aria-label="流量统计范围">
          <button
            v-for="option in [
              { value: 'day', label: '日' },
              { value: 'week', label: '周' },
              { value: 'month', label: '月' },
            ]"
            :key="option.value"
            type="button"
            :disabled="disabled"
            :aria-pressed="range === option.value"
            @click="handleRangeChange(option.value)"
          >
            <md-ripple />
            <SvgIcon v-if="range === option.value" :svg="checkIcon" />
            {{ option.label }}
          </button>
        </div>
        <RefreshButton :loading="loading" :disabled="disabled" @click="emit('refresh')" />
      </div>
    </header>
    <p v-if="disabled" class="traffic-empty m3-muted">网络已禁用，暂无流量分析</p>
    <template v-else>
      <ErrorState
        v-if="error"
        title="流量分析加载失败"
        :message="error.message"
        :status-code="error.status"
        :request-method="error.method"
        :request-url="error.url"
        :last-success-at="lastOkAt"
        retry-text="重试"
        @retry="emit('refresh')"
      />
      <dl class="traffic-totals">
        <div>
          <dt>{{ rangeText }}下载</dt>
          <dd>{{ formatBytes(analysisTotal.rx) }}</dd>
        </div>
        <div>
          <dt>{{ rangeText }}上传</dt>
          <dd>{{ formatBytes(analysisTotal.tx) }}</dd>
        </div>
        <div>
          <dt>{{ rangeText }}合计</dt>
          <dd>{{ formatBytes(analysisTotal.total) }}</dd>
        </div>
      </dl>
      <div v-if="chartOption && VChartComp" class="traffic-chart">
        <component :is="VChartComp" class="chart" :option="chartOption" autoresize />
      </div>
      <ErrorState
        v-else-if="chartOption && chartLoadError"
        title="流量图表加载失败"
        :message="chartLoadError"
        retry-text="重试图表"
        @retry="retryChartLoad"
      />
      <div v-else class="traffic-empty m3-muted" role="status">
        {{ chartOption && chartLoading ? '流量图表加载中…' : '暂无流量图表数据' }}
      </div>
      <div
        class="traffic-table"
        :aria-busy="!!loading"
        tabindex="0"
        role="region"
        :aria-label="`${title}明细，按时间倒序，可滚动查看`"
      >
        <table>
          <caption class="sr-only">{{ title }}明细</caption>
          <thead>
            <tr>
              <th scope="col">时间</th>
              <th scope="col">下载</th>
              <th scope="col">上传</th>
              <th scope="col">合计</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(bucket, index) in recentBuckets" :key="`${bucket.bucket}:${index}`">
              <th scope="row">{{ formatTrafficBucketTime(bucket) }}</th>
              <td>{{ formatBytes(bucket.rx_bytes) }}</td>
              <td>{{ formatBytes(bucket.tx_bytes) }}</td>
              <td>{{ formatBytes(bucket.total_bytes) }}</td>
            </tr>
            <tr v-if="!analysisBuckets.length">
              <td colspan="4" class="table-empty">{{ loading ? '正在加载…' : '暂无流量记录' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
</template>

<style scoped>
.traffic-panel {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
.traffic-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
}
.traffic-heading p {
  margin-top: 8px;
}
.range-buttons {
  display: flex;
  border: 1px solid var(--md-sys-color-outline);
  border-radius: 20px;
  overflow: hidden;
}
.range-buttons button {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  min-width: 52px;
  min-height: 40px;
  padding: 0 12px;
  color: var(--md-sys-color-on-surface);
  font-size: 14px;
  font-weight: 500;
}
.range-buttons button + button {
  border-left: 1px solid var(--md-sys-color-outline);
}
.range-buttons button[aria-pressed='true'] {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.range-buttons button:disabled {
  opacity: 0.38;
}
.range-buttons button:focus-visible {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: -3px;
}
.range-buttons .svg-icon {
  width: 18px;
  height: 18px;
}
.traffic-totals {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 140px), 1fr));
  gap: 16px;
}
.traffic-totals > div {
  padding: 16px;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container);
}
.traffic-totals dt {
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  line-height: 16px;
}
.traffic-totals dd {
  margin-top: 8px;
  font-size: 22px;
  line-height: 28px;
  overflow-wrap: anywhere;
}
.traffic-chart {
  width: 100%;
  height: 300px;
}
.traffic-empty {
  padding: 32px 16px;
  text-align: center;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container);
}
.traffic-table {
  /* One header and ten data rows; older rows remain available by scrolling. */
  max-height: calc(44px * 11);
  overflow: auto;
  scrollbar-gutter: stable;
}
.traffic-table:focus-visible {
  outline: 2px solid var(--md-sys-color-primary);
  outline-offset: 2px;
}
table {
  width: 100%;
  min-width: 480px;
  border-collapse: collapse;
  font-size: 14px;
  line-height: 20px;
}
th,
td {
  height: 44px;
  padding: 10px 12px;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
  text-align: right;
  white-space: nowrap;
}
th {
  font-weight: 500;
}
th:first-child {
  text-align: left;
}
thead th {
  position: sticky;
  top: 0;
  z-index: 1;
  background: var(--md-sys-color-surface-container-low);
  color: var(--md-sys-color-on-surface-variant);
}
.table-empty {
  text-align: center;
  color: var(--md-sys-color-on-surface-variant);
}
@media (max-width: 599px) {
  table {
    min-width: 0;
    table-layout: fixed;
    font-size: 12px;
    line-height: 16px;
  }
  th,
  td {
    padding: 4px;
    white-space: normal;
    overflow-wrap: anywhere;
  }
  th:first-child {
    width: 34%;
  }
}
</style>
