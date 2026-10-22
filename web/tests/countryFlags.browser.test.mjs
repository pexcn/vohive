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

test('Operator flags use local Noto artwork, including Taiwan, without emoji fonts', {
  skip: available ? false : 'Requires Chromium and Node.js with WebSocket support',
  timeout:60000
}, async () => {
const root=fileURLToPath(new URL('../',import.meta.url))
process.chdir(root)
const profile=await mkdtemp(path.join(tmpdir(),'vohive-pages-'))
const artifactDir=process.env.VOHIVE_SCREENSHOTS || path.join(profile,'screenshots')
let browser,socket,debugSend,debugEvaluate
const calls = [], streams = new Set()
const base = {running:true, healthy:true, lifecycle_phase:'online', backend_mode:'qmi', interface:'wwan0', network_enabled:false}
const fixtures = [
  {...base,id:'tw',name:'Taiwan fixture',modem:{native_mcc:'466',native_mnc:'92',native_spn:'Chunghwa'}},
  {...base,id:'gb',name:'Britain fixture',modem:{native_mcc:'234',native_mnc:'33',native_spn:'Orange'}},
  {...base,id:'unknown',name:'Unknown fixture',modem:{native_mcc:'999',native_mnc:'99',native_spn:'Unknown'}}
]
async function mockApi(req,res) {
  const url = new URL(req.url,'http://localhost')
  calls.push({method:req.method,path:url.pathname})
  const json=data=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(data))}
  if(req.method!=='GET'){json({error:'Unexpected mutation'});return}
  if(url.pathname.endsWith('/stream')){
    res.writeHead(200,{'Content-Type':'text/event-stream','Cache-Control':'no-cache'})
    res.write(': connected\n\n');streams.add(res);res.on('close',()=>streams.delete(res));return
  }
  if(url.pathname==='/api/devices'){json({devices:fixtures,device_limit:5});return}
  if(url.pathname.endsWith('/overview')){json({devices:[fixtures.find(d=>url.pathname.includes('/'+d.id+'/'))]});return}
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
    localStorage.setItem('go-4gproxy:mcc-mnc-table:v1',JSON.stringify({fetched_at:Date.now(),rows:[{mcc:'466',mnc:'92',iso:'tw',network:'Chunghwa',country:'Taiwan',country_code:'886'},{mcc:'234',mnc:'33',iso:'gb',network:'Orange',country:'United Kingdom',country_code:'44'},{mcc:'999',mnc:'99',iso:'zz',network:'Unknown',country:'Unknown',country_code:''}]}));
  `});
  await send('Emulation.setDeviceMetricsOverride',{width:1440,height:1000,deviceScaleFactor:1,mobile:false});
  const address=server.httpServer.address();const url=`http://127.0.0.1:${address.port}/`;
  await mkdir(artifactDir,{recursive:true});
  const snapshot=async name=>{await evaluate(`Promise.all([...document.querySelectorAll('md-dialog')].flatMap(el=>el.shadowRoot?.querySelector('dialog')?.getAnimations({subtree:true}) || []).map(a=>a.finished.catch(()=>{})))`);await evaluate('document.fonts.ready');await delay(120);await writeFile(path.join(artifactDir,name+'.png'),Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64'));};

  const operator = `Array.from(document.querySelectorAll('.field-label')).find(el=>el.textContent==='原运营商')?.parentElement`
  const flag = `(${operator})?.querySelector('img.country-flag')`
  await send('Page.navigate',{url:url+'#/devices'})
  await wait('document.querySelectorAll(".device-option").length === 3')
  await wait(`${flag}?.complete && ${flag}?.naturalWidth > 0`)
  await evaluate(`Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>window.copiedOperator=text}})`)
  for(const theme of ['dark','light']) {
    await evaluate(`document.documentElement.classList.toggle('dark',${theme==='dark'})`)
    for(const width of [1920,390,320]) {
      await send('Emulation.setDeviceMetricsOverride',{width,height:1000,deviceScaleFactor:1,mobile:width<600})
      for(const [index,iso,text] of [[1,'TW','Chunghwa (46692)'],[2,'GB','Orange (23433)']]) {
        await click('.device-option:nth-child('+index+')')
        await wait(`${flag}?.alt === '${iso} 国旗' && ${flag}?.complete && ${flag}?.naturalWidth > 0`)
        const rendered=await evaluate(`(()=>{
          const row=${operator},img=${flag},r=img.getBoundingClientRect(),button=row.querySelector('button');
          return {text:button.textContent.trim(),label:button.getAttribute('aria-label'),height:r.height,width:r.width,source:img.src};
        })()`)
        assert.equal(rendered.text,text,'Display and copy text contain no emoji')
        assert.equal(rendered.label,'复制原运营商：'+text)
        assert.equal(rendered.height,20)
        assert.equal(rendered.width,20)
        assert.ok(rendered.source.startsWith(url)||rendered.source.startsWith('data:image/svg+xml'))
        const svg=await evaluate(`fetch(${flag}.src).then(r=>r.text())`)
        assert.equal(svg,await readFile(path.join(root,'src/assets/flags',iso.toLowerCase()+'.svg'),'utf8'))
        if(iso==='TW'){
          assert.ok(svg.includes('fill="#fe0000"'))
          assert.ok(svg.includes('fill="#000095"'))
          assert.ok(svg.includes('fill="#fff"'))
        }
        await evaluate(`(${operator}).querySelector('button').click()`)
        await wait(`window.copiedOperator === ${JSON.stringify(text)}`)
        assert.equal(await evaluate('document.documentElement.scrollWidth>innerWidth'),false)
        await snapshot('operator-flag-'+iso+'-'+theme+'-'+width)
      }
      await click('.device-option:nth-child(3)')
      await wait(`(${operator})?.querySelector('button')?.textContent.trim()==='Unknown (99999)'`)
      assert.equal(await evaluate(`!!${flag}`),false,'Unknown countries show only the operator text')
    }
  }
  await click('.device-option:nth-child(1)')
  await wait(`${flag}?.complete && ${flag}?.naturalWidth > 0`)
  await evaluate(`${flag}.dispatchEvent(new Event('error'))`)
  await wait(`!${flag}`)
  assert.equal(await evaluate(`(${operator}).querySelector('button').textContent.trim()`),'Chunghwa (46692)','Image failures preserve the operator')
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
