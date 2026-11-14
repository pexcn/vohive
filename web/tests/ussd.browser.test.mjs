import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { spawn, spawnSync } from 'node:child_process'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const chromium = process.env.CHROMIUM_BIN || 'chromium'
const available = typeof WebSocket === 'function' && spawnSync(chromium, ['--version']).status === 0

test('USSD preserves composer focus and recovers from ended sessions', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout: 60000
}, async () => {
  const root = fileURLToPath(new URL('../', import.meta.url))
  process.chdir(root)
  const profile = await mkdtemp(path.join(tmpdir(), 'vohive-ussd-'))
  const fixture = path.join(root, '__ussd_fixture.vue')
  const calls = []
  let holdNext = false, release
  let nextCancelError, nextContinueError
  const server = await createServer({
    root, configFile: path.join(root, 'vite.config.ts'),
    server: { host: '127.0.0.1', port: 0, strictPort: false },
    plugins: [{
      name: 'ussd-fixture',
      resolveId(id) {
        if (id === '/ussd-fixture.vue') return fixture
        if (id === '/ussd-fixture.ts') return path.join(root, '__ussd_fixture.ts')
      },
      load(id) {
        if (id === path.join(root, '__ussd_fixture.ts')) return `import {createApp} from 'vue';import App from '/ussd-fixture.vue';import '/src/materialWeb.ts';import '/src/style.css';import '/src/material.css';createApp(App).mount('#app')`
        if (id !== fixture) return
        return `<script setup>
import {ref} from 'vue'
import DeviceUssdTab from '/src/components/DeviceUssdTab.vue'
const active=ref(true), mounted=ref(true)
window.setUssdActive=value=>active.value=value
window.unmountUssd=()=>mounted.value=false
</script><template><main><button id="other">其他控件</button><DeviceUssdTab v-if="mounted" v-show="active" device-id="fixture" :is-active="active" :vowifi-active="true"/></main></template><style>main{max-width:1000px;margin:auto;padding:16px}</style>`
      },
      configureServer(dev) {
        dev.middlewares.use(async (req, res, next) => {
          if (req.url?.startsWith('/api/')) {
            let raw=''; for await (const chunk of req) raw+=chunk
            const body=raw?JSON.parse(raw):{}
            calls.push({path:req.url,body})
            if(holdNext) {holdNext=false;await new Promise(resolve=>{release=resolve})}
            res.setHeader('Content-Type','application/json')
            const failure=req.url.endsWith('/ussd/cancel')?nextCancelError:req.url.endsWith('/ussd/continue')?nextContinueError:undefined
            if(failure) {
              nextCancelError=nextContinueError=undefined
              res.statusCode=500;res.end(JSON.stringify({status:'error',...failure}));return
            }
            if(req.url.endsWith('/ussd/cancel')) res.end(JSON.stringify({status:'ok'}))
            else res.end(JSON.stringify({status:'ok',channel:'vowifi',result:{status:1,text:'1. Balance\\n2. Usage',session_id:'fixture-session'}}))
            return
          }
          if (!req.url?.startsWith('/ussd-fixture.html')) return next()
          const html = await dev.transformIndexHtml('/ussd-fixture.html', `<!doctype html><html><head><meta name="theme-color" content="#f9f6ef"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="/favicon.svg"></head><body><div id="app"></div><script type="module" src="/ussd-fixture.ts"></script></body></html>`)
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
    inspect = () => evaluate(`({body:document.body.innerText,active:document.activeElement?.outerHTML})`)
    const wait = expression => until(() => evaluate(expression), expression)
    const click = async selector => {
      const point = await evaluate(`(()=>{const el=document.querySelector(${JSON.stringify(selector)});el.scrollIntoView({block:'center'});const r=el.getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()`)
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', button: 'left', clickCount: 1, ...point })
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', button: 'left', clickCount: 1, ...point })
    }
    const field = 'md-outlined-text-field[name=ussd-command]'
    const focused = `document.activeElement === document.querySelector('${field}')`
    const ready = `!document.querySelector('${field}').readOnly && !document.querySelector('${field}').disabled`
    const key = async (key, code = key, windowsVirtualKeyCode = 0) => {
      await send('Input.dispatchKeyEvent', {type:'keyDown',key,code,windowsVirtualKeyCode,...(key==='Enter'?{text:'\r'}:{})})
      await send('Input.dispatchKeyEvent', {type:'keyUp',key,code,windowsVirtualKeyCode})
    }
    const enter = () => key('Enter','Enter',13)
    const type = text => send('Input.insertText',{text})
    const focusField = () => evaluate(`document.querySelector('${field}').focus()`)
    await send('Runtime.enable'); await send('Page.enable'); await send('Page.bringToFront')
    await send('Page.navigate',{url:`http://127.0.0.1:${server.httpServer.address().port}/ussd-fixture.html`})
    await wait(`document.querySelector('${field}')?.shadowRoot?.querySelector('input')`)
    await focusField(); await type('*100#')
    holdNext=true; await enter()
    await until(()=>!!release,'pending initial USSD request')
    assert.equal(await evaluate(focused),true,'sending preserves the native input focus')
    assert.equal(await evaluate(`document.querySelector('${field}').readOnly`),true)
    await enter()
    assert.equal(calls.length,1,'Enter during a pending request cannot submit twice')
    release(); release=undefined
    await wait(`!!document.querySelector('.ussd-session-strip') && ${ready} && ${focused}`)
    assert.equal(await evaluate(`document.querySelector('${field}').value`),'')
    for(const answer of ['1','2']) {
      await type(answer); await enter()
      await until(()=>calls.filter(c=>c.path.endsWith('/ussd/continue')).length===Number(answer),'menu continuation '+answer)
      await wait(`${ready} && ${focused}`)
    }
    assert.deepEqual(calls.map(c=>c.body.command||c.body.input),['*100#','1','2'])
    // The send button also returns focus when the user remains in the composer.
    await type('3'); await click('.ussd-send'); await wait(`${ready} && ${focused}`)
    // A slow response must not take focus back from another control.
    holdNext=true; await type('4'); await enter()
    await until(()=>!!release,'pending menu request')
    await click('#other'); release(); release=undefined
    await wait(ready)
    assert.equal(await evaluate('document.activeElement.id'),'other')
    // Leaving the tab while a response is pending prevents late autofocus.
    await focusField(); await type('5'); holdNext=true; await enter()
    await until(()=>!!release,'pending request before hiding the tab')
    await evaluate('window.setUssdActive(false)')
    await click('#other'); release(); release=undefined
    await wait(ready)
    assert.equal(await evaluate('document.activeElement.id'),'other')
    await evaluate('window.setUssdActive(true)')
    // Success also represents a peer's 481 or an already removed dialog.
    await focusField(); await click('.ussd-cancel')
    await wait(`!document.querySelector('.ussd-session-strip') && ${ready} && ${focused}`)
    const startSession = async () => {
      await focusField(); await type('*100#'); await enter()
      await wait(`!!document.querySelector('.ussd-session-strip') && ${ready} && ${focused}`)
    }
    // A failed BYE can still terminate the local dialog: retain the error,
    // clear the stale session and allow another initial request immediately.
    await startSession(); await type('draft')
    nextCancelError={code:'USSD_SESSION_ENDED',message:'USSD BYE rejected: SIP/2.0 500 Server Error'}
    await click('.ussd-cancel')
    await wait(`!document.querySelector('.ussd-session-strip') && ${ready} && ${focused}`)
    assert.match(await evaluate('document.body.innerText'),/500 Server Error/)
    assert.equal(await evaluate(`document.querySelector('${field}').value`),'')
    // Failure before touching the dialog must stay visible and permit retry.
    await startSession()
    nextCancelError={message:'VoWiFi service unavailable'}
    await click('.ussd-cancel'); await wait(`${ready} && ${focused}`)
    assert.ok(await evaluate(`!!document.querySelector('.ussd-session-strip')`))
    assert.match(await evaluate('document.body.innerText'),/VoWiFi service unavailable/)
    await click('.ussd-cancel')
    await wait(`!document.querySelector('.ussd-session-strip') && ${ready} && ${focused}`)
    // An expired continuation also leaves the next input as a fresh code.
    await startSession()
    nextContinueError={code:'USSD_SESSION_ENDED',message:'USSD INFO rejected: SIP/2.0 481 Call/Transaction Does Not Exist'}
    await type('1'); await enter()
    await wait(`!document.querySelector('.ussd-session-strip') && ${ready} && ${focused}`)
    assert.match(await evaluate('document.body.innerText'),/481 Call\/Transaction Does Not Exist/)
    await startSession()
    assert.ok(calls.at(-1).path.endsWith('/actions/ussd'),'new code starts a new dialog after expiry')
    await click('.ussd-cancel')
    await wait(`!document.querySelector('.ussd-session-strip') && ${ready}`)
    assert.equal(errors.length,0,JSON.stringify(errors))
  } catch (error) {
    if(inspect) console.error(await inspect())
    throw error
  } finally {
    release?.()
    socket?.close(); browser?.kill('SIGTERM')
    await server.close()
    await rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:100})
  }
})
