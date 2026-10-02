import { useId, useState } from 'react'
import { Info, NotebookPen } from 'lucide-react'
import type { ManualInput, ManualRecord, UsageWindow } from './api'
import { Spinner } from './components'

function localInput(seconds: number | undefined) {
  if (!seconds) return ''
  const date = new Date(seconds * 1000)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}
function dateText(seconds: number) { return new Date(seconds * 1000).toLocaleString('zh-CN') }

export function ManualUsageForm({ record, busy, onSubmit, onCancel }: { record?: ManualRecord | null; busy: boolean; onSubmit: (input: ManualInput) => void; onCancel: () => void }) {
  const id = useId()
  const [fivePercent, setFivePercent] = useState(record?.fiveHour?.usedPercentage.toString() ?? '')
  const [fiveReset, setFiveReset] = useState(localInput(record?.fiveHour?.resetsAt))
  const [sevenPercent, setSevenPercent] = useState(record?.sevenDay?.usedPercentage.toString() ?? '')
  const [sevenReset, setSevenReset] = useState(localInput(record?.sevenDay?.resetsAt))
  const [limited, setLimited] = useState(localInput(record?.limitedUntil))
  const [error, setError] = useState('')
  function submit(event: React.FormEvent) {
    event.preventDefault()
    try {
      const now = Date.now() / 1000
      const parseTime = (value: string, field: string) => {
        const seconds = Math.floor(new Date(value).getTime() / 1000)
        if (!value || !Number.isFinite(seconds) || seconds <= now || seconds > now + 366 * 86400) throw new Error(`${field}需在未来一年以内。`)
        return seconds
      }
      const parseWindow = (percent: string, reset: string, label: string): UsageWindow | null => {
        if (!percent.trim() && !reset) return null
        const usedPercentage = Number(percent)
        if (!percent.trim() || !Number.isFinite(usedPercentage) || usedPercentage < 0 || usedPercentage > 100) throw new Error(`${label}已用百分比需为 0–100。`)
        return { usedPercentage, resetsAt: parseTime(reset, `${label}重置时间`) }
      }
      const input = { fiveHour: parseWindow(fivePercent, fiveReset, '5 小时'), sevenDay: parseWindow(sevenPercent, sevenReset, '7 天'), limitedUntil: limited ? parseTime(limited, '本地限额截止时间') : 0 }
      if (!input.fiveHour && !input.sevenDay && !input.limitedUntil) throw new Error('至少填写一个额度窗口或本地限额截止时间；清除记录请使用独立操作。')
      setError(''); onSubmit(input)
    } catch (err) { setError(err instanceof Error ? err.message : '请核对输入。') }
  }
  return <form className="manual-form" onSubmit={submit} noValidate>
    <p className="manual-source-note"><NotebookPen size={16} />来源：你的手动记录。不会覆盖官方状态栏，也不会自动切换账号。</p>
    {[{ label: '5 小时', percent: fivePercent, setPercent: setFivePercent, reset: fiveReset, setReset: setFiveReset, key: 'five' }, { label: '7 天', percent: sevenPercent, setPercent: setSevenPercent, reset: sevenReset, setReset: setSevenReset, key: 'seven' }].map(field => <fieldset className="manual-window-fields" key={field.key}><legend>{field.label}窗口 <span>选填</span></legend><div><label htmlFor={`${id}-${field.key}-percent`}>{field.label}已用百分比</label><input id={`${id}-${field.key}-percent`} type="number" min="0" max="100" step="any" value={field.percent} onChange={event => field.setPercent(event.target.value)} disabled={busy} placeholder="0–100" /></div><div><label htmlFor={`${id}-${field.key}-reset`}>{field.label}重置时间</label><input id={`${id}-${field.key}-reset`} type="datetime-local" value={field.reset} onChange={event => field.setReset(event.target.value)} disabled={busy} /></div></fieldset>)}
    <label htmlFor={`${id}-limited`}>本地限额截止时间</label><input id={`${id}-limited`} type="datetime-local" value={limited} onChange={event => setLimited(event.target.value)} disabled={busy} /><p className="field-hint">可单独标记“在此时间前受限”。全部时间使用设备的本地时区，须在未来一年内；到期标记失效，记录仍可查看或清除。</p>
    {error && <p className="form-error" role="alert">{error}</p>}
    <div className="dialog-actions"><button className="button secondary" type="button" onClick={onCancel} disabled={busy}>取消</button><button className="button primary" disabled={busy}>{busy && <Spinner />}保存手动记录</button></div>
  </form>
}

export function ManualUsageSummary({ record, error }: { record?: ManualRecord | null; error?: string }) {
  if (!record && !error) return null
  const now = Date.now() / 1000
  const valid = [record?.fiveHour, record?.sevenDay].filter((window): window is UsageWindow => !!window && window.resetsAt > now)
  const recommendation = valid.some(window => window.usedPercentage >= 95) ? '手动记录达到 95%，建议核对官方额度并准备手动切换。' : valid.some(window => window.usedPercentage >= 90) ? '手动记录达到 90%，可以开始整理交接。' : ''
  return <section className="manual-usage-summary" aria-label="手动额度记录"><div className="manual-summary-heading"><NotebookPen size={14} /><strong>手动记录</strong><span>独立本地备注</span></div>{error ? <p className="form-error">手动记录暂不可用，请检查本地状态。</p> : record && <><div className="manual-usage-windows">{([{ title: '5 小时', window: record.fiveHour }, { title: '7 天', window: record.sevenDay }]).map(item => <div key={item.title}><span>{item.title}</span><strong>{item.window ? `${item.window.usedPercentage}%` : '未记录'}</strong>{item.window && <small>{item.window.resetsAt <= now ? '已过期 · ' : '重置于 '}{dateText(item.window.resetsAt)}</small>}</div>)}</div>{record.limitedUntil > 0 && <p className={`manual-limit ${record.limitedUntil <= now ? 'expired' : ''}`}>{record.limitedUntil > now ? '本地标记限额' : '限额标记已到期'} · {dateText(record.limitedUntil)}</p>}{recommendation && <p className="manual-recommendation"><Info size={14} />{recommendation}</p>}<p className="manual-observed">记录于 {new Date(record.observedAt).toLocaleString('zh-CN')} · 非官方查询结果</p></>}</section>
}
