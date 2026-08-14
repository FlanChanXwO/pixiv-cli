# v1.0.0 下一轮内部架构重组计划

状态：内部架构重组已实施并完成本地验收（2026-08-08）。本报告只收口包边界、路径、自动门禁与
文档一致性；RC-11 所需的真实 public SDK/solver 外部 evidence 仍按独立发布门禁记录，不因本报告
完成而自动通过。

## 1. 目标与边界

v1.0.0 的 SDK 重写和初始 Phase 0–9 已经完成；本轮处理的是正式版前仍然存在的包边界、命名和维护成本问题。目标是让每个目录表达一个稳定职责，使 CLI、MCP、用例、协议适配、持久化和基础设施之间只有一条可解释的依赖方向。

本轮只做内部重组和必要的窄接口调整，不引入新的 Pixiv/FANBOX 产品能力，不改变公开 SDK 的数据模型和正常成功路径。涉及已有用户可见行为的地方（账号池配置、认证数据迁移、CLI 命令布局）必须同步更新三语 CLI reference、相关 Skill 和 release note。

以下决策在本计划中冻结：

- 运行时配置继续使用 TOML。RC 计划中的 YAML 仅指工作流或工具配置，不迁移产品配置格式。
- `internal/buildinfo` 保持独立，不移入 `update`；它承载 linker metadata，并被 CLI、更新逻辑和脚本共同使用。
- `internal/services` 继续保留，但含义限定为上游协议/外部服务适配层，不承载业务用例。
- 不创建 `internal/release`，也不在 `internal/update` 下创建 `release` 子包；所有发布检查、安装和回滚逻辑仍归 `internal/update`。
- `internal/cli` 是 thin controller。根目录只保留 `root.go`（及根控制器测试）；辅助逻辑不能继续平铺在 CLI 顶层。
- `command.go` 只出现在 CLI 命令包：子目录的 `command.go` 注册同目录命令，顶层 `root.go` 注册这些子目录构造器。`internal/update/command.go` 必须删除并按语义拆分。
- 不引入 DI framework 或全局 service locator；bootstrap 提供显式、可关闭的运行时资源容器。
- 不增加无证据的固定 timeout、重试、截断、静默 fallback 或隐藏降级。

## 2. 目标依赖方向

```text
cmd/pixiv
    -> internal/cli
        -> internal/application/<usecase>
            -> internal/application ports
            -> sdk/pixiv 或 sdk/fanbox
                -> internal/services/<product>/<protocol-adapter>
                    -> Pixiv/FANBOX upstream

internal/bootstrap
    -> 组装 application、SDK、persistence、filesystem、network、MCP

internal/persistence、internal/filesystem、internal/network、internal/browsercookies
    -> 只向更底层标准库或外部库依赖
```

必须由架构测试或 `go list` 检查保证：

- CLI/MCP 不直接导入 `internal/services`、`appapi`、`webapi`、`oauth`、`resource` 或 persistence 实现。
- application 不导入具体 `authdb`、`localstate` 或 CLI 包；它只依赖自己定义的 ports 和公开 SDK。
- bootstrap 可以导入所有实现包，但业务循环不写在 bootstrap。
- downloader 不反向依赖 application 的具体实现；二者通过 `internal/application/download` 的 DTO/port 连接。
- `internal/utils` 不依赖协议、数据库、文件系统、CLI 或应用层。

## 3. 目标目录与职责

目标树如下；文件名是职责提示，不要求一次提交完成所有文件移动。

```text
internal/
  cli/
    root.go                         # 根控制器，只注册子命令
    runtime/                        # CLI 运行期依赖与启动参数 seam
    output/                         # CLI 输出/NDJSON 适配，不放业务规则
    auth/command.go                 # auth 子命令注册；login/export/import/use/status
    config/command.go               # config 子命令注册
    pixiv/command.go                # Pixiv 数据命令注册，不产生 pixiv pixiv 前缀
    download/command.go             # 下载命令注册
    fanbox/command.go               # FANBOX 命令注册
    mcp/command.go                  # MCP 命令注册
    update/command.go               # update 命令注册
    version/command.go              # version 命令注册

  application/
    ports.go                        # 极少量跨用例公共 port
    pixiv/                          # 账号、登录、SDK adapter、账号池、搜索 catalog/date
    fanbox/                         # FANBOX 用例与其 ports
    config/                         # TOML schema/spec、默认值、合并、稀疏写回、配置 service
    download/                       # 下载用例、输入校验、source 展开、报告和 downloader port
    pagination/                     # 通用分页用例/遍历策略

  bootstrap/                        # Runtime、资源组装、依赖注入与关闭顺序
  persistence/
    authdb/                         # SQLite schema、repository、永久 migration
  filesystem/                       # 路径、权限、锁、原子写入与恢复
  network/                          # HTTP client/proxy policy 与可观测错误
  credentials/                      # refresh token/env/凭据校验
  record/                           # CLI NDJSON/MCP 稳定记录契约与映射
  downloader/                       # 资源传输、文件名、并发、ugoira、发布
    staticlib/                      # cgo-free manifest/digest 校验
    ugoira_rs/                      # Rust crate/vendor/staticlib
  ugoira/                           # canonical Rust encoder FFI 与 native tests
  browsercookies/                   # chromium/firefox/safari/core/secret/sqliteio
  mcpserver/
    pixiv/                          # Pixiv tool 注册与输入/输出适配
    fanbox/                         # FANBOX tool 注册与输入/输出适配
  services/
    pixiv/{appapi,model,oauth,protocol,resource}
    fanbox/{...}                    # 只放上游协议适配
  update/                           # 更新协调、版本策略、安装、清理、release client
  buildinfo/                        # linker metadata；不移动
  utils/{parse,text,uri}            # 纯函数、协议无关 helper
```

### 3.1 CLI 命令树

`root.go` 只完成全局 flag、配置/运行时选项解析和子命令挂载。每个命令目录的 `command.go` 只注册同目录的命令构造器；具体 handler 和输入解析与其职责文件同目录。`loginhelper`、`loginpage` 移入 `internal/cli/auth`；文件写入、代理、账号池和 Pixiv 协议逻辑不能通过 CLI 顶层 helper 偷渡。

CLI 子包不能导入父包 `internal/cli`，避免 root-to-child import cycle。共享的启动依赖放入 `internal/cli/runtime`，共享输出适配放入 `internal/cli/output`；领域行为放入对应 command/application 包，不建立新的 `cli/common` 垃圾桶。

### 3.2 Application 与 SDK

删除根 `internal/application` 中按产品或具体设施实现的文件：`account.go`、`account_pool.go`、`account_types.go`、`config.go`、`download.go`、`login.go`、`pagination.go`、`pixiv_client_adapter.go`、`record.go`、`runtime.go`、`sdk.go`、`search_dates.go`。它们分别移动或拆入上面的产品/用例包。

当前 50 余方法的 `application.SDKClient` 同时承担认证、登录、内容、资源和账号池操作，属于 shallow giant interface。按用例拆成窄接口（例如 `AccountRepository`、`LoginClient`、`CatalogClient`、`ResourceClient`、`PooledOperation`），公开 SDK 的 Pixiv/FANBOX entity 仍由 SDK 定义。application 通过 port 接收 persistence 和 downloader，不反向引用其具体实现。

若移除 CLI/application 对 `services/pixiv/protocol` 常量的依赖需要公开 seam，则在 `sdk/pixiv` 增加最小的 `LoginSession.AcceptsCallbackURL(raw string) bool`（或等价命名）方法；不得把协议常量重新暴露到 CLI。该项属于 additive public API，必须补 SDK 文档与测试。

### 3.3 配置、凭据与文件系统

所有配置实现并入 `internal/application/config`：`RuntimeConfig`、TOML schema/spec、默认值、effective merge、稀疏写回、默认账号 ID、`AccountPoolConfig`（仅保留 `enabled` 与 `strategy`）及 `ErrRemovedSetting`。删除顶层 `internal/config`；配置包定义所需的 file-store port，由 bootstrap 注入 filesystem 实现，不保留 bootstrap 专属 `ConfigFileStore`。

refresh-token 环境变量和 token 格式校验移入 `internal/credentials` 或 `application/pixiv`，避免 config 依赖 Pixiv 协议。

`internal/filesystem` 合并原 `utils/files`、`utils/atomicfile` 与 localstate 的路径/权限/锁原语：原子流写入、替换/恢复、私密文件 writer、commit outcome、用户数据目录、跨平台权限和文件锁都在此处。对外提供 `AppDataDir`、`UserDataFile`、`EnsurePrivateDir`、`WritePrivateFile`、`AtomicWrite`、`Replace` 等行为 API；不要把 `0600`/`0700` 等权限常量重新散播到所有调用方。`utils` 只保留可验证的纯函数；`filename`、`ids` 归 downloader，`media` 归 MCP 输出适配，`parallel` 归 downloader，`uri` 只保留纯路径/FileURI helper，代理错误移入 `network`。

### 3.4 Persistence 与迁移

将 `internal/storage` 改名为 `internal/persistence`，根目录只保留 `authdb`。无生产引用的 `storage/accountpool` 删除，不迁移；`storage/localstate` 的通用 JSON/锁实现并入 filesystem。

SQLite 永久 schema 只维护 `pixiv_account`、`fanbox_account`、`schema_migration` 三类表；账号池状态存入 Pixiv 账号行。RC schema 收口包含：Fanbox `creator_id` 非空、Pixiv `schedulable` 字段、`pool_last_selected` 的部分唯一索引，分别以后续 migration 文件完成并可重复执行。

旧 `auth.json` 不再自动读取或隐式迁移。删除 `storage/auth`、`authdb/legacy.go` 及自动迁移调用/测试；发布说明要求用户在旧版本执行 `pixiv auth export --all --output <private bundle>`，再在新版本执行 `pixiv auth import --file <bundle>`。SQLite migration 与此手工兼容流程严格分开，导入失败必须暴露原因。现有 CLI 测试通过 legacy auth package 构造 fixture 的调用点必须改为直接写入 `persistence/authdb` 并设置默认账号配置；同时清理 browsercookies 中指向旧测试 hook 的注释，不能以删除测试代替迁移 fixture。

现有 `account_pool.accounts` 到数据库 `schedulable` 的一次性迁移可以保留，但必须在 DB commit 成功后清理旧 TOML key，过程幂等、可观测，不能保留隐藏 fallback。

### 3.5 Bootstrap、Network 与 Services

`bootstrap.NewRuntime(options) (*Runtime, error)` 返回一次 CLI/MCP 进程使用的 typed resource container，`Runtime.Close() error` 负责逆序关闭且只关闭一次。删除 `NewServices`、`MCPRuntime`、`fanboxDBRegistry`、`CloseServices` 和 bootstrap 中的全局注册表；DB 打开、迁移或 client 创建错误不得被吞掉。

账号池的 `newPooledSDKOperation` 业务循环移入 `application/pixiv`，bootstrap 只注入 repository、client factory 和 clock/策略等依赖。MCP 自己持有 Runtime；CLI 在真正执行 `Run` 时构造一次，帮助、lazy parse 和 secret export 的既有语义不变。

根 `internal/services/pixiv`（仅 doc、drawing catalog、HTTP client 的薄壳）删除。绘图工具 catalog 移到 `application/pixiv`；代理 HTTP client 与 `ErrInvalidProxy` 移到 `internal/network`，由一个显式 proxy factory 统一创建。`internal/update` 接受注入的 `*http.Client`，删除浅层 `NewReleaseHTTPClient` wrapper，不在 update 内自行构造网络依赖。服务适配包只保留 `appapi`、`model`、`oauth`、`protocol`、`resource` 及 FANBOX 对等包。

### 3.6 Downloader、ugoira 与 browser cookies

将实际下载引擎 `internal/download` 改名为 `internal/downloader`；`internal/application/download` 只保留用例和 port。文件命名、ID 规范化、并发、资源传输、ugoira adapter、atomic publish 和 staticlib integrity 都归 downloader。

合并重复 Rust FFI：`internal/ugoira` 成为唯一 encoder/native test 入口，链接到 `internal/downloader/ugoira_rs`；cgo-free 的 manifest/digest 测试归 `internal/downloader/staticlib`。删除旧 `download/ugoira_rust*.go` 壳层前，先迁移 link assertion、native smoke 和 compile gate。同步修正 `build.sh`、staticlib/nativeevidence/release 脚本、`.github/workflows`、license bundle、开发文档中的旧路径。原有 deliberate compile-gate 与 native evidence 必须保留；当前 worktree `.git` 路径损坏导致的 `git check-attr` 测试失败需在合法 worktree 复验，不得当作代码回归。

移除 `internal/platform` wrapper：`platform/browsercookies` 直接成为 `internal/browsercookies`，保留 `core`、浏览器实现、secret 和 sqliteio。localstate 的路径/权限常量归 filesystem/persistence。必须同时把 FANBOX `--from-browser` 从当前 unavailable stub 接到新的 browsercookies provider；只移动包而不恢复真实调用链视为未完成。

### 3.7 Record、MCP 与 update

根 application 的 `record.go` 移到 `internal/record`，作为 CLI NDJSON/MCP 稳定记录契约：负责 SDK entity 到稳定 JSON 的映射、ID/过滤/解析和禁止元数据清理。SDK entity、usecase DTO、DB row 和 wire DTO 不得混用。

将 `internal/mcpserver` 按产品拆成 `pixiv`、`fanbox`；根包只保留 server 组装所需的最薄入口。把语义含糊的 `legacy_tools.go` 拆为按领域命名的文件，所有 tool 输入/输出适配通过 `internal/record` 和 application port 完成；继续保证 stdout 只输出 JSON-RPC，运行期失败保留 structured result 并设置 `isError=true`。

`internal/update` 按语义拆为 `coordinator.go`、`automatic_check.go`、`install_source.go`、`version_policy.go`、`process_runner.go`、`release_client.go`、`release_sources.go`、`release_installer.go`、`pending_cleanup.go`、`cache_permissions_*.go` 等。它不再含 CLI 注册文件；CLI 的 `internal/cli/update/command.go` 只负责命令挂载。`buildinfo` 原位保留。

保留 `sdk/doc.go`、`sdk/pixiv/doc.go`、`sdk/fanbox/doc.go` 作为公开 GoDoc 入口；删除 `internal/services/pixiv/doc.go`，内部职责写在主文件和维护者架构文档中。删除 `internal/testutil/socks5test`，将其两个调用点改为 network transport 配置断言；若仍需要真实 SOCKS handshake 测试，应放在 `internal/network`，不得恢复通用 testutil。

## 4. 实施顺序与交付物

每个阶段都必须保持可编译的小提交，并在阶段结束更新 import/路径检查；不得先大规模移动再用全局替换掩盖边界问题。

1. **基线与保护测试**：记录 `go list` 导入图、公开 SDK/CLI 行为、TOML precedence、MCP stdout、native evidence；先加入禁止旧依赖方向的架构测试。
2. **基础设施先行**：建立 `filesystem`、`network`、`credentials`、`record`，迁移纯 helper 和测试，移除 `platform`/`utils` 的重基础设施职责。
3. **Persistence 与配置**：改名为 `persistence/authdb`，完成 schema migration、手工 auth bundle 迁移协议和 `application/config`；验证 DB 错误传播及配置写回。
4. **Application ports**：按产品/用例拆分 application，消除具体 authdb/services 依赖，拆掉 giant SDK interface，迁移 account-pool executor。
5. **SDK seam 与服务边界**：删除 services 根包；必要时增加 SDK callback seam；将 drawing catalog、proxy factory 和 release HTTP client 调用方迁移到正确层。
6. **Downloader/native**：完成 `download`→`downloader`、ugoira FFI 收敛、staticlib/native workflow 路径更新和真实 native/compile gate 回归。
7. **CLI/MCP 树**：按 `root.go`→子目录 `command.go` 注册模型拆分 CLI；移动 auth helper；按 Pixiv/FANBOX 拆 MCP；确认无循环依赖。
8. **Update 与清理**：拆 update 语义文件，删除 update command.go、testutil、旧 storage/platform/services 根包和空目录；确认 buildinfo 不变。
9. **文档与发布门禁**：更新架构/上下文/ADR、开发文档、脚本路径和计划索引；若认证手工迁移或账号池配置造成用户可见变化，更新三语 CLI reference、Skill、`changelog/unreleased`/release note。

## 5. 接口、兼容性与安全约束

- 新增的内部 port 必须由调用方定义，接口只包含一个用例所需的方法；实现可以在 persistence、SDK adapter 或 downloader 中。
- Runtime、文件写入、迁移和安装均返回明确的 commit/close/error outcome；不得 `_, _ =` 忽略迁移、DB open 或 close 错误。
- proxy、browser cookie、token 和下载文件的路径/权限沿用现有安全契约；不把 secret 写入日志、普通输出、JSON 或 MCP。
- 配置只读 TOML，保持现有 precedence（默认值 < 文件 < 环境/显式运行参数）和 sparse writer 语义；移除设置必须返回 `ErrRemovedSetting`。
- 公开 SDK 仍是 CLI/MCP 获取 Pixiv 能力的唯一公共契约；不得为了减少移动工作而让调用方直连 internal protocol。
- 纯内部重构可以在 release note 标记 `None`，但手工认证迁移、旧 key 移除或命令语义变化必须记录为 breaking change，并提供迁移命令示例。

## 6. 验收清单

### 代码与测试

- `go test ./... -count=1`、`sh scripts/build.sh`、`go test ./scripts/documentation -count=1` 通过。
- 所有 package 的 `gopls check` 诊断已读；现有 `domain_cmd.go` style 诊断和 ugoira compile-gate 需分别标注为既有/有意门禁，不能用无关改动掩盖。
- 架构测试确认 CLI/MCP/application 的禁止导入、唯一 `command.go` 规则、无旧 `platform/storage/download` 路径和无 `internal/services/pixiv` 根导入。
- Runtime close 顺序、DB migration/error propagation、TOML sparse write、manual auth import/export、account-pool one-time migration、proxy factory、browser-cookie FANBOX route、record 输出和 MCP `isError` 均有聚焦测试。
- downloader/staticlib 的 cgo-free 测试、ugoira native smoke/link test、vendor/license/release evidence 在更新后的路径运行；worktree 元数据异常须在有效 Git worktree 复验。

### 文档与路径

- `docs/maintainers/architecture.md`、`CONTEXT.md`、相关 ADR、`docs/maintainers/development.md` 与本目录索引反映最终边界。
- 脚本、workflow、`.github/copilot-instructions.md`、native evidence 和 license bundle 不再引用旧目录。
- CLI/MCP/config/auth migration 的三语契约、产品 Skill 和 release note 与实现完全一致；没有把 YAML 当作运行时配置写入用户文档。
- `git diff --check` 通过，新增文件纳入 Git 跟踪；不提交 token、数据库、缓存、下载内容或机器配置。

## 7. 完成定义

当上述依赖方向、目录职责、迁移策略、测试和文档门禁全部满足，并且 `go test ./...` 与构建在合法 worktree 通过时，本轮架构重组才算完成。仅完成目录移动、保留旧 import alias、或通过静默 fallback 维持旧行为，均不算完成。

## 8. 本地验收记录（2026-08-08）

- `gopls` workspace diagnostics 已在最后一次代码修改后读取；最后修改的 Go 文件无
  compiler/type/unused diagnostics；原 `domain_cmd.go` 的 `fmt` style 建议已随文件迁移一并修复。
- `GOPROXY=off go test ./... -count=1`、`GOPROXY=off go test -race ./... -count=1`、
  `GOPROXY=off go vet ./...`、`sh scripts/build.sh`、`GOPROXY=off go test ./scripts/documentation -count=1`、
  `GOPROXY=off pre-commit run --all-files` 与 `git diff --check` 通过。
- 架构测试确认旧目录/壳层、CLI 根目录平铺实现、CLI child-to-root import、application 根实现、
  MCP 模糊 tool 文件和非 CLI `command.go` 均不存在；Runtime nil 初始化、配置 file-store 缺失、
  authdb migration/CAS、账号池迁移、manual auth bundle、proxy/browser route、record/MCP error
  契约均有聚焦覆盖。
- `GOPROXY=off go test ./internal/ugoira -run '^TestRustUgoiraEncoderNativeGIFAndAPNG$' -count=1 -v`、
  downloader source-digest/manifest tests、`go run ./scripts/nativeevidence policy --workflow
  .github/workflows/native-evidence.yml`、`go run ./scripts/browsernativeevidence policy --workflow
  .github/workflows/browser-evidence.yml`、`sh scripts/test-release-workflow.sh` 和
  `go run ./scripts/licensebundle --check` 均通过。`CGO_ENABLED=0 go test ./internal/ugoira` 的
  编译失败是保留的 deliberate Go 1.26.3 + cgo + Rust staticlib + target linker gate。
- `internal/browsercookies` 已补齐 macOS Keychain、Windows 当前用户 DPAPI、Linux Secret Service
  lookup、三平台 Chromium/Firefox profile root 与 v10/v11 GCM/legacy CBC 解密；合成 profile/key
  与 Safari 多页解析测试通过。对应 Windows/Linux 实机、Firefox 临时 profile 和 macOS Safari 的
  native provider evidence 仍按 RC-11 单独执行，不能以本机测试代替。
- 上述本地验证在合法 Git worktree `codex/v1-sdk-rewrite` 完成。当时记录的「本地架构门禁」
  指 `internal/architecture` 的全仓结构快照检查，该 gate 此后被整体删除（它本身不读取
  credential）。其 15 条规则的替代保障与删除理由见 `goal-4/architecture-replacement-matrix.md`；
  现由 owner 行为测试、Go 编译约束、public API golden 与 review checklist 承担。
  2026-08-08 获授权后的真实 Pixiv/solver evidence 与仍待完成的 FANBOX/native browser evidence
  统一记录在 [最终验证记录](final-verification-2026-08-08.md)。
