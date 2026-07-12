# 开发流程

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

当前工作树只含 Darwin/arm64 library，完整 manifest 与另外五个真实库尚未获得。因此：

- `sh scripts/build.sh` 当前应以“缺少 committed six-target Rust staticlib manifest”明确失败；
- 不应把本机 host library 复制、改名或当作别的平台产物；
- 在 Task 13/33 的 native runner 证据完成前，不应宣称 source build 或 `go install` 已跨平台可用。

可在具备目标工具链的受控环境运行：

```bash
sh scripts/build-staticlibs.sh --target <rust-target>
go test ./internal/download -run '^TestCommittedUgoiraStaticlibManifestWhenPresent$' -count=1
```

不要提交 `internal/download/ugoira_rs/target/`；它是机器产物。完成验证的
`internal/download/ugoira_rs/staticlib/`、其 `manifest.json` 和两份 knowledge graph 则是可追溯
输入，不能以 ignore 规则隐藏。

Rust crate 的 `.cargo/config.toml` 将 crates.io 替换为其相邻 `vendor/` 中完整的 locked
依赖闭包。`vendor/` 的每个 package 都带 Cargo 生成的 `.cargo-checksum.json`；它、Cargo config、
`Cargo.toml`/`Cargo.lock`、Rust source 和本地 `quantette` 都计入 staticlib source digest。不要手工
编辑 vendor 内容；升级依赖时必须重新以 `cargo vendor --locked --offline` 生成完整闭包并更新 digest
fixture 与许可证 bundle。`target/` 仍是机器产物，不计入 digest，也不得提交。

直接运行 Cargo 时必须在 crate 目录启动，确保 Cargo 发现 source replacement：

```bash
(
  cd internal/download/ugoira_rs
  cargo test --locked --offline
  cargo clippy --locked --offline --all-targets -- -D warnings
)
go run ./scripts/licensebundle --check
sh scripts/test-rust-vendor.sh
```

`scripts/test-rust-vendor.sh` 为 release workflow 的聚焦供应链回归：它建立临时空 `CARGO_HOME` 与
`CARGO_TARGET_DIR`，随后依次执行 `cargo metadata/build/test --locked --offline`，再以相同环境运行
六个 release target 的 `go run ./scripts/licensebundle --check`。因此 registry cache、网络 fallback、
缺失 vendor 内容或无效 checksum 都会明确失败，不能把 runner 的预热缓存当作离线可复现性证据。

### Native runner evidence 与 Task 13 受控回填

`.github/workflows/native-evidence.yml` 是独立的、非发布的 runner 入口：只允许审计后的 `main`
push 或指向 `refs/heads/main` 的 `workflow_dispatch`，全局 `permissions: {}`、job 仅 `contents: read`。它没有 `environment`、
secret、tag/Release/tap/signing 命令；YAML AST policy 同时固定六个 runner、full-SHA action、无凭据
checkout、vendored Rust 检查、单目标 staticlib、真实 cgo GIF/APNG smoke、版本化 binary 的
`pixiv version --json`、release-style archive 以及 artifact upload。可离线检查声明本身：

```bash
go test ./scripts/nativeevidence -count=1
go run ./scripts/nativeevidence policy --workflow .github/workflows/native-evidence.yml
```

该 policy command 只依赖 `internal/download/staticlib` 的 source-digest/manifest 契约，不导入 cgo
encoder；因此它必须能在每个 runner 构建目标 staticlib **之前**执行。若 policy gate 因缺库或 cgo
link 失败，属于 workflow bootstrap 缺陷，而不是可接受的“尚无 native evidence”结果。

每个 runner artifact 只有 `evidence/`：实际链接的 staticlib、版本化 binary、archive 及
`native-evidence.json`。record 会重算 Rust source digest 和三份 SHA-256，执行 binary 的
`version --json`，并逐一检查 archive 的 binary、`LICENSE`、`THIRD_PARTY_LICENSES.md` 与完整
`third_party/licenses` 常规文件树。它不持有 release/tap/signing credential，也不会创建 tag 或
Release。

**本地 unit fixture、policy 成功或 workflow 文件存在都不是 native runner evidence。** 在 Task 20 对
审计过的 `main` 只进行一次受控 push 之前，不得把它们当作六目标 staticlib 或发布资格。

Task 20 的 main push 成功后，Task 13 只能按以下过程回填可提交的 blobs：

1. 记录该 push 的精确 main SHA 和同一个成功 `native-evidence` workflow run 写入的版本
   `v0.1.0-native-evidence.<run-id>`；在 GitHub 检查该 `push/main` run 的 head SHA 完全相同，再下载
   **恰好**六个 `native-evidence-{darwin,linux,windows}-{amd64,arm64}` artifact。不得使用 tag、Release、
   手动 fixture 或不同 SHA 的 dispatch run；这两个值是受控回填的必填门禁，而非只靠人工阅读。
2. 在干净、非 symlink 的审计目录解压 artifact，保留每个平台的 `native-evidence.json`、staticlib、
   binary 和 archive；人工确认 run URL、matrix target、source SHA 与六个 artifact 名称。运行：

   ```bash
   go run ./scripts/nativeevidence consolidate \
     --repo-root . \
     --expected-version v0.1.0-native-evidence.<run-id> \
     --expected-commit <exact-main-sha> \
     --input-dir .native-evidence-download \
     --output-dir .native-evidence-backfill/staticlib
   ```

   该命令只接受完整六目标、同一 source digest 以及同一 expected version/commit 的记录；重新核验
   staticlib/binary/archive SHA 与 archive member hash，生成精确六条 `manifest.json`。任何缺 target、
   重复/错配 target、metadata、
   archive member、哈希或 symlink 都会在写入前阻断；输出必须是不存在的新目录，因此不会覆盖或发布
   部分结果。
3. 人工复核 `.native-evidence-backfill/staticlib` 与六份 record 的 target、source digest、SHA-256 和
   archive members，确认 artifact 均来自上述 main run。通过后才把该目录中六个 target library 与
   `manifest.json` 明确回填到 `internal/download/ugoira_rs/staticlib/`；逐文件复核哈希，随后审查
   `git diff`，不得复用或改名 host library。
4. 在回填后的工作树运行
   `go test ./internal/download -run '^TestCommittedUgoiraStaticlibManifestWhenPresent$' -count=1`、
   `go test ./internal/download -run '^TestRustUgoiraEncoderNativeGIFAndAPNG$' -count=1` 与
   `git diff --check`，再把六个 blobs、manifest、对应 evidence/review 摘要和更新后的 knowledge graph
   作为 Task 13 的独立审查提交。任一验证失败都阻断 release，不能以部分 artifact 继续。

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
./build/pixiv mcp --no-proxy
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

浏览器使用的系统代理不会自动传给 Go CLI。若 Pixiv token exchange 需要代理，请配置 `pixiv config set https_proxy http://127.0.0.1:7890`，在单次命令前设置 `https_proxy=...`，或对网络命令使用运行期覆盖 `--proxy http://127.0.0.1:7890`。`--no-proxy` 会清空本次命令的代理，即使环境变量或 `config.toml` 设置了 `https_proxy`；`--proxy` 和 `--no-proxy` 不能同用，也不会写入 `config.toml`。

当前支持代理覆盖的网络入口是 `auth add`、`auth login`、`auth check`、`search`、`detail`、`ranking`、`recommended`、`download` 和 `mcp` 启动。`auth list/use/remove` 与 `config path/get/set/unset` 不接受这些 flag。

真实登录依赖 Pixiv OAuth 网页流程可用。自动化测试使用 fake OAuth server 覆盖 callback 和 token exchange，不访问真实 Pixiv。

## 测试

当前测试覆盖 CLI 命令与 build metadata、显式/自动更新、`internal/application` 应用用例、`internal/config` 配置、`internal/storage/auth` 认证存储、Pixiv App API 认证重试、Pixiv facade/source、web fallback、HTTP client wiring、下载管理、Rust encoder/staticlib 合约和 MCP tool 注册：

```bash
go test ./...
sh scripts/build.sh
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

`go test ./...` 保持默认离线稳定；真实 Pixiv web API fallback e2e 默认跳过，只有设置 `PIXIV_E2E_WEB_API=1` 时才会联网。未设置 `PIXIV_WEB_API_PROXY` 时会直连。

代码改动完成前，应按变更范围补充或更新测试。若不能运行测试，需要在交付说明中写明原因和风险。

发布相关的本地 fixture/策略门禁还包括：

```bash
sh scripts/test-build-staticlibs.sh
sh scripts/test-package-release.sh
sh scripts/test-release-workflow.sh
go test ./scripts/nativeevidence -count=1
go run ./scripts/nativeevidence policy --workflow .github/workflows/native-evidence.yml
sh scripts/test-homebrew-formula.sh
git diff --check
```

fixture 只证明格式、失败语义和本地策略，不替代六个 native runner 的真实静态链接、GIF/APNG
smoke、版本化 archive 内容和 Homebrew 安装验收。

## 发布门禁、签名与 Homebrew 边界

`.github/workflows/release.yml` 为 `v*` tag 定义本地可审查的发布流程：先验证 SemVer，再在
darwin/linux/windows × amd64/arm64 runner 上构建 Rust staticlib、测试 Go/Rust、检查许可证并
封装固定名称的 archive；全部通过后才创建带 `checksums.txt` 和 Ed25519 `checksums.json` 的
GitHub Release。workflow 使用 full-SHA Actions、最小权限及 `release` Environment，但它尚未在
GitHub 实际运行。

`sh scripts/test-release-workflow.sh` 启动 `scripts/releaseworkflow` 的 YAML AST policy，而不是依赖
文本排版或行号。它精确检查 tag trigger、四份 job 的权限/依赖、六个 runner matrix、每一个
`uses` 的 40 位 SHA，以及 publish 的 SemVer channel 调用。默认分支 ancestry 必须在无
`environment`、无 secret 的 `verify_release_source` job 中完成；只有该 job 成功后，publish 才可
依赖它并声明精确的 `release` Environment、使用签名 metadata step 的两个预期 secret。policy 还会
拒绝 required job、默认分支 ancestry step 与 quality gate 的 `continue-on-error` 或条件 `if`；validate
与 build checkout 也必须显式 `persist-credentials: false`。为避免 shell 控制流隐藏 gate，每项质量
检查都是唯一的单命令 `bash` step：policy 精确验证其 run、crate cwd（Rust gate）和 shell，并拒绝
`env`、`defaults` 或其它 step 字段。解析器同时 fail-closed 地拒绝 YAML alias、merge key、任何重复
mapping key 以及 root/job 的 `env`、`defaults`，因此 GitHub 的覆盖或工作目录语义不会与本地检查分叉。
四个 job 的 `actions/checkout` 都固定为同一 full SHA 和精确的 `with` 字段；尤其
`verify_release_source` 只能按顺序执行 full-history、无凭据的 tag checkout 与默认分支 ancestry gate
这两个步骤，禁止 `ref`、`repository`、`path` 或中间切换 HEAD 的 step 改变被验证的提交。publish 的
checkout 同样只允许无凭据 tag source，避免签名 metadata 与构建 asset 所属提交不一致。
build job 必须实际运行 vendored Rust 离线检查、crate cwd 的 `cargo fmt --check` 与 locked/offline
Clippy `-D warnings`、普通及 race Go 测试、vet、许可证、封装、固定版本 `pre-commit==4.6.0`、
pre-commit 和 `git diff --check`。发布渠道仅可由
`go run ./scripts/releaseassets channel --version ...` 判定；build metadata 中的连字符不会使 stable
tag 误变为 prerelease。

这套本地检查只证明 workflow 声明的依赖和语义，**不**验证 GitHub `release` Environment、
secret 和 tag protection 的远端实际状态；它不替代 Task 20 的远端配置审计。

正式发布目前必须被以下条件阻断：完整 six-target staticlib/manifest 与真实 native artifact 证据
尚待 Task 20 的实际 main runner 收集与 Task 13 回填，并且尚未创建正式 tag、GitHub Release 或 tap
formula 提交。受保护 `release` Environment、生产 signing 私钥与公开仓库已按 Task 20 配置；不得以
这些远端前置条件或本地 fixture 成功、仅有 host library 或 workflow 文件存在来创建 Release/tap。

production Ed25519 public trust root 已在
[`internal/bootstrap/release_trust.go`](../internal/bootstrap/release_trust.go) 随源码提交：key ID 为
`ed25519-2c27e77742d3c33a`，其 SPKI DER SHA-256 fingerprint 为
`2c27e77742d3c33ad14be867d4e0519229a220898c9a7c868447eaef0951b4cf`。同包测试以已知真实签名验证
此映射；它只证明公开信任根进入 production wiring，并不证明实际签名、Release asset 或安装验收已经
完成。

生产 Ed25519 信任根的其余规则如下：

- 公钥、key ID 与 fingerprint 已以可审计源码变更进入受支持二进制；私钥绝不进入源码、
  release asset、日志或 formula。
- 私钥只能作为受保护 `release` Environment 的 secret 使用；恢复副本只可保存在受控的 macOS
  Keychain。Task 20 已将其写入这两处；它仍不得进入源码、日志、Release asset 或 formula。
- 轮换时先发布能够信任新 key ID 的版本，保留旧公钥直至旧版本退出支持，再通过新的受签名
  Release 停止使用旧 key。不得让既有二进制突然依赖一个未提交、不可验证的新信任根。

Homebrew tap 是独立发布面：stable 使用 `pixiv-cli`，pre-release 使用 `pixiv-cli-beta`，二者
都安装 `pixiv` 并相互冲突。Task 20 已创建专用 tap deploy key；其私钥只放在 source repository
的受保护 `release` Environment secret `HOMEBREW_TAP_DEPLOY_KEY`，公开 tap 只登记对应公钥。先在
常规 tap checkout 渲染、audit 并在 macOS/Linux 验证安装，再做受限提交/push。当前 tap 尚无
formula 或任何内容提交，也不能从本仓库或 workflow 读取、生成或记录 deploy key。

v0.1.0 不会进行 Apple notarization 或 Windows Authenticode。发布后直接下载仍可能被 Gatekeeper
或 SmartScreen 拦截/提示；这是需要在用户文档中保留的系统信誉边界，不能通过文档或脚本绕过。

## Git 与本地产物

`.gitignore` 已排除：

- `.DS_Store`
- 构建产物 `build/`、`pixiv`、`pixiv-cli`
- 本地下载目录 `downloads/`
- 本地数据库 `*.db`
- 常见缓存、日志、临时文件
- Rust `internal/download/ugoira_rs/target/`
- 两份 understand-anything 图谱的临时/垃圾扫描目录（图谱 JSON 和已跟踪 scan result 除外）

不要提交 Pixiv token、下载内容、本地数据库、机器相关配置、Ed25519 私钥或 tap deploy key。

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
- 安装渠道、更新通道、签名信任根、Release/tap 发布门禁或系统信誉提示。
- 新增限制、重试、超时、截断、降级或错误处理策略。
- 测试或构建命令。
