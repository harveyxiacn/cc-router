import { afterEach, describe, expect, it } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { AccountForm, QuotaMeter, HandoffReview } from './components'

afterEach(cleanup)

describe('account form', () => {
  it('rejects unsafe names and sends the entered name and label', () => {
    const submissions: string[][] = []
    render(<AccountForm busy={false} onSubmit={(name, label) => { submissions.push([name, label]) }} onCancel={() => {}} />)
    fireEvent.change(screen.getByLabelText('账号名称'), { target: { value: '../escape' } })
    fireEvent.change(screen.getByLabelText('显示标签'), { target: { value: '个人账户' } })
    fireEvent.click(screen.getByRole('button', { name: '添加账号' }))
    expect(submissions).toEqual([])
    fireEvent.change(screen.getByLabelText('账号名称'), { target: { value: 'personal' } })
    fireEvent.click(screen.getByRole('button', { name: '添加账号' }))
    expect(submissions).toEqual([['personal', '个人账户']])
  })
  it('offers official login by default and permits opting out', () => {
    const requests: boolean[] = []
    render(<AccountForm busy={false} onSubmit={(_name, _label, autoLogin) => { requests.push(autoLogin) }} onCancel={() => {}} />)
    const checkbox = screen.getByRole('checkbox', { name: '创建后打开官方登录' }) as HTMLInputElement
    expect(checkbox.checked).toBe(true)
    fireEvent.change(screen.getByLabelText('账号名称'), { target: { value: 'work' } })
    fireEvent.click(screen.getByRole('button', { name: '添加账号' }))
    expect(requests).toEqual([true])
    fireEvent.click(checkbox)
    fireEvent.click(screen.getByRole('button', { name: '添加账号' }))
    expect(requests).toEqual([true, false])
  })
})

describe('truthful quota and handoff states', () => {
  it('does not render a percentage or progressbar for unknown quota', () => {
    render(<QuotaMeter title="5 小时窗口" usage={null} window="fiveHour" />)
    expect(screen.getByText('暂无上报')).toBeTruthy()
    expect(screen.queryByRole('progressbar')).toBeNull()
  })
  it('prevents review until the current handoff is saved', () => {
    render(<HandoffReview saved={false} reviewed={false} onChange={() => {}} />)
    const checkbox = screen.getByRole('checkbox') as HTMLInputElement
    expect(checkbox.disabled).toBe(true)
    expect(screen.getByText('保存后可确认审阅')).toBeTruthy()
  })
})
