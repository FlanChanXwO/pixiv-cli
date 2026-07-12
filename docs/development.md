# 开发流程

## 环境检查

项目是 Go module，当前 `go.mod` 声明：

```text
go 1.26.3
```

开工前建议检查：

```bash
go version
go test ./...
sh scripts/build.sh
```

ugoira GIF 转换需要 `ffmpeg`：

```bash
ffmpeg -version
```

没有 `ffmpeg` 时，普通图片下载仍可工作；ugoira 转换会失败并返回明确错误。

## 运行

构建：

```bash
sh scripts/build.sh
```

默认输出到当前平台的 `build/pixiv` 或 `build/pixiv.exe`。Windows 通过 Git Bash、MSYS2 或 WSL 运行；需要交叉构建时继续直接使用 `go build`。

CLI 运行：

```bash
pixiv auth login
pixiv search "初音ミク" --json
pixiv download 123456
```

MCP stdio 运行：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./build/pixiv mcp
```

真实 token 写在 inline 环境变量里也可能进入 shell history；长期使用建议通过 MCP client 的私密环境配置或本地账号管理。

如网络环境需要代理，可额外设置：

```bash
https_proxy=http://127.0.0.1:7890 ./build/pixiv mcp
```

或只给本次启动覆盖代理：

```bash
./build/pixiv mcp --proxy http://127.0.0.1:7890
```

CLI 多账号认证保存在 `os.UserConfigDir()/pixiv/auth.json`，账号 key 是 Pixiv UID；全局配置保存在 `os.UserConfigDir()/pixiv/config.toml`，两个文件权限都为 `0600`。推荐使用 `pixiv auth login` 通过本地 loopback server 和浏览器 OAuth 登录；`auth add` 仍可从 stdin 读取 token，也支持 `--token`，但不建议在共享 shell 历史环境中使用。可用 `pixiv config path/get/set/unset` 管理全局配置。无 refresh token 时默认启用匿名 Pixiv web/ajax API fallback，可用 `pixiv config set web_fallback_enabled false` 关闭。
CLI 使用 Cobra/pflag，flag 可以写在位置参数前后；例如 `pixiv auth check 12345678 --json` 和 `pixiv search "初音ミク" --json` 都受支持。

## 获取 refresh token

浏览器 Cookie 里的 `PHPSESSID`、`device_token` 不是 Pixiv App API OAuth refresh token。推荐直接登录并保存账号：

```bash
pixiv auth login
```

| 项 | 说明 |
| --- | --- |
| 本地服务 | CLI 生成 PKCE/state，并启动本地 loopback HTTP server。 |
| 浏览器 | macOS 默认优先注册本地 `pixiv://` callback helper 并打开默认浏览器，因此可复用已有 Pixiv 登录态；需要用户在 Pixiv 页面确认账号；`--no-open` 可改为只打印登录 URL。 |
| 自动/手动回填 | CLI 默认通过 `pixiv://` helper、浏览器 URL/session 只读观察或 DevTools fallback 捕获 `pixiv://account/login`/官方 callback 请求，并保留终端粘贴兜底；若浏览器没有自动返回，也可在本地页面粘贴 callback URL、`pixiv://...` URL、Pixiv relay URL 或原始 code。 |
| state 校验 | 本地 loopback 回调必须匹配本次 state；Pixiv 官方 callback URL 与 `pixiv://account/login` 可在 Pixiv 未返回 state 时作为显式 fallback。 |
| token 保存 | refresh/access token 不打印；refresh token 按 Pixiv UID 写入 `auth.json`，权限为 `0600`。 |

默认浏览器打开时，macOS 会优先安装/注册一个本地 `PixivCLIURLHandler.app`，只把 Pixiv 返回的 `pixiv://account/login?...` URL 转交给本轮 CLI loopback，不读取 cookie、token 或浏览器存储。若本机无法注册该 helper，CLI 才退回专用 Chromium/Edge 用户资料目录并通过 DevTools 只监听 Pixiv OAuth 请求 URL；该 fallback 不安装扩展、不点击页面、不读取 cookie 或 token。macOS 的浏览器 URL 观察仍支持 Microsoft Edge、Chrome、Chromium 与 Safari，会读取浏览器标签页 URL，并扫描 Chromium 系浏览器的 session/history 状态文件；遇到 Pixiv `post-redirect` 授权接力页时会校验其 `return_to` 属于本轮 OAuth，然后等待 Pixiv 触发 `pixiv://` handoff，不再自动重开白页。浏览器可能停留在白色 relay 页，是否成功以终端最终输出为准。若手动粘贴 Pixiv relay URL，CLI 会打开该 relay URL 一次。状态不可读或 Pixiv 未生成 callback 时不会隐藏失败或假装登录成功，用户仍可用终端 prompt 或本地页面手动回填授权码。

浏览器使用的系统代理不会自动传给 Go CLI。若 Pixiv token exchange 需要代理，请配置 `pixiv config set https_proxy http://127.0.0.1:7890`，在单次命令前设置 `https_proxy=...`，或对网络命令使用运行期覆盖 `--proxy http://127.0.0.1:7890`。运行期代理优先级为 `--proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`；`--proxy` 不会写入 `config.toml`。

当前支持 `--proxy` 代理覆盖的网络入口是 `auth add`、`auth login`、`auth check`、`search`、`detail`、`ranking`、`recommended`、`download` 和 `mcp` 启动。`auth list/use/remove` 与 `config path/get/set/unset` 不接受该 flag。

真实登录依赖 Pixiv OAuth 网页流程可用。自动化测试使用 fake OAuth server 覆盖 callback 和 token exchange，不访问真实 Pixiv。

## 测试

当前测试覆盖 CLI 命令、`internal/application` 应用用例、`internal/config` 配置、`internal/storage/auth` 认证存储、Pixiv App API 认证重试、Pixiv facade/source、web fallback、HTTP client wiring、下载管理和 MCP tool 注册：

```bash
go test ./...
sh scripts/build.sh
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

`go test ./...` 保持默认离线稳定；真实 Pixiv web API fallback e2e 默认跳过，只有设置 `PIXIV_E2E_WEB_API=1` 时才会联网。未设置 `PIXIV_WEB_API_PROXY` 时会直连。

代码改动完成前，应按变更范围补充或更新测试。若不能运行测试，需要在交付说明中写明原因和风险。

## Git 与本地产物

`.gitignore` 已排除：

- `.DS_Store`
- 构建产物 `build/`、`pixiv`、`pixiv-cli`
- 本地下载目录 `downloads/`
- 本地数据库 `*.db`
- 常见缓存、日志、临时文件

不要提交 Pixiv token、下载内容、本地数据库或机器相关配置。

## Changelog

`CHANGELOG.md` 使用 Keep a Changelog 1.1.0 风格维护。未发布改动先写入 `[Unreleased]`，等正式切版本时再移动到对应版本段。

需要记录的改动：

- 用户可见的新功能、行为变化或 bug 修复。
- 配置、CLI、MCP tool、输出格式或兼容性变化。
- 废弃、移除、安全影响和迁移说明。

不强制记录纯内部重构、测试补充、文档清理和不会影响用户/集成方的工程整理。

## 文档同步

当以下内容变化时，同步更新 `docs/` 或 `README.md`：

- MCP tools、参数或返回语义。
- CLI 命令、参数、账号配置或输出语义。
- 环境变量或默认值。
- 下载、认证、代理、ugoira 等流程。
- 新增限制、重试、超时、截断、降级或错误处理策略。
- 测试或构建命令。
