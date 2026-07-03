# AGENTS.md

| 语言：默认使用简体中文答复；术语可夹英文；code-id、命令、路径保持英文；代码注释默认中文。 |
| 工具：能本地则本地；先收敛后放大；输入最小必要；关键步骤要可复现。 |
| 风险操作：联网、安装依赖、长任务、破坏动作、大改动前，先说明意图、影响与产物。 |
| 命令：每次命令必须显式设置工作目录；不要依赖隐式 `cd` 状态；优先小而直接的命令，控制输出规模。 |
| 检索：文本检索优先 `rg`；大文件先检索再分段读取；不要并发全读大文件。 |
| Git：不要回滚用户改动；初始化或提交前关注未跟踪文件；本地数据库、下载产物和构建产物不应进入版本库。 |

## 项目概览

这是 `github.com/FlanChanXwO/pixiv-mcp-server`，一个 Go 版 Pixiv MCP stdio server。它将 Pixiv 搜索、详情、推荐、排行榜、用户、收藏、下载与缩略图能力暴露为 MCP tools，供 MCP client 通过标准输入输出调用。

当前主要入口与模块：

- `cmd/pixiv/`：官方二进制入口，只负责委托给 `internal/cli`。
- `internal/cli/`：Cobra 命令树、TTY 交互、OAuth loopback 登录、文本/JSON 输出和 `pixiv mcp` 分发。
- `internal/cli/state/`：`auth.json` 账号存储、默认账号、认证文件路径和权限写入。
- `internal/cli/mcpapp/`：`pixiv mcp` 的配置、账号、Pixiv source、下载管理器与 MCP server 组装。
- `internal/config/`：`config.toml` schema、默认值、运行时配置合并以及 `get/set/unset` 写回。
- `internal/pixiv/`：Pixiv source facade，对 CLI/MCP 暴露稳定构造函数与常用模型 alias。
- `internal/pixiv/api/`：Pixiv App API client、OAuth refresh 和 authorization-code exchange。
- `internal/pixiv/web/`：匿名 Pixiv web/ajax API client，用于无 token fallback。
- `internal/pixiv/model/`：共享 Pixiv response/domain 类型与协议 typed const。
- `internal/mcpserver/`：MCP tool 注册与参数/返回文本适配。
- `internal/download/`：下载队列、文件命名、多页作品保存、ugoira zip 转 gif。
- `internal/utils/`：文件名清理、模板生成、ID 去重和 Pixiv web refresh token 输入解析。
- `manifest.json`：DXT/MCP 打包与用户配置声明。
- `docs/`：项目文档索引、架构、开发流程和工具说明。

## 本地环境

项目当前是 Go 工程，`go.mod` 声明 `go 1.26.3`。开工前至少检查：

```bash
go version
go test ./...
go build -o pixiv ./cmd/pixiv
```

可选运行依赖：

- `ffmpeg`：仅 ugoira 转 GIF 需要。缺失时普通图片下载不受影响。
- `PIXIV_REFRESH_TOKEN`：认证相关工具和个性化接口需要。
- `https_proxy` 或 `HTTPS_PROXY`：网络受限时可配置 Pixiv/API 访问代理。

## 配置

运行配置拆分为：

- `os.UserConfigDir()/pixiv/auth.json`：账号认证与默认账号。
- `os.UserConfigDir()/pixiv/config.toml`：全局可手改配置。

环境变量仍保留公开覆盖层：

- `PIXIV_REFRESH_TOKEN`：Pixiv refresh token。
- `DOWNLOAD_PATH`：下载目录，默认 `./downloads`。
- `FILENAME_TEMPLATE`：文件名模板，默认 `{author} - {title}_{id}`。
- `https_proxy` / `HTTPS_PROXY`：HTTP client 代理地址，优先小写变量。

不要把 token、下载文件、缓存数据库或个人路径写进代码或提交到 Git。

## 开发约束

- 新增或修改功能/API/配置/流程/测试限制时，同步更新 `docs/` 或 `README.md`。
- 代码组织保持包职责清晰，避免把多职责塞进单文件。
- 注释默认中文，解释意图、边界、约束和难点；不要写纯噪音注释。
- 错误处理要暴露真实原因，不要吞异常、伪装成功、静默降级或返回空结果冒充正常。
- 不要无依据新增固定超时、截断、条数限制、重试上限、轮询上限或隐藏兜底。确需新增限制时，必须有产品/平台/接口证据或测试依据，并在代码注释和交付总结中说明触发条件、目的、影响与风险。
- 抓取或展示 Pixiv 数据时，数据层优先保完整；展示层需要折叠时在 UI/输出层处理，不要无据裁剪原始数据。
- 涉及网络不稳时，可说明后临时使用 `localhost:7890` 或 `127.0.0.1:7890` 作为 HTTP(S) 代理，仅限当前必要操作；失败时暴露真实原因。

## 测试与回归

代码任务完成前应补充或更新测试，并运行相关回归。当前已有测试覆盖：

- `internal/cli/*_test.go`
- `internal/config`
- `internal/utils`
- `test/e2e/pixiv_binary_test.go`
- `internal/pixiv/...`
- `internal/download/manager_test.go`
- `internal/mcpserver/server_test.go`

常用命令：

```bash
go test ./...
go build -o pixiv ./cmd/pixiv
```

真实 Pixiv web fallback e2e 默认跳过；需要联网验证时显式运行：

```bash
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

若无法测试，必须说明原因和剩余风险。
