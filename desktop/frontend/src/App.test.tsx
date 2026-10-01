// These fixtures replace only the native IPC boundary in tests. The shipped app
// requires the actual Wails bridge and contains no simulated backend or accounts.
import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import App from './App'
import type { AppBinding, Snapshot } from './api'

afterEach(() => { cleanup(); delete window.go })

function fixture(failLogin = false) {
  const snapshot: Snapshot = { accounts: [], project: '', boundAccount: '', dataDir: 'test-only-data-directory' }
  const launches: unknown[][] = []
  const app: AppBinding = {
    GetSnapshot: async () => ({ ...snapshot, accounts: [...snapshot.accounts] }),
    ChooseDirectory: async () => '',
    CreateAccount: async (name, label) => { snapshot.accounts.push({ id: 'test-only-id', name, label, isDefault: false, active: false, usage: null }) },
    RenameAccount: async () => {}, RemoveAccount: async () => {}, SetDefault: async () => {}, Bind: async () => {}, Unbind: async () => {},
    CheckAccount: async () => { throw new Error('Identity must not be checked automatically') },
    Diagnose: async () => ({ version: '', findings: [], error: '' }),
    CreateHandoff: async () => { throw new Error('Not requested by this test') },
    ReadHandoff: async () => { throw new Error('Not requested by this test') },
    SaveHandoff: async () => { throw new Error('Not requested by this test') },
    Launch: async (...args) => { launches.push(args); if (failLogin) throw new Error('测试终端不可用') },
    InstallUsage: async () => {}, ExportMetadata: async () => '', ImportMetadata: async () => '',
    GetUpdateStatus: async () => ({ currentVersion:'0.1.0-alpha.1',latestVersion:'',releaseURL:'',notes:'',checkedAt:'',updateAvailable:false,prerelease:true,autoUpdate:false,phase:'idle',progress:0,message:'',error:'',canRollback:false,rollbackVersion:'' }),
    CheckForUpdates: async () => { throw new Error('Not requested by this test') },
    SetAutoUpdate: async () => {}, SetUpdateIdle: async () => {}, PrepareUpdate: async () => {}, ApplyUpdate: async () => {}, RollbackUpdate: async () => {}, ReportReady: async () => {}, OpenReleasePage: async () => {},
  }
  window.go = { main: { App: app } }
  return { snapshot, launches }
}

async function create(name: string, autoLogin = true) {
  fireEvent.click(await screen.findByRole('button', { name: '添加第一个账号' }))
  fireEvent.change(screen.getByLabelText('账号名称'), { target: { value: name } })
  if (!autoLogin) fireEvent.click(screen.getByRole('checkbox', { name: '创建后打开官方登录' }))
  const dialog = screen.getByRole('dialog')
  fireEvent.click([...dialog.querySelectorAll('button')].find(button => button.textContent === '添加账号')!)
  await waitFor(() => { expect(screen.queryByRole('dialog')).toBeNull() })
}

describe('first login workflow', () => {
  it('creates an account then opens the official login without requiring a project', async () => {
    const data = fixture(); render(<App />)
    await create('personal')
    await waitFor(() => { expect(data.launches).toEqual([['personal', '', 'login', false]]) })
    expect(data.snapshot.accounts[0].name).toBe('personal')
    expect((await screen.findByRole('button', { name: '登录' }) as HTMLButtonElement).disabled).toBe(false)
    expect((screen.getByRole('button', { name: '启动工作会话' }) as HTMLButtonElement).disabled).toBe(true)
  })
  it('preserves the created account and explains a failed terminal launch', async () => {
    const data = fixture(true); render(<App />)
    await create('work')
    expect(await screen.findByText(/账号 work 已创建，但官方登录终端未能打开/)).toBeTruthy()
    expect(data.snapshot.accounts.map(account => account.name)).toEqual(['work'])
    expect((await screen.findByRole('button', { name: '登录' }) as HTMLButtonElement).disabled).toBe(false)
  })
  it('supports opting out and only opens login when the login button is clicked', async () => {
    const data = fixture(); render(<App />)
    await create('manual', false)
    const login = await screen.findByRole('button', { name: '登录' })
    expect(data.launches).toEqual([])
    fireEvent.click(login)
    await waitFor(() => { expect(data.launches).toEqual([['manual', '', 'login', false]]) })
  })
})
