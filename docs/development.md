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

## understand-anything 图谱归一化

每次重新生成代码图谱后、提交六个 tracked 图谱产物前，必须从仓库根目录运行：

```bash
go run ./scripts/understandgraph normalize --root .
go test ./scripts/understandgraph -count=1
```

归一化器以 `go.mod`、Go AST 和已生成的 scan/graph/fingerprint 为输入：把 generator 展开的
Go file-to-file imports 改为 package module 边，区分 external test package，并统一 method ID 与
fingerprint 的 receiver-qualified name。它保留 non-Go importMap，并用 `docs/` 下当前 UTF-8 源文件
刷新文档图谱中每个 article 的 `knowledgeMeta.content` 全文及 `contentHash` SHA-256，避免 generator
的展示截断进入入库快照；缺少 article 路径、metadata object 或源文件时会在写入前显式失败。归一化前
还会用实际 Go 源码核验 scan 与 fingerprint 的 SHA-256、文件行数和函数行数；快照已过期时同样不会
写入。四份 JSON 会先完整 staging 再逐份 `ReplaceFile`。命令应连续运行两次；第二次不得产生任何
diff，解析、行号匹配或调用目标有歧义时必须先修复根因，不能手工猜测或跳过。

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

当前工作树的六库来自 run `29567721284`（head
`a93378631654f7a19b5e6052f68bdb3650438b03`）。该 run 在六个真实平台 runner 上按下述版本映射
安装 pinned toolchain，并全部通过 policy、精确源码 ref、locked/offline build、真实 cgo GIF/APNG smoke、
版本化 binary、archive/record 与 artifact upload。下载六份证据后，本地 fail-closed consolidation
继续校验同一 version、commit、source digest、目标集合与逐库 hash，再成套生成本目录和 manifest。
合并前使用临时 review ref 只为让 workflow 精确 checkout 该受审计 commit；产物回填后 workflow
已经恢复只接受 `refs/heads/main`。
六库 manifest 的 Rust source digest 为
`2f076376eb8a0ce0142fa6b03e856ef0e570c3d99b5fe98a73de0df95c70cc91`，与该 commit 的 vendored
Rust 输入一致；不得用旧 run `29559729696` 的 runner 默认 Rust `1.97.0` 产物替代。

合规 committed library 的编译器 provenance 必须按 target 固定，而不是使用可移动的 runner 默认
toolchain：`x86_64-apple-darwin` 与 `x86_64-pc-windows-msvc` 使用 Rust `1.96.0`；
`aarch64-apple-darwin`、`aarch64-pc-windows-msvc`、`x86_64-unknown-linux-gnu` 与
`aarch64-unknown-linux-gnu` 来自 Rust `1.96.1`。release test 与 production matrix 都必须携带这份
精确映射，并通过 `RUSTUP_TOOLCHAIN` 和带 `--no-self-update` 的 `rustup toolchain install` 使用它；
不能让 runner image 的 `stable` 更新改变重建 bytes。该映射记录当前六库及 v0.3.0 recovery 的来源，
不是允许永久混用工具链的惯例。后续升级 Rust 时必须从同一受审计 source 完整重建、链接并 smoke
验证六目标，同时更新六库、manifest、native evidence 与 release matrix；不得只更新单个平台的 pin。

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
slash；否则 Windows 的反斜杠路径会静默漏掉这些输入。run `29189725013` 的六个 matrix job 虽均
success，但其 Unix/Windows source digest 因该路径筛选缺陷分裂，`consolidate` 必须拒绝该 run；其
artifacts 不可回填或跨 run 拼接，修复后仍须从新 SHA 完整重跑六目标证据。
run `29191200569` 的六个 matrix job 均 success，且六份 record 已统一为同一 source identity，证明
上述 source digest 修复有效；但其 Windows archive 中 `LICENSE` 仍因 CRLF checkout 与 Unix/Git
blob bytes 不同，`consolidate` 再次 fail-closed，且未生成输出目录。该 run 同样不可回填或跨 run
拼接；固定 `LICENSE` 为 LF 后仍必须从新 SHA 完整重跑。
`target/` 仍是机器产物，不计入 digest，也不得提交。

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

matrix 的每个 target 还必须声明与 release test/production 完全相同的 `rust_toolchain`，job 通过
`RUSTUP_TOOLCHAIN` 绑定该值，并执行带 `--profile minimal --target ... --no-self-update` 的精确
`rustup toolchain install`。两个 verifier 共用 `scripts/internal/workflowpolicy` 中唯一的目标版本映射；
任一 workflow 删除、替换、重复或错误插值该映射，policy 都会 fail closed。

Windows 两个 target 的 Rust library 使用 `*-pc-windows-msvc`；相应 cgo selector 必须以
`-L${SRCDIR}/… -lugoira_rs` 声明库，不能把带盘符的绝对 `.lib` 路径直接传给 cgo；还必须显式携带
Rust `std` 所需的 `advapi32`、`ntdll`、`userenv`、`ws2_32` 与 `dbghelp` import libraries。native evidence
仅在 Windows 的 smoke 和版本化 binary 构建中显式设 `CC='clang -fuse-ld=lld'`：LLD 既能处理 MSVC
`.lib`，也让 Go 跳过 GCC 专属的 debug linker script；这不是运行时 fallback，也不改变 darwin/linux
的 C linker 选择。

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
   重复/错配 target、metadata、archive member、哈希或 symlink 都会在写入前阻断；输出必须是
   不存在的新目录，因此不会覆盖或发布部分结果。

   若同一 workflow run 的 runner record 出现不同 source digest，说明 checkout bytes 已使该组证据失去
   单一源码身份；即使六个 job 都完成，也不得混合回填该 run 的 artifacts，或与后续 run
   拼接。修复 checkout 字节规则后，必须重新 push 精确的已审计 SHA，并只使用其新产生的完整
   six-target run。
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

CLI 多账号认证保存在 `os.UserConfigDir()/pixiv/auth.json`，账号 key 是 Pixiv UID；全局配置保存在 `os.UserConfigDir()/pixiv/config.toml`。Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。推荐使用 `pixiv auth login` 通过本地 loopback server 和浏览器 OAuth 登录；`auth add` 仍可从 stdin 读取 token，也支持 `--token`，但不建议在共享 shell 历史环境中使用。可用 `pixiv config path/get/set/unset` 管理全局配置。无 refresh token 时默认启用匿名 Pixiv web/ajax API fallback，可用 `pixiv config set web_fallback_enabled false` 关闭。
CLI 使用 Cobra/pflag，flag 可以写在位置参数前后；例如 `pixiv auth check 12345678 --json` 和 `pixiv search "初音ミク" --json` 都受支持。

## 获取 refresh token

浏览器 Cookie（包括 `refresh_token=...`、`PHPSESSID`、`device_token`）不是可接受的 Pixiv App API OAuth refresh token，CLI、MCP、环境变量、SDK 与已存账号都会拒绝这类输入。推荐直接登录并保存账号：

```bash
pixiv auth login
```

| 项 | 说明 |
| --- | --- |
| 本地服务 | CLI 生成 PKCE/state，并启动本地 loopback HTTP server。 |
| 浏览器 | macOS 默认优先注册本地 `pixiv://` callback helper 并打开默认浏览器，因此可复用已有 Pixiv 登录态；需要用户在 Pixiv 页面确认账号；`--no-open` 可改为只打印登录 URL。 |
| 自动/手动回填 | CLI 接收本轮 loopback callback、当前登录尝试注册的 `pixiv://` helper 转交、终端粘贴和本地页面表单；若浏览器没有返回，也可手动粘贴 callback URL、`pixiv://...` URL、Pixiv relay URL 或原始 code。 |
| state 校验 | 本地 loopback 回调必须匹配本次 state；Pixiv 官方 callback URL 与 `pixiv://account/login` 可在 Pixiv 未返回 state 时作为显式 fallback。 |
| token 保存 | refresh/access token 不打印；refresh token 按 Pixiv UID 写入 `auth.json`。Unix-like 主动使用 `0700` 父目录与 `0600` 文件；Windows 首次创建继承父目录 ACL，替换既有目标保留其 ACL，不主动收紧或放宽 DACL。 |

默认浏览器打开时，macOS 会注册一个仅服务于当前登录尝试的本地 `PixivCLIURLHandler.app`，只把 Pixiv 返回的 `pixiv://account/login?...` URL 转交给本轮 CLI loopback。它不读取浏览器 Cookie、存储、历史、会话文件、标签页或网络流量；helper 不可用时不会启动受管 Chromium、DevTools/CDP 或浏览器状态扫描，只保留正常浏览器、loopback 和手动回填。遇到 Pixiv `post-redirect` 授权接力页时，用户可手动粘贴 relay URL；CLI 只在校验其属于本轮 OAuth 后打开一次。浏览器可能停留在白色 relay 页，是否成功以终端最终输出为准；若未生成 callback，CLI 不会隐藏失败或假装登录成功。

浏览器使用的系统代理不会自动传给 Go CLI。若 Pixiv token exchange 需要代理，请配置 `pixiv config set https_proxy http://127.0.0.1:7890`，在单次命令前设置 `https_proxy=...`，或对网络命令使用运行期覆盖 `--proxy http://127.0.0.1:7890`。`--no-proxy` 会清空本次命令的代理，即使环境变量或 `config.toml` 设置了 `https_proxy`；`--proxy` 和 `--no-proxy` 不能同用，也不会写入 `config.toml`。

当前支持代理覆盖的网络入口是 `auth add`、`auth login`、`auth check`、`search`、`search-options`、`detail`、`ranking`、`recommended`、`download` 和 `mcp` 启动。`auth list/use/remove` 与 `config path/get/set/unset` 不接受这些 flag。

真实登录依赖 Pixiv OAuth 网页流程可用。自动化测试使用 fake OAuth server 覆盖 callback 和 token exchange，不访问真实 Pixiv。

## 测试

当前测试覆盖 CLI 命令与 build metadata、显式/自动更新、`internal/application` 应用用例、`internal/config` 配置、`internal/storage/auth` 认证存储、Pixiv App API 认证重试、公开 Pixiv SDK/facade、web fallback、HTTP client wiring、下载管理、Rust encoder/staticlib 合约和 MCP tool 注册：

```bash
go test ./...
sh scripts/build.sh
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
PIXIV_E2E_REAL_API=1 PIXIV_E2E_REFRESH_TOKEN="<独立测试 refresh token>" PIXIV_E2E_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run AuthenticatedAppAPICanary -count=1 -v
PIXIV_E2E_REAL_API=1 PIXIV_E2E_USE_LOCAL_AUTH=1 PIXIV_E2E_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run AuthenticatedAppAPICanary -count=1 -v
```

`go test ./...` 保持默认离线稳定；真实 Pixiv web API fallback e2e 默认跳过，只有设置 `PIXIV_E2E_WEB_API=1` 时才会联网。未设置 `PIXIV_WEB_API_PROXY` 时会直连。上述 Web canary 被显式调用时，会先从匿名搜索结果逐项读取 detail 取得真实宽高，选择可分类的横纵比候选，再执行带 `--aspect-ratio` 的高级搜索并通过 detail 复核返回作品的横纵比；该说明描述测试覆盖，不表示 canary 已经运行。

认证 App API canary 必须设置 `PIXIV_E2E_REAL_API=1`，再明确选择一种认证来源；未选择则跳过，也不会匿名 fallback。`PIXIV_E2E_REFRESH_TOKEN` 是隔离模式：只使用显式传入的独立测试 token，不读取或写入本机 auth 配置、浏览器数据。`PIXIV_E2E_USE_LOCAL_AUTH=1` 是本机模式：子 CLI 复用当前用户的 `HOME`/`XDG_CONFIG_HOME` 和默认账号 store，按正常 CLI 行为把 rotated refresh token 写回本机 store；它不会继承 `PIXIV_REFRESH_TOKEN` 运行期覆盖，也拒绝 `PIXIV_E2E_BINARY`，只构建当前源码后运行。两种来源不能同时设置，且本机模式只应在用户明确授权时使用；运行期间不要并发启动其他使用同一账号 store 的 `pixiv` CLI 或 MCP 进程，以免 rotation 覆盖或旧 token 请求失败。上述认证 canary 被显式调用时，覆盖 `auth check`、`search-options`、完整用户详情和插画/漫画/小说/作者四类推荐；搜索筛选会从认证 baseline 与动态工具选项中选择实际存在的候选，分别要求分辨率、横纵比、作品类型、排除 AI 与绘图工具查询返回非空结果并校验每个返回作品。该说明只描述测试代码及调用方式，不表示 canary 已经运行。可选 `PIXIV_E2E_PROXY`（或 `PIXIV_WEB_API_PROXY`）只作用于该次测试；请勿把 token 写入 shell history、日志或仓库文件。

`PIXIV_E2E_BINARY` 与 `PIXIV_E2E_EXPECTED_VERSION` 供 CI 对已构建、已解压的 release binary 执行离线 e2e；它们不注入 token，也不启用真实 Pixiv API/Web fallback。`platform-smoke.yml` 在六个受支持 runner 上构建、封装、解压并运行这组 CLI/config/MCP stdio 验证。

代码改动完成前，应按变更范围补充或更新测试。若不能运行测试，需要在交付说明中写明原因和风险。

发布相关的本地 fixture/策略门禁还包括：

```bash
sh scripts/test-build-staticlibs.sh
sh scripts/test-package-release.sh
sh scripts/test-release-workflow.sh
go test ./scripts/nativeevidence -count=1
go run ./scripts/nativeevidence policy --workflow .github/workflows/native-evidence.yml
go test ./scripts/platformsmokeworkflow -count=1
sh scripts/test-homebrew-formula.sh
git diff --check
```

fixture 只证明格式、失败语义和本地策略，不替代六个 native runner 的真实静态链接、GIF/APNG
smoke、版本化 archive 内容和 Homebrew 安装验收。

`.github/workflows/ci.yml` 在 PR/main 运行 Linux quality gate（test、race、vet、build、package/release policy、pre-commit）。`.github/workflows/platform-smoke.yml` 在 PR/main 运行六平台离线已打包 binary smoke。两者都使用只读权限、固定 SHA action 与取消过期并发 run；真实 Pixiv e2e 不进入 GitHub Actions。

其中 Windows 的 `.zip` 由 GitHub runner 镜像预装的 `7z` 生成；其他平台继续使用 `zip`。
`scripts/test-package-release.sh` 会在 Windows runner 把伪造的调用委托给真实 `7z`，在其他开发机使用
`zip` fixture，并核对 archive member；因此 Git Bash 缺少 `zip` 会在 release test gate 直接暴露。
它用 MSYS 的 `winsymlinks:nativestrict` 创建受检链接：若 runner 不能创建原生 Windows link，测试会显式
失败，避免 Git Bash 的普通文件伪链接让 output ancestor 安全门形同虚设。

## 发布门禁、签名与 Homebrew 边界

`.github/workflows/release.yml` 默认由 `v[0-9]*` tag 触发：先验证 SemVer，再在
darwin/linux/windows × amd64/arm64 runner 上构建 Rust staticlib、测试 Go/Rust、检查许可证并
封装固定名称的 archive；全部通过后才创建带 `checksums.txt` 和 Ed25519 `checksums.json` 的
GitHub Release。workflow 使用 full-SHA Actions、最小权限及 `release` Environment。

若不可变 tag 的首次 run 在创建 GitHub Release 前失败，维护者只能从默认分支通过
`workflow_dispatch` 提交同一个 `release_tag` 进行恢复。validate 会校验该 tag 为 SemVer、存在、
已包含于默认分支且尚未有 Release；构建与发布始终 checkout 该 tag。恢复 run 可以只在无 Environment
的六平台 test job 中应用固定白名单的默认分支测试覆盖。v0.2.0 已完成的历史恢复使用四条路径：当时的
release workflow、位于 `pixiv/account_external_test.go` 的 Windows ACL 测试，以及 canonical
verifier 与其测试；这是不可改写的历史证据，不定义后续 tag 的 allowlist。

当前 v0.3.0 tag 已包含与默认分支相同的顶层 `pixiv/account_external_test.go`，质量门直接运行 tag 中的
测试，不需要也不能再次 overlay。该 tag 尚未包含拆分后的 release verifier 文件，所以当前恢复
allowlist 必须逐字列出以下路径，不能改成目录或 glob：

- `.github/workflows/release.yml`
- `scripts/internal/workflowpolicy/policy.go`
- `scripts/releaseworkflow/build_policy.go`
- `scripts/releaseworkflow/build_recovery_test.go`
- `scripts/releaseworkflow/homebrew_policy.go`
- `scripts/releaseworkflow/homebrew_policy_test.go`
- `scripts/releaseworkflow/main.go`
- `scripts/releaseworkflow/main_test.go`
- `scripts/releaseworkflow/publish_policy.go`
- `scripts/releaseworkflow/publish_security_test.go`
- `scripts/releaseworkflow/recovery_policy.go`
- `scripts/releaseworkflow/test_helpers_test.go`
- `scripts/releaseworkflow/workflow_policy.go`
- `scripts/releaseworkflow/workflow_policy_test.go`

全部拆出的 release test files 都必须 overlay，才能保留当前 mutation suite。共享 production helper
`scripts/internal/workflowpolicy/policy.go` 是两个 verifier 共用的 YAML policy 实现及唯一的 per-target
Rust toolchain 映射，也是从默认分支编译 release verifier 的必要依赖；它不参与生产资产构建，且共享包
自己的 `policy_test.go` 仍不进入恢复 overlay。提取前必须用
`git status --porcelain=v1 --untracked-files=all` 确认工作树为空，并显式确认 cached
diff 为空；覆盖通过一次 `git archive` 提取后，将 tracked diff 与未忽略的 untracked files 合并、按 C locale
排序，再与上述逐字 allowlist 比较，同时再次确认 cached diff 为空。这使旧 tag 中尚不存在的拆分文件也参与
fail-closed 核对。重新加入 account test、任意额外路径或生产源码都必须失败，test job 也不生成 release
artifact。该 job 成功后，独立的新 runner
才会以 `clean: true` checkout tag、重新构建 selected staticlib 并生成唯一可被 publish 下载的
`verified-release-*` assets；测试进程对环境变量、PATH 或临时目录的副作用不会进入生产 job。因此它不能用于
替换生产资产源码、移动 tag，或重发已经存在的 Release。

恢复 workflow 的定义可以来自受审计的默认分支，但 production worktree 仍只含 tag bytes。为重现
v0.3.0 tag 已提交 staticlib，test 与 production job 从相同六目标 matrix 读取上述 per-target Rust
toolchain，并在执行 tag 自带的构建脚本前精确安装；这属于 runner 构建环境约束，不把 main 文件或新库
覆盖进 production。重建库必须继续与 tag blob 通过 `git diff --exit-code` 的 byte-for-byte 检查；
toolchain pin 不能用来替换 tag staticlib、恢复 manifest、放宽 diff 或移动 tag。

### Verifier 源码导航

`scripts/releaseworkflow/` 按发布职责分卷：`main.go` 负责命令入口、文件读取与顶层 dispatch；
`build_policy.go` 负责 validate、test build 与 production build；`recovery_policy.go` 负责 tag trigger、
恢复覆盖与覆盖后的质量门顺序；`publish_policy.go` 负责 source verification、发布、签名与 channel；
`homebrew_policy.go` 负责 formula render、四平台验证与 tap deploy；`workflow_policy.go` 保存各领域共用的
job、step、command、action 与 permission helper。测试相应分为 `build_recovery_test.go`、
`publish_security_test.go`、`homebrew_policy_test.go` 与 `workflow_policy_test.go`；`main_test.go` 只保留
顶层入口行为，`test_helpers_test.go` 集中无策略含义的 YAML fixture 操作。

`scripts/nativeevidence/` 按 evidence 生命周期分卷：`main.go` 负责 subcommand 与 flag；`models.go` 保存
target 和 evidence schema；`record.go` 记录单 runner evidence；`consolidate.go` 校验并合并六目标结果；
`archive.go` 负责 release archive member 与 JSON；`filesystem.go` 负责路径、hash 和安全文件操作；
`workflow_policy.go` 只验证 native-evidence workflow。测试分别覆盖 policy、record、consolidate，fixture
helper 再按 workflow 与 evidence/archive 分开，避免把策略测试重新堆进单一文件。

`sh scripts/test-release-workflow.sh` 启动 `scripts/releaseworkflow` 的 YAML AST policy，而不是依赖
文本排版或行号。它精确检查 tag trigger、八份 job 的权限/依赖、六个 test/production runner matrix、每一个
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
test job 必须实际运行 vendored Rust 离线检查、crate cwd 的 `cargo fmt --check` 与 locked/offline
Clippy `-D warnings`、普通 Go 测试、vet、许可证、封装、固定版本 `pre-commit==4.6.0`、
pre-commit 和 `git diff --check`；production build job 不运行 overlay，且只从 clean tag tree 生成
`verified-release-*` artifact。发布渠道仅可由
`go run ./scripts/releaseassets channel --version ...` 判定；build metadata 中的连字符不会使 stable
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
`brew install --formula`，解析
`pixiv version --json` 并与 tag 比较。它不使用 workspace formula path、developer/环境变量 bypass，
也不克隆、写入或信任公开 tap。只有全部成功，最终受保护 `deploy_homebrew_tap` 才以 HTTPS
clone public tap、核对唯一 staged formula，并在最后一个 step 读取 deploy key；SSH push 固定官方
GitHub ED25519 known_hosts、启用 strict checking，目标精确为 `HEAD:main`。任何前置 job 失败都不会
写 tap。

这套本地检查只证明 workflow 声明的依赖和语义，**不**验证 GitHub `release` Environment、
secret 和 tag protection 的远端实际状态；它不替代 Task 20 的远端配置审计，也不替代正式 tag
产生的四架构 Homebrew 外部安装证据。由于 draft asset 的匿名 URL 不可被 Homebrew 下载，workflow
会先公开 Release 再安装；若安装失败，Release 已公开但 tap 不变，需要维护者显式处置，不能绕过 gate
手工 push。

正式发布目前仍必须被正式 tag、签名 GitHub Release、tap formula 与后续安装验收阻断。完整
six-target staticlib/manifest 与真实 native artifact 证据已由 run `29192425899` 收集并受控回填；
受保护 `release` Environment、生产 signing 私钥与公开仓库也已按 Task 20 配置，但这些前置条件本身
不等于 Release/tap 已创建或安装路径已验收。

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
的受保护 `release` Environment secret `HOMEBREW_TAP_DEPLOY_KEY`，公开 tap 只登记对应公钥。workflow
在独立 renderer 中生成 staging formula，并在四个原生 runner 验证安装，再由最终 protected job 做
受限提交/push。v0.3.0 的 stable formula 已提交到公开 tap；后续 stable/beta 发布仍不能从本仓库或
workflow artifact 读取、生成或记录 deploy key。

当前 Release 不会进行 Apple notarization 或 Windows Authenticode。直接下载仍可能被 Gatekeeper
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
