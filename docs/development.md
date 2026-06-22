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
go build -o pixiv ./cmd/pixiv-mcp-server
```

ugoira GIF 转换需要 `ffmpeg`：

```bash
ffmpeg -version
```

没有 `ffmpeg` 时，普通图片下载仍可工作；ugoira 转换会失败并返回明确错误。

## 运行

构建：

```bash
go build -o pixiv ./cmd/pixiv-mcp-server
```

CLI 运行：

```bash
pixiv account add main
pixiv search --json "初音ミク"
pixiv download 123456
```

MCP stdio 运行：

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./pixiv mcp
```

如网络环境需要代理，可额外设置：

```bash
https_proxy=http://127.0.0.1:7890 ./pixiv mcp
```

CLI 多账号 profile 保存在 `os.UserConfigDir()/pixiv/config.json`，文件权限为 `0600`。`account add` 默认从 stdin 读取 token，也支持 `--token`，但不建议在共享 shell 历史环境中使用。

## 获取 refresh token

浏览器 Cookie 里的 `PHPSESSID`、`device_token` 不是 Pixiv App API OAuth refresh token。推荐用项目内置的 Chromium 扩展走 Pixiv App OAuth PKCE 流程。

使用步骤：

1. 在 Chrome、Edge 或 Brave 打开 `chrome://extensions`。
2. 启用 Developer mode。
3. 点击 Load unpacked，选择 `scripts/pixiv-refresh-token-extension`。
4. 打开扩展弹窗，点击登录按钮。
5. 完成 Pixiv 登录后复制页面显示的 `refresh_token`。

扩展会监听 Pixiv OAuth callback 并自动换取 token。如果自动捕获失败，可在扩展弹窗里粘贴 callback URL、`pixiv://` URL 或原始 code 手动换取。扩展只把临时 PKCE verifier/state 写入 `chrome.storage.session`；refresh token 只展示给用户复制，不写入持久存储。

## 测试

当前测试覆盖配置加载、Pixiv 认证重试、下载管理和 MCP tool 注册：

```bash
go test ./...
go build -o pixiv ./cmd/pixiv-mcp-server
python3 scripts/test_pixiv_refresh_token_extension.py
```

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
