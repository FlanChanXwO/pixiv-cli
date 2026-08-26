# 开发流程

| 要做的事 | 从这里开始 |
| --- | --- |
| 检查本地工具链 | [环境检查](#环境检查) |
| 构建或核验 ugoira native library | [Rust ugoira staticlib](#rust-ugoira-staticlib) |
| 运行 CLI 与 MCP | [运行](#运行) |
| 处理登录和凭据 | [获取 refresh token](#获取-refresh-token) |
| 选择测试范围 | [测试](#测试) |
| 检查 release workflow | [发布门禁、签名与 Homebrew 边界](#发布门禁签名与-homebrew-边界) |
| 准备版本说明 | [Release notes and publication](#release-notes-and-publication) |

## 环境检查

项目是 Go module，当前 `go.mod` 声明：

```text
go 1.26.3
```

开工前建议检查 Go/cgo、Rust 与常规测试环境：

```bash
go version
go env GOVERSION CGO_ENABLED CC GOOS GOARCH
cargo --version
go test ./...
```

## Rust ugoira staticlib

生产 ugoira GIF/APNG 由内置 Rust encoder 完成，运行时不依赖 `ffmpeg`。`ffmpeg` 只可作为
开发质量对照：显式设置 `PIXIV_UGOIRA_QUALITY_FFMPEG=1` 后，Rust quality gate 才会调用它；
它不是本地构建或用户运行的前置条件。

帧源读取与 image decoder 使用同一条内存边界：边界值直接取 pinned `image` crate
`Limits::default().max_alloc`，而不是另设经验常量。ZIP member 声明大小超过该值时在读取前失败；
实际展开字节超过该值、内存预留失败或取消时也会在分块读取中显式失败，不截断输入或回退到
其他 encoder。取消 token 会在每个读取块前后以及 image decode 前后检查；但 `image` crate 的单帧
decoder 没有取消回调，所以已经进入其内部的 decode 不能中途打断，只能在返回后立即观察取消。
聚焦回归覆盖声明大小超限、实际累计字节超限、读取中取消、正常边界输入、正常 GIF/APNG 和腐坏
ZIP；这一限制的目的仅是防止帧源在 decoder 自身限制生效前无界占用内存，影响是超限作品明确报错。

受支持的 Go 源码构建需要下列条件：

- Go `1.26.3`；
- `CGO_ENABLED=1`；
- 当前 `GOOS/GOARCH` 对应的 C linker；
- Rust crate 对应 target 的 committed `staticlib`；
- 同一份 Rust source 生成的六目标 `staticlib/manifest.json`。

固定 target 是 darwin/linux/windows 的 amd64/arm64。`scripts/build-staticlibs.sh` 使用 locked
Cargo 输入生成 target library；只有同一次成功得到全部六个真实库并逐个核验 SHA-256 后，才会
写入带 Rust source digest 的 `manifest.json`。单 target 调用会使已有 manifest 失效，避免用
局部重建证明全平台一致性。

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

六个 committed library 与 `manifest.json` 是已验证输入：manifest 绑定 Rust source digest、六 target、
path 与逐目标 SHA-256，由 `internal/media/ugoira/staticlib` 的完整性测试锁定。当前 manifest 的 source
digest 与六库 SHA-256 与受审计 source 逐字节一致；升级 Rust 时必须从同一受审计 source 完整重建、链接并
smoke 验证六目标，同时更新六库、manifest、native evidence 与 release matrix——不得只更新单个平台的 pin。

合规 committed library 的编译器 provenance 必须按 target 固定，而不是使用可移动的 runner 默认
toolchain：`x86_64-apple-darwin` 与 `x86_64-pc-windows-msvc` 使用 Rust `1.96.0`；
`aarch64-apple-darwin`、`aarch64-pc-windows-msvc`、`x86_64-unknown-linux-gnu` 与
`aarch64-unknown-linux-gnu` 来自 Rust `1.96.1`。release test 与 production matrix 都必须携带这份
精确映射，并通过 `RUSTUP_TOOLCHAIN` 和带 `--no-self-update` 的 `rustup toolchain install` 使用它；
不能让 runner image 的 `stable` 更新改变重建 bytes。该映射记录来源，不是允许永久混用工具链的惯例；
升级 Rust 时必须重建并同步固定全部六目标。

可在具备目标工具链的受控环境运行：

```bash
sh scripts/build-staticlibs.sh --target <rust-target>
go test ./internal/media/ugoira/staticlib -run '^TestCommittedManifestWhenPresent$' -count=1
```

不要提交 `internal/media/ugoira/rust/target/`；它是机器产物。完成验证的
`internal/media/ugoira/rust/staticlib/` 及其 `manifest.json` 是可追溯输入，不能以 ignore 规则隐藏。

Rust crate 的 `.cargo/config.toml` 将 crates.io 替换为其相邻 `vendor/` 中完整的 locked
依赖闭包。`vendor/` 的每个 package 都带 Cargo 生成的 `.cargo-checksum.json`；它、Cargo config、
`Cargo.toml`/`Cargo.lock`、`build.rs`、`.cargo/**`、Rust source 和本地 `quantette` 都计入
staticlib source digest。不要手工编辑 vendor 内容；升级依赖时必须重新以
`cargo vendor --locked --offline` 生成完整闭包并更新 digest
fixture 与许可证 bundle。根 `.gitattributes` 对上述 first-party crate 输入、整个 `vendor/**` 与固定本地
`quantette` source 设置 `-text`；这只保留 Git blob 原始 bytes，不会重写正常内容，并防止 Windows
checkout 把 LF 改为 CRLF 后破坏 Cargo checksum、source digest 或 licensebundle。对
release archive 的 `LICENSE`、licensebundle 的 `THIRD_PARTY_LICENSES.md` 与
`third_party/licenses/**` 则固定 `text eol=lf`，使 archive member audit 与 byte-for-byte `--check` 在
Windows 保持稳定。
摘要器还必须在判断 `src/`、`.cargo/` 与 `vendor/` 之前，把 `filepath.Rel` 的平台分隔符规范化为
slash；否则 Windows 的反斜杠路径会静默漏掉这些输入。
`target/` 仍是机器产物，不计入 digest，也不得提交。

直接运行 Cargo 时必须在 crate 目录启动，确保 Cargo 发现 source replacement：

```bash
(
  cd internal/media/ugoira/rust
  cargo test --locked --offline
  cargo clippy --locked --offline --all-targets -- -D warnings
)
go run ./scripts/cmd/licensebundle --check
sh scripts/test-rust-vendor.sh
```

`scripts/test-rust-vendor.sh` 为 release workflow 的聚焦供应链回归：它建立临时空 `CARGO_HOME` 与
`CARGO_TARGET_DIR`，随后依次执行 `cargo metadata/build/test --locked --offline`，再以相同环境运行
六个 release target 的 `go run ./scripts/cmd/licensebundle --check`。因此 registry cache、网络 fallback、
缺失 vendor 内容或无效 checksum 都会明确失败，不能把 runner 的预热缓存当作离线可复现性证据。

### Native runner evidence

`.github/workflows/native-evidence.yml` 是独立的、非发布的 runner 入口：只允许审计后的、包含非文档输入的 `main`
push 或指向 `refs/heads/main` 的 `workflow_dispatch`。仅 `README*.md`、`docs/**`、`changelog/**` 或 `skills/**` 的 push
不启动它；任一其他路径以及手动触发仍运行完整矩阵。全局 `permissions: {}`、job 仅 `contents: read`。它没有 `environment`、
secret、tag/Release/tap/signing 命令；YAML AST policy 同时固定六个 runner、full-SHA action、无凭据
checkout、vendored Rust 检查、单目标 staticlib、真实 cgo GIF/APNG smoke、版本化 binary 的
`pixiv --version`、release-style archive 以及 artifact upload。可离线检查声明本身：

matrix 的每个 target 还必须声明与 release test/production 完全相同的 `rust_toolchain`，job 通过
`RUSTUP_TOOLCHAIN` 绑定该值，并执行带 `--profile minimal --target ... --no-self-update` 的精确
`rustup toolchain install`。两个 verifier 共用 `scripts/internal/releasecontract` 中唯一的目标版本映射；
任一 workflow 删除、替换、重复或错误插值该映射，policy 都会 fail closed。

Windows 两个 target 的 Rust library 使用 `*-pc-windows-msvc`；相应 cgo selector 必须以
`-L${SRCDIR}/… -lugoira_rs` 声明库，不能把带盘符的绝对 `.lib` 路径直接传给 cgo；还必须显式携带
Rust `std` 所需的 `advapi32`、`ntdll`、`userenv`、`ws2_32` 与 `dbghelp` import libraries。native evidence
仅在 Windows 的 smoke 和版本化 binary 构建中显式设 `CC='clang -fuse-ld=lld'`：LLD 既能处理 MSVC
`.lib`，也让 Go 跳过 GCC 专属的 debug linker script；这不是运行时 fallback，也不改变 darwin/linux
的 C linker 选择。

```bash
go test ./scripts/internal/nativeevidence -count=1
go run ./scripts/cmd/nativeevidence policy --workflow .github/workflows/native-evidence.yml
```

该 policy command 只依赖 `internal/media/ugoira/staticlib` 的 source-digest/manifest 契约，不导入 cgo
encoder；因此它必须能在每个 runner 构建目标 staticlib **之前**执行。若 policy gate 因缺库或 cgo
link 失败，属于 workflow bootstrap 缺陷，而不是可接受的“尚无 native evidence”结果。

每个 runner artifact 只有 `evidence/`：实际链接的 staticlib、版本化 binary、archive 及
`native-evidence.json`。schema 2 record 会独立记录 workflow 提供的 `source_commit`，重算 Rust source digest
和三份 SHA-256，执行 binary 的 `--version` 并要求精确单行输出，再逐一检查 archive 的 binary、`LICENSE`、`THIRD_PARTY_LICENSES.md` 与完整
`third_party/licenses` 常规文件树。它不持有 release/tap/signing credential，也不会创建 tag 或
Release。

`.github/workflows/browser-evidence.yml` 是另一条 credential-free 的原生 provider contract matrix，
在 macOS、Linux、Windows 的 amd64/arm64 runner 上执行 `internal/browsercookies/...` 的平台代码与合成 fixture 回归，
并由 `scripts/cmd/browsernativeevidence` 校验 workflow 的 runner、action SHA、固定 Firefox 153.0.3
发行包 checksum、清理命令和 secret boundary。`firefox_native` job 只在 runner 临时目录解包官方包，
让 Firefox 生成隔离 profile/schema，再注入明确的 synthetic cookie 运行 provider contract；它不读取
用户浏览器 profile、Keychain、DPAPI 或 Secret Service，也不上传 package/profile/database。真实
profile/session evidence 仍只能在受保护的 release-prep host 取得，不能把该 workflow 的成功当作真实
用户浏览器导入成功。

> [!WARNING]
> Native evidence 不可回填或跨 run 拼接。任一 workflow run 的 runner record 出现不同 source digest 时，即使六个 job 都完成，也不得混合回填；必须从修复后的新 SHA 完整重跑六目标。本地 unit fixture、policy 成功或 workflow 文件存在都不是六目标 native evidence。

需要受控回填 committed 六目标库时，workflow run 的 main SHA 与其产出的 `v0.1.0-native-evidence.<run-id>` 版本必须完全匹配；下载恰好六个 `native-evidence-{darwin,linux,windows}-{amd64,arm64}` artifact，再在干净、非 symlink 的输出目录上运行 `scripts/cmd/nativeevidence consolidate`。consolidator 只接受完整六目标、同一 source digest 与同一 expected version/commit 的记录；重新核验 staticlib/binary/archive SHA 与 archive member hash，生成精确六条 `manifest.json`，对任何缺 target、重复/错配 target、metadata、archive member、哈希或 symlink 都在写入前阻断。人工复核后，把六库与 `manifest.json` 回填到 `internal/media/ugoira/rust/staticlib/`，再运行 `TestCommittedManifestWhenPresent`、`TestRustUgoiraEncoderNativeGIFAndAPNG` 与 `git diff --check`，把六个 blobs 与 manifest 作为独立审查提交。任一验证失败都阻断 release，不能以部分 artifact 继续。

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
`filename_template`、`directory_template`、`request_interval`、`https_proxy`、`log_level` 与 `log_format`。
其余高级 TOML 由用户手工维护。首次配置 bootstrap 使用 `internal/config/settings` schema 元数据与
`tomledit` 自动生成精简文件，只落盘标记为 baseline 的默认项，且绝不覆盖已有文件。

> [!NOTE]
> 已删除的 `[web] fallback_enabled` 若仍存在会返回 `removed_setting`，用 `pixiv config unset web_fallback_enabled` 清理。`[logging].level`（`info|debug`）与 `[logging].format`（`text|json`）是启动时生效的配置；`PIXIV_LOG_LEVEL` 与 `PIXIV_LOG_FORMAT` 覆盖文件值。

### Flag 解析

CLI 使用 Cobra/pflag，flag 可以写在位置参数前后；例如 `pixiv auth check 12345678 --json` 和 `pixiv search "初音ミク" --json` 都受支持。

Pixiv command proxy、`[pixiv.network]`、环境变量与 `[network]`，以及 FANBOX 独立的 `[fanbox.network]`/`[fanbox.flaresolverr]` 配置，均按各自服务边界解析，FlareSolverr 仅用于 challenge recovery。

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

当前测试覆盖 CLI 命令与 build metadata、显式/自动更新、`internal/services/{pixiv,fanbox}/account` 账号服务、`internal/services/pixiv/pool` 账号池、`internal/storage/database` 认证存储与 `internal/config/settings` 配置、`internal/shared/lifecycle` 生命周期、`internal/shared/pagination` 逻辑分页、`internal/shared/traversal` 泛型可重入遍历、Pixiv App API 认证重试、公开 SDK（`sdk`/`sdk/pixiv`/`sdk/fanbox`）、HTTP client wiring、下载管理、Rust encoder/staticlib 合约和 `internal/mcpserver/{pixiv,fanbox}/tools` tool 注册。`internal/account` 与 `internal/session` 已删除，不保留兼容测试入口。测试文件布局与 same-package 例外见[测试文件布局](#测试文件布局)：

```bash
go test ./...
sh scripts/build.sh
# 浏览器 provider 的离线 fixture/crypto/权限分类回归；真实跨平台 host evidence 另按 release-prep 执行。
go test ./internal/browsercookies/... -count=1
# 真实 SDK e2e 需要本机凭据（Pixiv 读本地 pixiv-cli.db 选中账号，FANBOX 读 Keychain）：
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
# 运行两项当前 SDK E2E；脚本不接受 token 或其他凭据输入。
scripts/test-e2e.sh
# 只运行其中一项，或只验证单帖 post.info。
scripts/test-e2e.sh --pixiv-only
scripts/test-e2e.sh --fanbox-post-only
```

`go test ./...` 保持默认离线稳定；真实 SDK e2e 在未显式设置 `PIXIV_SDK_E2E=1` 或 `FANBOX_SDK_E2E=1` 时跳过。
显式启用后，缺少本机授权凭据或 FANBOX 非 secret target 会直接失败并暴露缺口，不会以 skip 伪装 release evidence。

`scripts/test-e2e.sh` 只选择当前的 public SDK E2E 测试：Pixiv 测试从本地 `pixiv-cli.db` 读取选中账号，
FANBOX 测试从约定的 macOS Keychain item 读取 `FANBOXSESSID`。FANBOX 的
`FANBOX_E2E_CREATOR_ID`、`FANBOX_E2E_TAG`、`FANBOX_E2E_POST_ID` 与 `FANBOX_E2E_POST_URL` 只接受
显式、非 secret 的测试目标；不接受 refresh token、session 或完整 Cookie 作为参数/环境变量。可选的
`PIXIV_E2E_PROXY` 只表示非 secret 的代理 URI；`FANBOX_E2E_SOLVER_URL` 与
`FANBOX_E2E_SOLVER_PROXY` 是可选的非 secret recovery 拓扑配置，默认不启用 solver。未显式启用真实 E2E 时测试默认 skip；显式启用但缺少本机
凭据或 FANBOX 目标时会失败，不能把默认 skip 或自动发现记为 release evidence。

v1 的真实 SDK E2E 是 `TestRealPixivSDKRead` 与 `TestRealFanboxSDKRead`（见 [测试](#测试) 的 `PIXIV_SDK_E2E=1` / `FANBOX_SDK_E2E=1` 命令）。Pixiv 侧测试进程只从本地 `pixiv-cli.db` 的选中账号读取 refresh token，打开 `sdk/pixiv` 验证 identity 并完成一个稳定 detail/list 与 `Resource` 读取，rotation 后的 credentials 先按正常 repository transaction 持久化再继续内容请求。FANBOX 侧直接通过 macOS Keychain 读取授权 `FANBOXSESSID` item，并使用显式 creator/tag/post/page URL 目标逐项验证 `Creator`、`Creators`、`CreatorTags`、`CreatorPosts`、`TaggedPosts`、`Post`、`Home`、`Supporting`、`ResolveURL`、`OpenResource` 与 `SaveResource`；列表目标在服务端返回 cursor 时各跟进一次 continuation，帖子详情必须发现 file attachment 并在临时目录完整读取。session 失效时明确报 `credentials_expired` 并要求重新导入，不 fallback。release-prep 运行后由操作者扫描 stdout、stderr、test log 与 evidence；token、Cookie、signed URL 与原始 response body 不得进入 argv、环境 dump、日志、test name、artifact 或失败 diff。以上说明描述测试覆盖，不表示真实 e2e 已经运行；请勿把 token 写入 shell history、日志或仓库文件。

对于合法但没有 file attachment 的文章详情，补充使用 `TestRealFanboxSDKPostInfo`：它只要求显式
post ID/page URL，验证公共 SDK 的 `Post`、非空 body、`ResolveURL` 与资源清单，并允许
`file_assets=0`；它不能替代严格资源路径的 `TestRealFanboxSDKRead`，严格路径会对详情中的每个
file attachment 完成 HEAD、完整保存和字节数核对。

显式代理下，资源传输固定协商 HTTP/1.1，而 App API、OAuth 保持原有协议协商。该 e2e 的资源读取用于回归这一资源传输边界；它不为慢速正常下载增加固定超时。若 Pixiv 返回不带有效 `Retry-After` 的 429，真实 e2e 保留诊断并明确失败，不会猜测等待或无限重试。

`PIXIV_E2E_BINARY` 与 `PIXIV_E2E_EXPECTED_VERSION` 供 CI 对已构建、已解压的 release binary 执行离线 e2e；它们不注入 token，也不启用真实 Pixiv API。`platform-smoke.yml` 在六个受支持 runner 上构建、封装、解压并运行这组 CLI/config/MCP stdio 验证。

代码改动完成前，应按变更范围补充或更新测试。若不能运行测试，需要在交付说明中写明原因和风险。

发布相关的本地 fixture/策略门禁还包括：

```bash
sh scripts/test-build-staticlibs.sh
sh scripts/test-package-release.sh
go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml
go test ./scripts/cmd/nativeevidence -count=1
go run ./scripts/cmd/nativeevidence policy --workflow .github/workflows/native-evidence.yml
go test ./scripts/internal/browsernativeevidence -count=1
go run ./scripts/cmd/browsernativeevidence policy --workflow .github/workflows/browser-evidence.yml
go test ./scripts/tests/platformsmokeworkflow -count=1
sh scripts/test-homebrew-formula.sh
git diff --check
```

fixture 只证明格式、失败语义和本地策略，不替代六个 native runner 的真实静态链接、GIF/APNG
smoke、版本化 archive 内容和 Homebrew 安装验收。

`.github/workflows/ci.yml` 与 `.github/workflows/platform-smoke.yml` 会先对 PR/main 的 diff 执行严格路径分类。仅 `README*.md`、`docs/**`、`changelog/**` 或 `skills/**` 的改动保留名称稳定的 Quality gate，但只运行 `go test ./scripts/tests/documentation -count=1`；六平台 packaged-binary smoke 会被标记为 skipped，始终执行的 `Platform smoke gate` 会核对这是预期结果。任一其他路径、空 diff、无法比较的初始 push 或手动触发都执行完整 Linux quality gate（test、race、vet、build、package/release policy、pre-commit）和六平台离线已打包 binary smoke；同一汇总 gate 只有在全部 matrix 成功后才通过。CI 的 Windows runner job 聚焦 root callback wiring 的 `TestAuthURLCallback*`、`TestAuthURLHandlerInstall` 与 `TestNormalCLIInvocationEnsuresPersistentHandlerWithoutBlockingCommand`，再运行 `internal/cli/commands/pixiv/auth/loginhelper` 的完整原生 callback-handler 契约；完整 `internal/cli` 已由 Linux quality gate 覆盖，避免把无关的全包 SQLite 压力拖入 Windows handler job。`.github/workflows/browser-evidence.yml` 只在 browser provider 相关输入变更的 `main` push 或手动 dispatch 上运行无凭据的 macOS/Linux/Windows provider contract matrix。分类器无法读取 diff 时明确失败，绝不静默跳过。所有 workflow 都使用只读权限与固定 SHA action；真实 Pixiv/FANBOX SDK E2E 不进入 PR/main 常规 CI。仅发布 tag 的 `release.yml` 会在 validate 后运行无凭据 SDK E2E contract gate，production build 明确依赖该 job；真实 SDK E2E 仍按 release-prep 在授权环境独立验收。

`.github/workflows/pr-metadata.yml` 在 PR `opened`、`reopened` 与 `synchronize` 时使用 `pull_request_target` 更新元数据：`actions/labeler` 从 base branch 的 `.github/labeler.yml` 按路径叠加已有的 `area: docs`、`area: frontend`、`area: backend`、`area: github-actions`、`area: tests` 和 `release` 标签；随后只将 PR 作者追加为 assignee，绝不移除人工指派或标签。该 job 仅有 `contents: read` 与 `pull-requests: write`，不 checkout、不运行 PR 分支代码，因此 fork PR 也不会获得写权限或执行不受信任输入。工作流与配置首次合并到默认分支后才会对后续 PR 生效；引入该配置本身的 PR 需要在 GitHub 手动补标签。

`scripts/tests/installers` 使用本地伪 Release、伪 `curl` 与 checksum fixture 验证安装器，不访问 GitHub。Unix
job 实际运行 `install.sh`，覆盖 SHA-256、带空格目录、版本预检和校验失败不覆盖旧 binary；Windows
amd64/arm64 platform-smoke 还会用真实 `cmd.exe`、`certutil.exe` 与 `tar.exe` 运行 `install.cmd`，并始终
传入 `--no-path`，因此测试不修改 runner 用户注册表。平台 workflow policy 固定要求这项测试存在。

其中 Windows 的 `.zip` 由 GitHub runner 镜像预装的 `7z` 生成；其他平台继续使用 `zip`。
`scripts/test-package-release.sh` 会在 Windows runner 把伪造的调用委托给真实 `7z`，在其他开发机使用
`zip` fixture，并核对 archive member；因此 Git Bash 缺少 `zip` 会在 release test gate 直接暴露。
它用 MSYS 的 `winsymlinks:nativestrict` 创建受检链接：若 runner 不能创建原生 Windows link，测试会显式
失败，避免 Git Bash 的普通文件伪链接让 output ancestor 安全门形同虚设。

### 测试文件布局

生产文件 `x.go` 对应同目录最多一个 `x_test.go`；平台专用测试用 `x_<platform>_test.go`，必须有真实 base owner。新 owner 的测试一律用 external test package（`X_test`）；只有下列目录允许 same-package，因为它们观察未导出的内部状态。新增 same-package 例外必须在此登记 permanent 理由，否则视为违规。

| 目录 | same-package 理由 |
| --- | --- |
| `internal/cli` | composition root 测试观察未导出的 root wiring、invocation lifecycle 与 close ordering；这些 seam 不构成公开 API。 |
| `internal/browsercookies/chromium` | 测试直接构造 provider 并注入 encryption key override，观察未导出的 cookie 记录解密路径与 profile 发现逻辑。 |
| `internal/browsercookies/firefox` | 测试观察未导出的 profile 发现（`profiles.ini` 解析）、cookie 数据库路径解析与记录布局。 |
| `internal/browsercookies/safari` | 测试直接调用未导出的 `parseBinaryCookies`，断言 binarycookies 记录布局。 |
| `internal/browsercookies/secret` | 测试构造 `SecretService{command: ...}` 注入未导出字段并断言未导出的 sentinel error 与命令输出脱敏行为。 |
| `internal/update/installer` | 测试注入未导出的 `assetURLValidator` seam 与 checksum 校验函数，用真实 fixture 二进制验证 root `--version` 预检与失败时不替换旧可执行。 |
| `internal/update/release` | `source_route_test.go` 观察未导出的 source route 选择与 canonical API URL cache 状态；该目录其余测试已用 external package。 |
| `scripts/internal/browsernativeevidence` | 测试观察未导出的环境探测并注入合成 Firefox cookie 种子。 |
| `scripts/internal/changescope` | 测试直接调用未导出的路径解析（`splitNULPaths`、`docsOnlyPaths`）与 change-scope 判定。 |
| `scripts/internal/homebrewformula` | 测试直接调用未导出的 formula 渲染与版本校验（`renderFormula`、`validateFormulaVersion`、`checkDynamicVersionNeeds`）。 |
| `scripts/internal/licensebundle` | 测试观察未导出的 `defaultBundleFileOps`、`generateFromTargetMetadata` 与 license 文本归一化，注入假 cargo metadata。 |
| `scripts/internal/linuxabi` | 测试直接调用未导出的 glibc 版本解析与 ABI 比对（`parseGLIBCVersion`、`checkImportedSymbols`）。 |
| `scripts/internal/nativeevidence` | 测试直接调用未导出的 record/consolidate/policy seam，覆盖 schema 2、独立 `source_commit`、精确 binary `--version` 输出、六目标 hash/archive 校验与 mutation 回滚。 |
| `scripts/internal/prepublishhomebrew` | 测试注入未导出的 `test.mutate` 与 CI 变更检测，观察 formula 生成与预发布检查的失败路径。 |
| `scripts/internal/publicapi` | 测试观察未导出的 `unexported`/`hidden` 符号解析与 golden 比对逻辑，用 `writeFixture` 生成 fixture。 |
| `scripts/internal/releaseassets` | 测试注入未导出的 `injectReleaseSources`/`injectWindowsReleaseSources`，观察 asset archive 命名（`archiveName`）与 checksums 生成。 |
| `scripts/internal/releasenotes` | 测试观察未导出的 GitHub client 调用映射，注入 fake client 断言来源审计。 |
| `scripts/internal/releaseworkflow` | 测试注入未导出的 `mutation.mutate`/`mutation.run` 观察工作流状态机与 git 环境注入，锁定 Version-only linker 与 root `--version` Homebrew 门禁。 |

当前没有 temporary 项。本清单不接受「迁移期」「未来」类无期限表述。新增目录须说明观察的**具体未导出符号**并确认导出最小接口不可行；删除目录须提供删除任务与测试迁移证据（external package 可编译 + 覆盖率不变）。

`e2e/` 与 `scripts/tests/clawhubworkflow/` 也是 `package X`，但它们没有生产代码（纯测试载体），「观察未导出生产状态」问题不适用，不在本清单范围。跨平台差异：带 build tag 的测试文件（如 `scripts/internal/*`）在不同 `GOOS`/`GOARCH` 下可见文件数不同；验证时分别用 `GOOS=darwin`、`GOOS=windows`、`GOOS=linux` 运行 `go list` 确认目录集合一致。

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

这是 v1 中**不得有任何入口**的能力的维护者侧权威清单，是负面契约：为下列能力新增 CLI/MCP/SDK 入口即为缺陷。禁止以 schema 占位或 mock 空结果「预留」。

**Unsupported（v1 明确不支持；新增入口即缺陷）：**

| ID | 唯一 owner | 当前证据 | close-out 条件 |
| --- | --- | --- | --- |
| `ART-SEARCH-RATING` | `internal/cli/commands/pixiv/search` + `sdk/pixiv` | CLI `--rating` 报告 "rating filter is not supported by the v1 App API search contract"；MCP `search_illust` schema 无 rating 参数 | 仅当 v1 App API search contract 新增 rating 语义；届时同步 SDK 字段、CLI flag、MCP schema、locale 文档与本清单 |
| `NOVEL-SEARCH-ADVANCED` | 无 owner（不得新增） | SDK/MCP schema 无 advanced 字段 | 上游 contract 出现后可评估；禁止 schema 占位 |

**Evidence-gated（当前无入口；新增须先满足 close-out 条件）：**

| ID | 唯一 owner | 当前证据 | close-out 条件 |
| --- | --- | --- | --- |
| `NOVEL-RANKING` | 无 owner | SDK 无 `NovelRanking` 导出；MCP 无 `novel_ranking` tool | 上游 App API 提供小说排行后 |
| `NOVEL-BOOKMARK-MUTATION` | 无 owner | SDK 无 `AddNovelBookmark` 类导出；`user_novel_bookmarks` 只读 | 同上 |
| `COMMENT-WRITE` | 无 owner | MCP `comment_post`/`comment_add` 目录 = 0；SDK `PostComment`/`DeleteComment` 导出 = 0 | 上游提供可验证的写入 contract 后 |
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
darwin/linux/windows × amd64/arm64 的 Rust staticlib、测试 Go/Rust、检查许可证并封装固定名称的 archive。
该 workflow 不读取或注入 Pixiv/FANBOX credential。真实 SDK E2E、native browser 和一次性 solver acceptance
必须在授权环境按本页对应流程完成，不能把 contract gate 当作真实 release evidence。

`releaseassets finalize` 还从 immutable tag 读取 `scripts/install.sh` 与 `scripts/install.cmd`，把它们以固定
名称复制到 Release，并与六个平台 archive 一同写入 `checksums.txt` 和 Ed25519 签名 manifest。publish
policy 锁定 finalize 参数及完整八资产上传集合；Homebrew renderer 也要求 checksum 集合包含两个 installer，
但 formula 仍只下载对应平台 archive。

release.yml 只接受 `v[0-9]*` tag push，不再提供 `workflow_dispatch`、`release_tag` 输入或 test-only
overlay。tag run 失败时应修复默认分支上的原因，并按正常的不可变 tag 发布流程重新处理；不会从默认分支
把新 verifier、测试或生产源码注入旧 tag，也不会为已有 Release 提供手工 recovery 入口。validate、test
build、production build 与 publish 都绑定同一个 tag；生产构建在独立 runner 上从 clean tag tree 重建
staticlib，并继续以 `git diff --exit-code` 做 byte-for-byte 校验。

GitHub Release 与 GHCR 是独立系统，无法原子提交。因此容器发布拆成 Release 前的 `build_container`
和 Release 后的 `publish_container`。若 GHCR 发布失败，release workflow 必须保持 failed；恢复方式是用
同一批 verified-container artifact 和 immutable tag 重跑失败的 `publish_container` job——不要为了修复
registry 发布而重建或重签 native 资产。exact-version manifest 总是推送；只有现有 channel classifier
报告 stable 时才推进 `latest`。不使用 retry loop 隐藏 push 失败。

### 容器发布验证

`build_container` 在共享 `build` 门禁后运行，并与 `build_production` 并行；它不会等待生产资产重建。两个原生
target 分别是 `ubuntu-22.04` 对应 `linux/amd64`、`ubuntu-22.04-arm` 对应 `linux/arm64`。每个 target 都从
immutable tag checkout，在 clean tree 重建对应 Rust staticlib，通过 Linux ABI gate 构建版本化 Linux binary，
运行容器打包测试，构建 pinned glibc runtime 镜像，并验证非 root 执行、精确版本、`/home/pixiv/.pixiv-cli/`
下的 `pixiv config path`、`/work` 以及 OCI provenance（`org.opencontainers.image.source`、revision、version
和 licenses），最后导出 `verified-container-linux-amd64` 与 `verified-container-linux-arm64`。build job 只持有
`contents: read`；只有 `publish_container` 在 GitHub Release 后用 `packages: write` 消费这些 artifact。

维护者聚焦检查：

```bash
go test ./scripts/internal/releaseworkflow -count=1
go test ./scripts/tests/containerrelease -count=1
go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml
```

无凭据容器 smoke workflow 会在相关变更时构建两个原生架构，并执行 version、非 root、state-path 和工作目录
断言；正式 tagged release 不能用这些本地检查替代该 CI evidence。

Release policy 的共享契约位于 `scripts/internal/releasecontract` 与 `scripts/internal/workflow/yaml`。
前者持有唯一的 per-target Rust toolchain 映射和六平台契约，后者提供 YAML AST 安全操作；两者都直接
参与正常 release policy 和 production build 校验。保留的 release verifier 测试只覆盖 tag trigger、
build quality、production isolation、publish/Homebrew policy、release notes 与 workflow YAML 安全边界；
历史 recovery 计划和验收报告保留原始文字与路径，不作为当前流程说明。

### Verifier 源码导航

`scripts/cmd/releaseworkflow/` 按发布职责分卷：`main.go` 负责命令入口、文件读取与顶层 dispatch；
`build_policy.go` 负责 validate、test build 与 production build；`e2e_policy.go` 锁定受保护真实 E2E 的
Environment、secret 可达性、输入映射与 build 依赖；`workflow_policy.go` 负责 tag trigger、
job、step、command、action 与 permission helper；`publish_policy.go` 负责 source verification、发布、签名与 channel；
`homebrew_policy.go` 负责 formula render、四平台验证与 tap deploy。测试集中在各 policy 文件及
`releaseworkflow_test.go`、`homebrew_policy_test.go`、`release_notes_policy_test.go` 与
`workflow_policy_test.go`；命令入口不再保留只验证透传的顶层 `main_test.go`。

`scripts/cmd/nativeevidence/` 按 evidence 生命周期分卷：`main.go` 负责 subcommand 与 flag；`models.go` 保存
target 和 evidence schema；`record.go` 记录单 runner evidence；`consolidate.go` 校验并合并六目标结果；
`archive.go` 负责 release archive member 与 JSON；`filesystem.go` 负责路径、hash 和安全文件操作；
`workflow_policy.go` 只验证 native-evidence workflow。测试分别覆盖 policy、record、consolidate，fixture
helper 再按 workflow 与 evidence/archive 分开，避免把策略测试重新堆进单一文件。

`go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` 启动 release workflow 的 YAML AST policy，而不是依赖
文本排版或行号。它精确检查 tag trigger、九份 job 的权限/依赖、受保护 E2E 的 secret 映射和发布阻断、六个 test/production runner matrix、每一个
`uses` 的 40 位 SHA，以及 publish 的 SemVer channel 调用。默认分支 ancestry 必须在无
`environment`、无 secret 的 `verify_release_source` job 中完成；只有该 job 成功后，publish 才可
依赖它并声明精确的 `release` Environment、使用签名 metadata step 的两个预期 secret。policy 对所有
scalar 中的 GitHub expression 按表达式边界扫描 `secrets` context；单引号字符串中的 `}`/`}}`
以及两个单引号转义不会提前结束扫描，因此签名 metadata step 之外的格式化 secret 引用也会
fail-closed。policy 还会拒绝 required job、默认分支 ancestry step 与 quality gate 的 `continue-on-error` 或条件 `if`；validate
与 build checkout 也必须显式 `persist-credentials: false`。为避免 shell 控制流隐藏 gate，每项质量
检查都是唯一的单命令 `bash` step：policy 精确验证其 run、crate cwd（Rust gate）和 shell，并拒绝
未审计的 `env`、`defaults` 或其它 step 字段。唯一允许的变量是 root 的 `RELEASE_TAG`，以及 build
matrix 绑定的 `CC` 与 per-target `RUSTUP_TOOLCHAIN`；Windows 必须使用 `clang -fuse-ld=lld` 链接 MSVC Rust staticlib，避免 MinGW
GCC 与 `.lib` ABI 混用。解析器同时 fail-closed 地拒绝 YAML alias、merge key 和任何重复 mapping key，
因此 GitHub 的覆盖或工作目录语义不会与本地检查分叉。validate 固定 checkout 受审计的 workflow SHA；
其余生产 source checkout 固定为精确 tag。尤其
`verify_release_source` 只能按顺序执行 full-history、无凭据的 tag checkout 与默认分支 ancestry gate
这两个步骤，禁止 `ref`、`repository`、`path` 或中间切换 HEAD 的 step 改变被验证的提交。publish 的
checkout 同样只允许无凭据 tag source，避免签名 metadata 与构建 asset 所属提交不一致。
build job 必须实际运行 vendored Rust 离线检查、crate cwd 的 `cargo fmt --check` 与 locked/offline
Clippy `-D warnings`、普通 Go 测试、vet、许可证、封装、固定版本 `pre-commit==4.6.0`、
pre-commit 和 `git diff --check`；production build job 只从 clean tag tree 生成
`verified-release-*` artifact。发布渠道仅可由
`go run ./scripts/cmd/releaseassets channel --version ...` 判定；build metadata 中的连字符不会使 stable
tag 误变为 prerelease。

Go 1.26.3 不支持 Windows ARM64 的 race detector，因此该唯一 matrix entry 显式跳过 `go test -race`；
其余五个原生目标仍运行 race gate，workflow policy 固定这个条件，禁止扩张为任意条件跳过。
test matrix 还固定 `GIT_CONFIG_*` 为 `core.autocrlf=false`，使 Git for Windows checkout 保留 immutable
tag 的 LF blob bytes；否则 pre-commit 的 `gofmt` 会把 runner 的 CRLF 转换误报为源码未格式化。该配置
仅用于 test gate，独立 production build 仍从 tag 的干净默认 checkout 构建资产。

publish 核对并公开 Release 后，立即上传同一份 `release/checksums.txt`；policy 拒绝中间 step、路径
替换或发布后改写。`render_homebrew_formula` 只下载该 artifact，并把 releaseassets 的 stable/
prerelease 结果直接映射为 `pixiv-cli`/`pixiv-cli-beta`。随后精确四目标 matrix（macOS Intel/arm64、
Linux amd64/arm64）先用 `brew tap-new pixiv-cli-release/staging --no-git` 创建各 runner 的隔离
local tap，再以 `brew trust --tap pixiv-cli-release/staging` 显式信任这一个临时命名空间；将唯一
staging formula 放入其 `Formula/`，随后用 `pixiv-cli-release/staging/<formula>` 执行真实
`brew install --formula`。macOS 在原生 runner 的临时 tap 中运行；Linux 在短生命周期、固定 digest 的
`homebrew/brew` 容器内运行，并将 staging formula 目录以只读 bind mount 传入容器。随后执行
`test "$(pixiv --version)" = "pixiv $RELEASE_TAG"` 并与 tag 比较。它不使用 workspace formula path、developer/环境变量 bypass，
也不克隆、写入或信任公开 tap。只有全部成功，最终受保护 `deploy_homebrew_tap` 才以 HTTPS
clone public tap、核对唯一 staged formula，并在最后一个 step 读取 deploy key；SSH push 固定官方
GitHub ED25519 known_hosts、启用 strict checking，目标精确为 `HEAD:main`。任何前置 job 失败都不会
写 tap。

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
安装验收。`go run ./scripts/cmd/prepublishhomebrew --workflow .github/workflows/homebrew-prepublish-verify.yml` 会在本地和质量门中检查该 workflow 的不可变边界。

正式发布目前仍必须被正式 tag、签名 GitHub Release、tap formula 与后续安装验收阻断。完整
six-target staticlib/manifest 与真实 native artifact 证据必须已受控收集并回填（见「Rust ugoira staticlib」一节）；
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
都安装 `pixiv` 并相互冲突。专用 tap deploy key 的私钥只放在 source repository
的受保护 `release` Environment secret `HOMEBREW_TAP_DEPLOY_KEY`，公开 tap 只登记对应公钥。workflow
在独立 renderer 中生成 staging formula，并在四个原生 runner 验证安装，再由最终 protected job 做
受限提交/push。后续 stable/beta 发布仍不能从本仓库或
workflow artifact 读取、生成或记录 deploy key。

当前 Release 不会进行 Apple notarization 或 Windows Authenticode。直接下载仍可能被 Gatekeeper
或 SmartScreen 拦截/提示；这是需要在用户文档中保留的系统信誉边界，不能通过文档或脚本绕过。

成功结束的 `Release` workflow 会同时触发 `.github/workflows/publish-skillhub.yml` 与 `.github/workflows/publish-clawhub.yml`。GitHub 以
`github.token` 创建 Release 时不会递归触发 `release` event，因此不能将该 event 用作可靠的自动化
交接。完成 Homebrew 部署的 Release 会交出只含精确 release tag 的短期 artifact；这避免恢复发布的
`workflow_run.head_branch` 为 `main` 时把分支名误作版本。SkillHub workflow 只 checkout 该不可变 tag，
并确认该 tag 属于默认分支、对应 GitHub Release 已公开且版本满足 SemVer 后，才对
`skills/pixiv-cli/` 与前一个已合并的语义版本 tag 比较。目录未变化时工作流成功跳过；目录变化时才运行
SkillHub CLI 的 dry-run 和提交。产品 `SKILL.md` 的 SemVer 必须与 CLI Release tag 相同；release 的 tag-source
validation 会在受保护 E2E 和任何发布凭据之前拒绝不匹配的版本。`SKILLHUB_TOKEN`
仅进入最后的提交步骤，CLI 必须返回 `skillId` 和审核状态；这证明 SkillHub 已接收提交，但平台审核完成前
公开详情页可能仍不可见。
若任一独立发布失败，可通过对应 workflow 的 `workflow_dispatch` 输入既有发布 tag 恢复，不能用 main 的
后续内容替代该 tag。ClawHub workflow 与 SkillHub 使用同一不可变 tag handoff：它先验证公开非 draft Release、默认分支祖先关系、`SKILL.md` 版本与 tag 一致性以及产品 skill 的改动，再在不含凭据的环境运行固定版本的 ClawHub CLI dry-run，并以其 SHA-256 产物指纹校验实际发布物。只有最终 publish/inspect 步骤，以及不重发版本的 `verify_only` 人工恢复步骤会收到 `CLAWHUB_TOKEN`；后者只登录并读取审核结果，绝不调用 publish。正常 publish 必须确认产品 skill、对应版本和精确产物指纹；当 ClawHub static scan 已 clean、但聚合安全结论仍为 `pending` 时，会明确 warning 而不把已接收的发布物误报为失败。`skill-card.md` 也可能异步生成并产生 warning。两类 warning 都不等同于最终安全结论：`verify_only` 只在 aggregate security 为 clean 时通过，便于在平台扫描完成后作不重发的最终核验。任何其他原因仍会失败。当前平台也不会为普通 CLI publish 暴露 server-resolved GitHub provenance，因此该项以受信 tag checkout 和指纹匹配替代，并同样保留 warning。

</details>

## Git 与本地产物

`.gitignore` 已排除：

- `.DS_Store`
- 构建产物 `build/`、`pixiv`、`pixiv-cli`
- 本地下载目录 `downloads/`
- 本地数据库 `*.db`
- 常见缓存和临时文件
- Rust `internal/media/ugoira/rust/target/`

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
