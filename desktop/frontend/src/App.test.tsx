// These fixtures replace only the native IPC boundary in tests. The shipped app
// requires the actual Wails bridge and contains no simulated backend or accounts.
import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    RecordManualUsage: async () => {}, ClearManualUsage: async () => {}, ListBackups: async () => [], CreateBackup: async () => { throw new Error('Not requested') }, PreviewBackup: async () => { throw new Error('Not requested') }, RestoreBackup: async () => { throw new Error('Not requested') },
    GetUpdateStatus: async () => ({ currentVersion:'0.1.0-alpha.1',latestVersion:'',releaseURL:'',notes:'',checkedAt:'',updateAvailable:false,prerelease:true,autoUpdate:false,phase:'idle',progress:0,message:'',error:'',canRollback:false,rollbackVersion:'' }),
    CheckForUpdates: async () => { throw new Error('Not requested by this test') },
    SetAutoUpdate: async () => {}, SetUpdateIdle: async () => {}, PrepareUpdate: async () => {}, ApplyUpdate: async () => {}, RollbackUpdate: async () => {}, ReportReady: async () => {}, OpenReleasePage: async () => {},
  }
  window.go = { main: { App: app } }
  return { snapshot, launches, app }
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

describe('manual quota records and local backups', () => {
  it('records manual usage separately from official reports without launching a session', async () => {
    const data = fixture()
    data.snapshot.accounts.push({ id: 'personal-id', name: 'personal', label: '个人', isDefault: true, active: false, usage: null })
    let recorded: unknown
    const resetDate = new Date(Date.now() + 3600000); resetDate.setSeconds(0, 0)
    const reset = new Date(resetDate.getTime() - resetDate.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
    Object.assign(data.app, { RecordManualUsage: async (name: string, input: unknown) => { recorded = { name, input } } })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: 'personal 更多操作' }))
    fireEvent.click(screen.getByRole('button', { name: '手动记录额度 / 本地限额' }))
    fireEvent.change(screen.getByLabelText('5 小时已用百分比'), { target: { value: '95' } })
    fireEvent.change(screen.getByLabelText('5 小时重置时间'), { target: { value: reset } })
    fireEvent.change(screen.getByLabelText('本地限额截止时间'), { target: { value: reset } })
    fireEvent.click(screen.getByRole('button', { name: '保存手动记录' }))
    await waitFor(() => expect(recorded).toEqual({ name: 'personal', input: { fiveHour: { usedPercentage: 95, resetsAt: resetDate.getTime() / 1000 }, sevenDay: null, limitedUntil: resetDate.getTime() / 1000 } }))
    expect(data.launches).toEqual([])
    expect(data.snapshot.accounts[0].usage).toBeNull()
  })
  it('requires a reset time for a percentage and retains the form on failure', async () => {
    const data = fixture()
    data.snapshot.accounts.push({ id: 'personal-id', name: 'personal', label: '个人', isDefault: true, active: false, usage: null })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: 'personal 更多操作' }))
    fireEvent.click(screen.getByRole('button', { name: '手动记录额度 / 本地限额' }))
    fireEvent.change(screen.getByLabelText('7 天已用百分比'), { target: { value: '101' } })
    fireEvent.click(screen.getByRole('button', { name: '保存手动记录' }))
    expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringMatching(/0–100/))
    fireEvent.change(screen.getByLabelText('7 天已用百分比'), { target: { value: '95' } })
    fireEvent.click(screen.getByRole('button', { name: '保存手动记录' }))
    expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringMatching(/重置时间/))
    expect(screen.getByRole('dialog')).toBeTruthy()
  })
  it('previews local account and project configuration and requires explicit restore review', async () => {
    const data = fixture()
    const info = { id: 'backup-1', createdAt: '2026-10-02T00:00:00Z', reason: 'manual', accountCount: 1, bindingCount: 1, restorable: true }
    const restored: unknown[][] = []
    const updateStatus = data.app.GetUpdateStatus
    data.app.GetUpdateStatus = async () => ({ ...await updateStatus(), phase: 'ready', updateAvailable: true, latestVersion: '0.2.0' })
    Object.assign(data.app, { ListBackups: async () => [info], CreateBackup: async () => info, PreviewBackup: async () => ({ info, accounts: [{ id: 'old-id', name: 'work', label: '工作' }], defaultAccountId: 'old-id', projectBindings: { '/test-project': 'old-id' }, digest: 'reviewed-digest' }), RestoreBackup: async (...args: unknown[]) => { restored.push(args); return info } })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: '环境诊断' }))
    fireEvent.click(await screen.findByRole('button', { name: '查看备份列表' }))
    fireEvent.click(await screen.findByRole('button', { name: '预览备份 backup-1' }))
    const dialog = await screen.findByRole('dialog')
    expect((screen.getByRole('button', { name: '现在重启更新' }) as HTMLButtonElement).disabled).toBe(true)
    expect(within(dialog).getByText('/test-project')).toBeTruthy()
    const restore = within(dialog).getByRole('button', { name: '恢复此备份' }) as HTMLButtonElement
    expect(restore.disabled).toBe(true)
    expect(restored).toEqual([])
    fireEvent.click(within(dialog).getByRole('checkbox', { name: '我已审阅账号与项目绑定，确认恢复本地配置' }))
    fireEvent.click(restore)
    await waitFor(() => expect(restored).toEqual([['backup-1', 'reviewed-digest', true]]))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(data.launches).toEqual([])
  })
  it('recovers a corrupt registry even if refreshing the backup list later fails', async () => {
    const data = fixture()
    const info = { id: 'backup-1', createdAt: '2026-10-02T00:00:00Z', reason: 'manual', accountCount: 1, bindingCount: 0, restorable: true }
    let corrupt = true
    Object.assign(data.app, { GetSnapshot: async () => { if (corrupt) throw new Error('登记文件损坏'); return { ...data.snapshot } }, ListBackups: async () => { if (!corrupt) throw new Error('备份列表暂不可读取'); return [info] }, PreviewBackup: async () => ({ info, accounts: [], defaultAccountId: '', projectBindings: {}, digest: 'digest' }), RestoreBackup: async () => { corrupt = false; return { ...info, id: 'evidence', restorable: false } } })
    render(<App />)
    expect(await screen.findByText('登记文件损坏')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '打开本地备份' }))
    fireEvent.click(await screen.findByRole('button', { name: '查看备份列表' }))
    const previewButton = await screen.findByRole('button', { name: '预览备份 backup-1' }) as HTMLButtonElement
    await waitFor(() => expect(previewButton.disabled).toBe(false))
    fireEvent.click(previewButton)
    fireEvent.click(await screen.findByRole('checkbox', { name: '我已审阅账号与项目绑定，确认恢复本地配置' }))
    fireEvent.click(screen.getByRole('button', { name: '恢复此备份' }))
    await waitFor(() => expect(screen.queryByText('登记文件损坏')).toBeNull())
    expect(await screen.findByText(/恢复前损坏文件已保留/)).toBeTruthy()
  })
  it('disables damaged backup entries and does not display a zero timestamp as a real date', async () => {
    const data = fixture()
    Object.assign(data.app, { ListBackups: async () => [{ id: 'bad', createdAt: '0001-01-01T00:00:00Z', reason: 'invalid', accountCount: 0, bindingCount: 0, restorable: false, error: '本地备份已损坏' }] })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: '环境诊断' }))
    fireEvent.click(await screen.findByRole('button', { name: '查看备份列表' }))
    expect(await screen.findByText('本地备份已损坏')).toBeTruthy()
    expect(screen.getByText('时间未知')).toBeTruthy()
    expect((screen.getByRole('button', { name: '预览备份 bad' }) as HTMLButtonElement).disabled).toBe(true)
  })
  it('shows expired manual records separately and clears them only after confirmation', async () => {
    const data = fixture()
    data.snapshot.accounts.push({ id: 'personal-id', name: 'personal', label: '个人', isDefault: true, active: false, usage: null, manualUsage: { source: 'manual', observedAt: new Date().toISOString(), fiveHour: { usedPercentage: 95, resetsAt: Math.floor(Date.now() / 1000) - 10 }, sevenDay: null, limitedUntil: Math.floor(Date.now() / 1000) - 10 } })
    Object.assign(data.app, { ClearManualUsage: async () => { data.snapshot.accounts[0].manualUsage = null } })
    render(<App />)
    const manual = await screen.findByRole('region', { name: '手动额度记录' })
    expect(within(manual).getByText('95%')).toBeTruthy()
    expect(within(manual).getByText(/已过期/)).toBeTruthy()
    expect(within(manual).queryByText(/建议核对官方额度/)).toBeNull()
    expect(screen.getAllByText('暂无上报').length).toBe(2)
    fireEvent.click(screen.getByRole('button', { name: 'personal 更多操作' }))
    fireEvent.click(screen.getByRole('button', { name: '清除手动记录' }))
    expect(data.snapshot.accounts[0].manualUsage).not.toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '确认清除' }))
    await waitFor(() => expect(screen.queryByRole('region', { name: '手动额度记录' })).toBeNull())
    expect(data.launches).toEqual([])
  })
  it('retains manual input when the native save is rejected', async () => {
    const data = fixture()
    data.snapshot.accounts.push({ id: 'personal-id', name: 'personal', label: '个人', isDefault: true, active: false, usage: null })
    Object.assign(data.app, { RecordManualUsage: async () => { throw new Error('测试写入失败') } })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: 'personal 更多操作' }))
    fireEvent.click(screen.getByRole('button', { name: '手动记录额度 / 本地限额' }))
    const date = new Date(Date.now() + 3600000)
    const reset = new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
    fireEvent.change(screen.getByLabelText('本地限额截止时间'), { target: { value: reset } })
    fireEvent.click(screen.getByRole('button', { name: '保存手动记录' }))
    expect(await screen.findByText('测试写入失败')).toBeTruthy()
    expect((screen.getByLabelText('本地限额截止时间') as HTMLInputElement).value).toBe(reset)
    expect(screen.getByRole('dialog')).toBeTruthy()
  })
  it('prevents backup restore and update restart while a handoff draft is unsaved', async () => {
    const data = fixture()
    data.snapshot.accounts.push({ id: 'personal-id', name: 'personal', label: '个人', isDefault: true, active: false, usage: null })
    const info = { id: 'backup-1', createdAt: new Date().toISOString(), reason: 'manual', accountCount: 1, bindingCount: 0, restorable: true }
    let restores = 0
    Object.assign(data.app, { ChooseDirectory: async () => '/test-project', ReadHandoff: async () => ({ path: '/test-project/.cc-router/handoff.md', content: 'saved', digest: 'handoff-digest' }), ListBackups: async () => [info], PreviewBackup: async () => ({ info, accounts: [], defaultAccountId: '', projectBindings: {}, digest: 'backup-digest' }), RestoreBackup: async () => { restores++; return info }, GetUpdateStatus: async () => ({ currentVersion: '0.1.0-alpha.1', latestVersion: '0.2.0', releaseURL: '', notes: '', checkedAt: '', updateAvailable: true, prerelease: false, autoUpdate: false, phase: 'ready', progress: 1, message: '', error: '', canRollback: false, rollbackVersion: '' }) })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: '选择项目文件夹' }))
    await screen.findByTitle('/test-project')
    fireEvent.click(screen.getByRole('button', { name: '工作交接' }))
    fireEvent.click(screen.getByRole('button', { name: '读取' }))
    await waitFor(() => expect((screen.getByLabelText('交接内容') as HTMLTextAreaElement).value).toBe('saved'))
    fireEvent.change(screen.getByLabelText('交接内容'), { target: { value: 'unsaved draft' } })
    fireEvent.click(screen.getByRole('button', { name: '环境诊断' }))
    fireEvent.click(screen.getByRole('button', { name: '查看备份列表' }))
    const preview = await screen.findByRole('button', { name: '预览备份 backup-1' }) as HTMLButtonElement
    await waitFor(() => expect(preview.disabled).toBe(false)); fireEvent.click(preview)
    const review = await screen.findByRole('checkbox', { name: '我已审阅账号与项目绑定，确认恢复本地配置' }) as HTMLInputElement
    expect(review.disabled).toBe(true)
    expect((screen.getByRole('button', { name: '恢复此备份' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: '现在重启更新' }) as HTMLButtonElement).disabled).toBe(true)
    expect(restores).toBe(0)
  })
  it('keeps a successfully created backup visible if fetching the full list fails', async () => {
    const data = fixture()
    const info = { id: 'created-backup', createdAt: new Date().toISOString(), reason: 'manual', accountCount: 0, bindingCount: 0, restorable: true }
    Object.assign(data.app, { CreateBackup: async () => info, ListBackups: async () => { throw new Error('列表不可用') } })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: '环境诊断' }))
    const create = screen.getByRole('button', { name: '创建本地备份' }) as HTMLButtonElement
    await waitFor(() => expect(create.disabled).toBe(false)); fireEvent.click(create)
    expect(await screen.findByText('本地配置备份已创建。')).toBeTruthy()
    expect(screen.getByRole('button', { name: '预览备份 created-backup' })).toBeTruthy()
  })
})
