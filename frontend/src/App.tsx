import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { ArrowDownToLine, ArrowUpRight, Check, ChevronDown, CircleAlert, Clock3, CopyPlus, Cpu, Eye, FolderOpen, Gamepad2, Gauge, Globe2, House, Languages, LoaderCircle, Package, Play, RefreshCw, RotateCcw, Smartphone, X } from 'lucide-react'
import { api, defaults, errorMessage, type Environment, type Settings, type Status, type LuaStatus } from './api'
import { FeatureCard, OptionRow, SelectionTrack, Switch } from './components'
import { VERSION, fpsPresets, keyName, languageNames, serverLabels, serverMode, servers, type ServerMode, type Tab } from './constants'

export function App() {
 const [settings, setSettings] = useState<Settings>(defaults)
 const [status, setStatus] = useState<Status>({ running: 0, ports: [], message: '' })
 const [loaded, setLoaded] = useState(false)
 const [busy, setBusy] = useState(false)
 const [notice, setNotice] = useState<{ text: string; error?: boolean } | null>(null)
 const [recording, setRecording] = useState(false)
 const [languageInfo, setLanguageInfo] = useState<{text: string; voice: string; version: string} | null>(null)
 const [languageError, setLanguageError] = useState('')
 const [hostsOpen, setHostsOpen] = useState(false)
 const [tab, setTab] = useState<Tab>('home')
 const [luaSessions, setLuaSessions] = useState<LuaStatus[]>([])
 const [luaPID, setLuaPID] = useState(0)
 const [luaBusy, setLuaBusy] = useState(false)
 const [luaResult, setLuaResult] = useState<LuaStatus | null>(null)
 const [env, setEnv] = useState<Environment | null>(null)
 const [envOpen, setEnvOpen] = useState(false)
 // 收起面板时保留最后一个分类的内容，播完退出动画再卸载
 const [view, setView] = useState<Exclude<Tab, 'home'>>('game')
 const [closing, setClosing] = useState(false)
 const closeTimer = useRef(0)
 const selectTab = (next: Tab) => {
  if (next === tab) next = 'home'
  if (next === tab) return
  clearTimeout(closeTimer.current)
  if (next === 'home') {
   setClosing(true)
   closeTimer.current = window.setTimeout(() => setClosing(false), 220)
  } else {
   setClosing(false)
   setView(next)
  }
  setTab(next)
 }
 const [dirty, setDirty] = useState(false)
 const saveQueue = useRef<Promise<void>>(Promise.resolve())
 const revision = useRef(0)
 const change = <K extends keyof Settings>(key: K, value: Settings[K]) => { revision.current++; setSettings(s => ({ ...s, [key]: value, ...(key === 'multiInstance' && !value ? { instances: 1 } : {}) })); setDirty(true) }
 useEffect(() => {
  let live = true
  Promise.all([api.load(), api.status()]).then(([cfg, state]) => { if (live) { setSettings({ ...defaults, ...cfg, luaScripts: cfg.luaScripts || [], luaDisabledScripts: cfg.luaDisabledScripts || [], instances: 1 }); setStatus(state); setLoaded(true) } }).catch(e => { if (live) { setLoaded(true); setNotice({ text: `读取设置失败，已使用默认设置：${e.message}`, error: true }) } })
  const off = api.subscribe(s => { if (live) setStatus(s) })
  return () => { live = false; off() }
 }, [])
 const refreshEnv = () => api.environment().then(setEnv).catch(e => setNotice({ text: errorMessage(e), error: true }))
 useEffect(() => { if (tab === 'env' || !env) void refreshEnv() }, [tab])
 useEffect(() => {
  if (tab !== 'lua') return
  let live = true
  const refresh = () => api.luaSessions().then(sessions => { if (!live) return; setLuaSessions(sessions); setLuaPID(pid => sessions.some(s => s.pid === pid) ? pid : sessions[0]?.pid || 0) }).catch(() => {})
  void refresh(); const timer = setInterval(refresh, 1000)
  return () => { live = false; clearInterval(timer) }
 }, [tab])
 async function addLua() {
  try { const path = await api.pickLua(); if (path && !settings.luaScripts.includes(path)) change('luaScripts', [...settings.luaScripts, path]) }
  catch (e) { setNotice({ text: errorMessage(e), error: true }) }
 }
 async function runLua() {
  setLuaBusy(true)
  try { setLuaResult(await api.runLua(luaPID, settings.luaScripts.filter(path => !settings.luaDisabledScripts.includes(path)))) }
  catch (e) { setNotice({ text: errorMessage(e), error: true }) }
  finally { setLuaBusy(false) }
 }
 useEffect(() => {
  if (!recording) return
  const handler = (event: KeyboardEvent) => { event.preventDefault(); event.stopPropagation(); if (event.key === 'Escape') { setRecording(false); return }; const code = event.keyCode; if (code > 0 && code < 255) { change('timeStopKey', code); setRecording(false) } }
  window.addEventListener('keydown', handler, true)
  return () => window.removeEventListener('keydown', handler, true)
 }, [recording])
 useEffect(() => {
  if (tab === 'home' || recording) return
  const handler = (event: KeyboardEvent) => { if (event.key === 'Escape') selectTab('home') }
  window.addEventListener('keydown', handler)
  return () => window.removeEventListener('keydown', handler)
 }, [tab, recording])
 useEffect(() => { if (!notice || notice.error) return; const timer = window.setTimeout(() => setNotice(null), 3500); return () => clearTimeout(timer) }, [notice])
 useEffect(() => {
  setLanguageInfo(null); setLanguageError('')
  if (!settings.gamePath) return
  let live = true
  api.languageInfo(settings.gamePath).then(info => { if (live) setLanguageInfo(info) }).catch(e => { if (live) setLanguageError(errorMessage(e)) })
  return () => { live = false }
 }, [settings.gamePath])
 useEffect(() => {
  if (!loaded || !dirty) return
  const currentRevision = revision.current
  const timer = window.setTimeout(() => {
   saveQueue.current = saveQueue.current.catch(() => {}).then(() => api.save(settings))
   void saveQueue.current.then(() => { if (revision.current === currentRevision) setDirty(false) }).catch(e => setNotice({ text: errorMessage(e), error: true }))
  }, 300)
  return () => clearTimeout(timer)
 }, [settings, loaded, dirty])
 async function restoreLanguage() {
  setBusy(true)
  try { await api.restoreLanguage(settings.gamePath); change('textLanguage', ''); change('voiceLanguage', ''); setLanguageInfo(await api.languageInfo(settings.gamePath)); setNotice({text:'已恢复 Poem 修改前的游戏语言'}) }
  catch(e) { setNotice({text:errorMessage(e),error:true}) }
  finally { setBusy(false) }
 }
 async function choosePath() {
  try { const path = await api.pick(settings.gamePath ? settings.gamePath.replace(/\/[^/]+$/, '') : settings.winePrefix ? `${settings.winePrefix}/drive_c` : ''); if (path) change('gamePath', path) } catch (e) { setNotice({ text: errorMessage(e), error: true }) }
 }
 async function chooseWine() {
  try { const path = await api.pickWine(); if (path) change('winePath', path) } catch (e) { setNotice({ text: errorMessage(e), error: true }) }
 }
 async function choosePrefix() {
  try { const path = await api.pickPrefix(settings.winePrefix); if (path) change('winePrefix', path) } catch (e) { setNotice({ text: errorMessage(e), error: true }) }
 }
 async function start() {
  setBusy(true); setNotice(null)
  try { await saveQueue.current.catch(() => {}); const state = await api.start(settings); setStatus(state); setDirty(false); setNotice({ text: state.message }) } catch (e) { setNotice({ text: errorMessage(e), error: true }); try { setStatus(await api.status()) } catch { /* Keep the launch error visible if the bridge disconnects. */ } } finally { setBusy(false) }
 }

 const locked = !loaded || busy
 const retail = settings.loginChannel === 'official' || settings.loginChannel === 'bilibili'
 const wineReady = !!settings.winePath && !!settings.winePrefix
 const canLaunch = loaded && !busy && wineReady && !!settings.gamePath && (status.running === 0 || settings.multiInstance)
 const pathLabel = settings.gamePath ? settings.gamePath.replace(/\\/g, '/').split('/').slice(-2).join(' / ') : '尚未选择 StarRail.exe'
 const mode = serverMode(settings.serverURL)
 const launchLabel = busy ? '正在启动…' : !wineReady ? '请先设置 Wine' : status.running && settings.multiInstance ? '再开一个' : status.running ? '游戏运行中' : '启动游戏'
 const luaEnabledCount = settings.luaScripts.filter(path => !settings.luaDisabledScripts.includes(path)).length
 const tabs: { id: Tab; label: string; icon: ReactNode; state?: boolean }[] = [
  { id: 'home', label: '首页', icon: <House size={16} /> },
  { id: 'env', label: '运行环境', icon: <Cpu size={16} />, state: wineReady },
  { id: 'game', label: '游戏功能', icon: <Gamepad2 size={16} /> },
  ...(retail ? [] : [{ id: 'proxy' as const, label: '网络代理', icon: <Globe2 size={16} />, state: settings.proxyEnabled }]),
  { id: 'language', label: '语言', icon: <Languages size={16} /> },
  { id: 'lua', label: 'Lua', icon: <Play size={16} /> },
 ]
 const panelOpen = tab !== 'home'
 const panelShown = panelOpen || closing
 // 面板高度跟随内容，切换分类时以过渡动画改变高度，而不是跳变
 const panelContent = useRef<HTMLDivElement>(null)
 const [panelHeight, setPanelHeight] = useState(0)
 useLayoutEffect(() => {
  const content = panelContent.current
  if (!content) return
  const measure = () => setPanelHeight(content.offsetHeight + 50)
  measure()
  const observer = new ResizeObserver(measure)
  observer.observe(content)
  return () => observer.disconnect()
 }, [view, panelShown])

 return <div className={`shell ${panelOpen ? 'panel-open' : ''}`}>
  <div className="art" aria-hidden="true" />
  <div className="scrim" aria-hidden="true" />
  <header className="titlebar">
   <div className="brand" title={`Poem ${VERSION}`}><strong>Poem <span>Launcher</span></strong></div>
   <nav className="nav glass"><SelectionTrack className="tabs" role="tablist" active={tab}>{tabs.map(t => <button key={t.id} role="tab" id={`tab-${t.id}`} aria-selected={tab === t.id} aria-controls={t.id === 'home' ? undefined : 'panel-body'} className="tab" onClick={() => selectTab(t.id)}>{t.icon}{t.label}{t.state !== undefined && <span className={`tab-state ${t.state ? 'on' : ''}`} aria-label={t.state ? '已开启' : '已关闭'} />}</button>)}</SelectionTrack></nav>
   <div className="window-controls glass">
    {status.running > 0 && <span className="run-chip" title={status.ports.length ? `代理端口 ${status.ports.join(' / ')} · 游戏运行时关闭窗口只会最小化` : '游戏运行时关闭窗口只会最小化'}><i />运行中 {status.running}</span>}
   </div>
  </header>

  <main className="stage">
   {panelShown && <section className={`panel glass ${panelOpen ? '' : 'leaving'}`} style={panelHeight ? { height: panelHeight } : undefined}>
    <button className="panel-close icon-button" aria-label="收起设置" title="收起 · Esc" onClick={() => selectTab('home')}><X size={16} /></button>
    <div key={view} className="panel-body" id="panel-body" role="tabpanel" aria-labelledby={`tab-${view}`}><div ref={panelContent}>
     {view === 'env' && <>
      <div className="panel-intro"><h2>运行环境</h2><p>游戏通过 Wine 运行，Poem 在同一个容器中启动游戏并加载模块</p></div>
      <div className="cols">
       <div className="col">
        <div>
         <span className="group-label">Wine 程序</span>
         {env && env.runtimes.length > 0 && <div className="chips env-chips">{env.runtimes.map(r => <button key={r.path} type="button" className={`chip ${settings.winePath === r.path ? 'active' : ''}`} title={r.path} disabled={locked || status.running > 0} onClick={() => change('winePath', r.path)}>{r.name}</button>)}</div>}
         <div className="input-row"><input className="input" aria-label="Wine 程序路径" placeholder="/Applications/CrossOver.app/Contents/SharedSupport/CrossOver/bin/wine" value={settings.winePath} onChange={e => change('winePath', e.target.value)} spellCheck={false} disabled={locked || status.running > 0} /><button className="icon-button" aria-label="浏览 Wine 程序" title="浏览" disabled={locked || status.running > 0} onClick={() => void chooseWine()}><FolderOpen size={16} /></button></div>
         <small className="hint">支持 CrossOver、Whisky、Game Porting Toolkit、Homebrew 等提供的 wine / wine64</small>
        </div>
        <div>
         <span className="group-label">Wine 容器（prefix）</span>
         {env && env.prefixes.length > 0 && <div className="chips env-chips">{env.prefixes.map(p => <button key={p.path} type="button" className={`chip ${settings.winePrefix === p.path ? 'active' : ''}`} title={p.path} disabled={locked || status.running > 0} onClick={() => change('winePrefix', p.path)}>{p.name}</button>)}</div>}
         <div className="input-row"><input className="input" aria-label="Wine 容器路径" placeholder="~/Library/Application Support/CrossOver/Bottles/StarRail" value={settings.winePrefix} onChange={e => change('winePrefix', e.target.value)} spellCheck={false} disabled={locked || status.running > 0} /><button className="icon-button" aria-label="浏览 Wine 容器" title="浏览" disabled={locked || status.running > 0} onClick={() => void choosePrefix()}><FolderOpen size={16} /></button></div>
         <small className="hint">包含 drive_c 的目录；游戏应已能在此容器中正常运行</small>
        </div>
        <div className={`option ${envOpen ? 'on' : ''}`}>
         <button type="button" className={`disclosure ${envOpen ? 'open' : ''}`} aria-expanded={envOpen} onClick={() => setEnvOpen(!envOpen)}>额外环境变量<ChevronDown size={14} /></button>
         {envOpen && <textarea aria-label="额外环境变量" placeholder={'每行一个 KEY=VALUE，例如\nWINEMSYNC=1\nD3DM_SUPPORT_DXR=1'} value={settings.wineEnv} onChange={e => change('wineEnv', e.target.value)} spellCheck={false} disabled={locked} />}
        </div>
       </div>
       <div className="col">
        <div className="lua-toolbar"><strong>功能模块</strong><span className="toolbar-actions"><button className="secondary-button" disabled={!env} onClick={() => env && void api.openFolder(env.modulesDir)}><Package size={15} />打开模块文件夹</button><button className="icon-button" aria-label="重新检测" title="重新检测" onClick={() => void refreshEnv()}><RefreshCw size={15} /></button></span></div>
        <div className="lua-list">{env?.modules.map(m => <div className="lua-file module-row" key={m.name}>
         <span className={`module-state ${m.present ? 'on' : ''}`}>{m.present ? <Check size={14} /> : <X size={14} />}</span><div><strong>{m.name}</strong><small>{m.feature}</small></div>
        </div>)}</div>
        <p className="hint">预编译模块不随 Poem 分发。把 DLL 放进模块文件夹后，对应功能即可使用；缺少某个模块只影响它对应的开关。Lua 桥接和 Poem 助手已内置。</p>
        {env && <p className="hint" title={env.dataDir}>设置、证书与日志位于 {env.dataDir}</p>}
       </div>
      </div>
     </>}

     {view === 'game' && <>
      <div className="panel-intro"><h2>游戏功能</h2><p>下次启动游戏时生效</p></div>
      <div className="feature-grid">
       <FeatureCard wide icon={<Clock3 size={18} />} title="战斗时停" desc="按住快捷键暂停战斗，松开恢复" checked={settings.timeStop} onChange={v => change('timeStop', v)} disabled={locked}>
        <label htmlFor="time-stop-key">快捷键</label>
        <button id="time-stop-key" className={`key-button ${recording ? 'recording' : ''}`} disabled={locked || !settings.timeStop} onClick={() => setRecording(!recording)}>{recording ? '按下按键 · Esc 取消' : keyName(settings.timeStopKey)}</button>
       </FeatureCard>
       <FeatureCard wide icon={<Gauge size={18} />} title="帧率上限" desc="解除默认限制，自定义目标帧率" checked={settings.unlockFPS} onChange={v => change('unlockFPS', v)} disabled={locked}>
        <div className="fps-input"><input aria-label="帧率上限" type="number" min={30} max={1000} value={settings.targetFPS} onChange={e => change('targetFPS', +e.target.value)} disabled={locked || !settings.unlockFPS} /><span>FPS</span></div>
        <div className="chips">{fpsPresets.map(fps => <button key={fps} className={`chip ${settings.targetFPS === fps ? 'active' : ''}`} disabled={locked || !settings.unlockFPS} onClick={() => change('targetFPS', fps)}>{fps}</button>)}</div>
       </FeatureCard>
       <FeatureCard icon={<CopyPlus size={18} />} title="解除多开" desc={settings.multiInstance ? '可继续点击“再开一个”' : '只允许运行一个游戏'} checked={settings.multiInstance} onChange={v => change('multiInstance', v)} disabled={locked} />
       <FeatureCard icon={<Smartphone size={18} />} title="移动端 UI" desc="使用游戏的触屏操作界面" checked={settings.touchUI} onChange={v => change('touchUI', v)} disabled={locked} />
       <FeatureCard icon={<Eye size={18} />} title="角色去虚化" desc="近距离镜头保持角色不透明" checked={settings.removeBlur} onChange={v => change('removeBlur', v)} disabled={locked} />
      </div>
     </>}

     {!retail && view === 'proxy' && <>
      <div className="panel-intro"><h2>网络代理</h2><p>游戏流量将转发至指定服务器</p></div>
      <div className={`master ${settings.proxyEnabled ? 'on' : ''}`}>
       <span className="badge"><Globe2 size={18} /></span>
       <div><strong>启用代理</strong><small>{settings.proxyEnabled ? '启动游戏时加载代理模块，首次使用会安装当前用户证书' : '关闭后不加载代理模块'}</small></div>
       <Switch checked={settings.proxyEnabled} onChange={v => change('proxyEnabled', v)} label="启用代理" disabled={locked} />
      </div>
      <fieldset className="cols" disabled={!settings.proxyEnabled || locked}>
       <div className="col">
       <div>
        <span className="group-label">服务器地址</span>
        <SelectionTrack className="segmented" role="radiogroup" aria-label="服务器地址模式" active={mode}>{(Object.keys(serverLabels) as ServerMode[]).map(m => <button key={m} type="button" role="radio" aria-checked={mode === m} className={mode === m ? 'active' : ''} onClick={() => change('serverURL', m === 'custom' ? '' : servers[m])}>{serverLabels[m]}</button>)}</SelectionTrack>
        <input className="input" aria-label="服务器地址" placeholder="http://127.0.0.1:21000" value={settings.serverURL} onChange={e => change('serverURL', e.target.value)} spellCheck={false} />
        <small className="hint">游戏流量将转发至此地址</small>
       </div>
       <div>
        <label className="group-label" htmlFor="proxy-port">代理端口</label>
        <input id="proxy-port" className="input port" type="number" min={0} max={65535} value={settings.proxyPort} onChange={e => change('proxyPort', +e.target.value)} />
        <small className="hint">填 0 自动分配；端口被占用时自动选择空闲端口</small>
       </div>
       </div>
       <div className="col">
       <div className={`option ${settings.rsaPatch ? 'on' : ''}`}>
        <OptionRow title="RSA 密钥替换" desc="同时用于代理响应和 SDK 处理" control={<Switch checked={settings.rsaPatch} onChange={v => change('rsaPatch', v)} label="RSA 密钥替换" disabled={!settings.proxyEnabled || locked} />} />
        {settings.rsaPatch && <textarea aria-label="RSA 公钥" placeholder={'粘贴 2048 位 RSA 公钥（AQAB）\nBase64 / PEM / XML；留空使用默认密钥'} value={settings.rsaKey} onChange={e => change('rsaKey', e.target.value)} spellCheck={false} />}
       </div>
       <div className={`option ${settings.webRedirect ? 'on' : ''}`}>
        <OptionRow title="网页 URL 重定向" desc="将游戏内网页请求重定向到服务器" control={<Switch checked={settings.webRedirect} onChange={v => change('webRedirect', v)} label="网页 URL 重定向" disabled={!settings.proxyEnabled || locked} />} />
        <button type="button" className={`disclosure ${hostsOpen ? 'open' : ''}`} aria-expanded={hostsOpen} onClick={() => setHostsOpen(!hostsOpen)}>重定向域名<ChevronDown size={14} /></button>
        {hostsOpen && <textarea aria-label="重定向域名" placeholder="每行一个域名；留空使用默认列表" value={settings.webHosts} onChange={e => change('webHosts', e.target.value)} spellCheck={false} />}
       </div>
       </div>
      </fieldset>
     </>}

     {view === 'lua' && <>
      <div className="panel-intro"><h2>本地 Lua 脚本</h2><p>游戏启动并进入场景后自动执行，脚本不经过服务器</p></div>
      <div className="cols">
      <div className="col">
      <div className={`master ${settings.luaEnabled ? 'on' : ''}`}>
       <span className="badge"><Play size={18} /></span>
       <div><strong>启动游戏后自动执行</strong><small>下次启动生效；进入可游玩场景后按列表顺序执行一次</small></div>
       <Switch checked={settings.luaEnabled} onChange={v => change('luaEnabled', v)} label="启动游戏后自动执行 Lua" disabled={locked || luaBusy} />
      </div>
      <div className="lua-toolbar"><strong>已启用 {luaEnabledCount} / {settings.luaScripts.length} 个脚本</strong><button className="secondary-button" disabled={locked || luaBusy || settings.luaScripts.length >= 16} onClick={() => void addLua()}><FolderOpen size={15} />添加脚本</button></div>
      <div className="lua-list">{settings.luaScripts.length ? settings.luaScripts.map((path, i) => <div className="lua-file" key={path}>
       <input className="lua-checkbox" type="checkbox" aria-label={`启用 ${path.replace(/\\/g, '/').split('/').pop()}`} checked={!settings.luaDisabledScripts.includes(path)} disabled={locked || luaBusy} onChange={e => change('luaDisabledScripts', e.target.checked ? settings.luaDisabledScripts.filter(p => p !== path) : [...settings.luaDisabledScripts, path])} />
       <span className="lua-number">{i + 1}</span><div><strong>{path.replace(/\\/g, '/').split('/').pop()}</strong><small title={path}>{path}</small></div>
       <button className="icon-button" aria-label={`上移脚本 ${i + 1}`} disabled={i === 0 || luaBusy} onClick={() => { const paths = [...settings.luaScripts]; [paths[i - 1], paths[i]] = [paths[i], paths[i - 1]]; change('luaScripts', paths) }}>↑</button>
       <button className="icon-button" aria-label={`移除脚本 ${i + 1}`} disabled={luaBusy} onClick={() => { change('luaScripts', settings.luaScripts.filter(p => p !== path)); change('luaDisabledScripts', settings.luaDisabledScripts.filter(p => p !== path)) }}><X size={15} /></button>
      </div>) : <p className="hint">添加 UTF-8 编码的 .lua 文件；按列表顺序执行。</p>}</div>
      </div>
      <div className="col">
      <div>
      <label className="group-label" htmlFor="lua-game">目标游戏窗口</label>
      <select id="lua-game" className="input" value={luaPID} onChange={e => { setLuaPID(+e.target.value); setLuaResult(null) }} disabled={!luaSessions.length || luaBusy}>
       {!luaSessions.length && <option value={0}>请先通过 Poem 启动游戏</option>}
       {luaSessions.map(session => <option key={session.pid} value={session.pid}>StarRail.exe · PID {session.pid}</option>)}
      </select>
      </div>
      <div className="lua-execute"><button className="secondary-button" disabled={locked || luaBusy || !luaPID || !settings.luaScripts.some(path => !settings.luaDisabledScripts.includes(path)) || ['queued', 'running', 'scheduled'].includes(luaSessions.find(s => s.pid === luaPID)?.state || '')} onClick={() => void runLua()}>{luaBusy ? <LoaderCircle size={15} className="spin" /> : <Play size={15} />}立即重新执行</button></div>
      {(() => { const session = luaSessions.find(s => s.pid === luaPID); const result = luaResult?.pid === luaPID && (!session?.id || session.id === '0') ? luaResult : session; const names: Record<string, string> = { idle: '未加载', waiting: '等待游戏 xLua 环境', ready: '模块已就绪', queued: '等待游戏主线程执行', scheduled: '等待进入游戏场景', running: '执行中', success: '执行完成', error: '执行失败' }; return result && <div className={`lua-result ${result.state === 'error' ? 'failed' : ''}`} role="status"><strong>{names[result.state] || result.state}</strong><span>{result.message}</span></div> })()}
      <p className="hint">只执行勾选的脚本，选择自动保存。取消勾选不撤销已经生效的修改或协程；重启游戏后按新的选择加载。</p>
      </div>
      </div>
     </>}

     {view === 'language' && <>
      <div className="panel-intro"><h2>游戏语言</h2><p>下次启动时应用，并自动备份原语言配置</p></div>
      {!settings.gamePath ? <div className="empty-state"><Languages size={26} /><strong>请先选择游戏路径</strong><span>Poem 需要读取游戏目录中的语言配置</span><button className="secondary-button" onClick={() => void choosePath()} disabled={locked}><FolderOpen size={15} />选择 StarRail.exe</button></div> : <>
       {languageError && <p className="warning"><CircleAlert size={15} />当前版本语言配置读取失败：{languageError}</p>}
       {languageInfo && <div className="lang-current"><span>当前文字 <b>{languageNames[languageInfo.text] || languageInfo.text}</b></span><span>当前语音 <b>{languageNames[languageInfo.voice] || languageInfo.voice}</b></span>{languageInfo.version && <span>数据版本 <b>{languageInfo.version}</b></span>}</div>}
       <div className="cols">{(['textLanguage', 'voiceLanguage'] as const).map(key => <div className="lang-group" key={key}>
        <span className="group-label">{key === 'textLanguage' ? '文字语言' : '语音语言'}</span>
        <div className="pills" role="radiogroup" aria-label={key === 'textLanguage' ? '文字语言' : '语音语言'}>{[['', '保持当前'], ...Object.entries(languageNames)].map(([code, name]) => <button key={code} type="button" role="radio" aria-checked={settings[key] === code} className={`pill ${settings[key] === code ? 'active' : ''}`} disabled={locked || !!languageError} onClick={() => change(key, code)}>{name}</button>)}</div>
       </div>)}</div>
       <div className="lang-footer"><small className="hint">语音需要游戏已安装对应语言包</small><button className="secondary-button" disabled={locked || status.running > 0 || !settings.gamePath} onClick={() => void restoreLanguage()}><RotateCcw size={14} />恢复原语言</button></div>
      </>}
     </>}
    </div></div>
   </section>}
  </main>

  <footer className="dock glass">
   <div className={`path ${settings.gamePath ? '' : 'empty'}`}>
    <button className="path-pick" onClick={() => void choosePath()} disabled={locked} title={settings.gamePath ? '更换 StarRail.exe' : '选择 StarRail.exe'}><FolderOpen size={17} /></button>
    <div className="path-copy"><small>游戏路径</small><span title={settings.gamePath}>{pathLabel}</span></div>
    <button className="icon-button" title="打开游戏目录" aria-label="打开游戏目录" disabled={!settings.gamePath || locked} onClick={() => void api.openFolder(settings.gamePath.replace(/[\\/][^\\/]+$/, ''))}><ArrowUpRight size={16} /></button>
   </div>
   <SelectionTrack className="segmented" role="radiogroup" aria-label="登录渠道" active={settings.loginChannel || 'private'}>{([{id:'private',name:'私服'},{id:'official',name:'官服'},{id:'bilibili',name:'B服'}] as const).map(c => <button key={c.id} role="radio" aria-checked={(settings.loginChannel || 'private') === c.id} disabled={locked || status.running > 0} onClick={() => { change('loginChannel', c.id); if (c.id !== 'private' && tab === 'proxy') selectTab('game') }}>{c.name}</button>)}</SelectionTrack>
   {settings.loginChannel === 'bilibili' && <button className="dock-icon" title="下载 / 更新 B服组件（约 85 MB，启动时自动安装）" aria-label="下载或更新 B服组件" disabled={locked || status.running > 0} onClick={async () => { setBusy(true); setNotice({text:'正在从米哈游下载 B服组件…'}); try {setNotice({text:await api.prepareBilibili()})} catch(e) {setNotice({text:errorMessage(e),error:true})} finally {setBusy(false)} }}><ArrowDownToLine size={17} /></button>}
   <button className="launch-button" onClick={() => void start()} disabled={!canLaunch}>{busy ? <LoaderCircle size={19} className="spin" /> : <Play size={18} fill="currentColor" />}<span>{launchLabel}</span></button>
  </footer>

  {notice && <div role={notice.error ? 'alert' : 'status'} className={`toast ${notice.error ? 'error' : ''}`}>{notice.error ? <CircleAlert size={16} /> : <Check size={16} />}<span>{notice.text}</span><button aria-label="关闭提示" onClick={() => setNotice(null)}><X size={14} /></button></div>}
 </div>
}
