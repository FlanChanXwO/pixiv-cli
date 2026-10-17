# 开发流程

[English](../../en/maintainers/development.md) | 简体中文 | [文档索引](../../index-zh-CN.md)

| 要做的事 | 从这里开始 |
| --- | --- |
| 检查本地工具链 | [环境检查](#环境检查) |
| 构建或测试 ugoira 编码 | [Ugoira Go encoder](#ugoira-go-encoder) |
| 运行 CLI 与 MCP | [运行](#运行) |
| 处理登录和凭据 | [获取 refresh token](#获取-refresh-token) |
| 选择测试范围 | [测试](#测试) |
| 检查 release workflow | [发布门禁、签名与 Homebrew 边界](#发布门禁签名与-homebrew-边界) |
| 准备版本说明 | [Release notes and publication](#release-notes-and-publication) |

## 环境检查

项目是 Go module，所需 Go toolchain 版本只以 `go.mod` 为事实来源。

开工前建议检查 Go 与常规测试环境：

```bash
go version
go env GOVERSION CGO_ENABLED GOOS GOARCH
CGO_ENABLED=0 go test ./...
```

## Ugoira Go encoder

默认下载器构造 `ugoira.NewEncoder()`，在 Go 中生成 GIF/APNG，不调用 `ffmpeg`。PNG 帧使用 Go 标准库解码器，
JPEG 帧使用 owner-local `internal/media/ugoira/jpeg` 解码器。

每帧的 ZIP member 源字节与解码前内存估算分别采用 512 MiB 边界，继承 pinned Rust `image` crate 的
`Limits::default().max_alloc`。声明大小超限时在读取前失败，实际展开字节超限时在分块读取中失败；
无效或溢出的尺寸、超限的解码估算也会返回错误，不截断输入或回退到其他后端。每个读取块前后以及
image decode 前后均检查取消。PNG/JPEG 解码循环没有取消回调，所以单帧解码期间发生的取消要等
该次解码返回后才能观察到。

源码构建使用 `go.mod` 声明的 Go 版本。`scripts/build.sh` 显式设置 `CGO_ENABLED=0`，输出 `build/pixiv`（Windows 为 `build/pixiv.exe`），不需要 Rust 工具链、native library 或 C linker：

```bash
sh scripts/build.sh
CGO_ENABLED=0 go test ./internal/media/ugoira/... -count=1
```

只有 race-detector 测试需要 `CGO_ENABLED=1` 和可用的 C 工具链。编码器没有后端选择选项或运行期 fallback。

`scripts/build-platform.sh` 负责精确 target 的 CGO0 构建和可选规范归档。参数为 `--target GOOS/GOARCH`、`--output-dir DIR`、可选 `--version VERSION`（不带 `v`）与 `--package`（要求 version）。它保留 Windows executable 后缀和 Linux ABI gate。runner/capability metadata 仍在 `ci/platforms.json`；测试、source identity、权限、审批和发布仍由 workflow 拥有。

Linux Release 的公开 ABI 基线是 glibc 2.35。release test/production、native evidence、packaged
binary smoke 与 Homebrew install matrix 的 Linux runner 必须固定为 `ubuntu-22.04` 和
`ubuntu-22.04-arm`；quality、validate、publish 等不产出 Linux binary 的 job 可继续使用更新 runner。
每次生成 Linux executable 后必须运行：

```bash
go run ./scripts/cmd/linuxabi --binary <linux-elf>
```

该门禁读取 ELF `SHT_GNU_verneed` 的真实 loader contract，并兼查 imported symbol version；任何高于
`GLIBC_2.35` 的依赖都会在打包前失败。不能只依赖“binary 在构建 runner 上能运行”的 smoke，因为这会
让 runner 自身的新 glibc 隐藏向后兼容回归。

release archive 的 `LICENSE`、`THIRD_PARTY_LICENSES.md` 与 `third_party/licenses/**` 在 `.gitattributes` 中继续固定 `text eol=lf`，使归档审计和逐字节许可证检查在 Windows 保持稳定。

### 历史 Rust 来源

已删除的库来自 native run `29567721284`，它取代了 run `29192425899` 的首次收集。历史 compiler 为：`x86_64-apple-darwin` 与 `x86_64-pc-windows-msvc` 使用 Rust `1.96.0`；`aarch64-apple-darwin`、`aarch64-pc-windows-msvc`、`x86_64-unknown-linux-gnu` 与 `aarch64-unknown-linux-gnu` 使用 Rust `1.96.1`。源码、vendor、库与 manifest 保留在 Git 历史中，不再是当前构建输入；独立移植归属仍按下节保留。

### 许可证 bundle

`go run ./scripts/cmd/licensebundle` 对固定六个 release target，以 `CGO_ENABLED=0` 运行
`go list -mod=readonly -deps -json ./cmd/pixiv`，取实际生产 Go module 的并集。解析离线执行
（`GOPROXY=off`、`GOSUMDB=off`），不受本机 workspace/flag 覆盖影响；所需模块源码必须已缓存。
它不使用 Cargo，也不纳入未进入生产图的 test/tool-only module。`--check` 对比索引和完整生成正文树，不发布改动。

`third_party/attributions.json` 固定额外的移植来源声明，维护用正文放在 `third_party/attributions/`，
与生成的 `third_party/licenses/` 分开。选用的 MIT 正文、Go BSD/patent grant 与 SunPro 声明在删除
Rust 源码后仍保留。`fhttp v0.6.8` 的模块归档缺少独立 license 文件，因此以精确版本的声明条目提供其
源码头部引用的 Go BSD 正文。缺失或空正文会在发布前失败。嵌套组件的声明同样收录；上游 CRLF 统一为
LF，适配现有归档 checkout 契约，保留完整文字。索引与正文树继续沿用可回滚发布及现有 release archive 成员契约。

### Native runner evidence

`.github/workflows/native-evidence.yml` 仍是要求 `refs/heads/main` 的显式维护入口，不在普通 main push 后重复执行。全局权限为空，job 仅 `contents: read`，action 固定 full SHA，checkout 不持久化 credential，不使用发布或签名 Environment。runner 来自 registry 的 `native-evidence` capability；每个目标验证 Go license bundle，经 `scripts/build-platform.sh` 构建，再执行 Go GIF/APNG 回归并上传版本化 binary/archive record。

schema 3 绑定 workflow 的精确 `source_commit`、target、binary version 与 binary/archive/member SHA-256；执行 `--version`，逐项校验归档中的 binary、根 notices 和完整 license tree。旧 schema 与 Rust source-digest/staticlib 字段会被拒绝。`consolidate` 复验完整六目标、同 source、同 version 的 bundle，拒绝 symlink 与不安全 file/directory path，再原子发布含各 target bundle 和 `summary.json` 的全新目录，不向源码回填库。

从同一审计 native-evidence run 设置 `RUN_ID` 与 `SOURCE_COMMIT`，下载其六份 artifact，再使用新的绝对路径：

```bash
go run ./scripts/cmd/nativeevidence consolidate \
  --repo-root . --expected-version "v0.1.0-native-evidence.${RUN_ID}" \
  --expected-commit "$SOURCE_COMMIT" --input-dir /absolute/evidence \
  --output-dir /absolute/new-complete-evidence
```

PR Platform worker 在 native 与解包 binary smoke 后也上传 schema 3 `native-evidence-<goos>-<goarch>` bundle，版本为 `v0.2.0-ci.<run-id>`。producer 仍是受信任的 base-ref workflow；修改 PR head workflow 不会更新该 producer。必须核对实际 run 与精确 head，不拼接不同 run，也不把 fixture、cross-build 或 workflow 定义当作六平台原生执行。

`.github/workflows/browser-evidence.yml` 是另一条显式手动触发、credential-free 的原生 provider contract matrix，
在 macOS、Linux、Windows 的 amd64/arm64 runner 上执行 `internal/browsercookies/...` 的平台代码与合成 fixture 回归，
普通 `main` push 不再附带重复运行该矩阵。
GitHub Windows runner 不提供该 contract 需要的 `sqlite3` CLI，因此两个 Windows job 都通过
`scripts/install-browser-sqlite.ps1` 安装与架构匹配的 SQLite 3.53.4 官方 tools 包，并在原有 SQLite preflight
之前用固定 URL 与 SHA-256 校验下载内容。聚焦的 test-only workflow contract 只锁 credential、full-SHA action、
fixture、固定 SQLite provisioning 与 cleanup 等安全边界，
不再由生产代码重新实现整份 workflow；`scripts/cmd/browsernativeevidence firefox-contract` 仅保留为
隔离 Firefox profile/schema 的真实运行时 helper。`firefox_native` job 只在 runner 临时目录解包官方包，
让 Firefox 生成隔离 profile/schema，再注入明确的 synthetic cookie 运行 provider contract；它不读取
用户浏览器 profile、Keychain、DPAPI 或 Secret Service，也不上传 package/profile/database。真实
profile/session evidence 仍只能在受保护的 release-prep host 取得，不能把该 workflow 的成功当作真实
用户浏览器导入成功。

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
pixiv auth use 12345678
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./build/pixiv mcp
```

MCP 使用本地 `auth use` 选定的 Pixiv 账号；refresh token 不属于配置文件或环境变量入口。

如网络环境需要代理，可额外设置：

```bash
https_proxy=http://127.0.0.1:7890 ./build/pixiv mcp
```

或只给本次启动覆盖代理：

```bash
./build/pixiv mcp --proxy http://127.0.0.1:7890
./build/pixiv mcp --no-proxy
```

CLI 的认证、配置、回调桥接、Release 检查缓存与 callback helper 都位于当前用户主目录下的 `.pixiv-cli`。

### 本地路径与权限

- macOS/Linux：`~/.pixiv-cli`；Windows：`%USERPROFILE%\.pixiv-cli`。
- 账号凭据保存在 `pixiv-cli.db`（SQLite，账号 key 是 Pixiv UID / FANBOX UID）。
- 内置 migration 将数据库推进到 schema v3。旧 v1 数据库会原地升级；若初始 schema 已包含后续字段，则只记录对应迁移而不重复执行冲突 DDL；未知的更新 schema 仍会 fail closed。
- 全局配置保存在 `config.toml`。
- Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。

> [!WARNING]
> 新版本不自动读取或删除旧 `auth.json`。跨版本迁移须在旧版本执行 `pixiv auth export --all --output <private bundle>`，再通过 shell 重定向或管道在新版本执行 `pixiv auth import < bundle.json`。

### 登录方式

推荐使用 `pixiv auth login` 通过本地 loopback server 和浏览器 OAuth 登录。服务器同时配置 `login_relay_public_url` 与 `login_relay_listen_addr` 时，会输出一次性远程 handoff URL，并直接转交已安装 pixiv-cli 的 desktop handler 完成登录，不渲染项目中间页或手动 callback 表单。

其他登录入口：

- 已有 raw token 可用 `pixiv auth import` 输入。
- 账号备份使用 `auth export` 与 `auth import < bundle.json`。

### 配置管理

`pixiv config path/get/set/unset` 管理 `account_pool_enabled`、`account_pool_strategy`、`download_path`、
`filename_template`、`directory_template`、`request_interval`、`https_proxy`、`log_level`、`log_format`、
`reverse_search_provider`、`reverse_search_pixiv_only` 与 `saucenao_api_key`。
其余高级 TOML 由用户手工维护。尤其是 reverse-search transport 与 challenge recovery 位于
`[reverse_search.network]` 和 `[reverse_search.flaresolverr]`；这些 table 不会由 baseline config 生成，并在启动时
读取为 snapshot。首次配置 bootstrap 由 `internal/config/settings` 的**字段标签**驱动，用 `tomledit` 自动生成精简文件，
只落盘标记 `example:"true"` 的默认项，且绝不覆盖已有文件。

新增一项**普通配置**（可由 `config get/set/unset` 管理、或仅从 `config.toml`/环境变量读取）只需三步：

1. 按下方链接的标签约定，在 `internal/config/settings/config.go` 的 `RuntimeConfig`（或嵌套配置组）添加字段与声明。
2. 补一个聚焦的行为测试（默认值/来源/可见性或 schema 错误）。
3. 更新对应文档。

不需要再同步注册表、环境变量 `switch`、`Runtime()` 的逐字段赋值、初始文件清单或 CLI 别名清单。
标签约定与各文件职责的完整说明见[架构说明的配置声明一节](architecture.md#配置声明结构体字段标签是唯一事实来源)。

> [!NOTE]
> 已删除的 `[web] fallback_enabled` 若仍存在会返回 `removed_setting`，用 `pixiv config unset web_fallback_enabled` 清理。`[logging].level`（`info|debug`）与 `[logging].format`（`text|json`）是启动时生效的配置；`PIXIV_LOG_LEVEL` 与 `PIXIV_LOG_FORMAT` 覆盖文件值。

### Flag 解析

CLI 使用 Cobra/pflag，flag 可以写在位置参数前后；例如 `pixiv auth check 12345678 --json` 和 `pixiv search "初音ミク" --json` 都受支持。

Pixiv command proxy、`[pixiv.network]`、环境变量与 `[network]`，FANBOX 独立的
`[fanbox.network]`/`[fanbox.flaresolverr]` 配置，以及 reverse-search 的
`[reverse_search.network]`/`[reverse_search.flaresolverr]` 配置，均按各自服务边界解析。reverse search 有
standard source/SauceNAO、ascii2d browser 与 FlareSolverr JSON-control 三个独立网络面；FlareSolverr 仅用于
challenge recovery。

## 获取 refresh token

浏览器 Cookie（包括 `refresh_token=...`、`PHPSESSID`、`device_token`）不是可接受的 Pixiv App API OAuth refresh token，CLI、MCP、环境变量、SDK 与已存账号都会拒绝这类输入。推荐直接登录并保存账号：

```bash
pixiv auth login
```

| 项 | 说明 |
| --- | --- |
| 本地服务 | CLI 生成 PKCE/state，并启动本地 loopback HTTP server。 |
| 浏览器 | macOS 与 Windows 的普通 CLI 启动会准备当前用户的 persistent `pixiv://` callback helper；本地登录打开默认浏览器，因此可复用已有 Pixiv 登录态；`--no-open` 可改为只打印登录 URL。 |
| 回调接收 | CLI 接收本轮 loopback callback、一次性 desktop handoff 和本地页面表单。远程 handoff 不提供手动 callback 回填。 |
| state 校验 | 本地 loopback 回调必须匹配本次 state；Pixiv 官方 callback URL 与 `pixiv://account/login` 可在 Pixiv 未返回 state 时作为显式 fallback。 |
| token 保存 | refresh/access token 不打印；refresh token 按 Pixiv UID 写入 `pixiv-cli.db`。legacy `auth.json` 不属于新 CLI 的读取、迁移或删除路径；跨版本迁移必须由旧 CLI 显式导出 bundle，再由新 CLI 显式导入。Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。 |

本地登录的 active loopback bridge 优先接收 Pixiv 返回的 `pixiv://account/login?...`，并把 callback 交给本轮 CLI listener；OAuth exchange 完成后，浏览器显示固定的结果页。跨机器登录时，server 启动后只显示一次性 handoff URL；浏览器打开后直接转交 `pixiv://account/remote-login`，本机领取本次 OAuth URL，并把 callback 回传同一会话；本地只保存本次 handoff state，新的 handoff 会替换旧状态。远程 flow 需要已安装 CLI 的 desktop handler，不提供移动端手动回填。server 会核验提交内容属于本次会话且为官方 callback；Pixiv 带有 state 时必须匹配，再由本次 PKCE verifier 完成 exchange。`pixiv auth devices` 已移除；已有 `remote-devices.json` 会被忽略。HTTP 与 HTTPS 都可用于 relay；direct TLS 和同机 TLS reverse proxy 都受支持。旧 `login_relay_secret` 与 `login_relay_target_url` 配置会被静默忽略。

浏览器使用的系统代理不会自动传给 Go CLI。`https_proxy`、`--proxy` 与更新路径都接受 `http`、`https`、`socks5`、`socks5h` URI。若 Pixiv token exchange 需要代理，请配置 `pixiv config set https_proxy socks5h://127.0.0.1:7890`，在单次命令前设置 `https_proxy=...`，或对网络命令使用运行期覆盖 `--proxy socks5h://127.0.0.1:7890`。`--no-proxy` 会清空本次命令的代理，即使环境变量或 `config.toml` 设置了 `https_proxy`；`--proxy` 和 `--no-proxy` 不能同用，也不会写入 `config.toml`。请求节奏通过 `PIXIV_REQUEST_INTERVAL` 或 `[network].request_interval` 配置。debug 诊断通过 `pixiv config set log_level debug`，可选 `pixiv config set log_format json`，只写 stderr 且只在启动时读取。

当前支持代理覆盖的网络入口是 direct-token `auth import`、`auth login`、`auth check`、`search`、`timeline`、`detail`、`ranking`、`recommended`、`download` 和 `mcp` 启动。bundle-form `auth import` 明确拒绝代理 flag；`auth export/list/use/remove` 与 `config path/get/set/unset` 不接受这些 flag。

### 认证 import/export

> [!WARNING]
> 以下 secret 边界是硬约束：token 只允许在显式、不带 `--output` 的 `pixiv auth export` 写 stdout，其他路径不得暴露 secret；bundle 是未加密、含 secret 的 point-in-time backup，不是 live sync。

`pixiv auth import [REFRESH_TOKEN]` 会经 App OAuth 校验输入并保存 rotation 后的 token。位置参数会进入 argv/shell history；无参 TTY 使用隐藏输入，无参非 TTY 读取完整 stdin，并按首个非空白字节自动区分 raw token 与 versioned bundle。bundle 严格 decode、完全离线地 merge 并原子写回；失败不得回退 OAuth，且与位置 token、`--proxy`、`--no-proxy` 冲突。restore 保留已有 default，仅当本地尚无 default 时采用 bundle default。

`pixiv auth export [UID]` 省略 UID 时选择默认账号；不带 `--output` 时只向 stdout 写 raw token 与换行。`pixiv auth export --all` 不带 `--output` 时只向 stdout 写 versioned secret bundle。两者是唯一 secret stdout 例外，且都只读本地 store，不刷新、不联网、不修改状态，并跳过 startup pending-update cleanup 与 automatic update。`--output PATH` 总是写 bundle，默认拒绝覆盖，只有 `--force` 可 replacement；stdout 仅为 path/account count 摘要。其他 stdout、stderr、JSON、MCP result 与错误仍不得暴露 secret。

bundle 是未加密、含 secret 的 point-in-time backup，不是 live sync；token rotation 后旧 bundle及其他机器副本可能 stale。任意目标 export writer 在 Unix-like 使用 `0600` 文件且不改变既有 parent；Windows 明确设置 owner 与 protected DACL，只授权当前用户、LocalSystem、builtin Administrators。Windows 行为有 CI tests，后续验收可本地交叉编译；这里不声称已在真实 Windows 主机执行。

restore 原子写失败时检查 public `LocalWriteCommitOutcome`：pre-commit 是 `not_committed`；replacement 后 durability/cleanup 失败是 `committed`，须重新加载确认；recovery 状态无法确定是 `unknown`，须人工核验。不得把 `committed` 或 `unknown` 描述为成功 rollback。

真实登录依赖 Pixiv OAuth 网页流程可用。自动化测试使用 fake OAuth server 覆盖 callback 和 token exchange，不访问真实 Pixiv。

## 测试

当前测试覆盖 CLI 命令与 build metadata、显式/自动更新、`internal/services/{pixiv,fanbox}/account` 账号服务、`internal/services/pixiv/pool` 账号池、`internal/services/reversesearch` source/provider fixture 与聚合、`internal/storage/database` 认证存储与 `internal/config/settings` 配置、`internal/shared/lifecycle` 生命周期、`internal/shared/pagination` 逻辑分页、`internal/shared/traversal` 泛型可重入遍历、Pixiv App API 认证重试、公开 SDK（`sdk`/`sdk/pixiv`/`sdk/fanbox`）、HTTP client wiring、下载管理、Go encoder/native evidence 合约和 `internal/mcpserver/{pixiv,fanbox}/tools` tool 注册。`internal/account` 与 `internal/session` 已删除，不保留兼容测试入口。测试文件布局与 same-package 例外见[测试文件布局](#测试文件布局)：

```bash
go test ./...
sh scripts/build.sh
# 浏览器 provider 的离线 fixture/crypto/权限分类回归；真实跨平台 host evidence 另按 release-prep 执行。
go test ./internal/browsercookies/... -count=1
# 真实 SDK e2e 需要本机凭据（Pixiv 读本地 pixiv-cli.db 选中账号，FANBOX 读 Keychain）：
PIXIV_E2E_READ_USER_ID=<secondary-uid> \
PIXIV_SDK_E2E=1 go test ./e2e -run TestRealPixivSDKRead -count=1 -v
FANBOX_E2E_CREATOR_ID=<non-secret-creator-id> FANBOX_E2E_TAG=<non-secret-tag> \
FANBOX_E2E_POST_ID=<non-secret-post-id> FANBOX_E2E_POST_URL=<non-secret-post-url> \
FANBOX_SDK_E2E=1 go test ./e2e -run TestRealFanboxSDKRead -count=1 -v
# 若 native 请求触发真实 challenge，可额外显式开启 recovery；默认不配置。
FANBOX_E2E_SOLVER_URL=http://127.0.0.1:8191 \
FANBOX_E2E_SOLVER_PROXY=http://host.docker.internal:7890 \
FANBOX_E2E_CREATOR_ID=<non-secret-creator-id> FANBOX_E2E_TAG=<non-secret-tag> \
FANBOX_E2E_POST_ID=<non-secret-post-id> FANBOX_E2E_POST_URL=<non-secret-post-url> \
FANBOX_SDK_E2E=1 go test ./e2e -run TestRealFanboxSDKRead -count=1 -v
# 单帖 post.info 验收；只需要 post id/page URL，允许合法的零文件资源详情。
FANBOX_E2E_POST_ID=<non-secret-post-id> FANBOX_E2E_POST_URL=<non-secret-post-url> \
FANBOX_SDK_E2E=1 FANBOX_E2E_POST_ONLY=1 go test ./e2e -run TestRealFanboxSDKPostInfo -count=1 -v
# 显式观察反向搜图上游兼容性；默认不会运行。
# 请预先从私有环境 export SAUCENAO_API_KEY，不要内联在命令行中。
export SAUCENAO_API_KEY
PIXIV_REVERSE_SEARCH_E2E=1 \
PIXIV_REVERSE_SEARCH_SOURCE=<private-test-image-path-or-url> \
PIXIV_REVERSE_SEARCH_PROVIDER=all \
go test ./e2e -run TestRealReverseSearch -count=1 -v
```

没有独立的 E2E wrapper 脚本：真实联网观测直接运行上方的 `go test` 命令，使离线套件与带凭据观测共用同一条 orchestration。

`go test ./...` 保持默认离线稳定；真实 SDK e2e 在未显式设置 `PIXIV_SDK_E2E=1` 或 `FANBOX_SDK_E2E=1` 时跳过。
显式启用后，缺少本机授权凭据或 FANBOX 非 secret target 会直接失败并暴露缺口，不会以 skip 伪装 release evidence。

反向搜图 provider fixture 以及 CLI/MCP/config 回归属于离线套件。真实反向搜图网络观察只有显式设置
`PIXIV_REVERSE_SEARCH_E2E=1` 才运行，必须提供 source；provider 为 SauceNAO 或 `all` 时还必须提供
`SAUCENAO_API_KEY`，仅 ascii2d 时不要求 key。脚本不接受 source/key 参数，也不会回显它们，只应配合获授权的
测试图片运行。它用于观察第三方兼容性，不是默认 release 门禁；skip 或上游不可用不能被报告成真实网络成功。

独立的 `TestRealReverseSearchMCPReusesSolverSession` 检查在显式启用时还要求
`PIXIV_REVERSE_SEARCH_SOLVER_URL`；可选的 `PIXIV_REVERSE_SEARCH_SOLVER_PROXY` 只表示 FlareSolverr browser
upstream proxy，不代理 solver control request 或 native ascii2d image upload，solver session state 和 source 也不会
作为 test evidence 持久化。

上方的 `PIXIV_SDK_E2E=1` / `FANBOX_SDK_E2E=1` 命令只选择当前的 public SDK E2E 测试：Pixiv 测试从本地 `pixiv-cli.db` 读取选中账号；`PIXIV_E2E_READ_USER_ID` 是可选的非 secret 本地账号 selector，显式提供时必须命中已保存 Pixiv 账号，格式错误或账号不存在都会在联网前 fail closed，且不会 fallback 到 configured default；release evidence 应显式选择获授权的 secondary account，省略时才保留 configured-default 兼容行为。
FANBOX 测试从约定的 macOS Keychain item 读取 `FANBOXSESSID`。FANBOX 的
`FANBOX_E2E_CREATOR_ID`、`FANBOX_E2E_TAG`、`FANBOX_E2E_POST_ID` 与 `FANBOX_E2E_POST_URL` 只接受
显式、非 secret 的测试目标；不接受 refresh token、session 或完整 Cookie 作为参数/环境变量。可选的
`PIXIV_E2E_PROXY` 只表示非 secret 的代理 URI；`FANBOX_E2E_SOLVER_URL` 与
`FANBOX_E2E_SOLVER_PROXY` 是可选的非 secret recovery 拓扑配置，默认不启用 solver。未显式启用真实 E2E 时测试默认 skip；显式启用但缺少本机
凭据或 FANBOX 目标时会失败，不能把默认 skip 或自动发现记为 release evidence。

v1 的真实 SDK E2E 是 `TestRealPixivSDKRead` 与 `TestRealFanboxSDKRead`（见 [测试](#测试) 的 `PIXIV_SDK_E2E=1` / `FANBOX_SDK_E2E=1` 命令）。Pixiv 侧测试进程只从本地 `pixiv-cli.db` 的选中账号读取 refresh token；release-prep 应设置 `PIXIV_E2E_READ_USER_ID` 显式选择获授权的 secondary account，避免隐式使用 configured main/default account。随后打开 `sdk/pixiv` 验证 identity 并完成一个稳定 detail/list 与 `Resource` 读取，rotation 后的 credentials 先按正常 repository transaction 持久化再继续内容请求。FANBOX 侧直接通过 macOS Keychain 读取授权 `FANBOXSESSID` item，并使用显式 creator/tag/post/page URL 目标逐项验证 `Creator`、`Creators`、`CreatorTags`、`CreatorPosts`、`TaggedPosts`、`Post`、`Home`、`Supporting`、`ResolveURL`、`OpenResource` 与 `SaveResource`；列表目标在服务端返回 cursor 时各跟进一次 continuation，帖子详情必须发现 file attachment 并在临时目录完整读取。session 失效时明确报 `credentials_expired` 并要求重新导入，不 fallback。release-prep 运行后由操作者扫描 stdout、stderr、test log 与 evidence；token、Cookie、signed URL 与原始 response body 不得进入 argv、环境 dump、日志、test name、artifact 或失败 diff。以上说明描述测试覆盖，不表示真实 e2e 已经运行；请勿把 token 写入 shell history、日志或仓库文件。

对于合法但没有 file attachment 的文章详情，补充使用 `TestRealFanboxSDKPostInfo`：它只要求显式
post ID/page URL，验证公共 SDK 的 `Post`、非空 body、`ResolveURL` 与资源清单，并允许
`file_assets=0`；它不能替代严格资源路径的 `TestRealFanboxSDKRead`，严格路径会对详情中的每个
file attachment 完成 HEAD、完整保存和字节数核对。

显式代理下，资源传输固定协商 HTTP/1.1，而 App API、OAuth 保持原有协议协商。该 e2e 的资源读取用于回归这一资源传输边界；它不为慢速正常下载增加固定超时。若 Pixiv 返回不带有效 `Retry-After` 的 429，真实 e2e 保留诊断并明确失败，不会猜测等待或无限重试。

`PIXIV_E2E_BINARY` 与 `PIXIV_E2E_EXPECTED_VERSION` 供 CI 对已构建、已解压的 release binary 执行离线 e2e；它们不注入 token，也不启用真实 Pixiv API。`platform-smoke.yml` 在六个受支持 runner 上构建、封装、解压并运行这组 CLI/config/MCP stdio 验证。

代码改动完成前，应按变更范围补充或更新测试。若不能运行测试，需要在交付说明中写明原因和风险。

发布相关的本地 fixture/策略门禁还包括：

```bash
sh scripts/test-build-platform.sh
sh scripts/test-package-release.sh
go test ./tools/release ./tools/platformmatrix -count=1
go test ./scripts/internal/nativeevidence -count=1
go test ./scripts/internal/browsernativeevidence -count=1
sh scripts/test-homebrew-formula.sh
git diff --check
```

fixture 只证明格式、失败语义和本地策略，不替代六个 native runner 的真实静态链接、GIF/APNG
smoke、版本化 archive 内容和 Homebrew 安装验收。

`.github/workflows/ci.yml` 只承载只读的 `Quality gate`，只响应 `pull_request` 与 `workflow_dispatch`，不再响应 tag push。它的 change-scope classification 与 `pr-metadata.yml` 使用同一信任模型：先解析受保护 base branch 的当前 tip，从该 tip checkout 出 `scripts/classify-change-scope.sh` 与 `.github/ci-change-scope.gitignore`，再 fetch 精确 PR HEAD 只用于 diff 范围，因此 PR 无法修改自己的 skip 判定；分类器失败 fail closed，不会退化为 skip。`.github/workflows/pr-metadata.yml` 是受信的 `pull_request_target` coordinator：从当前 base tip 验证 PR 模板与 verification declaration，使用 `.github/ci-change-scope.gitignore` 对精确 PR-head diff 做可信分类，发布 `PR template gate` / `PR commands gate` commit status，dispatch 所需 smoke worker，并把 `Platform smoke` / `Container smoke` 作为真实 job-level required check 暴露出来；coordinator job 本身不执行 PR 代码。独立的 `Platform smoke worker` / `Container smoke worker` Check Run 只作为由 base-ref worker workflow 完成的可信结果桥接，required smoke job 等待这些结果。普通 pattern 表示纯文档，`?pattern` 表示只需 Quality，`!pattern` 表示需要 Quality + Platform + Container；`pr-metadata.yml` 是 smoke controller（它决定分类、dispatch 两个 worker 并持有 required gate），因此必须使用 `!pattern`。不需要的 Quality 与 smoke job 都由 GitHub Actions 原生显示为 `Skipped`，不再伪装成 success。仅修改 PR body 时也会重新对未变化的 head 做可信分类：需要 smoke 的 job 复用同一 head 已有的 worker 结果，真正不需要 smoke 的 job 才继续显示 `Skipped`，因此编辑 PR body 不能把失败 smoke 替换成可绕过的 skip。

`pr-metadata.yml` 的状态维护只操作 PR，使用 `pull-requests: write`，不申请普通 issue 写权限。读取当前 PR、读取状态评论和创建/更新/删除评论失败时，日志保留 GitHub API 原始错误并标明失败请求；元数据失败仍阻断 required smoke gate。实际 workflow shell 的 API fixture 只在 Linux 运行，匹配 Ubuntu runner 的工具环境；权限契约测试仍跨平台执行。

Smoke gate 按精确 PR head 与 worker 名称使用 `filter=all` 查询并遍历分页，按 `started_at` 选择最新 check，同一时间以 ID 决定先后，不依赖响应顺序。PR 更新后，`filter=latest` 可能隐藏其他 check suite 的 worker；worker 缺失或失败时仍阻断 gate。

Platform worker 从受信 workflow ref 解析六平台 matrix，只有 matrix job checkout 精确 PR head，并且 token 仅为 `contents: read`；独立 publish job 不 checkout PR，只持有完成内部 worker Check Run 所需的最小 `checks: write`。Container worker 对 Linux amd64/arm64 使用同样的隔离模型。六个原生 job 继续并行运行，其中 Windows worker 仍承担 root callback wiring 与原生 `loginhelper` 契约；两个容器 job 也继续并行。内部 matrix 不作为 required PR check，最终由对应的 PR gate job 镜像 aggregate worker 结果，失败时保留 worker details URL。普通分支与 `main` push 不运行 CI；稳定 `vX.Y.Z` tag push 只运行 `release.yml`（Quality gate 不再响应 tag，tag 上的正式门禁由 Release 独占），Release 自己执行正式六平台测试/构建与两平台容器验证，因此 tag 不重复 PR smoke matrix。`pr-verification.yml` 的 `dispatch` job 在分配 runner 前先用 `contains(github.event.comment.body, '/test')` 做廉价预过滤：它是 `tools/prmeta --check-trigger` 的宽松超集，只产生少量 false positive，不会漏掉合法触发，最终授权仍由 `tools/prmeta` 判定。browser/native evidence 保留为显式维护入口。真实 Pixiv/FANBOX SDK E2E 不进入普通 PR CI；仅发布 tag 的 `release.yml` 在 validate 后运行无凭据 SDK E2E contract gate，真实 SDK E2E 仍按 release-prep 在授权环境独立验收。

`scripts/tests/installers` 使用本地伪 Release、伪 `curl` 与 checksum fixture 验证安装器，不访问 GitHub。Unix
job 实际运行 `install.sh`，覆盖 SHA-256、带空格目录、版本预检和校验失败不覆盖旧 binary；Windows
amd64/arm64 platform-smoke 还会用真实 `cmd.exe`、`certutil.exe` 与 `tar.exe` 运行 `install.cmd`，并始终
传入 `--no-path`，因此测试不修改 runner 用户注册表。platform-smoke workflow 会直接运行这项 installer contract，作为平台 job 的一部分；这里不存在另一套 runtime workflow policy 实现。

其中 Windows 的 `.zip` 由 GitHub runner 镜像预装的 `7z` 生成；其他平台继续使用 `zip`。
`scripts/test-package-release.sh` 会在 Windows runner 把伪造的调用委托给真实 `7z`，在其他开发机使用
`zip` fixture，并核对 archive member；因此 Git Bash 缺少 `zip` 会在 release test gate 直接暴露。
它用 MSYS 的 `winsymlinks:nativestrict` 创建受检链接：若 runner 不能创建原生 Windows link，测试会显式
失败，避免 Git Bash 的普通文件伪链接让 output ancestor 安全门形同虚设。

Quality 同时处理 `pull_request.edited`，确保 PR 改为指向 `main` 后会发布必需的 Quality 结果。标题和描述编辑也会触发分类及必要的 Quality 验证。job 保持固定必需名称，包括纯文档 skip；GitHub 不会求值 skipped job 的动态名称。纯文档改动仍使用真实 job-level skip。

### 离线 ugoira 对照

动图编码器对比使用独立的显式入口 `TestRealPixivUgoiraCapture`。它与 `TestRealPixivSDKRead` 共用本地
账号选择和 rotation 持久化，再通过公开 SDK `UgoiraMetadata`、`SaveResource` 采集。作品 ID 为逗号分隔的
正整数，quality 必须明确为 `original` 或 `medium`；该质量不可用时直接失败，不替换。只有获得素材下载授权后才运行：

```bash
PIXIV_UGOIRA_CAPTURE=1 PIXIV_UGOIRA_IDS=150808387,150247639,150356234 \
PIXIV_UGOIRA_QUALITY=medium PIXIV_UGOIRA_DIR="$PWD/build/ugoira-parity/samples" \
go test ./e2e -run '^TestRealPixivUgoiraCapture$' -count=1 -v
```

这条命令采集 medium。下方样本表中的 1080p original 来自该处说明的 `originalSrc`，已另行采集并保存在独立目录。

每个新建 ID 目录包含完整 `frames.zip` 和 `sample.json`；后者记录有序文件名/延迟、实际质量、尺寸、
总时长、字节数、ZIP SHA-256 和帧元数据 SHA-256，日志另记录 manifest SHA-256。URL、resource ref、
headers 和凭证不会序列化。既有样本拒绝覆盖；采集失败只清理本次新建目录。这些私有产物放在已忽略的
`build/` 下，禁止提交。`PIXIV_E2E_PROXY` 沿用 SDK E2E 的非 secret 代理含义；普通测试默认跳过采集。

旧基线须从迁移前源码单独编译一次，记录源码 commit、Go/构建设置和 binary SHA-256，并保持 binary 不变。
当前测试 driver 与默认下载器同用 `ugoira.NewEncoder()`，无需凭证或网络。另建当前 driver，不得覆盖 `baseline.test`：

```bash
mkdir -p build/ugoira-parity
CGO_ENABLED=0 go test -c -trimpath -o build/ugoira-parity/current.test ./e2e
GOMAXPROCS=1 RAYON_NUM_THREADS=1 \
UGOIRA_PARITY_SAMPLE="$PWD/build/ugoira-parity/samples/150808387" \
UGOIRA_PARITY_OUTPUT="$PWD/build/ugoira-parity/current-150808387.gif" \
UGOIRA_PARITY_FORMAT=gif UGOIRA_PARITY_MAX_EDGE=0 \
build/ugoira-parity/current.test -test.run '^TestUgoiraParityEncode$' -test.v
```

driver 要求绝对路径、与 `gif` 或 `apng` 匹配且尚不存在的输出，以及 uint32 最大边长（`0` 表示不缩放）。
编码前校验 ZIP 与帧元数据哈希，输出 `UGOIRA_PARITY_RESULT`，包含输入哈希、格式、最大边长、`Encode`
耗时、输出字节数、Go 版本和 `GOMAXPROCS`。每轮使用新输出，新旧实现保持相同输入和运行参数；峰值进程 RSS
另用同一主机工具测量。日志耗时不含编译、采集、输入哈希或进程启动。fixture/driver 本身不能证明真实画质、
性能或六平台等价。

工具链变化时保留冻结 baseline，必要时从同一已审计旧 producer 构建独立命名的 legacy comparator；先核对它与冻结输出的身份，再记录双方 build mode，以相同工具链比较。迁移资源验收按每个真实样本/格式各预热一次、交替五次，时间、峰值进程 RSS 与输出字节数比值分别不得超过 1.10；RSS 不是 Go heap。

每次冻结旧版/Go 对照均设置 `GOMAXPROCS=1` 和 `RAYON_NUM_THREADS=1`。旧 Rust 量化器的 worker 数会改变输出像素，而不只影响耗时；选择已保存输出评分时，也须匹配固定基线的 worker 设置。

`TestUgoiraSyntheticCorpus` 生成旧版三个质量场景，并补充透明/重复帧、单像素和奇数尺寸场景。
未设置 `UGOIRA_PARITY_CORPUS` 时使用临时目录；显式指定绝对目录后保留 schema 1 样本，供两版 binary
共同使用，已有样本目录拒绝覆盖。`TestUgoiraParityInspect` 独立解码完整画布 GIF 帧，并重放 APNG 的
Source/None 差分矩形，检查帧数、尺寸、循环、延迟和 GIF 透明语义。输入必须是不缩放的输出（`MaxEdge=0`）。
PNG 源帧要求逐像素还原；JPEG 像素差异单列，因为参考使用 Go JPEG 解码器。质量检查不计入性能测量，
通过 `UGOIRA_QUALITY_RESULT` 输出每帧指标及输入/输出哈希。

```bash
UGOIRA_PARITY_CORPUS="$PWD/build/ugoira-parity/synthetic" \
go test ./e2e -run '^TestUgoiraSyntheticCorpus$' -count=1
UGOIRA_PARITY_SAMPLE="$PWD/build/ugoira-parity/samples/150808387" \
UGOIRA_PARITY_OUTPUT="$PWD/build/ugoira-parity/baseline-150808387.gif" \
go test ./e2e -run '^TestUgoiraParityInspect$' -count=1 -v
```

RGB SSIM 取三通道均值。MSSIMSimple 使用不重叠的 8×8 窗口，边缘窗口按实际面积加权；协方差是偏差乘积
之和，不是总体方差。旧 `image` 0.25 的 `DynamicImage::into_luma8` 实际走 CICP：BT.709/D65 矩阵、
float32 乘加和四舍五入。指标保留已实测的 arm64/FMA 基准；较旧的整数 `Pixel::to_luma` helper 不是这条路径。
设置 `UGOIRA_PARITY_RUST_QUALITY` 可运行 `TestUgoiraSSIMRustCalibration`，读取旧质量 gate 保存的
`<scene>-rust.gif` 和 `<scene>-ssim.csv`，按 CSV 九位小数采用 1e-9 校准容差。

Go 生产编码器使用 owner-local `internal/media/ugoira/jpeg`：复用 Go 1.26.3 的 JPEG 解析，迁移旧
zune-jpeg 0.5.15 的整数 IDCT 与 14-bit BT.601 转换。标准库 IDCT 的舍入差异会改变调色板选择，导致
固定画质门槛失败；只对齐色彩系数不能修复全部样本。该解码器不注册全局图像格式，不新增模块依赖。
解析器的 BSD 许可证保存在该包内，最终许可证 bundle 仍须保留 zune-jpeg 的 MIT 归属。17×9 合成
`jpeg-{444,progressive,gray,restart}` fixture 与旧版解码的 RGBA 参考覆盖这些路径，普通测试不依赖
Rust、FFmpeg 或 jpegtran。Go 缩放核使用限定区间内的正弦/余弦多项式，以 `math.FMA` 固定求值次序，
与旧基准运行时 `sinf` 的结果对齐。`testdata/lanczos-kernel.json` 中的 1,031 组数值参考保护
Lanczos3 舍入行为，Go 路径不调用平台数学库。
Go APNG writer 使用固定 Paeth 预测与逐帧独立的 BestSpeed zlib 流，保留 RGBA8、无损像素及
Source/None 差分矩形播放语义。

迁移样本的六个公开作品 ID 如下。尺寸来自所有 ZIP 帧的完整解码，不能只用作品展示尺寸。
第一个 original ZIP 由 Pixiv 公开 `ugoira_meta` 的 `originalSrc` 明确给出，完整帧列表与已认证 SDK 元数据
逐项一致；该作品的 SDK 元数据仅提供 medium。这只是测试素材采集，不是产品 fallback。

| 作品 ID | ZIP 质量 | 实际尺寸 | 帧数 | 覆盖 |
| --- | --- | --- | --- | --- |
| 150808387 | original | 1920×1080 | 63 | 高分辨率、局部运动、细线 |
| 150247639 | medium | 423×600 | 8 | 复杂插画、大面积变化 |
| 150356234 | medium | 219×320 | 121 | 小尺寸、长序列、渐变 |
| 150863948 | medium | 600×420 | 12 | 局部运动、平涂 |
| 150863611 | medium | 600×338 | 37 | 渐变、纹理、局部运动 |
| 150863218 | medium | 480×270 | 128 | 全画面变化、密集纹理 |

original 和 medium 必须保存在不同目录，本地保留完整元数据和哈希；不要推导 original URL 或把 medium
重标为 original。上述来源记录不能替代后续逐样本画质及五次新旧交替性能验证。


### 可选 FFmpeg 质量对照

设置 `PIXIV_UGOIRA_QUALITY_FFMPEG=1`，用本地 FFmpeg executable 对照 Go GIF encoder。未精确设置为 `1` 时，两项测试在查找 FFmpeg 或读取输入前跳过。默认命令为 `ffmpeg`，可用 `PIXIV_UGOIRA_QUALITY_FFMPEG_BIN` 指定其他 executable。显式开启后，工具缺失或执行失败会明确失败，不安装工具或切换生产编码器。

```bash
PIXIV_UGOIRA_QUALITY_FFMPEG=1 \
PIXIV_UGOIRA_QUALITY_OUTPUT_DIR="$PWD/build/ugoira-parity/ffmpeg-synthetic" \
go test ./e2e -run '^TestUgoiraFFmpegSynthetic$' -count=1 -v

PIXIV_UGOIRA_QUALITY_FFMPEG=1 \
PIXIV_UGOIRA_QUALITY_ZIP=/absolute/path/to/frames.zip \
PIXIV_UGOIRA_QUALITY_FRAMES=/absolute/path/to/frames.json \
PIXIV_UGOIRA_QUALITY_CASE=public-sample \
PIXIV_UGOIRA_QUALITY_OUTPUT_DIR="$PWD/build/ugoira-parity/ffmpeg-public" \
go test ./e2e -run '^TestUgoiraFFmpegPublic$' -count=1 -v
```

public test 只读取本地文件。`frames.json` 是完整有序的 `[{"file":"000000.jpg","delay":80}]` 数组，不是 `sample.json` wrapper；使用已采集样本时，须提取其完整 `frames` 数组。质量对照帧名和旧 gate 一样必须为 flat filename。参考编码沿用 concat duration 与 `palettegen=stats_mode=full`/`paletteuse` filter；提取文件使用生成名称，防止元数据注入 concat 指令。

三个原合成场景要求每帧及平均 RGB/luma MSSIMSimple 均不低于 FFmpeg；public 输入要求平均值，两者沿用原 `1e-12` 比较容差。测试重放 GIF delta/透明帧和 disposal，检查两种输出的循环与帧数，并要求 Go 延迟符合元数据向上取整至百分之一秒的规则。此比较器的 JPEG reference 使用旧 IDCT/14-bit 色彩转换，16-bit 源通道保留旧舍入，原 RGB 指标丢弃 alpha；独立 parity inspector 的标准库 JPEG reference 不变。

`QUALITY_RESULT` 输出场景、帧数和四项平均值。设置 output directory 时保留 `<case>-go.gif`、`<case>-go-ffmpeg.gif` 和 `<case>-go-ssim.csv`（列为 `go_luma_ssim`、`ffmpeg_luma_ssim`、`go_rgb_ssim`、`ffmpeg_rgb_ssim`），质量比较失败时也保存。每次使用新目录保留各轮结果；用于保存的 case 必须是 flat artifact name。Go-run 文件名与历史 `*-rust.gif`、`*-ffmpeg.gif`、`*-ssim.csv` 校准记录分开，保留后者不变。public 样本可能在旧编码器也不及 FFmpeg 时使该可选比较失败；保留负结果，并与同一冻结旧输出对照后再判断是否属于迁移回退。

### 测试文件布局

生产文件 `x.go` 对应同目录最多一个 `x_test.go`；平台专用测试用 `x_<platform>_test.go`，必须有真实 base owner。新 owner 的测试一律用 external test package（`X_test`）；只有下列目录允许 same-package，因为它们观察未导出的内部状态。新增 same-package 例外必须在此登记 permanent 理由，否则视为违规。

| 目录 | same-package 理由 |
| --- | --- |
| `internal/media/ugoira` | 测试直接调用 `readFrameSource`、`frameSource`、`validateDecodedSize`、`frameNRGBA`、`decodeInto`、`resizeFrame`、`apngWriter`，无需巨量分配即可验证分配边界和取消、保留透明像素的 RGB 和缓冲复用时前帧的独立所有权，与固定上游参考向量对比 Lanczos3 像素，检查 APNG 序列溢出和写入失败，以及 reservoir 状态、去重权重/索引、Oklab/Wu 精确参考值、固定种子 K-means 中心与逐像素抖动索引；`colorGrid` 测试验证单元边界的精确候选上下界，`frameQuantizer` 测试验证复用误差行时完整清零、普通量化结果仍独立持有索引、显式缓冲复用只发生在上一帧消费完成后，LZW 测试覆盖跨帧字典/码宽重置和写失败后的恢复；GIF 测试观察 `gifWriter`/`gifBlocks`，验证全局表、流式 LZW 子块和写入失败，生命周期测试观察 `encodeGate`，不靠 sleep 即可同步排队取消；导出这些 codec 内部机制会仅为测试扩大 `Encoder` 契约。 |
| `internal/config/settings` | `schema_test.go` 将合成类型交给私有 `deriveSchemaFromTags` 编译函数，验证非法声明的生产拒绝路径，避免导出反射元数据或重置生产缓存。 |
| `internal/cli` | composition root 测试观察未导出的 root wiring、invocation lifecycle 与 close ordering；这些 seam 不构成公开 API。 |
| `internal/cli/commands/pixiv/search` | 通过真实 SDK 与 HTTP fixture 观察私有 searchArtworks 逻辑页续读。CLI/MCP wire 不暴露这些 cursor，为测试导出应用内部接口会扩大公开契约。 |
| `internal/mcpserver/pixiv/tools/search_illust` | 通过真实 SDK 与 HTTP fixture 观察私有 searchArtworks 逻辑页续读。CLI/MCP wire 不暴露这些 cursor，为测试导出应用内部接口会扩大公开契约。 |
| `internal/browsercookies/chromium` | 测试直接构造 provider 并注入 encryption key override，观察未导出的 cookie 记录解密路径与 profile 发现逻辑。 |
| `internal/browsercookies/firefox` | 测试观察未导出的 profile 发现（`profiles.ini` 解析）、cookie 数据库路径解析与记录布局。 |
| `internal/browsercookies/safari` | 测试直接调用未导出的 `parseBinaryCookies`，断言 binarycookies 记录布局。 |
| `internal/browsercookies/secret` | 测试构造 `SecretService{command: ...}` 注入未导出字段并断言未导出的 sentinel error 与命令输出脱敏行为。 |
| `internal/update/installer` | 测试注入未导出的 `assetURLValidator` seam 与 checksum 校验函数，用真实 fixture 二进制验证 root `--version` 预检与失败时不替换旧可执行。 |
| `internal/update/release` | `source_route_test.go` 观察未导出的 source route 选择与 canonical API URL cache 状态；该目录其余测试已用 external package。 |
| `internal/storage/database` | 测试观察未导出的 `tableInfoQuery` 白名单与迁移兼容 seam，确保 SQL 标识符始终来自固定字面量，旧 schema 不能静默绕过契约。 |
| `sdk/pixiv` | `cursor_test.go` 观察未导出的 cursor 构造与 client-instance binding，以验证精确的 query-bound 无效 continuation，而不扩大 public SDK surface。 |
| `scripts/internal/browsernativeevidence` | 测试观察未导出的环境探测并注入合成 Firefox cookie 种子。 |
| `scripts/internal/homebrewformula` | 测试直接调用未导出的 formula 渲染与版本校验（`renderFormula`、`validateFormulaVersion`、`checkDynamicVersionNeeds`）。 |
| `scripts/internal/licensebundle` | 测试观察未导出的 `defaultBundleFileOps`、`modulesFromMetadata`、`packageLicenseFiles` 与 license 文本归一化；用可执行 Go fixture 验证六目标离线命令契约。 |
| `scripts/internal/linuxabi` | 测试直接调用未导出的 glibc 版本解析与 ABI 比对（`parseGLIBCVersion`、`checkImportedSymbols`）。 |
| `scripts/internal/nativeevidence` | 测试直接调用未导出的 record/consolidate seam，覆盖 schema 3、独立 `source_commit`、精确 binary `--version` 输出、六目标 hash/archive 校验与 mutation 回滚。 |
| `scripts/internal/publicapi` | 测试观察未导出的 `unexported`/`hidden` 符号解析与 golden 比对逻辑，用 `writeFixture` 生成 fixture。 |
| `scripts/internal/releaseassets` | 测试注入未导出的 `injectReleaseSources`/`injectWindowsReleaseSources`，观察 asset archive 命名（`archiveName`）与 checksums 生成。 |
| `scripts/internal/releasenotes` | 测试观察未导出的 GitHub client 调用映射，注入 fake client 断言来源审计。 |

当前没有 temporary 项。本清单不接受「迁移期」「未来」类无期限表述。新增目录须说明观察的**具体未导出符号**并确认导出最小接口不可行；删除目录须提供删除任务与测试迁移证据（external package 可编译 + 覆盖率不变）。

`e2e/` 也是 `package X`，但它没有生产代码（纯测试载体），「观察未导出生产状态」问题不适用，不在本清单范围。跨平台差异：带 build tag 的测试文件（如 `scripts/internal/*`）在不同 `GOOS`/`GOARCH` 下可见文件数不同；验证时分别用 `GOOS=darwin`、`GOOS=windows`、`GOOS=linux` 运行 `go list` 确认目录集合一致。

```bash
# 列出测试留在生产包内的目录（package X 而非 X_test）
go list -json ./... | python3 -c 'import json,sys
dec=json.JSONDecoder(); s=sys.stdin.read(); i=0; same=[]
while i<len(s):
    try: p,i=dec.raw_decode(s,i)
    except json.JSONDecodeError: break
    while i<len(s) and s[i] in " \n\t": i+=1
    if p.get("Dir") and p.get("TestGoFiles") and not p["ImportPath"].endswith(("_test",)):
        same.append((p["ImportPath"],len(p["TestGoFiles"])))
for ip,n in sorted(same): print(ip,n)'
# 期望结果 = 上表 Permanent 目录
```

### 能力边界

这是 v1 中**不得有可发布入口**的能力的维护者侧权威清单，是负面契约：为下列能力新增已发布的 CLI/MCP/SDK 入口即为缺陷。Evidence-gated 表中记录的 SDK-only migration seam 不属于可发布入口，也不得据此记为 capability 已完成。禁止以 schema 占位或 mock 空结果「预留」。

**Unsupported（v1 明确不支持；新增入口即缺陷）：**

| ID | 唯一 owner | 当前证据 | close-out 条件 |
| --- | --- | --- | --- |
| `ART-SEARCH-RATING` | `internal/cli/commands/pixiv/search` + `internal/shared/searchfilter` + `sdk/pixiv` | CLI artwork search 对规范化的 `x_restrict` 做本地过滤，并将过滤条件绑定到 cursor；不发送上游 rating 字段。MCP `search_illust` 没有独立 rating 参数 | 保留 CLI 本地过滤契约并测试 cursor 与过滤条件的一致性；新增 MCP 参数或上游字段需要独立的行为依据，并同步 schema 与文档 |
| `NOVEL-SEARCH-ADVANCED` | 无 owner（不得新增） | SDK/MCP schema 无 advanced 字段 | 上游 contract 出现后可评估；禁止 schema 占位 |

**Evidence-gated（可存在 SDK-only migration seam；可发布入口仍须先满足 close-out 条件）：**

| ID | 唯一 owner | 当前证据 | close-out 条件 |
| --- | --- | --- | --- |
| `NOVEL-RANKING` | `sdk/pixiv` + `internal/cli/commands/pixiv/ranking`（T18/T30；MCP 后续） | SDK 与 CLI 已在 internal `/v1/novel/ranking` adapter 之上暴露 additive `NovelRanking` seam；CLI 通过 `--type novel` 显式选择；MCP 无 `novel_ranking` tool，live/public 发布 evidence 仍未闭合 | 完成 live 第二页、shared cursor、MCP 与发布兼容门禁；此前仍是 evidence-gated，不得记为 `public_ready` |
| `NOVEL-BOOKMARK-MUTATION` | `sdk/pixiv` + `internal/mcpserver/pixiv`（G1-T13；CLI 后续） | SDK 已暴露 additive typed `AddNovelBookmark`/`RemoveNovelBookmark`；MCP 已暴露 `add_novel_bookmark`/`remove_novel_bookmark`；offline outcome、校验与 no-replay evidence 已存在，strict/live、read-back 与发布 evidence 仍未闭合 | 完成 strict/live mutation evidence、同账号 read-back、清理及兼容/发布门禁；此前仍是 evidence-gated，不得记为 `public_ready` |
| `COMMENT-WRITE` | `sdk/pixiv`（T16；CLI/MCP 后续） | SDK 已暴露按 namespace 区分的 `PostArtworkComment`/`ReplyArtworkComment`/`DeleteArtworkComment` 及 novel 对应方法；MCP `comment_post`/`comment_add` 目录仍 = 0；响应 ID、read-back、清理和 strict live evidence 尚未闭合 | 完成 strict/live 写入 evidence、同账号 read-back、清理及 T33/T38 兼容门禁后；此前仍是 evidence-gated，不得记为 `public_ready` |
| `NOTIFICATION` | 无 owner | MCP `notification` 目录 = 0；SDK `Notification*` 导出 = 0 | 同上 |
| `AUTOCOMPLETE` | 无 owner | MCP `autocomplete` 目录 = 0；SDK `Autocomplete*` 导出 = 0；未并入 `search` | 同上 |
| `WEB-RESTRICTED-READ` | 无 owner | 无 `webapi` 包；`web_fallback_enabled` 是 tombstone key（`config get/set` → `removed_setting`） | 不得重开匿名 Web 路径；任何恢复 Web/AJAX 的提议须先修订 AGENTS 冻结契约并经 ADR |
| `USER-BLOCK-MUTE-REPORT` | 无 owner | MCP `mute`/`report` 目录 = 0；SDK `BlockUser`/`MuteUser`/`ReportUser` 导出 = 0 | 上游提供可验证的 mutation contract 后 |
| `WATCHLIST-MARKER` | 无 owner | MCP `watchlist` 目录 = 0；SDK `Watchlist*` 导出 = 0 | 上游提供后 |
| `BOOKMARK-USERS` | 无 owner | 无对应 tool/SDK 导出；`bookmark_detail` 只覆盖当前用户详情 | 上游提供后可评估 |
| `SPOTLIGHT-PIXIVISION` | 无 owner（范围外） | MCP `spotlight`/`pixivision` 目录 = 0；SDK `Spotlight*`/`Pixivision*` 导出 = 0 | 明确范围外；仅当产品范围变化时重新评估 |

为上述任一能力新增或恢复入口是功能变更：须更新对应行并同步相关用户文档。审查者手动重跑每项下的 negative grep / 目录存在性检查。

## 发布门禁、签名与 Homebrew 边界

`.github/workflows/release.yml` 默认由 `v[0-9]*` tag 触发：先验证 SemVer，再在 immutable tag 上运行无凭据的
SDK E2E contract gate，确认测试入口和默认 skip/离线边界没有被破坏；随后才构建
darwin/linux/windows × amd64/arm64 的 CGO0 Go binary，运行 Go 测试（race step 单独启用 cgo）、检查许可证并封装固定名称的 archive。
该 workflow 不读取或注入 Pixiv/FANBOX credential。真实 SDK E2E、native browser 和一次性 solver acceptance
必须在授权环境按本页对应流程完成，不能把 contract gate 当作真实 release evidence。

`releaseassets finalize` 还从 immutable tag 读取 `scripts/install.sh` 与 `scripts/install.cmd`，把它们以固定
名称复制到 Release，并与六个平台 archive 一同写入 `checksums.txt` 和 Ed25519 签名 manifest。publish
policy 锁定 finalize 参数及完整八资产上传集合；Homebrew renderer 也要求 checksum 集合包含两个 installer，
但 formula 仍只下载对应平台 archive。

release.yml 只接受 `v[0-9]*` tag push，不再提供 `workflow_dispatch`、`release_tag` 输入或 test-only
overlay。tag run 失败时应修复默认分支上的原因，并按正常的不可变 tag 发布流程重新处理；不会从默认分支
把新 verifier、测试或生产源码注入旧 tag，也不会为已有 Release 提供手工 recovery 入口。validate、test
build、production build 与 publish 都绑定同一个 tag；生产构建在独立 runner 上从 clean tag tree 以 CGO0 构建，并继续以 `git diff --exit-code` 做 byte-for-byte 校验。

GitHub Release 与 registry 是独立系统，无法原子提交。因此 `.github/workflows/publish-dockerhub.yml` 作为独立的
Release 后容器 publisher，同时负责 GHCR 与 Docker Hub。它从完成的 Release run 接收 immutable handoff 和两份
verified container artifact，校验 release tag 与 source identity，再把同一批已加载镜像 bytes 发布到
`ghcr.io/flanchanxwo/pixiv-cli` 与 `docker.io/flanchanxwo/pixiv-cli`；只有 Docker Hub 登录使用受保护
`release` Environment 的 secret `DOCKER_HUB_TOKEN`，并通过 stdin 传入。它不会重建镜像。若 registry 发布失败，
应从默认分支只使用原始 `release_run_id` dispatch，以复用同一批 verified artifact。exact-version tag 总是发布，
恢复旧 stable 时即使不更新 `latest` 也会成功，只有最新 stable release 才推进 `latest`。不使用 retry loop 隐藏 push 失败。

### 容器发布验证

`build_container` 在共享 `build` 门禁后运行，并与 `build_production` 并行；它不会等待生产资产重建。两个原生
target 分别是 `ubuntu-22.04` 对应 `linux/amd64`、`ubuntu-22.04-arm` 对应 `linux/arm64`。每个 target 都从
immutable tag checkout，在 clean tree 以 CGO0 构建版本化 Linux binary 并通过 Linux ABI gate，
运行容器打包测试，构建 pinned glibc runtime 镜像，并验证非 root 执行、精确版本、`/home/pixiv/.pixiv-cli/`
下的 `pixiv config path`、`/work` 以及 OCI provenance（`org.opencontainers.image.source`、revision、version
和 licenses），最后导出 `verified-container-linux-amd64` 与 `verified-container-linux-arm64`。build job 只持有
`contents: read`；只有独立容器 publisher 在 GitHub Release 后消费这些 artifact，并为 GHCR 推送申请 `packages: write`。

维护者聚焦检查：

```bash
go test ./scripts/tests/containerrelease -count=1
go test ./tools/release ./tools/platformmatrix -count=1
```

无凭据容器 smoke workflow 会在相关变更时构建两个原生架构，并执行 version、非 root、state-path 和工作目录
断言；正式 tagged release 不能用这些本地检查替代该 CI evidence。

共享平台 runner/capability metadata 只位于 `ci/platforms.json`，由 `tools/platformmatrix` 校验并输出；
release archive identity 仍由 `scripts/internal/releasecontract` 持有。workflow 测试只保留行为与安全边界，
不再镜像 job 数量、step 位置或 multiline shell 全文。历史 recovery 计划和验收报告保留原始文字与路径，
不作为当前流程说明。

### Verifier 源码导航

Release 验证现在优先检查真实行为，不再维护第二套 workflow policy 实现。`tools/platformmatrix`
负责校验共享平台注册表；`tools/release` 统一负责可复用的发布信任与 artifact 校验，包括 immutable
tag/default-branch ancestry、published Release 状态、Release-run handoff identity、精确的
archive/container 集合与 checksums。各 publisher workflow 只保留自身渠道的 credential、权限、变更
检测和发布命令；recovery run 可以只验证成功的 `Release` handoff，而原有契约明确要求 run 与 tag
同一提交的 publisher（例如 Docker Hub）则继续额外绑定 handoff run 与 immutable tag commit。

`scripts/cmd/nativeevidence/` 只保留 CLI 入口与参数分发；具体 evidence 生命周期实现在
`scripts/internal/nativeevidence/`：`models.go` 保存 target 和 evidence schema；`record.go` 记录单 runner
evidence；`consolidate.go` 校验并合并六目标结果；`archive.go` 负责 release archive member 与 JSON；
`filesystem.go` 负责路径、hash 和安全文件操作。
`scripts/cmd/nativeevidence/` 只分发 `record` 与 `consolidate`；workflow 测试只覆盖 credential、action
固定与 build ownership 等安全/行为边界，不再重新实现 workflow 的精确 YAML 形状。

release workflow 本身就是顺序与权限的事实来源。生产 archive、container、prepared checksums 以及针对
真实 production archive 的 Homebrew 安装验证都必须在 `release-approval` 前完成；审批后由
保存发布 secrets 的 `release` environment 消费已经批准的同一批 artifact，不重新构建。

`go.mod` 当前声明的 Go toolchain 不支持 Windows ARM64 的 race detector，但 release matrix 仍会在六个原生目标上实际执行 race gate：
其中五个平台运行 `go test -race ./...`，Windows ARM64 则必须执行同一命令并精确匹配 Go 官方
`-race is not supported on windows/arm64` 诊断；其他任何失败仍视为 gate 失败，因此不会再出现 matrix step skip。
test matrix 还固定 `GIT_CONFIG_*` 为 `core.autocrlf=false`，使 Git for Windows checkout 保留 immutable
tag 的 LF blob bytes；否则 pre-commit 的 `gofmt` 会把 runner 的 CRLF 转换误报为源码未格式化。该配置
仅用于 test gate，独立 production build 仍从 tag 的干净默认 checkout 构建资产。

Release 的 preparation 阶段固化一份 immutable handoff（`release/release-handoff.json`），记录 release run、tag、commit 以及每个 production/container 产物的 size 与 SHA256，publish 再以已批准的 `release/checksums.txt` 复验；policy 拒绝中间 step、路径替换或发布后改写。Homebrew 不再属于 Release workflow：独立 publisher `publish-homebrew.yml` 消费该 handoff、校验 production section，并把 releaseassets 的 stable/
prerelease 结果直接映射为 `pixiv-cli`/`pixiv-cli-beta`。随后精确四目标 matrix（macOS Intel/arm64、
Linux amd64/arm64）先用 `brew tap-new pixiv-cli-release/staging --no-git` 创建各 runner 的隔离
local tap，再以 `brew trust --tap pixiv-cli-release/staging` 显式信任这一个临时命名空间；将唯一
staging formula 放入其 `Formula/`，随后用 `pixiv-cli-release/staging/<formula>` 执行真实
`brew install --formula`。macOS 在原生 runner 的临时 tap 中运行；Linux 在短生命周期、固定 digest 的
`homebrew/brew` 容器内运行，并将 staging formula 目录以只读 bind mount 传入容器。随后执行
`test "$(pixiv --version)" = "pixiv $RELEASE_TAG"` 并与 tag 比较。它不使用 workspace formula path、developer/环境变量 bypass，
也不克隆、写入或信任公开 tap。只有全部成功，`publish-homebrew.yml` 中受保护的 `deploy_homebrew_tap` 才以 HTTPS
clone public tap，并由受信默认分支 tip 上的 `scripts/cmd/homebrewrecovery` 做单调判定：请求版本必须
不低于 tap 当前 Formula 版本（优先复用 `internal/releaseversion` 的 SemVer 比较，不做字符串比较）；
同版本且 bytes 完全一致时判定为已发布而 no-op 成功，不产生任何 commit 或 push；请求版本更旧，或同版本
但内容不同，都在读取 deploy key 之前 fail closed，从而同一 `release_run_id` 的重复恢复幂等，较旧的恢复
请求无法回退已发布 Formula（`pixiv-cli` 与 `pixiv-cli-beta` 各自独立比较）。仅当判定为需要写入时，才核对唯一
staged formula，并在最后一个 step 读取 deploy key；SSH push 固定官方 GitHub ED25519 known_hosts、启用
strict checking，目标精确为 `HEAD:main`。任何前置 job 失败都不会写 tap。

这套本地检查只证明 workflow 声明的依赖和语义，**不**验证 GitHub `release` Environment、
secret 和 tag protection 的远端实际状态；它不替代远端配置审计，也不替代正式 tag
产生的四架构 Homebrew 外部安装证据。由于 draft asset 的匿名 URL 不可被 Homebrew 下载，workflow
会先公开 Release 再安装；若安装失败，Release 已公开但 tap 不变，需要维护者显式处置，不能绕过 gate
手工 push。

GitHub hosted Linux runner 上 Linuxbrew 的 `Resource` staging cleanup 有直接 backtrace 证据会触发
`FileUtils.chmod` 的 `EINVAL`，且该错误早于 `--keep-tmp` 等候选项可介入的阶段。为保证门禁仍是真实的
formula 安装，Linux 分支使用 `docker run --rm` 启动固定 digest 的 `homebrew/brew` 镜像，向容器传入只读的
绝对 staging-formula bind mount；容器只创建本地 staging tap、复制该 formula、执行普通 tap-qualified
`brew install --formula` 并把 `pixiv --version` 与 `RELEASE_TAG` 精确比较。`HOMEBREW_NO_AUTO_UPDATE=1`
与 `HOMEBREW_NO_ENV_HINTS=1` 仅消除自动更新及提示造成的漂移，不改变 formula 或安装语义。容器不读取
secret、不写 host mount、不使用公开 tap，也不使用 `HOMEBREW_TEMP`、source/debug/keep-tmp flags。固定的
Homebrew 4.6 容器镜像不提供 `brew trust`；这不是安全绕过：该 tap 仅在 `--rm` 容器内由 `brew tap-new`
创建，唯一 formula 从只读 mount 复制，且不会触及公开 tap。macOS 原生 Homebrew 保留显式
`brew trust --tap`；Linux 与 macOS 都使用同一 root `--version` gate，不依赖 Python/Ruby JSON parser。
版本比较发生在 `brew install` 之后，不能改变安装验收路径。本地 Docker 已在 arm64 和 amd64 QEMU
做过同一 formula 安装实验；
GitHub runner 的预发布演练仍是正式发布前必须取得的外部证据。

### 发布前只读 Homebrew 演练

<details>
<summary>展开操作边界与平台证据</summary>

在创建任何新 tag 或 Release 前，维护者可从默认分支手动运行
`.github/workflows/homebrew-prepublish-verify.yml`，并传入一个**已公开、非 draft、非 prerelease** 的
stable Release tag。它先验证输入为带 `v` 前缀的 SemVer、执行分支为默认分支，并确认 GitHub Release 的
tag 与输入一致；随后只下载该 Release 已发布的 `checksums.txt`，渲染 `pixiv-cli` staging formula，最后在
macOS Intel/arm64 与 Linux amd64/arm64 四个生产同款 runner 上执行真实的本地 staging-tap 安装。

这是一项只读 rehearsal：它没有 `release` Environment、secret、tag checkout、Release/asset 编辑或创建，
也不会 clone、提交或推送 Homebrew tap。Linux 分支在固定 digest、短生命周期 Homebrew 容器中安装只读挂载的
本地 staging formula；macOS 保持原生普通安装命令。
它用于在正式发布之前复现 Homebrew 安装链路，**不替代**正式 tag 发布、签名 Release、tap 部署或发布后的
安装验收。质量门只保留 formula 与 artifact 的行为级校验，不再复制一套 workflow policy 实现。

正式发布目前仍必须被正式 tag、签名 GitHub Release、tap formula 与后续安装验收阻断。完整
同 source/version 的完整六目标 Go binary/archive 证据必须已收集并核验（见 [Native runner evidence](#native-runner-evidence)）；
受保护 `release` Environment、生产 signing 私钥与公开仓库也必须已配置，但这些前置条件本身
不等于 Release/tap 已创建或安装路径已验收。

production Ed25519 public trust root 已在
[`internal/update/installer/release_installer.go`](../../../internal/update/installer/release_installer.go) 随源码提交：key ID 为
`ed25519-2c27e77742d3c33a`，其 SPKI DER SHA-256 fingerprint 为
`2c27e77742d3c33ad14be867d4e0519229a220898c9a7c868447eaef0951b4cf`。同包测试以已知真实签名验证
此映射；它只证明公开信任根进入 production wiring，并不证明实际签名、Release asset 或安装验收已经
完成。

生产 Ed25519 信任根的其余规则如下：

- 公钥、key ID 与 fingerprint 已以可审计源码变更进入受支持二进制；私钥绝不进入源码、
  release asset、日志或 formula。
- 私钥只能作为受保护 `release` Environment 的 secret 使用；恢复副本只可保存在受控的 macOS
  Keychain。它不得进入源码、日志、Release asset 或 formula。
- 轮换时先发布能够信任新 key ID 的版本，保留旧公钥直至旧版本退出支持，再通过新的受签名
  Release 停止使用旧 key。不得让既有二进制突然依赖一个未提交、不可验证的新信任根。

Homebrew tap 是独立发布面：stable 使用 `pixiv-cli`，pre-release 使用 `pixiv-cli-beta`，二者
都安装 `pixiv` 并相互冲突。`publish-homebrew.yml` 先验证原始 Release handoff 与归档 checksum，
再渲染并推送 formula；原生安装检查使用平台注册表。专用 `HOMEBREW_TAP_DEPLOY_KEY` 只通过
`release` Environment 提供给授权的 tap 写入步骤，不得进入源码、日志或 artifact。
这个下游 publisher 不增加人工审批边界；唯一的最终审批由 `release-approval` 承担。

当前 Release 不会进行 Apple notarization 或 Windows Authenticode。直接下载仍可能被 Gatekeeper
或 SmartScreen 拦截/提示；这是需要在用户文档中保留的系统信誉边界，不能通过文档或脚本绕过。

成功结束的 `Release` workflow 通过 `workflow_run` 触发独立的 Homebrew、Docker Hub、SkillHub
与 ClawHub publisher。Release handoff 绑定原始 run、tag、commit 与已准备产物的身份，不是仅含 tag
的文件，也不依赖 Homebrew 部署先完成。各 publisher 在发布前重新验证身份与自身需要的产物。
不得从 `workflow_run.head_branch` 推断版本，也不得用后续 main 内容替代。

SkillHub 检查不可变 tag、默认分支祖先关系、公开 Release、SemVer，以及产品 skill 相对前一个已合并
语义版本 tag 的变更。未变化时跳过发布；变化时执行 dry-run 和提交。产品 `SKILL.md` 的版本必须与
CLI Release tag 相同。`SKILLHUB_TOKEN` 仅进入最终提交步骤；返回 `skillId` 和审核状态证明已接收，
不代表立即公开或审核通过。

经授权的人工恢复向对应 publisher 提供原始 `release_run_id`，不接受替代 tag 或当前 main。
ClawHub 另有 `verify_only`，只核验已经提交的版本，不重发。其 dry-run 不含凭据，最终 publish/inspect
核对精确产物指纹。static scan 已 clean 但 aggregate security 仍 pending，或 `skill-card.md` 延迟生成，
会明确 warning，不把已接收误报成失败；这些 warning 也不证明最终通过，`verify_only` 仍要求聚合安全
结论为 clean。平台未暴露服务端解析的 GitHub provenance 时，明确该限制，以受信 tag checkout 和指纹
作为现有证据，不宣称已经独立验证 provenance。

</details>

## Git 与本地产物

`.gitignore` 已排除：

- `.DS_Store`
- 构建产物 `build/`、`pixiv`、`pixiv-cli`
- 本地下载目录 `downloads/`
- 本地数据库 `*.db`
- 常见缓存和临时文件

不要提交 Pixiv token、下载内容、本地数据库、机器相关配置、Ed25519 私钥或 tap deploy key。

## Release notes and publication

`changelog/` 按版本目录维护英文与简体中文发布说明。每个非空章节依次使用 `Breaking changes`、`Added`、`Changed`、`Fixed`、`Security`、`Documentation`、`Maintenance`；双语文件使用对应译名，并在末尾提供相同范围的 `Full Changelog` compare 链接。首个版本使用该 tag 的 commits 链接。

每条面向结果的说明内联列出来源 PR；没有关联 PR 的变更使用真实短 SHA commit 链接。一个条目可以归并多个相关来源。没有用户可见影响的改动归入 `Maintenance`，不能跳过。每个版本还会列出首次合并且不属于仓库所有者或 bot 的外部贡献者。`changelog/unreleased/` 只保留 release-prep 提示，不是普通 PR 的编辑目标。

PR 正文只保留“变更”“验证”“自查”。分类、breaking 判断和版本摘要由 release-prep 维护者结合最终 Markdown 章节与兼容性评估决定，不从 PR body 读取，也不要求 PR 作者提前判断版本号。

完成合并、测试和审查后，使用 `scripts/cmd/releasenotes audit` 收集 tag 范围内的 PR、direct commit、作者和首次贡献者。审计报告只放在本地临时目录或 CI 的 `$RUNNER_TEMP`，不提交仓库。维护者逐项核对后直接编写 `changelog/vX.Y.Z/en.md` 与 `zh-CN.md`，再用 `validate --audit` 检查章节顺序、双语来源集合、compare footer、遗漏来源和范围外来源。正式 tag 上的 `release_notes_audit` job 会以只读 `contents` 与 `pull-requests` 权限重新执行同一套审计和校验。

版本选择不等于发布授权。创建 release-prep PR、合并、创建或推送 tag、触发发布、同步历史 GitHub Release 前，维护者须在当前会话明确确认具体版本、commit/tag 范围和预期影响。完整操作路径见 `.agents/skills/pixiv-cli-release-notes/SKILL.md`：普通 PR → 合并后审计 → 直接编写双语 Markdown → 校验 → release-prep PR → tag 与 GitHub Release → SkillHub / ClawHub 验证。

`sync-history` 默认 dry-run。明确传入 `--apply` 后，既有 GitHub Release 只更新正文；缺失的历史 Release 以现有 tag 创建且不含资产。两种情况都会读取远端正文并与本地双语渲染结果核对。

## 文档同步

当以下内容变化时，同步更新双语 README、双语 CLI reference 或对应 `docs/`：

- MCP tools、参数或返回语义。
- CLI 命令、参数、账号配置或输出语义。
- 环境变量或默认值。
- 下载、认证、代理、ugoira 等流程。
- 安装渠道、更新通道、签名信任根、Release/tap 发布门禁或系统信誉提示。
- 新增限制、重试、超时、截断、降级或错误处理策略。
- 测试或构建命令。
