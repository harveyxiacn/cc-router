import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { AppBinding, UpdateInfo } from './api'
import { UpdatesPanel, useUpdates } from './updates'

afterEach(() => { cleanup(); delete window.go; vi.useRealTimers() })
function fixture() {
  const status: UpdateInfo = { currentVersion:'0.1.0-alpha.1', latestVersion:'0.2.0', releaseURL:'https://github.com/harveyxiacn/cc-router/releases', notes:'已签名的发行说明', checkedAt:'', updateAvailable:true, prerelease:false, autoUpdate:true, phase:'ready', progress:100, message:'更新已准备好', error:'', canRollback:true, rollbackVersion:'0.0.9' }
  const calls = { apply:vi.fn(async()=>{}), rollback:vi.fn(async()=>{}), idle:vi.fn(async(_idle:boolean)=>{}), ready:vi.fn(async()=>{}), auto:vi.fn(async(enabled:boolean)=>{status.autoUpdate=enabled}) }
  window.go={ main:{ App:{GetUpdateStatus:async()=>({...status}), CheckForUpdates:async()=>({...status}), ApplyUpdate:calls.apply, RollbackUpdate:calls.rollback, SetUpdateIdle:calls.idle, ReportReady:calls.ready, SetAutoUpdate:calls.auto, PrepareUpdate:async()=>{}, OpenReleasePage:async()=>{}} as unknown as AppBinding } }
  return {status,calls}
}
function Harness({blocked=false,ready=true}:{blocked?:boolean;ready?:boolean}) {const updates=useUpdates(blocked,ready);return <UpdatesPanel updates={updates} blocked={blocked}/>}
async function mount(blocked=false) { render(<Harness blocked={blocked}/>);await act(async()=>{await Promise.resolve()}) }

describe('signed update controls',()=>{
  it('reports health after a rendered snapshot and applies only after 30 seconds without interaction',async()=>{
    vi.useFakeTimers();const {calls}=fixture();await mount()
    await act(async()=>{vi.advanceTimersByTime(29_000)})
    expect(calls.ready).toHaveBeenCalledTimes(1);expect(calls.apply).not.toHaveBeenCalled()
    fireEvent.keyDown(window,{key:'a'})
    await act(async()=>{vi.advanceTimersByTime(29_000)})
    expect(calls.apply).not.toHaveBeenCalled()
    await act(async()=>{vi.advanceTimersByTime(1000)})
    expect(calls.apply).toHaveBeenCalledTimes(1)
    await act(async()=>{vi.advanceTimersByTime(10_000)})
    expect(calls.apply).toHaveBeenCalledTimes(1)
  })
  it('does not apply while a draft or dialog blocks restarting',async()=>{
    vi.useFakeTimers();const {calls}=fixture();await mount(true)
    await act(async()=>{vi.advanceTimersByTime(60_000)})
    expect(calls.apply).not.toHaveBeenCalled();expect(calls.idle).not.toHaveBeenCalledWith(true)
    expect((screen.getByRole('button',{name:'现在重启更新'}) as HTMLButtonElement).disabled).toBe(true)
  })
  it('allows deliberate restart without waiting for the inactivity timer and confirms rollback',async()=>{
    const {calls}=fixture();await mount()
    fireEvent.click(screen.getByRole('button',{name:'现在重启更新'}))
    await act(async()=>{await Promise.resolve()});expect(calls.apply).toHaveBeenCalledTimes(1);expect(calls.idle).toHaveBeenCalledWith(true)
  })
  it('lets the user disable automatic updates and keeps release navigation in the native bridge',async()=>{
    const {calls}=fixture();await mount()
    fireEvent.click(screen.getByRole('checkbox',{name:'后台自动更新'}))
    await act(async()=>{await Promise.resolve()});expect(calls.auto).toHaveBeenCalledWith(false)
  })
  it('does not request a restart when reporting the safe state fails',async()=>{
    const {calls}=fixture();calls.idle.mockImplementation(async(idle:boolean)=>{if(idle)throw new Error('安全状态无法确认')})
    await mount();fireEvent.click(screen.getByRole('button',{name:'现在重启更新'}))
    await act(async()=>{await Promise.resolve()})
    expect(calls.apply).not.toHaveBeenCalled();expect(screen.getByRole('alert').textContent).toContain('安全状态无法确认')
  })
  it('renders the native fractional download progress as a percentage',async()=>{
    const {status}=fixture();status.phase='downloading';status.progress=.5
    await mount();expect(screen.getByRole('progressbar').getAttribute('aria-valuenow')).toBe('50')
  })
})
