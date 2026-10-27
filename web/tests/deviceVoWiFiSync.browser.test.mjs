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

test('Device list and VoWiFi progress share live snapshots without stale rollbacks', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout:60000
}, async () => {
const root=fileURLToPath(new URL('../',import.meta.url))
process.chdir(root)
const profile=await mkdtemp(path.join(tmpdir(),'vohive-pages-'))
const artifactDir=process.env.VOHIVE_SCREENSHOTS || path.join(profile,'screenshots')
let browser,socket,debugSend,debugEvaluate
const calls = [], streams = new Set(), overviewStreams = new Map()
const readinessKeys = ['sim_ready','access_ready','tunnel_ready','ims_ready','sms_ready']
const runtime = (second,count) => ({
  ...Object.fromEntries(readinessKeys.map((key,i)=>[key,i<count])),
  dataplane_mode:'userspace', last_reason:count===5?'sms_ready':count===0?'stopped':'ims_registering',
  updated_at:`2026-10-27T00:00:${String(second).padStart(2,'0')}Z`
})
const device = (id,second,count) => ({
  id,name:'Fixture '+id,running:true,healthy:true,lifecycle_phase:'online',backend_mode:'qmi',
  interface:'wwan0',network_enabled:false,network_connected:false,vowifi_enabled:true,
  vowifi_runtime:runtime(second,count),modem:{operator:'Fixture',reg_status:1}
})
let listSnapshot = [device('a',1,3),device('b',1,0)]
const detailSnapshot = structuredClone(listSnapshot)
let holdList = false, releaseList, holdDetail = false, releaseDetail
async function mockApi(req,res) {
  const url = new URL(req.url,'http://localhost')
  calls.push({method:req.method,path:url.pathname})
  const json=data=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data))}
  if(req.method!=='GET'){json({error:'Unexpected mutation'});return}
  const streamMatch=url.pathname.match(/^\/api\/devices\/([^/]+)\/overview\/stream$/)
  if(streamMatch){
    const id=streamMatch[1]
    res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'})
    res.write(': connected\n\n');streams.add(res);overviewStreams.set(id,res)
    res.on('close',()=>{streams.delete(res);if(overviewStreams.get(id)===res)overviewStreams.delete(id)});return
  }
  if(url.pathname==='/api/devices'){
    const body=structuredClone({devices:listSnapshot,device_limit:5})
    if(holdList){holdList=false;await new Promise(resolve=>{releaseList=resolve})}
    json(body);return
  }
  if(url.pathname.endsWith('/overview')){
    const body=structuredClone({devices:[detailSnapshot.find(d=>url.pathname.includes('/'+d.id+'/'))]})
    if(holdDetail&&url.pathname.includes('/b/')){holdDetail=false;await new Promise(resolve=>{releaseDetail=resolve})}
    json(body);return
  }
  json({})
}
function pushOverview(id,data) {
  const response=overviewStreams.get(id)
  assert.ok(response,'Selected device stream is connected')
  response.write('event: overview\ndata: '+JSON.stringify({devices:[data]})+'\n\n')
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
  debugSend=send;debugEvaluate=evaluate;
  await send('Runtime.enable');await send('Page.enable');await send('Network.enable');
  await send('Emulation.setEmulatedMedia',{features:[{name:'prefers-color-scheme',value:'light'}]});
  await send('Network.setBlockedURLs',{urls:['https://*']});
  await send('Page.addScriptToEvaluateOnNewDocument',{source:`
    localStorage.setItem('token','fixture-token');localStorage.setItem('theme','dark');
    const nativeSetTimeout=window.setTimeout.bind(window);
    window.setTimeout=(callback,ms,...args)=>{
      const id=nativeSetTimeout(callback,ms,...args);
      if(ms===15000)window.listPoll={id,run:()=>{clearTimeout(id);callback(...args)}};
      return id;
    };

    localStorage.setItem('go-4gproxy:mcc-mnc-table:v1',JSON.stringify({fetched_at:Date.now(),rows:[{mcc:'001',mnc:'01',iso:'',network:'Fixture',country:'Fixture',country_code:''}]}));
  `});
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  const address=server.httpServer.address();const url=`http://127.0.0.1:${address.port}/`;
  await mkdir(artifactDir,{recursive:true});
  const snapshot=async name=>{await evaluate(`Promise.all([...document.querySelectorAll('md-dialog')].flatMap(el=>el.shadowRoot?.querySelector('dialog')?.getAnimations({subtree:true}) || []).map(a=>a.finished.catch(()=>{})))`);await evaluate('document.fonts.ready');await delay(120);await writeFile(path.join(artifactDir,name+'.png'),Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64'));};


  await send('Page.navigate',{url:url+'#/devices'})
  await wait('document.querySelectorAll(".readiness-steps li").length===5 && !!window.listPoll')
  await until(()=>overviewStreams.has('a'),'Initial overview stream')
  const expectRuntime = async count => {
    const status=count===5?'ok':count>0?'partial':'off'
    const tone=count===5?'success':count>0?'warning':'danger'
    await until(()=>evaluate(`(()=>{
      const label=document.querySelector('.device-option[aria-pressed="true"] .device-vowifi');
      const steps=[...document.querySelectorAll('.readiness-steps li')];
      return label?.dataset.status==='${status}' && document.querySelector('.runtime-status')?.dataset.tone==='${tone}' && steps.length===5 && steps.every((el,i)=>el.dataset.state===(i<${count}?'ready':'pending'));
    })()`),'List and detail render the same readiness',2000)
    const colors=await evaluate(`(()=>{
      const label=document.querySelector('.device-option[aria-pressed="true"] .device-vowifi'),probe=document.createElement('span');
      probe.style.color='var(--vh-success)';document.body.append(probe);const success=getComputedStyle(probe).color;probe.remove();
      return {text:label.textContent.trim(),color:getComputedStyle(label).color,normal:getComputedStyle(label.parentElement).color,success};
    })()`)
    assert.equal(colors.text,'Wi-Fi Calling')
    assert.equal(colors.color,count===5?colors.success:colors.normal)
  }
  const startListPoll = async () => {
    const previous=await evaluate('window.listPoll.id')
    await evaluate('window.listPoll.run();void 0')
    return previous
  }
  const finishListPoll = async previous => wait(`window.listPoll.id!==${previous}`)

  await expectRuntime(3)
  const beforePush=calls.length
  pushOverview('a',device('a',2,5))
  await expectRuntime(5)
  assert.equal(calls.length,beforePush,'SSE updates both views without another API request')
  await snapshot('wifi-ready')

  // Start a list request, then deliver a newer reconnecting state before it returns.
  holdList=true
  const pendingPoll=await startListPoll()
  await until(()=>!!releaseList,'Held list request')
  pushOverview('a',device('a',3,3))
  await expectRuntime(3)
  releaseList();releaseList=null
  await finishListPoll(pendingPoll)
  await expectRuntime(3)

  // A newer list snapshot also advances the right panel without waiting for SSE.
  listSnapshot=[device('a',4,5),device('b',1,0)]
  const freshPoll=await startListPoll()
  await finishListPoll(freshPoll)
  await expectRuntime(5)
  pushOverview('a',device('a',2,3))
  await delay(100)
  await expectRuntime(5)

  // Opening another device: its late initial HTTP response must not undo a live update.
  holdDetail=true
  await send('Page.navigate',{url:url+'?fixture=second#/devices?device=b'})
  await until(()=>!!releaseDetail&&overviewStreams.has('b'),'Second device HTTP request and SSE')
  pushOverview('b',device('b',5,5))
  await expectRuntime(5)
  releaseDetail();releaseDetail=null
  await wait(`performance.getEntriesByType('resource').some(entry=>entry.name.endsWith('/b/overview'))`)
  await delay(100)
  await expectRuntime(5)
  assert.equal(await evaluate('document.querySelector(".device-header h2").textContent'),'Fixture b')
  pushOverview('b',device('b',6,0))
  await expectRuntime(0)
  await snapshot('wifi-disconnected')
  assert.equal(calls.filter(c=>c.method!=='GET').length,0)
  assert.equal(exceptions.length,0,JSON.stringify(exceptions))

} catch(error) {
  if(debugEvaluate) console.error((await debugEvaluate('document.body.innerText')).slice(0,3000))
  if(debugSend) await writeFile(path.join(artifactDir,'failure.png'),Buffer.from((await debugSend('Page.captureScreenshot',{format:'png'})).data,'base64'))
  throw error
} finally {
  releaseList?.();releaseDetail?.();socket?.close();browser?.kill('SIGTERM')
  for(const response of streams)response.destroy()
  await server.close()
  await rm(profile,{recursive:true,force:true,maxRetries:5,retryDelay:100})
}
})
