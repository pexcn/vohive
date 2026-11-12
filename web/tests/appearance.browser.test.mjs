import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { spawn, spawnSync } from 'node:child_process'
import { mkdtemp, readFile, rm, mkdir, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const chromium = process.env.CHROMIUM_BIN || 'chromium'
const available = typeof WebSocket === 'function' && spawnSync(chromium, ['--version']).status === 0

test('appearance settings preserve presets, mode, first paint and chart colors', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout: 60000
}, async () => {
  const root = fileURLToPath(new URL('../', import.meta.url))
  const profile = await mkdtemp(path.join(tmpdir(), 'vohive-appearance-'))
  const artifacts = process.env.VOHIVE_SCREENSHOTS || path.join(profile, 'screenshots')
  const fixture = path.join(root, '__appearance_fixture.vue')
  const server = await createServer({
    root, configFile: path.join(root, 'vite.config.ts'),
    server: { host: '127.0.0.1', port: 0, strictPort: false },
    plugins: [{
      name: 'appearance-fixture',
      resolveId(id) {
        if (id === '/appearance-fixture.vue') return fixture
        if (id === '/appearance-fixture.ts') return path.join(root, '__appearance_fixture.ts')
      },
      load(id) {
        if (id === path.join(root, '__appearance_fixture.ts')) return `import {createApp} from 'vue';import App from '/appearance-fixture.vue';import '/src/materialWeb.ts';import '/src/style.css';import '/src/material.css';createApp(App).mount('#app')`
        if (id !== fixture) return
        return `<script setup>
import AppearanceSettings from '/src/components/AppearanceSettings.vue'
import TrafficAnalysisPanel from '/src/components/TrafficAnalysisPanel.vue'
const analysis = {buckets: [], chart: {timestamps: ['10:00','11:00'], devices: ['fixture'], series: {fixture: [100,200]}}}
</script><template><main><AppearanceSettings/><TrafficAnalysisPanel :analysis="analysis" range="day" mode="global"/></main></template><style>main{max-width:1000px;margin:auto;padding:16px;display:grid;gap:24px}</style>`
      },
      configureServer(dev) {
        dev.middlewares.use(async (req, res, next) => {
          if (!req.url?.startsWith('/appearance-fixture.html')) return next()
          const html = await dev.transformIndexHtml('/appearance-fixture.html', `<!doctype html><html><head><meta name="theme-color" content="#f9f6ef"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="/favicon.svg"></head><body><div id="app"></div><script type="module" src="/appearance-fixture.ts"></script></body></html>`)
          res.setHeader('Content-Type', 'text/html'); res.end(html)
        })
      }
    }]
  })
  let browser, socket, inspect
  const errors = []
  const delay = ms => new Promise(resolve => setTimeout(resolve, ms))
  async function until(check, label) {
    const end = Date.now() + 15000
    while (Date.now() < end) {
      const result = await check()
      if (result) return result
      await delay(30)
    }
    throw new Error('Timed out: ' + label)
  }
  try {
    await server.listen()
    browser = spawn(chromium, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--no-first-run', '--remote-debugging-port=0', '--user-data-dir=' + profile, 'about:blank'], { stdio: 'ignore' })
    const port = await until(async () => {
      try { return (await readFile(path.join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0] } catch { return null }
    }, 'Chromium startup')
    const targets = await (await fetch('http://127.0.0.1:' + port + '/json/list')).json()
    socket = new WebSocket(targets.find(target => target.type === 'page').webSocketDebuggerUrl)
    await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject })
    const pending = new Map()
    let id = 0
    socket.onmessage = event => {
      const message = JSON.parse(event.data)
      if (message.id) {
        const waiter = pending.get(message.id); pending.delete(message.id)
        if (message.error) waiter.reject(message.error); else waiter.resolve(message.result)
      }
      if (message.method === 'Runtime.exceptionThrown') errors.push(message.params.exceptionDetails)
    }
    const send = (method, params = {}) => new Promise((resolve, reject) => {
      const request = ++id; pending.set(request, { resolve, reject })
      socket.send(JSON.stringify({ id: request, method, params }))
    })
    const evaluate = async expression => {
      const result = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true })
      if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails))
      return result.result.value
    }
    inspect = () => evaluate(`({body:document.body.innerText,chart:document.querySelector('.chart')?.outerHTML,errors:${JSON.stringify(errors)}})`)
    const wait = expression => until(() => evaluate(expression), expression)
    const click = async selector => {
      const point = await evaluate(`(()=>{const el=document.querySelector(${JSON.stringify(selector)});el.scrollIntoView({block:'center'});const r=el.getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()`)
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', button: 'left', clickCount: 1, ...point })
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', button: 'left', clickCount: 1, ...point })
      await wait('!document.documentElement.classList.contains("theme-transition")')
    }
    const primary = `getComputedStyle(document.documentElement).getPropertyValue('--md-sys-color-primary').trim()`
    const chartPrimary = `document.querySelector('.chart')?.__vueParentComponent?.exposed?.getOption?.()?.color?.[0]`
    const readFirstPaint = async () => {
      // These scripts are parser-blocking and run before Vue or other modules.
      const html = await (await fetch(url)).text()
      const scripts = [...html.matchAll(/<script(?![^>]*type="module")[^>]*>([\s\S]*?)<\/script>/g)].map(match => match[1])
      return evaluate(`(()=>{for(const source of ${JSON.stringify(scripts)})new Function(source)();return ${primary}})()`)
    }
    await send('Runtime.enable'); await send('Page.enable')
    await send('Emulation.setEmulatedMedia', { features: [{ name: 'prefers-color-scheme', value: 'light' }] })
    await send('Emulation.setDeviceMetricsOverride', { width: 1200, height: 900, deviceScaleFactor: 1, mobile: false })
    const url = `http://127.0.0.1:${server.httpServer.address().port}/appearance-fixture.html`
    await send('Page.navigate', { url })
    await wait('document.querySelectorAll("input[name=appearance-color]").length === 2')
    assert.equal(await evaluate(primary), '#65568c')
    await wait(`${chartPrimary} === '#65568c'`)
    const status = await evaluate(`['--vh-success','--vh-warning','--md-sys-color-error'].map(role=>getComputedStyle(document.documentElement).getPropertyValue(role).trim())`)
    await click('input[value=green]')
    await wait(`${chartPrimary} === '#007a50'`)
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'system')
    assert.equal(await evaluate('localStorage.getItem("color-scheme")'), 'green')
    assert.deepEqual(await evaluate(`['--vh-success','--vh-warning'].map(role=>getComputedStyle(document.documentElement).getPropertyValue(role).trim())`), status.slice(0, 2))
    assert.ok(await evaluate('document.querySelector("link[rel=icon]").href.startsWith("data:image/svg+xml,")'))
    assert.equal(await evaluate('document.querySelector("meta[name=theme-color]").content'), '#f4f7f5')
    assert.equal(await readFirstPaint(), '#007a50')
    await send('Page.reload')
    await wait('document.querySelector("input[value=green]")?.checked')
    assert.equal(await evaluate(primary), '#007a50')
    await click('.appearance-modes button:nth-child(3)')
    await wait(`${chartPrimary} === '#55dfa5'`)
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'dark')
    // Radio navigation remains keyboard-accessible, independently of mode.
    await send('Page.bringToFront')
    await evaluate('document.querySelector("input[value=green]").focus()')
    for (const type of ['keyDown', 'keyUp']) await send('Input.dispatchKeyEvent', { type, key: 'ArrowLeft', code: 'ArrowLeft', windowsVirtualKeyCode: 37 })
    await wait('document.querySelector("input[value=blue]").checked && !document.documentElement.classList.contains("theme-transition")')
    await wait(`${chartPrimary} === '#aac7ff'`)
    assert.equal(await evaluate('localStorage.getItem("theme")'), 'dark')
    await send('Emulation.setDeviceMetricsOverride', { width: 320, height: 720, deviceScaleFactor: 1, mobile: true })
    await click('input[value=green]')
    assert.equal(await evaluate('document.documentElement.scrollWidth <= 320'), true)
    await mkdir(artifacts, { recursive: true })
    await writeFile(path.join(artifacts, 'appearance-green-dark-mobile.png'), Buffer.from((await send('Page.captureScreenshot', { format: 'png' })).data, 'base64'))
    await evaluate('localStorage.setItem("color-scheme","removed-preset")')
    await send('Page.reload')
    await wait('document.querySelector("input[value=blue]")?.checked')
    assert.equal(await evaluate(primary), '#aac7ff')
    assert.equal(errors.length, 0, JSON.stringify(errors))
  } catch (error) {
    if (inspect) console.error(await inspect())
    throw error
  } finally {
    socket?.close(); browser?.kill('SIGTERM')
    await server.close()
    await rm(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
  }
})
