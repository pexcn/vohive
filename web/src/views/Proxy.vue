<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import PageHeader from '../components/PageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import ListSkeleton from '../components/ListSkeleton.vue'
import ErrorState from '../components/ErrorState.vue'
import { usePollingScheduler } from '../composables/usePollingScheduler'
import { useProxyStore } from '../stores/proxy'
import { useUpstreamProxyStore } from '../stores/upstream-proxy'
import type { ProxyInstance, ProxyDevice, ProxyMode, UpstreamProxy, UpstreamProxyCountry } from '../types/api'
import { toAppError } from '../services/http'
import {
  upstreamProxyAddressWarning,
  upstreamProxyIPv6AddressHint
} from '../utils/upstreamProxyAddress'
import SvgIcon from '../components/SvgIcon.vue'
import RefreshButton from '../components/RefreshButton.vue'
import { notify as showNotice } from '../stores/feedback'
import StatusLight from '../components/StatusLight.vue'
import { syncMaterialSwitch as vSyncSwitch } from '../directives/materialSwitch'
import addIcon from '@material-symbols/svg-400/outlined/add.svg?raw'
import playIcon from '@material-symbols/svg-400/outlined/play_arrow.svg?raw'
import stopIcon from '@material-symbols/svg-400/outlined/stop.svg?raw'
import refreshIcon from '@material-symbols/svg-400/outlined/refresh.svg?raw'
import editIcon from '@material-symbols/svg-400/outlined/edit.svg?raw'
import deleteIcon from '@material-symbols/svg-400/outlined/delete.svg?raw'
import routerIcon from '@material-symbols/svg-400/outlined/router.svg?raw'
import linkIcon from '@material-symbols/svg-400/outlined/link.svg?raw'
import earthIcon from '@material-symbols/svg-400/outlined/language.svg?raw'
import visibilityIcon from '@material-symbols/svg-400/outlined/visibility.svg?raw'
import hiddenIcon from '@material-symbols/svg-400/outlined/visibility_off.svg?raw'

const formError = ref('')
function showError(message: string) {
  formError.value = message
  showNotice({ message })
}
function notify(message: string) {
  showNotice({ message })
}
function fieldValue(event: Event) {
  return (event.target as HTMLInputElement).value
}
function fieldSelected(event: Event) {
  return (event.target as HTMLElement & { selected: boolean }).selected
}
function guardClose(event: Event, busy: boolean) {
  if (busy) event.preventDefault()
}
function changeTab(event: Event) {
  const tab = (event.target as HTMLElement & { activeTab?: HTMLElement }).activeTab?.dataset.tab
  if (tab === 'upstream' || tab === 'outbound') activeTab.value = tab
}
function refreshActive() {
  if (activeTab.value === 'upstream') void fetchUpstream()
  else void fetchOverview()
}
const confirmation = ref<{ title: string; message: string; run: () => Promise<boolean> } | null>(null)
const confirmBusy = ref(false)
async function runConfirmation() {
  if (!confirmation.value || confirmBusy.value) return
  confirmBusy.value = true
  formError.value = ''
  try {
    if (await confirmation.value.run()) confirmation.value = null
  } finally {
    confirmBusy.value = false
  }
}
const instanceAction = ref('')
const instanceLoading = ref(false)
let instanceEditGeneration = 0
const upstreamSaving = ref(false)
const countryQuery = ref('')
const showInstancePassword = ref(false)
const showUpstreamPassword = ref(false)

// ── Tab 控制 ──
const activeTab = ref('upstream') // 默认展示前置代理

// ══════════════════════════════════════════════════════
// 本地出站代理
// ══════════════════════════════════════════════════════
const proxyStore = useProxyStore()
const { statusMap } = storeToRefs(proxyStore)

const initialLoading = ref(true)
const refreshing = ref(false)
const loadError = ref<{ message: string; status?: number } | null>(null)
const instances = ref<ProxyInstance[]>([])
const devices = ref<ProxyDevice[]>([])
const saving = ref(false)

const drawerOpen = ref(false)
const editingInstance = ref<ProxyInstance | null>(null)
const instanceForm = ref<ProxyInstance>({
  id: '',
  name: '',
  device_id: '',
  enabled: true,
  mode: 'socks5',
  listen_addr: '0.0.0.0',
  listen_port: 1080,
  auth_enabled: false,
  username: '',
  password: ''
})

const modeOptions: Array<{ label: string; value: ProxyMode }> = [
  { label: 'SOCKS5', value: 'socks5' },
  { label: 'HTTP', value: 'http' }
]

const instancesWithStatus = computed(() => {
  return instances.value.map((inst) => ({
    ...inst,
    status: statusMap.value[inst.id] || { id: inst.id, running: false }
  }))
})

watch(
  () => instanceForm.value.auth_enabled,
  (enabled) => {
    if (!enabled) {
      instanceForm.value.username = ''
      instanceForm.value.password = ''
    }
  }
)

async function fetchOverview(opts: { silent?: boolean; initial?: boolean } = {}) {
  const isInitial = opts.initial === true
  const silent = opts.silent === true
  if (isInitial) {
    initialLoading.value = true
  } else if (!silent) {
    refreshing.value = true
  }
  loadError.value = null

  try {
    const result = await proxyStore.fetchOverview()
    if (!result.ok) throw new Error(result.error.message)
    instances.value = proxyStore.instances.map((inst) => ({
      ...inst,
      mode: inst.mode || 'socks5'
    }))
    devices.value = proxyStore.devices
  } catch (e: unknown) {
    const err = toAppError(e)
    loadError.value = {
      message: err.message || '加载代理配置失败',
      status: err.status
    }
  } finally {
    if (isInitial) {
      initialLoading.value = false
    } else if (!silent) {
      refreshing.value = false
    }
  }
}

async function saveConfig(nextInstances: ProxyInstance[] = instances.value) {
  if (saving.value) return false
  saving.value = true
  try {
    const result = await proxyStore.saveConfig(nextInstances)
    if (!result.ok) throw new Error(result.error.message || '保存失败')
    notify('配置已保存')
    await fetchOverview()
    return true
  } catch (e: unknown) {
    const err = toAppError(e)
    showError(err.message || '保存失败')
    return false
  } finally {
    saving.value = false
  }
}

async function startInstance(id: string) {
  if (instanceAction.value || saving.value) return
  instanceAction.value = id
  try {
    const result = await proxyStore.startInstance(id)
    if (!result.ok) throw new Error(result.error.message || '启动失败')
    notify('已启动')
    await fetchOverview()
  } catch (e: unknown) {
    const err = toAppError(e)
    showError(err.message || '启动失败')
  } finally {
    instanceAction.value = ''
  }
}

async function stopInstance(id: string) {
  if (instanceAction.value || saving.value) return
  instanceAction.value = id
  try {
    const result = await proxyStore.stopInstance(id)
    if (!result.ok) throw new Error(result.error.message || '停止失败')
    notify('已停止')
    await fetchOverview()
  } catch (e: unknown) {
    const err = toAppError(e)
    showError(err.message || '停止失败')
  } finally {
    instanceAction.value = ''
  }
}

async function restartInstance(id: string) {
  if (instanceAction.value || saving.value) return
  instanceAction.value = id
  try {
    const result = await proxyStore.restartInstance(id)
    if (!result.ok) throw new Error(result.error.message || '重启失败')
    notify('已重启')
    await fetchOverview()
  } catch (e: unknown) {
    const err = toAppError(e)
    showError(err.message || '重启失败')
  } finally {
    instanceAction.value = ''
  }
}

function resetForm() {
  instanceForm.value = {
    id: '',
    name: '',
    device_id: devices.value[0]?.id || '',
    enabled: true,
    mode: 'socks5',
    listen_addr: '0.0.0.0',
    listen_port: 1080,
    auth_enabled: false,
    username: '',
    password: ''
  }
}

async function openDrawer(inst?: ProxyInstance) {
  showInstancePassword.value = false
  formError.value = ''
  const generation = ++instanceEditGeneration
  instanceLoading.value = false
  if (!inst) {
    editingInstance.value = null
    resetForm()
    instanceForm.value.id = `proxy-${Date.now()}`
    instanceForm.value.listen_port = 10800 + instances.value.length
    drawerOpen.value = true
    return
  }

  editingInstance.value = inst
  instanceForm.value = { ...inst, mode: inst.mode || 'socks5' }
  drawerOpen.value = true

  instanceLoading.value = true
  try {
    const result = await proxyStore.fetchInstance(inst.id)
    if (generation !== instanceEditGeneration || !drawerOpen.value) return
    if (result.ok) {
      instanceForm.value = { ...result.data, mode: result.data.mode || 'socks5' }
    }
  } catch {
    if (generation === instanceEditGeneration && drawerOpen.value)
      showError('读取完整实例配置失败，已使用概览数据')
  } finally {
    if (generation === instanceEditGeneration) instanceLoading.value = false
  }
}

async function saveForm() {
  formError.value = ''
  if (saving.value || instanceLoading.value) return
  const form = { ...instanceForm.value }

  if (!form.id) {
    showError('实例 ID 不能为空')
    return
  }
  if (!form.device_id) {
    showError('必须绑定设备')
    return
  }
  if (form.mode !== 'socks5' && form.mode !== 'http') {
    showError('代理模式仅支持 SOCKS5 或 HTTP')
    return
  }
  if (!Number.isInteger(form.listen_port) || form.listen_port <= 0 || form.listen_port > 65535) {
    showError('监听端口无效')
    return
  }
  if (!form.listen_addr) {
    form.listen_addr = '0.0.0.0'
  }

  if (form.auth_enabled) {
    form.username = (form.username || '').trim()
    form.password = (form.password || '').trim()
    if (!form.username || !form.password) {
      showError('启用认证时必须填写用户名和密码')
      return
    }
  } else {
    form.username = ''
    form.password = ''
  }

  const next = instances.value.slice()
  if (editingInstance.value) {
    const index = next.findIndex((instance) => instance.id === editingInstance.value!.id)
    if (index >= 0) next[index] = form
  } else {
    if (next.some((instance) => instance.id === form.id)) {
      showError('实例 ID 已存在')
      return
    }
    next.push(form)
  }
  if (await saveConfig(next)) drawerOpen.value = false
}

function deleteInstance(id: string) {
  formError.value = ''
  confirmation.value = {
    title: '删除代理实例',
    message: `确定删除实例 ${id}？`,
    run: () => saveConfig(instances.value.filter((instance) => instance.id !== id))
  }
}

function formatModeLabel(mode: string | undefined) {
  return mode === 'http' ? 'HTTP' : 'SOCKS5'
}

const pollEnabled = computed(() => !initialLoading.value && instances.value.length > 0)
usePollingScheduler(() => fetchOverview({ silent: true }), 5000, {
  enabled: pollEnabled,
  maxIntervalMs: 60000,
  backgroundIntervalMs: 15000
})


// ══════════════════════════════════════════════════════
// 漫游前置代理
// ══════════════════════════════════════════════════════
const upstreamStore = useUpstreamProxyStore()

const upstreamLoading = ref(true)
const upstreamRefreshing = ref(false)
const upstreamError = ref<{ message: string; status?: number } | null>(null)

// ── 编辑 Drawer ──
const upstreamDrawerOpen = ref(false)
const editingUpstream = ref<UpstreamProxy | null>(null)
const upstreamForm = ref<UpstreamProxy>({
  id: '',
  name: '',
  addr: '',
  username: '',
  password: '',
  enabled: true
})

// ── 国家规则管理 Drawer ──
const countryRuleDrawerOpen = ref(false)
const countryRuleTargetProxy = ref<UpstreamProxy | null>(null)
const selectedCountryCode = ref('')

const availableCountries = computed(() => {
  if (!countryRuleTargetProxy.value) return []
  return upstreamStore.countries.filter((country) => {
    const rule = upstreamStore.getRuleForCountry(country.country_code)
    return !rule || rule.upstream_proxy_id === countryRuleTargetProxy.value!.id
  })
})

const currentProxyCountryRules = computed(() => {
  if (!countryRuleTargetProxy.value) return []
  return upstreamStore.getRulesForProxy(countryRuleTargetProxy.value.id)
})

// 前置代理列表（带国家规则数量）
const upstreamProxiesWithRuleCount = computed(() => {
  return upstreamStore.proxies.map(p => ({
    ...p,
    ruleCount: upstreamStore.getRulesForProxy(p.id).length
  }))
})

async function fetchUpstream(opts: { silent?: boolean; initial?: boolean } = {}) {
  const isInitial = opts.initial === true
  const silent = opts.silent === true
  if (isInitial) {
    upstreamLoading.value = true
  } else if (!silent) {
    upstreamRefreshing.value = true
  }
  upstreamError.value = null

  try {
    const result = await upstreamStore.fetchAll()
    if (!result.ok) throw new Error(result.error.message)
  } catch (e: unknown) {
    const err = toAppError(e)
    upstreamError.value = {
      message: err.message || '加载前置代理失败',
      status: err.status
    }
  } finally {
    if (isInitial) {
      upstreamLoading.value = false
    } else if (!silent) {
      upstreamRefreshing.value = false
    }
  }
}

function openUpstreamDrawer(proxy?: UpstreamProxy) {
  showUpstreamPassword.value = false
  formError.value = ''
  if (!proxy) {
    editingUpstream.value = null
    upstreamForm.value = {
      id: '',
      name: '',
      addr: '',
      username: '',
      password: '',
      enabled: true
    }
  } else {
    editingUpstream.value = proxy
    upstreamForm.value = { ...proxy }
    // 密码脱敏时清空，让用户重新输入
    if (upstreamForm.value.password === '****') {
      upstreamForm.value.password = ''
    }
  }
  upstreamDrawerOpen.value = true
}

async function saveUpstreamForm() {
  formError.value = ''
  if (upstreamSaving.value) return
  const form = { ...upstreamForm.value }
  form.id = (form.id || '').trim()
  form.name = (form.name || '').trim()
  form.addr = (form.addr || '').trim()

  if (!form.id) {
    showError('ID 不能为空')
    return
  }
  if (!form.addr) {
    showError('Socks5 地址不能为空')
    return
  }
  const addrWarning = upstreamProxyAddressWarning(form.addr)
  if (addrWarning) {
    showError(addrWarning)
    return
  }

  upstreamSaving.value = true
  try {
    if (editingUpstream.value) {
      // 更新
      const result = await upstreamStore.updateProxy(form.id, form)
      if (!result.ok) throw new Error(result.error.message || '更新失败')
      notify('前置代理已更新，并通过连通性探测')
    } else {
      // 新增
      const result = await upstreamStore.createProxy(form)
      if (!result.ok) throw new Error(result.error.message || '创建失败')
      notify('前置代理已创建，并通过连通性探测')
    }
    upstreamDrawerOpen.value = false
    await fetchUpstream()
  } catch (e: unknown) {
    const err = toAppError(e)
    showError(err.message || '保存失败')
  } finally {
    upstreamSaving.value = false
  }
}

function deleteUpstream(proxy: UpstreamProxy) {
  formError.value = ''
  confirmation.value = {
    title: '删除前置代理',
    message: `确定删除「${proxy.name || proxy.id}」？\n绑定的国家规则将一并删除，相关国家会恢复直连。`,
    run: async () => {
      upstreamSaving.value = true
      try {
        const result = await upstreamStore.deleteProxy(proxy.id)
        if (!result.ok) throw new Error(result.error.message || '删除失败')
        notify('前置代理已删除')
        await fetchUpstream()
        return true
      } catch (error) {
        showError(toAppError(error).message || '删除失败')
        return false
      } finally {
        upstreamSaving.value = false
      }
    }
  }
}

function openCountryRuleDrawer(proxy: UpstreamProxy) {
  formError.value = ''
  countryRuleTargetProxy.value = proxy
  selectedCountryCode.value = ''
  countryQuery.value = ''
  countryRuleDrawerOpen.value = true
}

async function doUpsertCountryRule() {
  formError.value = ''
  if (upstreamSaving.value) return
  if (!countryRuleTargetProxy.value || !selectedCountryCode.value) {
    showError('请选择国家')
    return
  }

  upstreamSaving.value = true
  try {
    const result = await upstreamStore.upsertCountryRule(selectedCountryCode.value, {
      upstream_proxy_id: countryRuleTargetProxy.value.id,
      enabled: true
    })
    if (!result.ok) throw new Error(result.error.message || '保存规则失败')
    notify('国家规则已保存')
    selectedCountryCode.value = ''
    await fetchUpstream()
  } catch (e: unknown) {
    const err = toAppError(e)
    showError(err.message || '保存规则失败')
  } finally {
    upstreamSaving.value = false
  }
}

function doDeleteCountryRule(countryCode: string) {
  formError.value = ''
  confirmation.value = {
    title: '删除国家规则',
    message: `删除 ${countryCode} 规则后，该国家将默认直连。重启 VoWiFi 后生效。`,
    run: async () => {
      upstreamSaving.value = true
      try {
        const result = await upstreamStore.deleteCountryRule(countryCode)
        if (!result.ok) throw new Error(result.error.message || '删除规则失败')
        notify('国家规则已删除，该国家将默认直连')
        await fetchUpstream()
        return true
      } catch (error) {
        showError(toAppError(error).message || '删除规则失败')
        return false
      } finally {
        upstreamSaving.value = false
      }
    }
  }
}

const filteredCountries = computed(() => {
  const query = countryQuery.value.trim().toLowerCase()
  return availableCountries.value.filter(country => formatCountryLabel(country).toLowerCase().includes(query))
})

function formatCountryLabel(country: UpstreamProxyCountry): string {
  const name = country.country_name || country.country_code
  const mccs = country.mccs?.length ? ` · MCC ${country.mccs.join('/')}` : ''
  return `${country.country_code} · ${name}${mccs}`
}

// ── 统一初始化 ──
onMounted(() => {
  fetchOverview({ initial: true })
  fetchUpstream({ initial: true })
})

// 前置代理轮询
const upPollEnabled = computed(() => !upstreamLoading.value && activeTab.value === 'upstream')
usePollingScheduler(() => fetchUpstream({ silent: true }), 10000, {
  enabled: upPollEnabled,
  maxIntervalMs: 60000,
  backgroundIntervalMs: 30000
})
</script>

<template>
  <div class="m3-page proxy-page">
    <PageHeader>
      <template #actions>
        <RefreshButton
          :loading="
            activeTab === 'upstream'
              ? upstreamLoading || upstreamRefreshing
              : initialLoading || refreshing
          "
          :disabled="saving || upstreamSaving"
          @click="refreshActive"
        />
        <md-filled-button
          type="button"
          :disabled="saving || upstreamSaving"
          @click="activeTab === 'upstream' ? openUpstreamDrawer() : openDrawer()"
        >
          <SvgIcon slot="icon" :svg="addIcon" />
          {{ activeTab === 'upstream' ? '新增代理' : '新增实例' }}
        </md-filled-button>
      </template>
    </PageHeader>
    <md-tabs aria-label="代理类型" @change="changeTab">
      <md-primary-tab
        id="proxy-tab-upstream"
        aria-controls="proxy-upstream"
        :active="activeTab === 'upstream'"
        data-tab="upstream"
      >
        <SvgIcon slot="icon" :svg="earthIcon" />
        漫游前置代理
      </md-primary-tab>
      <md-primary-tab
        id="proxy-tab-outbound"
        aria-controls="proxy-outbound"
        :active="activeTab === 'outbound'"
        data-tab="outbound"
      >
        <SvgIcon slot="icon" :svg="routerIcon" />
        本地出站代理
      </md-primary-tab>
    </md-tabs>
    <section
      v-show="activeTab === 'upstream'"
      id="proxy-upstream"
      role="tabpanel"
      aria-labelledby="proxy-tab-upstream"
      class="proxy-section"
    >
      <div class="section-heading">
        <h2 class="m3-title">
          漫游前置代理
          <span class="m3-badge">{{ upstreamStore.proxies.length }}</span>
        </h2>
        <p class="m3-muted">通过 SOCKS5 连接海外运营商，代理端须支持 UDP Associate。</p>
      </div>
      <ErrorState
        v-if="upstreamError"
        title="加载前置代理失败"
        :message="upstreamError.message"
        :status-code="upstreamError.status"
        @retry="fetchUpstream"
      />
      <ListSkeleton v-if="upstreamLoading && !upstreamStore.proxies.length" :rows="2" />
      <EmptyState
        v-else-if="!upstreamStore.proxies.length"
        title="暂无前置代理"
        subtitle="新增 SOCKS5 代理，再为 SIM 归属国家配置规则；未配置的国家默认直连"
      />
      <div v-else class="proxy-list">
        <article
          v-for="proxy in upstreamProxiesWithRuleCount"
          :key="proxy.id"
          class="m3-card proxy-item"
          :aria-label="proxy.name || proxy.id"
        >
          <div class="proxy-identity">
            <span class="proxy-avatar"><SvgIcon :svg="earthIcon" /></span>
            <div>
              <h3>{{ proxy.name || proxy.id }}</h3>
              <p class="proxy-address">{{ proxy.addr }}</p>
              <p class="m3-muted">
                {{ proxy.username ? `鉴权：${proxy.username}` : '无需账号认证' }}
              </p>
            </div>
          </div>
          <div class="proxy-item-footer">
            <span class="proxy-state">
              <StatusLight :tone="proxy.enabled ? 'success' : 'neutral'" :animated="false" />
              {{ proxy.enabled ? '已启用' : '已禁用' }}
            </span>
            <span class="m3-badge">{{ proxy.ruleCount }} 个国家规则</span>
          </div>
          <div class="m3-actions proxy-actions">
            <md-text-button
              type="button"
              :disabled="upstreamSaving"
              @click="openCountryRuleDrawer(proxy)"
            >
              <SvgIcon slot="icon" :svg="linkIcon" />
              国家规则
            </md-text-button>
            <md-icon-button
              :aria-label="`编辑 ${proxy.name || proxy.id}`"
              :disabled="upstreamSaving"
              @click="openUpstreamDrawer(proxy)"
            >
              <SvgIcon :svg="editIcon" />
            </md-icon-button>
            <md-icon-button
              class="danger"
              :aria-label="`删除 ${proxy.name || proxy.id}`"
              :disabled="upstreamSaving"
              @click="deleteUpstream(proxy)"
            >
              <SvgIcon :svg="deleteIcon" />
            </md-icon-button>
          </div>
        </article>
      </div>
    </section>
    <section
      v-show="activeTab === 'outbound'"
      id="proxy-outbound"
      role="tabpanel"
      aria-labelledby="proxy-tab-outbound"
      class="proxy-section"
    >
      <div class="section-heading">
        <h2 class="m3-title">
          本地出站实例
          <span class="m3-badge">{{ instances.length }}</span>
        </h2>
        <p class="m3-muted">每个实例绑定一个设备，通过该设备的网络接口提供出口。</p>
      </div>
      <ErrorState
        v-if="loadError"
        title="加载代理配置失败"
        :message="loadError.message"
        :status-code="loadError.status"
        @retry="fetchOverview"
      />
      <ListSkeleton v-if="initialLoading && !instances.length" :rows="3" />
      <EmptyState
        v-else-if="!instances.length"
        title="暂无代理实例"
        subtitle="新增实例并选择出口设备"
      />
      <div v-else class="proxy-list">
        <article
          v-for="inst in instancesWithStatus"
          :key="inst.id"
          class="m3-card proxy-item"
          :aria-label="inst.name || inst.id"
        >
          <div class="proxy-identity">
            <span class="proxy-avatar"><SvgIcon :svg="routerIcon" /></span>
            <div>
              <h3>{{ inst.name || inst.id }}</h3>
              <p class="proxy-address">
                {{ formatModeLabel(inst.mode) }} · {{ inst.listen_addr }}:{{ inst.listen_port }}
              </p>
              <p class="m3-muted">
                出口：{{ devices.find((d) => d.id === inst.device_id)?.name || inst.device_id }}
              </p>
            </div>
          </div>
          <div class="proxy-item-footer">
            <span class="proxy-state">
              <StatusLight :tone="inst.status.running ? 'success' : 'neutral'" :animated="false" />
              {{ inst.status.running ? '运行中' : '已停止' }}
            </span>
            <span class="m3-badge">{{ inst.enabled ? '已启用' : '已禁用' }}</span>
            <span class="m3-badge">{{ inst.auth_enabled ? '账号认证' : '免认证' }}</span>
          </div>
          <p v-if="inst.status.last_error" class="proxy-error">{{ inst.status.last_error }}</p>
          <div class="m3-actions proxy-actions">
            <md-text-button
              v-if="!inst.status.running"
              type="button"
              :disabled="!inst.enabled || saving || !!instanceAction"
              @click="startInstance(inst.id)"
            >
              <SvgIcon slot="icon" :svg="playIcon" />
              启动
            </md-text-button>
            <md-text-button
              v-else
              type="button"
              :disabled="saving || !!instanceAction"
              @click="stopInstance(inst.id)"
            >
              <SvgIcon slot="icon" :svg="stopIcon" />
              停止
            </md-text-button>
            <md-icon-button
              :aria-label="`重启 ${inst.name || inst.id}`"
              :disabled="!inst.enabled || saving || !!instanceAction"
              @click="restartInstance(inst.id)"
            >
              <SvgIcon :svg="refreshIcon" />
            </md-icon-button>
            <md-icon-button
              :aria-label="`编辑 ${inst.name || inst.id}`"
              :disabled="saving || !!instanceAction"
              @click="openDrawer(inst)"
            >
              <SvgIcon :svg="editIcon" />
            </md-icon-button>
            <md-icon-button
              class="danger"
              :aria-label="`删除 ${inst.name || inst.id}`"
              :disabled="saving || !!instanceAction"
              @click="deleteInstance(inst.id)"
            >
              <SvgIcon :svg="deleteIcon" />
            </md-icon-button>
          </div>
        </article>
      </div>
    </section>

    <md-dialog
      class="proxy-dialog"
      :open="drawerOpen"
      @cancel="guardClose($event, saving)"
      @closed="drawerOpen = false"
    >
      <div slot="headline">{{ editingInstance ? '编辑代理实例' : '新增代理实例' }}</div>
      <form id="instance-form" slot="content" class="proxy-form" @submit.prevent="saveForm">
        <p v-if="formError" class="proxy-error" role="alert">{{ formError }}</p>
        <div class="form-pair">
          <md-outlined-text-field
            label="实例 ID"
            name="id"
            required
            :value="instanceForm.id"
            :disabled="!!editingInstance || saving || instanceLoading"
            @input="instanceForm.id = fieldValue($event)"
          />
          <md-outlined-text-field
            label="名称"
            name="name"
            :value="instanceForm.name"
            :disabled="saving || instanceLoading"
            @input="instanceForm.name = fieldValue($event)"
          />
        </div>
        <md-outlined-select
          label="出口设备"
          name="device_id"
          required
          :value="instanceForm.device_id"
          :disabled="saving || instanceLoading"
          @change="instanceForm.device_id = fieldValue($event)"
        >
          <md-select-option
            v-for="d in devices"
            :key="d.id"
            :value="d.id"
            :selected="instanceForm.device_id === d.id"
          >
            <div slot="headline">{{ d.name }} ({{ d.interface }})</div>
          </md-select-option>
        </md-outlined-select>
        <md-outlined-select
          label="代理协议"
          name="mode"
          :value="instanceForm.mode"
          :disabled="saving || instanceLoading"
          @change="instanceForm.mode = fieldValue($event) as ProxyMode"
        >
          <md-select-option
            v-for="opt in modeOptions"
            :key="opt.value"
            :value="opt.value"
            :selected="instanceForm.mode === opt.value"
          >
            <div slot="headline">{{ opt.label }}</div>
          </md-select-option>
        </md-outlined-select>
        <div class="form-pair">
          <md-outlined-text-field
            label="监听地址"
            name="listen_addr"
            :value="instanceForm.listen_addr"
            :disabled="saving || instanceLoading"
            @input="instanceForm.listen_addr = fieldValue($event)"
          />
          <md-outlined-text-field
            label="监听端口"
            name="listen_port"
            type="number"
            required
            min="1"
            max="65535"
            step="1"
            :value="String(instanceForm.listen_port)"
            :disabled="saving || instanceLoading"
            @input="instanceForm.listen_port = Number(fieldValue($event))"
          />
        </div>
        <label class="form-switch">
          <span>
            启用实例
            <small>禁用后不会自动启动</small>
          </span>
          <md-switch
            v-sync-switch
            aria-label="启用实例"
            :selected="instanceForm.enabled"
            :disabled="saving || instanceLoading"
            @change="instanceForm.enabled = fieldSelected($event)"
          />
        </label>
        <label class="form-switch">
          <span>
            账号认证
            <small>关闭后允许免认证连接</small>
          </span>
          <md-switch
            v-sync-switch
            aria-label="账号认证"
            :selected="instanceForm.auth_enabled"
            :disabled="saving || instanceLoading"
            @change="instanceForm.auth_enabled = fieldSelected($event)"
          />
        </label>
        <div v-if="instanceForm.auth_enabled" class="form-pair">
          <md-outlined-text-field
            label="用户名"
            name="username"
            autocomplete="off"
            :value="instanceForm.username"
            :disabled="saving || instanceLoading"
            @input="instanceForm.username = fieldValue($event)"
          />
          <md-outlined-text-field
            label="密码"
            name="password"
            :type="showInstancePassword ? 'text' : 'password'"
            autocomplete="new-password"
            :value="instanceForm.password || ''"
            :disabled="saving || instanceLoading"
            @input="instanceForm.password = fieldValue($event)"
          >
            <md-icon-button
              slot="trailing-icon"
              type="button"
              :aria-label="showInstancePassword ? '隐藏密码' : '显示密码'"
              :aria-pressed="showInstancePassword"
              :disabled="saving || instanceLoading"
              @click="showInstancePassword = !showInstancePassword"
            >
              <SvgIcon :svg="showInstancePassword ? hiddenIcon : visibilityIcon" />
            </md-icon-button>
          </md-outlined-text-field>
        </div>
        <p v-if="instanceLoading" class="m3-muted" role="status">正在读取完整配置…</p>
      </form>
      <div slot="actions">
        <md-text-button type="button" :disabled="saving" @click="drawerOpen = false">
          取消
        </md-text-button>
        <md-filled-button type="submit" form="instance-form" :disabled="saving || instanceLoading">
          {{ saving ? '保存中…' : '保存' }}
        </md-filled-button>
      </div>
    </md-dialog>

    <md-dialog
      class="proxy-dialog"
      :open="upstreamDrawerOpen"
      @cancel="guardClose($event, upstreamSaving)"
      @closed="upstreamDrawerOpen = false"
    >
      <div slot="headline">{{ editingUpstream ? '编辑前置代理' : '新增前置代理' }}</div>
      <form id="upstream-form" slot="content" class="proxy-form" @submit.prevent="saveUpstreamForm">
        <p v-if="formError" class="proxy-error" role="alert">{{ formError }}</p>
        <div class="form-pair">
          <md-outlined-text-field
            label="代理 ID"
            name="id"
            required
            :value="upstreamForm.id"
            :disabled="!!editingUpstream || upstreamSaving"
            @input="upstreamForm.id = fieldValue($event)"
          />
          <md-outlined-text-field
            label="名称"
            name="name"
            :value="upstreamForm.name"
            :disabled="upstreamSaving"
            @input="upstreamForm.name = fieldValue($event)"
          />
        </div>
        <md-outlined-text-field
          label="SOCKS5 地址"
          name="addr"
          required
          placeholder="host:1080 或 [IPv6]:1080"
          :value="upstreamForm.addr"
          :disabled="upstreamSaving"
          @input="upstreamForm.addr = fieldValue($event)"
        />
        <p class="m3-muted">
          {{ upstreamProxyIPv6AddressHint }}。保存时会探测 SOCKS5 握手与 UDP Associate。
        </p>
        <label class="form-switch">
          <span>
            启用代理
            <small>禁用后，该代理的国家规则会回退直连</small>
          </span>
          <md-switch
            v-sync-switch
            aria-label="启用代理"
            :selected="upstreamForm.enabled"
            :disabled="upstreamSaving"
            @change="upstreamForm.enabled = fieldSelected($event)"
          />
        </label>
        <div class="form-pair">
          <md-outlined-text-field
            label="用户名（可选）"
            name="username"
            autocomplete="off"
            :value="upstreamForm.username"
            :disabled="upstreamSaving"
            @input="upstreamForm.username = fieldValue($event)"
          />
          <md-outlined-text-field
            label="密码（可选）"
            name="password"
            :type="showUpstreamPassword ? 'text' : 'password'"
            autocomplete="new-password"
            :value="upstreamForm.password || ''"
            :disabled="upstreamSaving"
            @input="upstreamForm.password = fieldValue($event)"
          >
            <md-icon-button
              slot="trailing-icon"
              type="button"
              :aria-label="showUpstreamPassword ? '隐藏密码' : '显示密码'"
              :aria-pressed="showUpstreamPassword"
              :disabled="upstreamSaving"
              @click="showUpstreamPassword = !showUpstreamPassword"
            >
              <SvgIcon :svg="showUpstreamPassword ? hiddenIcon : visibilityIcon" />
            </md-icon-button>
          </md-outlined-text-field>
        </div>
        <p v-if="editingUpstream" class="m3-muted">密码留空会保持原密码不变。</p>
      </form>
      <div slot="actions">
        <md-text-button
          type="button"
          :disabled="upstreamSaving"
          @click="upstreamDrawerOpen = false"
        >
          取消
        </md-text-button>
        <md-filled-button type="submit" form="upstream-form" :disabled="upstreamSaving">
          {{ upstreamSaving ? '保存中…' : editingUpstream ? '更新' : '创建' }}
        </md-filled-button>
      </div>
    </md-dialog>

    <md-dialog
      class="proxy-dialog"
      :open="countryRuleDrawerOpen"
      @cancel="guardClose($event, upstreamSaving)"
      @closed="countryRuleDrawerOpen = false"
    >
      <div slot="headline">
        国家规则 · {{ countryRuleTargetProxy?.name || countryRuleTargetProxy?.id }}
      </div>
      <div slot="content" class="proxy-form">
        <p v-if="formError" class="proxy-error" role="alert">{{ formError }}</p>
        <p class="m3-notice">
          按 SIM 归属 MCC 匹配国家。未配置规则的国家默认直连；修改后需重启 VoWiFi 生效。
        </p>
        <h3>已配置国家</h3>
        <p v-if="!currentProxyCountryRules.length" class="m3-muted">尚未配置国家规则</p>
        <div v-for="rule in currentProxyCountryRules" :key="rule.country_code" class="country-rule">
          <div>
            <strong>{{ rule.country_code }} · {{ rule.country_name || rule.country_code }}</strong>
            <p class="m3-muted">
              MCC {{ rule.mccs.join('/') || '-' }} · {{ rule.enabled ? '已启用' : '已禁用' }}
            </p>
          </div>
          <md-text-button
            type="button"
            class="danger"
            :disabled="upstreamSaving"
            @click="doDeleteCountryRule(rule.country_code)"
          >
            删除规则
          </md-text-button>
        </div>
        <form id="country-form" class="proxy-form" @submit.prevent="doUpsertCountryRule">
          <md-outlined-text-field
            label="查找国家"
            name="country_query"
            :value="countryQuery"
            @input="countryQuery = fieldValue($event)"
          />
          <md-outlined-select
            label="国家"
            name="country_code"
            required
            :value="selectedCountryCode"
            :disabled="upstreamSaving"
            @change="selectedCountryCode = fieldValue($event)"
          >
            <md-select-option
              v-for="country in filteredCountries"
              :key="country.country_code"
              :value="country.country_code"
              :selected="selectedCountryCode === country.country_code"
            >
              <div slot="headline">{{ formatCountryLabel(country) }}</div>
            </md-select-option>
          </md-outlined-select>
          <p v-if="!filteredCountries.length" class="m3-muted">没有可选的匹配国家</p>
        </form>
      </div>
      <div slot="actions">
        <md-text-button
          type="button"
          :disabled="upstreamSaving"
          @click="countryRuleDrawerOpen = false"
        >
          关闭
        </md-text-button>
        <md-filled-button
          type="submit"
          form="country-form"
          :disabled="!selectedCountryCode || upstreamSaving"
        >
          {{ upstreamSaving ? '保存中…' : '保存规则' }}
        </md-filled-button>
      </div>
    </md-dialog>

    <md-dialog
      :open="!!confirmation"
      @cancel="guardClose($event, confirmBusy)"
      @closed="confirmation = null"
    >
      <div slot="headline">{{ confirmation?.title }}</div>
      <div slot="content" class="confirmation-message">
        {{ confirmation?.message }}
        <p v-if="formError" class="proxy-error" role="alert">{{ formError }}</p>
      </div>
      <div slot="actions">
        <md-text-button type="button" :disabled="confirmBusy" @click="confirmation = null">
          取消
        </md-text-button>
        <md-filled-button
          type="button"
          class="danger"
          :disabled="confirmBusy"
          @click="runConfirmation"
        >
          {{ confirmBusy ? '处理中…' : '删除' }}
        </md-filled-button>
      </div>
    </md-dialog>
  </div>
</template>

<style scoped>
.proxy-section,
.proxy-form {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.section-heading {
  display: grid;
  gap: 8px;
}
.section-heading h2 {
  display: flex;
  align-items: center;
  gap: 12px;
}
.proxy-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 320px), 1fr));
  gap: 16px;
}
.proxy-item {
  display: flex;
  flex-direction: column;
  gap: 20px;
}
.proxy-identity {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}
.proxy-identity > div {
  min-width: 0;
}
h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 500;
  line-height: 24px;
  overflow-wrap: anywhere;
}
.proxy-avatar {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.proxy-address {
  margin: 4px 0;
  font: 14px/20px var(--vh-typeface-mono);
  overflow-wrap: anywhere;
}
.proxy-item-footer,
.proxy-state {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 12px;
}
.proxy-actions {
  margin-top: auto;
  padding-top: 8px;
  border-top: 1px solid var(--md-sys-color-outline-variant);
}
.proxy-actions md-text-button {
  margin-right: auto;
}
.proxy-error {
  color: var(--md-sys-color-error);
  font-size: 14px;
  overflow-wrap: anywhere;
}
.proxy-dialog {
  width: min(560px, calc(100vw - 48px));
  max-width: calc(100vw - 48px);
}
.proxy-form {
  width: 100%;
  padding: 0;
}
.proxy-form[slot='content'] {
  padding: 24px 24px 8px;
}
.form-pair {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
.proxy-form md-outlined-text-field,
.proxy-form md-outlined-select {
  width: 100%;
  min-width: 0;
}
.form-switch,
.country-rule {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px;
  border-radius: 12px;
  background: var(--md-sys-color-surface-container);
  font-size: 14px;
}
.form-switch small {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: var(--md-sys-color-on-surface-variant);
}
.country-rule > div {
  min-width: 0;
  overflow-wrap: anywhere;
}
.country-rule md-text-button {
  flex-shrink: 0;
}
.confirmation-message {
  white-space: pre-line;
}
md-icon-button.danger {
  --md-icon-button-icon-color: var(--md-sys-color-error);
}
md-text-button.danger {
  --md-text-button-label-text-color: var(--md-sys-color-error);
}
md-filled-button.danger {
  --md-filled-button-container-color: var(--md-sys-color-error);
  --md-filled-button-label-text-color: var(--md-sys-color-on-error);
}
@media (max-width: 599px) {
  .form-pair {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
