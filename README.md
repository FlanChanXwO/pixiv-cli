# Pixiv CLI / MCP Server

Go 版 Pixiv 工具集：默认作为 `pixiv` CLI 使用，需要 MCP 时显式运行 `pixiv mcp`。

它复用 Pixiv App API，支持搜索、详情、排行、推荐、下载、多账号 refresh token profile，以及 MCP stdio server。

## 构建

```bash
go build -o pixiv ./cmd/pixiv-mcp-server
```

## 获取 refresh token

`PIXIV_REFRESH_TOKEN` 必须是 Pixiv App API OAuth refresh token。网页 Cookie 里的 `PHPSESSID`、`device_token` 不能直接用。

推荐用 CLI 浏览器 OAuth 登录，并直接保存到本地 profile：

```bash
pixiv account login main
```

`account login` 流程：

| 阶段 | 行为 |
| --- | --- |
| 初始化 | CLI 生成 PKCE verifier/challenge 和 OAuth state，并启动本地 loopback HTTP server。 |
| 浏览器 | 默认打开系统默认浏览器；使用 `--no-open` 时只打印登录 URL 和本地页面地址。 |
| 回调 | 浏览器若没有自动返回，本地页面可粘贴 callback URL、`pixiv://...` URL 或原始 authorization code。 |
| 校验 | URL 派生的回调必须匹配本次 state；原始 code 是显式 fallback。 |
| 保存 | refresh/access token 不会打印；refresh token 保存到既有账号配置，文件权限为 `0600`。 |

真实登录依赖 Pixiv OAuth 网页流程可用；自动化测试使用 fake OAuth server，不访问真实 Pixiv。

## CLI 使用

先登录并保存一个账号：

```bash
pixiv account login main
```

高级/脚本场景也可以导入已有 token：

```bash
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv account add main
```

也可以直接传 token，但 `--token` 参数可能进入 shell history：

```bash
pixiv account add --token 'YOUR_REFRESH_TOKEN' main
```

常用命令：

```bash
pixiv account list
pixiv account use main
pixiv account check

pixiv search "初音ミク"
pixiv search --json "初音ミク"
pixiv detail 123456
pixiv ranking --mode day
pixiv recommended
pixiv download 123456 789012
```

账号配置保存到 `os.UserConfigDir()/pixiv/config.json`，文件权限固定为 `0600`。输出默认给人读；加 `--json` 输出机器可解析 JSON。
因为 CLI 使用 Go 标准库 `flag`，选项必须写在位置参数前面，例如 `pixiv search --json "初音ミク"`，不要写成 `pixiv search "初音ミク" --json`。

### CLI 命令表

| 命令 | 用法 | 说明 |
| --- | --- | --- |
| `account add` | `pixiv account add [--token TOKEN] NAME` | 添加或替换账号 profile；不传 `--token` 时从 stdin/终端读取。 |
| `account login` | `pixiv account login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] NAME` | 通过本地 loopback server 和浏览器 OAuth 登录并保存 profile；不会输出 refresh token。 |
| `account list` | `pixiv account list [--json]` | 列出本地 profile；不会输出 refresh token。 |
| `account use` | `pixiv account use NAME` | 设置默认 profile。 |
| `account remove` | `pixiv account remove NAME` | 删除 profile；若删除默认 profile，会自动选第一个剩余 profile。 |
| `account check` | `pixiv account check [--json] [NAME]` | 刷新 token 并验证账号；成功后会记录 `user_id`。 |
| `search` | `pixiv search [options] WORD` | 搜索插画。 |
| `detail` | `pixiv detail [options] ILLUST_ID` | 查看单个作品详情。 |
| `ranking` | `pixiv ranking [options]` | 查看 Pixiv 插画排行榜。 |
| `recommended` | `pixiv recommended [options]` | 查看个性化推荐，需要认证。 |
| `download` | `pixiv download [options] ILLUST_ID...` | 下载一个或多个作品，需要认证。 |
| `mcp` | `pixiv mcp` | 启动 MCP stdio server。 |

### `account login` 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--json` | `false` | 输出保存结果 JSON；不会输出 refresh/access token。 |
| `--no-open` | `false` | 不自动打开默认浏览器，只打印登录 URL 和本地 loopback 页面地址。 |
| `--addr` | `127.0.0.1:0` | 本地 loopback 监听地址；端口 `0` 表示自动分配。 |
| `--use` | `false` | 登录成功后设为默认 profile；若当前没有默认 profile，也会自动设为默认。 |
| `--timeout` | `0` | 等待登录完成的最大时长；`0` 表示不由 CLI 主动限时。 |

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
| `--profile NAME` | `search/detail/ranking/recommended/download` | 默认 profile | 选择本地账号 profile。 |
| `--refresh-token TOKEN` | `search/detail/ranking/recommended/download` | 空 | 临时覆盖 profile/env token。 |
| `--json` | `account` 子命令和数据命令 | `false` | 输出机器可解析 JSON。 |
| `--download-path PATH` | 数据命令；实际只影响 `download` | `DOWNLOAD_PATH` 或 `./downloads` | 下载目录。 |
| `--filename-template TEMPLATE` | 数据命令；实际只影响 `download` | `FILENAME_TEMPLATE` 或 `{author} - {title}_{id}` | 文件名模板。 |

### 环境变量

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | 空 | Pixiv App API OAuth refresh token；可被 profile 或 `--refresh-token` 覆盖。 |
| `DOWNLOAD_PATH` | `./downloads` | 下载目录。 |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | 文件名模板。 |
| `https_proxy` / `HTTPS_PROXY` | 空 | HTTP(S) 代理；优先使用小写 `https_proxy`。 |

配置优先级：命令行 flag > 选中的 profile > 环境变量 > 默认值。

## MCP 使用

MCP stdio server 需要显式启动：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./pixiv mcp
```

真实 token 写在 inline 环境变量里也可能进入 shell history；长期使用建议通过 MCP client 的私密环境配置或本地 profile 管理。

日志写入 stderr，stdout 保留给 MCP JSON-RPC。

MCP client 配置示例：

```json
{
  "mcpServers": {
    "pixiv-server": {
      "command": "/absolute/path/to/pixiv",
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

- `account add/login/list/remove/use/check`
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
go build -o pixiv ./cmd/pixiv-mcp-server
./pixiv --help
./pixiv mcp --help
```
