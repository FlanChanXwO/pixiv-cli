# AGENTS.md

本文件是本仓库 AI coding agents 的唯一主规则入口。`CLAUDE.md` 只引用本文件；其它工具提示不得复制本文件全文。

## 默认交互

- 默认使用简体中文答复；术语可夹英文；code-id、命令、路径保持英文。
- 代码注释默认中文，只解释意图、边界、约束和难点，避免复述代码。
- 每次 shell 命令必须显式设置工作目录，不依赖隐式 `cd` 状态。
- 文本检索优先 `rg`；大文件先检索再分段读。
- 保留用户改动，不回滚无关文件；新增/修改文件默认应纳入 Git 跟踪。

## 项目身份

`github.com/FlanChanXwO/pixiv-cli` 是 Go 版 Pixiv CLI 与 MCP stdio server。它通过 `pixiv` CLI 和 `pixiv mcp` 暴露 Pixiv 搜索、详情、排行、推荐、用户、收藏、下载、token refresh 和缩略图能力。

核心命令：

```bash
go test ./...
go build -o pixiv ./cmd/pixiv
```

真实 Pixiv web fallback e2e 默认跳过；需要联网时显式运行：

```bash
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

## 包边界

- `cmd/pixiv`：二进制入口，只委托 `internal/cli`。
- `internal/cli`：Cobra、TTY 交互、OAuth loopback、文本/JSON 输出和 `pixiv mcp` 分发；不要放业务 wiring。
- `internal/application`：账号、配置、作品查询、下载和登录完成等应用用例。
- `internal/bootstrap`：生产 composition root，唯一组装 config、auth storage、Pixiv source、download manager、MCP runtime 的地方。
- `internal/storage/auth`：`auth.json` 账号存储、默认账号和私有文件写入。
- `internal/config`：`config.toml` schema、defaults、effective runtime config 和 get/set/unset。
- `internal/pixiv`：Pixiv source facade；CLI/MCP 不应绕过它直接依赖 App/Web client。
- `internal/pixiv/api`、`internal/pixiv/web`、`internal/pixiv/model`：分别承接 App API、匿名 web fallback 和共享模型/协议 typed const。
- `internal/mcpserver`：MCP tool 注册、输入/输出适配和 tool 文本；stdio runtime 由 `internal/bootstrap` 启动。
- `internal/download`：下载队列、文件落盘、多页作品、ugoira zip 和 `ffmpeg` 转换。
- `internal/utils/*`：无业务语义工具；`internal/common/constants` 只放跨包基础设施常量。

## 硬约束

- 不提交 token、下载内容、本地数据库、缓存或机器相关配置。
- 不打印 refresh token；认证错误、网络错误、Pixiv/API 错误要暴露真实原因。
- 不新增无依据的固定超时、截断、条数限制、重试上限、静默 fallback 或隐藏降级。确需新增时，必须有证据、代码注释、测试或文档说明。
- 修改 CLI/MCP tool、配置键、环境变量、输出语义、下载/认证/代理流程时，同步更新 `README.md` 或 `docs/`。
- 用户可感知的新增、修复、变更、废弃、移除或安全影响，同步更新 `CHANGELOG.md` 的 `[Unreleased]`；纯内部重构、测试和文档清理可不记。
- 代码改动必须补充或更新聚焦测试，并运行相关回归；不能测试时说明原因和风险。

## Agent 文档路由

- 架构与包职责：`docs/architecture.md`、`CONTEXT.md`、`docs/adr/`。
- MCP tools：`docs/mcp-tools.md`；改 tool 时同步测试、README/docs，必要时更新 changelog。
- 开发流程、配置、测试：`docs/development.md`。
- AI 协作文档地图与 checklist：`docs/agents/`。
- Repo-local skills：`.agents/skills/`，只在对应任务需要时读取。
