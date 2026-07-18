# AGENTS.md

这是一个 Go 版 Pixiv CLI 与 MCP stdio server。它通过 `pixiv` CLI 和 `pixiv mcp` 暴露 Pixiv 搜索、详情、排行、推荐、用户、收藏、下载、token refresh 和缩略图能力。

## 核心命令

```bash
go test ./...
sh scripts/build.sh
```

真实 Pixiv web fallback e2e 默认跳过；需要联网时显式运行：

```bash
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

## 边界规则

各包职责描述见 `docs/architecture.md`；以下是不可违反的规则：

- `cmd/pixiv` 只委托 `internal/cli`；`internal/cli` 是 thin controller，业务用例在 `internal/application`，生产组装只在 `internal/bootstrap`。
- CLI/MCP 的 Pixiv 能力只经 `internal/application.SDKService` 调用顶层 `pixiv` public SDK；不得直连 `internal/pixiv/appapi`、`webapi`、`oauth` 或 `resource` 协议适配包。
- MCP tool 注册和输入/输出适配在 `internal/mcpserver`；stdio runtime 由 `internal/bootstrap` 启动。
- `internal/utils/*` 保持协议无关；`internal/common/constants` 只放跨包基础设施常量。

## 注意事项

- 不提交 token、下载内容、本地数据库、缓存或机器相关配置。
- refresh token 只允许用户显式执行 `pixiv auth token [UID]` 时原样写 stdout；该命令之外不得写入 stdout、stderr、JSON、MCP、日志或错误。认证错误、网络错误、Pixiv/API 错误要暴露真实原因。
- web fallback 只有一条规则：refresh token 为空且 `web_fallback_enabled=true` 时走匿名 web/ajax API；有 refresh token 一律优先 App API，App API 出错不自动 fallback。
- token 优先级：CLI 为 `--refresh-token` > `--uid` > `PIXIV_REFRESH_TOKEN` > 默认 UID；MCP 为 `PIXIV_REFRESH_TOKEN` > 默认 UID。
- MCP 模式 stdout 保留给 JSON-RPC；日志和诊断写 stderr。
- 不新增无依据的固定超时、截断、条数限制、重试上限、静默 fallback 或隐藏降级。确需新增时，必须有证据、代码注释、测试或文档说明。
- 现存知识图谱仅作为历史快照保留，不是当前代码或文档的权威依据，也不作为开发、审查或交付门禁；常规功能分支不得生成或更新图谱产物。
- 修改 CLI/MCP tool、配置键、环境变量、输出语义、下载/认证/代理流程时，同步更新双语 README、双语 CLI reference 或对应 `docs/`；涉及命令语义时同步检查 `skills/pixiv-cli/`。
- 用户可感知的新增、修复、变更、废弃、移除或安全影响，同步更新 `CHANGELOG.md` 的 `[Unreleased]`；纯内部重构、测试和文档清理可不记。
- 代码改动必须补充或更新聚焦测试，并运行相关回归；不能测试时说明原因和风险。

## 文档路由

- 架构与包职责：`docs/architecture.md`、`CONTEXT.md`、`docs/adr/`。
- CLI 完整契约：`docs/cli-reference.md`、`docs/cli-reference.zh-CN.md`；README 只作为双语入口。
- MCP tools：`docs/mcp-tools.md`；改 tool 时用 `.agents/skills/pixiv-cli-mcp-tool/`。
- 开发流程、配置、测试：`docs/development.md`。
- AI 协作文档地图与 checklist：`docs/agents/`。
- Repo-local skills：`.agents/skills/`（review / docs / commit-msg / mcp-tool），只在对应任务需要时读取。
- 产品 skill（教 agent 使用 `pixiv` CLI，面向使用者分发）：`skills/pixiv-cli/`，全英文；改 CLI 命令/flag/语义时同步更新。
