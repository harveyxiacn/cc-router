# CC Router

本地 Claude Code 多账号启动器与项目交接工具。Go 编写，使用未修改的官方 Claude Code 登录和运行。账号选择明确、官方配置独立、项目进度可交接。

**当前为 0.1.0-alpha.1。** 自动化测试和可编译平台不等于真实账号验收；[兼容性记录](docs/compatibility.md) 列出验证边界。项目与 Anthropic 无隶属关系。

## 安装

先按[官方说明](https://code.claude.com/docs/en/setup)安装 Claude Code（本项目要求 >=2.1.268）。从本仓库 Releases 下载对应平台压缩包，核对 SHA256SUMS，解压并把目录加入 PATH。Linux/macOS 解压后执行 `chmod +x cc-router ccr`。Alpha 二进制尚未签名或公证，macOS 正式发布需要完成这些步骤。

源码构建需要 Go 1.26+，建议使用最新受支持补丁版本：

```sh
go build -buildvcs=false -o cc-router ./cmd/ccr
# Windows: go build -buildvcs=false -o cc-router.exe ./cmd/ccr
```

以下示例使用 `ccr`；`cc-router` 接受相同命令。**musistudio/claude-code-router 已使用 ccr 名称**；如果已经安装该项目，请使用本项目的 `cc-router`，不要覆盖现有命令。

## 快速开始

```sh
ccr init
ccr doctor
ccr account add personal --label Personal
ccr account add work --label Work
ccr login personal
ccr login work
ccr use personal
ccr bind work
ccr run
ccr run personal -- --continue
```

每个账号首次在官方界面完成登录；需要时使用不同浏览器 profile 选择正确账号。进入 Claude 后通过 `/status` 核对实际邮箱与认证/计费方式。本地标签只是标签。不同设备分别登录，工具不导出登录信息。

启动优先级：显式账号 > **当前目录**绑定 > 全局默认 > 终端选择菜单。绑定只作用于规范化后的同一目录，不自动信任仓库提供的账号配置，也不会向父目录寻找绑定。`use` 和 `bind` 只影响未来启动的进程。`--` 后的普通 Claude 参数按原值传递，认证/配置覆盖参数会被预检拒绝。

```sh
ccr account list
ccr account rename personal private --label Private
ccr account remove work
ccr unbind
ccr status private
ccr doctor private
ccr export > accounts.json
ccr import accounts.json
```

移除只删除登记并清理默认/绑定引用，保留官方目录。重新添加同名账号会创建新 ID、需要重新登录；工具不会自动重新关联遗留凭据。导出只有账号名、标签和默认账号名，不包含路径、项目绑定、凭据、历史。导入按名称合并元数据，已有 ID 保持不变。

## 额度预警与手动切换

使用[官方 statusline](https://code.claude.com/docs/en/statusline#rate-limit-usage)的 5h/7d 数据，不读取 token、不调用私有额度接口。**任一窗口 >=90% 提醒准备交接，>=95% 建议切换**，不自动中断或轮换。

`ccr usage config` 输出以下配置片段。把 `statusLine` 字段合并到所选账号的 `settings.json` 中（保留其它设置；如果已有状态栏，先保存原设置并选择是否替换）。可使用官方 `/statusline` 命令帮助配置。确保 `cc-router` 在 Claude 所用 shell 的 PATH 中。

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

收到提示后：

1. 在旧会话让 Claude 总结目标、修改、检查结果和下一步；等工具执行完成，正常退出。
2. `ccr handoff` 创建 `.cc-router/handoff.md`，补充或更新内容。模板只收集过滤后的 Git 文件状态，不采集 diff、文件内容或会话记录。
3. `ccr switch work` 显示交接位置；审阅后执行 `ccr switch work --handoff-reviewed`。
4. 新会话先读取交接文件，核对项目规则、实际代码和 Git 状态，再继续。

交接文件默认本地忽略，已有文件不会被覆盖。它是可能过期的背景资料，不得覆盖项目规则，也不会被工具当成命令执行。`switch` 始终开启新会话，不接受 `--resume`。`run -- --continue` 和 `--resume` 交给官方 CLI 处理；默认仍是所选账号自己的历史。跨账号完整 transcript 恢复尚未验证，工具不共享或复制历史目录。

Alpha 对同一账号、同一目录各允许一个受管会话，冲突时拒绝启动，建议独立 worktree/其它账号。锁仅约束本工具启动的进程，无法约束直接运行 Claude 或其它编辑器。

## 数据与边界

工具数据默认存放在 Windows `%LOCALAPPDATA%\cc-router`、Linux `$XDG_DATA_HOME/cc-router`（默认 `~/.local/share/cc-router`）、macOS `~/Library/Application Support/cc-router`。可通过绝对路径 `CCR_HOME` 指定本地私有目录，不应置于 Git、网盘或公共共享目录。

`accounts.json` 存储工具元数据；`profiles/<随机ID>` 是官方配置目录。重命名不移动目录。JSON 损坏/未知 schema 会报错并保留原文件。写入使用跨进程锁、临时文件和替换；目前只有 schema v1，不做静默迁移。自行备份元数据，官方凭据始终由官方客户端管理。桌面启动器通过 `cc-router --data-dir <绝对目录> <命令>` 显式选择同一数据目录，避免系统终端的继承环境不同。

启动时检测已知认证环境、配置和企业策略冲突，错误仅显示来源/字段名，不打印密钥值。设置热重载和未来 CLI 字段不在静态预检的保证范围内。动态/不支持的组织策略需要用官方客户端处理，工具不尝试绕过。详见 [安全说明](SECURITY.md) 和 [研究报告](docs/research.md)。

没有 HTTP 服务、token 导入、自动重放、后台遥测、云同步和自动更新。卸载只需移除可执行文件/PATH 条目；官方配置目录默认保留。`CCR_CLAUDE_BIN` 可指定可信官方可执行文件的绝对路径，不能填写 shell 命令。

## 开发

```sh
go test ./...
go vet ./...
go test -race ./... # requires a supported C compiler/toolchain
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
```

PowerShell：`./scripts/build.ps1` 生成六个目标架构的二进制压缩包和 SHA256SUMS。CI 运行 Windows/macOS/Linux 测试与 Linux race/vulnerability 检查。

[实施计划](docs/plans/2026-10-01-cc-router.md) · [兼容性与验收](docs/compatibility.md) · [开源调研](docs/research.md)

## License and acknowledgements

MIT © 2026 Harvey Xia. 本项目独立实现，参考 [Remeic/ccm](https://github.com/Remeic/ccm) 的进程边界设计，以及官方 Claude Code 文档。没有移植上述候选项目的源代码。若未来引入上游代码，将保留相应许可和来源。
