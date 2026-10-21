<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { storeToRefs } from 'pinia'
import PageHeader from '../components/PageHeader.vue'
import SvgIcon from '../components/SvgIcon.vue'
import ErrorState from '../components/ErrorState.vue'
import MaterialSnackbar from '../components/MaterialSnackbar.vue'
import StatusLight from '../components/StatusLight.vue'
import { syncMaterialSwitch as vSyncSwitch } from '../directives/materialSwitch'
import downloadIcon from '@material-symbols/svg-400/outlined/download.svg?raw'
import deleteIcon from '@material-symbols/svg-400/outlined/delete.svg?raw'
import pauseIcon from '@material-symbols/svg-400/outlined/pause.svg?raw'
import playIcon from '@material-symbols/svg-400/outlined/play_arrow.svg?raw'
import searchIcon from '@material-symbols/svg-400/outlined/search.svg?raw'
import { useLogsStore } from '../stores/logs'
import { useEventStream } from '../composables/useEventStream'

import type { LogEntry } from '../services/logs'

const logsStore = useLogsStore()
const { logs, loading, error } = storeToRefs(logsStore)

const paused = ref(false)
const autoScroll = ref(true)
const levelFilter = ref<'all' | 'debug' | 'info' | 'warn' | 'error'>('all')
const searchQuery = ref('')
const maxLogs = 1000 // 最大保留日志条数
const notice = ref('')
let disposed = false
const levels = ['all', 'debug', 'info', 'warn', 'error'] as const

function changeAutoScroll(event: Event) {
  autoScroll.value = (event.target as HTMLElement & { selected: boolean }).selected
}

function changeLevel(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  if (levels.includes(value as typeof levels[number])) levelFilter.value = value as typeof levels[number]
}

// 日志容器引用
const logContainer = ref<HTMLElement | null>(null)

// 过滤后的日志
const filteredLogs = computed(() => {
  let result = logs.value

  // 级别过滤（精确匹配选中的级别）
  if (levelFilter.value !== 'all') {
    result = result.filter(log => 
      log.level.toLowerCase() === levelFilter.value.toLowerCase()
    )
  }

  // 搜索过滤
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase()
    result = result.filter(log =>
      log.message.toLowerCase().includes(q) ||
      log.caller.toLowerCase().includes(q) ||
      (log.fields && log.fields.toLowerCase().includes(q))
    )
  }

  return result
})

const stream = useEventStream<LogEntry>({
  path: '/logs/stream',
  eventName: 'log',
  query: { level: '' },
  parse: (payload) => JSON.parse(payload) as LogEntry,
  onEvent: (entry) => {
    if (paused.value) return
    logsStore.append(entry, maxLogs)
    if (!autoScroll.value) return
    nextTick(() => {
      if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight
    })
  }
})

const { connected, lastError: lastConnectError } = stream

function connect() {
  stream.setPaused(false)
}

function disconnect() {
  stream.disconnect()
}

// 暂停/继续
function togglePause() {
  paused.value = !paused.value
  stream.setPaused(paused.value)
}

// 清空日志
function clearLogs() {
  logsStore.clear()
}

// 导出日志
function exportLogs() {
  const content = filteredLogs.value.map(log => {
    const time = new Date(log.time).toLocaleString()
    const fields = log.fields ? ` ${log.fields}` : ''
    return `[${time}] ${log.level.toUpperCase().padEnd(5)} ${log.caller} ${log.message}${fields}`
  }).join('\n')

  const blob = new Blob([content], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `logs-${new Date().toISOString().slice(0, 10)}.txt`
  a.click()
  URL.revokeObjectURL(url)
  notice.value = '已导出日志'
}

// 格式化日期时间
function formatDateTime(isoTime: string): string {
  try {
    const d = new Date(isoTime)
    const yyyy = d.getFullYear()
    const MM = String(d.getMonth() + 1).padStart(2, '0')
    const dd = String(d.getDate()).padStart(2, '0')
    const HH = String(d.getHours()).padStart(2, '0')
    const mm = String(d.getMinutes()).padStart(2, '0')
    const ss = String(d.getSeconds()).padStart(2, '0')
    return `${yyyy}-${MM}-${dd} ${HH}:${mm}:${ss}`
  } catch {
    return isoTime
  }
}

// 加载历史日志
async function loadHistory() {
  const result = await logsStore.fetchHistory(500)
  if (!result.ok) return
  nextTick(() => {
    if (logContainer.value) logContainer.value.scrollTop = logContainer.value.scrollHeight
  })
}

onMounted(async () => {
  await loadHistory()
  if (!disposed) connect()
})

onUnmounted(() => {
  disposed = true
  disconnect()
})

watch(levelFilter, () => {
  stream.setQuery({ level: levelFilter.value === 'all' ? '' : levelFilter.value })
  if (!paused.value) {
    connect()
  }
})
</script>

<template>
  <div class="m3-page logs-page">
    <PageHeader title="实时日志" subtitle="查看系统运行日志，支持级别过滤和内容搜索">
      <template #actions>
        <md-outlined-button type="button" :aria-pressed="paused" @click="togglePause">
          <SvgIcon slot="icon" :svg="paused ? playIcon : pauseIcon" />
          {{ paused ? '继续' : '暂停' }}
        </md-outlined-button>
        <md-text-button type="button" :disabled="!logs.length" @click="clearLogs">
          <SvgIcon slot="icon" :svg="deleteIcon" />
          清空
        </md-text-button>
        <md-filled-button type="button" :disabled="!filteredLogs.length" @click="exportLogs">
          <SvgIcon slot="icon" :svg="downloadIcon" />
          导出
        </md-filled-button>
      </template>
    </PageHeader>
    <ErrorState
      v-if="error"
      title="历史日志加载失败"
      :message="error.message"
      @retry="loadHistory"
    />
    <section class="m3-card log-controls" aria-label="日志筛选">
      <div class="log-toolbar">
        <span class="log-connection" role="status">
          <StatusLight
            :tone="paused ? 'neutral' : connected ? 'success' : 'warning'"
            :animated="false"
          />
          {{ paused ? '已暂停' : connected ? '实时连接中' : '等待连接' }}
        </span>
        <span class="m3-muted">显示 {{ filteredLogs.length }} / {{ logs.length }} 条</span>
        <label class="log-follow">
          自动追尾
          <md-switch
            v-sync-switch
            aria-label="自动追尾"
            :selected="autoScroll"
            @change="changeAutoScroll"
          />
        </label>
      </div>
      <div v-if="!paused && !connected && lastConnectError" class="m3-notice" role="status">
        日志流连接失败：{{ lastConnectError }}
      </div>
      <div class="log-filters">
        <md-outlined-select label="日志级别" :value="levelFilter" @change="changeLevel">
          <md-select-option
            v-for="level in levels"
            :key="level"
            :value="level"
            :selected="levelFilter === level"
          >
            <div slot="headline">{{ level === 'all' ? '全部级别' : level.toUpperCase() }}</div>
          </md-select-option>
        </md-outlined-select>
        <md-outlined-text-field
          label="搜索日志"
          :value="searchQuery"
          supporting-text="内容、调用位置或附加字段"
          @input="searchQuery = ($event.target as HTMLInputElement).value"
        >
          <SvgIcon slot="leading-icon" :svg="searchIcon" />
        </md-outlined-text-field>
      </div>
    </section>
    <section class="log-viewer" aria-label="日志内容" :aria-busy="loading">
      <div ref="logContainer" class="log-scroll" tabindex="0" aria-label="可滚动的日志列表">
        <div v-if="!filteredLogs.length" class="log-empty m3-muted">
          {{
            loading
              ? '加载历史日志…'
              : logs.length
                ? '没有匹配的日志'
                : paused
                  ? '日志已暂停'
                  : connected
                    ? '等待日志…'
                    : '等待日志流连接…'
          }}
        </div>
        <div v-for="(log, index) in filteredLogs" :key="index" class="log-entry">
          <time class="log-time" :datetime="log.time">{{ formatDateTime(log.time) }}</time>
          <span class="log-level" :data-level="log.level.toLowerCase()">
            {{ log.level.toUpperCase() }}
          </span>
          <span class="log-caller" :title="log.caller">{{ log.caller }}</span>
          <span>{{ log.message }}</span>
          <span v-if="log.fields" class="log-fields">{{ log.fields }}</span>
        </div>
      </div>
    </section>
    <MaterialSnackbar :message="notice" @dismiss="notice = ''" />
  </div>
</template>

<style scoped>
.log-controls {
  display: grid;
  gap: 16px;
}
.log-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
}
.log-connection,
.log-follow {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
}
.log-follow {
  margin-left: auto;
}
.log-filters {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr);
  gap: 16px;
}
.log-viewer {
  min-width: 0;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 12px;
  overflow: hidden;
}
.log-scroll {
  height: 60vh;
  min-height: 280px;
  overflow: auto;
  padding: 16px;
  background: var(--md-sys-color-surface-container-lowest);
  color: var(--md-sys-color-on-surface);
  font: 12px/20px var(--vh-typeface-mono);
}
.log-scroll:focus-visible {
  outline: 2px solid var(--md-sys-color-primary);
  outline-offset: -2px;
}
.log-entry {
  display: flex;
  align-items: baseline;
  gap: 12px;
  width: max-content;
  min-width: 100%;
  padding: 2px 0;
  white-space: pre;
}
.log-entry:hover {
  background: var(--md-sys-color-surface-container);
}
.log-time,
.log-caller {
  color: var(--md-sys-color-on-surface-variant);
}
.log-caller {
  width: 180px;
  flex-shrink: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.log-level {
  width: 48px;
  flex-shrink: 0;
  color: var(--md-sys-color-on-surface-variant);
  font-weight: 500;
}
.log-level[data-level='debug'] {
  color: var(--md-sys-color-tertiary);
}
.log-level[data-level='info'] {
  color: var(--md-sys-color-primary);
}
.log-level[data-level='warn'] {
  color: var(--vh-warning);
}
.log-level[data-level='error'],
.log-level[data-level='fatal'] {
  color: var(--md-sys-color-error);
}
.log-fields {
  color: var(--md-sys-color-tertiary);
}
.log-empty {
  padding: 32px 16px;
  text-align: center;
}
@media (max-width: 599px) {
  .log-filters {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
