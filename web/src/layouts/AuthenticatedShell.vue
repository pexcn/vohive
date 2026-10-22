<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import LoadingScreen from '../components/LoadingScreen.vue'
import ErrorBoundary from '../components/ErrorBoundary.vue'
import SwitchDark from '../components/SwitchDark.vue'
import { debugCollector } from '../debug/collector'
import SvgIcon from '../components/SvgIcon.vue'
import dashboardIcon from '@material-symbols/svg-400/outlined/dashboard.svg?raw'
import devicesIcon from '@material-symbols/svg-400/outlined/devices.svg?raw'
import proxyIcon from '@material-symbols/svg-400/outlined/language.svg?raw'
import smsIcon from '@material-symbols/svg-400/outlined/sms.svg?raw'
import logsIcon from '@material-symbols/svg-400/outlined/article.svg?raw'
import settingsIcon from '@material-symbols/svg-400/outlined/settings.svg?raw'
import menuIcon from '@material-symbols/svg-400/outlined/menu.svg?raw'
import closeIcon from '@material-symbols/svg-400/outlined/close.svg?raw'
import logoutIcon from '@material-symbols/svg-400/outlined/logout.svg?raw'
import debugIcon from '@material-symbols/svg-400/outlined/bug_report.svg?raw'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const collapsed = ref(false)
const isMobile = ref(false)
const drawerOpen = ref(false)
const debugOpen = ref(false)
const logoutOpen = ref(false)
const navigationDrawer = ref<HTMLDialogElement | null>(null)
const DebugPanel = defineAsyncComponent(() => import('../components/DebugPanel.vue'))

const menuItems = [
  { index: '/', label: '仪表盘', icon: dashboardIcon },
  { index: '/devices', label: '设备管理', icon: devicesIcon },
  { index: '/proxy', label: '代理管理', icon: proxyIcon },
  { index: '/sms', label: '短信中心', icon: smsIcon },
  { index: '/logs', label: '实时日志', icon: logsIcon },
  { index: '/settings', label: '系统设置', icon: settingsIcon }
]

function handleLogout() {
  logoutOpen.value = true
}

function confirmLogout() {
  logoutOpen.value = false
  auth.logout()
  void router.push('/login')
}

watch(drawerOpen, (open) => {
  const drawer = navigationDrawer.value
  if (!drawer) return
  if (open && !drawer.open) drawer.showModal()
  else if (!open && drawer.open) drawer.close()
})

function syncIsMobile() {
  if (typeof window === 'undefined') return
  isMobile.value = window.matchMedia('(max-width: 767px)').matches
  if (!isMobile.value) {
    drawerOpen.value = false
  }
}

function handleNavToggle() {
  if (isMobile.value) {
    drawerOpen.value = true
  } else {
    collapsed.value = !collapsed.value
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.ctrlKey && e.shiftKey && String(e.key || '').toLowerCase() === 'd') {
    e.preventDefault()
    debugOpen.value = !debugOpen.value
    localStorage.setItem('debug_panel_open', debugOpen.value ? '1' : '0')
  }
}

onMounted(() => {
  syncIsMobile()
  window.addEventListener('resize', syncIsMobile, { passive: true })

  const saved = localStorage.getItem('debug_panel_open')
  debugOpen.value = saved === '1'

  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('resize', syncIsMobile)
  window.removeEventListener('keydown', onKeydown)
})

watch(
  () => route.fullPath,
  () => {
    drawerOpen.value = false
  }
)

watch(
  () => debugOpen.value,
  (v) => {
    localStorage.setItem('debug_panel_open', v ? '1' : '0')
  }
)

watch(
  () => debugCollector.openPanelRequestAt.value,
  (ts) => {
    if (!ts) return
    debugOpen.value = true
  }
)

const activePath = computed(() => route.path)
const currentPage = computed(() => menuItems.find(item => item.index === activePath.value)?.label || 'VoHive')
</script>

<template>
  <div v-if="auth.isAuthenticated && route.name !== 'Login'" class="app-shell">
    <aside v-if="!isMobile" class="navigation" :class="{ expanded: !collapsed }">
      <div class="navigation-brand">
        <span class="brand-mark">VH</span>
        <span v-if="!collapsed">VoHive</span>
      </div>
      <nav aria-label="主导航" class="navigation-links">
        <RouterLink
          v-for="(item, index) in menuItems"
          :id="`desktop-navigation-${index}`"
          :key="item.index"
          :to="item.index"
          class="navigation-link"
          :aria-current="activePath === item.index ? 'page' : undefined"
          :title="item.label"
        >
          <md-ripple v-if="!collapsed" />
          <span class="navigation-indicator">
            <md-ripple v-if="collapsed" :for="`desktop-navigation-${index}`" />
            <SvgIcon :svg="item.icon" />
          </span>
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>
      <div class="navigation-footer">
        <md-icon-button aria-label="退出登录" title="退出登录" @click="handleLogout">
          <SvgIcon :svg="logoutIcon" />
        </md-icon-button>
        <span v-if="!collapsed" class="m3-muted">管理会话</span>
      </div>
    </aside>

    <dialog
      ref="navigationDrawer"
      class="navigation-drawer"
      aria-label="主导航"
      @close="drawerOpen = false"
      @click="$event.target === $event.currentTarget && (drawerOpen = false)"
    >
      <div class="drawer-content">
        <div class="m3-row drawer-heading">
          <span class="navigation-brand">
            <span class="brand-mark">VH</span>
            VoHive
          </span>
          <md-icon-button aria-label="关闭导航" @click="drawerOpen = false">
            <SvgIcon :svg="closeIcon" />
          </md-icon-button>
        </div>
        <nav class="navigation-links expanded">
          <RouterLink
            v-for="item in menuItems"
            :key="item.index"
            :to="item.index"
            class="navigation-link"
            :aria-current="activePath === item.index ? 'page' : undefined"
          >
            <md-ripple />
            <SvgIcon :svg="item.icon" />
            <span>{{ item.label }}</span>
          </RouterLink>
        </nav>
        <md-text-button type="button" @click="handleLogout">
          <SvgIcon slot="icon" :svg="logoutIcon" />
          退出登录
        </md-text-button>
      </div>
    </dialog>

    <div class="shell-body">
      <header class="top-app-bar">
        <div class="m3-actions">
          <md-icon-button
            :aria-label="isMobile ? '打开导航' : collapsed ? '展开导航' : '收起导航'"
            :aria-expanded="isMobile ? drawerOpen : !collapsed"
            @click="handleNavToggle"
          >
            <SvgIcon :svg="menuIcon" />
          </md-icon-button>
          <span class="app-bar-title">{{ currentPage }}</span>
        </div>
        <div class="m3-actions">
          <md-icon-button
            aria-label="打开调试面板"
            title="调试面板（Ctrl+Shift+D）"
            @click="debugOpen = true"
          >
            <SvgIcon :svg="debugIcon" />
          </md-icon-button>
          <SwitchDark />
        </div>
      </header>
      <main id="main-content" class="shell-main">
        <div class="main-inner">
          <router-view v-slot="{ Component, route: r }">
            <ErrorBoundary v-if="Component" title="页面渲染失败">
              <component :is="Component" :key="r.fullPath" />
            </ErrorBoundary>
            <LoadingScreen v-else title="正在加载页面…" subtitle="正在准备页面组件与资源" />
          </router-view>
        </div>
      </main>
    </div>

    <md-dialog :open="logoutOpen" @closed="logoutOpen = false">
      <div slot="headline">退出登录？</div>
      <div slot="content">退出当前管理会话，设备会继续运行。</div>
      <div slot="actions">
        <md-text-button type="button" @click="logoutOpen = false">取消</md-text-button>
        <md-filled-button type="button" @click="confirmLogout">退出</md-filled-button>
      </div>
    </md-dialog>
    <DebugPanel v-model="debugOpen" />
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  height: 100%;
  overflow: hidden;
}
.navigation {
  flex: none;
  display: flex;
  flex-direction: column;
  width: 80px;
  background: var(--md-sys-color-surface);
  padding: 16px 0;
}
.navigation.expanded {
  width: 240px;
}
.navigation-brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  min-height: 48px;
  font-size: 22px;
  line-height: 28px;
}
.brand-mark {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: var(--md-sys-color-primary-container);
  color: var(--md-sys-color-on-primary-container);
  font-size: 16px;
  font-weight: 500;
}
.navigation-links {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 24px 0;
}
.navigation-link {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  color: var(--md-sys-color-on-surface-variant);
  text-decoration: none;
  font-size: 12px;
  font-weight: 500;
  line-height: 16px;
}
.navigation-indicator {
  position: relative;
  display: grid;
  place-items: center;
  width: 56px;
  height: 32px;
  border-radius: 16px;
  overflow: hidden;
}
.navigation-link[aria-current='page'] {
  color: var(--md-sys-color-on-surface);
}
.navigation-link[aria-current='page'] .navigation-indicator {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.expanded .navigation-link {
  position: relative;
  flex-direction: row;
  gap: 16px;
  min-height: 56px;
  margin: 0 12px;
  padding: 0 16px;
  border-radius: 28px;
  overflow: hidden;
  font-size: 14px;
  line-height: 20px;
}
.expanded .navigation-link[aria-current='page'] {
  background: var(--md-sys-color-secondary-container);
  color: var(--md-sys-color-on-secondary-container);
}
.expanded .navigation-indicator {
  width: 24px;
  height: 32px;
  overflow: visible;
}
.navigation-link:focus-visible {
  outline: 3px solid var(--md-sys-color-primary);
  outline-offset: -3px;
  border-radius: 16px;
}
.navigation-footer {
  margin-top: auto;
  padding: 16px;
  display: flex;
  align-items: center;
  gap: 12px;
}
.shell-body {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.top-app-bar {
  flex: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 64px;
  padding: 0 16px;
  background: var(--md-sys-color-surface);
}
.app-bar-title {
  font-size: 22px;
  line-height: 28px;
}
.shell-main {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 16px 24px 32px;
}
.main-inner {
  width: 100%;
  max-width: 1280px;
  margin: 0 auto;
}
.navigation-drawer {
  position: fixed;
  inset: 0 auto 0 0;
  width: min(280px, calc(100vw - 56px));
  height: 100dvh;
  max-height: 100dvh;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0 16px 16px 0;
  background: var(--md-sys-color-surface-container-low);
  color: var(--md-sys-color-on-surface);
}
.navigation-drawer::backdrop {
  background: color-mix(in srgb, var(--md-sys-color-scrim) 32%, transparent);
}
.drawer-content {
  min-height: 100%;
  padding: 16px 0;
}
.drawer-content .navigation-links {
  gap: 4px;
  padding: 16px 0;
}
.drawer-content .navigation-link {
  min-height: 48px;
}
.drawer-heading {
  padding: 0 16px;
}
.drawer-content > md-text-button {
  margin: 16px;
}
@media (max-width: 599px) {
  .shell-main {
    padding: 8px 16px 24px;
  }
  .top-app-bar {
    padding: 0 8px;
  }
}
</style>
