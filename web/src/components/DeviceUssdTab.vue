<script setup lang="ts">
import { fieldValue } from '../utils/material'
import { ref, computed } from 'vue'
import { devicesService } from '../services/devices'

const props = defineProps<{
  deviceId: string
  vowifiActive?: boolean
}>()

const ussdCmd = ref('')
const ussdTimeoutMs = ref(45000)
const sending = ref(false)
const sessionId = ref('')
const sessionChannel = ref('')
const history = ref<Array<{ ts: number; type: 'req' | 'res' | 'err' | 'sys'; content: string; dcs?: number; channel?: string }>>([])

const isMultiRound = computed(() => !!sessionId.value)
const inputPlaceholder = computed(() => isMultiRound.value ? '输入菜单选项数字' : '例如 *100# 或菜单回复数字')

async function sendUSSD() {
  const cmd = String(ussdCmd.value || '').trim()
  if (!cmd || sending.value) return
  
  history.value.push({ ts: Date.now(), type: 'req', content: cmd })
  sending.value = true
  ussdCmd.value = ''

  try {
    let d: { status?: number; text?: string; rawText?: string; dcs?: number; sessionId?: string; channel?: string }

    if (isMultiRound.value) {
      // 多轮模式：通过 continue 接口发送后续输入
      const result = await devicesService.continueUSSD(props.deviceId, {
        session_id: sessionId.value,
        input: cmd,
        timeout_ms: ussdTimeoutMs.value || 45000
      }, (ussdTimeoutMs.value || 45000) + 2000)
      if (!result.ok) throw new Error(result.error.message || '请求异常')
      d = result.data
    } else {
      // 首轮模式：发起新 USSD 请求
      const result = await devicesService.sendUSSD(props.deviceId, {
        command: cmd,
        timeout_ms: ussdTimeoutMs.value || 45000
      }, (ussdTimeoutMs.value || 45000) + 2000)
      if (!result.ok) throw new Error(result.error.message || '请求异常')
      d = result.data
    }

    // 更新通道和会话信息
    if (d.channel) sessionChannel.value = d.channel

    if (d.status === 5) {
      history.value.push({
        ts: Date.now(),
        type: 'err',
        content: `[网络不支持/无响应]\n` + (d.text || d.rawText || '[空响应]'),
        dcs: d.dcs,
        channel: d.channel
      })
      endSession()
    } else if (d.status === 2) {
      history.value.push({
        ts: Date.now(),
        type: 'err',
        content: `[被网络终止]\n` + (d.text || d.rawText || '[空响应]'),
        dcs: d.dcs,
        channel: d.channel
      })
      endSession()
    } else {
      history.value.push({
        ts: Date.now(),
        type: 'res',
        content: d.text || d.rawText || '[空响应]',
        dcs: d.dcs,
        channel: d.channel
      })
      // status=1 表示网络期望后续输入（多轮）
      if (d.status === 1 && d.sessionId) {
        sessionId.value = d.sessionId
      } else {
        endSession()
      }
    }
  } catch (e: unknown) {
    history.value.push({
      ts: Date.now(),
      type: 'err',
      content: e instanceof Error ? e.message : '请求异常'
    })
    endSession()
  } finally {
    sending.value = false
  }
}

async function cancelSession() {
  if (!sessionId.value) return
  try {
    await devicesService.cancelUSSD(props.deviceId, sessionId.value)
    history.value.push({
      ts: Date.now(),
      type: 'sys',
      content: '会话已手动取消'
    })
  } catch {
    // 忽略取消错误
  }
  endSession()
}

function endSession() {
  sessionId.value = ''
  sessionChannel.value = ''
}

function clearHistory() {
  history.value = []
  endSession()
}
</script>

<template>
  <section class="m3-stack" aria-label="USSD 终端">
    <div class="m3-row">
      <div>
        <h2 class="m3-title">USSD 交互终端</h2>
        <p class="m3-muted">发送代码并查看网络菜单响应</p>
      </div>
      <md-text-button v-if="isMultiRound" type="button" :disabled="sending" @click="cancelSession"
        >取消会话</md-text-button
      >
    </div>
    <p v-if="isMultiRound" class="m3-notice">
      多轮会话中 · {{ sessionChannel === 'vowifi' ? 'VoWiFi' : 'CS' }}
    </p>
    <div class="terminal-log" role="log" aria-label="USSD 会话记录" aria-live="polite">
      <p v-if="!history.length && !sending" class="m3-muted">暂无 USSD 会话记录</p>
      <div v-for="(msg, i) in history" :key="i" class="terminal-entry">
        <p class="m3-label">
          {{ new Date(msg.ts).toLocaleTimeString() }}
          <span v-if="msg.dcs !== undefined">· DCS {{ msg.dcs }}</span
          ><span v-if="msg.channel"> · {{ msg.channel === 'vowifi' ? 'VoWiFi' : 'CS' }}</span>
        </p>
        <pre
          :class="
            msg.type === 'req'
              ? 'terminal-command'
              : ['terminal-response', { 'm3-error': msg.type === 'err' }]
          "
          >{{ msg.content }}</pre>
      </div>
      <p v-if="sending" class="m3-muted" role="status">等待网络响应…</p>
    </div>
    <form class="m3-form" @submit.prevent="sendUSSD">
      <div class="m3-form-grid">
        <md-outlined-text-field
          name="ussd-command"
          :label="isMultiRound ? '菜单回复' : 'USSD 代码'"
          :placeholder="inputPlaceholder"
          :value="ussdCmd"
          :disabled="sending"
          @input="ussdCmd = fieldValue($event)"
        /><md-outlined-text-field
          label="超时（毫秒）"
          type="number"
          min="1"
          :value="String(ussdTimeoutMs)"
          :disabled="sending"
          @input="ussdTimeoutMs = Number(fieldValue($event))"
        />
      </div>
      <div class="m3-actions">
        <md-text-button type="button" :disabled="sending" @click="clearHistory"
          >清空记录</md-text-button
        ><md-filled-button type="submit" :disabled="sending || !ussdCmd.trim()">{{
          sending ? '发送中…' : isMultiRound ? '回复' : '发送'
        }}</md-filled-button>
      </div>
    </form>
  </section>
</template>
