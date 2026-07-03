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
go build -o pixiv ./cmd/pixiv
```

ugoira GIF 转换需要 `ffmpeg`：

```bash
ffmpeg -version
```

没有 `ffmpeg` 时，普通图片下载仍可工作；ugoira 转换会失败并返回明确错误。

## 运行

构建：

```bash
go build -o pixiv ./cmd/pixiv
```

CLI 运行：

```bash
pixiv auth login main
pixiv search "初音ミク" --json
pixiv download 123456
```

MCP stdio 运行：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./pixiv mcp
```

真实 token 写在 inline 环境变量里也可能进入 shell history；长期使用建议通过 MCP client 的私密环境配置或本地账号管理。

如网络环境需要代理，可额外设置：

```bash
https_proxy=http://127.0.0.1:7890 ./pixiv mcp
```

CLI 多账号认证保存在 `os.UserConfigDir()/pixiv/auth.json`，全局配置保存在 `os.UserConfigDir()/pixiv/config.toml`，两个文件权限都为 `0600`。推荐使用 `pixiv auth login NAME` 通过本地 loopback server 和浏览器 OAuth 登录；`auth add` 仍可从 stdin 读取 token，也支持 `--token`，但不建议在共享 shell 历史环境中使用。可用 `pixiv config path/get/set/unset` 管理全局配置。无 refresh token 时默认启用匿名 Pixiv web/ajax API fallback，可用 `pixiv config set web_fallback_enabled false` 关闭。
CLI 使用 Cobra/pflag，flag 可以写在位置参数前后；例如 `pixiv auth check main --json` 和 `pixiv search "初音ミク" --json` 都受支持。

## 获取 refresh token

浏览器 Cookie 里的 `PHPSESSID`、`device_token` 不是 Pixiv App API OAuth refresh token。推荐直接登录并保存账号：

```bash
pixiv auth login main
```

| 项 | 说明 |
| --- | --- |
| 本地服务 | CLI 生成 PKCE/state，并启动本地 loopback HTTP server。 |
| 浏览器 | 默认打开系统默认浏览器；`--no-open` 可改为只打印登录 URL。 |
| 手动回填 | 若浏览器没有自动返回，本地页面可粘贴 callback URL、`pixiv://...` URL 或原始 code。 |
| state 校验 | URL 派生回调必须匹配本次 state；原始 code 是显式 fallback。 |
| token 保存 | refresh/access token 不打印；refresh token 写入 `auth.json`，权限为 `0600`。 |

真实登录依赖 Pixiv OAuth 网页流程可用。自动化测试使用 fake OAuth server 覆盖 callback 和 token exchange，不访问真实 Pixiv。

## 测试

当前测试覆盖 CLI 命令、`internal/application` 应用用例、`internal/config` 配置、`internal/storage/auth` 认证存储、Pixiv App API 认证重试、Pixiv facade/source、web fallback、HTTP client wiring、下载管理和 MCP tool 注册：

```bash
go test ./...
go build -o pixiv ./cmd/pixiv
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

`go test ./...` 保持默认离线稳定；真实 Pixiv web API fallback e2e 默认跳过，只有设置 `PIXIV_E2E_WEB_API=1` 时才会联网。未设置 `PIXIV_WEB_API_PROXY` 时会直连。

代码改动完成前，应按变更范围补充或更新测试。若不能运行测试，需要在交付说明中写明原因和风险。

## Git 与本地产物

`.gitignore` 已排除：

- `.DS_Store`
- 构建产物 `pixiv`、`pixiv-mcp-server`
- 本地下载目录 `downloads/`
- 本地数据库 `*.db`
- 常见缓存、日志、临时文件

不要提交 Pixiv token、下载内容、本地数据库或机器相关配置。

## 文档同步

当以下内容变化时，同步更新 `docs/` 或 `README.md`：

- MCP tools、参数或返回语义。
- CLI 命令、参数、账号配置或输出语义。
- 环境变量或默认值。
- 下载、认证、代理、ugoira 等流程。
- 新增限制、重试、超时、截断、降级或错误处理策略。
- 测试或构建命令。
