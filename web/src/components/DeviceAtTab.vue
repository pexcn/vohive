<script setup lang="ts">
import { fieldValue } from '../utils/material'
import { computed, ref, watch } from 'vue'
import { AT_TEMPLATES } from '../constants/atTemplates'
import { devicesService } from '../services/devices'
import SvgIcon from './SvgIcon.vue'
import terminalIcon from '@material-symbols/svg-400/outlined/terminal.svg?raw'
import sendIcon from '@material-symbols/svg-400/outlined/send.svg?raw'
import clearIcon from '@material-symbols/svg-400/outlined/delete_sweep.svg?raw'
import settingsIcon from '@material-symbols/svg-400/outlined/tune.svg?raw'

const props = defineProps<{
  deviceId: string
  backendMode?: string
  atPort?: string
  running?: boolean
}>()

const atCmd = ref('')
const atTemplate = ref('')
const atTimeoutSeconds = ref(10)
const settingsOpen = ref(false)
const timeoutOptions = [5, 10, 15, 30, 60]
const atSending = ref(false)
const atLog = ref<HTMLElement | null>(null)
const atHistory = ref<Array<{ ts: number; cmd: string; ok: boolean; response: string }>>([])
const displayedHistory = computed(() => atHistory.value.map(entry => {
  const firstLine = entry.response.match(/^\s*([^\r\n]+)(?:\r\n|\r|\n|$)/)
  const response = firstLine?.[1].trim() === entry.cmd
    ? entry.response.slice(firstLine[0].length)
    : entry.response
  const okLine = entry.ok ? response.match(/(?:^|\r\n|\r|\n)[ \t]*OK\s*$/) : null
  return {
    ...entry,
    displayResponse: okLine ? response.slice(0, okLine.index).trimEnd() : response,
    hasOkStatus: !!okLine
  }
}))

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

watch([() => atHistory.value.length, atSending], () => {
  if (atLog.value) atLog.value.scrollTop = atLog.value.scrollHeight
}, { flush: 'post' })

async function sendAT() {
  const cmd = String(atCmd.value || '').trim()
  if (!cmd || atSending.value || !canUseATTerminal.value) return
  atSending.value = true
  atCmd.value = '' // 清空输入框
  try {
    const result = await devicesService.sendAT(props.deviceId, {
      cmd: cmd,
      timeout_ms: atTimeoutSeconds.value * 1000
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

function sendFromKeyboard(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  void sendAT()
}

function clearATHistory() {
  atHistory.value = []
}
</script>

<template>
  <section class="m3-stack at-terminal" aria-label="AT 终端">
    <div class="m3-heading">
      <h2 class="m3-title">AT 终端</h2>
      <p class="m3-muted">发送指令并查看完整响应</p>
    </div>
    <div v-if="!canUseATTerminal" class="m3-notice m3-heading">
      <h3>{{ unavailableTitle }}</h3>
      <p>{{ unavailableDescription }}</p>
    </div>
    <div v-else class="at-console">
      <div class="at-log-heading">
        <span>会话记录 <small v-if="atHistory.length">{{ atHistory.length }} 条</small></span>
        <md-text-button
          class="at-clear"
          type="button"
          :disabled="atSending || !atHistory.length"
          @click="clearATHistory"
        ><SvgIcon slot="icon" :svg="clearIcon" />清空记录</md-text-button>
      </div>
      <div ref="atLog" class="terminal-log at-log" role="log" aria-label="AT 会话记录" aria-live="polite">
        <div v-if="!atHistory.length && !atSending" class="at-empty">
          <SvgIcon :svg="terminalIcon" />
          <strong>暂无会话记录</strong>
          <p>输入 AT 指令，响应会显示在这里</p>
        </div>
        <article v-for="(h, i) in displayedHistory" :key="h.ts + ':' + i" class="terminal-entry">
          <header class="at-entry-heading">
            <span class="at-prompt" aria-hidden="true">›</span>
            <pre class="terminal-command">{{ h.cmd }}</pre>
            <time :datetime="new Date(h.ts).toISOString()">{{ new Date(h.ts).toLocaleTimeString() }}</time>
          </header>
          <pre class="terminal-response" :class="{ 'm3-error': !h.ok }">{{ h.displayResponse }}<span v-if="h.hasOkStatus" class="at-ok">{{ h.displayResponse ? '\nOK' : 'OK' }}</span></pre>
        </article>
        <p v-if="atSending" role="status" class="m3-muted">等待模组响应…</p>
      </div>
      <form class="m3-form at-composer" @submit.prevent="sendAT">
        <div class="at-command-line">
          <md-outlined-text-field
            name="at-command"
            label="AT 指令"
            placeholder="例如 AT+CSQ"
            :value="atCmd"
            :disabled="atSending"
            @input="atCmd = fieldValue($event)"
            @keydown.enter="sendFromKeyboard"
          />
          <md-filled-button class="at-send" type="submit" :disabled="atSending || !atCmd.trim()">
            <SvgIcon slot="icon" :svg="sendIcon" />{{ atSending ? '发送中…' : '发送' }}
          </md-filled-button>
        </div>
        <div>
          <div class="at-settings-row">
            <md-outlined-select
              class="at-template"
              name="at-template"
              label="快捷指令模板"
              :value="atTemplate"
              :disabled="atSending"
              @change="atTemplate = fieldValue($event)"
            >
              <md-select-option value="" :selected="!atTemplate">
                <div slot="headline">自由输入</div>
              </md-select-option>
              <template v-for="group in atTemplates" :key="group.label">
                <md-select-option
                  v-for="item in group.items"
                  :key="item.value"
                  :value="item.value"
                  :selected="atTemplate === item.value"
                ><div slot="headline">{{ group.label }} · {{ item.label }}</div></md-select-option>
              </template>
            </md-outlined-select>
            <md-text-button
              class="at-more-settings"
              type="button"
              :aria-expanded="settingsOpen"
              :aria-controls="`at-settings-${deviceId}`"
              :disabled="atSending"
              @click="settingsOpen = !settingsOpen"
            ><SvgIcon slot="icon" :svg="settingsIcon" />{{ settingsOpen ? '收起设置' : '更多设置' }}</md-text-button>
          </div>
          <Transition name="at-settings">
            <div v-if="settingsOpen" :id="`at-settings-${deviceId}`" class="at-settings-reveal">
              <div>
                <div class="at-settings-fields">
                  <md-outlined-select
                    class="at-timeout"
                    name="at-timeout"
                    label="响应超时"
                    :value="String(atTimeoutSeconds)"
                    :disabled="atSending"
                    @change="atTimeoutSeconds = Number(fieldValue($event))"
                  >
                    <md-select-option
                      v-for="seconds in timeoutOptions"
                      :key="seconds"
                      :value="String(seconds)"
                      :selected="atTimeoutSeconds === seconds"
                    ><div slot="headline">{{ seconds }} 秒{{ seconds === 10 ? '（默认）' : '' }}</div></md-select-option>
                  </md-outlined-select>
                </div>
              </div>
            </div>
          </Transition>
        </div>
      </form>
    </div>
  </section>
</template>

<style scoped>
.at-console {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 16px;
}
.at-log-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 52px;
  padding: 0 16px;
  background: var(--md-sys-color-surface-container);
}
.at-log-heading > span {
  font-weight: 500;
}
.at-log-heading small {
  margin-left: 8px;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  font-weight: 400;
}
.at-clear {
  --md-text-button-container-height: 36px;
  --md-text-button-leading-space: 12px;
  --md-text-button-trailing-space: 12px;
}
.at-log {
  height: 328px;
  padding: 20px;
  border-radius: 0;
}
.terminal-entry {
  margin-bottom: 0;
}
.terminal-entry + .terminal-entry,
.terminal-entry + [role='status'] {
  margin-top: 20px;
}
.at-entry-heading {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.at-prompt {
  color: var(--md-sys-color-primary);
  font-size: 22px;
}
.terminal-command {
  flex: 1;
  min-width: 0;
  padding: 0;
  border-radius: 0;
  background: none;
  color: var(--md-sys-color-primary);
}
.at-entry-heading time {
  flex: none;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 11px;
}
.terminal-response {
  margin-top: 6px;
  padding-left: 16px;
  font-size: 13px;
  line-height: 22px;
}
.at-ok {
  color: var(--vh-success);
}
.at-empty {
  display: grid;
  place-content: center;
  justify-items: center;
  gap: 8px;
  height: 100%;
  text-align: center;
  font-family: var(--md-ref-typeface-plain);
  color: var(--md-sys-color-on-surface-variant);
}
.at-empty .svg-icon {
  width: 36px;
  height: 36px;
  margin-bottom: 8px;
  opacity: 0.65;
}
.at-empty strong {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  color: var(--md-sys-color-on-surface);
}
.at-empty p {
  font-size: 13px;
}
.at-composer {
  padding: 20px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
  background: var(--md-sys-color-surface-container);
}
.at-command-line {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
}
.at-command-line md-outlined-text-field {
  --md-outlined-text-field-input-text-font: var(--vh-typeface-mono);
}
.at-send {
  --md-filled-button-container-height: 48px;
  --md-filled-button-leading-space: 22px;
  --md-filled-button-trailing-space: 22px;
}
.at-send .svg-icon,
.at-clear .svg-icon,
.at-more-settings .svg-icon {
  width: 18px;
  height: 18px;
}
.at-settings-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.at-template {
  flex: 0 1 360px;
  width: 360px;
  min-width: 0;
}
.at-more-settings {
  flex: none;
  margin-left: auto;
}
.at-settings-reveal {
  display: grid;
  grid-template-rows: 1fr;
}
.at-settings-reveal > div {
  min-height: 0;
  overflow: hidden;
}
.at-settings-fields {
  margin-top: 16px;
  padding-top: 20px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.at-timeout {
  width: 184px;
}
@media (prefers-reduced-motion: no-preference) {
  .at-settings-enter-active,
  .at-settings-leave-active {
    transition: grid-template-rows 220ms cubic-bezier(0.2, 0, 0, 1), opacity 220ms;
  }
  .at-settings-enter-from,
  .at-settings-leave-to {
    grid-template-rows: 0fr;
    opacity: 0;
  }
}
.at-settings-leave-active {
  pointer-events: none;
}
@media (max-width: 640px) {
  .at-log-heading {
    padding: 0 8px 0 12px;
  }
  .at-log-heading small {
    display: none;
  }
  .at-log {
    height: 340px;
    padding: 16px 12px;
  }
  .terminal-response {
    padding-left: 0;
    font-size: 12px;
    line-height: 20px;
  }
  .at-entry-heading {
    gap: 6px;
  }
  .at-entry-heading time {
    font-size: 10px;
  }
  .at-composer {
    padding: 16px 12px;
  }
  .at-command-line {
    grid-template-columns: minmax(0, 1fr) 80px;
    gap: 10px;
  }
  .at-send {
    --md-filled-button-leading-space: 12px;
    --md-filled-button-trailing-space: 12px;
  }
  .at-send .svg-icon,
  .at-more-settings .svg-icon {
    display: none;
  }
  .at-settings-row {
    gap: 8px;
  }
  .at-template {
    flex: 1;
    width: auto;
  }
  .at-more-settings {
    --md-text-button-leading-space: 10px;
    --md-text-button-trailing-space: 10px;
  }
  .at-timeout {
    width: 100%;
  }
}
</style>
