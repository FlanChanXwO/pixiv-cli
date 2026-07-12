# Pixiv CLI / MCP Server

Go 版 Pixiv 工具集：默认作为 `pixiv` CLI 使用，需要 MCP 时显式运行 `pixiv mcp`。

它优先复用 Pixiv App API，支持搜索、详情、排行、推荐、下载、多账号 refresh token 管理，以及 MCP stdio server。未配置 refresh token 时，默认对搜索、详情、排行、用户搜索和下载启用匿名 Pixiv web/ajax API fallback。

源码按 CLI controller、application services、bootstrap、config、Pixiv facade/source、download、MCP server 分包；账号存储在 `internal/storage/auth`，基础工具按 `internal/utils/*` 子包组织，基础设施常量限制在 `internal/common/constants`。Pixiv App API、web fallback 与共享模型分别收在 `internal/pixiv/api`、`internal/pixiv/web`、`internal/pixiv/model`。

用户可感知变化记录在 [CHANGELOG.md](CHANGELOG.md)。

## 构建

```bash
sh scripts/build.sh
```

默认输出到当前平台的 `build/pixiv` 或 `build/pixiv.exe`。Windows 通过 Git Bash、MSYS2 或 WSL 运行；需要交叉构建时继续直接使用 `go build`。

或直接安装命令入口：

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@latest
```

## 获取 refresh token

`PIXIV_REFRESH_TOKEN` 必须是 Pixiv App API OAuth refresh token。网页 Cookie 里的 `PHPSESSID`、`device_token` 不能直接用。

推荐用 CLI 浏览器 OAuth 登录，并直接保存到本地账号：

```bash
pixiv auth login
```

`auth login` 流程：

| 阶段 | 行为 |
| --- | --- |
| 初始化 | CLI 生成 PKCE verifier/challenge 和 OAuth state，并启动本地 loopback HTTP server。 |
| 浏览器 | macOS 默认优先注册本地 `pixiv://` callback helper 并打开默认浏览器，因此可复用已有 Pixiv 登录态；需要用户在 Pixiv 页面确认账号；使用 `--no-open` 时只打印登录 URL 和本地页面地址。 |
| 回调 | CLI 通过 `pixiv://` helper、浏览器 URL/session 只读观察或 DevTools fallback 捕获本轮 `pixiv://account/login`/官方 callback 请求，并继续监听本地 callback、终端粘贴和本地页面表单；浏览器若没有自动返回，可粘贴 callback URL、`pixiv://...` URL、Pixiv relay URL 或原始 authorization code。 |
| 校验 | 本地 loopback 回调必须匹配本次 state；Pixiv 官方 callback URL 与 `pixiv://account/login` 可在 Pixiv 未返回 state 时作为显式 fallback。 |
| 保存 | refresh/access token 不会打印；refresh token 按 Pixiv UID 保存到 `auth.json`，文件权限为 `0600`。 |

默认浏览器打开时，macOS 会优先安装/注册一个本地 `PixivCLIURLHandler.app`，只把 Pixiv 返回的 `pixiv://account/login?...` URL 转交给本轮 CLI loopback，不读取 cookie、token 或浏览器存储。若本机无法注册该 helper，CLI 才退回专用 Chromium/Edge 用户资料目录并通过 DevTools 只监听 Pixiv OAuth 请求 URL；该 fallback 不安装扩展、不点击页面、不读取 cookie 或 token。macOS 仍保留 Microsoft Edge、Chrome、Chromium 与 Safari 标签页/浏览器状态文件的只读观察；遇到 Pixiv `post-redirect` 授权接力页时，会校验其 `return_to` 属于本轮 OAuth，然后等待 Pixiv 触发 `pixiv://` handoff，不再自动重开白页。浏览器可能停留在白色 relay 页，是否成功以终端最终输出为准。若手动粘贴 Pixiv relay URL，CLI 会打开该 relay URL 一次。若 Pixiv 未生成 callback，CLI 不会伪造成功，仍可使用终端或本地页面手动回填。

浏览器使用的系统代理不会自动传给 Go CLI。若 Pixiv token 端点在当前网络下需要代理，请先配置：

```bash
pixiv config set https_proxy http://127.0.0.1:7890
```

也可以只给本次网络命令临时覆盖代理：

```bash
pixiv auth login --proxy http://127.0.0.1:7890
```

`--proxy URL` 只影响当前网络命令，不写入 `config.toml`；运行期代理优先级为 `--proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`。

真实登录依赖 Pixiv OAuth 网页流程可用；自动化测试使用 fake OAuth server，不访问真实 Pixiv。

## CLI 使用

先登录并保存一个账号：

```bash
pixiv auth login
```

高级/脚本场景也可以导入已有 token：

```bash
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth add
```

也可以直接传 token，但 `--token` 参数可能进入 shell history：

```bash
pixiv auth add --token 'YOUR_REFRESH_TOKEN'
```

常用命令：

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
pixiv config path
pixiv config get download_path
pixiv config set download_path ~/Downloads/pixiv
pixiv config unset https_proxy

pixiv search "初音ミク"
pixiv search "初音ミク" --json
pixiv detail 123456
pixiv ranking --mode day
pixiv recommended
pixiv download 123456 789012
```

账号认证保存到 `os.UserConfigDir()/pixiv/auth.json`，账号 key 是 Pixiv UID；全局配置保存到 `os.UserConfigDir()/pixiv/config.toml`，两个文件权限都固定为 `0600`。输出默认给人读；加 `--json` 输出机器可解析 JSON。
CLI 使用 Cobra/pflag，选项可以写在位置参数前后，例如 `pixiv auth check 12345678 --json` 和 `pixiv search "初音ミク" --json` 都是正式支持的写法。

### CLI 命令表

| 命令 | 用法 | 说明 |
| --- | --- | --- |
| `auth add` | `pixiv auth add [--token TOKEN] [--json] [--proxy URL]` | 校验 refresh token 或包含 `refresh_token=...` 的 Cookie，并按 Pixiv UID 添加或替换账号；不传 `--token` 时从 TTY/stdin 读取。 |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--proxy URL]` | 通过本地 loopback server 和浏览器 OAuth 登录，按 Pixiv UID 保存账号；不会输出 refresh token。 |
| `auth list` | `pixiv auth list [--json]` | 列出本地账号；不会输出 refresh token。 |
| `auth use` | `pixiv auth use [UID]` | 设置默认账号；TTY 下可交互选择。 |
| `auth remove` | `pixiv auth remove [UID] [--yes]` | 删除账号；TTY 下默认确认，删除默认账号后会自动选第一个剩余账号。 |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL]` | 刷新 token 并验证账号；成功后会记录 `user_id` 和可获取到的 username。 |
| `config path` | `pixiv config path` | 输出 `config.toml` 路径。 |
| `config get` | `pixiv config get KEY` | 输出一个生效中的配置值。 |
| `config set` | `pixiv config set KEY VALUE` | 写入一个已知配置键到 `config.toml`。 |
| `config unset` | `pixiv config unset KEY` | 从 `config.toml` 删除一个已知配置键。 |
| `search` | `pixiv search [options] WORD` | 搜索插画。 |
| `detail` | `pixiv detail [options] ILLUST_ID` | 查看单个作品详情。 |
| `ranking` | `pixiv ranking [options]` | 查看 Pixiv 插画排行榜。 |
| `recommended` | `pixiv recommended [options]` | 查看个性化推荐，需要认证。 |
| `download` | `pixiv download [options] ILLUST_ID...` | 下载一个或多个作品；无 token 时默认走匿名 web fallback。 |
| `mcp` | `pixiv mcp [--proxy URL]` | 启动 MCP stdio server；代理覆盖只在本次启动时生效。 |

### `auth login` 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--json` | `false` | 输出保存结果 JSON；不会输出 refresh/access token。 |
| `--no-open` | `false` | 不自动打开 managed/browser，也不观察浏览器 URL；只打印登录 URL 和本地 loopback 页面地址。 |
| `--addr` | `127.0.0.1:0` | 本地 loopback 监听地址；端口 `0` 表示自动分配。 |
| `--use` | `false` | 登录成功后设为默认账号；若当前没有默认账号，也会自动设为默认。 |
| `--timeout` | `0` | 等待登录完成的最大时长；`0` 表示不由 CLI 主动限时。 |
| `--proxy URL` | 空 | 本次 token exchange 代理覆盖；不会保存到 `config.toml`。 |

### 数据命令参数

| 命令 | 参数 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `search` | `--search-target` | `partial_match_for_tags` | 搜索范围。 |
| `search` | `--sort` | `date_desc` | 排序方式。 |
| `search` | `--duration` | 空 | Pixiv API 的时间范围参数。 |
| `search` | `--offset` | `0` | 分页偏移。 |
| `search` | `--r18` | `false` | 在搜索词后追加 `R-18`。 |
| `ranking` | `--mode` | `day` | 排行榜模式。 |
| `ranking` | `--date` | 空 | 排行榜日期，格式通常为 `YYYY-MM-DD`。 |
| `ranking` | `--offset` | `0` | 分页偏移。 |
| `recommended` | `--offset` | `0` | 分页偏移。 |
| `detail` | `ILLUST_ID` | 必填 | Pixiv 作品 ID。 |
| `download` | `ILLUST_ID...` | 必填 | 一个或多个 Pixiv 作品 ID。 |

### 通用参数

| 参数 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--uid UID` | `search/detail/ranking/recommended/download` | `auth.json.default_user_id` | 选择本地账号。 |
| `--profile UID` | `search/detail/ranking/recommended/download` | 空 | `--uid` 的 deprecated alias。 |
| `--refresh-token TOKEN` | `search/detail/ranking/recommended/download` | 空 | 临时覆盖账号/env token。 |
| `--json` | `auth` 子命令和数据命令 | `false` | 输出机器可解析 JSON。 |
| `--download-path PATH` | 数据命令；实际只影响 `download` | `DOWNLOAD_PATH`、`config.toml` 或 `./downloads` | 下载目录。 |
| `--filename-template TEMPLATE` | 数据命令；实际只影响 `download` | `FILENAME_TEMPLATE`、`config.toml` 或 `{author} - {title}_{id}` | 文件名模板。 |
| `--proxy URL` | `auth add/login/check`、数据命令、`mcp` | `https_proxy`/`HTTPS_PROXY`、`config.toml` 或空 | 临时使用 HTTP(S) 代理；只影响当前命令。 |

### `config` 支持的键

| KEY | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `download_path` | string | `./downloads` | 下载目录。 |
| `filename_template` | string | `{author} - {title}_{id}` | 文件名模板。 |
| `https_proxy` | string | 空 | HTTP(S) 代理，优先使用环境变量中的小写 `https_proxy`。 |
| `web_fallback_enabled` | bool | `true` | 无 refresh token 时，允许匿名 Pixiv web/ajax API fallback；写入为 `[web] fallback_enabled = true/false`。 |
| `output_json` | bool | `false` | 数据命令默认输出 JSON。 |
| `login_open_browser` | bool | `true` | `auth login` 默认是否自动打开浏览器。 |
| `login_timeout` | duration | `0s` | `auth login` 默认等待时长。 |
| `login_use_after_login` | bool | `false` | `auth login` 默认是否设为当前默认账号。 |

### 环境变量

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | 空 | Pixiv App API OAuth refresh token；可被账号选择或 `--refresh-token` 覆盖。 |
| `DOWNLOAD_PATH` | `./downloads` | 下载目录。 |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | 文件名模板。 |
| `https_proxy` / `HTTPS_PROXY` | 空 | HTTP(S) 代理；优先使用小写 `https_proxy`。 |

认证优先级：`--refresh-token` > `--uid`/deprecated `--profile` > `PIXIV_REFRESH_TOKEN` > `auth.json.default_user_id`。

设置类字段优先级：命令行 flag > 环境变量 > `config.toml` > 默认值。代理的运行期优先级为 `--proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`，且命令行覆盖不会持久化。

### 匿名 web fallback

当 `--refresh-token`、`PIXIV_REFRESH_TOKEN` 和默认账号都没有提供 refresh token，且 `web_fallback_enabled=true` 时，下列能力自动走 Pixiv web/ajax API：`search`、`detail`、`ranking`、`download`，以及 MCP tools `search_illust`、`illust_detail`、`illust_ranking`、`search_user`、`download`、`get_thumbnail_base64`。

有 refresh token 时仍优先使用 App API；token 无效、App API 网络错误或服务端错误不会自动 fallback，会直接暴露真实错误。

匿名 fallback 的差异：

- `search_user` 不是 Pixiv 官方用户搜索；它通过 web 作品搜索结果按 `userId` 去重，返回“相关作品作者”。
- 静态单页/多页下载使用 `/ajax/illust/{id}/pages` 的 `original` URL。
- ugoira 下载使用 `/ajax/illust/{id}/ugoira_meta` 的 `originalSrc` zip 和 frames，并继续依赖本机 `ffmpeg` 转 GIF。
- web fallback 不新增专用代理环境变量，继续使用 `--proxy`、`https_proxy` / `HTTPS_PROXY` 或 `pixiv config set https_proxy ...`。

关闭方式：

```bash
pixiv config set web_fallback_enabled false
```

## MCP 使用

MCP stdio server 需要显式启动：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./build/pixiv mcp
```

MCP 的代理覆盖是启动期设置：

```bash
./build/pixiv mcp --proxy http://127.0.0.1:7890
```

未设置 `PIXIV_REFRESH_TOKEN` 时，`pixiv mcp` 会先回退到 `auth.json.default_user_id`；如果仍没有 refresh token 且 `web_fallback_enabled=true`，支持匿名 fallback 的 MCP tools 会直接使用 Pixiv web/ajax API。真实 token 写在 inline 环境变量里也可能进入 shell history；长期使用建议通过 MCP client 的私密环境配置或本地账号管理。

日志写入 stderr，stdout 保留给 MCP JSON-RPC。

MCP client 配置示例：

```json
{
  "mcpServers": {
    "pixiv-server": {
      "command": "/absolute/path/to/pixiv-cli/build/pixiv",
      "args": ["mcp"],
      "env": {
        "PIXIV_REFRESH_TOKEN": "your refresh token or cookie with refresh_token=...",
        "DOWNLOAD_PATH": "./downloads",
        "FILENAME_TEMPLATE": "{author} - {title}_{id}"
      }
    }
  }
}
```

## 命令概览

CLI 命令：

- `auth add/login/list/remove/use/check`
- `config path/get/set/unset`
- `search`
- `detail`
- `ranking`
- `recommended`
- `download`
- `mcp`

MCP tools：

`set_download_path`, `download`, `refresh_token`, `set_refresh_token`,
`download_random_from_recommendation`, `search_illust`, `illust_detail`,
`illust_related`, `illust_ranking`, `search_user`, `illust_recommended`,
`trending_tags_illust`, `illust_follow`, `user_bookmarks`, `user_following`,
and `get_thumbnail_base64`.

## 开发验证

```bash
go test ./...
sh scripts/build.sh
./build/pixiv --help
./build/pixiv mcp --help
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

真实 Pixiv web fallback e2e 默认跳过；只有设置 `PIXIV_E2E_WEB_API=1` 时才会联网。
