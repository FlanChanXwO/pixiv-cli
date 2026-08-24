# AGENTS.md

这是一个 Go 版 Pixiv CLI 与 MCP stdio server。它通过 `pixiv` CLI 和 `pixiv mcp` 暴露 Pixiv 搜索、详情、排行、推荐、用户、收藏、下载、token refresh 和缩略图能力，通过 `pixiv fanbox` CLI 与 `pixiv fanbox mcp` 暴露 FANBOX 能力。公开能力只来自 `sdk`、`sdk/pixiv`、`sdk/fanbox`。

## 核心命令

```bash
go test ./...
sh scripts/build.sh
```

真实 SDK e2e 默认跳过；需要本机凭据时显式运行（Pixiv 读本地 `pixiv-cli.db` 选中账号，FANBOX 读 macOS Keychain 授权 session）：

```bash
PIXIV_SDK_E2E=1 go test ./e2e -run TestRealPixivSDKRead -count=1 -v
FANBOX_E2E_CREATOR_ID=<non-secret-creator-id> FANBOX_E2E_TAG=<non-secret-tag> \
FANBOX_E2E_POST_ID=<non-secret-post-id> FANBOX_E2E_POST_URL=<non-secret-post-url> \
FANBOX_SDK_E2E=1 go test ./e2e -run TestRealFanboxSDKRead -count=1 -v
```

FANBOX E2E target variables are explicit non-secret test targets; the session remains in the macOS Keychain.

`TestRealFanboxSDKRead` 是唯一可以记为「完整 FANBOX read」的测试，它强制要求 post 目标带 first-party file attachment。`TestRealFanboxSDKPostInfo`（额外需要 `FANBOX_E2E_POST_ONLY=1`）只覆盖 `Post`/body/`ResolveURL`，通过时记为 **partial-pass**，不得替代完整 read 的状态。

## 边界规则

各包职责描述见 `docs/zh-CN/maintainers/architecture.md`；以下是不可违反的规则：

- `cmd/pixiv` 只委托 `internal/cli`；`internal/cli/root.go` 负责命令树、全局生命周期与生产组装，具体命令位于 `internal/cli/commands` 及其 Pixiv/FANBOX owner 子目录，不恢复旧 resource graph/requirements 层。
- CLI/MCP 的 Pixiv/FANBOX 能力只经公开 `sdk/pixiv`、`sdk/fanbox` 及 owner-local 窄端口调用；不得直连 `internal/services/{pixiv,fanbox}` 协议适配包。
- reverse-search is the only cross-boundary exception：生产组装仅允许 `internal/cli/root.go` 依赖 `internal/services/reversesearch/assembly`；`internal/cli/commands` 与 `internal/mcpserver` 只能依赖 `internal/services/reversesearch` 顶层契约，不得导入 `internal/services/reversesearch/saucenao`、`internal/services/reversesearch/ascii2d` 或其他其子包。provider 协议构造、HTTP client、凭据与代理只由 composition root/assembly 持有。
- MCP 聚合与输入/输出适配在 `internal/mcpserver/{pixiv,fanbox}`，具体 tool 位于各自 `tools/<tool>`；stdio runtime 由 CLI MCP 命令启动。
- `internal/shared/*` 承载跨命令共享机制，`internal/utils/{parse,text,uri}` 保持协议无关；配置 schema/snapshot 在 `internal/config/settings`，本地路径和权限在 `internal/config/paths`，文件机制在 `internal/storage/file/*`。
- 测试文件遵循 [`docs/zh-CN/maintainers/development.md` 测试文件布局](docs/zh-CN/maintainers/development.md#测试文件布局)的 same-stem、平台后缀和 same-package 例外规则；该节是唯一 canonical test-layout 规则。

## 注意事项

- 不提交 token、下载内容、本地数据库、缓存或机器相关配置。
- refresh token 只允许用户显式执行不带 `--output` 的 `pixiv auth export [UID]` 或 `pixiv auth export --all` 时写 stdout；前者输出 raw token，后者输出含 secret 的 bundle。除此之外不得写入 stdout、stderr、JSON、MCP 或错误；完整契约见现有 locale 的 CLI reference。
- 没有匿名 Web fallback：内容命令要求认证态本地账号（`pixiv auth use` 或手工 `[account_pool]`），否则返回认证要求；已删除的 `web_fallback_enabled` 若仍显式存在则返回 `removed_setting`，用 `pixiv config unset web_fallback_enabled` 清理。
- CLI 数据命令只使用本地 `auth use` 账号或手工 `[account_pool]`，拒绝 `--uid`/`--refresh-token` 并忽略 `PIXIV_REFRESH_TOKEN`；公开 SDK 仍可显式提供凭据。MCP 凭据选择遵循其独立 runtime 配置。
- MCP 模式 stdout 保留给 JSON-RPC；运行期失败保留 structured result 并设置 `isError=true`，不创建项目级日志。
- 不新增无依据的固定超时、截断、条数限制、重试上限、静默 fallback 或隐藏降级。确需新增时，必须有证据、代码注释、测试或文档说明。
- 修改 CLI/MCP tool、配置键、环境变量、输出语义、下载/认证/代理流程时，同步更新现有 locale 的 README、CLI reference 或对应 `docs/<locale>/`；涉及命令语义时同步检查 `skills/pixiv-cli/`。
- PR 只写改动、验证结果和检查清单。发布准备阶段用 `scripts/cmd/releasenotes audit` 审计 tag 范围，再直接整理 `changelog/vX.Y.Z/` 双语说明；每个 PR 或 direct commit 都要在两种语言中有来源，纯内部改动归入 `Maintenance`。
- 代码改动必须补充或更新聚焦测试，并运行相关回归；不能测试时说明原因和风险。

## 文档路由

- 架构与包职责：`docs/zh-CN/maintainers/architecture.md`。
- CLI 完整契约：`docs/en/cli-reference.md`、`docs/zh-CN/cli-reference.md`；README 只作为多语言入口。
- MCP tools：`docs/en/mcp-tools.md`、`docs/zh-CN/mcp-tools.md`；改 tool 时用 `.agents/skills/pixiv-cli-mcp-tool/`。
- 开发流程、配置、测试：`docs/zh-CN/maintainers/development.md`（含测试文件布局与能力边界）。
- AI 协作规则：`docs/index.md`（英文）与 `docs/index-zh-CN.md`；文档规范位于 `.agents/skills/pixiv-cli-docs/`，review checklist 位于 `.agents/skills/pixiv-cli-review/`。
- Repo-local skills：`.agents/skills/`（PR / CI / release-notes / review / docs / commit-msg / mcp-tool），只在对应任务需要时读取。
- 产品 skill（教 agent 使用 `pixiv` CLI，面向使用者分发）：`skills/pixiv-cli/`，全英文；改 CLI 命令/flag/语义时同步更新。
