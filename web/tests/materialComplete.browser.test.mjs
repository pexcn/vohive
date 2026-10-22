import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { spawn, spawnSync } from 'node:child_process'
import { mkdtemp, readFile, writeFile, rm, mkdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const chromium = process.env.CHROMIUM_BIN || 'chromium'
const available = typeof WebSocket === 'function' && spawnSync(chromium, ['--version']).status === 0

test(
  'Remaining Material pages preserve drafts, confirmations and explicit actions',
  {
    skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
    timeout: 60000
  },
  async () => {
    const root = fileURLToPath(new URL('../', import.meta.url))
    process.chdir(root)
    const profile = await mkdtemp(path.join(tmpdir(), 'vohive-material-complete-'))
    const artifactDir = process.env.VOHIVE_SCREENSHOTS || path.join(profile, 'screenshots')
    let browser, socket, debugSend, debugEvaluate
    const exceptions = []
    let failContacts = false
    let failNext = false,
      holdNext = false,
      releaseMutation
    const calls = []
    const threads = [
      {
        imsi: 'fixture-imsi',
        peer: 'Alice',
        device_id: 'fixture',
        last_timestamp: '2026-10-21T10:00:00Z',
        last_sms_id: 1,
        last_content: 'hello'
      },
      {
        imsi: 'fixture-imsi',
        peer: 'Bob',
        device_id: 'fixture',
        last_timestamp: '2026-10-20T10:00:00Z',
        last_sms_id: 2,
        last_content: 'unread'
      }
    ]
    let profiles = [
      {
        eid: 'fixture-eid',
        aid_hex: 'A00001',
        profiles: [
          { iccid: '8900000000000001234', name: 'Active fixture', state: 1 },
          { iccid: '8900000000000005678', name: 'Inactive fixture', state: 0 },
          { iccid: '8900000000000009012', name: 'Second inactive fixture', state: 0 }
        ]
      }
    ]
    const settings = {
      telegram: { enabled: true, bot_token: 'fixture-token', chat_id: 123 },
      feishu: { enabled: true, chat_ids: ['one', 'two'] },
      qq: { enabled: true, group_ids: 'group' },
      email: {
        enabled: true,
        smtp_host: 'smtp.example',
        smtp_port: 587,
        username: 'user',
        password: 'secret',
        from_address: 'from@example',
        to_addresses: ['to@example']
      },
      pushplus: { enabled: true, token: 'fixture', topic: 'topic', channel: 'wechat' },
      bark: {
        enabled: true,
        urls: ['https://bark.example/fixture'],
        group: 'fixture',
        level: 'active'
      },
      webhook: {
        enabled: true,
        urls: ['https://hook.example/fixture'],
        secret: 'secret',
        timeout_ms: 1000,
        retry_max: 2,
        text_template: '{{text}}',
        headers: { Authorization: 'fixture', 'Content-Type': 'ignored' }
      }
    }
    async function mockApi(req, res) {
      const url = new URL(req.url, 'http://localhost')
      let raw = ''
      for await (const chunk of req) raw += chunk
      const body = raw ? JSON.parse(raw) : null
      calls.push({
        method: req.method,
        path: url.pathname,
        body,
        query: Object.fromEntries(url.searchParams)
      })
      const json = (data, status = 200) => {
        res.statusCode = status
        res.setHeader('Content-Type', 'application/json')
        res.end(JSON.stringify(data))
      }
      // No request can escape this fixture to a modem, SIM or notification recipient.
      if (req.method !== 'GET') {
        if (holdNext) {
          holdNext = false
          await new Promise((resolve) => {
            releaseMutation = resolve
          })
        }
        if (failNext) {
          failNext = false
          json({ error: 'fixture rejected operation' }, 500)
          return
        }
        if (url.pathname.endsWith('/actions/at')) {
          json({ ok: true, response: 'OK' })
          return
        }
        if (url.pathname.endsWith('/actions/ussd')) {
          json({
            result: { status: 1, text: '1. Balance', session_id: 'fixture-session' },
            channel: 'vowifi'
          })
          return
        }
        if (url.pathname.endsWith('/ussd/continue')) {
          json({ result: { status: 0, text: 'Balance fixture' }, channel: 'vowifi' })
          return
        }
        if (req.method === 'DELETE' && url.pathname.includes('/esim/profiles/'))
          profiles = profiles.map((g) => ({
            ...g,
            profiles: g.profiles.filter((p) => !url.pathname.endsWith('/' + p.iccid))
          }))
        json({ applied: true, parts_total: 1 })
        return
      }
      if (url.pathname === '/api/devices') {
        json({ devices: [{ id: 'fixture', name: 'Fixture modem', running: true, healthy: true }] })
        return
      }
      if (url.pathname === '/api/sms/contacts') {
        if (failContacts) {
          failContacts = false
          json({ error: 'Fixture contacts unavailable' }, 500)
          return
        }
        json(threads)
        return
      }
      if (url.pathname === '/api/sms/thread') {
        json([
          {
            id: 1,
            sender: url.searchParams.get('peer'),
            content: 'Fixture message',
            type: 1,
            timestamp: '2026-10-21T10:00:00Z'
          }
        ])
        return
      }
      if (url.pathname === '/api/system/info') {
        json({ version: 'fixture', config: '/fixture/config', docs: {} })
        return
      }
      if (url.pathname === '/api/settings/notifications') {
        json(settings)
        return
      }
      if (url.pathname.endsWith('/esim')) {
        json({
          chip_info: {
            sku_name: 'Fixture eUICC',
            eids: [
              { eid: 'fixture-eid', aid: 'A00001', free_nvram: '1 MB', free_nvram_bytes: 1048576 }
            ]
          },
          profiles
        })
        return
      }
      if (url.pathname.endsWith('/esim/profiles')) {
        json(profiles)
        return
      }
      if (url.pathname.endsWith('/overview')) {
        json({ devices: [{ esim_switching: true }] })
        return
      }
      if (url.pathname.startsWith('/api/cards/')) {
        json({ network_enabled: false, airplane_enabled: false, vowifi_enabled: false })
        return
      }
      json({ error: 'Unknown fixture request: ' + url.pathname }, 404)
    }

    const server = await createServer({
      root,
      configFile: path.join(root, 'vite.config.ts'),
      server: { host: '127.0.0.1', port: 0, strictPort: false },
      plugins: [
        {
          name: 'material-complete-fixture',
          resolveId(id) {
            if (id === '/fixture.vue') return path.join(root, '__material_fixture.vue')
          },
          load(id) {
            if (id !== path.join(root, '__material_fixture.vue')) return
            return `<script setup>
import {ref} from 'vue'
import MaterialFeedback from '/src/components/MaterialFeedback.vue'
import DeviceEsimTab from '/src/components/DeviceEsimTab.vue'
import DeviceAtTab from '/src/components/DeviceAtTab.vue'
import DeviceUssdTab from '/src/components/DeviceUssdTab.vue'
import DeviceConfigTab from '/src/components/DeviceConfigTab.vue'
import FieldRow from '/src/components/FieldRow.vue'
import StatusLight from '/src/components/StatusLight.vue'
import {notify,requestConfirmation} from '/src/stores/feedback'
const config=ref({id:'fixture',name:'Fixture modem',device_backend:'qmi',control_device:'/dev/wwan0qmi0'})
window.feedbackFixture={notify,requestConfirmation,results:[]}
window.fieldSubmits=0
const onFieldSubmit=()=>window.fieldSubmits++
</script><template><main class="fixture m3-stack"><DeviceEsimTab device-id="fixture" :is-active="true" :device-online="true"/><DeviceAtTab device-id="fixture" :running="true" at-port="/dev/fixture-at" backend-mode="qmi"/><DeviceUssdTab device-id="fixture" :vowifi-active="true"/><DeviceConfigTab :edit-config="config" :saving="false" :deleting="false"/><form @submit.prevent="onFieldSubmit"><FieldRow id="field-copy" label="测试字段" value="fixture-copy" copyable/><FieldRow id="field-empty" label="空值" value="--" copyable/><FieldRow id="field-plain" label="文本" value="plain"/></form><StatusLight id="status-motion" tone="success"/></main><MaterialFeedback/></template><style>.fixture{padding:24px;max-width:1000px;margin:auto}</style>`
          },
          configureServer(dev) {
            dev.middlewares.use(async (req, res, next) => {
              if (req.url?.startsWith('/api/')) {
                await mockApi(req, res)
                return
              }
              if (req.url === '/fixture.html') {
                res.setHeader('Content-Type', 'text/html')
                res.end(
                  `<html class="dark"><head><meta name="viewport" content="width=device-width, initial-scale=1.0"></head><body style="background:var(--md-sys-color-surface);color:var(--md-sys-color-on-surface);font-family:var(--md-ref-typeface-plain)"><div id="fixture"></div><script type="module">import {createApp} from '/node_modules/.vite/deps/vue.js';import {createPinia} from '/node_modules/.vite/deps/pinia.js';import '/src/materialWeb.ts';import '/src/style.css';import '/src/material.css';import Fixture from '/fixture.vue';createApp(Fixture).use(createPinia()).mount('#fixture')</script></body></html>`
                )
                return
              }
              next()
            })
          }
        }
      ]
    })
    const delay = (ms) => new Promise((resolve) => setTimeout(resolve, ms))
    async function until(check, label, timeout = 20000) {
      const end = Date.now() + timeout
      while (Date.now() < end) {
        const result = await check()
        if (result) return result
        await delay(50)
      }
      throw new Error('Timed out: ' + label)
    }
    try {
      await server.listen()
      browser = spawn(
        chromium,
        [
          '--headless=new',
          '--no-sandbox',
          '--disable-dev-shm-usage',
          '--no-first-run',
          '--remote-debugging-port=0',
          '--user-data-dir=' + profile,
          'about:blank'
        ],
        { stdio: 'ignore' }
      )
      const port = await until(async () => {
        try {
          return (await readFile(path.join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0]
        } catch {
          return null
        }
      }, 'Chromium startup')
      const targets = await (await fetch('http://127.0.0.1:' + port + '/json/list')).json()
      socket = new WebSocket(targets.find((t) => t.type === 'page').webSocketDebuggerUrl)
      await new Promise((resolve, reject) => {
        socket.onopen = resolve
        socket.onerror = reject
      })
      let id = 0
      const pending = new Map()
      socket.onmessage = (event) => {
        const message = JSON.parse(event.data)
        if (message.id) {
          const waiter = pending.get(message.id)
          pending.delete(message.id)
          if (message.error) waiter.reject(message.error)
          else waiter.resolve(message.result)
        }
        if (message.method === 'Runtime.exceptionThrown')
          exceptions.push(message.params.exceptionDetails)
      }
      const send = (method, params = {}) =>
        new Promise((resolve, reject) => {
          const n = ++id
          pending.set(n, { resolve, reject })
          socket.send(JSON.stringify({ id: n, method, params }))
        })
      const evaluate = async (expression) => {
        const result = await send('Runtime.evaluate', {
          expression,
          awaitPromise: true,
          returnByValue: true
        })
        if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails))
        return result.result.value
      }
      const wait = async (expression) => until(() => evaluate(expression), expression)
      const click = async (selector) => {
        const ariaMatch = selector.match(/^md-icon-button\[aria-label="([^"]+)"\]$/)
        const target = ariaMatch
          ? `Array.from(document.querySelectorAll('md-icon-button')).find(el=>el.ariaLabel===${JSON.stringify(ariaMatch[1])})`
          : `document.querySelector(${JSON.stringify(selector)})`
        const rect = await until(
          () =>
            evaluate(
              `(async()=>{const el=${target};if(!el)return null;await el.updateComplete;if(!el.checkVisibility())return null;const modal=el.closest('md-dialog');if(modal){await Promise.all(modal.shadowRoot.querySelector('dialog').getAnimations({subtree:true}).map(a=>a.finished.catch(()=>{})))}el.scrollIntoView({block:'center'});const r=el.getBoundingClientRect();return r.width&&r.height?{x:r.x+r.width/2,y:r.y+r.height/2}:null})()`
            ),
          'visible ' + selector
        )
        await send('Input.dispatchMouseEvent', {
          type: 'mousePressed',
          button: 'left',
          clickCount: 1,
          ...rect
        })
        await send('Input.dispatchMouseEvent', {
          type: 'mouseReleased',
          button: 'left',
          clickCount: 1,
          ...rect
        })
      }
      debugSend = send
      debugEvaluate = evaluate
      await send('Runtime.enable')
      await send('Page.enable')
      await send('Network.enable')
      await send('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-color-scheme', value: 'light' }]
      })
      await send('Network.setBlockedURLs', { urls: ['https://*'] })
      await send('Page.addScriptToEvaluateOnNewDocument', {
        source: `
    localStorage.setItem('token','fixture-token');localStorage.setItem('theme','dark');
  `
      })
      await send('Emulation.setDeviceMetricsOverride', {
        width: 1440,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false
      })
      const address = server.httpServer.address()
      const url = `http://127.0.0.1:${address.port}/`
      await mkdir(artifactDir, { recursive: true })
      const snapshot = async (name) => {
        await evaluate(
          `Promise.all([...document.querySelectorAll('md-dialog')].flatMap(el=>el.shadowRoot?.querySelector('dialog')?.getAnimations({subtree:true}) || []).map(a=>a.finished.catch(()=>{})))`
        )
        await evaluate('document.fonts.ready')
        await delay(120)
        await writeFile(
          path.join(artifactDir, name + '.png'),
          Buffer.from((await send('Page.captureScreenshot', { format: 'png' })).data, 'base64')
        )
      }
      const key = async (key, code = key, vk = 0) => {
        await send('Input.dispatchKeyEvent', {
          type: 'keyDown',
          key,
          code,
          windowsVirtualKeyCode: vk
        })
        await send('Input.dispatchKeyEvent', {
          type: 'keyUp',
          key,
          code,
          windowsVirtualKeyCode: vk
        })
      }
      const fill = async (selector, value) => {
        await evaluate(
          `(()=>{const host=document.querySelector(${JSON.stringify(selector)});const el=host.shadowRoot.querySelector('input,textarea');el.focus();el.select()})()`
        )
        await send('Input.insertText', { text: value })
      }
      const selectIndex = async (selector, index) => {
        await click(selector)
        await evaluate(
          `window.menuClosed=false;document.querySelector(${JSON.stringify(selector)}).addEventListener('closed',()=>window.menuClosed=true,{once:true})`
        )
        await key('Home', 'Home', 36)
        for (let n = 0; n < index; n++) await key('ArrowDown', 'ArrowDown', 40)
        await key('Enter', 'Enter', 13)
        await wait('window.menuClosed')
      }
      const closed = (selector) =>
        `!document.querySelector(${JSON.stringify(selector)}).shadowRoot.querySelector('dialog').open`
      const noOverflow = async () =>
        assert.equal(await evaluate('document.scrollingElement.scrollWidth<=innerWidth'), true)
      const confirm = '.app-root > md-dialog'
      await send('Page.navigate', { url: url + '#/sms' })
      await wait(
        'document.querySelectorAll(".sms-thread").length===2 && !!document.querySelector(".sms-bubble")'
      )
      assert.deepEqual(
        await evaluate('[...document.querySelectorAll("h1")].map(el=>el.textContent.trim())'),
        ['短信中心']
      )
      assert.equal(await evaluate('document.querySelectorAll(".sms-page .refresh-icon").length'),0)
      assert.equal(
        calls.filter((c) => c.method !== 'GET').length,
        0,
        'Navigation cannot send SMS or notification tests'
      )
      assert.equal(await evaluate('document.querySelectorAll(".sms-unread").length'), 1)
      await click('.sms-thread:not(.selected) .sms-thread-main')
      await wait('document.querySelectorAll(".sms-unread").length===0')
      failContacts = true
      await wait(
        `document.querySelector('.sms-page .error-state')?.textContent.includes('Fixture contacts unavailable')`
      )
      await click('.sms-page .error-state md-outlined-button')
      await wait(`!document.querySelector('.sms-page .error-state')`)
      await evaluate('location.hash="#/settings"')
      await wait(
        'document.querySelector("md-outlined-text-field[name=bot_token]")?.value==="fixture-token"'
      )
      assert.deepEqual(
        await evaluate('[...document.querySelectorAll("h1")].map(el=>el.textContent.trim())'),
        ['系统设置']
      )
      await fill('md-outlined-text-field[name=chat_id]', '-321')
      await selectIndex('md-outlined-select[name=notification-channel]', 4)
      await fill('md-outlined-text-field[name=smtp_port]', '465')
      await wait(
        `Array.from(document.querySelectorAll('md-switch')).some(el=>el.ariaLabel==='SMTP SSL' && el.selected)`
      )
      await selectIndex('md-outlined-select[name=notification-channel]', 6)
      await fill('md-outlined-text-field[name=secret]', 'new-secret')
      assert.equal(calls.filter((c) => c.method !== 'GET').length, 0)
      await click('.settings-page [aria-label="通知设置"] > .m3-row md-filled-button')
      await until(() => calls.some((c) => c.method === 'PUT'), 'Save settings')
      const payload = calls.find((c) => c.method === 'PUT').body
      assert.equal(payload.telegram.chat_id, -321)
      assert.equal(payload.email.smtp_port, 465)
      assert.equal(payload.email.use_ssl, true)
      assert.deepEqual(payload.feishu.chat_ids, ['one', 'two'])
      assert.equal(payload.qq.group_ids, 'group')
      assert.equal(payload.pushplus.topic, 'topic')
      assert.equal(payload.bark.level, 'active')
      assert.deepEqual(payload.webhook.headers, { Authorization: 'fixture' })
      assert.equal(payload.webhook.secret, 'new-secret')
      await wait(
        '!document.querySelector(".settings-page [aria-label=\\\"通知设置\\\"] > .m3-row md-filled-button").disabled'
      )
      await click('.settings-page [aria-label="通知设置"] .m3-actions md-outlined-button')
      await until(
        () => calls.some((c) => c.path.endsWith('/webhook/test')),
        'Explicit notification test'
      )
      assert.equal(calls.filter((c) => c.path.endsWith('/test')).length, 1)
      await evaluate(`document.querySelector('.shell-main').scrollTop=0`)
      await snapshot('settings-dark-desktop')
      await send('Emulation.setDeviceMetricsOverride', {
        width: 390,
        height: 844,
        deviceScaleFactor: 1,
        mobile: true
      })
      await noOverflow()
      await snapshot('settings-dark-mobile')
      await evaluate('location.hash="#/sms"')
      await wait('document.querySelectorAll(".sms-thread").length===2')
      assert.equal(
        await evaluate('document.querySelectorAll(".sms-unread").length'),
        0,
        'Read markers survive navigation and responsive layout changes'
      )
      await click('.sms-thread:first-child .sms-thread-main')
      await wait('!!document.querySelector("md-outlined-text-field[name=sms-reply]")')
      await fill('md-outlined-text-field[name=sms-reply]', 'retain draft')
      await evaluate(
        `document.querySelector('md-outlined-text-field[name=sms-reply]').dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',isComposing:true,bubbles:true}))`
      )
      assert.equal(
        calls.filter((c) => c.path === '/api/sms/send').length,
        0,
        'IME confirmation cannot send SMS'
      )
      failNext = true
      holdNext = true
      await click('.sms-composer md-filled-button')
      await until(() => !!releaseMutation, 'Held SMS')
      await evaluate(
        `document.querySelector('.sms-composer').dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}))`
      )
      assert.equal(calls.filter((c) => c.path === '/api/sms/send').length, 1)
      releaseMutation()
      releaseMutation = null
      await wait('!document.querySelector(".sms-composer md-filled-button").disabled')
      assert.equal(
        await evaluate('document.querySelector("md-outlined-text-field[name=sms-reply]").value'),
        'retain draft'
      )
      assert.deepEqual(calls.find((c) => c.path === '/api/sms/send').body, {
        imsi: 'fixture-imsi',
        phone: 'Alice',
        message: 'retain draft'
      })
      await click('.sms-message-meta md-icon-button')
      await wait(`document.querySelector('${confirm}').open`)
      await click(confirm + ' [slot=actions] md-text-button')
      await wait(closed(confirm))
      assert.equal(calls.filter((c) => c.method === 'DELETE').length, 0)
      // Long feedback remains keyboard-scrollable and does not cover the mobile composer.
      await evaluate(
        `(async()=>{const {notify}=await import('/src/stores/feedback.ts');notify({message:'Long failure '+('diagnostic detail\\n'.repeat(100)),tone:'error'})})()`
      )
      await wait(`document.querySelector('.snackbar-message')?.tabIndex===0`)
      for (const height of [844, 480]) {
        await send('Emulation.setDeviceMetricsOverride', {
          width: 390,
          height,
          deviceScaleFactor: 1,
          mobile: true
        })
        await evaluate(`document.querySelector('.sms-composer').scrollIntoView({block:'end'})`)
        await wait(
          `(()=>{const bar=document.querySelector('.snackbar').getBoundingClientRect(),anchor=document.querySelector('.sms-composer').getBoundingClientRect();return bar.bottom<=anchor.top-8 && bar.top>=0 && bar.right<=innerWidth})()`
        )
        assert.equal(
          await evaluate(
            `(()=>{const text=document.querySelector('.snackbar-message');return text.scrollHeight>text.clientHeight && text.clientHeight<=240})()`
          ),
          true
        )
      }
      await evaluate(`document.querySelector('.snackbar-message').focus()`)
      await key('PageDown', 'PageDown', 34)
      await wait(`document.querySelector('.snackbar-message').scrollTop>0`)
      await snapshot('sms-long-feedback-mobile')
      await click('.snackbar md-text-button')
      await wait(`!document.querySelector('.snackbar')`)
      await send('Emulation.setDeviceMetricsOverride', {
        width: 390,
        height: 844,
        deviceScaleFactor: 1,
        mobile: true
      })
      await noOverflow()
      await snapshot('sms-dark-mobile')
      await send('Emulation.setDeviceMetricsOverride', {
        width: 1440,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false
      })
      await wait('!!document.querySelector(".sms-devices")')
      await snapshot('sms-dark-desktop')

      // Actual public device components, with no hardware and no replacement service logic.
      await send('Page.navigate', { url: url + 'fixture.html' })
      await wait(
        'document.querySelectorAll(".esim-profile").length===3 && !!window.feedbackFixture'
      )
      assert.equal(await evaluate('document.querySelector(".fixture .refresh-icon").parentElement.textContent.trim()'),'刷新','eSIM keeps its refresh action and label')
      const writesBefore = calls.filter((c) => c.method !== 'GET').length
      assert.equal(
        await evaluate(
          'document.querySelector(".fixture > section:last-of-type md-outlined-select").disabled'
        ),
        true
      )
      await click('.esim-profile:nth-of-type(2) md-text-button.danger')
      const deviceConfirm = '#fixture > md-dialog'
      await wait(`document.querySelector('${deviceConfirm}').open`)
      assert.equal(
        await evaluate(`document.querySelector('${deviceConfirm} md-filled-button').disabled`),
        true
      )
      await fill(deviceConfirm + ' md-outlined-text-field', '0000')
      assert.equal(
        await evaluate(`document.querySelector('${deviceConfirm} md-filled-button').disabled`),
        true
      )
      await fill(deviceConfirm + ' md-outlined-text-field', '5678')
      await click(deviceConfirm + ' md-filled-button')
      await until(
        () => calls.some((c) => c.method === 'DELETE' && c.path.endsWith('5678')),
        'Typed eSIM deletion'
      )
      await wait(closed(deviceConfirm))
      await wait('document.querySelectorAll(".esim-profile").length===2')
      assert.equal(calls.find((c) => c.method === 'DELETE').query.aid_hex, 'A00001')
      assert.equal(
        calls.filter((c) => c.method !== 'GET').length,
        writesBefore + 1,
        'Entering subpages cannot switch/download eSIM or send AT/USSD'
      )
      await click('.esim-profile:last-of-type .m3-actions md-text-button:nth-of-type(2)')
      await wait('document.querySelectorAll(".esim-profile md-switch").length===3')
      failNext = true
      holdNext = true
      await click('.esim-profile md-switch')
      await until(() => !!releaseMutation, 'Held card policy update')
      assert.equal(
        calls.filter((c) => c.method === 'PUT' && c.path.startsWith('/api/cards/')).length,
        1
      )
      releaseMutation()
      releaseMutation = null
      await wait(
        `!document.querySelector('.esim-profile md-switch').selected && !document.querySelector('.esim-profile md-switch').shadowRoot.querySelector('input').checked`
      )
      await wait(`!!document.querySelector('.esim-profile .m3-error')`)
      await click('.esim-profile .m3-actions md-outlined-button')
      await wait(`document.querySelector('${deviceConfirm}').open`)
      await click(deviceConfirm + ' md-filled-button')
      await until(
        () => calls.some((c) => c.path.endsWith('/esim/actions/switch')),
        'Explicit switch'
      )
      await wait(closed(deviceConfirm))
      await wait(
        'document.querySelector(".esim-page .m3-notice")?.textContent.includes("切卡处理中")'
      )
      assert.equal(
        await evaluate(
          'document.querySelector(".esim-profile .m3-actions md-outlined-button").disabled'
        ),
        true
      )
      await fill('md-outlined-text-field[name=at-command]', 'AT+CSQ')
      await click('[aria-label="AT 终端"] md-filled-button')
      await until(() => calls.some((c) => c.path.endsWith('/actions/at')), 'Explicit AT')
      assert.deepEqual(calls.find((c) => c.path.endsWith('/actions/at')).body, {
        cmd: 'AT+CSQ',
        timeout_ms: 10000
      })
      await fill('md-outlined-text-field[name=ussd-command]', '*100#')
      await click('[aria-label="USSD 终端"] md-filled-button')
      await wait('!!document.querySelector("[aria-label=\\\"USSD 终端\\\"] .m3-notice")')
      await fill('md-outlined-text-field[name=ussd-command]', '1')
      await click('[aria-label="USSD 终端"] md-filled-button')
      await until(() => calls.some((c) => c.path.endsWith('/ussd/continue')), 'USSD continuation')
      assert.deepEqual(calls.find((c) => c.path.endsWith('/ussd/continue')).body, {
        session_id: 'fixture-session',
        input: '1',
        timeout_ms: 45000
      })
      await evaluate(
        `window.progress=feedbackFixture.notify({message:'progress',duration:0});feedbackFixture.notify({message:'new result'});progress.close();void 0`
      )
      await wait('document.querySelector(".snackbar")?.textContent.includes("new result")')
      // Copy controls retain native button keyboard semantics without submitting their form.
      await evaluate(
        `window.copies=[];Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>{window.copies.push(text)}}});document.querySelector('#field-copy button').focus();void 0`
      )
      assert.equal(
        await evaluate(`document.querySelector('#field-copy button').getAttribute('aria-label')`),
        '复制测试字段：fixture-copy'
      )
      assert.equal(
        await evaluate(`document.activeElement===document.querySelector('#field-copy button')`),
        true,
        'Copy button receives focus'
      )
      await send('Input.dispatchKeyEvent', {
        type: 'keyDown',
        key: 'Enter',
        code: 'Enter',
        windowsVirtualKeyCode: 13,
        text: '\r'
      })
      await send('Input.dispatchKeyEvent', {
        type: 'keyUp',
        key: 'Enter',
        code: 'Enter',
        windowsVirtualKeyCode: 13
      })
      await until(() => evaluate('window.copies.length===1'), 'Enter copies the field')
      await key(' ', 'Space', 32)
      await until(() => evaluate('window.copies.length===2'), 'Space copies the field')
      assert.deepEqual(await evaluate('window.copies'), ['fixture-copy', 'fixture-copy'])
      assert.equal(await evaluate('window.fieldSubmits'), 0)
      assert.equal(
        await evaluate(
          `document.querySelectorAll('#field-empty button,#field-plain button').length`
        ),
        0
      )
      await send('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-reduced-motion', value: 'reduce' }]
      })
      assert.equal(
        await evaluate(`getComputedStyle(document.querySelector('#status-motion')).animationName`),
        'none'
      )
      await send('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-reduced-motion', value: 'no-preference' }]
      })
      assert.notEqual(
        await evaluate(`getComputedStyle(document.querySelector('#status-motion')).animationName`),
        'none'
      )
      await send('Emulation.setDeviceMetricsOverride', {
        width: 390,
        height: 600,
        deviceScaleFactor: 1,
        mobile: true
      })
      await evaluate(
        `feedbackFixture.requestConfirmation({title:'First',message:'one'}).catch(v=>feedbackFixture.results.push(v));feedbackFixture.requestConfirmation({title:'Second',message:'<img src=x onerror=alert(1)>',alert:true}).then(()=>feedbackFixture.results.push('accepted'));void 0`
      )
      await wait(`document.querySelector('${deviceConfirm}').open`)
      await evaluate(
        `window.previousFocus=document.activeElement;feedbackFixture.notify({message:'Modal failure '+('diagnostic details\\n'.repeat(100)),tone:'error'});void 0`
      )
      await wait('document.querySelector(".snackbar")?.textContent.includes("Modal failure")')
      assert.equal(
        await evaluate('document.activeElement===window.previousFocus'),
        true,
        'Showing a notice must not steal modal focus'
      )
      await wait(
        `(()=>{const bar=document.querySelector('.snackbar').getBoundingClientRect(),actions=document.querySelector('#fixture > md-dialog [slot=actions]').getBoundingClientRect();return bar.bottom<=actions.top-8 && bar.top>=0})()`
      )
      await snapshot('modal-long-feedback-mobile')
      await click('.snackbar md-text-button')
      await wait('!document.querySelector(".snackbar")')
      await key('Escape', 'Escape', 27)
      await wait(
        `document.querySelector('${deviceConfirm} [slot=headline]').textContent==='Second'`
      )
      assert.deepEqual(await evaluate('feedbackFixture.results'), ['cancel'])
      assert.equal(
        await evaluate(`document.querySelectorAll('${deviceConfirm} img').length`),
        0,
        'Confirmation text must be escaped'
      )
      await click(deviceConfirm + ' md-filled-button')
      await wait(closed(deviceConfirm))
      assert.deepEqual(await evaluate('feedbackFixture.results'), ['cancel', 'accepted'])
      await send('Emulation.setDeviceMetricsOverride', {
        width: 1440,
        height: 1000,
        deviceScaleFactor: 1,
        mobile: false
      })
      await evaluate('document.documentElement.classList.remove("dark");window.scrollTo(0,0)')
      await snapshot('esim-light-desktop')
      await send('Emulation.setDeviceMetricsOverride', {
        width: 390,
        height: 844,
        deviceScaleFactor: 1,
        mobile: true
      })
      await noOverflow()
      await snapshot('device-subpages-light-mobile')
      assert.equal(exceptions.length, 0, JSON.stringify(exceptions))
    } catch (error) {
      if (debugEvaluate)
        console.error(
          (await debugEvaluate('document.body.innerText')).slice(0, 3000),
          JSON.stringify(exceptions),
          await debugEvaluate(
            `({copies:window.copies,submits:window.fieldSubmits,active:document.activeElement?.outerHTML})`
          )
        )
      if (debugSend)
        await writeFile(
          path.join(artifactDir, 'failure.png'),
          Buffer.from((await debugSend('Page.captureScreenshot', { format: 'png' })).data, 'base64')
        )
      throw error
    } finally {
      socket?.close()
      browser?.kill('SIGTERM')
      releaseMutation?.()
      await server.close()
      await rm(profile, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
    }
  }
)
