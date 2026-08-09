# Pixiv CLI 参考手册

[English](../en/cli-reference.md) | 简体中文 | [项目首页](../../README.zh-CN.md)

本文是 `pixiv` 命令的完整契约：安装、认证、命令、flag、配置、环境变量、匿名 fallback 和更新。
SDK 与 MCP 细节不在此重复，入口见[相关文档](#相关文档)。

> 独立 `pixiv filter` 与 `--ugoira-format` 已移除。视觉列表在管道中会自动输出 canonical NDJSON；用 `--filter EXPR` 筛选作品，例如 `bookmarkCount >= 5000 and xRestrict == 0`。下载使用 `--ugoira-mode gif|apng|zip|frames`，支持 `3-` 页选择、`--archive`、`--write-metadata`、`--directory-template`、`--retries` 和 `--retry-delay`。模板可用 `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}`、`{num}`；代理接受 `http`、`https`、`socks5` 与 `socks5h`。配置包含 `directory_template`、`request_interval`（也可用 `PIXIV_REQUEST_INTERVAL` 或本次 `--sleep-request` 覆盖）。

用户可感知变化记录在[按版本归档的更新日志](../../changelog/README.zh-CN.md)。

[GitHub Releases 页面]: https://github.com/FlanChanXwO/pixiv-cli/releases

## 安装与构建

> **发布状态**：受支持 binary 的 Ed25519 公钥、key ID 与 fingerprint 已提交到
> [`internal/bootstrap/release_trust.go`](../../internal/bootstrap/release_trust.go)；公开 source/tap repositories、
> 受保护 `release` Environment 与隔离 credentials 已配置。v0.4.4 已作为公开 GitHub Release 发布，包含六个
> 平台 archive、checksum 与签名清单。GitHub Release 与 tap 是相互独立的发布物；当前状态请以官方
> [GitHub Releases 页面]和 `brew info FlanChanXwO/tap/pixiv-cli` 为准。后续版本仍必须通过同一套 tag、签名、
> 资产与 Homebrew 门禁后才可作为可信下载来源。

### 官方安装脚本

仓库提供两个面向最新 stable Release 的用户级 bootstrap 脚本：

```bash
# Linux/macOS
curl -fsSLo /tmp/pixiv-install.sh https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/install.sh
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

Linux Release 资产要求 glibc 2.35 或更新版本。release、native-evidence 与 packaged-smoke job 都在
Ubuntu 22.04 上为两个 Linux 架构构建，并拒绝 GNU version requirement 高于 `GLIBC_2.35` 的 ELF。安装器的
binary 预检会在替换现有安装前显露 loader 失败。

这是首次 bootstrap 的信任边界：`pixiv` 尚不存在时，脚本没有内置 Ed25519 verifier。SHA-256 校验可以
发现传输损坏或 archive 不匹配，但来源真实性仍依赖 HTTPS 与官方 GitHub repository/Release 账号；执行前
应审阅安装脚本。安装完成后，后续 `pixiv update` 会使用 binary 内置的 Ed25519 trust root 验证 Release 更新。

随正式版本发布的安装器内嵌静态 Release-source 列表。它始终从 GitHub HTTPS 直连获取权威 `checksums.txt`，再仅对匹配的平台 archive 探测免费候选；候选返回的 checksum 必须与直连文件逐字一致。安装前 archive 仍必须匹配该直连 SHA-256。列表不会从远端拉取，只会随签名 Release 更新。

官方安装脚本会初始化当前用户的按需 `pixiv://` handler，Homebrew 在 `post_install` 中做同样操作。若提示 warning，已验证的 binary 仍安装成功，只是桌面集成未完成。macOS 与 Windows 上，下一次普通 `pixiv` 命令会再次尝试；手工解压 archive 因而会在首次使用时修复桌面集成。桌面 Linux 需要 `xdg-mime` 与 `gio`；headless Linux 可运行 relay server，但不会注册浏览器 handler。

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
例如已发布的 v0.4.4 可使用 `@v0.4.4`；始终使用已发布的精确 tag，而不是分支名。

### Homebrew

macOS/Linux 用户可通过 stable formula 安装：

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

未来 beta/pre-release 通道使用：

```bash
brew install FlanChanXwO/tap/pixiv-cli-beta
```

两个 formula 都安装同名 `pixiv`，因此相互冲突；它们只下载已验证的 macOS/Linux Release
资产，不引入 `ffmpeg` 依赖。GitHub Release 与公开 tap 属于独立发布通道，请用
`brew info FlanChanXwO/tap/pixiv-cli` 和 [GitHub Releases 页面]查询当前状态，不要依赖本手册中硬编码的
版本。beta formula 只随 pre-release 发布。

### 直接下载

发布流程会为 darwin、linux、windows 的 amd64/arm64 生成六个固定名称的 archive：
`pixiv-cli_<version>_<os>_<arch>.tar.gz`（Windows 为 `.zip`），以及 `checksums.txt` 与
Ed25519 签名的 `checksums.json`。已发布的 v0.4.4 提供完整资产；后续版本只有在同一发布门禁完成后才应作为
可供信任的直接下载来源。

当前 Release 不包含 Apple notarization 或 Windows Authenticode。即使从已验证 Release
下载，macOS Gatekeeper 或 Windows SmartScreen 仍可能显示系统信誉提示；请只从项目的
GitHub Release 页面取得资产，核对版本、checksum 和签名说明，切勿绕过不明来源的警告。

## 获取 refresh token

`PIXIV_REFRESH_TOKEN` 是原始的 Pixiv App API OAuth refresh token。

推荐用 CLI 浏览器 OAuth 登录，并直接保存到本地账号：

```bash
pixiv auth login
```

`auth login` 流程：

| 阶段 | 行为 |
| --- | --- |
| 初始化 | CLI 生成 PKCE verifier/challenge 和 OAuth state，并启动本地 loopback HTTP server。 |
| 浏览器 | macOS 与 Windows 的普通 CLI 启动会准备当前用户 `pixiv://` callback helper；桌面 Linux 会在交互式登录时初始化 XDG handler。CLI 打开默认浏览器，可复用已有 Pixiv 登录态；使用 `--no-open` 时只打印登录 URL 和本地页面地址。 |
| 回调 | CLI 接收本轮 loopback callback、一次性桌面 handoff、终端粘贴或本地页面表单。helper 转交后，默认浏览器会在 OAuth exchange 完成时打开本地最终成功或失败页。 |
| 校验 | 本地 loopback 回调必须匹配本次 state；Pixiv 官方 callback URL 与 `pixiv://account/login` 可在 Pixiv 未返回 state 时作为显式 fallback。 |
| 保存 | refresh/access token 不会打印；refresh token 按 Pixiv UID 保存到本地 SQLite 数据库。Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。 |

handler 会持久注册，但只在系统打开 `pixiv://` 时按需运行：macOS 使用 `PixivCLIURLHandler.app`，Windows 使用当前用户协议关联，桌面 Linux 使用 XDG desktop entry；旧 handler 会私有记录。本地活跃的 loopback bridge 永远优先。没有本地 bridge 时，`pixiv://account/login` 只会由活跃的一次性桌面 handoff 接收，`pixiv://account/remote-login` 用于启动该 handoff。其他 `pixiv://` URL 会定向交给旧 handler。需要更换 handler 时，请使用系统提供的关联 UI。

在无 GUI 的 SSH 服务器上，应继续把 listener 绑定到 loopback，并选择一个未占用的固定端口，方便从
本地转发。先在服务器运行：

```bash
pixiv auth login --no-open --addr 127.0.0.1:41871
```

再在本机另一个终端运行：

```bash
ssh -N -L 41871:127.0.0.1:41871 USER@SERVER
```

随后用本地浏览器打开 `http://127.0.0.1:41871/`。该 tunnel 只连接服务器 loopback，不会把 callback
端口暴露到公网。它只能让手工页面可达，不能代替浏览器所在机器接收 Pixiv 最终的 `pixiv://` callback。浏览器机器已安装 pixiv-cli 时，请使用下方的一次性桌面 handoff。也可以把完整的最终 callback URL 粘贴回原 `auth login` prompt。不要把登录 listener 绑定到公网接口；`--addr` 会刻意只接受 loopback 地址。

### 跨机器一次性 handoff relay

当服务器保存账号而授权浏览器位于另一台设备时，在服务器配置 `login_relay_public_url` 与
`login_relay_listen_addr`。执行 `pixiv auth login` 会输出一个仅用于本次登录的远程 handoff URL。打开该 URL 会直接重定向到 `pixiv://account/remote-login`；不会渲染 pixiv-cli 的会话页、确认页或复制 callback 的表单。

已安装 pixiv-cli 的桌面端会由本机 CLI 领取该次会话、启动 OAuth URL，并把结果 callback 回传服务器。handoff 仅在本次会话有效，新的 handoff 会替换此前本机的 handoff。没有桌面 handler 的客户端无法完成该 relay 流程，应使用已安装 pixiv-cli 的桌面端。

relay 可使用 HTTP 或 HTTPS。可直接提供 TLS PEM，或以同机反向代理终止 HTTPS 并让 listener 只监听 loopback。旧 `login_relay_secret` 与 `login_relay_target_url` 设置会被静默忽略；`pixiv auth devices` 已移除。`pixiv config` 只管理下载路径、文件名模板和 HTTPS 代理；高级 relay 设置仍保存在私有 `config.toml`。

浏览器使用的系统代理不会自动传给 Go CLI。若 Pixiv token 端点在当前网络下需要代理，请先配置：

```bash
pixiv config set https_proxy http://127.0.0.1:7890
```

也可以只给本次网络命令临时覆盖代理：

```bash
pixiv auth login --proxy http://127.0.0.1:7890
```

`--proxy URL` 与 `--no-proxy` 都只影响当前命令，不写入 `config.toml`；两者不能同时使用。`--no-proxy` 会清空本次命令的代理，即使环境变量或配置里存在 `https_proxy`。

配置 HTTP(S) 代理时，媒体资源传输（如 `download`，包括 ugoira）会刻意使用 HTTP/1.1；App API、OAuth 与 Web 元数据请求仍保留其常规协议协商。此行为规避部分代理特有的 HTTP/2 流重置，不改变认证或所选下载质量。

真实登录依赖 Pixiv OAuth 网页流程可用；自动化测试使用 fake OAuth server，不访问真实 Pixiv。

### 导入认证

direct import 接受原始 Pixiv App OAuth refresh token：

```bash
pixiv auth import                         # TTY 隐藏输入
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth import
pixiv auth import 'YOUR_REFRESH_TOKEN'    # 会出现在 argv/shell history
```

`pixiv auth import [REFRESH_TOKEN]` 通过 App OAuth 校验 raw token，以 Pixiv 返回的 UID 为准，并保存 rotation 后的 refresh token。无参数时，TTY 使用隐藏输入；非 TTY 从 stdin 读取一行 opaque 内容，只移除一个末尾 LF 或 CRLF。位置参数虽方便，却可能被进程列表、shell history、wrapper 或审计工具记录。`--json` 只改变不含 secret 的账号摘要；`--proxy` 与 `--no-proxy` 只影响本次 direct validation，且不能同用。

direct import 成功时报告 `added uid:UID` 或 `updated uid:UID`，text 在 username 可用时另输出 `username:NAME`。JSON 精确为一个无 secret account item，例如 `{"user_id":12345678,"username":"display name","status":"added"}`。`status` 仅为 `added` 或 `updated`；两种形式均不暴露 default、token 是否存在、输入 token 或 rotation 后的 token。

离线恢复 export bundle：

```bash
pixiv auth import --file account.pxauth
pixiv auth export --all | ssh trusted-host pixiv auth import --file -
```

`--file PATH` 从文件读取，`--file -` 从 stdin 读取完整 bundle。该模式完全离线，不校验或 rotation token，并拒绝位置 token、`--proxy`、`--no-proxy`。restore 按 UID 原子 merge 全部账号：已有账号更新，新账号添加；本地已有 default 保持不变，仅本地无 default 时采用 bundle default。默认文本输出按输入 bundle 顺序逐项列出安全的 added/updated UID 和最终 default。`--json` 返回 `{"accounts":[{"user_id":12345678,"username":"display name","status":"added"}],"default_user_id":12345678}`；account item 只暴露 `user_id`、`username` 与 `status`。

### 导出与备份认证

```bash
pixiv auth export                         # 默认账号 raw token
pixiv auth export 12345678                # 指定账号 raw token
pixiv auth export --all                   # stdout 上的全账号 versioned bundle
pixiv auth export 12345678 --output account.pxauth
pixiv auth export --all --output accounts.pxauth
pixiv auth export --all --output accounts.pxauth --force
```

不带 `--output` 时，只有两种形式可向 stdout 写 secret：默认/UID export 精确输出已存 raw token 与一个换行；`--all` 只输出 versioned JSON bundle。成功时 stderr 为空。export 严格 local-only：只读本地 SQLite 数据库，不读取 `PIXIV_REFRESH_TOKEN`，不刷新、不访问 Pixiv、不修改 auth/config，并跳过 startup pending-update cleanup 与 automatic update。`--all` 不能和 UID 同用，`--force` 必须配合 `--output`，export 不接受 JSON/代理 flag。

带 `--output PATH` 时，单账号和 `--all` 都写 bundle，不写 raw token。默认拒绝覆盖既有文件，只有显式 `--force` 才 replacement；成功 stdout 只有 output path 与 account count。Unix-like 目标文件为 `0600`，既有 parent 权限与 ownership 不变。Windows 明确设置文件 owner 与 protected DACL，只允许当前用户、LocalSystem、builtin Administrators 完全控制。CI tests 覆盖该 Windows policy；本文不声称本次 release 验收已在真实 Windows filesystem 运行。

bundle 是未加密、含 secret 的 point-in-time backup，不是 live sync。必须像原始 token 一样保存和传输；token rotation 会令旧 bundle 或其他机器副本 stale。strict versioned codec 拒绝不支持的 schema/version、未知或重复字段、尾随 JSON、重复/非正 UID、空 token，以及未指向 bundle 内账号的 default UID。顶层与 account object 的 key 必须严格使用 canonical 拼写和大小写；`Schema`、`Default_User_ID`、`User_ID`、`Refresh_Token` 等 alias 即使与 canonical key 并存也会被拒绝。

export 选择或 I/O 失败时，stdout 不会收到 secret 诊断。restore 原子写失败时，`LocalWriteCommitOutcome=not_committed` 表示 replacement 未发生；`committed` 表示 replacement 已发生但后续 durability/cleanup 失败，必须重新加载 store；`unknown` 表示 recovery 无法确认目标状态，需人工检查。不得把 `committed` 或 `unknown` 视为已成功 rollback。其他 stdout/stderr、JSON、MCP result、日志和错误仍禁止暴露 refresh token。不会新增 persistent auth import/export MCP tool；既有 session-scoped MCP 认证行为不变。

## CLI 使用

先登录并保存一个账号：

```bash
pixiv auth login
```

高级/脚本场景也可在不把 token 放进 argv 的情况下导入：

```bash
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth import
```

常用命令：

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
pixiv auth refresh
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

所有持久的应用管理数据直接保存到当前用户主目录：macOS/Linux 为 `~/.pixiv-cli`，Windows 为 `%USERPROFILE%\.pixiv-cli`。其中包括 `pixiv-cli.db`、`config.toml`、回调桥接状态、Release 检查缓存和 macOS 回调 helper；账号认证以 Pixiv UID 为 key。Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。输出默认给人读；只有 help 中提供 `--json` 的命令可输出机器可解析 JSON，`auth export` 明确不提供该 flag。
首次执行普通命令时，若不存在 `config.toml`，CLI 会生成只含下载、输出、登录与更新常用设置的基础文件，且绝不覆盖已有文件。代理、登录超时和 Premium 状态缓存等高级设置会保持省略，直到用户显式配置；help、version、secret export 和内部 OAuth callback 不会创建该文件。
CLI 使用 Cobra/pflag，选项可以写在位置参数前后，例如 `pixiv auth check 12345678 --json` 和 `pixiv search "初音ミク" --json` 都是正式支持的写法。

### v0.8.0 数据命令契约

账号池关闭时，所有非写入的数据读取、推荐、时间线与下载使用 `pixiv auth use` 选定的本地账号。只有 `[account_pool]` 显式设置 `enabled = true` 时才启用数据库账号池；账号行的 `schedulable` 控制是否参加调度，`strategy` 默认 `round_robin`，也支持 `random`。使用 `pixiv auth pool status|enable|disable` 查看或修改调度状态。写操作、认证和配置不使用账号池。数据命令拒绝 `--uid`、`--refresh-token`，并忽略 `PIXIV_REFRESH_TOKEN`。

视觉列表接入管道时会自动输出 NDJSON；也可显式使用 `--ndjson`。每行都是带稳定字符串 `id`、`type`、`url` 的规范 Record，其余 SDK 字段会保留。`download`、`bookmark add/remove`、`follow add/remove` 可不带位置 ID 直接消费它们。视觉列表与 `download` 使用 `--filter EXPR` 在本地按作品字段筛选；动作成功时 stdout 保持为空，安全诊断写入 stderr。`--on-error=skip|fail-fast` 控制 stdin 中格式错误或不兼容 Record 的处理；`--json` 与 `--ndjson` 不能同时使用。

Ugoira 下载使用 `--ugoira-mode gif|apng|zip|frames`，默认 `gif`；指定页码或非 original 质量时会明确报错。

### CLI 命令表

| 命令 | 用法 | 说明 |
| --- | --- | --- |
| `auth import` | `pixiv auth import [REFRESH_TOKEN] [--file PATH] [--json] [--proxy URL\|--no-proxy]` | direct input 校验并保存 rotation 后的 token；无参 TTY 隐藏输入，非 TTY 读取 raw stdin。`--file PATH|-` 改为离线原子恢复 bundle，并与 token/代理输入冲突。 |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--relay-public-url URL --relay-listen-addr ADDR] [--relay-tls-cert-file PATH --relay-tls-key-file PATH] [--proxy URL\|--no-proxy]` | 使用普通 loopback OAuth；完整 server relay 配置存在时输出一次性 handoff URL，直接启动已安装的 desktop CLI handler。按 Pixiv UID 保存账号，绝不输出 refresh token。 |
| `auth list` | `pixiv auth list [--json]` | 列出本地账号；不会输出 refresh token。文本中 `*` 表示默认账号，`✓`/`-` 分别表示本地保存/缺少 refresh token；这些只是本地状态标记，不代表已在线验证有效。 |
| `auth pool` | `pixiv auth pool status [--json]`；`pixiv auth pool enable UID... [--all]`；`pixiv auth pool disable UID... [--all]` | 查看或修改非 secret 的数据库调度状态。`status` 显示 `enabled`、`strategy`、`schedulable`、`frozen_until` 与当前 `eligible`；enable/disable 会先校验全部 UID，再提交整批。 |
| `auth export` | `pixiv auth export [UID] [--all] [--output PATH] [--force]` | 本地导出默认/指定账号或全部账号；无 `--output` 时单账号输出 raw token、`--all` 输出 bundle；带 `--output` 时都写私有 bundle，stdout 仅安全摘要。`--force` 必须与 `--output` 同用。 |
| `auth use` | `pixiv auth use [UID] [--json]` | 设置默认账号；TTY 下可交互选择。 |
| `auth remove` | `pixiv auth remove [UID] [--yes] [--json]` | 删除账号；TTY 下默认确认，删除默认账号后会自动选第一个剩余账号。 |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL\|--no-proxy]` | 刷新 token 并验证账号；成功后会记录 `user_id` 和可获取到的 username。 |
| `auth refresh` | `pixiv auth refresh [UID] [--all] [--json] [--proxy URL\|--no-proxy]` | 刷新指定/默认已保存账号的 OAuth access token 与 rotation 后 refresh token，再强制读取 profile 更新 Pixiv 高级会员缓存。`--all` 刷新全部已保存账号；JSON 固定返回 `accounts`。 |
| `config path` | `pixiv config path` | 输出 `config.toml` 路径；不存在时创建基础文件。 |
| `config get` | `pixiv config get KEY` | 输出一个生效中的配置值。 |
| `config set` | `pixiv config set KEY [VALUE]` | 写入已知配置键，包括 `account_pool_enabled`、`account_pool_strategy`、`download_path`、`filename_template`、`directory_template`、`request_interval` 与 `https_proxy`。 |
| `config unset` | `pixiv config unset KEY` | 从 `config.toml` 删除一个已知配置键。 |
| `version` | `pixiv version [--json]` | 输出当前二进制的 `version`、`commit`、`build_date`；根 `pixiv --version` 只输出版本。 |
| `update` | `pixiv update [--check] [--prerelease] [--proxy URL]` | 检查或执行与当前安装来源匹配的更新；`--json` 仅可与 `--check` 同用。 |
| `search` | `pixiv search [options] WORD` | 搜索插画。 |
| `novel search` | `pixiv novel search [options] WORD` | 通过认证 App API 搜索小说。 |
| `detail` | `pixiv detail [options] ILLUST_ID_OR_URL` | 查看单个作品 ID 或受支持 Pixiv 作品 URL 的详情。 |
| `ranking` | `pixiv ranking [options]` | 查看 Pixiv 插画排行榜。 |
| `recommended` | `pixiv recommended all\|illust\|manga\|novel\|user [--page N --limit N --json]` | 查看指定类个性化推荐；`all` 按插画、漫画、小说、作者顺序完整返回，需要认证。 |
| `timeline following` | `pixiv timeline following --type illust\|novel [--restrict public\|private --page N --limit N --json\|--ndjson]` | 读取关注作者的新作。 |
| `timeline latest` | `pixiv timeline latest --type illust\|manga\|novel [--page N --limit N --json\|--ndjson]` | 读取 App 最新作品。 |
| `mypixiv users` | `pixiv mypixiv users [--page N --limit N --json\|--ndjson]` | 列出所选账号的 MyPixiv 用户。 |
| `mypixiv works` | `pixiv mypixiv works [USER_ID] --type illust\|manga\|novel [--page N --limit N --json\|--ndjson]` | 列出 MyPixiv 作品；省略 USER_ID 时只允许 `illust` 或 `novel`。 |
| `user search` | `pixiv user search WORD [--page N --limit N --json]` | 搜索用户；JSON 和文本会标明结果来自官方 App 用户搜索，还是匿名“相关插画作者”fallback。 |
| `user detail` | `pixiv user detail USER_ID [--json]` | 查看指定用户的完整公开资料；`USER_ID` 必填。 |
| `user artworks` | `pixiv user artworks [USER_ID] [--type TYPE --filter EXPR --page N --limit N]` | 查看用户作品；省略 `USER_ID` 时使用当前认证用户。 |
| `user bookmarks` | `pixiv user bookmarks [USER_ID] [--restrict public\|private --tag TAG --filter EXPR --page N --limit N]` | 查看用户收藏，可按可见性和 tag 筛选；省略 `USER_ID` 时使用当前认证用户。 |
| `user following` | `pixiv user following [USER_ID] [--restrict public\|private --page N --limit N]` | 查看用户关注，可按可见性筛选；省略 `USER_ID` 时使用当前认证用户。 |
| `bookmark add` | `pixiv bookmark add ILLUST_ID [--restrict public\|private --tag TAG...]` | 收藏作品；`--tag` 可重复使用。 |
| `bookmark remove` | `pixiv bookmark remove ILLUST_ID` | 取消收藏作品；不接受可见性或 tag 参数。 |
| `follow add` | `pixiv follow add USER_ID [--restrict public\|private]` | 按选定可见性关注用户。 |
| `follow remove` | `pixiv follow remove USER_ID` | 取消关注用户；不接受可见性参数。 |
| `download` | `pixiv download [options] SRC...` | 下载作品 PID/URL、允许的 CDN 直链，或从受支持的用户 URL 下载全部视觉作品。 |
| `mcp` | `pixiv mcp [--proxy URL\|--no-proxy]` | 启动 MCP stdio server；代理覆盖只在本次启动时生效。 |
| `fanbox auth` | `pixiv fanbox auth import|list|use|remove|status` | 导入并管理本地 FANBOX session；session 值永不输出。native `--proxy`/`--no-proxy` 只影响本次 FANBOX 命令。 |
| `fanbox creators` | `pixiv fanbox creators [--kind supporting\|following] [--page N --limit N]` | 列出 supporting 或 following FANBOX creator。 |
| `fanbox posts` | `pixiv fanbox posts SOURCE [--page N --limit N]` | 按 creator、tag、post ID 或支持的 FANBOX URL 列出帖子。 |
| `fanbox tags` | `pixiv fanbox tags CREATOR` | 列出 creator 使用的 featured tag。 |
| `fanbox home` / `supporting` | `pixiv fanbox home|supporting [--page N --limit N]` | 读取认证 FANBOX home 或 supporting feed。 |
| `fanbox post` | `pixiv fanbox post POST_ID` | 读取一个帖子及其安全 asset 摘要。 |
| `fanbox download` | `pixiv fanbox download SOURCE...` | 将 FANBOX 帖子 asset 保存到配置的下载目录下。 |
| `fanbox mcp` | `pixiv fanbox mcp [--proxy URL\|--no-proxy]` | 启动只读 FANBOX MCP stdio server；native 代理不会修改 FlareSolverr 配置。 |

下载文件名会规范化文件名模板以及 URL 推导扩展名中的跨平台非法字符；扩展名还会替换 ASCII 控制字符并移除 Windows 不接受的尾随点或空格。扩展名仍来自上游 URL，不使用 allowlist、MIME 猜测或静默替代。

### `auth login` 参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--json` | `false` | 输出保存结果 JSON；不会输出 refresh/access token。 |
| `--no-open` | `false` | 不自动打开系统默认浏览器，也不做浏览器观察；只打印登录 URL 和本地 loopback 页面地址。 |
| `--addr` | `127.0.0.1:0` | 本地 loopback 监听地址；端口 `0` 表示自动分配。 |
| `--use` | `false` | 登录成功后设为默认账号；若当前没有默认账号，也会自动设为默认。 |
| `--timeout` | `0` | 等待登录完成的最大时长；`0` 表示不由 CLI 主动限时。 |
| `--relay-public-url` | config | 本次 server relay 的公开 HTTP(S) base URL。 |
| `--relay-listen-addr` | config | 本次 server relay 的监听 host:port。 |
| `--relay-tls-cert-file` / `--relay-tls-key-file` | config | 直连 TLS 的 PEM 对，必须同时提供；未提供时 HTTPS 公开 URL 要求同机反向代理和 loopback listener。 |
| `--proxy URL` / `--no-proxy` | 空 | 本次 token exchange 代理覆盖；不会保存到 `config.toml`。 |

### 数据命令参数

| 命令 | 参数 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `search` | `--search-by` | `tag-partial` | 搜索字段：`tag-partial`、`tag-exact`、`title-caption`，或仅 App OAuth 可用的 `tag-title-caption`（标签、标题、说明文字）。 |
| `novel search` | `--search-by` | `tag-partial` | 搜索字段：`tag-partial`、`tag-exact` 或 `title-caption`。 |
| `search`、`novel search` | `--sort` | `date_desc` | 排序方式：`date_desc` 或 `date_asc`。 |
| `search` | `--period` | 空 | 快捷时间范围：`day`、`week`、`month`、`half-year` 或 `year`；省略则不限制时间。不能和 `--start-date`、`--end-date` 同用。 |
| `novel search` | `--period` | 空 | 时间范围：`day`、`week` 或 `month`；省略则不限制时间。 |
| `search` | `--start-date` / `--end-date` | 空 | 包含边界的 `YYYY-MM-DD` 日期；可只给一端，两端都给时起始不得晚于结束；不能和 `--period` 同用。 |
| `search`、`novel search` | `--rating` | `all` | 分级筛选：`sfw`、`r18`、`r18g`、`mature` 或 `all`。 |
| `search` | `--type` | `all` | 作品类型：`all`、`illust-and-ugoira`、`illust`、`manga` 或 `ugoira`。 |
| `search` | `--ai-mode` | `all` | AI 筛选：`all`、`exclude` 或 `only`；Pixiv `AIType==2` 表示 AI 生成。 |
| `search` | `--aspect-ratio` | `all` | 横纵比：`all`、`landscape`、`portrait` 或 `square`。 |
| `search` | `--resolution` | `all` | 分辨率：`all`、`high`、`medium` 或 `low`；宽高两个维度分别都需满足 `>=3000`、`1000..2999` 或 `<=999`。 |
| `search` | `--draw-tool` | 空 | 本版本绘图工具目录中的精确名称。唯一的一次编辑拼写错误会给出建议；含混前缀会直接报错。 |
| `search` | `--bookmark-min` / `--bookmark-max` | 空 | 包含边界的非负公开收藏数；需要 App OAuth 和有效的 Pixiv 高级会员，且最小值不能大于最大值。已保存账号会先检查缓存的自身 profile 状态，非会员在本地拦截。 |
| `novel search` | `--min-text-length` | `0` | 正文最少字符数；`0` 关闭下界。 |
| `novel search` | `--max-text-length` | `0` | 正文最多字符数；`0` 关闭上界，且不能小于非零下界。 |
| `novel search` | `--original-only` | `false` | 仅保留 Pixiv 标记为原创的小说。 |
| 列表命令 | `--limit` | 一个上游批次 | 最大条数；省略时只取一个上游批次，`0` 表示持续读取到没有下一批。 |
| 列表命令 | `--page` | 空 | 从 1 开始的逻辑页；必须与正数 `--limit` 同用。 |
| `ranking` | `--mode` | `day` | 可用 `day`、`day_male`、`day_female`、`week`、`week_original`、`week_rookie`、`month`、`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、`day_male_r18`、`day_female_r18`、`week_r18`、`week_r18g`；最后九种需要认证。 |
| `ranking` | `--date` | 空 | 排行榜日期，格式通常为 `YYYY-MM-DD`。 |
| `recommended KIND` | `--page`、`--limit` | 各流独立分页 | 每条流独立分页；`all` 会对插画、漫画、小说、作者分别应用相同分页语义。 |
| `timeline following` | `--type`、`--restrict` | 必填、`public` | 类型为 `illust` 或 `novel`；可见性为 `public` 或 `private`。 |
| `timeline latest`、`mypixiv works` | `--type` | 必填 | 时间线支持 `illust`、`manga`、`novel`；省略 USER_ID 的 MyPixiv 只支持 `illust`、`novel`。 |
| Record 动作 | `--on-error` | `skip` | 对格式错误/不兼容记录选择写 stderr 后跳过，或 `fail-fast`。 |
| `download` | `--pages` | 空 | 1-based 页选择，如 `1,3-5` 或 `3-`；默认下载全部页。页不存在会明确失败。 |
| `download` | `--quality` | `original` | 静态图质量：`original`、`regular`（最长边 1200）、`small`（最长边 540）、`thumb`（250×250 居中裁剪）、`mini`（48×48 居中裁剪）。Ugoira 对非 original 质量或页选择返回 unsupported。 |
| `download` | `--download-path` | `DOWNLOAD_PATH`、`config.toml` 或 `./downloads` | 下载目录；其他命令不接受此参数。 |
| `download` | `--ugoira-mode` | `gif` | Ugoira 输出：`gif`、`apng`、`zip` 或 `frames`。 |
| `download` | `--filename-template` | `FILENAME_TEMPLATE`、`config.toml` 或 `{author} - {title}_{id}` | 支持 `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}`、`{num}`。未知占位符或不配对花括号会报错。 |
| `download` | `--directory-template` | 空 | 使用同一占位符的安全相对目录模板。 |
| 视觉列表、`download` | `--filter EXPR` | 空 | 类型化本地插画表达式；与常规筛选参数按 AND 组合。 |
| `download` | `--archive` / `--write-metadata` | 空 / `false` | SQLite 完整作品归档与每件产物 JSON sidecar。 |
| `download` | `--retries` / `--retry-delay` | `3` / `1s` | 符合条件的资源读取重试；有效 `Retry-After` 覆盖等待。 |
| `download` | `--concurrency` | `0`（自动） | 下载 worker 数；`0` 使用 `2 × GOMAXPROCS`，正数精确采用。 |
| `user artworks` | `--type` | `illust` | Pixiv 作品类型：`illust`、`manga` 或 `ugoira`。 |
| `user bookmarks` | `--restrict` | `public` | 收藏可见性：`public` 或 `private`。 |
| `user bookmarks` | `--tag` | 空 | 精确收藏 tag 筛选。 |
| `user following` | `--restrict` | `public` | 关注可见性：`public` 或 `private`。 |
| `bookmark add` | `--restrict` | `public` | 新收藏的可见性：`public` 或 `private`。 |
| `bookmark add` | `--tag` | 空 | 收藏 tag；可重复使用。 |
| `follow add` | `--restrict` | `public` | 新关注的可见性：`public` 或 `private`。 |
| `detail` | `ILLUST_ID_OR_URL` | 必填 | 正整数作品 ID，或受支持的 Pixiv 作品 URL。 |
| `download` | `SRC...` | 必填 | 作品 PID/URL、允许的 CDN URL、用户主页/作品页、公开书签页或插画系列页。CDN 文件仅使用 URL 文件名，不支持依赖作品元数据的选项。 |

有 refresh token 时，`search` 由 App API 执行。分辨率、横纵比、工具、作品类型和 `ai-mode=exclude` 由 App 筛选，分级和 `ai-mode=only` 对 App 返回批次筛选；认证、网络或服务端失败会返回分类错误。全部筛选都会绑定 opaque cursor，cursor 不能用于不同筛选组合。本地筛选跳过连续空上游批次时，CLI/MCP 会补拉到首个非空逻辑批次或真正结束。指定正数 `--limit` 或 `--page` 时，按过滤后的逻辑结果跨批填满；`--limit 0` 遍历全部过滤结果；未指定 `--limit` 时读取一个上游批次，但会跳过前导空批。App 还会执行显式日期与仅限 Pixiv 高级会员的收藏数边界；收藏数不是点赞字段，不得文案为点赞。作品 JSON/文本包含稳定作品页 URL `https://www.pixiv.net/artworks/{id}`，作为首字段/每件作品第一行。

交互式终端运行 `download` 时，命令会在传输前对全部选中资源执行安全 HEAD 探测；全部资源大小可确定时，stderr 显示批次总字节进度。重定向、JSON 与 NDJSON 输出保持不变。公开 SDK 即使无法得知总大小也会提供单资源进度；取消会保留可安全续传的、带 validator 的残片。

### 绘图工具目录

`--draw-tool` 与 MCP `tool` 只接受此版本目录中的精确值；普通帮助和错误信息不展开目录。

```text
SAI · Photoshop · CLIP STUDIO PAINT · IllustStudio · ComicStudio · Pixia · AzPainter4 · Painter · Illustrator · GIMP
FireAlpaca · 網上描繪 · AzPainter · CGillust · 描繪聊天室 · 手畫博克 · MS_Paint · PictBear · openCanvas · PaintShopPro
EDGE · drawr · COMICWORKS · AzDrawing · SketchBookPro · PhotoStudio · Paintgraphic · MediBang Paint · NekoPaint · Inkscape
ArtRage · AzDrawing4 · Fireworks · ibisPaint · AfterEffects · mdiapp · GraphicsGale · Krita · kokuban.in · RETAS STUDIO
emote · 4thPaint · ComiLabo · pixiv Sketch · Pixelmator · Procreate · Expression · PicturePublisher · Processing · Live2D
dotpict · Aseprite · Pastela · Poser · Metasequoia · Blender · Shade · 3dsMax · DAZ Studio · ZBrush
Comi Po! · Maya · Lightwave3D · 六角大王 · Vue · SketchUp · CINEMA4D · XSI · CARRARA · Bryce
STRATA · Sculptris · modo · AnimationMaster · VistaPro · Sunny3D · 3D-Coat · Paint 3D · VRoid Studio · 筆芯筆
鉛筆 · 原子筆 · 毫筆 · 顏色鉛筆 · Copic麥克筆 · 沾水筆 · 透明水彩 · 毛筆 · 記號筆 · 麥克筆
水溶性彩色铅笔 · 涂料 · 丙烯顏料 · 鋼筆 · 粉彩 · 噴筆 · 顏色墨水 · 蠟筆 · 油彩 · COUPY-PENCIL · 顏彩
```

收藏数边界对已保存账号复用固定 24 小时的自身 profile Premium 缓存。缓存未命中或过期时，会先读取 profile；确认非会员即在本地失败，不向 Pixiv 搜索端点发请求。使用 `pixiv auth refresh [UID]`（或 `--all`）可强制刷新 OAuth token 与该状态。直接传入 SDK access token 时没有可验证的本地账号身份，无法使用这项已保存账号预检。

### 插画标签查询语法

已在认证 App API 上验证：插画 `search` 选择标签模式时，`tag-exact` 适合布尔标签筛选。`tagA tagB`
表示同时要求两个完整标签（AND），`tagA OR tagB` 表示任一完整标签即可（OR）；`OR` 必须大写。字面量
`AND` 不是已验证的运算符，应以空格分隔两个标签。

默认的 `tag-partial` 也接受已验证的大写 `OR` 语法，但每个词都是模糊标签条件，不能把结果描述成严格的
精确标签 AND：它可能匹配部分标签、别名或翻译标签，而作品未必显式列出输入的完整标签。`title-caption` 和仅 App OAuth 可用的 `tag-title-caption`
都没有已记录的布尔标签契约。尚未验证对字面量大写 `OR` 标签/关键词的转义语法；需要严格查询时请避免该 token 并使用精确标签。

`novel search` 仅走 App API。App 请求只表达关键词匹配、日期排序和时间范围；分级、正文长度与原创条件逐批依据稳定返回字段验证。字段缺失会明确返回上游响应错误，不会静默视为不匹配。筛选跳过上游批次时，逻辑 `--page`/`--limit` 语义不变。小说 JSON 包含稳定作品页 URL `https://www.pixiv.net/novel/show.php?id={id}`、`x_restrict`、`text_length` 与 `is_original`。

认证态 `detail`、pages 与 ugoira metadata 只使用 App API。App 的页数不一致或缺少页面资源会明确失败，不会改发
匿名 Web 请求。认证 ugoira 未取得 original ZIP 时会使用已验证的 App medium ZIP，下载器直接选择该资源。仅幂等
App JSON 读取在首次 429 且 `Retry-After` 有效时按命令 context 等待并重试一次；header 缺失/非法、第二次 429、
写操作和资源下载绝不重放。
`detail --json` 保留 Pixiv 原始 HTML `caption`；普通 `detail` 输出将其安全转为纯文本，作品列表不会输出 caption。

`detail` 接受正整数作品 ID，或规范 HTTPS `pixiv.net`/`www.pixiv.net` 作品 URL：`/artworks/{id}`；可带 locale、query 和 fragment。不接受用户页、小说、短链、FANBOX、Pixivision、Sketch、旧式或任意其他 URL。

`download` 还接受受策略允许的 CDN 直链、`/users/{id}` 与 `/users/{id}/artworks`。用户 URL 会通过 App OAuth 完整遍历并下载 `illust`、`manga`、`ugoira`，小说不在下载集合内。URL 在本地解析为受支持引用。单个作品失败不会阻止其余作品，取消会立即停止。下载会在 `.pixiv-cache` 持久化 ETag/Last-Modified 元数据，只用 validator 匹配的 `If-Range` 安全续传残片，并原子发布更新。`download` 是动作：成功 stdout 为空；安全失败写入 stderr，无法完成时以非零码退出。不会保存下载历史，也不做跨次去重。

### 通用参数

| 参数 | 适用命令 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `--ndjson` | 数据列表/读取命令 | `false` | 每行输出一个规范 Record，用于流式 filter 与 action；不能与 `--json` 同用。 |
| `--json` | 安全数据读取、认证摘要、`version`、`update --check` | `false` | 在命令提供时输出一个完整结果文档。下载和写动作不输出成功报告。 |
| `--sleep-request DURATION` | 联网命令和 `mcp` | 配置/默认值 | 本次请求起始间隔，覆盖 `PIXIV_REQUEST_INTERVAL` 与 `[network].request_interval`。 |
| `--proxy URL` | 联网命令和 `mcp` | `https_proxy`/`HTTPS_PROXY`、`config.toml` 或空 | 仅本次使用 `http`、`https`、`socks5` 或 `socks5h` 代理 URI；`auth import --file` 禁用。 |
| `--no-proxy` | 同 `--proxy` | 空 | 仅本次清空代理；不能与 `--proxy` 或 bundle restore 同用。 |
| `--debug` | 所有 CLI 命令、`mcp` 与 `fanbox mcp` | `false` | 只向 stderr 写安全的实时英文诊断；不创建日志文件，也不改变 stdout、路由、重试或结果 shape。`auth export` 与隐藏 OAuth callback 仍保持 stderr 为空。 |

### CLI 可管理的 `config` 别名

`pixiv config get/set/unset` **只接受**下列别名。其他运行时设置只能手工维护在私有 `config.toml` 中；CLI 不提供通用配置编辑器。

| KEY | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `account_pool_enabled` | boolean | `false` | 为安全读取/下载启用数据库账号池。 |
| `account_pool_strategy` | string | `round_robin` | 账号池策略，只能是 `round_robin` 或 `random`。 |
| `download_path` | string | `./downloads` | 下载目录。 |
| `filename_template` | string | `{author} - {title}_{id}` | 文件名模板。 |
| `directory_template` | string | 空 | 相对下载目录模板。 |
| `request_interval` | duration | `0` | 请求起始间隔；`PIXIV_REQUEST_INTERVAL` 与一次性的 `--sleep-request` 可覆盖。 |
| `https_proxy` | string | 空 | 全局 `http`、`https`、`socks5` 或 `socks5h` 代理 URI；小写 `https_proxy` 环境变量优先。 |

手工 TOML 可以包含 `[account_pool]`、`[network]`、`[pixiv.network]`、`[fanbox.network]`、`[fanbox.flaresolverr]`、`[login]`、`[update]` 等高级运行时段：

```toml
[network]
https_proxy = "http://global-proxy.example:7890"

[pixiv.network]
proxy_url = "socks5h://pixiv-proxy.example:1080"

[fanbox.network]
proxy_url = ""                    # 显式选择 FANBOX native direct
user_agent = "my-native-agent/1.0"

[fanbox.flaresolverr]
url = "http://127.0.0.1:8191"
proxy_url = "socks5://solver-upstream.example:1080"
```

`[pixiv.network].proxy_url` 与 `[fanbox.network].proxy_url` 区分缺失和显式空值：命令 `--proxy`/`--no-proxy` > 对应 service key（含显式空值） > `https_proxy`/`HTTPS_PROXY` > `[network].https_proxy` > direct。FANBOX native 只接受不带 userinfo 的 HTTP(S) CONNECT；Pixiv 接受 HTTP(S)、SOCKS5 与 SOCKS5H。`user_agent` 只修改 FANBOX native header，不改变 Chrome 146 TLS profile，也不保证绕过 Cloudflare。FlareSolverr 可选且仅 challenge-only；service URL 与 upstream proxy 独立于 native FANBOX proxy。默认 config generator 不创建这些可选 table。

`[account_pool]` 只保存 `enabled` 与 `strategy`；每个账号的 `schedulable`、冻结和 marker 状态位于 `pixiv-cli.db`。旧 `account_pool.accounts` 会一次性迁移为数据库标记后删除，并保留该表其他内容。不要把 refresh token 写入 `config.toml`。历史 `data/account-pool.json` scheduler 不会被自动读取、迁移或删除。旧 `[logging]` 表会为兼容性被忽略；`log_level` 不是受支持的 `pixiv config` 键。

v1 CLI 不会读取或迁移旧的 `~/.pixiv-cli/auth.json`。从旧版本切换前，请在旧 CLI 执行
`pixiv auth export --all --output <private bundle>`，再在 v1 执行 `pixiv auth import --file <bundle>`。
迁移必须显式完成，旧文件不会成为隐式 credential 来源。

### 环境变量

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | 空 | 受支持的 public SDK/MCP runtime 凭据输入；CLI 数据命令会忽略它。 |
| `DOWNLOAD_PATH` | `./downloads` | 下载目录。 |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | 文件名模板。 |
| `DIRECTORY_TEMPLATE` | 空 | 相对下载目录模板。 |
| `PIXIV_REQUEST_INTERVAL` | 空 | 请求起始间隔。 |
| `https_proxy` / `HTTPS_PROXY` | 空 | `http`、`https`、`socks5` 或 `socks5h` 代理 URI；优先使用小写 `https_proxy`。 |

CLI 数据命令在账号池关闭时使用 `pixiv auth use` 的显式/默认账号，启用时从数据库选择 eligible 账号；不接受身份选择参数，也不读取 `PIXIV_REFRESH_TOKEN`。

设置类字段按 service 分域：命令 `--proxy URL`/`--no-proxy` > 对应 service proxy（含显式空值） > `https_proxy`/`HTTPS_PROXY` > `[network].https_proxy` > direct。代理覆盖不会持久化；update 只使用通用 network fallback，不消费 FANBOX 或 solver 配置。

### Debug 诊断

可在命令前后传入 `--debug`，观察安全的生命周期、账号池、网络、challenge、solver、下载与错误事件：

```bash
pixiv --debug detail 123456
pixiv fanbox --debug post 12221352
pixiv --debug mcp 2>debug.log
```

每行都写入 stderr，带明确的产品+子系统模块和完整英文句子；不会创建 `logs/`、daily file、JSON event stream，也不会写 raw URL、Cookie、token、signed query、proxy userinfo 或 clearance。stdout 与 MCP JSON-RPC 不变。`pixiv auth export` 即使带 `--debug` 也不创建诊断 scope，因此 raw-token/bundle stdout 与空 stderr 契约保持原样。unknown option 会在 scope 创建前以 exit code `2` 报告。

### 移除的匿名 web fallback

v1 已删除匿名 Web API fallback。内容命令要求先通过 `pixiv auth use` 或启用数据库
`[account_pool]` 选择已认证的本地账号；否则返回认证要求。已删除的
`web_fallback_enabled` 配置若仍存在于 `config.toml` 会返回 `removed_setting`，
可用 `pixiv config unset web_fallback_enabled` 清除。

无效 token 与 App API 网络或服务器错误会返回安全的、已分类的失败。

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

未显式使用 `--proxy`、已配置的 `https_proxy` 或 `HTTPS_PROXY` 时，Release binary 更新会并发探测内嵌 source 列表。支持 API 的候选用于 GitHub Releases API；支持 archive 的候选用于签名 manifest、checksum 与平台 archive。首个有效响应成为首选路由；某个 asset 下载失败时会静默依次尝试其余已声明路由各一次，全部失败才会在错误中列出每条失败路由。候选不会改变规范 Release URL、SemVer 选择、Ed25519 验证或 SHA-256 验证。自动更新通知只使用支持 API 的候选，并保持原有的三秒总时限和 24 小时缓存。

更新检查只选择 canonical SemVer tag。stable 检查先排除 GitHub 已标记的 prerelease；
`--prerelease` 则将其纳入当前通道。若当前通道的任一非 draft published Release 使用非
SemVer tag，检查会报告该 tag 并 fail-closed。

受支持 binary 已内置 production Ed25519 public key/key ID/fingerprint；私钥只保存在受保护的
`release` Environment 与受控 macOS Keychain 恢复副本。当前已发布的受签名 Release 请以
[GitHub Releases 页面]为准；`pixiv update --check` 仍只是只读检查，不能替代对选中版本资产、checksum 与
签名的安装验证。

普通 CLI 命令成功后会尽力检查 stable 更新。它跳过 MCP、help、`version`、`update`、全部 `auth export`、`auth import --file` 与开发构建，
对同一用户 cache 最多每 24 小时查询一次，并为自动检查设定最多 3 秒的等待时间。发现新版本或
检查失败只写 stderr（失败为 warning），不改变业务命令退出码，也不会污染 JSON stdout 或 MCP
JSON-RPC stdout。可关闭自动检查：

```bash
# ~/.pixiv-cli/config.toml
[update]
check_enabled = false
```

## 相关文档

本参考手册只定义 CLI 边界；其他接口与维护流程以对应权威文档为准：

- [Go SDK](sdk.md)：public client、模型、分页、资源和 typed error。
- [MCP tools](mcp-tools.md)：tool 名称、输入 schema、输出和 stdio 行为。
- [架构](../maintainers/architecture.md)：包职责和运行流程。
- [开发流程](../maintainers/development.md)：环境、测试、构建和发布门禁。
- [Agent skill](../../skills/pixiv-cli/SKILL.md)：供 Agent 安全驱动已安装 CLI 的说明。
