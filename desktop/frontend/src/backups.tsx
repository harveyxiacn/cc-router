import { useEffect, useState } from 'react'
import { Archive, Info, RotateCcw } from 'lucide-react'
import type { BackupInfo, BackupPreview } from './api'
import { backend } from './api'
import { Dialog, Spinner } from './components'

type Run = (key: string, action: () => Promise<void>, message?: string, refreshAfter?: boolean) => Promise<void>
const reasons: Record<string, string> = { manual: '手动创建', 'pre-restore': '恢复前保护', 'pre-update': '升级前保护', invalid: '损坏备份' }
function timestamp(value: string) { const date = new Date(value); return Number.isFinite(date.getTime()) && date.getUTCFullYear() >= 1970 ? date.toLocaleString('zh-CN') : '时间未知' }
export function BackupsPanel({ busy, dirty, unavailable, actionError, run, onDialogChange, onRestored }: { busy: boolean; dirty: boolean; unavailable: boolean; actionError: string; run: Run; onDialogChange: (open: boolean) => void; onRestored: () => void }) {
  const [backups, setBackups] = useState<BackupInfo[] | null>(null)
  const [preview, setPreview] = useState<BackupPreview | null>(null)
  const [reviewed, setReviewed] = useState(false)
  const [previous, setPrevious] = useState<BackupInfo | null>(null)
  const [listError, setListError] = useState('')
  useEffect(() => () => onDialogChange(false), [onDialogChange])
  async function list() { await run('backups-list', async () => { setBackups(await backend().ListBackups()); setListError('') }, '', false) }
  async function create() { await run('backups-create', async () => {
    const created = await backend().CreateBackup()
    try { setBackups(await backend().ListBackups()); setListError('') }
    catch { setBackups(current => [created, ...(current ?? []).filter(item => item.id !== created.id)]); setListError('备份已创建，但完整列表暂不可刷新，请稍后重新查看。') }
  }, '本地配置备份已创建。', false) }
  async function inspect(id: string) { await run('backups-preview', async () => { const result = await backend().PreviewBackup(id); setReviewed(false); onDialogChange(true); setPreview(result) }, '', false) }
  function close() { onDialogChange(false); setPreview(null) }
  async function restore() {
    if (!preview || !reviewed || dirty) return
    await run('backups-restore', async () => {
      const before = await backend().RestoreBackup(preview.info.id, preview.digest, true)
      setPrevious(before); close(); setReviewed(false); onRestored()
      try { setBackups(await backend().ListBackups()); setListError('') }
      catch { setBackups(null); setListError('配置已恢复，但备份列表暂不可刷新，请稍后重新查看。') }
    }, '本地账号与项目配置已恢复；官方账号目录保持原样。')
  }
  function accountName(id: string) { const account = preview?.accounts.find(account => account.id === id); return account ? `${account.label || account.name} · ${account.name}` : id ? '未知账号' : '未设默认' }
  return <section className="surface backups-panel"><div className="backup-panel-heading"><span className="surface-icon"><Archive size={22} /></span><div><span className="eyebrow">LOCAL CONFIGURATION</span><h2>本地配置备份</h2><p>保存账号登记、默认选择与项目绑定；恢复前先审阅。</p></div><div className="metadata-actions"><button className="button secondary" disabled={busy} onClick={() => { void list() }}>查看备份列表</button><button className="button primary" disabled={busy || unavailable} onClick={() => { void create() }}><Archive size={15} />创建本地备份</button></div></div><p className="backup-boundary"><Info size={15} />只处理这台设备的工具配置，保留账号 ID 与本地项目绑定。官方配置、凭据和会话历史不会备份或替换；跨设备标签迁移请用元数据导入／导出。</p>
    {listError && <p className="form-error" role="alert">{listError}</p>}{previous && <div className="backup-previous"><div><strong>{previous.restorable ? '恢复前配置已备份，可用此备份撤销' : '恢复前损坏文件已保留供检查，不能用于恢复'}</strong><code>{previous.id}</code></div>{previous.restorable && <button className="button secondary" disabled={busy} onClick={() => { void inspect(previous.id) }}>审阅恢复前配置</button>}</div>}
    {backups && (backups.length ? <ul className="backup-list">{backups.map(backup => <li key={backup.id}><div><strong>{timestamp(backup.createdAt)}</strong><small>{reasons[backup.reason] || backup.reason} · {backup.accountCount} 个账号 · {backup.bindingCount} 个绑定</small><code>{backup.id}</code>{backup.error && <small className="form-error">{backup.error}</small>}</div><span className={`badge ${backup.restorable ? 'default-badge' : 'blocked-badge'}`}>{backup.restorable ? '可恢复' : backup.error ? '损坏不可恢复' : '仅保留取证'}</span><button className="button secondary" aria-label={`预览备份 ${backup.id}`} disabled={busy || !backup.restorable} onClick={() => { void inspect(backup.id) }}>预览与审阅</button></li>)}</ul> : <p className="backup-empty">尚无本地备份。可先创建一份，再进行配置调整。</p>)}
    {preview && <Dialog title="审阅本地配置恢复" description="恢复会替换当前账号登记、默认选择和项目绑定，并保留恢复前的配置备份。官方账号目录、登录凭据与交接文件不变。" busy={busy} onClose={close}><div className="backup-preview"><div className="backup-preview-summary"><strong>{timestamp(preview.info.createdAt)}</strong><code>{preview.info.id}</code><span>默认账号：{accountName(preview.defaultAccountId)}</span></div><h3>备份内账号</h3>{preview.accounts.length ? <ul>{preview.accounts.map(account => <li key={account.id}><strong>{account.label || account.name}</strong><code>{account.name}</code></li>)}</ul> : <p className="muted">没有账号登记</p>}<h3>备份内项目绑定</h3>{Object.keys(preview.projectBindings).length ? <ul>{Object.entries(preview.projectBindings).map(([path, id]) => <li className="backup-binding" key={path}><code>{path}</code><span>{accountName(id)}</span></li>)}</ul> : <p className="muted">没有项目绑定</p>}</div>{dirty && <p className="form-error" role="alert">交接草稿尚未保存，请先保存或明确放弃草稿，再恢复配置。</p>}{actionError && <p className="form-error" role="alert">{actionError}</p>}<label className="backup-review-control"><input type="checkbox" checked={reviewed} disabled={busy || dirty} onChange={event => setReviewed(event.target.checked)} /><span>我已审阅账号与项目绑定，确认恢复本地配置</span></label><div className="dialog-actions"><button className="button secondary" disabled={busy} onClick={close}>取消</button><button className="button primary" disabled={busy || dirty || !reviewed || !preview.info.restorable} onClick={() => { void restore() }}>{busy ? <Spinner /> : <RotateCcw size={15} />}恢复此备份</button></div></Dialog>}
  </section>
}
