<script setup lang="ts">
import { fieldValue } from '../utils/material'
import { computed, ref, watch } from 'vue'
import { devicesService } from '../services/devices'
import { errorMessage } from '../services/http'
import SvgIcon from './SvgIcon.vue'
import dialpadIcon from '@material-symbols/svg-400/outlined/dialpad.svg?raw'
import sendIcon from '@material-symbols/svg-400/outlined/send.svg?raw'
import clearIcon from '@material-symbols/svg-400/outlined/delete_sweep.svg?raw'
import settingsIcon from '@material-symbols/svg-400/outlined/tune.svg?raw'

const props = defineProps<{
  deviceId: string
  vowifiActive?: boolean
}>()

type UssdMessage = {
  ts: number
  type: 'req' | 'res' | 'err' | 'sys'
  content: string
  dcs?: number
  channel?: string
}

const ussdCmd = ref('')
const timeoutSeconds = ref(45)
const timeoutOptions = [15, 30, 45, 60]
const settingsOpen = ref(false)
const sending = ref(false)
const cancelling = ref(false)
const busy = computed(() => sending.value || cancelling.value)
const sessionId = ref('')
const sessionChannel = ref('')
const history = ref<UssdMessage[]>([])
const ussdLog = ref<HTMLElement | null>(null)
const rounds = computed(() => {
  const entries: Array<{ request: UssdMessage; responses: UssdMessage[] }> = []
  for (const message of history.value) {
    if (message.type === 'req') entries.push({ request: message, responses: [] })
    else entries.at(-1)?.responses.push(message)
  }
  return entries
})

const isMultiRound = computed(() => !!sessionId.value)
const sessionEnded = computed(() => history.value.length > 0 && !isMultiRound.value && !busy.value)
const inputPlaceholder = computed(() => isMultiRound.value ? '输入菜单选项，例如 1' : '例如 *100#')

watch([() => history.value.length, busy], () => {
  if (ussdLog.value) ussdLog.value.scrollTop = ussdLog.value.scrollHeight
}, { flush: 'post' })

async function sendUSSD() {
  const cmd = ussdCmd.value.trim()
  if (!cmd || busy.value) return
  const timeoutMs = timeoutSeconds.value * 1000

  history.value.push({ ts: Date.now(), type: 'req', content: cmd })
  sending.value = true
  ussdCmd.value = ''

  try {
    let d: { status?: number; text?: string; rawText?: string; dcs?: number; sessionId?: string; channel?: string }

    if (isMultiRound.value) {
      const result = await devicesService.continueUSSD(props.deviceId, {
        session_id: sessionId.value,
        input: cmd,
        timeout_ms: timeoutMs
      }, timeoutMs + 2000)
      if (!result.ok) throw new Error(result.error.message || '请求异常')
      d = result.data
    } else {
      const result = await devicesService.sendUSSD(props.deviceId, {
        command: cmd,
        timeout_ms: timeoutMs
      }, timeoutMs + 2000)
      if (!result.ok) throw new Error(result.error.message || '请求异常')
      d = result.data
    }

    if (d.channel) sessionChannel.value = d.channel

    if (d.status === 5) {
      history.value.push({
        ts: Date.now(), type: 'err',
        content: `[网络不支持/无响应]\n${d.text || d.rawText || '[空响应]'}`,
        dcs: d.dcs, channel: d.channel
      })
      endSession()
    } else if (d.status === 2) {
      history.value.push({
        ts: Date.now(), type: 'err',
        content: `[被网络终止]\n${d.text || d.rawText || '[空响应]'}`,
        dcs: d.dcs, channel: d.channel
      })
      endSession()
    } else {
      history.value.push({
        ts: Date.now(), type: 'res', content: d.text || d.rawText || '[空响应]',
        dcs: d.dcs, channel: d.channel
      })
      if (d.status === 1 && d.sessionId) sessionId.value = d.sessionId
      else endSession()
    }
  } catch (e: unknown) {
    history.value.push({ ts: Date.now(), type: 'err', content: errorMessage(e, '请求异常') })
    endSession()
  } finally {
    sending.value = false
  }
}

async function cancelSession() {
  if (!sessionId.value || busy.value) return
  cancelling.value = true
  try {
    const result = await devicesService.cancelUSSD(props.deviceId, sessionId.value)
    if (!result.ok) throw new Error(result.error.message || '取消会话失败')
    history.value.push({ ts: Date.now(), type: 'sys', content: '会话已手动取消' })
    ussdCmd.value = ''
    endSession()
  } catch (e: unknown) {
    history.value.push({ ts: Date.now(), type: 'err', content: errorMessage(e, '取消会话失败') })
  } finally {
    cancelling.value = false
  }
}

function endSession() {
  sessionId.value = ''
  sessionChannel.value = ''
}

function clearHistory() {
  if (busy.value) return
  history.value = []
  endSession()
}

function sendFromKeyboard(event: KeyboardEvent) {
  if (event.isComposing) return
  event.preventDefault()
  void sendUSSD()
}
</script>

<template>
  <section class="m3-stack ussd-terminal" aria-label="USSD 终端">
    <div class="m3-heading">
      <h2 class="m3-title">USSD 交互终端</h2>
      <p class="m3-muted">发送代码并查看网络菜单响应</p>
    </div>
    <div class="ussd-console">
      <div class="ussd-log-heading">
        <div class="ussd-log-title">
          <span>会话记录</span>
          <span v-if="sessionEnded" class="ussd-ended">会话已结束</span>
        </div>
        <md-text-button class="ussd-clear" type="button" :disabled="busy || !history.length" @click="clearHistory">
          <SvgIcon slot="icon" :svg="clearIcon" />清空记录
        </md-text-button>
      </div>
      <div ref="ussdLog" class="terminal-log ussd-log" role="log" aria-label="USSD 会话记录" aria-live="polite">
        <div v-if="!history.length && !busy" class="ussd-empty">
          <SvgIcon :svg="dialpadIcon" />
          <strong>暂无会话记录</strong>
          <p>输入 USSD 代码，网络响应会显示在这里</p>
        </div>
        <article v-for="(round, index) in rounds" :key="round.request.ts + ':' + index" class="ussd-round">
          <header class="ussd-request-heading">
            <span class="ussd-prompt" aria-hidden="true">›</span>
            <pre class="ussd-command">{{ round.request.content }}</pre>
            <time :datetime="new Date(round.request.ts).toISOString()">{{ new Date(round.request.ts).toLocaleTimeString() }}</time>
          </header>
          <div v-for="(message, responseIndex) in round.responses" :key="responseIndex" class="ussd-response">
            <div class="ussd-response-meta" :title="message.dcs !== undefined ? `DCS ${message.dcs}` : undefined">
              <span>{{ message.type === 'sys' ? '会话状态' : message.type === 'err' ? '错误响应' : '网络响应' }}</span>
              <span v-if="message.channel" class="ussd-channel">{{ message.channel === 'vowifi' ? 'VoWiFi' : 'CS' }}</span>
            </div>
            <pre :class="{ 'm3-error': message.type === 'err' }">{{ message.content }}</pre>
          </div>
        </article>
        <p v-if="sending" class="m3-muted ussd-pending" role="status">等待网络响应…</p>
      </div>
      <form class="m3-form ussd-composer" :class="{ 'ussd-session-active': isMultiRound }" @submit.prevent="sendUSSD">
        <div v-if="isMultiRound" class="ussd-session-strip">
          <span class="ussd-session-state" role="status">
            <i class="ussd-session-dot" aria-hidden="true"></i>
            {{ cancelling ? '正在取消会话' : sending ? '等待网络响应' : '等待菜单回复' }}<template v-if="sessionChannel"> · {{ sessionChannel === 'vowifi' ? 'VoWiFi' : 'CS' }}</template>
          </span>
          <md-text-button class="ussd-cancel" type="button" :disabled="busy" @click="cancelSession">
            {{ cancelling ? '取消中…' : '取消会话' }}
          </md-text-button>
        </div>
        <div class="ussd-command-line">
          <md-outlined-text-field
            name="ussd-command"
            :label="isMultiRound ? '菜单回复' : 'USSD 代码'"
            :placeholder="inputPlaceholder"
            :value="ussdCmd"
            :disabled="busy"
            @input="ussdCmd = fieldValue($event)"
            @keydown.enter="sendFromKeyboard"
          />
          <md-filled-button class="ussd-send" type="submit" :disabled="busy || !ussdCmd.trim()">
            <SvgIcon slot="icon" :svg="sendIcon" />{{ sending ? '发送中…' : isMultiRound ? '回复' : '发送' }}
          </md-filled-button>
        </div>
        <div class="ussd-settings">
          <div class="ussd-settings-row">
            <md-text-button
              class="ussd-more-settings"
              type="button"
              :aria-expanded="settingsOpen"
              :aria-controls="`ussd-settings-${deviceId}`"
              :disabled="busy"
              @click="settingsOpen = !settingsOpen"
            ><SvgIcon slot="icon" :svg="settingsIcon" />{{ settingsOpen ? '收起设置' : '更多设置' }}</md-text-button>
          </div>
          <Transition name="ussd-settings">
            <div v-if="settingsOpen" :id="`ussd-settings-${deviceId}`" class="ussd-settings-reveal">
              <div>
                <div class="ussd-settings-fields">
                  <md-outlined-select
                    class="ussd-timeout"
                    name="ussd-timeout"
                    label="响应超时"
                    :value="String(timeoutSeconds)"
                    :disabled="busy"
                    @change="timeoutSeconds = Number(fieldValue($event))"
                  >
                    <md-select-option
                      v-for="seconds in timeoutOptions"
                      :key="seconds"
                      :value="String(seconds)"
                      :selected="timeoutSeconds === seconds"
                    ><div slot="headline">{{ seconds }} 秒{{ seconds === 45 ? '（默认）' : '' }}</div></md-select-option>
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
.ussd-console {
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--md-sys-color-outline-variant);
  border-radius: 16px;
}
.ussd-log-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 52px;
  padding: 0 16px;
  background: var(--md-sys-color-surface-container);
}
.ussd-log-title {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  font-weight: 500;
}
.ussd-ended {
  padding: 3px 8px;
  border-radius: 6px;
  background: var(--md-sys-color-surface-container-highest);
  color: var(--md-sys-color-on-surface-variant);
  font-size: 12px;
  font-weight: 400;
  line-height: 18px;
}
.ussd-clear,
.ussd-cancel {
  --md-text-button-container-height: 36px;
  --md-text-button-leading-space: 12px;
  --md-text-button-trailing-space: 12px;
}
.ussd-log {
  height: 360px;
  padding: 20px;
  border-radius: 0;
}
.ussd-round + .ussd-round {
  margin-top: 24px;
}
.ussd-request-heading {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.ussd-prompt {
  color: var(--md-sys-color-primary);
  font-size: 22px;
}
.ussd-command {
  flex: 1;
  min-width: 0;
  color: var(--md-sys-color-primary);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.ussd-request-heading time {
  flex: none;
  color: var(--md-sys-color-on-surface-variant);
  font-size: 11px;
}
.ussd-response {
  margin-top: 10px;
  padding-left: 16px;
}
.ussd-response-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
  color: var(--md-sys-color-on-surface-variant);
  font: 11px/18px var(--md-ref-typeface-plain);
}
.ussd-channel {
  padding: 0 6px;
  border-radius: 4px;
  background: var(--md-sys-color-surface-container-highest);
}
.ussd-response pre {
  font: 13px/22px var(--vh-typeface-mono);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.ussd-pending {
  margin-top: 16px;
}
.ussd-empty {
  display: grid;
  place-content: center;
  justify-items: center;
  gap: 8px;
  height: 100%;
  text-align: center;
  font-family: var(--md-ref-typeface-plain);
  color: var(--md-sys-color-on-surface-variant);
}
.ussd-empty .svg-icon {
  width: 36px;
  height: 36px;
  margin-bottom: 8px;
  opacity: 0.65;
}
.ussd-empty strong {
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  color: var(--md-sys-color-on-surface);
}
.ussd-empty p {
  font-size: 13px;
}
.ussd-composer {
  padding: 20px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
  background: var(--md-sys-color-surface-container);
}
.ussd-session-strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--md-sys-color-outline-variant);
}
.ussd-session-state {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 8px;
  background: var(--md-sys-color-primary-container);
  color: var(--md-sys-color-on-primary-container);
  font-size: 12px;
  line-height: 20px;
}
.ussd-session-dot {
  flex: none;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}
.ussd-command-line {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 16px;
}
.ussd-command-line md-outlined-text-field {
  --md-outlined-text-field-input-text-font: var(--vh-typeface-mono);
}
.ussd-send {
  --md-filled-button-container-height: 48px;
  --md-filled-button-leading-space: 22px;
  --md-filled-button-trailing-space: 22px;
}
.ussd-send .svg-icon,
.ussd-clear .svg-icon,
.ussd-more-settings .svg-icon {
  width: 18px;
  height: 18px;
}
.ussd-settings-row {
  display: flex;
  justify-content: flex-end;
}
.ussd-settings-reveal {
  display: grid;
  grid-template-rows: 1fr;
}
.ussd-settings-reveal > div {
  min-height: 0;
  overflow: hidden;
}
.ussd-settings-fields {
  margin-top: 16px;
  padding-top: 20px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.ussd-timeout {
  width: 184px;
}
@media (prefers-reduced-motion: no-preference) {
  .ussd-settings-enter-active,
  .ussd-settings-leave-active {
    transition: grid-template-rows 220ms cubic-bezier(0.2, 0, 0, 1), opacity 220ms;
  }
  .ussd-settings-enter-from,
  .ussd-settings-leave-to {
    grid-template-rows: 0fr;
    opacity: 0;
  }
}
.ussd-settings-leave-active {
  pointer-events: none;
}
@media (max-width: 640px) {
  .ussd-log-heading {
    padding: 8px 8px 8px 12px;
  }
  .ussd-log-title {
    gap: 6px;
  }
  .ussd-ended {
    padding: 2px 6px;
    font-size: 10px;
  }
  .ussd-clear {
    --md-text-button-leading-space: 8px;
    --md-text-button-trailing-space: 8px;
  }
  .ussd-clear .svg-icon,
  .ussd-more-settings .svg-icon {
    display: none;
  }
  .ussd-log {
    padding: 16px 12px;
  }
  .ussd-response {
    padding-left: 0;
  }
  .ussd-response pre {
    font-size: 12px;
  }
  .ussd-request-heading {
    gap: 6px;
    font-size: 13px;
  }
  .ussd-request-heading time {
    font-size: 10px;
  }
  .ussd-composer {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 16px 8px;
    padding: 16px 12px;
  }
  .ussd-command-line,
  .ussd-settings,
  .ussd-settings-row {
    display: contents;
  }
  .ussd-command-line md-outlined-text-field {
    grid-column: 1 / -1;
    grid-row: 1;
  }
  .ussd-send {
    grid-column: 2;
    grid-row: 2;
    --md-filled-button-leading-space: 20px;
    --md-filled-button-trailing-space: 20px;
  }
  .ussd-more-settings {
    grid-column: 1;
    grid-row: 2;
    justify-self: start;
    --md-text-button-leading-space: 8px;
    --md-text-button-trailing-space: 8px;
  }
  .ussd-session-strip {
    grid-column: 1 / -1;
    grid-row: 1;
    gap: 4px;
  }
  .ussd-session-state {
    gap: 6px;
    padding: 5px 8px;
    font-size: 11px;
  }
  .ussd-cancel {
    --md-text-button-leading-space: 8px;
    --md-text-button-trailing-space: 8px;
  }
  .ussd-session-active .ussd-command-line md-outlined-text-field {
    grid-row: 2;
  }
  .ussd-session-active .ussd-send,
  .ussd-session-active .ussd-more-settings {
    grid-row: 3;
  }
  .ussd-settings-reveal {
    grid-column: 1 / -1;
  }
  .ussd-timeout {
    width: 100%;
  }
}
</style>
