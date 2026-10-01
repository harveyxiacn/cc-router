# 开源方案与风险评估

核验日期：2026-10-01。范围依据用户提供的项目计划：Go 本地账号启动器、独立官方配置目录、显式选号与项目交接。以下是文档和部分源码审查，没有运行上游完整测试，也不是对上游的安全认证。

## 结论

独立实现一个小型 Go CLI，优先借鉴 **Remeic/ccm** 的进程生命周期和测试思路。现有大型 router 的网关、协议转换、凭证池会显著扩大维护范围，不适合作为本计划的整库 fork 底座。本项目尚未复制任何上游代码。

| 候选 | 核验事实与来源 | 复用判断 |
| --- | --- | --- |
| [claude-code-router](https://github.com/musistudio/claude-code-router) | MIT；当前 v3.1.1；Node、多包 CLI/Web/Electron 网关 | 网关/路由/fallback 超出范围；[CLI package](https://github.com/musistudio/claude-code-router/blob/main/packages/cli/package.json) 已占用 `ccr`，提供 `cc-router` 替代名称 |
| [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) | MIT；Go 网关；OAuth、协议转换、账户负载均衡和 SDK | 语言相同，认证责任不同；未来合法 API gateway 可重新评估固定版本的 SDK |
| [CC Switch](https://github.com/farion1231/cc-switch) | MIT；Tauri/Rust/React 桌面应用、配置与路由管理 | 可研究交互，不作为轻量 Go CLI 底座 |
| [ccm](https://github.com/Remeic/ccm) | MIT；Node >=24；包 v0.7.0；官方 CLI + CLAUDE_CONFIG_DIR | 最接近，借鉴生命周期和测试；不直接移植 |
| [claudenv](https://github.com/bodasooqa/claudenv) | MIT；Bash/Zsh 项目绑定；import 递归复制目录 | 借鉴绑定，不采用复制 profile 的导入方式 |
| [Rust claude-switch](https://github.com/Abhishek21k/claude-switch) | MIT；[profile.rs](https://github.com/Abhishek21k/claude-switch/blob/main/src/profile.rs) 包含凭据读取/提取 | 不符合本项目不处理凭据的边界 |
| [claude-profile-switch](https://github.com/guibes/claude-profile-switch) | MIT；README 描述交换配置与将加密凭据存入 Git | 不符合无凭据导出的边界 |

### 最值得借鉴的 ccm

[claude.ts](https://github.com/Remeic/ccm/blob/main/src/lib/claude.ts) 直接启动官方 CLI、继承终端，并对状态命令使用超时。我们借鉴其边界，但加入认证环境冲突检查。完整继承环境可能引入认证覆盖，这是基于源码的风险推断，未对其复现漏洞。

[config.ts](https://github.com/Remeic/ccm/blob/main/src/lib/config.ts) 在读取/解析异常时返回空配置，使用固定临时文件再 rename；在该文件层面未见跨进程锁。本项目区分首次启动与损坏文件，以锁保护读改写，使用随机临时文件原子替换。未声称上游已经发生数据丢失。

[profile-store.ts](https://github.com/Remeic/ccm/blob/main/src/lib/profile-store.ts) 的目录/配置一致性检查和回滚值得参考。本项目使用稳定随机 ID，重命名只改登记，不移动目录，从而减少与 Keychain 和运行进程的关联风险。

它有[三平台 CI](https://github.com/Remeic/ccm/blob/main/.github/workflows/ci.yml)，但 [CLI 测试](https://github.com/Remeic/ccm/blob/main/tests/lib/claude.test.ts) 使用子进程 mock，不能据此推断三平台真实登录已经验证。

## 官方支持和风险

1. **认证与计费混用。** [官方认证文档](https://code.claude.com/docs/en/authentication#log-in-with-multiple-accounts) 明确支持 CLAUDE_CONFIG_DIR 多账号和 macOS Keychain 按目录隔离。但 Console 无 API key 登录使用外部 Anthropic profile。订阅启动要检查 API/token/provider/gateway/federation 覆盖，并使用独立 ANTHROPIC_CONFIG_DIR（见 [WIF reference](https://platform.claude.com/docs/en/manage-claude/wif-reference#configuration-directory)）。
2. **状态并非额度或计费保证。** [CLI reference](https://code.claude.com/docs/en/cli-reference) 支持 `auth status --json`，>=2.1.268 提供 configDirectory；完整 JSON schema 并未承诺稳定。不能把标签当邮箱、把 429 当额度耗尽，或虚构剩余额度。未知输出降级为未知。
3. **配置来源复杂。** [Settings](https://code.claude.com/docs/en/settings)、[managed settings](https://code.claude.com/docs/en/managed-settings) 和[环境变量](https://code.claude.com/docs/en/env-vars) 包含环境、项目、企业策略、Registry/MDM、远程及动态 helper。静态预检不等于整个会话的认证保证；不能执行 helper 来猜测结果或绕过组织策略。热重载和新版本字段需要兼容性维护。
4. **凭据与条款。** [官方法律与合规说明](https://code.claude.com/docs/en/legal-and-compliance#authentication-and-credential-use) 支持用户通过原版 Claude Code 使用自己的订阅；不能把第三方 token 收集/中转和订阅池化当成获得授权。不同地区适用的 [Consumer Terms](https://www.anthropic.com/legal/consumer-terms) 需按实际账户核对；本项目不提供“防封”承诺。人工选号也不自动证明任意使用场景合规。
5. **交接内容。** 仅采集 Git 元数据和路径摘要，排除敏感文件名，不导出 diff、对话或历史。交接默认本地排除，内容可能过期或被修改，不能覆盖项目规则或自动执行。
6. **终端与并发。** 三平台真实 Ctrl+C、窗口调整、路径空格/中文、并发写入和真实身份隔离需要分别验证。交叉编译和 mock 无法替代真机测试。macOS 签名、公证是正式发布门槛。

## 用户确认的用量与续接设计

在调研后的需求补充中，用户选择“提醒并手动确认切换”。[官方 statusline](https://code.claude.com/docs/en/statusline#rate-limit-usage) 已提供 5h/7d 用量百分比和重置时间，无需读取 OAuth token；使用任一窗口达到 90% 准备交接、95% 建议切换的默认值。这些是可配置的产品阈值，不是官方推荐值或保证可用余量。

[官方会话文档](https://code.claude.com/docs/en/sessions) 支持 transcript 路径 resume，同时说明内部 JSONL 格式会随版本变化。跨账号恢复、权限恢复和项目工具配置一致性尚未实测。因此默认采用新会话 + 用户审阅交接 + 当前代码/Git 状态。可以在后续提供经验证、显式选择的完整对话续接，但不共享历史目录，不拷贝凭据。

## 开源与维护（许可）

当前独立实现采用 MIT。若以后复制上游实质代码，应保存对应版权、许可原文和来源 commit；主项目 MIT 不能代替对每个依赖/资源的许可检查。[CCR LICENSE](https://github.com/musistudio/claude-code-router/blob/main/LICENSE)、[CLIProxyAPI LICENSE](https://github.com/router-for-me/CLIProxyAPI/blob/main/LICENSE)、[CC Switch LICENSE](https://github.com/farion1231/cc-switch/blob/main/LICENSE)、[ccm LICENSE](https://github.com/Remeic/ccm/blob/main/LICENSE)。

建议按具体固定版本评估接口和测试，而非依赖 star 数量。首版没有网络监听、遥测、云同步或自动更新服务。后续 Gateway 如立项，应独立配置、只用允许的 API/云凭证，并另做预算、重试、流中断和工具副作用审查。
