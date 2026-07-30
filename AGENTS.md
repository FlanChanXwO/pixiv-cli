# AGENTS.md

这是一个 Go 版 Pixiv CLI 与 MCP stdio server。它通过 `pixiv` CLI 和 `pixiv mcp` 暴露 Pixiv 搜索、详情、排行、推荐、用户、收藏、下载、token refresh 和缩略图能力。

## 核心命令

```bash
go test ./...
sh scripts/build.sh
```

真实 Pixiv web fallback e2e 默认跳过；需要联网时显式运行：

```bash
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./e2e -run WebAPIFallbackReal -count=1 -v
```

## 边界规则

各包职责描述见 `docs/maintainers/architecture.md`；以下是不可违反的规则：

- `cmd/pixiv` 只委托 `internal/cli`；`internal/cli` 是 thin controller，业务用例在 `internal/application`，生产组装只在 `internal/bootstrap`。
- CLI/MCP 的 Pixiv 能力只经 `internal/application.SDKService` 调用顶层 `pixiv` public SDK；不得直连 `internal/services/pixiv/appapi`、`webapi`、`oauth` 或 `resource` 协议适配包。
- MCP tool 注册和输入/输出适配在 `internal/mcpserver`；stdio runtime 由 `internal/bootstrap` 启动。
- `internal/utils/*` 保持协议无关；本地状态路径常量位于 `internal/platform/localstate`，日志字段常量位于 `internal/logging`。

## 注意事项

- 不提交 token、下载内容、本地数据库、缓存或机器相关配置。
- refresh token 只允许用户显式执行不带 `--output` 的 `pixiv auth export [UID]` 或 `pixiv auth export --all` 时写 stdout；前者输出 raw token，后者输出含 secret 的 bundle。除此之外不得写入 stdout、stderr、JSON、MCP、日志或错误；完整契约见三语 CLI reference。
- web fallback 只有一条规则：refresh token 为空且 `web_fallback_enabled=true` 时走匿名 web/ajax API；有 refresh token 一律优先 App API，App API 出错不自动 fallback。
- CLI 数据命令只使用本地 `auth use` 账号或手工 `[account_pool]`，拒绝 `--uid`/`--refresh-token` 并忽略 `PIXIV_REFRESH_TOKEN`；公开 SDK 仍可显式提供凭据。MCP 凭据选择遵循其独立 runtime 配置。
- MCP 模式 stdout 保留给 JSON-RPC；操作日志写用户主目录 `~/.pixiv-cli/logs`（Windows 为 `%USERPROFILE%\.pixiv-cli\logs`）的按日纯文本 `YYYY-MM-DD.txt`，终端默认无日志痕迹。
- 不新增无依据的固定超时、截断、条数限制、重试上限、静默 fallback 或隐藏降级。确需新增时，必须有证据、代码注释、测试或文档说明。
- 修改 CLI/MCP tool、配置键、环境变量、输出语义、下载/认证/代理流程时，同步更新现有 locale 的 README、CLI reference 或对应 `docs/<locale>/`；涉及命令语义时同步检查 `skills/pixiv-cli/`。
- 功能 PR 以 `.github/PULL_REQUEST_TEMPLATE.md` 的 release-note 声明记录用户可感知的新增、修复、变更、废弃、移除或安全影响；合并后的 release-prep PR 使用 `scripts/releasenotes` 生成 `changelog/vX.Y.Z/` 双语说明。`changelog/unreleased/` 保留为 release-prep 入口；纯内部重构、测试和文档清理可选择 `None` 并说明理由。
- 代码改动必须补充或更新聚焦测试，并运行相关回归；不能测试时说明原因和风险。

## 文档路由

- 架构与包职责：`docs/maintainers/architecture.md`、`CONTEXT.md`、`docs/maintainers/adr/`。
- CLI 完整契约：`docs/en/cli-reference.md`、`docs/zh-CN/cli-reference.md`、`docs/ja/cli-reference.md`；README 只作为多语言入口。
- MCP tools：`docs/en/mcp-tools.md`、`docs/zh-CN/mcp-tools.md`；改 tool 时用 `.agents/skills/pixiv-cli-mcp-tool/`。
- 开发流程、配置、测试：`docs/maintainers/development.md`。
- AI 协作文档地图与 checklist：`docs/maintainers/agents/`。
- Repo-local skills：`.agents/skills/`（review / docs / commit-msg / mcp-tool），只在对应任务需要时读取。
- 产品 skill（教 agent 使用 `pixiv` CLI，面向使用者分发）：`skills/pixiv-cli/`，全英文；改 CLI 命令/flag/语义时同步更新。
