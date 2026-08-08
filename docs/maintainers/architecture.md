# 架构说明

## 总体流程

`cmd/pixiv/main.go` 是唯一官方二进制入口，它只负责调用 `internal/cli`：

1. `pixiv` 无参数显示 CLI 帮助。
2. `pixiv auth/config/version/update/search/timeline/detail/ranking/recommended/user/bookmark/follow/download` 进入 CLI 模式；`pixiv fanbox` 进入 FANBOX 模式；`auth import` 负责 direct token import 或 bundle restore，`auth export` 负责本地 secret snapshot。
3. `pixiv mcp` 与 `pixiv fanbox mcp` 委托 `internal/bootstrap` 组装并运行各自独立的 MCP stdio server。
4. CLI 与 MCP 通过 `internal/bootstrap` 共享生产 wiring：
   - 账号凭据来自 `~/.pixiv-cli/pixiv-cli.db`（SQLite，`internal/persistence/authdb`；Windows：`%USERPROFILE%\.pixiv-cli\pixiv-cli.db`）；旧 `auth.json` 不自动读取，用户须显式导出/导入 bundle
   - 全局配置来自 `~/.pixiv-cli/config.toml`（Windows：`%USERPROFILE%\.pixiv-cli\config.toml`）
   - 公开环境变量作为覆盖层参与合并
5. 没有匿名 Web fallback：所有内容命令都要求认证态本地账号或显式凭据；已删除的 `web_fallback_enabled` 若仍显式存在则返回 `removed_setting`。

## 包职责

### `cmd/pixiv`

负责生成 `pixiv` binary 的 `main` package。它不承载业务逻辑，只委托 `internal/cli.Run` 并返回进程退出码。

### `internal/cli`

负责 CLI 用户态的命令分发与输出：

- Cobra 命令树、help 和 flag 解析。
- 文本/JSON 输出。
- `auth import [REFRESH_TOKEN]` 的输入 adapter：位置参数直接作为 opaque token；无参 TTY 隐藏输入，非 TTY 读取 raw stdin；`--file PATH|-` 解码并离线恢复 bundle。
- `auth export` 的 secret-output adapter：不带 `--output` 时，默认/UID 选择只输出 raw token 与换行，`--all` 只输出 versioned bundle；`--output` 改为私有文件并只输出无 secret 摘要。
- CLI 协议的 `--page`/`--limit` 解析与错误文案；解析后的逻辑分页计划交给 application 共享遍历引擎。
- `auth login` 的 loopback OAuth、浏览器打开和 TTY 交互。
- `pixiv mcp` 分发。
- `pixiv version`、根 `--version` 与 `pixiv update` 的输入/输出适配。
- 普通 CLI 成功命令后的只读自动更新提示；提示和失败 warning 仅写 stderr。

它不直接拥有账号存储变更、Pixiv client 构造或下载管理器构造；这些职责由 `internal/application` 与 `internal/bootstrap` 承接。

命令树由 `root.go` 统一处理全局 flag、lazy runtime 与退出码，再交给
`internal/cli/{auth,config,pixiv,download,fanbox,mcp,update,version}/command.go` 注册各领域命令。
共享启动 factory 位于 `internal/cli/runtime`，通用 JSON fallback 位于 `internal/cli/output`；这些子包不反向导入
`internal/cli`，根包只通过小的 host seam 挂载既有 handler。

`version` 的 JSON stdout 精确为 `version`、`commit`、`build_date`。自动更新只在普通业务命令
成功后运行，跳过 MCP、help、`version`、`update`、全部 `auth export`、`auth import --file` 和开发构建；它选择 stable Release、使用用户
cache 的 24 小时节流，并最多等待 3 秒。配置、网络、来源识别失败只作为 stderr warning，不能
改变已成功业务命令的退出码，也不能混入 JSON stdout 或 MCP JSON-RPC。
进程启动时的 Windows pending-update cleanup 也属于潜在 mutation；全部 `auth export` invocation 在 Cobra
解析前即识别并跳过该 cleanup，其他命令仍沿用正常 startup cleanup。root bool flag 的重复覆盖语义由
聚焦测试保护，不能让 `--help=false` 等写法误绕开 secret export 边界。

### `internal/cli/auth/loginhelper`

负责 `auth login` 的系统 URL scheme helper、持久 handler manifest、一次性 remote handoff 私有状态与 remote callback client。
`internal/cli` 只经该包安装按需 handler，保留 OAuth、loopback HTTP、系统浏览器、TTY 和 relay server 编排。handler
只允许精确的 `pixiv://account/login` 与 `pixiv://account/remote-login` 进入本轮操作；活跃 loopback 优先，远程 callback
只投递给活跃的一次性 handoff，其他 `pixiv://` URL 定向给 manifest 保存的旧 handler。desktop private state 只保存当前
handoff 的 relay origin、会话 ID 与 capability；server 不保存 desktop 设备记录，public SDK 不暴露这些状态。remote callback
只接受同一 relay base 的一次性 result URL，`internal/cli` 打开无敏感的最终页后等待 OAuth exchange。
Darwin 独立持有嵌入 Swift、`Info.plist`、LaunchServices；Windows 使用当前用户 registry/class 启动；desktop Linux 使用
XDG desktop entry 与 `gio`。headless Linux 不注册 handler，但可运行 relay server。

### `internal/buildinfo`

保存由 Go linker 注入的 `Version`、`Commit`、`BuildDate`。本地默认是 `dev`/`unknown`/`unknown`；
只有 version 为 `dev` 的构建被视为开发构建，并必须拒绝自更新。

### `internal/application`

负责 CLI/MCP 之外的应用用例编排：

- `AccountService`：账号 import/list/export/remove/use/check；bundle export/restore 只经 public SDK 读取或写入本地 store，direct token import 仍经 OAuth 验证并保存 rotation 后的 token。
- `ConfigService`：`config.toml` path/get/set/unset。
- `LoginService`：生成 PKCE/state、authorization-code exchange，并保存账号；Pixiv 登录 URL 构造仍留在 CLI adapter。
- `SDKService`：为 CLI/MCP 打开 `sdk/pixiv` 与 `sdk/fanbox` client，并把调用方选择的账号/代理/JSON 设置映射到 SDK operation snapshot；作品查询和下载均从该 snapshot 的 public SDK 能力继续执行。
- `DownloadService`：把同一 operation snapshot、本次下载路径和文件名模板交给 bootstrap 注入的窄 factory，并委托下载；应用层不构造具体 manager。
- 分页遍历：统一负责 opaque cursor 跟随、逻辑 skip/limit、单批兼容语义与重复 cursor 止环；CLI 可按批流式消费，MCP 可经同一引擎收集结果。各 adapter 只解析各自协议输入并组织输出。

实现按用例拆在 `application/{pixiv,fanbox,config,download,pagination}`；根包只保留跨用例
`ports.go`。Pixiv application ports 按认证、作品、小说、用户、mutation、resource 与账号池分域，
bootstrap 通过 repository、proxy factory 和 SDK factory 注入具体设施。

### `internal/diagnostics`

提供显式、内存内的 typed diagnostics scope。CLI root、Pixiv MCP、FANBOX MCP、Pixiv/FANBOX
network transport、账号池、下载与 FlareSolverr 只通过 scope 发出允许的模块、operation、route、status、
proxy、UA、request ID、reason 和计数等字段；默认使用 Nop sink。CLI 的 `--debug` 将安全事件实时写入
stderr，MCP request scope 只影响诊断，不改变 JSON-RPC stdout。该包不创建日志文件、不保存 response body、
Cookie、token、signed query 或 arbitrary error dump；公共 SDK 在没有显式 scope 时保持静默。

### `internal/bootstrap`

生产 composition root，负责把 `internal/application/config`、`internal/persistence/authdb`、`sdk/pixiv`、`sdk/fanbox`、`internal/downloader`、`internal/mcpserver/{pixiv,fanbox}`、更新 release client/installer 和 application services 组装起来。测试可以替换 service 里的小接口或 factory，不需要复制生产 wiring。

`NewUpdateCoordinator` 通过 `productionReleaseInstallerOptions` 为 Release installer 注入随受支持
binary 提交的 Ed25519 key ID→public key 映射，并在每次组装时复制 map 与 key bytes，避免调用方污染
production trust root。该公开 key 的 SPKI fingerprint 与已知签名 fixture 由同包测试验证；私钥不在
bootstrap、源码或运行时配置中。只读更新检查不需要该 key；当前 v0.3.0 已是可安装的公开 Release，
但该 wiring 本身仍不能代替每个版本独立的发布验收。

### `internal/persistence/authdb`

负责本地账号状态的 SQLite authority：

- `pixiv-cli.db` 的 schema、migration ledger 与 repository；账号 identity/credential 以 `user_id`
  为 key，Pixiv rotation 与 FANBOX session replacement 使用 `credential_revision` compare-and-swap。
- SQLite migration 只维护永久 schema；旧 `auth.json` 不属于启动迁移路径。
- 平台对应的私有 DB/journal 文件权限（Unix-like `0700`/`0600`）。
- auth export 只读取默认、精确 UID 或全部本地账号；不读取环境 token、不刷新、不联网、不修改状态。

旧 `auth.json` 不再作为 store 或 fixture API。用户须在旧版本显式执行
`pixiv auth export --all --output <private bundle>`，再在新版本执行
`pixiv auth import --file <bundle>`；失败原因直接返回，不自动删除旧文件。

### `internal/application/config`

负责 `config.toml` 及运行时配置：

- `config.toml` schema、默认值和配置键定义。
- 运行时配置合并：`config.toml` 与公开环境变量。
- `pixiv config path/get/set/unset` 需要的强类型解析与稀疏写回。

配置拆分如下：

- `pixiv-cli.db`：保存账号 identity 与 credential（`pixiv_account`/`fanbox_account`），DB 文件权限为 `0600`；旧 `auth.json` 不自动读取。
- `config.toml`：只保存用户显式设置过的全局配置键，包括 `[pixiv.auth].default_user_id` 与 `[fanbox.auth].default_user_id`；Unix-like 文件权限为 `0600`。未设置默认账号时按 `sort_order` 选首个账号。

运行时设置使用 `koanf` 合并 `config.toml` 与公开环境变量；`config set/unset` 使用 `tomledit` 写回，尽量保留注释、顺序和布局。

`application/config` 定义 file-store port，由 bootstrap 注入 `internal/filesystem` 的原子写入协议：于目标同目录使用不含
凭据内容的随机文件名创建临时文件，完成全部写入并执行 file `Sync`，关闭文件后才替换目标。Unix-like 平台
主动把父目录与文件分别收紧为 `0700`、`0600`，原子替换后继续同步目标目录；
若本次调用新建了一层或多层目录，则在替换提交后按 leaf→root 顺序同步目标目录及每个新目录
的外层 parent，使文件 entry 与新目录 entry 均进入 durability 边界；既有目录仍只同步目标目录。
任一目录同步失败时仍会尝试其余目录并合并错误，替换已经提交，调用方不能假定旧文件仍在。替换前的写入、file `Sync` 或关闭
失败会保持旧目标；普通替换失败以及可恢复的部分完成失败也会保持或恢复旧目标，并清理临时
文件。若部分完成后的恢复本身失败，调用方会收到组合错误，目标路径可能暂时缺失，但旧内容的
同目录 recovery backup 与新内容的 source temp 都会保留供人工恢复；此时“No temp residual”不适用。
其他临时文件清理失败不会被吞掉，而会与主错误一并返回。

Windows 在替换前同样执行 file `Sync` 与关闭：目标存在时，使用带同目录唯一 recovery backup 的
`ReplaceFileW`；首次创建使用不覆盖目标的 `MoveFileEx`。`ERROR_UNABLE_TO_MOVE_REPLACEMENT`
保持 target/source 原名；`ERROR_UNABLE_TO_MOVE_REPLACEMENT_2` 会尝试把已移动到 backup 的旧目标
恢复，恢复失败则保留 backup/source。成功替换后的 backup 清理失败属于已提交错误，仍按已提交
路径处理。Windows 首次创建的文件继承父目录 ACL，替换既有目标时保留该目标 ACL；本协议不会
主动添加或放宽 ACL，但也不声称 `Mkdir`/`Chmod` 会收紧 DACL，亦不提供 POSIX mode 或 directory
fsync 的等价保证。

`update_check_enabled` 对应 `[update] check_enabled`，默认 `true`；它只控制普通 CLI 成功后的
自动检查，不禁用用户显式执行的 `pixiv update`。

### `internal/browsercookies`

负责只读的浏览器 cookie provider，不属于公开 SDK，也不依赖 FANBOX、Pixiv、CLI、MCP 或账号 store。
`core` 只接受固定的 `CookieQuery`，按安全 profile identifier 发现浏览器目录，并以脱敏 `Secret`
返回目标值；`chromium` 支持显式的 Chrome/Edge provider，`firefox` 解析 `profiles.ini`，`safari`
只在 macOS 解析 `Cookies.binarycookies`。未知的 Chromium derivative 不做模糊识别，多个 profile 未指定
时明确失败。

平台秘密边界保持在 `secret` 子包：macOS 走 Keychain，Windows 走当前用户 DPAPI，Linux 通过固定属性的
Secret Service `secret-tool` 查询。Chromium provider 按平台 profile 根目录读取 Local State/Cookies，
支持 v10/v11 GCM 与 legacy CBC；系统工具缺失、权限、锁、schema drift 和解密格式错误都映射为稳定错误，
不把 cookie、密钥、绝对路径或命令输出写入日志、错误、JSON 或 MCP。跨平台 native provider evidence
仍须按 v1.0.0 release-prep runbook 在对应 host 执行，离线 fixture 和交叉编译只证明代码/格式契约。

### `internal/update`

负责安装来源识别、GitHub Releases 查询、SemVer 选择、cache、显式更新策略与 Release binary
安装协议：

- Homebrew 通过 executable symlink 与 keg `INSTALL_RECEIPT.json` 区分 stable/beta；两个
  formula 的转换会先卸载冲突 formula，失败时显式尝试恢复原 formula。
- `go install` 需同时匹配 Go build info 与实际 `GOBIN`/`GOPATH/bin` executable；更新总是使用
  选中 Release 的精确 tag。
- 其他正式二进制视为 Release 安装。安装前需选中本平台 archive、验证 Ed25519 签名的
  `checksums.json`、验证 archive SHA-256、解包后运行 `pixiv version --json` 预检，再以同目录
  原子替换。
- GitHub Releases API 是唯一查询后端；draft 被排除，stable 检查不纳入 prerelease。ETag/cache
  用于节流与原子保存。
- 更新选择器对当前检查通道的 published Release 强制 canonical SemVer；任一 tag 不合法时
  fail-closed 并报告该 tag，不会跳过它而选择较旧版本。stable 选择会在校验前先排除
  GitHub 已标记的 prerelease；完整信任边界见 [ADR 0008](adr/0008-ed25519-signed-multi-channel-release-trust.md)。

该包不得把签名、checksum、HTTP、archive、替换或权限错误伪装成“无更新”。production trusted key、
签名私钥与 Keychain 恢复副本、受保护 `release` Environment 和公开 remote 已按 Task 20 配置；完整六目标
native evidence 与 staticlib manifest 已回填。v0.3.0 已完成正式 tag、受签名 Release 与 stable tap formula；
Release 安装的失败语义仍是保护边界，而不是临时降级。

### `sdk`、`sdk/pixiv`、`sdk/fanbox`

公开 SDK 是唯一对外契约面，只从这三个 package 导出：

- `sdk`：共享的 `Page[T]`、`Cursor`（Text/JSON codec）、`Error`（sentinel、context chain、脱敏）、`ResourceRef`/`Resource` 与资源 request/response/save 类型。
- `sdk/pixiv`：Pixiv App-only SDK。`Open/OpenWith/New/NewWith` 构造器、OAuth `LoginSession`、credentials rotation、规范化模型、opaque cursor、`ParseURL` 与资源读取。没有匿名 Web 路径。
- `sdk/fanbox`：FANBOX SDK。`Client.ValidateSession`、creator/tag/post/home/supporting、两类 pagination 与资源读取；不读取浏览器、DB 或 Pixiv credentials，也不 import `sdk/pixiv`。

FANBOX native transport 使用 Firefox 148 profile，并只在构造时接收显式的 HTTP client、proxy、UA 与可选
FlareSolverr options。`FANBOXSESSID` 只允许在 `api.fanbox.cc` 与 `downloads.fanbox.cc` 的受校验请求中
传播；第三方 CDN、Pixiv host 与 solver control request 均不携带该 Cookie。solver control transport
直连 FlareSolverr，solver upstream proxy 只作为 solver 配置传入，不继承 native 或宿主环境 proxy；
challenge 之外的 API/资源错误不自动进入 solver。

旧顶层 `pixiv/` facade 已在 v1 删除；`pixiv` 作为 import alias 保留给 `sdk/pixiv`。认证备份（`AuthExportSelection`/bundle codec）仍经 `sdk/pixiv` 的同一 local snapshot 边界。bundle 含 opaque refresh-token secret，是未加密的 point-in-time backup，不是 live sync；token rotation 后旧 bundle 或其他机器上的副本可能 stale。

调用方在自身 adapter 中定义 source mode、budget、filter、cursor 持久化和 HTTP presentation。本仓库不提供 HTTP Provider、Discover、Probe、Capabilities、RSS 或 crawler。

### `internal/services/pixiv/protocol`、`appapi`、`oauth`、`resource`

内部协议包只由公开 SDK 组合：

- `protocol`：上游 base、profile header、endpoint catalog 与脱敏 adapter failure 的唯一来源；不读配置、不发请求，也不保存响应 body、URL、header 或凭据。
- `appapi`：有凭据的 App content API 与 raw DTO/mapper；幂等 JSON 读取只会在首次 429 的有效 `Retry-After` 后按 context 重试一次。
- `oauth`：PKCE、code exchange、refresh 与 token state。
- `resource`：受 policy 约束的 resource transport、redirect/header/body 边界。

v1 已删除 `internal/services/pixiv/webapi` 与匿名 Web/AJAX 路径：App API 出错直接返回规范化错误，不自动切换协议。搜索的分辨率、横纵比、绘图工具、作品类型与屏蔽 AI 在 `appapi` adapter 翻译成上游参数，分级和仅 AI 则由公开 SDK 基于规范化字段筛选。R18/R18G/mature 与动态搜索选项返回认证需求，不伪造空结果。`internal/services/fanbox` 是 FANBOX 协议 adapter，只由 `sdk/fanbox` 组合。

### `internal/services/pixiv/model`

集中 Pixiv response/domain 类型以及 Pixiv 协议枚举 typed const，例如 search target、sort、ranking mode、restrict 和 illust type。MCP delivery 等传输层常量仍留在 `internal/mcpserver`。

### `internal/mcpserver`

负责将 Pixiv 与下载能力注册为 MCP tools。所有 Pixiv 内容、认证、资源和写操作都通过 `SDKService` 使用 public SDK；旧构造器保留的首个 API 参数只是废弃占位，生产路径不会读取。下载由 operation snapshot 对应的 `DownloadManager` 执行。MCP 的 nullable `page`/`limit` 只在本 adapter 解析，逻辑分页遍历由 application 共享引擎执行；旧 offset wire 字段已移除。stdio runtime 由 `internal/bootstrap` 组装和启动。

包内按产品拆为 `internal/mcpserver/pixiv` 与 `internal/mcpserver/fanbox`，根包只保留构造转发；Pixiv 子包按 `server.go`、`registration.go`、`read_tools.go`、`download_tools.go`、`record_filters.go`、`sdk_runtime.go`、`sdk_tools.go` 与 `timeline_tools.go` 分工。MCP 不自行实现表达式、重试、归档或文件模板：它在打开 SDK operation 前编译输入，再把下载适配交给 application 下载用例；公开 SDK 不承担批量下载语义。运行期 handler 的失败结果保留其 structured output 并使用 `isError=true`；正常空结果不会伪装成失败。完整 wire 语义见 [MCP 工具](../zh-CN/mcp-tools.md#错误与分页)。

### `internal/downloader`

负责下载和本地文件落盘：

面向 CLI/MCP 的下载编排由 `internal/application`（`DownloadClient` 最小能力接口）与 `internal/downloader.Manager` 统一持有：来源展开、作品筛选、SQLite archive、目录/文件模板、sidecar、开放页选择、资源重试、进度事件和 ugoira 模式均不能在 adapter 中复制。公开 SDK 只保留单资源 `OpenResource`/`SaveResource`，不再暴露批量下载。

- `Download` 会同步下载 ID 列表，并返回每个作品的实际产物路径。
- 单页作品保存到下载目录。
- 多页作品和 ugoira 会建立作品子目录。
- 单页与多页作品从上游 URL path 推导扩展名，并与模板生成的文件名一样规范化跨平台非法字符；扩展名还会替换 ASCII 控制字符并移除 Windows 非法尾随点/空格，但不猜测或静默替换扩展名。
- ugoira 先下载 SDK 验证的 `download_url` zip，再由 Rust FFI encoder 合成为 GIF/APNG，或由 public SDK 原样发布 ZIP/提取 frames；认证态可合法选择 App medium，绝不把它标记成 original。

Rust crate 以 target 专用 staticlib 接入 cgo：darwin/linux/windows 各有 amd64/arm64 selector；Linux
selector 还显式链接系统 `libm`，承接 Rust/image staticlib 的 `sinf`/`expf` 符号；Windows selector 以
`-L${SRCDIR}/… -lugoira_rs` 与 Rust std 的 Windows import libraries 传递 `*-pc-windows-msvc` library，
避免 cgo 拒绝带驱动器号的裸 `.lib` 路径；native-evidence 在 Windows 的链接步骤明确使用 runner 提供的
`clang -fuse-ld=lld`，避免 Go 的 GCC-only debug linker script 交给 MSVC linker。受支持
的 release/source build 必须从同一 Rust source digest 的六目标 `manifest.json` 选择并链接真实库；
无 cgo、无 target library 或无 C linker 时应在编译/构建期明确失败，不能回退到 `ffmpeg` 或 runtime
stub。Source identity 纳入 first-party crate 的 Cargo/build/source 输入、vendor 闭包与本地
`quantette`；这些路径在 `.gitattributes` 中禁用 Git 文本转换，使 Windows 与 Unix checkout 保持同一
Git blob bytes，而不是在摘要算法中掩盖差异。摘要算法先把 `filepath.Rel` 的 Windows 反斜杠转换为
logical slash path，再筛选 `src/`、`.cargo/` 与 `vendor/`；run `29189725013` 证明若顺序相反，即使
六个 runner job 均 success，也会产生 Unix/Windows digest split 并被 consolidation fail-closed。
该 run 不可回填或与后续 run 拼接。Rust `target/` 是机器产物，staticlib/manifest 是经过 native
验证后才可提交的发布输入。
release archive 的 `LICENSE` 与生成的许可证 bundle 同样固定 LF checkout；run `29191200569` 的六份
source digest 已一致，但 Windows archive 的 `LICENSE` bytes 仍与 Unix/Git blob 分裂，因此
consolidation 再次 fail-closed。该 run 也不可回填或跨 run 拼接，必须从修复后的新 SHA 完整重跑。
`internal/downloader/staticlib` 只承载 source digest 与 manifest 的完整性契约，不导入 cgo encoder；
因此 native-evidence 的 **policy** gate 可以在目标库生成前执行。`record` 与 `consolidate` 同样不
触发 cgo 链接，但分别仍需要已生成的 staticlib/binary/archive 与完整六份 evidence 输入；下载运行时
仍只由 `internal/ugoira` 通过唯一 FFI 入口链接 `internal/downloader/ugoira_rs`。

run `29192425899` 已在六个平台完成 native build、真实 cgo GIF/APNG smoke、binary/archive record，
并经本地 fail-closed consolidation 回填六个 target library 与统一 manifest；source build 会在链接前
校验该 manifest 和库哈希。`ffmpeg` 仅保留给显式启用的开发质量对照，不在生产下载路径中。

## Release assets 与信任边界

`scripts/releaseassets` 以固定六目标封装 archive：darwin/linux 为 `.tar.gz`，Windows 为 `.zip`；
每个 archive 包含一个 `pixiv`/`pixiv.exe`、`LICENSE`、`THIRD_PARTY_LICENSES.md` 与完整
`third_party/licenses`；它们在 Git 中固定 LF，以便 licensebundle 在 Windows 也可按字节校验。
Windows Git Bash 缺少 `zip` 时，打包脚本明确使用 runner 预装的 `7z`；其他平台使用 `zip`。两条分支
生成相同的 member 集合，缺少对应归档器会直接失败而不会产出不完整 asset。
finalize 阶段收集这六个 archive 的 SHA-256 到 `checksums.txt`，并为原始
checksum bytes 生成带 key ID 的 Ed25519 `checksums.json`。

`.github/workflows/release.yml` 将签名/发布放在受保护的 `release` Environment 中；它使用最小权限和
full-SHA Actions，并在草稿 Release 上传后核对 asset 集合才发布。发布 job 将同一份已验证
`release/checksums.txt` 交给下游 renderer；stable/prerelease 分别生成唯一的 `pixiv-cli`/
`pixiv-cli-beta` formula。四个原生 macOS/Linux runner 将该 formula 安放在各自隔离的 local staging
tap 后，以 tap-qualified formula name 真实安装并核对 `pixiv version --json`；此路径不使用或写入
公开 tap，Homebrew 6 所需的 trust 仅精确写入 runner 本地 staging tap 的临时 trust store。最终受保护
job 才能读取独立 tap deploy key 并只 push 对应 formula。
v0.3.0 的正式 tag 已走完此发布路径并推送 stable formula；后续 tag 仍必须独立满足同一安装 gate。完整
六目标 native 成功证据已回填 staticlib/manifest。production signing 私钥、Environment 与公开 remote
已按 Task 20 配置，受支持 binary 的公开 trust root 已在 `internal/bootstrap/release_trust.go` 配置。Rust crates.io 依赖已由
crate 内 source replacement 固定到完整 vendor 闭包，并以空 Cargo cache 离线 metadata/build/test 与六
target 许可证检查验证。

Homebrew formula 模板由已验证的六目标 `checksums.txt` 生成，仅使用 macOS/Linux asset；stable
`pixiv-cli` 与 beta `pixiv-cli-beta` 相互冲突且同装 `pixiv`。tap credential 与发布 key 是不同的
信任域：tap 私钥只允许进入最终受保护 deploy job 的最后 push step，不能代替 Release Ed25519 trust
root。公开 tap 的 stable formula 已对应 v0.3.0；beta formula 仍只由 pre-release 通道写入。由于 draft
Release 的匿名 URL 不可安装，Release 会先公开再执行四架构 gate；失败会保留已公开 Release 供处置，
但不会写对应 formula。

当前 Release archive 不计划包含 Apple notarization 或 Windows Authenticode。用户收到 Gatekeeper 或
SmartScreen 提示时，必须回到已验证的项目 GitHub Release、checksum 和签名记录，不能把系统提示视为
可由 CLI 静默绕过的错误。

### `internal/filesystem`

不保留通用 constants 包。本地私有目录/文件权限与 `AppDataDirName` 归 `internal/filesystem`。Pixiv 协议值、MCP delivery 值、config key/default 等仍留在所属领域包。项目不维护 operation 日志或诊断元数据包；错误直接由 CLI、MCP 或 public SDK 的既有接口传递。

### `internal/utils`

只保留无协议语义的纯 helper：`parse` 负责正整数解析，`text` 负责字符串默认值，`uri`
负责 URL path 提取与 file URI 生成。

文件名清理、模板展开、ID 去重归 `internal/downloader/{filename,ids}`；稳定记录的媒体类型归
`internal/record/media`；refresh-token 校验归 `internal/credentials`；路径、权限、原子写入和
secret export writer 归 `internal/filesystem`。这样 `internal/utils` 不再承担具体设施或产品领域职责。

## 已知约束

- `appapi`、`oauth` 与 resource transport 使用 caller/SDK 注入的 HTTP client；默认 client 专用于当前 SDK client、无整请求固定 timeout，取消与 deadline 由 context 传播。显式 client 保持调用方策略；详见 [ADR 0010](adr/0010-http-client-timeout-and-context.md)。
- `pixiv mcp` 与 `pixiv fanbox mcp` 是两个独立 MCP stdio server 的显式启动方式；直接执行 `pixiv` 不会启动 MCP。
- 不新增持久账号 import/export MCP tool；既有 session-scoped MCP 认证 tool 与 wire contract 不变。
- 账号 credential 保存在 SQLite `pixiv-cli.db`（BLOB，非加密）；Unix-like DB/journal 文件权限为 `0600`。获得当前用户文件访问权的攻击者仍能读取 credential，自动 backup 被禁止。
- `config.toml` 采用稀疏写入，不会把默认值整份落盘。
- `download_random_from_recommendation` 的 `count` 缺省为 5，显式值须为 1..20，超范围会返回参数错误而非静默钳制。20 限制的是请求作品数：一次请求可触发多个作品下载，每个作品又可展开为多页/多文件，全部产物元数据会进入同一 structured response；该边界避免无界放大下载工作与 JSON-RPC 输出，不截断单个作品的文件。推荐列表不足请求数时下载实际可用数量。
- `download` 只返回本地路径、`file://` URI、`mime_type`、页号与大小；不内嵌 ImageContent 或 base64 缩略图。
