import type { UsageSnapshot } from './api'

export type QuotaTone = 'unknown' | 'good' | 'prepare' | 'switch' | 'stale'
export interface QuotaView { known: boolean; stale: boolean; text: string; tone: QuotaTone; percentage: number; resetText: string }

export function quotaView(usage: UsageSnapshot | null, window: 'fiveHour' | 'sevenDay', now = Date.now()): QuotaView {
  const unknown: QuotaView = { known: false, stale: false, text: '暂无上报', tone: 'unknown', percentage: 0, resetText: '在官方会话中启用额度状态栏' }
  const value = usage?.[window]
  const observed = Date.parse(usage?.observedAt ?? '')
  if (!usage || usage.source !== 'official-statusline' || !value || !Number.isFinite(value.usedPercentage) || value.usedPercentage < 0 || value.usedPercentage > 100 || !Number.isFinite(value.resetsAt) || value.resetsAt <= 0 || !Number.isFinite(observed) || observed > now + 60_000) return unknown
  const stale = now - observed > 15 * 60_000 || value.resetsAt * 1000 <= now
  const percentage = value.usedPercentage
  const reset = new Date(value.resetsAt * 1000)
  if (!Number.isFinite(reset.getTime())) return unknown
  return {
    known: true, stale, text: `${Number(percentage.toFixed(1))}%`, percentage,
    tone: stale ? 'stale' : percentage >= 95 ? 'switch' : percentage >= 90 ? 'prepare' : 'good',
    resetText: value.resetsAt * 1000 <= now ? '窗口已到期，请在官方 /usage 核实' : `重置于 ${reset.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })}`,
  }
}
export function accountNameIsValid(name: string) { return /^[a-z0-9][a-z0-9_-]{0,63}$/.test(name) }
export function errorMessage(error: unknown) { return error instanceof Error ? error.message : typeof error === 'string' ? error : '操作未完成，请重试。' }
export function observedText(usage: UsageSnapshot | null) {
  if (!usage || !Number.isFinite(Date.parse(usage.observedAt))) return '尚未收到官方上报'
  return `最近上报 ${new Date(usage.observedAt).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })}`
}
