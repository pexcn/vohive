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

test('Material proxy and log pages retain request, stream and scroll behavior', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout:60000
}, async () => {
const root=fileURLToPath(new URL('../',import.meta.url))
process.chdir(root)
const profile=await mkdtemp(path.join(tmpdir(),'vohive-pages-'))
const artifactDir=process.env.VOHIVE_SCREENSHOTS || path.join(profile,'screenshots')
let browser,socket,debugSend,debugEvaluate
const proxies = [
  {id:'de',name:'Germany',addr:'de.example:1080',username:'user',password:'****',enabled:true},
  {id:'gb',name:'Britain',addr:'gb.example:1080',username:'',password:'',enabled:false}
]
let instances = [{id:'local',name:'Local',device_id:'modem-a',mode:'socks5',listen_addr:'0.0.0.0',listen_port:1080,enabled:true,auth_enabled:true,username:'old',password:'secret'}]
const countries = [
  {country_code:'DE',country_name:'Germany',mccs:['262']},
  {country_code:'GB',country_name:'Britain',mccs:['234','235']},
  {country_code:'US',country_name:'United States',mccs:['310','311']}
]
let rules = countries.slice(0,2).map((c,i)=>({...c,upstream_proxy_id:i?'gb':'de',enabled:true}))
let holdHistory=false, releaseHistory
let running = false, failNext = false, holdNext = false, releaseMutation
const calls = [], streams = new Set()
const logs = [
  {time:'2026-10-21T10:00:00Z',level:'info',caller:'fixture/start.go:1',message:'ready'},
  {time:'2026-10-21T10:00:01Z',level:'warn',caller:'fixture/network.go:2',message:'pending registration',fields:'device=modem-a'}
]
const analysis = { buckets: Array.from({length:24},(_,hour)=>({bucket:String(hour),period_start:`2026-10-21T${String(hour).padStart(2,'0')}:00:00Z`,rx_bytes:100,tx_bytes:50,total_bytes:150})), chart:{timestamps:[],devices:[],series:{}} }
async function mockApi(req,res) {
  const url = new URL(req.url,'http://localhost')
  let raw='';for await (const chunk of req) raw+=chunk
  const body=raw?JSON.parse(raw):null
  calls.push({method:req.method,path:url.pathname,body,query:Object.fromEntries(url.searchParams)})
  const json=(data,status=200)=>{res.statusCode=status;res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data))}
  if(req.method!=='GET') {
    if(holdNext){holdNext=false;await new Promise(resolve=>{releaseMutation=resolve})}
    if(failNext){failNext=false;json({error:'fixture rejected save'},500);return}
    if(url.pathname==='/api/proxy-instances/config'){instances=body.instances;json({});return}
    if(url.pathname.startsWith('/api/proxy-instances/local/actions/')){running=!url.pathname.endsWith('/stop');json({});return}
    if(url.pathname==='/api/upstream-proxies/de'&&req.method==='PUT'){Object.assign(proxies[0],body);json({});return}
    if(url.pathname==='/api/upstream-proxies/de'&&req.method==='DELETE'){proxies.shift();rules=rules.filter(r=>r.upstream_proxy_id!=='de');json({});return}
    if(url.pathname==='/api/upstream-proxy-country-rules/US'){
      rules=rules.filter(r=>r.country_code!=='US')
      if(req.method==='PUT')rules.push({...countries[2],...body})
      json({});return
    }
    json({error:'Unexpected fixture mutation'},500);return
  }
  if(url.pathname==='/api/dashboard/devices'){json([]);return}
  if(url.pathname==='/api/traffic/analysis'){json(analysis);return}
  if(url.pathname==='/api/proxy-instances/overview'){json({instances,devices:[{id:'modem-a',name:'Modem A',interface:'wwan0'}],status:[{id:'local',running}]});return}
  if(url.pathname==='/api/proxy-instances/local'){json(instances[0]);return}
  if(url.pathname==='/api/upstream-proxies'){json(proxies.map(p=>({...p,password:p.password?'****':''})));return}
  if(url.pathname==='/api/upstream-proxy-countries'){json(countries);return}
  if(url.pathname==='/api/upstream-proxy-country-rules'){json(rules);return}
  if(url.pathname==='/api/logs/history'){if(holdHistory){holdHistory=false;await new Promise(resolve=>{releaseHistory=resolve})}json({logs});return}
  if(url.pathname==='/api/logs/stream'){
    res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'})
    res.write(': connected\n\n');streams.add(res);res.on('close',()=>streams.delete(res));return
  }
  json({error:'Unknown fixture request: '+url.pathname},404)
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
    localStorage.setItem('go-4gproxy:mcc-mnc-table:v1',JSON.stringify({fetched_at:Date.now(),rows:[{mcc:'262',mnc:'02',iso:'de',network:'Vodafone',country:'Germany',country_code:'49'}]}));
  `});
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  const address=server.httpServer.address();const url=`http://127.0.0.1:${address.port}/`;
  await mkdir(artifactDir,{recursive:true});
  const snapshot=async name=>{await evaluate(`Promise.all([...document.querySelectorAll('md-dialog')].flatMap(el=>el.shadowRoot?.querySelector('dialog')?.getAnimations({subtree:true}) || []).map(a=>a.finished.catch(()=>{})))`);await evaluate('document.fonts.ready');await delay(120);await writeFile(path.join(artifactDir,name+'.png'),Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64'));};
  const key=async (key,code=key,vk=0)=>{await send('Input.dispatchKeyEvent',{type:'keyDown',key,code,windowsVirtualKeyCode:vk});await send('Input.dispatchKeyEvent',{type:'keyUp',key,code,windowsVirtualKeyCode:vk});};
  // A hidden caption must never create a second, outer document scrollbar.
  await send('Page.navigate',{url:url+'#/'})
  await wait('document.querySelectorAll(".traffic-table tbody tr").length === 24')
  assert.equal(await evaluate('document.scrollingElement.scrollHeight'),1000)
  await evaluate('window.scrollTo(0,100000)')
  assert.equal(await evaluate('scrollY'),0)
  assert.equal(await evaluate('document.querySelector(".traffic-table").clientHeight'),484)
  assert.ok(await evaluate('document.querySelector(".shell-main").scrollHeight > document.querySelector(".shell-main").clientHeight'))
  await evaluate('location.hash="#/proxy"')
  await wait('document.querySelectorAll("#proxy-upstream article").length === 2')
  assert.equal(calls.filter(c=>c.method!=='GET').length,0,'Navigation may only read configuration')
  await snapshot('proxy-dark-desktop')
  const dialog = id=>`document.querySelector('#${id}').closest('md-dialog')`
  const closed = id=>`!${dialog(id)}.shadowRoot.querySelector('dialog').open`
  const fill = async (selector,value)=>{
    await evaluate(`(()=>{const el=document.querySelector(${JSON.stringify(selector)});el.shadowRoot.querySelector('input').focus();el.shadowRoot.querySelector('input').select()})()`)
    await send('Input.insertText',{text:value})
  }
  const selectLast = async selector=>{
    await click(selector)
    await evaluate(`window.menuClosed=false;document.querySelector(${JSON.stringify(selector)}).addEventListener('closed',()=>{window.menuClosed=true},{once:true})`)
    await key('End','End',35);await key('Enter','Enter',13)
    await wait('window.menuClosed')
  }
  await click('md-icon-button[aria-label="编辑 Germany"]')
  await wait(`${dialog('upstream-form')}.open`)
  assert.equal(await evaluate('document.querySelector("#upstream-form md-outlined-text-field[name=password]").value'),'')
  await fill('#upstream-form md-outlined-text-field[name=addr]','updated.example:1080')
  failNext=true;holdNext=true
  await click('md-filled-button[form="upstream-form"]')
  await until(()=>!!releaseMutation,'Held mutation')
  await click('md-filled-button[form="upstream-form"]')
  await key('Escape','Escape',27)
  assert.equal(await evaluate(`${dialog('upstream-form')}.open`),true,'Pending save must not dismiss the editor')
  assert.equal(calls.filter(c=>c.method==='PUT'&&c.path==='/api/upstream-proxies/de').length,1)
  releaseMutation()
  await wait('document.querySelector("#upstream-form [role=alert]")?.textContent.includes("fixture rejected")')
  assert.equal(await evaluate('document.querySelector("#upstream-form md-outlined-text-field[name=addr]").value'),'updated.example:1080')
  await click('md-filled-button[form="upstream-form"]')
  await wait(closed('upstream-form'))
  assert.equal(calls.find(c=>c.method==='PUT').body.password,'','Keep the existing upstream password when blank')

  await click('#proxy-upstream article md-text-button')
  await wait(`${dialog('country-form')}.open`)
  assert.deepEqual(await evaluate('Array.from(document.querySelectorAll("#country-form md-select-option")).map(el=>el.value)'),['DE','US'],'Do not offer countries owned by another proxy')
  await fill('#country-form md-outlined-text-field','Britain')
  await wait('document.querySelectorAll("#country-form md-select-option").length === 0')
  await fill('#country-form md-outlined-text-field','United')
  await wait('document.querySelectorAll("#country-form md-select-option").length === 1')
  await selectLast('#country-form md-outlined-select')
  await click('md-filled-button[form="country-form"]')
  await until(()=>calls.some(c=>c.method==='PUT'&&c.path.endsWith('/US')),'Country rule save')
  assert.deepEqual(calls.find(c=>c.method==='PUT'&&c.path.endsWith('/US')).body,{upstream_proxy_id:'de',enabled:true})
  await wait('document.querySelectorAll(".country-rule").length === 2')
  await click('.country-rule:last-of-type md-text-button')
  await wait('Array.from(document.querySelectorAll("md-dialog")).some(el=>el.open && el.textContent.includes("删除国家规则"))')
  const confirmationButton='.proxy-page md-dialog:last-of-type [slot=actions] md-text-button'
  await click(confirmationButton)
  await wait('!document.querySelector(".proxy-page md-dialog:last-of-type").shadowRoot.querySelector("dialog").open')
  assert.equal(calls.filter(c=>c.method==='DELETE').length,0,'Cancel must not delete a route')
  await click('.country-rule:last-of-type md-text-button')
  await wait('document.querySelector(".proxy-page md-dialog:last-of-type").open')
  await click('.proxy-page md-dialog:last-of-type [slot=actions] md-filled-button')
  await wait('!document.querySelector(".proxy-page md-dialog:last-of-type").shadowRoot.querySelector("dialog").open')
  assert.equal(calls.filter(c=>c.method==='DELETE'&&c.path.endsWith('/US')).length,1)
  await wait('document.querySelectorAll(".country-rule").length === 1')
  await key('Escape','Escape',27)
  await wait(closed('country-form'))

  await click('#proxy-tab-outbound')
  await wait('document.querySelector("#proxy-outbound").checkVisibility()')
  await click('md-icon-button[aria-label="编辑 Local"]')
  await wait('document.querySelector("#instance-form md-outlined-text-field[name=password]")?.value === "secret"')
  await click('#instance-form md-icon-button')
  assert.equal(await evaluate('document.querySelector("#instance-form md-outlined-text-field[name=password]").type'),'text')
  await click('#instance-form md-icon-button')
  await selectLast('#instance-form md-outlined-select[name=mode]')
  await fill('#instance-form md-outlined-text-field[name=listen_port]','8080')
  await fill('#instance-form md-outlined-text-field[name=username]','new-user')
  await click('md-filled-button[form="instance-form"]')
  await wait(closed('instance-form'))
  const saved=calls.find(c=>c.method==='PUT'&&c.path==='/api/proxy-instances/config').body.instances[0]
  assert.equal(saved.mode,'http');assert.equal(saved.listen_port,8080);assert.equal(saved.device_id,'modem-a');assert.equal(saved.auth_enabled,true);assert.equal(saved.username,'new-user');assert.equal(saved.password,'secret')
  await click('#proxy-outbound article md-text-button')
  await until(()=>calls.some(c=>c.path.endsWith('/actions/start')),'Explicit start')
  await wait('document.querySelector("#proxy-outbound article").textContent.includes("运行中")')
  await click('#proxy-outbound article md-text-button')
  await until(()=>calls.some(c=>c.path.endsWith('/actions/stop')),'Explicit stop')

  await evaluate('location.hash="#/logs"')
  await wait('document.querySelectorAll(".log-entry").length === 2 && document.querySelector(".log-connection").textContent.includes("实时连接")')
  for(const response of streams)response.write('event: log\ndata: '+JSON.stringify({...logs[1],message:'live warning'})+'\n\n')
  await wait('document.querySelectorAll(".log-entry").length === 3')
  await click('.logs-page .page-heading md-outlined-button')
  await wait('document.querySelector(".log-connection").textContent.includes("已暂停")')
  await until(()=>streams.size===0,'Pause closes the event stream')
  await click('.log-filters md-outlined-select')
  await key('Home','Home',36);for(let n=0;n<3;n++)await key('ArrowDown','ArrowDown',40);await key('Enter','Enter',13)
  await wait('document.querySelectorAll(".log-entry").length === 2')
  await fill('.log-filters md-outlined-text-field','live warning')
  await wait('document.querySelectorAll(".log-entry").length === 1')
  await evaluate(`window.exportBlob=null;URL.createObjectURL=blob=>{window.exportBlob=blob;return 'blob:fixture'}`)
  await click('.logs-page .page-heading md-filled-button')
  const exported=await evaluate('window.exportBlob.text()')
  assert.ok(exported.includes('live warning'));assert.ok(!exported.includes('pending registration'));assert.ok(!exported.includes('ready'))
  const beforeResume=calls.filter(c=>c.path==='/api/logs/stream').length
  await click('.logs-page .page-heading md-outlined-button')
  await wait('document.querySelector(".log-connection").textContent.includes("实时连接")')
  assert.equal(calls.filter(c=>c.path==='/api/logs/stream').length,beforeResume+1)
  assert.equal(calls.filter(c=>c.path==='/api/logs/stream').at(-1).query.level,'warn')
  await click('.log-follow md-switch')
  assert.equal(await evaluate('document.querySelector(".log-follow md-switch").selected'),false)
  await snapshot('logs-dark-desktop')
  await click('md-icon-button[aria-label="主题：暗色模式"]');await click('md-icon-button[aria-label="主题：跟随设备"]')
  await wait('!document.documentElement.classList.contains("dark")')
  await snapshot('logs-light-desktop')
  await send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true})
  await wait('!document.querySelector(".navigation")')
  assert.equal(await evaluate('document.scrollingElement.scrollHeight'),844)
  assert.equal(await evaluate('document.querySelector(".shell-main").scrollWidth <= document.querySelector(".shell-main").clientWidth'),true)
  await snapshot('logs-light-mobile')
  await click('.logs-page .page-heading md-text-button')
  await wait('document.querySelectorAll(".log-entry").length === 0')
  await evaluate('location.hash="#/proxy"')
  await wait('document.querySelectorAll("#proxy-upstream article").length === 2')
  holdHistory=true
  await evaluate('location.hash="#/logs"')
  await until(()=>!!releaseHistory,'Delayed history request')
  await evaluate('location.hash="#/proxy"')
  await wait('document.querySelectorAll("#proxy-upstream article").length === 2')
  const streamCount=calls.filter(c=>c.path==='/api/logs/stream').length
  releaseHistory()
  await delay(200)
  assert.equal(calls.filter(c=>c.path==='/api/logs/stream').length,streamCount,'Leaving the page cancels delayed stream startup')
  await click('md-icon-button[aria-label="编辑 Germany"]')
  await wait(`${dialog('upstream-form')}.open`)
  await snapshot('proxy-light-mobile-editor')
  const modalBounds=await evaluate(`${dialog('upstream-form')}.shadowRoot.querySelector('dialog').getBoundingClientRect().toJSON()`)
  assert.ok(modalBounds.width <= 390 && modalBounds.x >= 24 && modalBounds.bottom <= 820,JSON.stringify(modalBounds))
  assert.equal(await evaluate('getComputedStyle(document.querySelector("#upstream-form")).paddingLeft'),'24px')
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
