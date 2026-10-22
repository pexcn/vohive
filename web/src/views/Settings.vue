<script setup lang="ts">
import { notify, requestConfirmation } from '../stores/feedback'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import { useSettingsStore } from '../stores/settings'
import PageHeader from '../components/PageHeader.vue'
import FieldRow from '../components/FieldRow.vue'
import { fieldValue, fieldSelected } from '../utils/material'
import { syncMaterialSwitch as vSyncSwitch } from '../directives/materialSwitch'
import SvgIcon from '../components/SvgIcon.vue'
import visibilityIcon from '@material-symbols/svg-400/outlined/visibility.svg?raw'
import hiddenIcon from '@material-symbols/svg-400/outlined/visibility_off.svg?raw'

const settingsStore = useSettingsStore()
const {
  systemInfo,
  loadingNotifications,
  savingNotifications,
  testingWebhook,
  testingBark,
  testingEmail,
  changingPassword,
  passwordForm,
  telegramForm,
  feishuForm,
  qqForm,
  webhookSettings,
  barkSettings,
  emailForm,
  pushplusForm
} = storeToRefs(settingsStore)
const activeNotifyTab = ref('telegram')

const hasValidWebhookURLs = computed(() => {
  if (!Array.isArray(webhookSettings.value.urls)) {
    return false
  }
  return webhookSettings.value.urls.some((u) => String(u || '').trim().length > 0)
})

const hasValidBarkURLs = computed(() => {
  if (!Array.isArray(barkSettings.value.urls)) {
    return false
  }
  return barkSettings.value.urls.some((u) => String(u || '').trim().length > 0)
})

const hasValidEmailConfig = computed(() => {
  return !!(
    emailForm.value.smtp_host &&
    emailForm.value.smtp_port &&
    emailForm.value.username &&
    emailForm.value.password &&
    emailForm.value.from_address &&
    emailForm.value.to_addresses
  )
})

async function changePassword() {
  if (changingPassword.value) return
  if (passwordForm.value.new_password !== passwordForm.value.confirm_password) {
    notify({ tone: 'error', message: '两次输入的新密码不一致' })
    return
  }

  try {
    const result = await settingsStore.changePasswordFromForm()
    if (!result.ok) throw new Error(result.error.message || '更新失败')
    notify({ tone: 'success', message: '密码已更新' })
    settingsStore.resetPasswordForm()
  } catch {
    notify({ tone: 'error', message: '失败：后端尚未实现该功能或请求失败' })
  }
}

async function loadSystemInfo() {
  const result = await settingsStore.fetchSystemInfo()
  if (!result.ok) {
    console.error('系统信息读取失败', result.error)
  }
}

async function loadNotifications() {
  try {
    const result = await settingsStore.fetchNotifications()
    if (!result.ok) throw new Error(result.error.message || '通知配置加载失败')
    syncWebhookHeaderRowsFromSettings()
  } catch {
    notify({ tone: 'error', message: '通知配置加载失败' })
  }
}

function openAPIDocs() {
  const docsURL = String(systemInfo.value.docs?.swagger_ui || '').trim()
  if (!docsURL) {
    notify({ tone: 'warning', message: 'API 文档入口暂不可用' })
    return
  }
  window.open(docsURL, '_blank', 'noopener,noreferrer')
}

async function saveNotifications() {
  if (loadingNotifications.value || savingNotifications.value) return
  try {
    const result = await settingsStore.saveNotificationsFromForms()
    if (!result.ok) throw new Error(result.error.message || '通知配置保存失败')
    const applied = result.data.applied
    const warning = result.data.warning
    if (applied === false && warning) {
      notify({ tone: 'warning', message: warning })
    } else {
      notify({
        tone: 'success',
        message: '通知配置已保存（已写入 config.yaml）'
      })
    }
  } catch (e: unknown) {
    notify({
      tone: 'error',
      message: e instanceof Error ? e.message : '通知配置保存失败'
    })
  }
}

async function testWebhookNotification() {
  try {
    const result = await settingsStore.testWebhookFromForm()
    if (!result.ok) {
      throw new Error(result.error.message || 'Webhook 测试失败')
    }
    const data = result.data
    if (data.ok) {
      notify({ tone: 'success', message: data.message || '测试通知已发送' })
      return
    }
    if (Array.isArray(data.failed_urls) && data.failed_urls.length > 0) {
      notify({
        tone: 'error',
        message: `${data.message}\n失败 URL: ${data.failed_urls.join(', ')}`
      })
      return
    }
    notify({ tone: 'error', message: data.message || 'Webhook 测试失败' })
  } catch (e: unknown) {
    notify({
      tone: 'error',
      message: e instanceof Error ? e.message : 'Webhook 测试失败'
    })
  }
}

function addWebhookUrl() {
  if (!webhookSettings.value.urls) {
    webhookSettings.value.urls = []
  }
  webhookSettings.value.urls.push('')
}

function removeWebhookUrl(index: number) {
  webhookSettings.value.urls.splice(index, 1)
}

// 自定义请求头以「行」形式编辑（rows 为唯一编辑源），保存时单向回写为 map。
// 受保护的系统头由后端强制覆盖。
const PROTECTED_WEBHOOK_HEADERS = new Set(['content-type', 'x-vohive-signature'])
// 常用请求头可从预设选择，也可编辑名称输入框。
const COMMON_WEBHOOK_HEADERS = [
  'Authorization',
  'X-Api-Key',
  'X-Auth-Token',
  'X-Webhook-Token',
  'X-Signature',
  'X-Request-Id',
  'Accept',
  'User-Agent'
]
// 每行带稳定 id，避免删除中间行后复用其它行的编辑状态。
let webhookHeaderUid = 0
const webhookHeaderRows = ref<{ id: number; key: string; value: string }[]>([])

// 加载完成后调用，把已保存的 headers map 转换为可编辑的行
function syncWebhookHeaderRowsFromSettings() {
  const headers = webhookSettings.value.headers || {}
  webhookHeaderRows.value = Object.entries(headers).map(([key, value]) => ({
    id: webhookHeaderUid++,
    key,
    value: String(value ?? '')
  }))
}

// 行变化时单向回写为 map（丢弃空 key 与受保护头）。无反向 watch，故不会回环。
watch(
  webhookHeaderRows,
  (rows) => {
    const map: Record<string, string> = {}
    for (const row of rows) {
      const key = String(row.key || '').trim()
      if (!key || PROTECTED_WEBHOOK_HEADERS.has(key.toLowerCase())) continue
      map[key] = String(row.value ?? '')
    }
    webhookSettings.value.headers = map
  },
  { deep: true }
)

function addWebhookHeader() {
  webhookHeaderRows.value.push({ id: webhookHeaderUid++, key: '', value: '' })
}

function removeWebhookHeader(index: number) {
  webhookHeaderRows.value.splice(index, 1)
}

async function testBarkNotification() {
  try {
    const result = await settingsStore.testBarkFromForm()
    if (!result.ok) {
      throw new Error(result.error.message || 'Bark 测试失败')
    }
    const data = result.data
    if (data.ok) {
      notify({ tone: 'success', message: data.message || '测试通知已发送' })
      return
    }
    if (Array.isArray(data.failed_urls) && data.failed_urls.length > 0) {
      notify({
        tone: 'error',
        message: `${data.message}\n失败 URL: ${data.failed_urls.join(', ')}`
      })
      return
    }
    notify({ tone: 'error', message: data.message || 'Bark 测试失败' })
  } catch (e: unknown) {
    notify({
      tone: 'error',
      message: e instanceof Error ? e.message : 'Bark 测试失败'
    })
  }
}

async function testEmailNotification() {
  try {
    const result = await settingsStore.testEmailFromForm()
    if (!result.ok) {
      throw new Error(result.error.message || 'Email 测试失败')
    }
    const data = result.data
    if (data.ok) {
      notify({ tone: 'success', message: data.message || '测试邮件已发送' })
      return
    }
    notify({ tone: 'error', message: data.message || 'Email 测试失败' })
  } catch (e: unknown) {
    notify({
      tone: 'error',
      message: e instanceof Error ? e.message : 'Email 测试失败'
    })
  }
}

function addBarkUrl() {
  if (!barkSettings.value.urls) {
    barkSettings.value.urls = []
  }
  barkSettings.value.urls.push('')
}

function removeBarkUrl(index: number) {
  barkSettings.value.urls.splice(index, 1)
}

watch(
  () => emailForm.value.smtp_port,
  (newPort) => {
    if (Number(newPort) === 465) {
      emailForm.value.use_ssl = true
    }
  }
)

import { systemService, type UpdateInfo } from '../services/system'

const checkingUpdate = ref(false)
const applyingUpdate = ref(false)
const updateInfo = ref<UpdateInfo | null>(null)

async function doCheckUpdate() {
  checkingUpdate.value = true
  try {
    const res = await systemService.checkUpdate()
    if (!res.ok) throw new Error(res.error.message || '检查更新失败')
    updateInfo.value = res.data
    if (!res.data.has_update) {
      notify({ tone: 'success', message: '当前已是最新版本' })
    }
  } catch (e: any) {
    notify({ tone: 'error', message: e.message || '检查更新失败' })
  } finally {
    checkingUpdate.value = false
  }
}

async function doApplyUpdate() {
  if (!updateInfo.value) return

  if (updateInfo.value.is_docker) {
    void requestConfirmation({
      title: 'Docker 环境更新',
      message: '请通过拉取最新镜像并重启容器完成升级。容器内不执行程序文件热替换。',
      alert: true
    })
    return
  }

  try {
    await requestConfirmation({
      title: '应用更新',
      message: `最新版本：${updateInfo.value.latest_version}。确定更新并重启服务吗？`,
      details: updateInfo.value.release_note,
      confirmLabel: '立即更新',
      danger: true
    })
    applyingUpdate.value = true
    const res = await systemService.applyUpdate()
    if (!res.ok) throw new Error(res.error.message || '请求应用更新失败')
    notify({ tone: 'success', message: res.data?.message || '正在更新...' })
    setTimeout(() => {
      window.location.reload()
    }, 5000)
  } catch (e: any) {
    if (e !== 'cancel') {
      notify({ tone: 'error', message: e.message || '应用更新失败' })
    }
  } finally {
    applyingUpdate.value = false
  }
}

onMounted(() => {
  loadNotifications()
  loadSystemInfo()
})

const passwordFields = [
  { key: 'old_password', label: '当前密码' },
  { key: 'new_password', label: '新密码' },
  { key: 'confirm_password', label: '确认新密码' }
] as const
const revealed = reactive(new Set<string>())
function toggleSecret(key: string) {
  if (revealed.has(key)) revealed.delete(key)
  else revealed.add(key)
}
type SettingField = {
  key: string
  label: string
  type?: 'number' | 'password' | 'select' | 'textarea'
  hint?: string
  options?: string[]
  min?: string
  max?: string
}
const channels: Array<{ key: string; label: string; fields: SettingField[] }> = [
  {
    key: 'telegram',
    label: 'Telegram Bot',
    fields: [
      { key: 'bot_token', label: 'Bot Token', type: 'password' },
      { key: 'chat_id', label: 'Chat ID', type: 'number' },
      { key: 'admin_id', label: '管理员 ID', type: 'number' },
      { key: 'base_url', label: 'API 地址', hint: '留空直连 api.telegram.org；也可填写反代地址' },
      { key: 'proxy', label: 'HTTP 代理（可选）' }
    ]
  },
  {
    key: 'feishu',
    label: '飞书 Bot',
    fields: [
      { key: 'app_id', label: 'App ID' },
      { key: 'app_secret', label: 'App Secret', type: 'password' },
      { key: 'chat_ids', label: '群组 ID', hint: '多个群组以英文逗号分隔' }
    ]
  },
  {
    key: 'qq',
    label: 'QQ Bot',
    fields: [
      { key: 'app_id', label: 'App ID' },
      { key: 'app_secret', label: 'App Secret', type: 'password' },
      { key: 'group_ids', label: '群聊 OpenID', hint: '多个群聊以英文逗号分隔' },
      { key: 'direct_ids', label: '用户 OpenID', hint: '多个用户以英文逗号分隔' }
    ]
  },
  {
    key: 'bark',
    label: 'Bark',
    fields: [
      { key: 'group', label: '通知分组' },
      {
        key: 'level',
        label: '通知级别',
        type: 'select',
        options: ['timeSensitive', 'active', 'passive']
      },
      { key: 'icon', label: '图标 URL（可选）' }
    ]
  },
  {
    key: 'email',
    label: 'Email',
    fields: [
      { key: 'smtp_host', label: 'SMTP 主机' },
      { key: 'smtp_port', label: 'SMTP 端口', type: 'number', min: '1', max: '65535' },
      { key: 'username', label: '账号' },
      { key: 'password', label: '密码或授权码', type: 'password' },
      { key: 'from_address', label: '发件地址' },
      { key: 'to_addresses', label: '收件地址', hint: '多个收件人以英文逗号分隔' }
    ]
  },
  {
    key: 'pushplus',
    label: 'Pushplus',
    fields: [
      { key: 'token', label: 'Token', type: 'password' },
      { key: 'topic', label: '群组编码（可选）' },
      {
        key: 'channel',
        label: '通知渠道',
        type: 'select',
        options: ['wechat', 'webhook', 'cp', 'mail']
      }
    ]
  },
  {
    key: 'webhook',
    label: 'Webhook',
    fields: [
      { key: 'secret', label: '签名密钥（可选）', type: 'password' },
      { key: 'text_template', label: '文本模板', type: 'textarea' },
      { key: 'timeout_ms', label: '超时（毫秒）', type: 'number', min: '0' },
      { key: 'retry_max', label: '最大重试次数', type: 'number', min: '0', max: '10' }
    ]
  }
]
const channelForms = computed(
  () =>
    ({
      telegram: telegramForm.value,
      feishu: feishuForm.value,
      qq: qqForm.value,
      bark: barkSettings.value,
      email: emailForm.value,
      pushplus: pushplusForm.value,
      webhook: webhookSettings.value
    }) as unknown as Record<string, Record<string, unknown>>
)
const activeChannel = computed(() =>
  channels.find((channel) => channel.key === activeNotifyTab.value)!
)
const activeForm = computed(() => channelForms.value[activeNotifyTab.value]!)
const notificationBusy = computed(() => loadingNotifications.value || savingNotifications.value)
const testing = computed(() => testingWebhook.value || testingBark.value || testingEmail.value)
const canTest = computed(
  () =>
    !!activeForm.value.enabled &&
    (activeNotifyTab.value === 'webhook'
      ? hasValidWebhookURLs.value
      : activeNotifyTab.value === 'bark'
        ? hasValidBarkURLs.value
        : activeNotifyTab.value === 'email' && hasValidEmailConfig.value)
)
function updateField(field: SettingField, event: Event) {
  const value = fieldValue(event)
  activeForm.value[field.key] =
    field.type === 'number' ? (value === '' ? null : Number(value)) : value
}
function testActiveChannel() {
  if (testing.value || !canTest.value) return
  if (activeNotifyTab.value === 'webhook') void testWebhookNotification()
  else if (activeNotifyTab.value === 'bark') void testBarkNotification()
  else if (activeNotifyTab.value === 'email') void testEmailNotification()
}
watch(activeNotifyTab, () => revealed.clear())
</script>

<template>
  <div class="m3-page settings-page">
    <PageHeader title="系统设置" subtitle="账号安全、通知渠道与系统信息" />
    <div class="settings-top">
      <section class="m3-card m3-stack">
        <h2 class="m3-title">账号安全</h2>
        <form class="m3-form" @submit.prevent="changePassword">
          <md-outlined-text-field
            v-for="field in passwordFields"
            :key="field.key"
            :name="field.key"
            :label="field.label"
            :type="revealed.has(field.key) ? 'text' : 'password'"
            :autocomplete="field.key === 'old_password' ? 'current-password' : 'new-password'"
            required
            :disabled="changingPassword"
            :value="passwordForm[field.key]"
            @input="passwordForm[field.key] = fieldValue($event)"
            ><md-icon-button
              type="button"
              slot="trailing-icon"
              :aria-label="revealed.has(field.key) ? '隐藏密码' : '显示密码'"
              @click="toggleSecret(field.key)"
              ><SvgIcon
                :svg="revealed.has(field.key) ? hiddenIcon : visibilityIcon" /></md-icon-button
          ></md-outlined-text-field>
          <div class="m3-actions">
            <md-filled-button type="submit" :disabled="changingPassword">{{
              changingPassword ? '更新中…' : '更新密码'
            }}</md-filled-button>
          </div>
        </form>
      </section>
      <section class="m3-card m3-stack">
        <h2 class="m3-title">系统信息</h2>
        <FieldRow label="版本" :value="systemInfo.version" monospace /><FieldRow
          label="构建时间"
          :value="systemInfo.build_time"
          monospace
        /><FieldRow label="配置路径" :value="systemInfo.config" monospace copyable /><FieldRow
          label="交流群"
          value="https://t.me/vohive"
          copyable
        />
        <div class="m3-actions">
          <md-outlined-button :disabled="checkingUpdate || applyingUpdate" @click="doCheckUpdate">{{
            checkingUpdate ? '检查中…' : '检查更新'
          }}</md-outlined-button
          ><md-text-button :disabled="!systemInfo.docs?.swagger_ui" @click="openAPIDocs"
            >API 文档</md-text-button
          >
        </div>
        <div v-if="updateInfo?.has_update" class="m3-inset m3-stack">
          <p>发现新版本 {{ updateInfo.latest_version }}</p>
          <pre class="release-notes">{{ updateInfo.release_note }}</pre>
          <md-filled-button :disabled="applyingUpdate" @click="doApplyUpdate">{{
            applyingUpdate ? '更新中…' : '更新程序'
          }}</md-filled-button>
        </div>
        <div class="m3-actions">
          <a
            v-if="systemInfo.docs?.openapi_yaml"
            :href="systemInfo.docs.openapi_yaml"
            target="_blank"
            rel="noopener noreferrer"
            >OpenAPI YAML</a
          ><a
            v-if="systemInfo.docs?.openapi_json"
            :href="systemInfo.docs.openapi_json"
            target="_blank"
            rel="noopener noreferrer"
            >OpenAPI JSON</a
          >
        </div>
      </section>
    </div>
    <section class="m3-card m3-stack" aria-label="通知设置">
      <div class="m3-row">
        <div>
          <h2 class="m3-title">通知渠道</h2>
          <p class="m3-muted">设置会统一保存；测试通知仅在点击时发送。</p>
        </div>
        <md-filled-button :disabled="notificationBusy" @click="saveNotifications">{{
          savingNotifications ? '保存中…' : '保存通知设置'
        }}</md-filled-button>
      </div>
      <div class="m3-form">
        <md-outlined-select
          label="通知渠道"
          name="notification-channel"
          :value="activeNotifyTab"
          @change="activeNotifyTab = fieldValue($event)"
          ><md-select-option
            v-for="channel in channels"
            :key="channel.key"
            :value="channel.key"
            :selected="activeNotifyTab === channel.key"
            ><div slot="headline">{{ channel.label }}</div></md-select-option
          ></md-outlined-select
        >
        <label class="m3-switch-row"
          ><span>启用 {{ activeChannel.label }}</span
          ><md-switch
            v-sync-switch
            :aria-label="`启用 ${activeChannel.label}`"
            :selected="!!activeForm.enabled"
            :disabled="notificationBusy"
            @change="activeForm.enabled = fieldSelected($event)"
        /></label>
        <div v-if="activeNotifyTab === 'bark' || activeNotifyTab === 'webhook'" class="m3-stack">
          <div class="m3-row">
            <h3>目标 URL</h3>
            <md-text-button
              :disabled="!activeForm.enabled || notificationBusy"
              @click="activeNotifyTab === 'bark' ? addBarkUrl() : addWebhookUrl()"
              >添加 URL</md-text-button
            >
          </div>
          <p
            v-if="!(activeNotifyTab === 'bark' ? barkSettings.urls : webhookSettings.urls).length"
            class="m3-muted"
          >
            尚未配置目标地址
          </p>
          <div
            v-for="(_url, index) in activeNotifyTab === 'bark'
              ? barkSettings.urls
              : webhookSettings.urls"
            :key="activeNotifyTab + index"
            class="settings-list-row"
          >
            <md-outlined-text-field
              :label="`URL ${index + 1}`"
              :value="String((activeForm.urls as string[])[index] || '')"
              :disabled="!activeForm.enabled || notificationBusy"
              @input="(activeForm.urls as string[])[index] = fieldValue($event)"
            /><md-text-button
              class="danger"
              :disabled="!activeForm.enabled || notificationBusy"
              @click="activeNotifyTab === 'bark' ? removeBarkUrl(index) : removeWebhookUrl(index)"
              >删除</md-text-button
            >
          </div>
        </div>
        <div :key="activeNotifyTab" class="m3-form-grid">
          <template v-for="field in activeChannel.fields" :key="field.key">
            <md-outlined-select
              v-if="field.type === 'select'"
              :label="field.label"
              :name="field.key"
              :value="String(activeForm[field.key] ?? '')"
              :disabled="!activeForm.enabled || notificationBusy"
              @change="updateField(field, $event)"
              ><md-select-option
                v-for="option in field.options"
                :key="option"
                :value="option"
                :selected="activeForm[field.key] === option"
                ><div slot="headline">{{ option }}</div></md-select-option
              ></md-outlined-select
            >
            <md-outlined-text-field
              v-else
              :label="field.label"
              :name="field.key"
              :type="
                field.type === 'password'
                  ? revealed.has(field.key)
                    ? 'text'
                    : 'password'
                  : field.type || 'text'
              "
              :min="field.min"
              :max="field.max"
              :supporting-text="field.hint"
              :rows="field.type === 'textarea' ? 4 : undefined"
              :value="String(activeForm[field.key] ?? '')"
              :disabled="!activeForm.enabled || notificationBusy"
              @input="updateField(field, $event)"
              ><md-icon-button
                v-if="field.type === 'password'"
                slot="trailing-icon"
                type="button"
                :aria-label="revealed.has(field.key) ? '隐藏密码' : '显示密码'"
                :disabled="!activeForm.enabled"
                @click="toggleSecret(field.key)"
                ><SvgIcon
                  :svg="revealed.has(field.key) ? hiddenIcon : visibilityIcon" /></md-icon-button
            ></md-outlined-text-field>
          </template>
        </div>
        <label v-if="activeNotifyTab === 'email'" class="m3-switch-row"
          ><span>SSL 连接<small>选择 465 端口时自动开启</small></span
          ><md-switch
            v-sync-switch
            aria-label="SMTP SSL"
            :selected="emailForm.use_ssl"
            :disabled="!emailForm.enabled || notificationBusy"
            @change="emailForm.use_ssl = fieldSelected($event)"
        /></label>
        <section v-if="activeNotifyTab === 'webhook'" class="m3-stack">
          <div class="m3-row">
            <h3>自定义请求头</h3>
            <md-text-button
              :disabled="!webhookSettings.enabled || notificationBusy"
              @click="addWebhookHeader"
              >添加请求头</md-text-button
            >
          </div>
          <p class="m3-muted">Content-Type 与 X-VoHive-Signature 由系统管理，不接受自定义覆盖。</p>
          <div v-for="(row, index) in webhookHeaderRows" :key="row.id" class="m3-inset m3-stack">
            <div class="m3-form-grid">
              <md-outlined-select
                label="常用请求头"
                :value="COMMON_WEBHOOK_HEADERS.includes(row.key) ? row.key : ''"
                :disabled="!webhookSettings.enabled || notificationBusy"
                @change="row.key = fieldValue($event)"
                ><md-select-option value="" :selected="!COMMON_WEBHOOK_HEADERS.includes(row.key)"
                  ><div slot="headline">自定义</div></md-select-option
                ><md-select-option
                  v-for="name in COMMON_WEBHOOK_HEADERS"
                  :key="name"
                  :value="name"
                  :selected="row.key === name"
                  ><div slot="headline">{{ name }}</div></md-select-option
                ></md-outlined-select
              ><md-outlined-text-field
                label="名称"
                :value="row.key"
                :disabled="!webhookSettings.enabled || notificationBusy"
                @input="row.key = fieldValue($event)"
              /><md-outlined-text-field
                label="值"
                :value="row.value"
                :disabled="!webhookSettings.enabled || notificationBusy"
                @input="row.value = fieldValue($event)"
              />
            </div>
            <md-text-button
              class="danger"
              :disabled="!webhookSettings.enabled || notificationBusy"
              @click="removeWebhookHeader(index)"
              >删除请求头</md-text-button
            >
          </div>
        </section>
        <div v-if="['bark', 'webhook', 'email'].includes(activeNotifyTab)" class="m3-actions">
          <md-outlined-button
            :disabled="!canTest || testing || notificationBusy"
            @click="testActiveChannel"
            >{{ testing ? '测试中…' : '发送测试通知' }}</md-outlined-button
          >
        </div>
      </div>
    </section>
  </div>
</template>
<style scoped>
.settings-top {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 320px), 1fr));
  gap: 24px;
}
.settings-list-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.settings-list-row md-outlined-text-field {
  flex: 1;
}
.release-notes {
  max-height: 180px;
  overflow: auto;
  font: 12px/20px var(--vh-typeface-mono);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
a {
  color: var(--md-sys-color-primary);
}
h3 {
  font-size: 16px;
  font-weight: 500;
}
</style>
