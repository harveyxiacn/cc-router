import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowUpRight, Download, RefreshCw, RotateCcw, ShieldCheck } from 'lucide-react'
import { backend } from './api'
import type { UpdateInfo } from './api'
import { Dialog, Spinner } from './components'
import { errorMessage } from './model'

const quietPeriod = 30_000
const phaseText: Record<string,string> = { idle:'等待检查', checking:'正在检查', available:'发现新版本', downloading:'正在下载', staging:'正在验证更新', ready:'更新已准备好', applying:'正在重启更新', installing:'正在重启更新', recovering:'正在恢复上一版本', probation:'正在确认新版本启动', upToDate:'已是最新版本', current:'已是最新版本', error:'更新暂不可用', disabled:'自动更新已关闭', unsupported:'当前安装不支持自动替换', 'rolled-back':'已恢复上一版本', rolledBack:'已恢复上一版本' }

export function useUpdates(blocked: boolean, snapshotReady: boolean) {
  const [status,setStatus]=useState<UpdateInfo|null>(null)
  const [error,setError]=useState('')
  const [busy,setBusy]=useState('')
  const [idle,setIdle]=useState(false)
  const [confirmRollback,setConfirmRollback]=useState(false)
  const state=useRef({blocked,confirmRollback});state.current={blocked,confirmRollback}
  const lastInteraction=useRef(Date.now())
  const idleValue=useRef<boolean|null>(null)
  const idleQueue=useRef(Promise.resolve(true))
  const idleError=useRef('')
  const operation=useRef(false)
  const attemptedVersion=useRef('')
  const alive=useRef(true)
  const reported=useRef(false)
  const pollSequence=useRef(0)
  const poll=useCallback(async()=>{
    const sequence=++pollSequence.current
    try {const result=await backend().GetUpdateStatus();if(alive.current&&sequence===pollSequence.current)setStatus(result)}
    catch(e){if(alive.current)setError(errorMessage(e))}
  },[])
  const notifyIdle=useCallback((value:boolean)=>{
    if(idleValue.current===value)return idleQueue.current
    idleValue.current=value;if(alive.current)setIdle(value)
    idleQueue.current=idleQueue.current.then(async()=>{await backend().SetUpdateIdle(value);return true}).catch(e=>{idleError.current=errorMessage(e);if(idleValue.current===value){idleValue.current=null;if(alive.current)setIdle(false)};if(alive.current)setError(idleError.current);return false})
    return idleQueue.current
  },[])
  useEffect(()=>{
    alive.current=true;void poll()
    const interval=window.setInterval(()=>{void poll()},5000)
    return()=>{alive.current=false;window.clearInterval(interval);pollSequence.current++;void notifyIdle(false)}
  },[poll,notifyIdle])
  useEffect(()=>{
    if(!snapshotReady||reported.current)return
    reported.current=true
    // Effects run after React commits the first usable workspace to the screen.
    void backend().ReportReady().catch(e=>{if(alive.current)setError(errorMessage(e))})
  },[snapshotReady])
  useEffect(()=>{
    function interaction(){lastInteraction.current=Date.now();void notifyIdle(false)}
    for(const event of ['keydown','pointerdown','mousemove','wheel','touchstart'])window.addEventListener(event,interaction,{passive:true})
    void notifyIdle(false)
    const timer=window.setInterval(()=>{if(operation.current)return;const safe=!state.current.blocked&&!state.current.confirmRollback&&snapshotReady;void notifyIdle(safe&&Date.now()-lastInteraction.current>=quietPeriod)},1000)
    return()=>{window.clearInterval(timer);for(const event of ['keydown','pointerdown','mousemove','wheel','touchstart'])window.removeEventListener(event,interaction)}
  },[notifyIdle,snapshotReady])
  useEffect(()=>{if(blocked||confirmRollback)void notifyIdle(false)},[blocked,confirmRollback,notifyIdle])

  const apply=useCallback(async(manual:boolean)=>{
    if(operation.current||state.current.blocked||state.current.confirmRollback||!snapshotReady||status?.phase!=='ready')return
    if(!manual&&(!status.autoUpdate||!idleValue.current||Date.now()-lastInteraction.current<quietPeriod||attemptedVersion.current===status.latestVersion))return
    operation.current=true;setBusy('apply');setError('')
    if(!manual)attemptedVersion.current=status.latestVersion
    try {
      if(!await notifyIdle(true))throw new Error(idleError.current||'无法确认安全重启状态，请重试。')
      if(state.current.blocked||state.current.confirmRollback||!manual&&Date.now()-lastInteraction.current<quietPeriod){operation.current=false;setBusy('');return}
      await backend().ApplyUpdate()
      // Keep the controls disabled until the native wrapper closes this window.
    } catch(e){setError(errorMessage(e));operation.current=false;setBusy('');lastInteraction.current=Date.now();await notifyIdle(false)}
  },[snapshotReady,status,notifyIdle])
  useEffect(()=>{if(idle&&status?.phase==='ready'&&status.autoUpdate)void apply(false)},[idle,status,apply])
  async function run(key:string,action:()=>Promise<void>){
    if(operation.current)return
    operation.current=true;setBusy(key);setError('');lastInteraction.current=Date.now();await notifyIdle(false)
    try {await action();await poll()}catch(e){setError(errorMessage(e))}finally{operation.current=false;setBusy('')}
  }
  return {status,error,busy,idle,confirmRollback,setConfirmRollback,
    check:()=>run('check',async()=>{const result=await backend().CheckForUpdates();setStatus(result)}),
    toggle:(enabled:boolean)=>run('toggle',()=>backend().SetAutoUpdate(enabled)),
    prepare:()=>run('prepare',()=>backend().PrepareUpdate()),
    apply:()=>apply(true),
    release:()=>run('release',()=>backend().OpenReleasePage()),
    rollback:async()=>{
      if(blocked||!snapshotReady||operation.current)return
      operation.current=true;setBusy('rollback');setError('');setConfirmRollback(false)
      try{if(!await notifyIdle(true))throw new Error(idleError.current||'无法确认安全重启状态，请重试。');await backend().RollbackUpdate()}catch(e){setError(errorMessage(e));operation.current=false;setBusy('');await notifyIdle(false)}
    },
  }
}

export function UpdatesPanel({updates,blocked}:{updates:ReturnType<typeof useUpdates>;blocked:boolean}) {
  const {status,busy,error}=updates
  const percent=Math.max(0,Math.min(100,Number.isFinite(status?.progress)?status!.progress*100:0))
  const downloading=status?.phase==='downloading'||status?.phase==='staging'
  return <section className="updates-panel">
    <div className="section-heading"><h2>更新与关于 <span className="badge default-badge">CC Router</span></h2><span>公开发行 · 签名校验</span></div>
    <div className="updates-summary"><span className="update-shield"><ShieldCheck size={25}/></span><div><span className="eyebrow">KEEP THE WORKSPACE CURRENT</span><h3>{phaseText[status?.phase??'']||'更新状态'}</h3><p>当前版本 <code>{status?.currentVersion||'读取中'}</code>{status?.latestVersion&&<> · 最新版本 <code>{status.latestVersion}</code></>}{status?.prerelease&&<span className="badge">预发行通道</span>}</p></div></div>
    <label className="auto-update-toggle"><input type="checkbox" aria-label="后台自动更新" checked={status?.autoUpdate??false} disabled={!!busy||!status} onChange={e=>{void updates.toggle(e.target.checked)}}/><span><strong>后台自动更新</strong><small>后台检查并下载已签名的官方发行包；没有未保存内容且停止操作 30 秒后，重启到新版本。已有工作会话结束后再安装。</small></span></label>
    {(status?.message||downloading)&&<div className="update-phase" role="status"><p>{status?.message}</p>{downloading&&<><div className="update-progress" role="progressbar" aria-label="更新下载进度" aria-valuenow={percent} aria-valuemin={0} aria-valuemax={100}><span style={{width:`${percent}%`}}/></div><small>{Math.round(percent)}%</small></>}</div>}
    {status?.phase==='ready'&&<p className="update-wait">{blocked?'请先保存交接草稿并关闭当前弹窗，更新会继续等待。':updates.idle?'已满足空闲条件，正在准备安全重启。':'更新已验证完成；自动安装将等待 30 秒无操作，也可明确选择现在重启。'}</p>}
    {(error||status?.error)&&<p className="form-error" role="alert">{error||status?.error}</p>}
    {status?.notes&&<details className="update-notes"><summary>查看发行说明</summary><pre>{status.notes}</pre></details>}
    <div className="update-actions"><button className="button secondary" disabled={!!busy||!status} onClick={()=>{void updates.check()}}>{busy==='check'?<Spinner/>:<RefreshCw size={15}/>}检查更新</button>
      {status?.updateAvailable&&status.phase!=='ready'&&!downloading&&<button className="button secondary" disabled={!!busy} onClick={()=>{void updates.prepare()}}><Download size={15}/>下载并验证</button>}
      {status?.phase==='ready'&&<button className="button primary" disabled={!!busy||blocked} onClick={()=>{void updates.apply()}}>{busy==='apply'?<Spinner/>:<RefreshCw size={15}/>}现在重启更新</button>}
      <button className="button text-button" disabled={!!busy} onClick={()=>{void updates.release()}}>官方发行页<ArrowUpRight size={14}/></button>
      {status?.canRollback&&<button className="button secondary" disabled={!!busy||blocked} onClick={()=>updates.setConfirmRollback(true)}><RotateCcw size={15}/>回退上一版本</button>}
    </div><p className="update-footnote">{status?.checkedAt&&Number.isFinite(Date.parse(status.checkedAt))&&<>最近检查：{new Date(status.checkedAt).toLocaleString('zh-CN',{hour12:false})}。<br/></>}更新只替换桌面应用与配套 CLI；账号凭据、会话历史与项目文件保留在原位置。新版本启动健康检查失败时恢复上一版本。</p>
    {updates.confirmRollback&&<Dialog title="回退上一版本" description={`将恢复 ${status?.rollbackVersion||'保留的上一版本'}，并重新启动工作台。`} busy={!!busy} onClose={()=>updates.setConfirmRollback(false)}><p className="muted">账号配置与工作文件会保留。请确认当前没有未保存的内容。</p><div className="dialog-actions"><button className="button secondary" disabled={!!busy} onClick={()=>updates.setConfirmRollback(false)}>取消</button><button className="button primary" disabled={!!busy||blocked} onClick={()=>{void updates.rollback()}}>确认回退并重启</button></div></Dialog>}
  </section>
}
