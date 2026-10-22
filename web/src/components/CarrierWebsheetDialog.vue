<script setup lang="ts">

import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { api } from '../stores/auth'
import type { CarrierWebsheetInfo } from '../types/api'

const props = defineProps<{
  modelValue: boolean
  websheet: CarrierWebsheetInfo | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  done: []
}>()

const loaded = ref(false)
const iframeEl = ref<HTMLIFrameElement | null>(null)
let completing = false
let websheetChannel: BroadcastChannel | null = null

const iframeSrc = computed(() => props.websheet?.embedUrl || '')
const websheetToken = computed(() => {
  const raw = props.websheet?.embedUrl || ''
  if (!raw) return ''
  try {
    const url = new URL(raw, window.location.origin)
    return url.searchParams.get('token') || ''
  } catch {
    return ''
  }
})

watch(() => props.modelValue, value => { if (value) loaded.value = false })

watch(() => props.websheet?.id, () => {
  loaded.value = false
})

async function sendCallback(callback: unknown) {
  const id = props.websheet?.id
  if (!id || !callback || typeof callback !== 'object') return
  try {
    await api.post(`/websheets/${id}/callback`, callback)
  } catch (err) {
    console.error('[CarrierWebsheetDialog] relay callback failed:', err)
  }
}

async function completeWebsheet() {
  if (completing) return
  completing = true
  try {
    const id = props.websheet?.id
    if (id) {
      try {
        await api.post(`/websheets/${id}/done`)
      } catch (err) {
        console.error('[CarrierWebsheetDialog] complete websheet failed:', err)
      }
    }
    emit('done')
    emit('update:modelValue', false)
  } finally {
    completing = false
  }
}

function isTerminalCallback(callback: unknown) {
  if (!callback || typeof callback !== 'object') return true
  const record = callback as { event?: unknown; method?: unknown; resultCode?: unknown }
  const value = String(record.event ?? record.method ?? record.resultCode ?? '').toLowerCase()
  if (!value) return true
  return !value.includes('phoneservicesaccountstatuschanged')
}

function isCurrentWebsheetMessage(data: unknown): data is { type: string; token?: unknown; callback?: unknown } {
  if (!data || typeof data !== 'object') return false
  const record = data as { type?: unknown; token?: unknown }
  if (record.type !== 'vohive-websheet-callback') return false
  const currentToken = websheetToken.value
  const incomingToken = typeof record.token === 'string' ? record.token : ''
  if (currentToken && incomingToken && incomingToken !== currentToken) return false
  return true
}

function handleShellMessage(data: unknown) {
  if (!props.modelValue || !isCurrentWebsheetMessage(data)) return
  if (isTerminalCallback(data.callback)) {
    void completeWebsheet()
  } else {
    void sendCallback(data.callback)
  }
}

function onMessage(event: MessageEvent) {
  handleShellMessage(event.data)
}

function onStorage(event: StorageEvent) {
  if (event.key !== 'vohive-websheet-complete' || !event.newValue) return
  try {
    handleShellMessage(JSON.parse(event.newValue))
  } catch {
    // Ignore stale or malformed completion notifications.
  }
}

onMounted(() => {
  window.addEventListener('message', onMessage)
  window.addEventListener('storage', onStorage)
  try {
    websheetChannel = new BroadcastChannel('vohive-websheet')
    websheetChannel.onmessage = event => handleShellMessage(event.data)
  } catch {
    websheetChannel = null
  }
})
onUnmounted(() => {
  window.removeEventListener('message', onMessage)
  window.removeEventListener('storage', onStorage)
  websheetChannel?.close()
  websheetChannel = null
})
</script>

<template>
  <md-dialog
    class="m3-dialog websheet-dialog"
    :open="modelValue"
    @closed="emit('update:modelValue', false)"
  >
    <div slot="headline">{{ websheet?.title || 'E911 地址' }}</div>
    <div slot="content">
      <div class="websheet-frame-shell">
        <div v-if="!loaded" class="websheet-loading" role="status">
          <md-circular-progress indeterminate aria-label="加载运营商页面" />
        </div>
        <iframe
          v-if="modelValue && iframeSrc"
          ref="iframeEl"
          :src="iframeSrc"
          title="运营商地址设置"
          sandbox="allow-forms allow-same-origin allow-scripts allow-popups allow-popups-to-escape-sandbox"
          @load="loaded = true"
        />
      </div>
    </div>
    <div slot="actions">
      <md-text-button @click="emit('update:modelValue', false)">关闭</md-text-button>
    </div>
  </md-dialog>
</template>
<style scoped>
.websheet-dialog {
  width: min(438px, calc(100vw - 48px));
  max-height: calc(100dvh - 48px);
}
.websheet-frame-shell {
  position: relative;
  height: min(620px, calc(100dvh - 220px));
  min-height: 120px;
}
.websheet-loading {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: var(--md-sys-color-surface);
}
iframe {
  display: block;
  width: 100%;
  height: 100%;
  border: 0;
}
</style>
