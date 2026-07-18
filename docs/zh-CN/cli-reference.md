# Pixiv CLI 参考手册

[English](../en/cli-reference.md) | 简体中文 | [日本語](../ja/cli-reference.md) | [项目首页](../../README.zh-CN.md)

本文是 `pixiv` 命令的完整契约：安装、认证、命令、flag、配置、环境变量、匿名 fallback 和更新。
SDK 与 MCP 细节不在此重复，入口见[相关文档](#相关文档)。

用户可感知变化记录在 [CHANGELOG.md](../../CHANGELOG.md)。

## 安装与构建

> **发布状态**：受支持 binary 的 Ed25519 公钥、key ID 与 fingerprint 已提交到
> [`internal/bootstrap/release_trust.go`](../../internal/bootstrap/release_trust.go)；公开 source/tap repositories、
> 受保护 `release` Environment 与隔离 credentials 已配置。v0.4.0 已作为正式 GitHub Release 发布，包含六个
> 平台 archive、checksum 与签名清单；stable Homebrew formula 已推送。后续版本仍必须通过同一套 tag、签名、资产与
> Homebrew 门禁后才可作为可用下载来源。

### 官方安装脚本

仓库提供两个面向最新 stable Release 的用户级 bootstrap 脚本：

```bash
# Linux/macOS
curl -fsSLo /tmp/pixiv-install.sh https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.sh
sh /tmp/pixiv-install.sh --add-to-path
```

```bat
rem Windows 命令提示符；不依赖 PowerShell
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd
call "%TEMP%\pixiv-install.cmd" --add-to-path
```

`install.sh` 支持 Linux/macOS AMD64 与 ARM64，默认安装到 `$HOME/.local/bin`；`install.cmd` 支持 Windows
AMD64 与 ARM64，默认安装到 `%LOCALAPPDATA%\Programs\pixiv`。两个脚本都只从官方最新 stable Release 下载
`checksums.txt` 与唯一匹配的 archive，先校验 SHA-256，再解压并预检暂存 binary，最后才替换 `pixiv`。
`--install-dir DIR` 可指定目录；`--no-path` 不修改 profile/registry。Unix 的 `--add-to-path` 只支持
`$HOME/.local/bin`，Windows 则只更新当前用户的 `Path`。脚本不会请求管理员/root 权限、安装前置工具、
读取 Pixiv 凭据或绕过系统信誉警告。

这是首次 bootstrap 的信任边界：`pixiv` 尚不存在时，脚本没有内置 Ed25519 verifier。SHA-256 校验可以
发现传输损坏或 archive 不匹配，但来源真实性仍依赖 HTTPS 与官方 GitHub repository/Release 账号；执行前
应审阅安装脚本。安装完成后，后续 `pixiv update` 会使用 binary 内置的 Ed25519 trust root 验证 Release 更新。

### 从源码构建

```bash
sh scripts/build.sh
```

受支持的源码构建需要 Go `1.26.3`、`CGO_ENABLED=1`、目标平台可用的 C linker，以及与
目标匹配的 Rust ugoira staticlib。它会输出 `build/pixiv` 或 `build/pixiv.exe`。Windows
可通过 Git Bash、MSYS2 或 WSL 运行构建命令。

当前工作树已保存 darwin/linux/windows × amd64/arm64 的六个 runner-verified staticlib 与同源
`manifest.json`；`scripts/build.sh` 会先校验 source digest、target/path 与每个库的 SHA-256，再构建
本机 binary。完整要求、证据回填流程和失败含义见[开发流程](../maintainers/development.md#rust-ugoira-staticlib)。

### Go 安装

正式 tag 发布后，使用精确 tag 安装：

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@vX.Y.Z
```

它仍使用本机 Go、cgo、C linker 和该 target 的 committed staticlib。六目标库与 manifest 已完整，
例如当前正式版本可使用 `@v0.4.0`；后续版本始终使用其精确 tag，而不是分支名。

### Homebrew

正式 stable Release 和真实 tap 均通过 audit/安装验证后，macOS/Linux 用户可安装：

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

未来 beta/pre-release 通道使用：

```bash
brew install FlanChanXwO/tap/pixiv-cli-beta
```

两个 formula 都安装同名 `pixiv`，因此相互冲突；它们只下载已验证的 macOS/Linux Release
资产，不引入 `ffmpeg` 依赖。当前 stable `pixiv-cli` formula 已在公开 tap；beta formula 只随
后续 pre-release 发布。

### 直接下载

发布流程会为 darwin、linux、windows 的 amd64/arm64 生成六个固定名称的 archive：
`pixiv-cli_<version>_<os>_<arch>.tar.gz`（Windows 为 `.zip`），以及 `checksums.txt` 与
Ed25519 签名的 `checksums.json`。v0.4.0 已提供完整资产；后续版本只有在同一发布门禁完成后才应作为
可供信任的直接下载来源。

当前 Release 不包含 Apple notarization 或 Windows Authenticode。即使从已验证 Release
下载，macOS Gatekeeper 或 Windows SmartScreen 仍可能显示系统信誉提示；请只从项目的
GitHub Release 页面取得资产，核对版本、checksum 和签名说明，切勿绕过不明来源的警告。

## 获取 refresh token

`PIXIV_REFRESH_TOKEN` 必须是原始的 Pixiv App API OAuth refresh token。网页 Cookie（包括 `refresh_token=...`、`PHPSESSID`、`device_token`）一律拒绝，不会从中提取或转换凭据。

推荐用 CLI 浏览器 OAuth 登录，并直接保存到本地账号：

```bash
pixiv auth login
```

`auth login` 流程：

| 阶段 | 行为 |
| --- | --- |
| 初始化 | CLI 生成 PKCE verifier/challenge 和 OAuth state，并启动本地 loopback HTTP server。 |
| 浏览器 | macOS 默认优先注册本地 `pixiv://` callback helper 并打开默认浏览器，因此可复用已有 Pixiv 登录态；需要用户在 Pixiv 页面确认账号；使用 `--no-open` 时只打印登录 URL 和本地页面地址。 |
| 回调 | CLI 仅接收本轮 loopback callback、当前登录尝试注册的 `pixiv://` helper 转交、终端粘贴或本地页面表单；浏览器若没有返回，可手动粘贴 callback URL、`pixiv://...` URL、Pixiv relay URL 或原始 authorization code。 |
| 校验 | 本地 loopback 回调必须匹配本次 state；Pixiv 官方 callback URL 与 `pixiv://account/login` 可在 Pixiv 未返回 state 时作为显式 fallback。 |
| 保存 | refresh/access token 不会打印；refresh token 按 Pixiv UID 保存到 `auth.json`。Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。 |

默认浏览器打开时，macOS 会注册一个仅服务于当前登录尝试的本地 `PixivCLIURLHandler.app`，只把 Pixiv 返回的 `pixiv://account/login?...` URL 转交给本轮 CLI loopback；它不读取浏览器 Cookie、存储、历史、会话文件、标签页或网络流量。若 helper 不可用，CLI 仍打开正常浏览器并等待 loopback 或手动回填，不会启动受管 Chromium、DevTools/CDP 或浏览器状态扫描。遇到 Pixiv `post-redirect` 授权接力页时，用户可手动粘贴 relay URL；CLI 只在校验其属于本轮 OAuth 后打开该 relay URL 一次。浏览器可能停留在白色 relay 页，是否成功以终端最终输出为准；若 Pixiv 未生成 callback，CLI 不会伪造成功。

浏览器使用的系统代理不会自动传给 Go CLI。若 Pixiv token 端点在当前网络下需要代理，请先配置：

```bash
pixiv config set https_proxy http://127.0.0.1:7890
```

也可以只给本次网络命令临时覆盖代理：

```bash
pixiv auth login --proxy http://127.0.0.1:7890
```

`--proxy URL` 与 `--no-proxy` 都只影响当前命令，不写入 `config.toml`；两者不能同时使用。`--no-proxy` 会清空本次命令的代理，即使环境变量或配置里存在 `https_proxy`。

真实登录依赖 Pixiv OAuth 网页流程可用；自动化测试使用 fake OAuth server，不访问真实 Pixiv。

### 导出已存 token

`pixiv auth token [UID]` 是唯一会主动打印 refresh token 的命令，用于与另一个可信本地程序显式互操作：

```bash
pixiv auth token           # 默认本地账号
pixiv auth token 12345678  # 精确选择本地账号
```

该命令只读取 `auth.json`：不刷新 token、不访问 Pixiv、不读取 `PIXIV_REFRESH_TOKEN`、不接受代理或 JSON
flag、不写入本地状态，也不运行自动更新检查。成功时 stdout 精确包含已存原始 refresh token 和一个换行，
stderr 为空。只能在私密终端中运行；必须交给另一个可信程序时应直接重定向。不要把输出粘贴到聊天、日志、
shell history、issue、测试或 Agent transcript。其他 CLI 命令、JSON 响应、日志和错误仍禁止暴露
refresh token。

没有默认账号、指定 UID 不存在、UID 非法或 auth store 无法读取时，命令以非零状态退出，stderr 输出安全
诊断，stdout 不输出 token；诊断不会包含 token 内容或 auth 文件路径。

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

pixiv version
pixiv version --json
pixiv --version
pixiv update --check
pixiv update --check --json

pixiv search "初音ミク"
pixiv search "初音ミク" --json
pixiv detail 123456
pixiv ranking --mode day
pixiv recommended all
pixiv download 123456 789012
```

账号认证保存到 `os.UserConfigDir()/pixiv/auth.json`，账号 key 是 Pixiv UID；全局配置保存到 `os.UserConfigDir()/pixiv/config.toml`。Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。输出默认给人读；只有 help 中提供 `--json` 的命令可输出机器可解析 JSON，`auth token` 明确不提供该 flag。
CLI 使用 Cobra/pflag，选项可以写在位置参数前后，例如 `pixiv auth check 12345678 --json` 和 `pixiv search "初音ミク" --json` 都是正式支持的写法。

### CLI 命令表

| 命令 | 用法 | 说明 |
| --- | --- | --- |
| `auth add` | `pixiv auth add [--token TOKEN] [--json] [--proxy URL\|--no-proxy]` | 校验原始 Pixiv App API refresh token，并按 Pixiv UID 添加或替换账号；Cookie 输入会被拒绝。不传 `--token` 时从 TTY/stdin 读取。 |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--proxy URL\|--no-proxy]` | 通过本地 loopback server 和浏览器 OAuth 登录，按 Pixiv UID 保存账号；不会输出 refresh token。 |
| `auth list` | `pixiv auth list [--json]` | 列出本地账号；不会输出 refresh token。 |
| `auth token` | `pixiv auth token [UID]` | 向 stdout 输出选中本地账号的原始 refresh token 和换行；默认使用当前账号，不联网、不刷新，也不接受 JSON/代理 flag。 |
| `auth use` | `pixiv auth use [UID] [--json]` | 设置默认账号；TTY 下可交互选择。 |
| `auth remove` | `pixiv auth remove [UID] [--yes] [--json]` | 删除账号；TTY 下默认确认，删除默认账号后会自动选第一个剩余账号。 |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL\|--no-proxy]` | 刷新 token 并验证账号；成功后会记录 `user_id` 和可获取到的 username。 |
| `config path` | `pixiv config path` | 输出 `config.toml` 路径。 |
| `config get` | `pixiv config get KEY` | 输出一个生效中的配置值。 |
| `config set` | `pixiv config set KEY VALUE` | 写入一个已知配置键到 `config.toml`。 |
| `config unset` | `pixiv config unset KEY` | 从 `config.toml` 删除一个已知配置键。 |
| `version` | `pixiv version [--json]` | 输出当前二进制的 `version`、`commit`、`build_date`；根 `pixiv --version` 只输出版本。 |
| `update` | `pixiv update [--check] [--prerelease] [--proxy URL]` | 检查或执行与当前安装来源匹配的更新；`--json` 仅可与 `--check` 同用。 |
| `search` | `pixiv search [options] WORD` | 搜索插画。 |
| `search-options` | `pixiv search-options [options] WORD` | 查询该关键词在 App API 中可用的绘图工具；需要认证，支持通用账号/token/代理参数和 `--json`。 |
| `detail` | `pixiv detail [options] ILLUST_ID` | 查看单个作品详情。 |
| `ranking` | `pixiv ranking [options]` | 查看 Pixiv 插画排行榜。 |
| `recommended` | `pixiv recommended all\|illust\|manga\|novel\|user [--page N --limit N --json]` | 查看指定类个性化推荐；`all` 按插画、漫画、小说、作者顺序完整返回，需要认证。 |
| `user detail` | `pixiv user detail USER_ID [--json]` | 查看指定用户的完整公开资料；`USER_ID` 必填。 |
| `user artworks` | `pixiv user artworks [USER_ID] [--type TYPE --page N --limit N]` | 查看用户作品；省略 `USER_ID` 时使用当前认证用户。 |
| `user bookmarks` | `pixiv user bookmarks [USER_ID] [--restrict public\|private --tag TAG --page N --limit N]` | 查看用户收藏，可按可见性和 tag 筛选；省略 `USER_ID` 时使用当前认证用户。 |
| `user following` | `pixiv user following [USER_ID] [--restrict public\|private --page N --limit N]` | 查看用户关注，可按可见性筛选；省略 `USER_ID` 时使用当前认证用户。 |
| `bookmark add` | `pixiv bookmark add ILLUST_ID [--restrict public\|private --tag TAG...]` | 收藏作品；`--tag` 可重复使用。 |
| `bookmark remove` | `pixiv bookmark remove ILLUST_ID` | 取消收藏作品；不接受可见性或 tag 参数。 |
| `follow add` | `pixiv follow add USER_ID [--restrict public\|private]` | 按选定可见性关注用户。 |
| `follow remove` | `pixiv follow remove USER_ID` | 取消关注用户；不接受可见性参数。 |
| `download` | `pixiv download [options] ILLUST_ID...` | 下载一个或多个作品；无 token 时默认走匿名 web fallback。 |
| `mcp` | `pixiv mcp [--proxy URL\|--no-proxy]` | 启动 MCP stdio server；代理覆盖只在本次启动时生效。 |

下载文件名会规范化文件名模板以及 URL 推导扩展名中的跨平台非法字符；扩展名还会替换 ASCII 控制字符并移除 Windows 不接受的尾随点或空格。扩展名仍来自上游 URL，不使用 allowlist、MIME 猜测或静默替代。

### `auth login` 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--json` | `false` | 输出保存结果 JSON；不会输出 refresh/access token。 |
| `--no-open` | `false` | 不自动打开系统默认浏览器，也不做浏览器观察；只打印登录 URL 和本地 loopback 页面地址。 |
| `--addr` | `127.0.0.1:0` | 本地 loopback 监听地址；端口 `0` 表示自动分配。 |
| `--use` | `false` | 登录成功后设为默认账号；若当前没有默认账号，也会自动设为默认。 |
| `--timeout` | `0` | 等待登录完成的最大时长；`0` 表示不由 CLI 主动限时。 |
| `--proxy URL` / `--no-proxy` | 空 | 本次 token exchange 代理覆盖；不会保存到 `config.toml`。 |

### 数据命令参数

| 命令 | 参数 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `search` | `--search-target` | `partial_match_for_tags` | 搜索范围。 |
| `search` | `--sort` | `date_desc` | 排序方式。 |
| `search` | `--duration` | 空 | Pixiv API 的时间范围参数。 |
| `search` | `--rating` | `all` | 分级筛选：`sfw`、`r18`、`r18g`、`mature` 或 `all`。 |
| `search` | `--type` | `all` | 作品类型：`all`、`illust-and-ugoira`、`illust`、`manga` 或 `ugoira`；`comics` 是 `manga` 的兼容 alias。 |
| `search` | `--ai-mode` | `all` | AI 筛选：`all`、`exclude` 或 `only`；Pixiv `AIType==2` 表示 AI 生成。 |
| `search` | `--ai-type` | `2` | Deprecated alias：`0=exclude`、`1=only`、`2=all`；与显式 `--ai-mode` 冲突。 |
| `search` | `--aspect-ratio` | `all` | 横纵比：`all`、`landscape`、`portrait` 或 `square`。 |
| `search` | `--resolution` | `all` | 分辨率：`all`、`high`、`medium` 或 `low`；宽高两个维度分别都需满足 `>=3000`、`1000..2999` 或 `<=999`。 |
| `search` | `--tool` | 空 | 上游绘图工具的精确名称；用已认证的 `search-options` 查询当前值。 |
| 列表命令 | `--limit` | 一个上游批次 | 最大条数；`0` 表示持续读取到没有下一批。 |
| 列表命令 | `--page` | 空 | 从 1 开始的逻辑页；必须与正数 `--limit` 同用。 |
| 列表命令 | `--offset` | `0` | 已废弃的逻辑偏移；不能与 `--page` 同用。 |
| `search` | `--r18` | `false` | Deprecated `--rating r18` alias；不再修改关键词，与显式非 R18 rating 冲突。 |
| `ranking` | `--mode` | `day` | 排行榜模式。 |
| `ranking` | `--date` | 空 | 排行榜日期，格式通常为 `YYYY-MM-DD`。 |
| `ranking` | `--offset` | `0` | 分页偏移。 |
| `recommended KIND` | `--page`、`--limit`、已废弃 `--offset` | 各流独立分页 | 每条流独立分页；`all` 会对插画、漫画、小说、作者分别应用相同分页语义。 |
| `user artworks` | `--type` | `illust` | 传给用户作品请求的 Pixiv illustration type。 |
| `user bookmarks` | `--restrict` | `public` | 收藏可见性：`public` 或 `private`。 |
| `user bookmarks` | `--tag` | 空 | 精确收藏 tag 筛选。 |
| `user following` | `--restrict` | `public` | 关注可见性：`public` 或 `private`。 |
| `bookmark add` | `--restrict` | `public` | 新收藏的可见性：`public` 或 `private`。 |
| `bookmark add` | `--tag` | 空 | 收藏 tag；可重复使用。 |
| `follow add` | `--restrict` | `public` | 新关注的可见性：`public` 或 `private`。 |
| `detail` | `ILLUST_ID` | 必填 | Pixiv 作品 ID。 |
| `download` | `ILLUST_ID...` | 必填 | 一个或多个 Pixiv 作品 ID。 |

有 refresh token 时，`search` 始终使用 App API。分辨率、横纵比、工具、作品类型和 `ai-mode=exclude` 由 App 筛选，分级和 `ai-mode=only` 对 App 返回批次筛选；App 失败不会回落 Web。全部筛选都会绑定 opaque cursor，旧 cursor 不能用于不同筛选组合。指定正数 `--limit` 或 `--page` 时，CLI 会持续读取上游批次，直到收集到对应数量的匹配作品、上游没有下一批，或检测到重复 cursor；未指定 `--limit` 时保留只读取一个上游批次的兼容默认行为。不提供收藏数筛选。

### 通用参数

| 参数 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--uid UID` | `search/search-options/detail/ranking/recommended/user/download` | `auth.json.default_user_id` | 选择本地账号。 |
| `--profile UID` | `search/search-options/detail/ranking/recommended/user/download` | 空 | `--uid` 的 deprecated alias。 |
| `--refresh-token TOKEN` | `search/search-options/detail/ranking/recommended/user/download` | 空 | 临时覆盖账号/env token；只接受原始 App API refresh token。 |
| `--json` | `auth add/login/list/use/remove/check`、`version`、`update --check` 和数据命令 | `false` | 输出机器可解析 JSON；`auth token` 和实际更新安装不接受。 |
| `--download-path PATH` | 数据命令；实际只影响 `download` | `DOWNLOAD_PATH`、`config.toml` 或 `./downloads` | 下载目录。 |
| `--filename-template TEMPLATE` | 数据命令；实际只影响 `download` | `FILENAME_TEMPLATE`、`config.toml` 或 `{author} - {title}_{id}` | 文件名模板。 |
| `--proxy URL` | `auth add/login/check`、数据命令、`mcp` | `https_proxy`/`HTTPS_PROXY`、`config.toml` 或空 | 临时使用 HTTP(S) 代理；只影响当前命令。 |
| `--no-proxy` | `auth add/login/check`、数据命令、`mcp` | 空 | 临时清空 HTTP(S) 代理；优先级同 `--proxy`，且不能与 `--proxy` 同用。 |

### `config` 支持的键

| KEY | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `download_path` | string | `./downloads` | 下载目录。 |
| `filename_template` | string | `{author} - {title}_{id}` | 文件名模板。 |
| `https_proxy` | string | 空 | HTTP(S) 代理，优先使用环境变量中的小写 `https_proxy`。 |
| `web_fallback_enabled` | bool | `true` | 无 refresh token 时，允许匿名 Pixiv web/ajax API fallback；写入为 `[web] fallback_enabled = true/false`。 |
| `log_level` | string | `warn` | stderr 结构化日志级别；可由 `PIXIV_LOG_LEVEL` 覆盖。显式设为 `info` 可保留操作诊断。 |
| `log_format` | string | `text` | 日志格式 `text` 或 `json`；可由 `PIXIV_LOG_FORMAT` 覆盖。 |
| `update_check_enabled` | bool | `true` | 普通 CLI 成功命令后是否检查稳定版更新；写入为 `[update] check_enabled = true/false`。 |
| `output_json` | bool | `false` | 数据命令默认输出 JSON。 |
| `login_open_browser` | bool | `true` | `auth login` 默认是否自动打开浏览器。 |
| `login_timeout` | duration | `0s` | `auth login` 默认等待时长。 |
| `login_use_after_login` | bool | `false` | `auth login` 默认是否设为当前默认账号。 |

### 环境变量

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | 空 | Pixiv App API OAuth refresh token；可被账号选择或 `--refresh-token` 覆盖。 |
| `PIXIV_LOG_LEVEL` | 空 | 覆盖 `log_level`。 |
| `PIXIV_LOG_FORMAT` | 空 | 覆盖 `log_format`。 |
| `DOWNLOAD_PATH` | `./downloads` | 下载目录。 |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | 文件名模板。 |
| `https_proxy` / `HTTPS_PROXY` | 空 | HTTP(S) 代理；优先使用小写 `https_proxy`。 |

认证优先级：`--refresh-token` > `--uid`/deprecated `--profile` > `PIXIV_REFRESH_TOKEN` > `auth.json.default_user_id`。

设置类字段优先级：命令行 flag > 环境变量 > `config.toml` > 默认值。代理的命令行覆盖只支持 `--proxy URL` / `--no-proxy`，且不会持久化。

### 匿名 web fallback

当 `--refresh-token`、`PIXIV_REFRESH_TOKEN` 和默认账号都没有提供 refresh token，且 `web_fallback_enabled=true` 时，CLI 的 `search`、`detail`、`ranking` 和 `download` 自动走 Pixiv web/ajax API。

有 refresh token 时仍优先使用 App API；token 无效、App API 网络错误或服务端错误不会自动 fallback，会直接返回安全、可分类的失败，不会将错误伪装成正常空结果。

匿名 fallback 的差异：

- 匿名 `search` 只执行 Web API 能可靠表达的筛选。分辨率、横纵比、绘图工具和作品类型会转译为 Web 参数；AI 筛选使用返回的作品字段。
- `rating=r18`、`r18g` 或 `mature` 会在匿名请求前明确返回需要认证，而不会伪装成空结果；`rating=all` 只表示匿名可见范围。
- `search-options` 仅支持 App API，无 refresh token 时明确返回 unsupported。搜索不会读取或保存 `PHPSESSID` 等浏览器 Cookie，也不会把 refresh token 转换成 Web session。
- `search_user` 不是 Pixiv 官方用户搜索；它通过 web 作品搜索结果按 `userId` 去重，返回“相关作品作者”。
- 静态单页/多页下载使用 `/ajax/illust/{id}/pages` 的 `original` URL。
- ugoira 下载使用 `/ajax/illust/{id}/ugoira_meta` 的 `originalSrc` zip 和 frames；受支持的发行构建通过内置 Rust encoder 生成 GIF/APNG，运行时不依赖 `ffmpeg`。
- web fallback 不新增专用代理环境变量，继续使用 `--proxy` / `--no-proxy`、`https_proxy` / `HTTPS_PROXY` 或 `pixiv config set https_proxy ...`。

代理 URL 格式错误时，受影响的 CLI 数据命令与更新检查会在联网前失败；诊断仅保留安全分类与静态上下文，不回显输入中的 userinfo、path 或 query。

关闭方式：

```bash
pixiv config set web_fallback_enabled false
```

## 版本与更新

`pixiv version` 输出可读的版本、commit 与构建日期；`pixiv version --json` 的 stdout 是只含
`version`、`commit`、`build_date` 的 JSON。根 `pixiv --version` 适合快速检查版本。

```bash
pixiv version
pixiv version --json
pixiv --version
```

显式更新先检查再安装；检查可使用 JSON，而实际安装不接受 `--json`：

```bash
pixiv update --check
pixiv update --check --json
pixiv update --check --prerelease
pixiv update --proxy http://127.0.0.1:7890
```

开发构建显示 `dev` 并拒绝自更新。正式安装时，更新器会识别 Homebrew stable/beta、`go install`
或 Release binary：stable/beta 按 `--prerelease` 在两个相互冲突的 formula 间切换；若切换
安装失败，会显式尝试恢复原 formula 并报告原错误和恢复结果。`go install` 使用精确 Release
tag；Release binary 在下载前校验 Ed25519 签名的 checksum 清单和 archive SHA-256，再预检
`pixiv version --json` 并原子替换可执行文件。

更新检查只选择 canonical SemVer tag。stable 检查先排除 GitHub 已标记的 prerelease；
`--prerelease` 则将其纳入当前通道。若当前通道的任一非 draft published Release 使用非
SemVer tag，检查会报告该 tag 并 fail-closed，不会跳过它而选择较旧版本。

受支持 binary 已内置 production Ed25519 public key/key ID/fingerprint；私钥只保存在受保护的
`release` Environment 与受控 macOS Keychain 恢复副本。v0.4.0 是当前已发布的受签名 Release；
`pixiv update --check` 仍只是只读检查，不能替代对选中版本资产、checksum 与签名的安装验证。

普通 CLI 命令成功后会尽力检查 stable 更新。它跳过 MCP、help、`version`、`update`、`auth token` 与开发构建，
对同一用户 cache 最多每 24 小时查询一次，并为自动检查设定最多 3 秒的等待时间。发现新版本或
检查失败只写 stderr（失败为 warning），不改变业务命令退出码，也不会污染 JSON stdout 或 MCP
JSON-RPC stdout。可关闭自动检查：

```bash
pixiv config set update_check_enabled false
```

## 相关文档

本参考手册只定义 CLI 边界；其他接口与维护流程以对应权威文档为准：

- [Go SDK](sdk.md)：public client、模型、分页、资源和 typed error。
- [MCP tools](mcp-tools.md)：tool 名称、输入 schema、输出和 stdio 行为。
- [架构](../maintainers/architecture.md)：包职责和运行流程。
- [开发流程](../maintainers/development.md)：环境、测试、构建和发布门禁。
- [Agent skill](../../skills/pixiv-cli/SKILL.md)：供 Agent 安全驱动已安装 CLI 的说明。
