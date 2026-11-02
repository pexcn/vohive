import test from 'node:test'
import assert from 'node:assert/strict'
import { spawn, spawnSync } from 'node:child_process'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import vue from '@vitejs/plugin-vue'

const chromium = process.env.CHROMIUM_BIN || 'chromium'
const available = typeof WebSocket === 'function' && spawnSync(chromium, ['--version']).status === 0
const delay = ms => new Promise(resolve => setTimeout(resolve, ms))

async function until(check, label) {
  for (let attempt = 0; attempt < 100; attempt++) {
    const value = await check()
    if (value) return value
    await delay(50)
  }
  throw new Error(`Timed out: ${label}`)
}

test('Material controls preserve policy rollback, theme modes and refresh state', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout: 30000
}, async () => {
  const temporary = await mkdtemp(path.join(tmpdir(), 'vohive-switch-'))
  const bootScript = (await readFile(new URL('../index.html', import.meta.url), 'utf8')).match(/<script>([\s\S]*?)<\/script>/)[1]
  let browser
  let socket
  let server
  try {
    server = await createServer({
      configFile: false,
      root: fileURLToPath(new URL('../', import.meta.url)),
      cacheDir: path.join(temporary, 'vite'),
      logLevel: 'error',
      server: { host: '127.0.0.1', port: 0, strictPort: false },
      optimizeDeps: { entries: [], include: ['vue', '@material/web/switch/switch.js', '@material/web/button/outlined-button.js', '@material/web/iconbutton/icon-button.js'] },
      plugins: [vue({ template: { compilerOptions: { isCustomElement: tag => tag.startsWith('md-') } } }), {
        name: 'switch-fixture',
        resolveId(id) { if (id === '/fixture.js') return '\0switch-fixture' },
        load(id) {
          if (id !== '\0switch-fixture') return
          return `
            import { createApp, h, ref, withDirectives } from 'vue'
            import '@material/web/switch/switch.js'
            import '@material/web/button/outlined-button.js'
            import '@material/web/iconbutton/icon-button.js'
            import '/src/material.css'
            import RefreshButton from '/src/components/RefreshButton.vue'
            import SwitchDark from '/src/components/SwitchDark.vue'
            import { useTheme } from '/src/composables/useTheme'
            import { syncMaterialSwitch } from '/src/directives/materialSwitch'
            const selected = ref(true), disabled = ref(false), changes = []
            createApp({setup: () => () => withDirectives(h('md-switch', {
              selected: selected.value, disabled: disabled.value,
              onChange(event) {
                selected.value = event.target.selected
                changes.push(selected.value)
              }
            }), [[syncMaterialSwitch]])}).mount('#app')
            const loading = ref(false), refreshDisabled = ref(false)
            let refreshes = 0
            createApp({setup: () => () => [
              h(SwitchDark),
              h(RefreshButton, {loading: loading.value, disabled: refreshDisabled.value,
                onClick() { refreshes++; loading.value = true }
              })
            ]}).mount('#controls')
            window.materialControls = {
              theme: useTheme(),
              setLoading(value) { loading.value = value },
              setDisabled(value) { refreshDisabled.value = value },
              get refreshes() { return refreshes }
            }
            window.fixture = {
              setSelected(value) { selected.value = value },
              setDisabled(value) { disabled.value = value },
              changes
            }
          `
        },
        configureServer(dev) {
          dev.middlewares.use((req, res, next) => {
            if (req.url !== '/') return next()
            res.setHeader('Content-Type', 'text/html')
            res.end(`<script>${bootScript}</script><div id="app"></div><div id="controls"></div><script type="module" src="/fixture.js"></script>`)
          })
        }
      }]
    })
    await server.listen()
    browser = spawn(chromium, [
      '--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--no-first-run',
      '--remote-debugging-port=0', `--user-data-dir=${temporary}/profile`, 'about:blank'
    ], { stdio: 'ignore' })
    const port = await until(async () => {
      try { return (await readFile(`${temporary}/profile/DevToolsActivePort`, 'utf8')).split('\n')[0] }
      catch { return null }
    }, 'browser startup')
    const targets = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json()
    socket = new WebSocket(targets.find(target => target.type === 'page').webSocketDebuggerUrl)
    await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject })
    const pending = new Map()
    const exceptions = []
    let sequence = 0
    socket.onmessage = event => {
      const message = JSON.parse(event.data)
      if (message.id) {
        const request = pending.get(message.id)
        pending.delete(message.id)
        if (message.error) request.reject(message.error)
        else request.resolve(message.result)
      }
      if (message.method === 'Runtime.exceptionThrown') exceptions.push(message.params.exceptionDetails)
    }
    const send = (method, params = {}) => new Promise((resolve, reject) => {
      const id = ++sequence
      pending.set(id, { resolve, reject })
      socket.send(JSON.stringify({ id, method, params }))
    })
    const evaluate = async expression => {
      const result = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
      assert.equal(result.exceptionDetails, undefined)
      return result.result.value
    }
    const wait = expression => until(() => evaluate(expression), expression)
    const click = async (selector = 'md-switch', offset = { x: 0, y: 0 }) => {
      const point = await evaluate(`(() => {
        const rect = document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect()
        return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }
      })()`)
      point.x += offset.x
      point.y += offset.y
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', button: 'left', clickCount: 1, ...point })
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', button: 'left', clickCount: 1, ...point })
      return point
    }
    const state = expected => `document.querySelector('md-switch')?.selected === ${expected} && document.querySelector('md-switch')?.shadowRoot.querySelector('input')?.checked === ${expected}`
    await send('Runtime.enable')
    await send('Page.enable')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'light' }] })
    await send('Page.addScriptToEvaluateOnNewDocument', { source: "if (!localStorage.getItem('theme')) localStorage.setItem('theme', 'dark')" })
    await send('Page.navigate', { url: `http://127.0.0.1:${server.httpServer.address().port}/` })
    await wait(`window.fixture && ${state(true)}`)
    await click()
    await wait(state(false))
    assert.deepEqual(await evaluate('window.fixture.changes'), [false])

    // A rejected request restores the previous policy without firing another action.
    await evaluate('window.fixture.setSelected(true)')
    await wait(state(true))
    assert.deepEqual(await evaluate('window.fixture.changes'), [false])
    await click()
    await wait(state(false))
    assert.deepEqual(await evaluate('window.fixture.changes'), [false, false])

    // Server/SIM policy changes must also update a switch that was already clicked.
    await evaluate('window.fixture.setSelected(true)')
    await wait(state(true))
    await evaluate('window.fixture.setSelected(false)')
    await wait(state(false))
    await evaluate('window.fixture.setDisabled(true)')
    await wait('document.querySelector("md-switch").shadowRoot.querySelector("input").disabled')
    await click()
    assert.deepEqual(await evaluate('window.fixture.changes'), [false, false])
    await evaluate('window.fixture.setDisabled(false)')
    await wait('!document.querySelector("md-switch").shadowRoot.querySelector("input").disabled')
    await click()
    await wait(state(true))
    assert.deepEqual(await evaluate('window.fixture.changes'), [false, false, true])
    // Explicit preferences persist; only system mode follows the OS.
    await wait('!!window.materialControls && !!document.querySelector("md-outlined-button")?.shadowRoot')
    assert.equal(await evaluate('window.materialControls.theme.mode.value'), 'dark')
    assert.equal(await evaluate('document.documentElement.classList.contains("dark")'), true)
    await evaluate(`(() => {
      window.themeReveals = []
      window.themeAnimations = []
      window.nativeThemeTransition = document.startViewTransition.bind(document)
      const animate = document.documentElement.animate.bind(document.documentElement)
      document.documentElement.animate = (frames, options) => {
        const animation = animate(frames, options)
        window.themeAnimations.push(animation)
        if (options.pseudoElement !== '::view-transition-image-pair(theme-reveal)') {
          window.themeReveals.push({frames, options})
        }
        return animation
      }
      document.documentElement.style.setProperty('view-transition-name', 'root', 'important')
    })()`)
    const themeIdle = () => wait('!window.materialControls.theme.isTransitioning.value && !document.documentElement.classList.contains("theme-transition")')
    const firstPoint = await click('md-icon-button', {x: -7, y: 5})
    await wait('localStorage.getItem("theme") === "system" && !document.documentElement.classList.contains("dark")')
    await wait('window.themeReveals.length === 1')
    await wait('window.themeAnimations.length === 2')
    await evaluate('window.themeAnimations.forEach(a => { a.pause(); a.currentTime = 325 })')
    const reveal = await evaluate(`({frames: window.themeReveals[0].frames, options: window.themeReveals[0].options,
      coordinates: document.querySelector('style[data-theme-reveal]').textContent,
      origin: getComputedStyle(document.documentElement, '::view-transition-group(theme-reveal)').transformOrigin,
      scale: Number(getComputedStyle(document.documentElement, '::view-transition-group(theme-reveal)').scale),
      projection: new DOMMatrixReadOnly(getComputedStyle(document.documentElement, '::view-transition-image-pair(theme-reveal)').transform).m44,
      width: innerWidth, height: innerHeight})`)
    assert.equal(reveal.origin, `${firstPoint.x}px ${firstPoint.y}px`)
    assert.ok(Math.abs(reveal.scale / reveal.projection - 1) < 1e-5, 'The page image stays at its original scale')
    const radius = Number(reveal.coordinates.match(/circle\(([\d.]+)px/)[1])
    for (const x of [0, reveal.width]) for (const y of [0, reveal.height]) {
      assert.ok(radius >= Math.hypot(x - firstPoint.x, y - firstPoint.y), 'Reveal reaches every viewport corner')
    }
    assert.equal(reveal.options.pseudoElement, '::view-transition-group(theme-reveal)')
    assert.equal(reveal.frames.length, 2, 'Only two transform keyframes are needed')
    await click('md-icon-button')
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'system', 'Repeated clicks do not overlap a reveal')
    assert.equal(await evaluate('window.themeReveals.length'), 1)
    await evaluate('window.themeAnimations.forEach(a => a.finish())')
    await themeIdle()
    assert.equal(await evaluate('!!document.querySelector("style[data-theme-reveal]")'), false)
    assert.equal(await evaluate('document.documentElement.style.getPropertyValue("view-transition-name")'), 'root')
    assert.equal(await evaluate('document.documentElement.style.getPropertyPriority("view-transition-name")'), 'important')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'dark' }] })
    await wait('document.documentElement.classList.contains("dark")')
    assert.equal(await evaluate('window.themeReveals.length'), 1, 'OS theme changes remain immediate')
    await click('md-icon-button')
    await wait('localStorage.getItem("theme") === "light" && !document.documentElement.classList.contains("dark")')
    await themeIdle()
    assert.equal(await evaluate('document.querySelector("md-icon-button").title'), '浅色模式')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'light' }] })
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'dark' }] })
    assert.equal(await evaluate('document.documentElement.classList.contains("dark")'), false)
    await click('md-icon-button')
    await wait('localStorage.getItem("theme") === "dark" && document.documentElement.classList.contains("dark")')
    await themeIdle()
    assert.equal(await evaluate('document.querySelector("md-icon-button").ariaLabel'), '深色模式')
    assert.equal(await evaluate('window.themeReveals.length'), 3, 'Both directions reveal the new theme')
    await click('md-icon-button')
    await themeIdle()
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'system')
    assert.equal(await evaluate('window.themeReveals.length'), 3, 'An unchanged appearance needs no reveal')

    const setMode = mode => evaluate(`(async () => {
      for (let i = 0; i < 3 && window.materialControls.theme.mode.value !== ${JSON.stringify(mode)}; i++) {
        await window.materialControls.theme.cycle()
      }
    })()`)
    await setMode('dark')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'light' }, { name: 'prefers-reduced-motion', value: 'reduce' }] })
    await click('md-icon-button')
    await wait('localStorage.getItem("theme") === "system" && !document.documentElement.classList.contains("dark")')
    await themeIdle()
    assert.equal(await evaluate('document.documentElement.classList.contains("dark")'), false)
    assert.equal(await evaluate('window.themeReveals.length'), 3, 'Reduced motion switches directly')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'light' }, { name: 'prefers-reduced-motion', value: 'no-preference' }] })
    await setMode('light')
    await evaluate('document.startViewTransition = undefined')
    await click('md-icon-button')
    await themeIdle()
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'dark', 'Unsupported browsers still switch themes')

    // Skipped or failed browser transitions must release the guard and retain the target theme.
    for (const start of [
      '(update) => { const transition = window.nativeThemeTransition(update); transition.skipTransition(); return transition }',
      '() => { throw new Error("Transition unavailable") }',
    ]) {
      await setMode('light')
      await evaluate('document.startViewTransition = ' + start)
      await click('md-icon-button')
      await themeIdle()
      assert.equal(await evaluate('localStorage.getItem("theme")'), 'dark')
    }
    await evaluate('document.startViewTransition = window.nativeThemeTransition')
    await setMode('light')
    const beforeKeyboard = await evaluate('window.themeReveals.length')
    const center = await evaluate(`(() => {
      const button = document.querySelector('md-icon-button'), rect = button.getBoundingClientRect()
      button.focus()
      return {x: rect.x + rect.width / 2, y: rect.y + rect.height / 2}
    })()`)
    await send('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13, text: '\r'})
    await send('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Enter', code: 'Enter', windowsVirtualKeyCode: 13})
    await wait(`window.themeReveals.length === ${beforeKeyboard + 1}`)
    assert.equal(await evaluate('getComputedStyle(document.documentElement, "::view-transition-group(theme-reveal)").transformOrigin'), `${center.x}px ${center.y}px`)
    await themeIdle()

    // Failure of one half or cancellation of a running reveal cleans up both.
    await setMode('light')
    await evaluate(`(() => {
      window.recordedRootAnimate = document.documentElement.animate
      document.documentElement.animate = (frames, options) => {
        if (options.pseudoElement === '::view-transition-image-pair(theme-reveal)') throw new Error('Second animation unavailable')
        return window.recordedRootAnimate(frames, options)
      }
    })()`)
    await click('md-icon-button')
    await themeIdle()
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'dark')
    assert.equal(await evaluate('!!document.querySelector("style[data-theme-reveal]")'), false)
    assert.equal(await evaluate('document.documentElement.style.getPropertyValue("view-transition-name")'), 'root')
    await evaluate('document.documentElement.animate = window.recordedRootAnimate')
    await setMode('light')
    const beforeCancel = await evaluate('window.themeAnimations.length')
    await click('md-icon-button')
    await wait(`window.themeAnimations.length === ${beforeCancel + 2}`)
    await evaluate('window.themeAnimations.at(-1).cancel()')
    await themeIdle()
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'dark')
    assert.equal(await evaluate('!!document.querySelector("style[data-theme-reveal]")'), false)
    assert.equal(await evaluate('document.documentElement.style.getPropertyPriority("view-transition-name")'), 'important')

    // Mobile capture bounds can exceed innerHeight when browser bars are visible.
    await evaluate(`(() => {
      window.originalInnerHeight = Object.getOwnPropertyDescriptor(window, 'innerHeight')
      Object.defineProperty(window, 'innerHeight', {value: document.documentElement.clientHeight - 56, configurable: true})
    })()`)
    await setMode('light')
    const previousReveals = await evaluate('window.themeReveals.length')
    const capturePoint = await click('md-icon-button')
    await wait(`window.themeReveals.length === ${previousReveals + 1}`)
    const bounds = await evaluate(`(() => {
      const root = document.documentElement
      const clip = document.querySelector('style[data-theme-reveal]').textContent
      const capture = getComputedStyle(root, '::view-transition-group(theme-reveal)')
      const finalScale = Number(window.themeReveals.at(-1).frames.at(-1).scale)
      return {radius: Number(clip.match(/circle\\(([\\d.]+)px/)[1]) * finalScale, width: parseFloat(capture.width), height: parseFloat(capture.height), innerHeight}
    })()`)
    assert.ok(bounds.height > bounds.innerHeight, 'Fixture models the mobile viewport mismatch')
    for (const x of [0, bounds.width]) for (const y of [0, bounds.height]) {
      assert.ok(bounds.radius >= Math.hypot(x - capturePoint.x, y - capturePoint.y), 'Reveal covers every captured corner before teardown')
    }
    await themeIdle()
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'dark')
    assert.equal(await evaluate('!!document.querySelector("style[data-theme-reveal]")'), false)
    await evaluate(`(() => {
      if (window.originalInnerHeight) Object.defineProperty(window, 'innerHeight', window.originalInnerHeight)
      else delete window.innerHeight
    })()`)
    await evaluate('window.materialControls = undefined')
    await send('Page.reload')
    await wait('window.materialControls?.theme.mode.value === "dark"')
    // The early index script and the mounted app agree after reloading.
    assert.equal(await evaluate('document.documentElement.classList.contains("dark")'), true)

    // Loading guards repeated actions without replacing the icon or fading the button.
    await evaluate('window.savedIcon = document.querySelector(".refresh-icon"); void 0')
    await click('md-outlined-button')
    await wait('document.querySelector("md-outlined-button").getAttribute("aria-busy") === "true"')
    await click('md-outlined-button')
    assert.equal(await evaluate('window.materialControls.refreshes'), 1)
    assert.equal(await evaluate('document.querySelector("md-outlined-button").disabled'), false)
    assert.equal(await evaluate('window.savedIcon === document.querySelector(".refresh-icon")'), true)
    await evaluate('window.materialControls.setLoading(false); window.materialControls.setDisabled(true)')
    await wait('document.querySelector("md-outlined-button").disabled')
    await click('md-outlined-button')
    assert.equal(await evaluate('window.materialControls.refreshes'), 1)
    await evaluate('window.materialControls.setDisabled(false)')
    await wait('!document.querySelector("md-outlined-button").disabled')
    const point = await evaluate(`(() => {
      const rect = document.querySelector('md-outlined-button').getBoundingClientRect()
      return {x: rect.x + rect.width / 2, y: rect.y + rect.height / 2}
    })()`)
    await send('Input.dispatchMouseEvent', {type: 'mousePressed', button: 'left', clickCount: 1, ...point})
    await wait('getComputedStyle(document.querySelector("md-outlined-button")).getPropertyValue("--md-outlined-button-container-shape").trim() === "8px"')
    await send('Input.dispatchMouseEvent', {type: 'mouseReleased', button: 'left', clickCount: 1, ...point})
    await wait('getComputedStyle(document.querySelector("md-outlined-button")).getPropertyValue("--md-outlined-button-container-shape").trim() === "20px"')
    assert.equal(await evaluate('window.materialControls.refreshes'), 2)
    assert.deepEqual(exceptions, [])
  } finally {
    socket?.close()
    browser?.kill('SIGTERM')
    await server?.close()
    await rm(temporary, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
  }
})
