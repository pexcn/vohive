<script setup lang="ts">
import { fieldValue } from '../utils/material'
import { computed, ref, watch } from 'vue'
import { AT_TEMPLATES } from '../constants/atTemplates'
import { devicesService } from '../services/devices'

const props = defineProps<{
  deviceId: string
  backendMode?: string
  atPort?: string
  running?: boolean
}>()

const atCmd = ref('')
const atTemplate = ref('')
const atTimeoutMs = ref(10000)
const atSending = ref(false)
const atHistory = ref<Array<{ ts: number; cmd: string; ok: boolean; response: string }>>([])

const atTemplates = AT_TEMPLATES
const hasATPort = computed(() => String(props.atPort || '').trim().length > 0)
const canUseATTerminal = computed(() => Boolean(props.running) && hasATPort.value)
const unavailableTitle = computed(() => {
  if (!props.running) return '当前设备未运行'
  if (!hasATPort.value) return '当前设备没有可用 AT 口'
  return 'AT 终端暂不可用'
})
const unavailableDescription = computed(() => {
  if (!props.running) {
    return '设备当前未启动，AT 终端暂时不可用。待设备运行后，如果存在可用的 AT 口，即可在这里直接发送 AT 指令。'
  }
  if (!hasATPort.value && props.backendMode === 'qmi') {
    return '设备当前处于纯 QMI 模式，但没有解析到可用的 AT 口，因此无法提供 AT 串口终端。'
  }
  if (!hasATPort.value) {
    return '设备当前没有可用的 AT 口，因此无法提供 AT 串口终端。'
  }
  return '当前设备暂时无法提供 AT 串口终端，请稍后重试。'
})

watch(
  () => atTemplate.value,
  (v) => {
    const cmd = String(v || '').trim()
    if (cmd) atCmd.value = cmd
  }
)

async function sendAT() {
  const cmd = String(atCmd.value || '').trim()
  if (!cmd || atSending.value || !canUseATTerminal.value) return
  atSending.value = true
  atCmd.value = '' // 清空输入框
  try {
    const result = await devicesService.sendAT(props.deviceId, {
      cmd: cmd,
      timeout_ms: atTimeoutMs.value || 10000
    })
    if (!result.ok) throw new Error(result.error.message || '请求异常')
    atHistory.value.push({
      ts: Date.now(),
      cmd,
      ok: result.data.ok,
      response: result.data.response
    })
  } catch (e: unknown) {
    atHistory.value.push({
      ts: Date.now(),
      cmd,
      ok: false,
      response: e instanceof Error ? e.message : '请求异常'
    })
  } finally {
    atSending.value = false
  }
}

function clearATHistory() {
  atHistory.value = []
}
</script>

<template>
  <section class="m3-stack" aria-label="AT 终端">
    <div>
      <h2 class="m3-title">AT 终端</h2>
      <p class="m3-muted">发送指令并查看完整响应</p>
    </div>
    <div v-if="!canUseATTerminal" class="m3-notice">
      <h3>{{ unavailableTitle }}</h3>
      <p>{{ unavailableDescription }}</p>
    </div>
    <template v-else>
      <div class="terminal-log" role="log" aria-label="AT 会话记录" aria-live="polite">
        <p v-if="!atHistory.length && !atSending" class="m3-muted">暂无 AT 会话记录</p>
        <div v-for="(h, i) in atHistory" :key="h.ts + ':' + i" class="terminal-entry">
          <p class="m3-label">{{ new Date(h.ts).toLocaleTimeString() }}</p>
          <pre class="terminal-command">{{ h.cmd }}</pre>
          <pre class="terminal-response" :class="{ 'm3-error': !h.ok }">{{ h.response }}</pre>
        </div>
        <p v-if="atSending" role="status" class="m3-muted">等待模组响应…</p>
      </div>
      <form class="m3-form" @submit.prevent="sendAT">
        <md-outlined-select
          label="快捷指令模板"
          :value="atTemplate"
          :disabled="atSending"
          @change="atTemplate = fieldValue($event)"
        >
          <md-select-option value="" :selected="!atTemplate"
            ><div slot="headline">自由输入</div></md-select-option
          >
          <template v-for="group in atTemplates" :key="group.label"
            ><md-select-option
              v-for="item in group.items"
              :key="item.value"
              :value="item.value"
              :selected="atTemplate === item.value"
              ><div slot="headline">{{ group.label }} · {{ item.label }}</div></md-select-option
            ></template
          >
        </md-outlined-select>
        <div class="m3-form-grid">
          <md-outlined-text-field
            name="at-command"
            label="AT 指令"
            placeholder="例如 AT+CSQ"
            :value="atCmd"
            :disabled="atSending"
            @input="atCmd = fieldValue($event)"
          /><md-outlined-text-field
            label="超时（毫秒）"
            type="number"
            min="1"
            :value="String(atTimeoutMs)"
            :disabled="atSending"
            @input="atTimeoutMs = Number(fieldValue($event))"
          />
        </div>
        <div class="m3-actions">
          <md-text-button type="button" :disabled="atSending" @click="clearATHistory"
            >清空记录</md-text-button
          ><md-filled-button type="submit" :disabled="atSending || !atCmd.trim()">{{
            atSending ? '发送中…' : '发送'
          }}</md-filled-button>
        </div>
      </form>
    </template>
  </section>
</template>
