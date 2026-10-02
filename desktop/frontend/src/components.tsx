import { useEffect, useId, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Check, LoaderCircle, X } from 'lucide-react'
import type { Account, UsageSnapshot } from './api'
import { accountNameIsValid, quotaView } from './model'

export function Spinner() { return <LoaderCircle size={16} className="spinner" aria-hidden="true" /> }

export function Dialog({ title, description, children, busy = false, onClose }: { title: string; description?: string; children: ReactNode; busy?: boolean; onClose: () => void }) {
  const label = useId()
  const container = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const dialog = container.current
    dialog?.querySelector<HTMLElement>('input, textarea, button')?.focus()
    return () => previous?.focus()
  }, [])
  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === 'Escape' && !busy) onClose()
    if (event.key !== 'Tab') return
    const focusable = [...(container.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]') ?? [])]
    const first = focusable[0], last = focusable.at(-1)
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
  }
  return <div className="dialog-backdrop" onMouseDown={event => { if (event.target === event.currentTarget && !busy) onClose() }}>
    <div className="dialog" role="dialog" aria-modal="true" aria-labelledby={label} ref={container} onKeyDown={onKeyDown}>
      <button className="icon-button dialog-close" aria-label="关闭对话框" disabled={busy} onClick={onClose}><X size={18} /></button>
      <span className="eyebrow">CC ROUTER / WORKSPACE</span><h2 id={label}>{title}</h2>
      {description && <p className="muted dialog-description">{description}</p>}
      {children}
    </div>
  </div>
}

export function AccountForm({ account, busy, onSubmit, onCancel }: { account?: Account; busy: boolean; onSubmit: (name: string, label: string, autoLogin: boolean) => void; onCancel: () => void }) {
  const [name, setName] = useState(account?.name ?? '')
  const [label, setLabel] = useState(account?.label ?? '')
  const [error, setError] = useState('')
  const [autoLogin, setAutoLogin] = useState(!account)
  const id = useId()
  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!accountNameIsValid(name)) { setError('名称需为 1–64 位小写字母、数字、连字符或下划线，首位为字母或数字。'); return }
    if (new TextEncoder().encode(label).length > 256 || /[\p{Cc}\p{Cf}]/u.test(label)) { setError('标签最多 256 字节，不能包含控制字符。'); return }
    setError(''); onSubmit(name, label || name, autoLogin)
  }
  return <form className="account-form" onSubmit={submit} noValidate>
    <label htmlFor={`${id}-name`}>账号名称</label>
    <input id={`${id}-name`} value={name} onChange={event => setName(event.target.value)} placeholder="例如 personal、work" autoComplete="off" spellCheck={false} maxLength={64} disabled={busy} autoFocus />
    <p className="field-hint">用于命令行选择账号；显示标签可使用中文。</p>
    <label htmlFor={`${id}-label`}>显示标签</label>
    <input id={`${id}-label`} value={label} onChange={event => setLabel(event.target.value)} placeholder="例如 个人账号" disabled={busy} />
    {!account && <label className="auto-login-control"><input type="checkbox" aria-label="创建后打开官方登录" checked={autoLogin} disabled={busy} onChange={event => setAutoLogin(event.target.checked)} /><span><strong>创建后打开官方登录</strong><small>官方 CLI 负责浏览器授权，无需先选择项目。</small></span></label>}
    {error && <p className="form-error" role="alert">{error}</p>}
    <div className="dialog-actions"><button type="button" className="button secondary" onClick={onCancel} disabled={busy}>取消</button><button className="button primary" disabled={busy}>{busy && <Spinner />}{account ? '保存修改' : '添加账号'}</button></div>
  </form>
}

export function QuotaMeter({ title, usage, window }: { title: string; usage: UsageSnapshot | null; window: 'fiveHour' | 'sevenDay' }) {
  const view = quotaView(usage, window)
  return <div className={`quota-meter tone-${view.tone}`}>
    <div className="quota-label"><span>{title}</span><strong className={view.known ? 'quota-number' : 'quota-unknown'}>{view.text}</strong></div>
    {view.known ? <div className="meter-track" role="progressbar" aria-label={`${title}已用额度${view.stale ? '（过期上报）' : ''}`} aria-valuenow={view.percentage} aria-valuemin={0} aria-valuemax={100}><span style={{ width: `${view.percentage}%` }} /></div> : <div className="meter-track meter-empty" aria-hidden="true" />}
    <p className="quota-caption">{view.stale ? '上报已过期 · ' : ''}{view.resetText}</p>
  </div>
}

export function HandoffReview({ saved, reviewed, onChange }: { saved: boolean; reviewed: boolean; onChange: (value: boolean) => void }) {
  return <label className={`review-control ${!saved ? 'disabled' : ''}`}>
    <input type="checkbox" checked={reviewed} disabled={!saved} onChange={event => onChange(event.target.checked)} />
    <span className="review-box" aria-hidden="true">{reviewed && <Check size={13} />}</span>
    <span><strong>我已审阅并保存当前交接内容</strong><small>{saved ? '确认后，将在另一个账号下启动新会话。' : '保存后可确认审阅'}</small></span>
  </label>
}
