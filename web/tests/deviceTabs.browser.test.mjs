import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'
import { spawn, spawnSync } from 'node:child_process'
import { mkdtemp, readFile, writeFile, rm, mkdir } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const chromium = process.env.CHROMIUM_BIN || 'chromium'
const available = typeof WebSocket === 'function' && spawnSync(chromium,['--version']).status === 0

test('Device tabs synchronize URLs and preserve drafts across tab navigation', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout:60000
}, async () => {
const root=fileURLToPath(new URL('../',import.meta.url))
process.chdir(root)
const profile=await mkdtemp(path.join(tmpdir(),'vohive-pages-'))
const artifactDir=process.env.VOHIVE_SCREENSHOTS || path.join(profile,'screenshots')
let browser,socket,debugSend,debugEvaluate
const calls = [], streams = new Set()
const fixtures = ['a','b'].map(id=>({
  id,name:'Fixture '+id,running:true,healthy:true,lifecycle_phase:'online',backend_mode:'qmi',
  at_port:'/dev/fixture-at',interface:'wwan0',network_enabled:false,modem:{imei:'fixture-imei-'+id}
}))
async function mockApi(req,res) {
  const url = new URL(req.url,'http://localhost')
  calls.push({method:req.method,path:url.pathname})
  const json=data=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data))}
  // Every endpoint is local; tab navigation must not send any device action.
  if(req.method!=='GET'){res.statusCode=500;json({error:'Unexpected mutation'});return}
  if(url.pathname.endsWith('/stream')){
    res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'})
    res.write(': connected\n\n');streams.add(res);res.on('close',()=>streams.delete(res));return
  }
  if(url.pathname==='/api/devices'){json({devices:fixtures,device_limit:5});return}
  const fixture=fixtures.find(d=>url.pathname.startsWith('/api/devices/'+d.id+'/'))
  if(fixture&&url.pathname.endsWith('/overview')){json({devices:[fixture]});return}
  if(fixture&&url.pathname.endsWith('/config')){json({config:{id:fixture.id,name:fixture.name,device_backend:'qmi'}});return}
  if(fixture&&url.pathname.endsWith('/esim')){json({chip_info:{sku_name:'Fixture eUICC',eids:[]},profiles:[]});return}
  json({})
}

const server = await createServer({
  root,configFile:path.join(root,'vite.config.ts'),
  server:{host:'127.0.0.1',port:0,strictPort:false},
  plugins:[{name:'material-pages-fixture',configureServer(dev) {
    dev.middlewares.use(async (req,res,next)=>{
      if(req.url?.startsWith('/api/')) { await mockApi(req,res); return }
      next()
    })
  }}]
})
const delay = ms => new Promise(resolve=>setTimeout(resolve,ms));
async function until(check, label, timeout=20000) {
  const end = Date.now()+timeout;
  while(Date.now()<end) { const result=await check(); if(result) return result; await delay(50); }
  throw new Error('Timed out: '+label);
}
try {
  await server.listen();
  browser = spawn(chromium,['--headless=new','--no-sandbox','--disable-dev-shm-usage','--no-first-run','--remote-debugging-port=0','--user-data-dir='+profile,'about:blank'],{stdio:'ignore'});
  const port = await until(async()=>{try{return (await readFile(path.join(profile,'DevToolsActivePort'),'utf8')).split('\n')[0]}catch{return null}},'Chromium startup');
  const targets = await (await fetch('http://127.0.0.1:'+port+'/json/list')).json();
  socket = new WebSocket(targets.find(t=>t.type==='page').webSocketDebuggerUrl);
  await new Promise((resolve,reject)=>{socket.onopen=resolve;socket.onerror=reject});
  let id=0; const pending=new Map(); const exceptions=[];
  socket.onmessage = event => {
    const message=JSON.parse(event.data);
    if(message.id) {const waiter=pending.get(message.id);pending.delete(message.id);if(message.error)waiter.reject(message.error);else waiter.resolve(message.result);}
    if(message.method==='Runtime.exceptionThrown') exceptions.push(message.params.exceptionDetails);
  };
  const send=(method,params={})=>new Promise((resolve,reject)=>{const n=++id;pending.set(n,{resolve,reject});socket.send(JSON.stringify({id:n,method,params}))});
  const evaluate=async expression=>{const result=await send('Runtime.evaluate',{expression,awaitPromise:true,returnByValue:true});if(result.exceptionDetails)throw new Error(JSON.stringify(result.exceptionDetails));return result.result.value};
  const wait=async expression=>until(()=>evaluate(expression),expression);
  const click=async selector=>{
    const ariaMatch=selector.match(/^md-icon-button\[aria-label="([^"]+)"\]$/);
    const target=ariaMatch?`Array.from(document.querySelectorAll('md-icon-button')).find(el=>el.ariaLabel===${JSON.stringify(ariaMatch[1])})`:`document.querySelector(${JSON.stringify(selector)})`;
    const rect=await until(()=>evaluate(`(async()=>{const el=${target};if(!el)return null;await el.updateComplete;if(!el.checkVisibility())return null;const modal=el.closest('md-dialog');if(modal){await Promise.all(modal.shadowRoot.querySelector('dialog').getAnimations({subtree:true}).map(a=>a.finished.catch(()=>{})))}el.scrollIntoView({block:'center'});const r=el.getBoundingClientRect();return r.width&&r.height?{x:r.x+r.width/2,y:r.y+r.height/2}:null})()`),'visible '+selector);
    await send('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...rect});
    await send('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...rect});
  };
  debugSend=send;debugEvaluate=evaluate;
  await send('Runtime.enable');await send('Page.enable');await send('Network.enable');
  await send('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'light'}]});
  await send('Network.setBlockedURLs',{urls:['https://*']});
  await send('Page.addScriptToEvaluateOnNewDocument',{source:`
    localStorage.setItem('token','fixture-token');localStorage.setItem('theme','dark');
    localStorage.setItem('go-4gproxy:mcc-mnc-table:v1',JSON.stringify({fetched_at:Date.now(),rows:[{mcc:'001',mnc:'01',iso:'',network:'Fixture',country:'Fixture',country_code:''}]}));
  `});
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  const address=server.httpServer.address();const url=`http://127.0.0.1:${address.port}/`;
  await mkdir(artifactDir,{recursive:true});
  const snapshot=async name=>{await evaluate(`Promise.all([...document.querySelectorAll('md-dialog')].flatMap(el=>el.shadowRoot?.querySelector('dialog')?.getAnimations({subtree:true}) || []).map(a=>a.finished.catch(()=>{})))`);await evaluate('document.fonts.ready');await delay(120);await writeFile(path.join(artifactDir,name+'.png'),Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64'));};

  const query = key => evaluate(`new URLSearchParams(location.hash.split('?')[1]).get(${JSON.stringify(key)})`)
  const expectTab = async tab => {
    await wait(`document.querySelector('.device-detail-tabs')?.activeTab?.dataset.tab===${JSON.stringify(tab)} && document.querySelector('#device-panel-${tab}')?.checkVisibility()`)
    assert.equal(await query('tab'),tab,'The URL matches the selected Material tab and visible panel')
  }
  const chooseTab = async tab => {await click('#device-tab-'+tab);await expectTab(tab)}
  const fill = async (selector,value) => {
    await wait(`!!document.querySelector(${JSON.stringify(selector)})?.shadowRoot?.querySelector('input')`)
    await evaluate(`(()=>{const input=document.querySelector(${JSON.stringify(selector)}).shadowRoot.querySelector('input');input.focus();input.select()})()`)
    await send('Input.insertText',{text:value})
    await wait(`document.querySelector(${JSON.stringify(selector)}).value===${JSON.stringify(value)}`)
  }
  const value = selector => evaluate(`document.querySelector(${JSON.stringify(selector)})?.value`)
  const atInput='md-outlined-text-field[name="at-command"]'
  const ussdInput='md-outlined-text-field[name="ussd-command"]'
  const esimInput='md-outlined-text-field[name="esim-smdp"]'
  const nameInput='#device-panel-config md-outlined-text-field:nth-child(2)'
  await send('Page.navigate',{url:url+'#/devices?device=a&tab=overview&keep=fixture'})
  await expectTab('overview')
  const historyLength=await evaluate('history.length')
  await evaluate('window.initialWorkspace=document.querySelector(".device-workspace")')
  const overviewRequests=()=>calls.filter(c=>c.path==='/api/devices'||c.path.includes('/overview')).length
  const beforeTabs=overviewRequests()
  for(const tab of ['esim','at','ussd','card','config','overview']) {
    await chooseTab(tab)
    assert.equal(await query('device'),'a')
    assert.equal(await query('keep'),'fixture','Tab changes retain unrelated query parameters')
    assert.equal(await evaluate('history.length'),historyLength,'Tabs replace the current entry without adding history')
    assert.equal(await evaluate('window.initialWorkspace===document.querySelector(".device-workspace")'),true,'Changing tabs keeps the device workspace mounted')
  }
  assert.equal(overviewRequests(),beforeTabs,'Tab query updates do not reload the device list or reconnect overview SSE')
  await chooseTab('at');await fill(atInput,'AT+CSQ')
  await chooseTab('ussd');await fill(ussdInput,'*100#')
  await chooseTab('esim')
  await click('.esim-download-entry md-filled-tonal-button')
  await fill(esimInput,'smdp.example.test')
  await click('.esim-download-dialog [slot=actions] md-text-button')
  await wait('!document.querySelector(".esim-download-dialog").shadowRoot.querySelector("dialog").open')
  await chooseTab('config');await fill(nameInput,'Unsaved device name')
  for(const [tab,selector,draft] of [
    ['at',atInput,'AT+CSQ'],['ussd',ussdInput,'*100#'],
    ['esim',esimInput,'smdp.example.test'],['config',nameInput,'Unsaved device name']
  ]) {
    await chooseTab(tab)
    await wait(`!!document.querySelector(${JSON.stringify(selector)})`)
    assert.equal(await value(selector),draft,'Tab navigation retains '+tab+' input')
  }
  await snapshot('device-tab-config-draft')
  // Native keyboard activation must use the same synchronization path.
  await evaluate("document.querySelector('#device-tab-at').focus()")
  for(const type of ['keyDown','keyUp']) await send('Input.dispatchKeyEvent',{type,key:'Enter',code:'Enter',windowsVirtualKeyCode:13})
  await expectTab('at')
  assert.equal(await value(atInput),'AT+CSQ')
  // External URL navigation, including history, updates the existing workspace.
  await evaluate('location.hash="#/devices?device=a&tab=ussd&keep=fixture"')
  await expectTab('ussd')
  await evaluate('history.back()');await expectTab('at')
  await evaluate('history.forward()');await expectTab('ussd')
  assert.equal(await value(ussdInput),'*100#')
  await evaluate('location.hash="#/devices?device=a&keep=fixture"')
  await wait('document.querySelector("#device-panel-overview")?.checkVisibility()')
  assert.equal(await evaluate('document.querySelector(".device-detail-tabs").activeTab.dataset.tab'),'overview','Removing the tab query resets the overview')
  await evaluate('location.hash="#/devices?device=a&tab=invalid"')
  await wait('document.querySelector(".device-detail-tabs")?.activeTab?.dataset.tab==="overview"')
  await evaluate('location.hash="#/devices?device=a&tab=esim"')
  await expectTab('esim')
  const oldOrigin=await evaluate('performance.timeOrigin')
  await send('Page.reload')
  await until(async()=>{try{return await evaluate(`performance.timeOrigin!==${oldOrigin} && document.querySelector('#device-panel-esim')?.checkVisibility()`)}catch{return false}},'Reload restores the linked tab')
  await expectTab('esim')
  assert.equal(await evaluate('document.querySelector(".device-option[aria-pressed=true]")?.textContent.includes("Fixture a")'),true)
  // An auto-selected device may have no device query; tab updates must still keep drafts.
  await evaluate('location.hash="#/devices?tab=at"')
  await expectTab('at');await fill(atInput,'AT+CREG?')
  await chooseTab('overview');await chooseTab('at')
  assert.equal(await query('device'),null)
  assert.equal(await value(atInput),'AT+CREG?')
  // Changing devices retains the previous reset behavior, isolating device drafts.
  await click('.device-option:nth-child(2)')
  await expectTab('at')
  await wait('document.querySelector(".device-option[aria-pressed=true]")?.textContent.includes("Fixture b")')
  assert.equal(await query('device'),'b')
  assert.equal(await value(atInput),'','A new device does not inherit another device terminal input')
  assert.equal(calls.filter(c=>c.method!=='GET').length,0)
  assert.equal(exceptions.length,0,JSON.stringify(exceptions))

} catch(error) {
  if(debugEvaluate) console.error((await debugEvaluate('document.body.innerText')).slice(0,3000))
  if(debugSend) await writeFile(path.join(artifactDir,'failure.png'),Buffer.from((await debugSend('Page.captureScreenshot',{format:'png'})).data,'base64'))
  throw error
} finally {
  socket?.close();browser?.kill('SIGTERM')
  for(const response of streams)response.destroy()
  await server.close()
  await rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:100})
}
})
