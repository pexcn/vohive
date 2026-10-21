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
    const click = async (selector = 'md-switch') => {
      const point = await evaluate(`(() => {
        const rect = document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect()
        return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 }
      })()`)
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', button: 'left', clickCount: 1, ...point })
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', button: 'left', clickCount: 1, ...point })
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
    // Existing explicit preferences survive migration. Only system mode follows the OS.
    await wait('!!window.materialControls && !!document.querySelector("md-outlined-button")?.shadowRoot')
    assert.equal(await evaluate('window.materialControls.theme.mode.value'), 'dark')
    assert.equal(await evaluate('document.documentElement.classList.contains("dark")'), true)
    await click('md-icon-button')
    await wait('localStorage.getItem("theme") === "system" && !document.documentElement.classList.contains("dark")')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'dark' }] })
    await wait('document.documentElement.classList.contains("dark")')
    await click('md-icon-button')
    await wait('localStorage.getItem("theme") === "light" && !document.documentElement.classList.contains("dark")')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'light' }] })
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'dark' }] })
    assert.equal(await evaluate('document.documentElement.classList.contains("dark")'), false)
    await click('md-icon-button')
    await wait('localStorage.getItem("theme") === "dark" && document.documentElement.classList.contains("dark")')
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
