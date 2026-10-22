<script setup lang="ts">
import { syncMaterialSwitch as vSyncSwitch } from '../directives/materialSwitch'
import { fieldSelected } from '../utils/material'
import { notify } from '../stores/feedback'
import { computed, onMounted, ref, watch } from 'vue'
import { debugCollector } from '../debug/collector'
import { copyToClipboard } from '../utils/clipboard'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
}>()

const open = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v)
})

const currentHref = computed(() => {
  return typeof window !== 'undefined' ? window.location.href : ''
})

const autoOpen = ref(false)

onMounted(() => {
  autoOpen.value = localStorage.getItem('debug_panel_auto_open') === '1'
})

watch(
  () => autoOpen.value,
  (v) => {
    localStorage.setItem('debug_panel_auto_open', v ? '1' : '0')
  }
)

function fmtTs(ts: number) {
  return new Date(ts).toLocaleString()
}

async function copySnapshot() {
  await copyToClipboard(
    JSON.stringify(debugCollector.sanitizedSnapshot(), null, 2),
    '已复制诊断信息'
  )
}

function downloadSnapshot() {
  try {
    const now = new Date()
    const stamp = `${now.getFullYear()}${String(now.getMonth() + 1).padStart(2, '0')}${String(now.getDate()).padStart(2, '0')}-${String(now.getHours()).padStart(2, '0')}${String(now.getMinutes()).padStart(2, '0')}${String(now.getSeconds()).padStart(2, '0')}`
    const blob = new Blob([JSON.stringify(debugCollector.sanitizedSnapshot(), null, 2)], {
      type: 'application/json;charset=utf-8'
    })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `debug-${stamp}.json`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    notify({ tone: 'success', message: '已下载诊断文件' })
  } catch {
    notify({ tone: 'error', message: '导出失败' })
  }
}

function clearAll() {
  debugCollector.clearAll()
  notify({ tone: 'success', message: '已清空' })
}

const debugSections = computed(() => [
  {
    title: '最近路由',
    items: debugCollector.routes.value
      .slice()
      .reverse()
      .map(
        (item) => `${fmtTs(item.ts)}\n${item.from || '-'} → ${item.to || '-'} ${item.name || ''}`
      )
  },
  {
    title: '最近 API 错误',
    items: debugCollector.apiErrors.value
      .slice()
      .reverse()
      .map(
        (item) =>
          `${fmtTs(item.ts)} · ${item.status || ''} ${item.method || ''} ${item.url || ''}\n${item.message}`
      )
  },
  {
    title: '最近前端错误',
    items: debugCollector.jsErrors.value
      .slice()
      .reverse()
      .map(
        (item) => `${fmtTs(item.ts)} · ${item.source || ''}\n${item.message}\n${item.stack || ''}`
      )
  },
  {
    title: '鉴权事件',
    items: debugCollector.authEvents.value
      .slice()
      .reverse()
      .map((item) => `${fmtTs(item.ts)} · ${item.kind} ${item.redirectTo || ''}`)
  }
])
</script>

<template>
  <md-dialog class="m3-dialog debug-dialog" :open="open" @closed="open = false">
    <div slot="headline">诊断面板</div>
    <div slot="content" class="m3-stack">
      <p class="debug-url m3-muted">{{ currentHref }}</p>
      <label class="m3-switch-row"
        ><span>出现前端错误时自动打开</span
        ><md-switch
          v-sync-switch
          aria-label="错误时自动打开诊断"
          :selected="autoOpen"
          @change="autoOpen = fieldSelected($event)"
      /></label>
      <section v-for="section in debugSections" :key="section.title" class="m3-inset m3-stack">
        <h3>{{ section.title }}</h3>
        <p v-if="!section.items.length" class="m3-muted">暂无记录</p>
        <div class="debug-records">
          <pre v-for="(entry, index) in section.items" :key="index">{{ entry }}</pre>
        </div>
      </section>
    </div>
    <div slot="actions">
      <md-text-button @click="clearAll">清空</md-text-button
      ><md-text-button @click="downloadSnapshot">导出</md-text-button
      ><md-text-button @click="copySnapshot">复制</md-text-button
      ><md-filled-button @click="open = false">关闭</md-filled-button>
    </div>
  </md-dialog>
</template>
<style scoped>
.debug-dialog {
  max-height: calc(100dvh - 48px);
}
.debug-url,
pre {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}
.debug-records {
  max-height: 260px;
  overflow: auto;
}
.debug-records pre {
  padding-block: 8px;
  font: 12px/20px var(--vh-typeface-mono);
}
h3 {
  font-size: 16px;
  font-weight: 500;
}
</style>
