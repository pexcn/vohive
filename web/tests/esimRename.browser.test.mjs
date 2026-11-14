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

test('eSIM rename reuses overview reads and preserves saved names on refresh failure', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout: 60000
}, async () => {
  const root = fileURLToPath(new URL('../', import.meta.url))
  process.chdir(root)
  const profile = await mkdtemp(path.join(tmpdir(), 'vohive-esim-rename-'))
  const fixture = path.join(root, '__esim_rename_fixture.vue')
  const calls = []
  let name = 'Original target'
  let releaseRefresh, failRefresh = false, failPatch = false
  const groups = device => [
    {eid:'eid-a',aid_hex:'A00001',profiles:[{iccid:'8900000000000005678',name:device==='fixture'?name:'Second device profile',state:0}]},
    {eid:'eid-b',aid_hex:'A00002',profiles:[{iccid:'8900000000000005678',name:'Other chip',state:0}]}
  ]
  const server = await createServer({
    root, configFile:path.join(root,'vite.config.ts'),
    server:{host:'127.0.0.1',port:0,strictPort:false},
    plugins:[{
      name:'esim-rename-fixture',
      resolveId(id) {
        if(id==='/esim-rename-fixture.vue') return fixture
        if(id==='/esim-rename-fixture.ts') return path.join(root,'__esim_rename_fixture.ts')
      },
      load(id) {
        if(id===path.join(root,'__esim_rename_fixture.ts')) return `import{createApp}from'vue';import App from'/esim-rename-fixture.vue';import'/src/materialWeb.ts';import'/src/style.css';import'/src/material.css';createApp(App).mount('#app')`
        if(id!==fixture) return
        return `<script setup>
import {ref} from 'vue'
import DeviceEsimTab from '/src/components/DeviceEsimTab.vue'
import MaterialFeedback from '/src/components/MaterialFeedback.vue'
import {activeNotice} from '/src/stores/feedback'
const device=ref('fixture')
window.setRenameDevice=value=>device.value=value
window.renameNotice=()=>activeNotice.value
</script><template><main><DeviceEsimTab :device-id="device" :is-active="true" :device-online="true"/></main><MaterialFeedback/></template><style>main{max-width:1100px;margin:auto;padding:16px}</style>`
      },
      configureServer(dev) {
        dev.middlewares.use(async(req,res,next)=>{
          const url=new URL(req.url,'http://fixture')
          if(url.pathname.startsWith('/api/')) {
            let raw='';for await(const chunk of req) raw+=chunk
            const body=raw?JSON.parse(raw):{}
            calls.push({path:url.pathname,query:url.search,method:req.method,body})
            const json=data=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data))}
            const device=url.pathname.split('/')[3]
            if(req.method==='PATCH') {
              if(failPatch){res.statusCode=500;json({error:'Fixture nickname rejected'});return}
              name=body.name;json({status:'ok'});return
            }
            if(url.pathname.endsWith('/esim/profiles')) {
              const result=groups(device), failed=failRefresh
              await new Promise(resolve=>{releaseRefresh=resolve})
              if(failed){res.statusCode=500;json({error:'Fixture logical channel unavailable'});return}
              json(result);return
            }
            if(url.pathname.endsWith('/esim')) {
              json({chip_info:{sku_name:'Fixture eUICC',eids:[{eid:'eid-a',aid:'A00001'},{eid:'eid-b',aid:'A00002'}]},profiles:groups(device)});return
            }
            res.statusCode=404;json({error:'Unexpected endpoint'});return
          }
          if(!url.pathname.startsWith('/esim-rename-fixture.html')) return next()
          const html=await dev.transformIndexHtml('/esim-rename-fixture.html',`<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="/favicon.svg"></head><body><div id="app"></div><script type="module" src="/esim-rename-fixture.ts"></script></body></html>`)
          res.setHeader('Content-Type','text/html');res.end(html)
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
    inspect = () => evaluate(`({body:document.body.innerText,active:document.activeElement?.tagName,notice:window.renameNotice?.()})`)
    const wait = expression => until(() => evaluate(expression), expression)
    const click = async selector => {
      const point = await evaluate(`(()=>{const el=document.querySelector(${JSON.stringify(selector)});el.scrollIntoView({block:'center'});const r=el.getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()`)
      await send('Input.dispatchMouseEvent', { type: 'mousePressed', button: 'left', clickCount: 1, ...point })
      await send('Input.dispatchMouseEvent', { type: 'mouseReleased', button: 'left', clickCount: 1, ...point })
    }
    const names=()=>evaluate(`Array.from(document.querySelectorAll('.esim-profile h4'),el=>el.textContent.trim())`)
    const patchCalls=()=>calls.filter(c=>c.method==='PATCH')
    const profileReads=()=>calls.filter(c=>c.path.endsWith('/esim/profiles'))
    const rename=async newName=>{
      await evaluate(`window.renameMenuOpened=false;document.querySelector('.esim-profile-menu').addEventListener('opened',()=>window.renameMenuOpened=true,{once:true})`)
      await click('.esim-profile-more');await wait('window.renameMenuOpened')
      await click('.esim-profile-menu md-menu-item:not(.danger)')
      await wait(`!!document.querySelector('.esim-rename-form md-outlined-text-field')?.shadowRoot?.querySelector('input')`)
      await evaluate(`document.querySelector('.esim-rename-form md-outlined-text-field').focus()`)
      await send('Input.dispatchKeyEvent',{type:'keyDown',key:'a',code:'KeyA',modifiers:2,windowsVirtualKeyCode:65})
      await send('Input.dispatchKeyEvent',{type:'keyUp',key:'a',code:'KeyA',modifiers:2,windowsVirtualKeyCode:65})
      await send('Input.insertText',{text:newName})
      const previous=patchCalls().length
      await click('.esim-rename-form md-filled-button')
      await until(()=>patchCalls().length===previous+1,'rename PATCH')
    }
    const pendingRefresh=async()=>{
      await until(()=>!!releaseRefresh,'pending profile refresh')
      assert.equal(profileReads().at(-1).query,'','reuse the invalidated overview without forcing a second scan')
    }
    const finishRefresh=()=>{releaseRefresh();releaseRefresh=undefined}
    await send('Runtime.enable');await send('Page.enable');await send('Page.bringToFront')
    await send('Page.navigate',{url:`http://127.0.0.1:${server.httpServer.address().port}/esim-rename-fixture.html`})
    await wait(`document.querySelectorAll('.esim-profile').length===2`)
    // The acknowledged name is visible before the list request completes.
    await rename('Saved name');await pendingRefresh()
    await wait(`document.querySelector('.esim-profile h4')?.textContent.trim()==='Saved name'`)
    assert.deepEqual(await names(),['Saved name','Other chip'],'only the selected AID is renamed')
    assert.equal(await evaluate('window.renameNotice().tone'),'success')
    assert.deepEqual(patchCalls().at(-1).body,{name:'Saved name',aid_hex:'A00001'})
    finishRefresh()
    await wait(`document.querySelector('.esim-toolbar .refresh-icon')?.classList.contains('busy')===false && !document.querySelector('.esim-rename-form')`)
    // A read failure must not undo the successful write or report it as failed.
    failRefresh=true
    await rename('Saved despite read failure');await pendingRefresh()
    await wait(`document.querySelector('.esim-profile h4')?.textContent.trim()==='Saved despite read failure'`)
    finishRefresh()
    await wait(`window.renameNotice()?.tone==='warning'`)
    assert.match(await evaluate('window.renameNotice().message'),/名称已保存，但列表刷新失败.*Fixture logical channel unavailable/)
    assert.deepEqual(await names(),['Saved despite read failure','Other chip'])
    // A rejected PATCH retains the editor and draft and never fetches a list.
    failPatch=true
    const readsBefore=profileReads().length
    await rename('Rejected name')
    await wait(`window.renameNotice()?.tone==='error'`)
    assert.match(await evaluate('window.renameNotice().message'),/Fixture nickname rejected/)
    assert.equal(await evaluate(`document.querySelector('.esim-rename-form md-outlined-text-field').value`),'Rejected name')
    assert.equal(profileReads().length,readsBefore)
    await click('.esim-rename-form md-text-button')
    await wait(`document.querySelector('.esim-profile h4')?.textContent.trim()==='Saved despite read failure'`)
    // A late refresh for the previous device must not overwrite a new device.
    failPatch=false;failRefresh=false
    await rename('Previous device name');await pendingRefresh()
    await evaluate(`window.setRenameDevice('second')`)
    await wait(`document.querySelector('.esim-profile h4')?.textContent.trim()==='Second device profile'`)
    finishRefresh()
    await wait(`document.querySelector('.esim-toolbar .refresh-icon')?.classList.contains('busy')===false`)
    assert.deepEqual(await names(),['Second device profile','Other chip'])
    assert.equal(errors.length,0,JSON.stringify(errors))
  } catch (error) {
    if(inspect) console.error(await inspect())
    throw error
  } finally {
    releaseRefresh?.()
    socket?.close(); browser?.kill('SIGTERM')
    await server.close()
    await rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:100})
  }
})
