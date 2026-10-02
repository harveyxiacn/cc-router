# CC Router

本地 Claude Code 多账号启动器与项目交接工具，提供跨平台桌面 GUI 和独立 CLI。Go 核心搭配 Wails 2 + React 界面，使用未修改的官方 Claude Code 登录和运行。账号选择明确、官方配置独立、项目进度可交接。

**当前源码版本为 0.2.0-beta.1。** 已发布版本以 [Releases](https://github.com/harveyxiacn/cc-router/releases) 为准。自动化测试和可编译平台不等于真实账号验收；[兼容性记录](docs/compatibility.md) 列出验证边界。项目与 Anthropic 无隶属关系。

## 安装

先按[官方说明](https://code.claude.com/docs/en/setup)安装 Claude Code（本项目要求 >=2.1.268）。从本仓库 [Releases](https://github.com/harveyxiacn/cc-router/releases) 下载对应平台安装包或便携压缩包，并核对 SHA256SUMS。仅使用 CLI 时，解压后只把 `cc-router`（Windows 为 `cc-router.exe`）复制到 PATH 中的目录；Linux/macOS 先执行 `chmod +x cc-router`。已发布的 alpha.1 CLI 包还带有可选兼容别名 `ccr`，请勿一并复制或将整个解压目录加入 PATH，以免启用冲突命令。OTA 更新清单使用 Ed25519 签名；本版尚无 Windows Authenticode 或 macOS Developer ID 签名/公证，这两类签名的用途不同。

0.2 的桌面构建另外生成 Windows `-setup.exe` 用户安装包与 macOS `.dmg`。Windows 安装器包含 GUI 和配套 CLI、快捷方式与卸载入口；macOS 按 Intel/Apple Silicon 选择 DMG 后复制应用。ZIP/tar.gz 保留供便携安装与 OTA 使用，安装包不替代更新清单的签名验证。系统代码签名是独立选项，未签名版本可能显示系统警告。详见[安装与发行包说明](docs/distribution.md)。

源码构建需要 Go 1.26+，建议使用最新受支持补丁版本：

```sh
go build -buildvcs=false -o cc-router ./cmd/ccr
# Windows: go build -buildvcs=false -o cc-router.exe ./cmd/ccr
```

主命令是 `cc-router`，以下示例统一使用它。`ccr` 仅为可选兼容别名；**musistudio/claude-code-router 已使用 ccr 名称**，因此不推荐安装或使用本项目的 `ccr` 别名，不要覆盖现有命令。

## 快速开始

桌面用户下载 `cc-router-desktop-<平台>-<架构>` 对应的安装包，或将便携包解压到固定的本地目录，无需把桌面程序目录加入 PATH。Windows/Linux 的 `cc-router-desktop` 与配套 `cc-router` 需要放在一起；macOS 配套 CLI 位于应用包内。打开桌面应用后：

1. 添加账号，默认勾选“创建后打开官方登录”；官方 CLI 会引导打开浏览器完成认证。也可稍后点击账号卡片的“登录”，无需先选项目。
2. 在账号的“更多操作”里安装额度状态栏；已有状态栏会保留并提示手动整合。
3. 选择真实项目文件夹，再启动工作会话；GUI 每 5 秒读取本地快照。未上报显示未知，超过 15 分钟或窗口到期显示过期，不会自行查询或切换账号。
4. 达到提醒阈值后，结束当前任务，在“工作交接”中保存并审阅笔记，明确选择目标账号再确认切换。

账号编辑、默认选择、项目绑定、交接编辑、环境诊断和元数据导入/导出都可在 GUI 操作。系统终端负责官方 Claude 的完整交互；桌面窗口不会嵌入或读取聊天记录。详细原生依赖与构建方法见 [桌面说明](desktop/README.md)。

## 后台更新与回滚

桌面应用运行时默认在启动及每 6 小时检查公开 GitHub Releases，后台下载并校验签名和 SHA-256。没有未保存交接、弹窗或操作，且 30 秒无交互并已关闭受管 Claude 会话后，才会退出并安装；也可以在“环境诊断”的更新区手动重启更新或关闭自动更新。

独立辅助进程在安装前备份账号登记和项目绑定，再同时更新 GUI 和配套 CLI，并保留上一版。新版需要在 45 秒内完成界面就绪确认；启动失败会自动恢复旧版并重新打开。成功升级后可手动回滚。更新不替换账号目录、交接文件或元数据。安装位置需要可写，详见[更新机制与发布流程](docs/updates.md)。

CLI 用户可以直接执行：

```sh
cc-router init
cc-router doctor
cc-router account add personal --label Personal
cc-router account add work --label Work
cc-router login personal
cc-router login work
cc-router use personal
cc-router bind work
cc-router run
cc-router run personal -- --continue
```

每个账号首次在官方界面完成登录；需要时使用不同浏览器 profile 选择正确账号。进入 Claude 后通过 `/status` 核对实际邮箱与认证/计费方式。本地标签只是标签。不同设备分别登录，工具不导出登录信息。

启动优先级：显式账号 > **当前目录**绑定 > 全局默认 > 终端选择菜单。绑定只作用于规范化后的同一目录，不自动信任仓库提供的账号配置，也不会向父目录寻找绑定。`use` 和 `bind` 只影响未来启动的进程。`--` 后的普通 Claude 参数按原值传递，认证/配置覆盖参数会被预检拒绝。

```sh
cc-router account list
cc-router account rename personal private --label Private
cc-router account remove work
cc-router unbind
cc-router status private
cc-router doctor private
cc-router export > accounts.json
cc-router import accounts.json
```

移除只删除登记并清理默认/绑定引用，保留官方目录。重新添加同名账号会创建新 ID、需要重新登录；工具不会自动重新关联遗留凭据。导出只有账号名、标签和默认账号名，不包含路径、项目绑定、凭据、历史。导入按名称合并元数据，已有 ID 保持不变。

## 额度预警与手动切换

使用[官方 statusline](https://code.claude.com/docs/en/statusline#rate-limit-usage)的 5h/7d 数据，不读取 token、不调用私有额度接口。**任一窗口 >=90% 提醒准备交接，>=95% 建议切换**，不自动中断或轮换。

GUI 的“安装额度状态栏”会写入当前账号 ID 和可执行文件绝对路径，仅保存用量数字、重置时间和接收时间，供桌面显示。安装后请保持程序路径稳定；移动程序时需更新状态栏命令。

仅使用 CLI 时，`cc-router usage config` 输出以下配置片段（只显示终端预警，不写入 GUI 用量缓存）。把 `statusLine` 字段合并到所选账号的 `settings.json` 中，保留其它设置；如果已有状态栏，先保存原设置并选择是否替换。可使用官方 `/statusline` 命令帮助配置。确保 `cc-router` 在 Claude 所用 shell 的 PATH 中。

```json
{
  "statusLine": {
    "type": "command",
    "command": "cc-router usage statusline --prepare-at 90 --switch-at 95"
  }
}
```

数据是官方最近报告的快照，首次响应前可能不存在；缺失、无效或已过期时显示 `unknown`，不会当作 0%。其它设备用量、单次大任务或模型独立限额可能使额度比显示更早耗尽。95% 是提醒阈值，不是保证有足够余量完成交接。`/usage` 用于官方核对；本项目不额外请求模型以刷新统计。

工具没有在后台登录其它账号查询余额。目标账号的额度未知时，启动后先用官方 `/usage` 核对，再开始大任务。

### 手动额度与本地限额记录

账号的“更多操作”提供“手动记录额度 / 本地限额”。可记录 5h、7d 的已用百分比与重置时间，或标记一个本地限额截止时间；至少填写一项，时间必须在未来一年内。记录由后端添加来源和记录时间，独立显示在官方用量下面，过期后明确标记。它只是个人提醒，不代表官方验证结果，也不阻止或自动轮换账号。

CLI 同样支持查看、记录和清除。每次记录会完整替换此前的手动记录，清除不影响官方状态栏数据。例如 PowerShell 中：

```powershell
# 百分比和重置时间应以你实际核对的值为准。
$reset = (Get-Date).AddHours(1).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
cc-router usage record personal --five-hour 95 --five-reset $reset
cc-router usage show personal
cc-router usage clear personal
```

7d 使用 `--seven-day` 与 `--seven-reset`，本地限额使用 `--limited-until`；日期统一接受带时区的 RFC3339。程序不读取对话来推算这些值。

### 手动切换与交接

收到提示后：

1. 在旧会话让 Claude 总结目标、修改、检查结果和下一步；等工具执行完成，正常退出。
2. `cc-router handoff` 创建 `.cc-router/handoff.md`，补充或更新内容。模板只收集过滤后的 Git 文件状态，不采集 diff、文件内容或会话记录。
3. `cc-router switch work` 显示交接位置；审阅后执行 `cc-router switch work --handoff-reviewed`。
4. 新会话先读取交接文件，核对项目规则、实际代码和 Git 状态，再继续。

交接文件默认本地忽略，已有文件不会被覆盖。它是可能过期的背景资料，不得覆盖项目规则，也不会被工具当成命令执行。`switch` 始终开启新会话，不接受 `--resume`。`run -- --continue` 和 `--resume` 交给官方 CLI 处理；默认仍是所选账号自己的历史。跨账号完整 transcript 恢复尚未验证，工具不共享或复制历史目录。

当前对同一账号、同一目录各允许一个受管会话，冲突时拒绝启动，建议独立 worktree/其它账号。锁仅约束本工具启动的进程，无法约束直接运行 Claude 或其它编辑器。

## 本地配置备份与恢复

“环境诊断”的备份区可创建、预览和恢复账号登记、默认选择及项目绑定。恢复需要审阅确认，并拒绝正在使用中的账号；恢复前自动保留当前配置，正常的恢复前备份可用于撤销。注册表损坏时，启动错误页仍提供备份恢复入口；损坏的原始注册表会被保留为不可直接恢复的诊断副本，超过 16 MiB 则拒绝自动覆盖。损坏的单个备份不会隐藏其他可用备份。

```sh
cc-router backup create
cc-router backup list
cc-router backup show BACKUP_ID
# 先审阅 show 输出，再填入其中的 digest。
cc-router backup restore BACKUP_ID --reviewed-digest SHA256
```

备份只保存在本地数据目录的 `backups` 中，绑定原数据目录并保留账号 ID；它包含本机项目路径，不适合公开分享，也不用于跨设备迁移。跨设备仍使用不含路径、ID 和凭据的 `export/import`，并重新登录。备份不读取、复制或回滚官方凭据和会话，也不备份项目文件、交接文件或用量缓存。

当前注册表继续采用 schema v1，可直接使用 alpha.1 数据。未知 schema 会保留原文件并报错；没有引入无实际需要的数据版本迁移。未来变更 schema 时必须提供备份、迁移和与程序回滚配套的恢复策略。

## 数据与边界

工具数据默认存放在 Windows `%LOCALAPPDATA%\cc-router`、Linux `$XDG_DATA_HOME/cc-router`（默认 `~/.local/share/cc-router`）、macOS `~/Library/Application Support/cc-router`。可通过绝对路径 `CCR_HOME` 指定本地私有目录，不应置于 Git、网盘或公共共享目录。

`accounts.json` 存储工具元数据；`profiles/<随机ID>` 是官方配置目录。重命名不移动目录。JSON 损坏/未知 schema 会报错并保留原文件。写入使用跨进程锁、临时文件和替换；目前只有 schema v1，不做静默迁移。自行备份元数据，官方凭据始终由官方客户端管理。桌面启动器通过 `cc-router --data-dir <绝对目录> <命令>` 显式选择同一数据目录，避免系统终端的继承环境不同。

启动时检测已知认证环境、配置和企业策略冲突，错误仅显示来源/字段名，不打印密钥值。设置热重载和未来 CLI 字段不在静态预检的保证范围内。动态/不支持的组织策略需要用官方客户端处理，工具不尝试绕过。详见 [安全说明](SECURITY.md) 和 [研究报告](docs/research.md)。

发行版没有对外监听的 HTTP 服务、token 导入、自动重放、后台遥测和云同步。更新器只访问本项目公开的 GitHub 发布信息和资产。卸载只需移除可执行文件/PATH 条目；官方配置目录默认保留。`CCR_CLAUDE_BIN` 可指定可信官方可执行文件的绝对路径，不能填写 shell 命令。开发用 Vite/Wails 服务只应用于本机。

## 开发

```sh
go test ./...
go vet ./...
go test -race ./... # requires a supported C compiler/toolchain
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

PowerShell：`./scripts/build.ps1` 生成六个 CLI 目标架构的二进制压缩包和 SHA256SUMS；安装原生依赖与 Wails 后，`./scripts/build-desktop.ps1` 在当前平台构建桌面应用及配套 CLI。桌面依赖隔离在 `desktop/go.mod`，CLI 核心仅依赖 Go 标准库。

CI 运行 Windows/macOS/Linux 核心测试、Linux race/vulnerability 检查以及三平台桌面构建。浏览器交互验证脚本为 `tests/gui_smoke.py`，使用显式的内存后端测试夹具，不访问真实账号。

[实施计划](docs/plans/2026-10-01-cc-router.md) · [兼容性与验收](docs/compatibility.md) · [开源调研](docs/research.md)

## License and acknowledgements

MIT © 2026 Harvey Xia. 本项目独立实现，参考 [Remeic/ccm](https://github.com/Remeic/ccm) 的进程边界设计，以及官方 Claude Code 文档。没有移植上述候选项目的源代码。若未来引入上游代码，将保留相应许可和来源。
