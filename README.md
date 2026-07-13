# Pixiv CLI / MCP / Go SDK

`pixiv-cli` 提供三种本地接入方式：`pixiv` CLI、`pixiv mcp` stdio server，以及可嵌入 Go 程序的公开包 [`pkg/pixiv`](pkg/pixiv)。不提供 HTTP server，也不提供 `Discover`、RSS 或 crawler 能力。

SDK 是具体的 `*pixiv.Client`；调用方按自己业务定义窄 adapter。采集模式、预算、过滤、cursor 持久化、入库和编排属于调用方。例如 `atri-setu-api` 可在自己的 adapter 中组合 `SearchIllust`、`UserArtworks`、`UserBookmarks`、`IllustRanking` 与详情/资源调用。

用户可感知改动见 [CHANGELOG.md](CHANGELOG.md)。SDK 契约见 [pixiv-sdk-interface.md](pixiv-sdk-interface.md)，架构决策见 [ADR 0007](docs/adr/0007-public-pixiv-sdk-and-caller-adapter.md)。

## 构建

```bash
sh scripts/build.sh
go test ./...
```

默认二进制为 `build/pixiv`。也可安装：

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@latest
```

## CLI

先以浏览器 OAuth 登录；refresh token 不会打印。

```bash
pixiv auth login
pixiv search "初音ミク" --json --limit 20
pixiv user artworks                 # 当前认证用户
pixiv user artworks 12345678
pixiv user bookmarks --restrict private
pixiv bookmark add 123456 --tag favourite
pixiv follow add 12345678
pixiv download 123456
```

主要命令：

| 命令 | 说明 |
| --- | --- |
| `auth add/login/list/use/remove/check` | 本地多账号管理；账号按 Pixiv UID 保存。 |
| `config path/get/set/unset` | 管理 `config.toml`。 |
| `search/detail/ranking/recommended` | 作品查询。 |
| `user artworks/bookmarks/following [USER_ID]` | 用户数据；省略 `USER_ID` 时读取当前认证用户。 |
| `bookmark add/remove`、`follow add/remove` | 收藏与关注写操作，需要认证。 |
| `download ILLUST_ID...` | 下载作品。 |
| `mcp` | 启动 MCP stdio server。 |

数据列表统一使用逻辑分页：

- `--limit N`：最多 N 项；`--limit 0` 读取到上游没有下一批。
- `--page N`：从第 N 个逻辑页取数据，必须同时给正数 `--limit`。
- 不给 `--limit`：为兼容旧行为，仅取一个上游批次。
- `--offset` 已废弃；不能与 `--page` 同用。SDK cursor 不暴露到 CLI。

通用认证优先级：`--refresh-token` > `--uid` > `PIXIV_REFRESH_TOKEN` > 默认 UID。`--profile` 是废弃 alias。网络命令可用 `--proxy URL`；优先级是 `--proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`，本次覆盖不会持久化。

### 配置与日志

账号保存在 `os.UserConfigDir()/pixiv/auth.json`，配置保存在 `os.UserConfigDir()/pixiv/config.toml`；两者写入权限为 `0600`。常用配置：

| key | 环境变量 | 默认 | 说明 |
| --- | --- | --- | --- |
| `download_path` | `DOWNLOAD_PATH` | `./downloads` | 下载目录。 |
| `filename_template` | `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | 下载文件名。 |
| `https_proxy` | `https_proxy` / `HTTPS_PROXY` | 空 | HTTP(S) 代理。 |
| `web_fallback_enabled` | - | `true` | 无 refresh token 时允许匿名 Web API。 |
| `output_json` | - | `false` | 数据命令默认 JSON 输出。 |
| `log_level` | `PIXIV_LOG_LEVEL` | `info` | `trace`、`debug`、`info`、`warn`、`error`。 |
| `log_format` | `PIXIV_LOG_FORMAT` | `text` | `text` 或 `json`。 |

日志由注入的 `slog.Logger` 输出 stderr；MCP stdout 永远只承载 JSON-RPC。日志不输出 refresh token、cookie、完整 URL、查询词或资源 header。

### 路由规则

有 refresh token 时，App API 是主路径；App API 的认证、网络或服务端失败直接返回，不自动回落 Web。无 refresh token 且 `web_fallback_enabled=true` 时，匿名白名单读操作才可用 Web API。`IllustDetail` 的 Web pages 补全和原始 ugoira 资源解析是显式 enrichment，不是 App 失败回退。

## Go SDK

导入包：

```go
import pixiv "github.com/FlanChanXwO/pixiv-cli/pkg/pixiv"
```

显式凭据：

```go
client, err := pixiv.NewClient(pixiv.Options{AccessToken: accessToken})
if err != nil { /* handle */ }
result, err := client.SearchIllust(ctx, pixiv.SearchIllustRequest{Word: "初音ミク"})
```

复用本地账号、配置和代理：

```go
client, err := pixiv.OpenDefault(pixiv.Options{UserID: 12345678})
if err != nil { /* handle */ }
result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: 12345678})
```

`OpenDefault` 每个公开操作都会重新读取本地配置和认证状态；一个需要多次续页的一致操作可先调用 `Snapshot(ctx)`。SDK 返回版本化 opaque `Cursor`，调用方只透传，不能解析或持久化为上游 offset。失败以 `*pixiv.Error` 和 `errors.Is` 的稳定 sentinel 分类。详见 [SDK 接口](pixiv-sdk-interface.md)。

资源 URL 必须先经 `ParseResourceRef` 验证，再用 `OpenResource` 或 `Download`；`OpenResource` 返回未预读的 body，调用方负责关闭。该边界用于图片代理而非通用 URL fetch，防止 SSRF。

## MCP

```bash
PIXIV_REFRESH_TOKEN=... ./build/pixiv mcp
```

MCP 使用 stdio，不监听 HTTP 端口。当前 tool、参数和 structured output 见 [docs/mcp-tools.md](docs/mcp-tools.md)。列表 tool 使用 `page`/`limit`，不暴露 cursor；失败 result 设置 `isError=true`，同时保留安全文本与 structured output。

## 测试

```bash
go test ./...
go test -race ./...
go vet ./...
sh scripts/build.sh
```

真实 Pixiv Web fallback e2e 默认跳过；需要联网时显式执行：

```bash
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

不要提交 token、下载内容、本地数据库、缓存或机器配置。
