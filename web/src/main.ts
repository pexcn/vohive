import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import { debugCollector } from './debug/collector'
import './style.css'
import './materialWeb'
import './material.css'

let bootFinished = false
let bootErrorOverlay: HTMLDivElement | null = null

function toErrorText(err: unknown) {
  if (err instanceof Error) {
    return `${err.name}: ${err.message}\n\n${err.stack || ''}`
  }
  if (typeof err === 'string') return err
  try {
    return JSON.stringify(err, null, 2)
  } catch {
    return String(err)
  }
}

function isChunkLoadLikeError(err: unknown) {
  const msg = (err instanceof Error ? err.message : typeof err === 'string' ? err : '') || ''
  return /Loading chunk|ChunkLoadError|dynamically imported module|Importing a module script failed|Failed to fetch dynamically imported module/i.test(
    msg
  )
}

function isScriptLoadErrorEvent(e: Event) {
  const t = e.target
  return t instanceof HTMLScriptElement && !!t.src
}

function showBootError(err: unknown, opts: { force?: boolean } = {}) {
  if (bootFinished && !opts.force) return
  try {
    debugCollector.recordJsError(err, 'boot')
    const text = toErrorText(err)

    if (!bootErrorOverlay) {
      const el = document.createElement('div')
      el.style.position = 'fixed'
      el.style.inset = '0'
      el.style.zIndex = '99999'
      el.style.background = 'var(--md-sys-color-surface, #111318)'
      el.style.color = 'var(--md-sys-color-on-surface, #e2e2e9)'
      el.style.padding = '24px'
      el.style.fontFamily =
        'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace'
      el.style.overflow = 'auto'
      el.innerHTML = `<div style="max-width: 980px; margin: 0 auto;">
  <div style="display:flex; align-items:center; justify-content:space-between; gap:12px; margin-bottom: 12px;">
    <div style="font-weight: 800; font-size: 18px;">前端异常（Boot Error）</div>
    <div style="display:flex; gap:10px;">
      <button data-action="continue" style="cursor:pointer; border:1px solid var(--md-sys-color-outline, #8e9099); background:transparent; color:inherit; padding:8px 12px; border-radius:24px;">继续使用</button>
      <button data-action="reload" style="cursor:pointer; border:0; background:var(--md-sys-color-primary-container, #284777); color:var(--md-sys-color-on-primary-container, #d6e3ff); padding:8px 12px; border-radius:24px;">刷新页面</button>
    </div>
  </div>
  <div style="opacity: 0.8; margin-bottom: 14px;">建议先尝试刷新；如果仍失败，请把下面内容截图发我（用于定位原因）</div>
  <pre data-role="error-text" style="white-space: pre-wrap; background: var(--md-sys-color-surface-container, #1e2025); padding: 16px; border-radius: 12px; border: 1px solid var(--md-sys-color-outline-variant, #44474e);"></pre>
</div>`
      el.addEventListener('click', (ev) => {
        const target = ev.target as HTMLElement | null
        const action = target?.getAttribute?.('data-action')
        if (action === 'continue') {
          el.remove()
          bootErrorOverlay = null
        } else if (action === 'reload') {
          window.location.reload()
        }
      })
      bootErrorOverlay = el
      document.body.appendChild(el)
    }

    const pre = bootErrorOverlay.querySelector('pre[data-role="error-text"]') as HTMLPreElement | null
    if (pre) pre.textContent = text
  } catch {
    // Avoid recursive boot error crashes if overlay rendering itself fails.
  }
}

window.addEventListener('error', (e) => {
  const ev = e as ErrorEvent
  debugCollector.recordJsError(ev.error || ev.message, 'window.error')
  if (isScriptLoadErrorEvent(e)) {
    const t = e.target as HTMLScriptElement
    showBootError(`脚本加载失败：${String(t?.src || '')}`, { force: true })
    return
  }
  if (!bootFinished || isChunkLoadLikeError(ev.error || ev.message)) {
    showBootError(ev.error || ev.message, { force: isChunkLoadLikeError(ev.error || ev.message) })
  }
})

window.addEventListener('unhandledrejection', (e) => {
  const ev = e as PromiseRejectionEvent
  debugCollector.recordJsError(ev.reason, 'unhandledrejection')
  if (!bootFinished || isChunkLoadLikeError(ev.reason)) {
    showBootError(ev.reason, { force: isChunkLoadLikeError(ev.reason) })
  }
})

const app = createApp(App)

app.config.errorHandler = (err) => {
  debugCollector.recordJsError(err, 'vue.errorHandler')
  if (!bootFinished || isChunkLoadLikeError(err)) {
    showBootError(err, { force: isChunkLoadLikeError(err) })
  }
}

app.use(createPinia())
app.use(router)

router.onError((err) => {
  showBootError(err, { force: true })
})

app.mount('#app')
bootFinished = true
