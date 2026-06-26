# 架构说明

## 总体流程

`cmd/pixiv-mcp-server/main.go` 是二进制入口，实际委托给 `internal/cli`：

1. `pixiv` 无参数显示 CLI 帮助。
2. `pixiv account/search/detail/ranking/recommended/download` 进入 CLI 模式。
3. `pixiv mcp` 才创建并运行 `internal/mcpserver` MCP stdio server。
4. MCP 模式从环境变量加载配置，创建 HTTP client，并在 `https_proxy` 或 `HTTPS_PROXY` 存在时配置代理。
5. MCP 模式创建 `internal/pixiv.Client`、`internal/download.Manager` 和 MCP server。
6. MCP 模式若启动时已有 `PIXIV_REFRESH_TOKEN`，先尝试自动认证；失败只记录 warning，server 仍会启动，后续可通过 MCP tool 手动设置或刷新 token。

## 包职责

### `internal/pixiv`

封装 Pixiv app API、OAuth refresh flow 和 authorization-code token exchange。主要职责：

- 保存 refresh token、access token 和 user ID。
- 用 Pixiv Android app 风格 header 访问 API。
- 在认证错误时尝试 refresh token 后重试一次请求。
- 将非 2xx 响应暴露为 `APIError`，保留状态码和响应体。

当前已实现接口包括搜索作品、作品详情、相关作品、排行榜、用户搜索、推荐、热门标签、关注动态、用户收藏、用户关注、ugoira metadata 和直接下载 URL。

### `internal/mcpserver`

负责将 Pixiv 与下载能力注册为 MCP tools。它定义了较窄的 `PixivAPI` 和 `DownloadManager` interface，便于测试和隔离。

输出目前以中文文本为主，适合直接返回给 LLM/MCP 客户端。认证相关工具会显式提示缺少 token、认证失败或自动认证失败的真实原因。

### `internal/cli`

负责命令行入口、参数解析、账号 profile 存储、文本/JSON 输出和 `pixiv mcp` 分发。CLI 使用 Go 标准库 `flag`，不引入额外 CLI 框架。

账号文件位于 `os.UserConfigDir()/pixiv/config.json`，写入权限为 `0600`。命令行配置优先级为 flag、选中的 profile、环境变量、默认值。

`pixiv account login NAME` 是推荐的 refresh token 获取与保存方式。CLI 会生成 PKCE verifier/challenge 和 OAuth state，启动本地 loopback HTTP server，并默认打开系统浏览器；使用 `--no-open` 时只打印登录 URL。URL 派生回调必须匹配 state，原始 code 粘贴仅作为显式 fallback。登录成功后只保存 refresh token 和 user ID，不打印 refresh/access token。

### `internal/download`

负责下载和本地文件落盘：

- `Download` 会同步下载 ID 列表，并返回每个作品的实际产物路径。
- `Enqueue` 会去重、排序并为每个 ID 启动后台任务。
- 内部 semaphore 当前并发为 5。
- 单页作品保存到下载目录。
- 多页作品和 ugoira 会建立作品子目录。
- ugoira 先下载 zip，再用 `ffmpeg` 合成为 GIF。

注意：ugoira 转换依赖本机 `ffmpeg`。初始化时会探测命令是否存在；缺失时 ugoira 下载会返回明确错误，普通图片下载不受影响。

### `pkg/config`

从环境变量读取：

- `PIXIV_REFRESH_TOKEN`
- `DOWNLOAD_PATH`
- `FILENAME_TEMPLATE`
- `https_proxy` / `HTTPS_PROXY`

`PIXIV_REFRESH_TOKEN` 会先规范化：原始 refresh token 原样使用；如果输入是 Cookie 字符串且包含 `refresh_token=...`，则提取该值。只有 `PHPSESSID`、`device_token` 等网页 Cookie 不能代替 App API OAuth refresh token。

默认下载目录是 `./downloads`，默认文件名模板是 `{author} - {title}_{id}`。

### `pkg/pixivutil`

提供文件名清理、模板展开、ID 去重和 refresh token 输入规范化：

- 非法文件名字符替换为 `_`。
- 支持 `{author}`、`{title}`、`{id}` 模板字段。
- 多页作品追加 `_pN` 后缀。
- 下载 ID 去重时会丢弃小于等于 0 的 ID，并排序。
- refresh token 输入可从包含 `refresh_token=...` 的 Cookie 字符串中提取真实 token。

## 已知约束

- `internal/pixiv.Client` 默认 HTTP timeout 为 60 秒，这是当前代码已有的客户端级保护。
- `pixiv mcp` 是 MCP stdio server 的显式启动方式；直接执行 `pixiv` 不再启动 MCP。
- CLI 账号文件以明文 JSON 保存 refresh token 和 user ID，不保存 access token，文件权限固定为 `0600`；需要系统钥匙串时再扩展。
- `account login` 的真实登录依赖 Pixiv OAuth 网页流程；测试使用 fake OAuth server 覆盖 callback 与 token exchange。
- `download_random_from_recommendation` 默认下载 5 个，当前代码将输入数量限制在最多 20 个。
- `download` 默认只返回本地路径和 `file://` URI；当 `delivery=image_content` 时，会把所有下载产物作为 MCP `ImageContent` 一并返回，不做无依据截断。
- `get_thumbnail_base64` 会将缩略图完整编码为 base64 文本返回，调用方需注意输出体积。
- `rsshub.db` 位于当前工作区根目录，但不是代码路径的一部分，已按本地数据处理。
