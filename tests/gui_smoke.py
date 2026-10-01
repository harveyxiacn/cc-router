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
  const updates = {currentVersion:'0.1.0-alpha.1',latestVersion:'0.1.0-alpha.1',releaseURL:'https://github.com/harveyxiacn/cc-router/releases',notes:'Fixture signed release notes',checkedAt:new Date(now).toISOString(),updateAvailable:false,prerelease:true,autoUpdate:false,phase:'upToDate',progress:0,message:'No newer signed release is available',error:'',canRollback:false,rollbackVersion:''};
  window.go = {main:{App:{
    GetSnapshot: async project => { note('GetSnapshot', project); return structuredClone({...data,project}); },
    ChooseDirectory: async () => 'C:\\Fixture\\project',
    CreateAccount: async (name,label) => { note('CreateAccount',name,label); data.accounts.push({id:'c'.repeat(32),name,label,isDefault:false,active:false,usage:null}); },
    RenameAccount: async (name,newName,label) => {const a=data.accounts.find(a=>a.name===name);a.name=newName;a.label=label;},
    RemoveAccount: async name => {data.accounts=data.accounts.filter(a=>a.name!==name);},
    SetDefault: async name => {data.accounts.forEach(a=>a.isDefault=a.name===name);},
    Bind: async (project,name) => {note('Bind',project,name);data.boundAccount=name;},
    Unbind: async () => {data.boundAccount='';},
    CheckAccount: async name => {note('CheckAccount',name);return {known:true,loggedIn:false,email:'',authMethod:'',version:'2.1.285'};},
    Diagnose: async () => ({version:'2.1.285',findings:[],error:''}),
    CreateHandoff: async () => structuredClone(handoff),
    ReadHandoff: async () => structuredClone({...handoff,digest:window.fixture.conflictRead?'external':handoff.digest}),
    SaveHandoff: async (project,content,digest) => {note('SaveHandoff',project,content,digest);if(window.fixture.conflictSave)throw new Error('handoff changed on disk; reload and merge your draft before saving');handoff={...handoff,content,digest:'saved'};return structuredClone(handoff);},
    Launch: async (...args) => {note('Launch',...args);},
    InstallUsage: async (...args) => {note('InstallUsage',...args);},
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
    print("GUI smoke passed: truthful quota, explicit first-login opt-in, project binding, review-gated switch, draft conflict preservation, update controls; no automatic identity check or account switch.")
