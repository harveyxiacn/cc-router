"""Browser smoke test of the real UI with an explicit in-memory native bridge fixture.

Run against the frontend development server. It never accesses real accounts.
Requires Python playwright and its Chromium browser.
"""
import os
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

FIXTURE = r"""
(() => {
  const now = Date.now();
  const data = {
    accounts: [
      {id:'a'.repeat(32),name:'personal',label:'个人账号',isDefault:true,active:false,usage:{source:'official-statusline', observedAt:new Date(now).toISOString(),fiveHour:{usedPercentage:95,resetsAt:Math.floor(now/1000)+3600},sevenDay:{usedPercentage:32,resetsAt:Math.floor(now/1000)+86400}}},
      {id:'b'.repeat(32),name:'work',label:'工作账号',isDefault:false,active:false,usage:null}
    ], project:'', boundAccount:'',dataDir:'C:\\Fixture\\cc-router'
  };
  let handoff = {path:'C:\\Fixture\\project\\.cc-router\\handoff.md',content:'# Goal\nFinish the fixture task.\n',digest:'first'};
  window.fixture = {calls:[], conflictSave:false, conflictRead:false};
  const note = (name,...args) => window.fixture.calls.push({name,args});
  let bindings = {};
  const backups = [];
  const saveBackup = reason => {
    const info = {id:'fixture-backup-'+(backups.length+1),createdAt:new Date().toISOString(),reason,accountCount:data.accounts.length,bindingCount:Object.keys(bindings).length,restorable:true};
    backups.unshift({info,accounts:data.accounts.map(({id,name,label})=>({id,name,label})),defaultAccountId:data.accounts.find(a=>a.isDefault)?.id||'',projectBindings:structuredClone(bindings),digest:'fixture-digest-'+info.id});
    return structuredClone(info);
  };
  window.fixture.data = data;
  const updates = {currentVersion:'0.1.0-alpha.1',latestVersion:'0.1.0-alpha.1',releaseURL:'https://github.com/harveyxiacn/cc-router/releases',notes:'Fixture signed release notes',checkedAt:new Date(now).toISOString(),updateAvailable:false,prerelease:true,autoUpdate:false,phase:'upToDate',progress:0,message:'No newer signed release is available',error:'',canRollback:false,rollbackVersion:''};
  window.go = {main:{App:{
    GetSnapshot: async project => { note('GetSnapshot', project); return structuredClone({...data,project}); },
    ChooseDirectory: async () => 'C:\\Fixture\\project',
    CreateAccount: async (name,label) => { note('CreateAccount',name,label); data.accounts.push({id:'c'.repeat(32),name,label,isDefault:false,active:false,usage:null}); },
    RenameAccount: async (name,newName,label) => {const a=data.accounts.find(a=>a.name===name);a.name=newName;a.label=label;},
    RemoveAccount: async name => {data.accounts=data.accounts.filter(a=>a.name!==name);},
    SetDefault: async name => {data.accounts.forEach(a=>a.isDefault=a.name===name);},
    Bind: async (project,name) => {note('Bind',project,name);data.boundAccount=name;bindings[project]=data.accounts.find(a=>a.name===name).id;},
    Unbind: async project => {data.boundAccount='';delete bindings[project];},
    CheckAccount: async name => {note('CheckAccount',name);return {known:true,loggedIn:false,email:'',authMethod:'',version:'2.1.285'};},
    Diagnose: async () => ({version:'2.1.285',findings:[],error:''}),
    CreateHandoff: async () => structuredClone(handoff),
    ReadHandoff: async () => structuredClone({...handoff,digest:window.fixture.conflictRead?'external':handoff.digest}),
    SaveHandoff: async (project,content,digest) => {note('SaveHandoff',project,content,digest);if(window.fixture.conflictSave)throw new Error('handoff changed on disk; reload and merge your draft before saving');handoff={...handoff,content,digest:'saved'};return structuredClone(handoff);},
    Launch: async (...args) => {note('Launch',...args);},
    InstallUsage: async (...args) => {note('InstallUsage',...args);},
    RecordManualUsage: async (name,input) => {note('RecordManualUsage',name,input);data.accounts.find(a=>a.name===name).manualUsage={...structuredClone(input),source:'manual',observedAt:new Date().toISOString()};},
    ClearManualUsage: async name => {note('ClearManualUsage',name);data.accounts.find(a=>a.name===name).manualUsage=null;},
    ListBackups: async () => backups.map(b=>structuredClone(b.info)),
    CreateBackup: async () => {note('CreateBackup');return saveBackup('manual');},
    PreviewBackup: async id => {note('PreviewBackup',id);const backup=backups.find(b=>b.info.id===id);if(!backup)throw new Error('Fixture backup is missing');return structuredClone(backup);},
    RestoreBackup: async (id,digest,reviewed) => {note('RestoreBackup',id,digest,reviewed);const backup=backups.find(b=>b.info.id===id);if(!backup||backup.digest!==digest||!reviewed)throw new Error('Fixture backup must be reviewed');const before=saveBackup('pre-restore');data.accounts=backup.accounts.map(a=>({...data.accounts.find(current=>current.id===a.id),...a,isDefault:a.id===backup.defaultAccountId}));bindings=structuredClone(backup.projectBindings);data.boundAccount=data.accounts.find(a=>a.id===Object.values(bindings)[0])?.name||'';return before;},
    ExportMetadata: async () => '{}', ImportMetadata: async () => {},
    ExportMetadataFile: async () => '', ImportMetadataFile: async () => '',
    GetUpdateStatus: async () => structuredClone(updates),
    CheckForUpdates: async () => {note('CheckForUpdates');return structuredClone(updates);},
    SetAutoUpdate: async enabled => {note('SetAutoUpdate',enabled);updates.autoUpdate=enabled;},
    SetUpdateIdle: async idle => {note('SetUpdateIdle',idle);},
    PrepareUpdate: async () => {note('PrepareUpdate');}, ApplyUpdate: async () => {note('ApplyUpdate');},
    RollbackUpdate: async () => {note('RollbackUpdate');}, ReportReady: async () => {note('ReportReady');},
    OpenReleasePage: async () => {note('OpenReleasePage');}
  }}};
})();
"""

with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={"width": 1440, "height": 1000}, device_scale_factor=1)
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    page.add_init_script(FIXTURE)
    page.goto(os.getenv("CC_ROUTER_TEST_URL", "http://127.0.0.1:5173"))
    page.wait_for_load_state("networkidle")
    expect(page.get_by_role("heading", name="个人账号", exact=True)).to_be_visible()
    expect(page.get_by_text("95%", exact=True)).to_be_visible()
    expect(page.get_by_text("暂无上报", exact=True)).to_have_count(2)
    assert page.evaluate("fixture.calls.filter(c => c.name === 'Launch' || c.name === 'CheckAccount').length") == 0

    page.get_by_role("button", name="personal 更多操作", exact=True).click()
    page.get_by_role("button", name="手动记录额度 / 本地限额", exact=True).click()
    reset_time = page.evaluate("() => {const d=new Date(Date.now()+3600000);return new Date(d.getTime()-d.getTimezoneOffset()*60000).toISOString().slice(0,16)}")
    page.get_by_label("5 小时已用百分比", exact=True).fill("47")
    page.get_by_label("5 小时重置时间", exact=True).fill(reset_time)
    page.get_by_label("本地限额截止时间", exact=True).fill(reset_time)
    Path(".scratch").mkdir(exist_ok=True)
    page.screenshot(path=".scratch/gui-manual-form.png", full_page=True, animations="disabled")
    page.get_by_role("button", name="保存手动记录", exact=True).click()
    manual = page.get_by_role("region", name="手动额度记录", exact=True)
    expect(manual.get_by_text("47%", exact=True)).to_be_visible()
    expect(manual.get_by_text("独立本地备注", exact=True)).to_be_visible()
    expect(page.get_by_text("95%", exact=True)).to_be_visible()
    assert page.evaluate("fixture.data.accounts[0].usage.source") == "official-statusline"
    assert page.evaluate("fixture.data.accounts[0].usage.fiveHour.usedPercentage") == 95
    assert page.evaluate("fixture.data.accounts[0].manualUsage.source") == "manual"
    page.screenshot(path=".scratch/gui-manual.png", full_page=True, animations="disabled")

    page.get_by_role("button", name="选择项目文件夹", exact=True).click()
    expect(page.get_by_role("button", name="启动工作会话").first).to_be_enabled()
    Path(".scratch").mkdir(exist_ok=True)
    page.screenshot(path=".scratch/gui-accounts.png", full_page=True, animations="disabled")

    page.get_by_role("button", name="添加账号", exact=True).click()
    page.get_by_label("账号名称", exact=True).fill("studio")
    page.get_by_label("显示标签", exact=True).fill("独立项目")
    expect(page.get_by_role("checkbox", name="创建后打开官方登录")).to_be_checked()
    page.get_by_role("dialog").get_by_role("button", name="添加账号", exact=True).click()
    expect(page.get_by_role("heading", name="独立项目", exact=True)).to_be_visible()
    page.wait_for_function("fixture.calls.some(c=>c.name==='Launch' && c.args[0]==='studio' && c.args[2]==='login')")

    page.get_by_role("button", name="项目与绑定", exact=True).click()
    page.get_by_label("选择绑定账号", exact=True).select_option("work")
    page.get_by_role("button", name="绑定项目", exact=True).click()
    expect(page.get_by_text("当前：work", exact=True)).to_be_visible()

    page.get_by_role("button", name="环境诊断", exact=True).click()
    page.get_by_role("button", name="创建本地备份", exact=True).click()
    expect(page.get_by_role("button", name="预览备份 fixture-backup-1", exact=True)).to_be_visible()
    page.get_by_role("button", name="账号工作台", exact=True).click()
    page.get_by_role("button", name="work 更多操作", exact=True).click()
    page.get_by_role("button", name="设为默认", exact=True).click()
    page.wait_for_function("fixture.data.accounts.find(a=>a.name==='work').isDefault")
    page.get_by_role("button", name="环境诊断", exact=True).click()
    page.get_by_role("button", name="查看备份列表", exact=True).click()
    page.get_by_role("button", name="预览备份 fixture-backup-1", exact=True).click()
    backup_dialog = page.get_by_role("dialog")
    expect(backup_dialog.get_by_text("默认账号：个人账号 · personal", exact=True)).to_be_visible()
    project_path = page.evaluate("fixture.calls.find(c=>c.name==='Bind').args[0]")
    expect(backup_dialog.get_by_text(project_path, exact=True)).to_be_visible()
    restore = backup_dialog.get_by_role("button", name="恢复此备份", exact=True)
    expect(restore).to_be_disabled()
    assert page.evaluate("fixture.calls.filter(c=>c.name==='RestoreBackup').length") == 0
    page.screenshot(path=".scratch/gui-backups.png", full_page=True, animations="disabled")
    backup_dialog.get_by_role("checkbox", name="我已审阅账号与项目绑定，确认恢复本地配置", exact=True).check()
    restore.click()
    expect(backup_dialog).not_to_be_visible()
    page.wait_for_function("fixture.data.accounts.find(a=>a.name==='personal').isDefault")
    expect(page.get_by_role("button", name="审阅恢复前配置", exact=True)).to_be_visible()
    assert page.evaluate("fixture.calls.find(c=>c.name==='RestoreBackup').args") == ["fixture-backup-1", "fixture-digest-fixture-backup-1", True]
    assert page.evaluate("fixture.data.accounts[0].usage.fiveHour.usedPercentage") == 95
    assert page.evaluate("fixture.data.accounts[0].manualUsage.fiveHour.usedPercentage") == 47

    page.get_by_role("button", name="工作交接", exact=True).click()
    switch = page.get_by_role("button", name="确认切换并启动", exact=True)
    expect(switch).to_be_disabled()
    page.get_by_role("button", name="创建模板", exact=True).click()
    editor = page.get_by_label("交接内容", exact=True)
    expect(editor).to_be_enabled()
    editor.fill("# Reviewed handoff\nTests pass; continue from current Git state.\n")
    expect(switch).to_be_disabled()
    page.get_by_role("button", name="保存", exact=True).click()
    review = page.get_by_role("checkbox")
    expect(review).to_be_enabled()
    expect(switch).to_be_disabled()
    review.check()
    page.get_by_label("目标账号", exact=True).select_option("work")
    switch.click()
    page.wait_for_function("fixture.calls.some(c=>c.name==='Launch')")
    assert page.evaluate("fixture.calls.filter(c=>c.name==='Launch')") == [
        {"name":"Launch","args":["studio", "C:\\Fixture\\project", "login", False]},
        {"name":"Launch","args":["work", "C:\\Fixture\\project", "switch", True]},
    ]

    page.evaluate("fixture.conflictSave = true")
    editor.fill("Unsaved draft must survive conflict.")
    page.get_by_role("button", name="保存", exact=True).click()
    expect(page.get_by_text("handoff changed on disk; reload and merge your draft before saving", exact=True)).to_be_visible()
    expect(editor).to_have_value("Unsaved draft must survive conflict.")
    expect(switch).to_be_disabled()
    page.screenshot(path=".scratch/gui-handoff.png", full_page=True, animations="disabled")
    page.get_by_role("button", name="环境诊断", exact=True).click()
    expect(page.get_by_role("heading", name="更新与关于")).to_be_visible()
    page.get_by_role("button", name="检查更新", exact=True).click()
    page.wait_for_function("fixture.calls.some(c=>c.name==='CheckForUpdates')")
    page.get_by_role("button", name="官方发行页", exact=True).click()
    page.wait_for_function("fixture.calls.some(c=>c.name==='OpenReleasePage')")
    assert page.evaluate("fixture.calls.filter(c=>c.name==='CheckAccount' || c.name==='ApplyUpdate').length") == 0
    page.screenshot(path=".scratch/gui-updates.png", full_page=True, animations="disabled")
    assert not errors, errors
    browser.close()
    print("GUI smoke passed: truthful official/manual quota separation, first-login opt-in, project binding, reviewed local backup restore, review-gated switch, draft conflict preservation, update controls; no automatic identity check or account switch.")
