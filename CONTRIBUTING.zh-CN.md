# 为 pixiv-cli 贡献

[English](CONTRIBUTING.md) | 简体中文

感谢你帮助改进 `pixiv-cli`。我们欢迎聚焦的 bug report、文档修复、测试和边界清晰的功能。

## 开始之前

- 先检索已有 issue 和 pull request，避免重复提交。
- 大功能、public API 变更、新依赖或认证变更应在实施前讨论。
- 不要在 issue、fixture、commit 或 CI 日志中包含 Pixiv token、Cookie、下载作品、本地数据库、缓存、机器配置或私有 API 响应。
- 保持改动聚焦；无关清理更适合单独提交 pull request。

## 开发环境

受支持的源码构建使用：

- `go.mod` 声明的 Go 版本；
- `CGO_ENABLED=1` 和目标平台可用的 C linker；
- 仓库中经 manifest 校验、与目标平台匹配的 Rust ugoira static library。

只有修改 ugoira encoder 或重新构建 static library 时才需要 Rust。不要在无关贡献中安装或重新生成 native artifact。Windows 构建通过 Git Bash、MSYS2 或 WSL 运行。

在仓库根目录构建和测试：

```bash
go test ./...
sh scripts/build.sh
./build/pixiv --help
```

Native library 校验、opt-in 真实 API 测试、发布门禁和平台细节见[开发流程](docs/zh-CN/maintainers/development.md)。

## 架构边界

- `cmd/pixiv` 委托 `internal/cli`；`internal/cli/root.go` 负责命令注册、全局生命周期与生产组装，具体命令位于 `internal/cli/commands`。
- CLI/MCP 的 Pixiv/FANBOX 能力经 owner-local 窄端口调用公开 `sdk/pixiv`、`sdk/fanbox`，不得直连 `internal/services/{pixiv,fanbox}` 协议适配包。
- MCP 产品聚合在 `internal/mcpserver/{pixiv,fanbox}`，具体 tool 位于各产品 `tools/<tool>`；stdio 由 CLI MCP 命令启动，stdout 只用于 JSON-RPC。
- `internal/shared/*` 负责跨子系统机制，`internal/utils/*` 保持协议无关，配置/路径归 `internal/config/{settings,paths}`；文件应聚焦于一个职责或少数紧密相关职责。

修改这些边界前，请阅读[架构说明](docs/zh-CN/maintainers/architecture.md)与仓库 [AGENTS.md](AGENTS.md)。

## 使用测试驱动开发

代码变更采用 red-green-refactor：

1. 添加一个会因目标行为尚未实现而失败的聚焦测试。
2. 实现让它通过的最小完整变更。
3. 在不改变已验证公开行为的前提下重构。
4. 先运行聚焦测试，再运行相关回归。

可行时通过 public boundary 测试公开行为。不得把真实的认证、网络、Pixiv API、文件系统或编码失败隐藏为空成功或静默 fallback；不得增加无依据的 timeout、截断、分页上限、重试限制或隐藏降级。

真实 Pixiv/FANBOX SDK 与反向搜图检查均为 opt-in。未经用户明确授权，不得使用其本地账号运行或上传图片；也不要把真实 token 放入可能写入 shell history 的命令行。

## Agent 辅助开发

从 [AGENTS.md](AGENTS.md) 开始。仓库内的 `pixiv-cli-*` 维护技能定义 Go 设计、聚焦测试、MCP/native、审查、PR 与发布流程，不依赖个人全局指令或 CCS 安装。不支持技能发现的客户端可直接读取对应 `SKILL.md`。Agent 指令、技能正文、引用文件与 UI 元数据使用英文；公开文档保留双语。

## 文档

修改命令、flag、SDK API、MCP tool、配置键、环境变量、输出契约、认证流程、代理行为、下载行为或已知限制时，在同一 pull request 同步文档。

- 保持 `README.md` 与 `README.zh-CN.md` 的行为语义对应。
- 保持 `docs/<locale>/` 下已有语言版本的行为语义对应；不得用未翻译占位内容冒充对应语言。
- 按文件职责更新 localized SDK/MCP contract 或 `docs/zh-CN/maintainers/`。
- PR 正文只留审查需要的内容：变更说明、实际验证和简短自查。发布准备阶段由维护者审计所有已合并 PR 和 direct commit，再直接编写带来源的英文与简体中文版本说明；纯内部改动归入 `Maintenance`。具体流程见[发布说明与发布](docs/zh-CN/maintainers/development.md#release-notes-and-publication)。
- CLI 命令、flag 或安全语义变化时检查 `skills/pixiv-cli/`。

稳定规则只在一个权威文档中定义，其他位置应链接过去，避免复制大段内容。

## Pull request checklist

请求 review 前确认：

- [ ] 改动保持聚焦，并说明了用户可感知行为。
- [ ] 行为变更有实际 Red/Green 和相关回归证据，或明确接受的阻塞；纯文档变更已检查内容、链接与元数据。
- [ ] [pixiv-cli-test](.agents/skills/pixiv-cli-test/SKILL.md) 中适用的本地检查通过，包括适用范围内的全量、race、native 检查；未执行项目已注明。
- [ ] 必需 CI 按当前 head 与实际路径分类器判断，不把 Markdown 变更自动视为豁免。
- [ ] 已安装且适用的既有 pre-commit 检查通过；缺少工具时据实报告，不静默安装。
- [ ] `git diff --check` 通过。
- [ ] 需要同步的英文与简体中文文档已对应。
- [ ] 未包含凭据、下载内容、本地状态或机器相关产物。

Commit message 推荐使用 Conventional Commits，例如 `fix(cli): preserve record identity` 或 `docs: clarify account selection`。除非未来规范明确要求，项目不要求 CLA、DCO sign-off 或 signed commit。

## 许可证

提交贡献即表示你同意该贡献可按仓库的 [MIT License](LICENSE) 分发。
