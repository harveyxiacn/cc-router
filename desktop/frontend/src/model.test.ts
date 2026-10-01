import { describe, expect, it } from 'vitest'
import { accountNameIsValid, quotaView, errorMessage } from './model'

describe('real usage display semantics', () => {
  const now = Date.parse('2026-10-01T12:00:00Z')
  it('keeps missing usage unknown', () => {
    expect(quotaView(null, 'fiveHour', now)).toMatchObject({ known: false, text: '暂无上报', tone: 'unknown' })
  })
  it('marks old reports and expired windows stale', () => {
    const usage = { observedAt: '2026-10-01T11:40:00Z', fiveHour: { usedPercentage: 93, resetsAt: now / 1000 + 3600 }, sevenDay: null, source: 'official-statusline' as const }
    expect(quotaView(usage, 'fiveHour', now)).toMatchObject({ known: true, stale: true, text: '93%', tone: 'stale' })
    expect(quotaView({ ...usage, observedAt: '2026-10-01T11:59:00Z', fiveHour: { ...usage.fiveHour, resetsAt: now / 1000 - 1 } }, 'fiveHour', now).stale).toBe(true)
  })
  it('only recommends switching for a fresh report at 95 percent', () => {
    const usage = { observedAt: '2026-10-01T11:59:00Z', fiveHour: { usedPercentage: 95, resetsAt: now / 1000 + 3600 }, sevenDay: null, source: 'official-statusline' as const }
    expect(quotaView(usage, 'fiveHour', now)).toMatchObject({ stale: false, tone: 'switch' })
    expect(quotaView({ ...usage, fiveHour: { ...usage.fiveHour, usedPercentage: 101 } }, 'fiveHour', now).known).toBe(false)
  })
})

describe('account and error input', () => {
  it('matches registry naming rules', () => {
    expect(accountNameIsValid('work-2')).toBe(true)
    for (const name of ['工作', '../escape', 'UPPER', 'space name', 'x'.repeat(65)]) expect(accountNameIsValid(name)).toBe(false)
  })
  it('preserves actionable backend errors without stringifying objects', () => {
    expect(errorMessage(new Error('保存冲突，请重新审阅'))).toBe('保存冲突，请重新审阅')
    expect(errorMessage('后端暂不可用')).toBe('后端暂不可用')
    expect(errorMessage({ token: 'secret' })).toBe('操作未完成，请重试。')
  })
})
