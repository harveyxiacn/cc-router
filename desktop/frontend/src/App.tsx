import { useCallback, useEffect, useRef, useState } from 'react'
import { Activity, ArrowLeftRight, ArrowUpRight, BadgeCheck, Check, ChevronRight, CirclePlus, FolderOpen, Info, LogIn, MoreHorizontal, NotebookPen, Pencil, Play, RefreshCw, ShieldCheck, Star, Terminal, Trash2, X } from 'lucide-react'
import type { Account, Diagnosis, Handoff, Identity, Snapshot } from './api'
import { backend } from './api'
import { AccountForm, Dialog, HandoffReview, QuotaMeter, Spinner } from './components'
import { errorMessage, observedText, quotaView } from './model'
import { UpdatesPanel, useUpdates } from './updates'
import { ManualUsageForm, ManualUsageSummary } from './manual'
import { BackupsPanel } from './backups'

type Page = 'accounts' | 'project' | 'handoff' | 'diagnostics'
type AccountDialog = { type: 'create' } | { type: 'rename' | 'remove' | 'usage' | 'manual' | 'clear-manual'; account: Account }
const pages = [
  { id: 'accounts' as const, title: '账号工作台', english: 'ACCOUNTS', icon: Activity },
  { id: 'project' as const, title: '项目与绑定', english: 'PROJECT', icon: FolderOpen },
  { id: 'handoff' as const, title: '工作交接', english: 'HANDOFF', icon: NotebookPen },
  { id: 'diagnostics' as const, title: '环境诊断', english: 'DIAGNOSTICS', icon: ShieldCheck },
]

export default function App() {
  const [page, setPage] = useState<Page>('accounts')
  const [project, setProject] = useState('')
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [connectionError, setConnectionError] = useState('')
  const [actionError, setActionError] = useState('')
  const [busy, setBusy] = useState('')
  const [toast, setToast] = useState('')
  const [dialog, setDialog] = useState<AccountDialog | null>(null)
  const [backupDialog, setBackupDialog] = useState(false)
  const [menu, setMenu] = useState('')
  const [identity, setIdentity] = useState<{ name: string; result: Identity } | null>(null)
  const [target, setTarget] = useState('')
  const [diagnosticAccount, setDiagnosticAccount] = useState('')
  const [diagnosis, setDiagnosis] = useState<Diagnosis | null>(null)
  const [handoff, setHandoff] = useState<Handoff | null>(null)
  const [draft, setDraft] = useState('')
  const [reviewed, setReviewed] = useState(false)
  const request = useRef(0)
  const currentProject = useRef(project)
  currentProject.current = project
  const operating = useRef(false)
  const accounts = snapshot?.accounts ?? []
  const saved = !!handoff && draft === handoff.content
  const dirty = !!handoff && !saved
  const updateBlocked = dirty || !!dialog || !!identity || !!busy || !!menu || backupDialog
  const updates = useUpdates(updateBlocked, !!snapshot && !connectionError)
  const restarting = updates.busy === 'apply' || updates.busy === 'rollback'
  const targetName = accounts.some(account => account.name === target) ? target : snapshot?.boundAccount || accounts.find(account => account.isDefault)?.name || accounts[0]?.name || ''
  const currentPage = pages.find(item => item.id === page)!

  const refresh = useCallback(async () => {
    const sequence = ++request.current
    try {
      const result = await backend().GetSnapshot(project)
      if (sequence === request.current && project === currentProject.current) { setSnapshot(result); setConnectionError('') }
    } catch (error) { if (sequence === request.current && project === currentProject.current) setConnectionError(errorMessage(error)) }
  }, [project])
  useEffect(() => {
    void refresh()
    const interval = window.setInterval(() => { void refresh() }, 5000)
    return () => { window.clearInterval(interval); request.current++ }
  }, [refresh])
  useEffect(() => {
    if (!toast) return
    const timeout = window.setTimeout(() => setToast(''), 4500)
    return () => window.clearTimeout(timeout)
  }, [toast])
  useEffect(() => {
    if (!menu) return
    function close(event: MouseEvent) { if (!(event.target as HTMLElement).closest('.menu-area')) setMenu('') }
    window.addEventListener('click', close)
    return () => window.removeEventListener('click', close)
  }, [menu])

  async function perform(key: string, action: () => Promise<void>, message = '', refreshAfter = true) {
    if (operating.current) return
    operating.current = true; setBusy(key); setActionError('')
    try { await action(); if (refreshAfter) await refresh(); if (message) setToast(message) }
    catch (error) { setActionError(errorMessage(error)) }
    finally { operating.current = false; setBusy('') }
  }
  async function chooseProject() {
    await perform('choose', async () => {
      if (dirty) throw new Error('交接草稿尚未保存。请先保存，或读取原文件以放弃草稿，再选择其他项目。')
      const selected = await backend().ChooseDirectory()
      if (selected && selected !== project) { currentProject.current = selected; request.current++; setSnapshot(null); setProject(selected); setHandoff(null); setDraft(''); setReviewed(false); setDiagnosis(null); setIdentity(null) }
    }, '', false)
  }
  function requireProject() { if (!project) throw new Error('请先选择一个真实项目文件夹。') }
  async function launch(account: Account, mode: 'run' | 'login') {
    await perform(`${mode}-${account.name}`, async () => { if (mode === 'run') requireProject(); await backend().Launch(account.name, project, mode, false) }, mode === 'login' ? '已打开官方登录终端，请完成浏览器授权并查看终端提示。' : '已在系统终端中启动工作会话。')
  }
  async function checkIdentity(name: string) {
    await perform(`identity-${name}`, async () => { requireProject(); const result = await backend().CheckAccount(name, project); setIdentity({ name, result }) })
  }
  async function loadHandoff(create: boolean) {
    await perform('handoff-read', async () => { requireProject(); const result = create ? await backend().CreateHandoff(project) : await backend().ReadHandoff(project); setHandoff(result); setDraft(result.content); setReviewed(false) }, create ? '交接文件已准备好，请补充实际工作内容。' : '')
  }
  async function saveHandoff() {
    await perform('handoff-save', async () => {
      if (!handoff) throw new Error('请先创建或读取交接文件。')
      const result = await backend().SaveHandoff(project, draft, handoff.digest)
      setHandoff(result); setDraft(result.content); setReviewed(false)
    }, '交接已保存。请重新审阅后确认切换。')
  }
  async function switchAccount() {
    await perform('switch', async () => {
      requireProject()
      if (!handoff || !saved || !reviewed || !targetName) throw new Error('请先保存、审阅交接内容，并选择目标账号。')
      const current = await backend().ReadHandoff(project)
      if (current.digest !== handoff.digest) { setReviewed(false); throw new Error('磁盘上的交接内容已变化。请重新读取、审阅后再切换；当前草稿已保留。') }
      await backend().Launch(targetName, project, 'switch', true)
    }, '已打开新账号会话，请让 Claude 阅读交接文件并核实工作区。')
  }

  return <div className="app-shell">
    <aside className="sidebar" inert={restarting}>
      <div className="brand"><span className="brand-mark" aria-hidden="true"><ArrowLeftRight size={21} strokeWidth={1.7} /></span><div><strong>CC Router</strong><small>KEEP YOUR FLOW.</small></div></div>
      <div className="sidebar-section-label">工作空间 <span>WORKSPACE</span></div>
      <nav aria-label="主导航">{pages.map(item => <button key={item.id} className={`nav-item ${page === item.id ? 'active' : ''}`} onClick={() => setPage(item.id)}><item.icon size={19} strokeWidth={1.6} /><span>{item.title}</span>{page === item.id && <span className="nav-dot" />}</button>)}</nav>
      <div className="sidebar-note"><span className="tiny-rule" /><NotebookPen size={23} strokeWidth={1.5} /><p>切换账号，<br />把工作交接清楚。</p><span>整理目标与验证结果，<br />让下一段会话接得上。</span><button onClick={() => setPage('handoff')}>去写交接 <ChevronRight size={13} /></button></div>
      <footer className="sidebar-footer"><span className="local-dot" /><span>本地保存 · 独立配置</span><small>{updates.status?.currentVersion || '版本读取中'} <span>CC ROUTER</span></small></footer>
    </aside>
    <main className="workspace" inert={restarting}>
      <header className="topbar"><div className="breadcrumb">工作空间 <ChevronRight size={12} /><strong>{currentPage.title}</strong></div><button className="project-chip" onClick={() => { void chooseProject() }} disabled={!!busy} title={project || '选择项目文件夹'}><FolderOpen size={14} /><span>{project ? project.split(/[\\/]/).filter(Boolean).at(-1) : '选择项目文件夹'}</span><ChevronRight size={13} /></button></header>
      <div className="workspace-body">
        <div className="page-heading"><div><span className="eyebrow">YOUR WORKSPACE / {currentPage.english}</span><h1>{currentPage.title}<span className="heading-period">.</span></h1><p className="page-description">{page === 'accounts' ? '分开管理账号与额度，让每一段工作都有清楚的起点。' : page === 'project' ? '把项目交给合适的账号；绑定决定之后的启动。' : page === 'handoff' ? '记录已经做过的事，让新会话从可靠的上下文开始。' : '明确检查环境与身份。账号标签不代表实际登录身份。'}</p></div><div className="heading-actions">{page === 'accounts' && <button className="button primary" disabled={!!busy || !snapshot} onClick={() => setDialog({ type: 'create' })}><CirclePlus size={16} />添加账号</button>}<button className="button icon-only secondary" aria-label="刷新本地快照" title="刷新本地快照" onClick={() => { void refresh() }}><RefreshCw size={16} /></button></div></div>
        {connectionError && <div className="notice error" role="alert"><Info size={18} /><div><strong>暂时无法读取本地工作台</strong><p>{connectionError}</p>{snapshot && <small>下方保留的是上次读取的快照，请恢复连接后再操作。</small>}</div><button className="text-button" onClick={() => { void refresh() }}>重试</button><button className="text-button" onClick={() => setPage('diagnostics')}>打开本地备份</button></div>}
        {actionError && !dialog && !backupDialog && <div className="notice error" role="alert"><Info size={18} /><div><strong>操作未完成</strong><p>{actionError}</p></div><button className="icon-button" aria-label="关闭错误提示" onClick={() => setActionError('')}><X size={15} /></button></div>}

        {page === 'accounts' && <>
          <div className="overview-strip"><div><span className="summary-label">已登记账号</span><strong>{snapshot ? accounts.length.toString().padStart(2, '0') : '—'}</strong></div><div><span className="summary-label">正在使用</span><strong>{snapshot ? accounts.filter(account => account.active).length.toString().padStart(2, '0') : '—'}</strong></div><div className="overview-project"><span className="summary-label">当前项目账号</span><strong>{snapshot?.boundAccount || (project ? '未绑定' : '未选择项目')}</strong></div><p><span className="local-dot" />仅刷新本地上报<br /><small>身份检查由你手动触发</small></p></div>
          <div className="section-heading"><h2>你的账号 <span>{snapshot ? accounts.length : '—'}</span></h2><span>额度来自官方状态栏的最近上报</span></div>
          {snapshot && accounts.length > 0 ? <div className="account-grid">{accounts.map((account, index) => {
            const views = [quotaView(account.usage, 'fiveHour'), quotaView(account.usage, 'sevenDay')]
            const warning = views.some(view => view.tone === 'switch') ? 'switch' : views.some(view => view.tone === 'prepare') ? 'prepare' : ''
            return <article className={`account-card ${account.isDefault ? 'default-card' : ''}`} key={account.id}>
              <div className="account-card-top"><span className={`account-avatar avatar-${index % 3}`}>{(account.label || account.name).slice(0, 1)}</span><div className="account-title"><h3>{account.label || account.name}</h3><span>{account.name}</span></div><div className="account-badges">{account.active && <span className="badge active-badge">使用中</span>}{account.isDefault && <span className="badge default-badge">默认</span>}</div><div className="menu-area"><button className="icon-button" aria-label={`${account.name} 更多操作`} aria-expanded={menu === account.id} onClick={() => setMenu(menu === account.id ? '' : account.id)}><MoreHorizontal size={20} /></button>{menu === account.id && <div className="account-menu">{!account.isDefault && <button disabled={!!busy} onClick={() => { setMenu(''); void perform('default', async () => { await backend().SetDefault(account.name) }, '默认账号已更新，仅影响之后的启动。') }}><Star size={14} />设为默认</button>}<button disabled={!!busy || account.active} onClick={() => { setMenu(''); setDialog({ type: 'rename', account }) }}><Pencil size={14} />编辑名称与标签</button><button disabled={!!busy || !project} onClick={() => { setMenu(''); void checkIdentity(account.name) }}><BadgeCheck size={14} />检查官方身份</button><button disabled={!!busy || account.active} onClick={() => { setMenu(''); setDialog({ type: 'usage', account }) }}><Activity size={14} />安装额度状态栏</button><button disabled={!!busy} onClick={() => { setMenu(''); setDialog({ type: 'manual', account }) }}><NotebookPen size={14} />手动记录额度 / 本地限额</button>{(account.manualUsage || account.manualUsageError) && <button disabled={!!busy} onClick={() => { setMenu(''); setDialog({ type: 'clear-manual', account }) }}><Trash2 size={14} />清除手动记录</button>}<button className="danger-text" disabled={!!busy || account.active} onClick={() => { setMenu(''); setDialog({ type: 'remove', account }) }}><Trash2 size={14} />移除账号登记</button></div>}</div></div>
              <div className="official-source-label">官方状态栏 · 最近上报</div><div className="account-card-quotas"><QuotaMeter title="5 小时窗口" usage={account.usage} window="fiveHour" /><QuotaMeter title="7 天窗口" usage={account.usage} window="sevenDay" /></div>
              {warning && <div className={`quota-warning ${warning}`}><Info size={13} />{warning === 'switch' ? '额度接近上限，建议保存工作并准备切换。' : '已用额度达到 90%，可以开始整理交接。'}<button onClick={() => { setPage('handoff') }}>写交接 <ArrowUpRight size={12} /></button></div>}
              <ManualUsageSummary record={account.manualUsage} error={account.manualUsageError} /><div className="account-observed"><span className="report-dot" />{account.usageError ? '本地上报暂不可用，请检查本地状态' : observedText(account.usage)}<span className="account-number">{(index + 1).toString().padStart(2, '0')}</span></div>
              <div className="account-card-actions"><button className="button start-button" disabled={!!busy || account.active || !project} onClick={() => { void launch(account, 'run') }}>{busy === `run-${account.name}` ? <Spinner /> : <Play size={14} />}启动工作会话<ArrowUpRight size={15} /></button><button className="button login-button" disabled={!!busy || account.active} onClick={() => { void launch(account, 'login') }} title="在官方终端中登录 claude.ai，无需先选择项目"><LogIn size={15} />登录</button></div>
            </article>
          })}<button className="add-account-card" disabled={!!busy} onClick={() => setDialog({ type: 'create' })}><span><CirclePlus size={24} strokeWidth={1.3} /></span><strong>添加另一个账号</strong><small>为不同的工作保留独立配置</small></button></div> : snapshot ? <div className="empty-state"><div className="empty-symbol"><ArrowLeftRight size={32} strokeWidth={1.2} /></div><span className="eyebrow">A FRESH START</span><h2>从第一个账号开始。</h2><p>添加账号后可直接打开官方登录。<br />完成浏览器授权，再选择项目继续工作。</p><ol className="first-use-steps"><li><span>01</span>添加本地账号</li><li><span>02</span>完成官方授权</li><li><span>03</span>选择项目并启动</li></ol><button className="button primary" onClick={() => setDialog({ type: 'create' })}><CirclePlus size={16} />添加第一个账号</button></div> : !connectionError && <div className="loading-state"><Spinner />正在读取本地登记…</div>}
          {!project && snapshot && accounts.length > 0 && <div className="notice subtle"><FolderOpen size={18} /><div><strong>为下一段工作选择项目</strong><p>账号可直接登录；启动工作、项目绑定与交接使用你选定的文件夹。</p></div><button className="button secondary" onClick={() => { void chooseProject() }} disabled={!!busy}>选择文件夹<ArrowUpRight size={14} /></button></div>}
          <div className="workspace-footnote"><ShieldCheck size={14} />账号标签是本地备注；身份、计费与实际额度请以官方 /status、/usage 为准。</div>
        </>}

        {page === 'project' && <section className="surface project-surface"><div className="surface-heading"><span className="surface-icon"><FolderOpen size={22} /></span><div><h2>当前工作文件夹</h2><p>绑定只影响之后的启动，不会改变正在运行的会话。</p></div></div><div className={`folder-selection ${project ? 'selected' : ''}`}><span className="eyebrow">PROJECT DIRECTORY</span><strong>{project || '还没有选择项目'}</strong><p>{project ? '选择规则：明确指定的账号 → 项目绑定 → 全局默认账号。' : '通过系统文件夹选择器指定工作目录。'}</p><button className="button secondary" disabled={!!busy} onClick={() => { void chooseProject() }}><FolderOpen size={15} />{project ? '更换文件夹' : '选择文件夹'}</button></div><div className="binding-row"><div><h3>项目账号绑定</h3><p>当前：{snapshot?.boundAccount || '未绑定，使用全局默认选择'}</p></div><label className="sr-only" htmlFor="binding-account">选择绑定账号</label><select id="binding-account" value={targetName} onChange={event => setTarget(event.target.value)} disabled={!!busy || accounts.length === 0}>{accounts.length === 0 && <option value="">先添加账号</option>}{accounts.map(account => <option key={account.id} value={account.name}>{account.label} · {account.name}</option>)}</select><button className="button primary" disabled={!!busy || !project || !targetName} onClick={() => { void perform('bind', async () => { await backend().Bind(project, targetName) }, '项目已绑定到所选账号。') }}>绑定项目</button>{snapshot?.boundAccount && <button className="text-button" disabled={!!busy} onClick={() => { void perform('unbind', async () => { await backend().Unbind(project) }, '项目绑定已解除。') }}>解除绑定</button>}</div><div className="local-data-note"><ShieldCheck size={17} /><div><strong>本地配置目录</strong><code>{snapshot?.dataDir || '等待连接'}</code><p>账号移除只删除登记，官方登录数据与会话历史会保留在设备上。</p></div></div></section>}

        {page === 'handoff' && <div className="handoff-layout"><section className="surface editor-surface"><div className="editor-top"><div><span className="eyebrow">LOCAL HANDOFF</span><h2>交接笔记</h2></div><div className="editor-actions"><button className="button secondary" disabled={!!busy || !project} onClick={() => { void loadHandoff(false) }}>读取{dirty && '（放弃草稿）'}</button><button className="button secondary" disabled={!!busy || !project || dirty} onClick={() => { void loadHandoff(true) }}>创建模板</button><button className="button primary" disabled={!!busy || !handoff || !dirty} onClick={() => { void saveHandoff() }}>{busy === 'handoff-save' && <Spinner />}保存</button></div></div><div className="editor-file"><NotebookPen size={14} /><span title={handoff?.path}>{handoff?.path || '.cc-router/handoff.md'}</span><span className={`save-state ${dirty ? 'unsaved' : ''}`}>{dirty ? '未保存' : handoff ? '已保存' : '未读取'}</span></div><label className="sr-only" htmlFor="handoff-content">交接内容</label><textarea id="handoff-content" value={draft} onChange={event => { setDraft(event.target.value); setReviewed(false) }} disabled={!!busy || !handoff} placeholder={project ? '创建模板，或读取已存在的交接文件。写下目标、已完成改动、实际验证结果与下一步。' : '先选择项目文件夹，再准备交接笔记。'} spellCheck={false} /><div className="editor-bottom"><span>Markdown · 仅本地保存</span><span>{draft.length.toLocaleString('zh-CN')} 字符</span></div></section><aside className="handoff-side"><div className="handoff-tip"><span className="eyebrow">PASS THE CONTEXT</span><h3>把下一步，<br />写得清楚一点。</h3><p>交接文件不会迁移聊天记录，也不会恢复另一个账号的会话。</p><ul><li><Check size={13} />目标与验收条件</li><li><Check size={13} />改动、检查及真实结果</li><li><Check size={13} />待解决的问题与下一步</li></ul></div><section className="surface switch-panel"><h3>在新账号下继续</h3><label htmlFor="switch-account">目标账号</label><select id="switch-account" value={targetName} onChange={event => setTarget(event.target.value)} disabled={!!busy || accounts.length === 0}>{accounts.length === 0 && <option value="">先添加账号</option>}{accounts.map(account => <option key={account.id} value={account.name}>{account.label} · {account.name}</option>)}</select><HandoffReview saved={saved && !busy} reviewed={reviewed} onChange={setReviewed} /><button className="button primary full-width" disabled={!!busy || !project || !targetName || !saved || !reviewed || accounts.find(account => account.name === targetName)?.active} onClick={() => { void switchAccount() }}>{busy === 'switch' ? <Spinner /> : <ArrowLeftRight size={15} />}确认切换并启动</button><p className="field-hint">先结束当前受管项目会话；新会话启动后，请让 Claude 阅读交接并核实工作区。</p></section></aside></div>}

        {page === 'diagnostics' && <section className="surface diagnostic-surface"><div className="surface-heading"><span className="surface-icon"><ShieldCheck size={22} /></span><div><h2>一次明确的环境检查</h2><p>检查变量、项目设置与管理策略；诊断仅显示来源和字段名。</p></div></div><div className="diagnostic-controls"><div><label htmlFor="diagnostic-account">检查范围</label><select id="diagnostic-account" value={diagnosticAccount} onChange={event => { setDiagnosticAccount(event.target.value); setDiagnosis(null) }} disabled={!!busy}><option value="">环境与项目（不检查账号）</option>{accounts.map(account => <option key={account.id} value={account.name}>{account.label} · {account.name}</option>)}</select></div><button className="button primary" disabled={!!busy || !project} onClick={() => { void perform('diagnose', async () => { requireProject(); setDiagnosis(await backend().Diagnose(diagnosticAccount, project)) }) }}>{busy === 'diagnose' ? <Spinner /> : <ShieldCheck size={15} />}运行诊断</button><button className="button secondary" disabled={!!busy || !project || !diagnosticAccount} onClick={() => { void checkIdentity(diagnosticAccount) }}><BadgeCheck size={15} />检查官方身份</button></div>{!project && <p className="form-error">请先选择项目文件夹。</p>}{diagnosis ? <div className="diagnosis-results"><div className="diagnosis-summary"><span>Claude Code</span><strong>{diagnosis.version || '版本未知'}</strong><span className={`badge ${diagnosis.findings.some(item => item.blocking) || diagnosis.error ? 'blocked-badge' : 'default-badge'}`}>{diagnosis.findings.some(item => item.blocking) || diagnosis.error ? '需要处理' : '检查完成'}</span></div>{diagnosis.error && <div className="notice error"><Info size={16} /><p>{diagnosis.error}</p></div>}{diagnosis.findings.length ? <div className="findings-list">{diagnosis.findings.map((finding, index) => <div className={`finding ${finding.blocking ? 'blocking' : ''}`} key={`${finding.source}-${finding.key}-${index}`}><span className="finding-mark">{finding.blocking ? '!' : 'i'}</span><div><strong>{finding.key}</strong><small>{finding.source}</small><p>{finding.message}</p></div><span className="finding-level">{finding.blocking ? '阻止启动' : '提示'}</span></div>)}</div> : !diagnosis.error && <div className="diagnosis-clear"><Check size={18} />在已检查来源中，未发现已知身份冲突。</div>}</div> : <div className="diagnostic-empty"><Terminal size={28} strokeWidth={1.2} /><p>检查按需进行，不会后台循环查询登录状态。</p><span>额度刷新只读取最近的官方本地上报。</span></div>}<div className="workspace-footnote"><Info size={14} />检查只反映当前设置。运行期间设置可能变化，最终以官方 /status 为准。</div></section>}
        {page === 'diagnostics' && <section className="surface metadata-panel"><div><h2>账号元数据</h2><p>只包含名称、标签与默认选择；不含凭据、历史和项目路径。</p><small>导入按名称合并。新账号需要在这台设备上重新登录。</small></div><div className="metadata-actions"><button className="button secondary" disabled={!!busy || !snapshot} onClick={() => { void perform('export', async () => { const path = await backend().ExportMetadata(); if (path) setToast(`已导出账号元数据到 ${path}`) }, '', false) }}>导出元数据</button><button className="button secondary" disabled={!!busy || !snapshot} onClick={() => { void perform('import', async () => { const path = await backend().ImportMetadata(); if (path) setToast('账号元数据已导入；请为新增账号完成官方登录。') }) }}>导入元数据</button></div></section>}
        {page === 'diagnostics' && <BackupsPanel busy={!!busy} dirty={dirty} unavailable={!snapshot} actionError={actionError} run={perform} onDialogChange={setBackupDialog} onRestored={() => { setDiagnosis(null); setIdentity(null); setTarget(''); setDiagnosticAccount('') }} />}{page === 'diagnostics' && <UpdatesPanel updates={updates} blocked={updateBlocked}/>}
      </div>
    </main>
    {restarting && <div className="restart-overlay" role="alert"><div><Spinner/><h2>正在安全重启工作台</h2><p>更新只替换应用文件，请等待新版本打开。</p></div></div>}
    {toast && <div className="toast" role="status"><span><Check size={15} /></span>{toast}</div>}
    {dialog && <Dialog title={dialog.type === 'create' ? '添加一个账号' : dialog.type === 'rename' ? '编辑账号' : dialog.type === 'remove' ? '移除账号登记' : dialog.type === 'manual' ? '手动记录额度与本地限额' : dialog.type === 'clear-manual' ? '清除手动记录' : '安装官方额度状态栏'} description={dialog.type === 'create' ? '先建立本地账号分组，再通过官方终端登录。' : dialog.type === 'rename' ? '修改名称与显示标签，保留登录配置和会话历史。' : dialog.type === 'remove' ? '官方登录数据和会话历史会保留在设备上。' : dialog.type === 'manual' ? `${dialog.account.label} · ${dialog.account.name} 的本地备注，可在官方 /usage 核对后填写。` : dialog.type === 'clear-manual' ? '只移除此账号的手动额度与限额备注，保留官方状态栏上报。' : '将官方 5 小时 / 7 天额度的最近上报显示在工作台。'} busy={!!busy} onClose={() => setDialog(null)}>
      {actionError && <p className="form-error" role="alert">{actionError}</p>}
      {(dialog.type === 'create' || dialog.type === 'rename') && <AccountForm account={dialog.type === 'rename' ? dialog.account : undefined} busy={!!busy} onCancel={() => setDialog(null)} onSubmit={(name, label, autoLogin) => { void perform(dialog.type, async () => {
        if (dialog.type === 'rename') { await backend().RenameAccount(dialog.account.name, name, label); setDialog(null); setToast('账号信息已更新。'); return }
        await backend().CreateAccount(name, label); setDialog(null)
        if (autoLogin) { try { await backend().Launch(name, project, 'login', false); setToast('账号已创建，官方登录终端已打开；请完成浏览器授权。') } catch (error) { await refresh(); throw new Error(`账号 ${name} 已创建，但官方登录终端未能打开：${errorMessage(error)}。请点击账号卡片的“登录”重试。`) } }
        else setToast('账号已添加，可随时点击“登录”打开官方授权流程。')
      }) }} />}
      {dialog.type === 'remove' && <><div className="remove-account-summary"><span className="account-avatar">{dialog.account.label.slice(0, 1)}</span><div><strong>{dialog.account.label}</strong><code>{dialog.account.name}</code></div></div><p className="muted">默认选择和指向此账号的项目绑定会一并清理。之后重新添加同名账号，会建立新的配置分组。</p><div className="dialog-actions"><button className="button secondary" disabled={!!busy} onClick={() => setDialog(null)}>取消</button><button className="button danger" disabled={!!busy} onClick={() => { void perform('remove', async () => { await backend().RemoveAccount(dialog.account.name); setDialog(null) }, '账号登记已移除，官方数据已保留。') }}>{busy && <Spinner />}移除登记</button></div></>}
      {dialog.type === 'manual' && <ManualUsageForm record={dialog.account.manualUsage} busy={!!busy} onCancel={() => setDialog(null)} onSubmit={input => { void perform('manual-record', async () => { await backend().RecordManualUsage(dialog.account.name, input); setDialog(null) }, '手动记录已保存，官方状态栏保持独立。') }} />}{dialog.type === 'clear-manual' && <><p className="muted">{dialog.account.label} · {dialog.account.name} 的手动记录将被清除；之后可重新记录。</p><div className="dialog-actions"><button className="button secondary" disabled={!!busy} onClick={() => setDialog(null)}>取消</button><button className="button primary" disabled={!!busy} onClick={() => { void perform('manual-clear', async () => { await backend().ClearManualUsage(dialog.account.name); setDialog(null) }, '手动记录已清除。') }}>{busy && <Spinner />}确认清除</button></div></>}{dialog.type === 'usage' && <><div className="usage-install-preview"><Activity size={20} /><div><strong>90% 提醒整理交接</strong><span>95% 建议手动切换</span></div><span className="badge default-badge">本地上报</span></div><p className="muted">在该账号的官方会话中启用；重启之后的新会话会开始上报。没有上报前，额度保持未知。</p><div className="notice subtle"><Info size={16} /><p>已有状态栏配置时，应用会拒绝覆盖，请在官方设置中手动整合。</p></div><div className="dialog-actions"><button className="button secondary" disabled={!!busy} onClick={() => setDialog(null)}>取消</button><button className="button primary" disabled={!!busy} onClick={() => { void perform('install-usage', async () => { await backend().InstallUsage(dialog.account.name, 90, 95); setDialog(null) }, '额度状态栏已安装；请启动新的官方工作会话。') }}>{busy && <Spinner />}安装到此账号</button></div></>}
    </Dialog>}
    {identity && <Dialog title="官方身份检查" description={`${identity.name} 的一次官方查询结果，标签不是登录身份。`} onClose={() => setIdentity(null)}><dl className="identity-details"><div><dt>登录状态</dt><dd>{!identity.result.known ? '未知，请在官方 /status 核实' : identity.result.loggedIn ? '已登录' : '未登录'}</dd></div><div><dt>邮箱</dt><dd>{identity.result.email || '未知'}</dd></div><div><dt>认证方式</dt><dd>{identity.result.authMethod || '未知'}</dd></div><div><dt>Claude 版本</dt><dd>{identity.result.version || '未知'}</dd></div></dl><div className="dialog-actions"><button className="button primary" onClick={() => setIdentity(null)}>知道了</button></div></Dialog>}
  </div>
}
