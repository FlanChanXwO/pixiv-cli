# Pixiv CLI / MCP Server

Go 版 Pixiv 工具集：默认作为 `pixiv` CLI 使用，需要 MCP 时显式运行 `pixiv mcp`。

它复用 Pixiv App API，支持搜索、详情、排行、推荐、下载、多账号 refresh token profile，以及 MCP stdio server。

## 构建

```bash
go build -o pixiv ./cmd/pixiv-mcp-server
```

## 获取 refresh token

`PIXIV_REFRESH_TOKEN` 必须是 Pixiv App API OAuth refresh token。网页 Cookie 里的 `PHPSESSID`、`device_token` 不能直接用。

推荐使用内置 Chromium 扩展导出：

1. 打开 Chrome、Edge 或 Brave 的 `chrome://extensions`。
2. 启用 Developer mode。
3. Load unpacked，选择 `scripts/pixiv-refresh-token-extension`。
4. 打开扩展弹窗并登录 Pixiv。
5. 复制扩展显示的 `refresh_token`。

如果自动捕获失败，可在扩展弹窗中粘贴 callback URL、`pixiv://` URL 或原始 code 手动换取。

## CLI 使用

先保存一个账号：

```bash
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv account add main
```

也可以直接传 token，但会进入 shell history：

```bash
pixiv account add main --token 'YOUR_REFRESH_TOKEN'
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

常用选项：

- `--profile NAME`：选择账号 profile。
- `--refresh-token TOKEN`：覆盖 profile/env token。
- `--download-path PATH`：覆盖下载目录。
- `--filename-template TEMPLATE`：覆盖文件名模板。
- `--json`：输出 JSON。

配置优先级：命令行 flag > 选中的 profile > 环境变量 > 默认值。

## MCP 使用

MCP stdio server 需要显式启动：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./pixiv mcp
```

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

- `account add/list/remove/use/check`
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
