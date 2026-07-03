# 架构说明

## 总体流程

`cmd/pixiv/main.go` 是唯一官方二进制入口，它只负责调用 `internal/cli`：

1. `pixiv` 无参数显示 CLI 帮助。
2. `pixiv auth/config/search/detail/ranking/recommended/download` 进入 CLI 模式。
3. `pixiv mcp` 才创建并运行 `internal/mcpserver` MCP stdio server。
4. CLI 与 MCP 共享同一套运行时配置解析：
   - 账号认证来自 `os.UserConfigDir()/pixiv/auth.json`
   - 全局配置来自 `os.UserConfigDir()/pixiv/config.toml`
   - 公开环境变量作为覆盖层参与合并
5. MCP 模式若没有 `PIXIV_REFRESH_TOKEN`，会回退到 `auth.json.default_account`；若仍无 refresh token 且 `web_fallback_enabled=true`，支持匿名能力的路径会走 Pixiv web/ajax API。

## 包职责

### `cmd/pixiv`

负责生成 `pixiv` binary 的 `main` package。它不承载业务逻辑，只委托 `internal/cli.Run` 并返回进程退出码。

### `internal/cli`

负责 CLI 用户态的命令分发与输出：

- Cobra 命令树、help 和 flag 解析。
- 文本/JSON 输出。
- `auth login` 的 loopback OAuth、浏览器打开和 TTY 交互。
- `pixiv mcp` 分发。

### `internal/cli/state`

负责本地账号状态：

- `auth.json` 读写与默认账号管理。
- 认证文件路径解析和 `0600` 权限写入。

### `internal/config`

负责 `config.toml` 及运行时配置：

- `config.toml` schema、默认值和配置键定义。
- 运行时配置合并：`config.toml` 与公开环境变量。
- `pixiv config path/get/set/unset` 需要的强类型解析与稀疏写回。

### `internal/cli/mcpapp`

负责 `pixiv mcp` 的 runtime wiring：读取配置和账号、创建 Pixiv source、创建下载管理器，并把它们交给 `internal/mcpserver`。

配置拆分如下：

- `auth.json`：只保存 `default_account` 与 `accounts[]`，文件权限固定为 `0600`。
- `config.toml`：只保存用户显式设置过的全局配置键，文件权限固定为 `0600`。

运行时设置使用 `koanf` 合并 `config.toml` 与公开环境变量；`config set/unset` 使用 `tomledit` 写回，尽量保留注释、顺序和布局。

### `internal/pixiv`

Pixiv 领域 facade。对 CLI/MCP 暴露稳定的 `Source`、`NewSource`、`NewOAuthClient`、HTTP client wiring 和常用模型 type alias。

source 策略只有一条：refresh token 为空且 `web_fallback_enabled=true` 时，`search/detail/ranking/search_user/download/ugoira metadata` 使用 web；只要存在 refresh token，就优先 app API，app API 的认证、网络或服务端错误不会自动 fallback。

### `internal/pixiv/api`

封装 Pixiv app API、OAuth refresh flow 和 authorization-code token exchange。当前实现使用 `resty` 作为底层 HTTP transport，主要职责：

- 保存 refresh token、access token 和 user ID。
- 用 Pixiv Android app 风格 header 访问 API。
- 在认证错误时 refresh token 后只重放一次原请求。
- 将非 2xx 响应暴露为 `APIError`，保留状态码和响应体。

当前已实现接口包括搜索作品、作品详情、相关作品、排行榜、用户搜索、推荐、热门标签、关注动态、用户收藏、用户关注、ugoira metadata 和直接下载 URL。

### `internal/pixiv/web`

封装匿名 Pixiv web/ajax API。它复用 CLI/MCP 的 HTTP proxy 配置，当前用于无 refresh token fallback：

- `/ajax/search/artworks/{word}`：匿名作品搜索。
- `/ajax/illust/{id}` 与 `/ajax/illust/{id}/pages`：作品详情和原图 URL。
- `ranking.php?format=json`：排行榜。
- `/ajax/illust/{id}/ugoira_meta`：ugoira zip 与 frames。
- pximg 下载时使用 Pixiv web Referer。

web API 字段缺失时不伪造 App API 数据；仅映射可从 web 响应确认的字段。

### `internal/pixiv/model`

集中 Pixiv response/domain 类型以及 Pixiv 协议枚举 typed const，例如 search target、sort、ranking mode、restrict 和 illust type。MCP delivery 等传输层常量仍留在 `internal/mcpserver`。

### `internal/mcpserver`

负责将 Pixiv 与下载能力注册为 MCP tools。它定义了较窄的 `PixivAPI` 和 `DownloadManager` interface，便于测试和隔离。

输出目前以中文文本为主，适合直接返回给 LLM/MCP 客户端。认证相关工具会显式提示缺少 token、认证失败或自动认证失败的真实原因。

### `internal/download`

负责下载和本地文件落盘：

- `Download` 会同步下载 ID 列表，并返回每个作品的实际产物路径。
- `Enqueue` 会去重、排序并为每个 ID 启动后台任务。
- 内部 semaphore 当前并发为 5。
- 单页作品保存到下载目录。
- 多页作品和 ugoira 会建立作品子目录。
- ugoira 先下载 zip，再用 `ffmpeg` 合成为 GIF。

注意：ugoira 转换依赖本机 `ffmpeg`。初始化时会探测命令是否存在；缺失时 ugoira 下载会返回明确错误，普通图片下载不受影响。

### `internal/utils`

提供文件名清理、模板展开、ID 去重和 refresh token 输入规范化：

- 非法文件名字符替换为 `_`。
- 支持 `{author}`、`{title}`、`{id}` 模板字段。
- 多页作品追加 `_pN` 后缀。
- 下载 ID 去重时会丢弃小于等于 0 的 ID，并排序。
- refresh token 输入可从包含 `refresh_token=...` 的 Cookie 字符串中提取真实 token。

## 已知约束

- `internal/pixiv/api.Client` 默认 HTTP timeout 为 60 秒，`internal/pixiv` facade 创建带代理的 HTTP client 时也保留该客户端级保护。
- `pixiv mcp` 是 MCP stdio server 的显式启动方式；直接执行 `pixiv` 不会启动 MCP。
- CLI 账号文件以明文 JSON 保存 refresh token 和 user ID，不保存 access token，文件权限固定为 `0600`；需要系统钥匙串时再扩展。
- `config.toml` 采用稀疏写入，不会把默认值整份落盘。
- `download_random_from_recommendation` 默认下载 5 个，当前代码将输入数量限制在最多 20 个。
- `download` 默认只返回本地路径和 `file://` URI；当 `delivery=image_content` 时，会把所有下载产物作为 MCP `ImageContent` 一并返回，不做无依据截断。
- `get_thumbnail_base64` 会将缩略图完整编码为 base64 文本返回，调用方需注意输出体积。
- 匿名 `search_user` fallback 语义是“作品搜索结果中的相关作者去重”，不是 Pixiv 官方用户名搜索。
