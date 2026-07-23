# 架构说明

## 总体流程

`cmd/pixiv/main.go` 是唯一官方二进制入口，它只负责调用 `internal/cli`：

1. `pixiv` 无参数显示 CLI 帮助。
2. `pixiv auth/config/version/update/search/search-options/detail/ranking/recommended/user/bookmark/follow/download` 进入 CLI 模式；`auth import` 负责 direct token import 或 bundle restore，`auth export` 负责本地 secret snapshot。
3. `pixiv mcp` 委托 `internal/bootstrap` 组装并运行 MCP stdio server。
4. CLI 与 MCP 通过 `internal/bootstrap` 共享生产 wiring：
   - 账号认证来自 `~/.pixiv-cli/auth.json`（Windows：`%USERPROFILE%\.pixiv-cli\auth.json`）
   - 全局配置来自 `~/.pixiv-cli/config.toml`（Windows：`%USERPROFILE%\.pixiv-cli\config.toml`）
   - 公开环境变量作为覆盖层参与合并
5. MCP 模式若没有 `PIXIV_REFRESH_TOKEN`，会回退到 `auth.json.default_user_id`；若仍无 refresh token 且 `web_fallback_enabled=true`，支持匿名能力的路径会走 Pixiv web/ajax API。

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

`version` 的 JSON stdout 精确为 `version`、`commit`、`build_date`。自动更新只在普通业务命令
成功后运行，跳过 MCP、help、`version`、`update`、全部 `auth export`、`auth import --file` 和开发构建；它选择 stable Release、使用用户
cache 的 24 小时节流，并最多等待 3 秒。配置、网络、来源识别失败只作为 stderr warning，不能
改变已成功业务命令的退出码，也不能混入 JSON stdout 或 MCP JSON-RPC。
进程启动时的 Windows pending-update cleanup 也属于潜在 mutation；全部 `auth export` invocation 在 Cobra
解析前即识别并跳过该 cleanup，其他命令仍沿用正常 startup cleanup。root bool flag 的重复覆盖语义由
聚焦测试保护，不能让 `--help=false` 等写法误绕开 secret export 边界。

### `internal/cli/loginhelper`

负责 `auth login` 使用的系统 URL scheme helper 安装。`internal/cli` 只经 `Install` 入口请求本次登录的
helper 并保留 OAuth、loopback HTTP、系统浏览器和 TTY 编排；Darwin 实现独立持有内嵌 Swift、
`Info.plist`、LaunchServices 注册及默认 handler 恢复逻辑，其他平台显式报告不支持该 helper。

### `internal/buildinfo`

保存由 Go linker 注入的 `Version`、`Commit`、`BuildDate`。本地默认是 `dev`/`unknown`/`unknown`；
只有 version 为 `dev` 的构建被视为开发构建，并必须拒绝自更新。

### `internal/application`

负责 CLI/MCP 之外的应用用例编排：

- `AccountService`：账号 import/list/export/remove/use/check；bundle export/restore 只经 public SDK 读取或写入本地 store，direct token import 仍经 OAuth 验证并保存 rotation 后的 token。
- `ConfigService`：`config.toml` path/get/set/unset。
- `LoginService`：生成 PKCE/state、authorization-code exchange，并保存账号；Pixiv 登录 URL 构造仍留在 CLI adapter。
- `SDKService`：为 CLI/MCP 打开顶层 `pixiv` client，并把调用方选择的账号/代理/JSON 设置映射到 SDK operation snapshot；作品查询和下载均从该 snapshot 的 public SDK 能力继续执行。
- `DownloadService`：把同一 operation snapshot、本次下载路径和文件名模板交给 bootstrap 注入的窄 factory，并委托下载；应用层不构造具体 manager。
- 分页遍历：统一负责 opaque cursor 跟随、逻辑 skip/limit、单批兼容语义与重复 cursor 止环；CLI 可按批流式消费，MCP 可经同一引擎收集结果。各 adapter 只解析各自协议输入并组织输出。

### `internal/bootstrap`

生产 composition root，负责把 `internal/config`、`internal/storage/auth`、顶层 `pixiv`、`internal/download`、`internal/mcpserver`、更新 release client/installer 和 application services 组装起来。测试可以替换 service 里的小接口或 factory，不需要复制生产 wiring。

`NewUpdateCoordinator` 通过 `productionReleaseInstallerOptions` 为 Release installer 注入随受支持
binary 提交的 Ed25519 key ID→public key 映射，并在每次组装时复制 map 与 key bytes，避免调用方污染
production trust root。该公开 key 的 SPKI fingerprint 与已知签名 fixture 由同包测试验证；私钥不在
bootstrap、源码或运行时配置中。只读更新检查不需要该 key；当前 v0.3.0 已是可安装的公开 Release，
但该 wiring 本身仍不能代替每个版本独立的发布验收。

### `internal/storage/auth`

负责本地账号状态：

- `auth.json` 读写与默认 UID 管理。
- 认证文件路径解析和平台对应的凭据文件写入保护。
- direct token import 保存 OAuth 验证/rotation 后由 Pixiv 返回的 UID、username 与 refresh token。
- bundle restore 在单次锁定的 snapshot 中 merge 全部账号并原子保存；已有 default 不变，仅空 store 采用 bundle default。
- auth export 只读取默认、精确 UID 或全部本地账号；不读取环境 token、不刷新、不联网、不修改状态。

### `internal/config`

负责 `config.toml` 及运行时配置：

- `config.toml` schema、默认值和配置键定义。
- 运行时配置合并：`config.toml` 与公开环境变量。
- `pixiv config path/get/set/unset` 需要的强类型解析与稀疏写回。

配置拆分如下：

- `auth.json`：只保存 `default_user_id` 与 `accounts[].user_id/username/refresh_token`；Unix-like 文件权限为 `0600`。
- `config.toml`：只保存用户显式设置过的全局配置键；Unix-like 文件权限为 `0600`。

运行时设置使用 `koanf` 合并 `config.toml` 与公开环境变量；`config set/unset` 使用 `tomledit` 写回，尽量保留注释、顺序和布局。

`auth.json` 与 `config.toml` 共用 `internal/utils/files` 的原子写入协议：于目标同目录使用不含
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

### `pixiv`

公开 concrete facade。`NewClient` 只使用显式 options；`OpenDefault` 复用本地 auth/config，并在需要 runtime configuration 的公开操作开始时取得一次 snapshot。它暴露规范化模型、opaque cursor、`*pixiv.Error`、账号/config、登录 session、资源流和下载；CLI/MCP 与外部 Go 程序消费同一契约。

认证备份同样只经 public facade：`AuthExportSelection` 以零值/default、`UserID` 或 `All` 选择本地 snapshot；`ExportAuthBundle`、`EncodeAuthExportBundle` 与 `DecodeAuthExportBundle` 负责版本化 strict codec；`RestoreAuthBundle` 离线 merge 并原子写回。bundle 含 opaque refresh-token secret，是未加密的 point-in-time backup，不是 live sync；token rotation 后旧 bundle 或其他机器上的副本可能 stale。restore 写失败通过 `Error.LocalWriteCommitOutcome` 区分 replacement 前的 `not_committed`、replacement 已完成但 durability/cleanup 失败的 `committed`，以及 recovery 结果无法确认的 `unknown`；调用方不得把后两者伪装成已 rollback。

调用方在自身 adapter 中定义 source mode、budget、filter、cursor 持久化和 HTTP presentation。本仓库不提供 HTTP Provider、Discover、Probe、Capabilities、RSS 或 crawler。

### `internal/pixiv/protocol`、`appapi`、`webapi`、`oauth`、`resource`

内部协议包只由 facade 组合：

- `protocol`：上游 base、profile header、endpoint catalog 与脱敏 adapter failure 的唯一来源；不读配置、不发请求，也不保存响应 body、URL、header 或凭据。
- `appapi`：有凭据的 App content API 与 raw DTO/mapper；幂等 JSON 读取只会在首次 429 的有效 `Retry-After` 后按 context 重试一次。
- `webapi`：匿名白名单读；不接收 SDK Authorization/Cookie，也不承担认证态 metadata enrichment。
- `oauth`：PKCE、code exchange、refresh 与 token state。
- `resource`：受 policy 约束的 resource transport、redirect/header/body 边界。

`webapi` 包内按职责导航：`client.go` 编排各 Web operation，`transport.go` 负责 HTTP 与脱敏错误边界，
`pagination.go` 和 `parameters.go` 分别处理 Web 页码以及 endpoint 参数映射，`dto.go` 只声明 wire shape，
`decoder.go` 校验弹性数值、必需列表与 ajax envelope，`mapper.go` 将 Web DTO 规范化为共享 model。

有 token 时 App API 是主路径，失败不自动 Web fallback；搜索的分辨率、横纵比、绘图工具、作品类型与
屏蔽 AI 在 `appapi` adapter 翻译成上游参数，分级和仅 AI 则由 public facade 基于规范化字段筛选。
无 token 且 `web_fallback_enabled=true` 时才允许明确白名单 Web read；Web 搜索只转译已验证可靠的筛选，
R18/R18G/mature 与动态搜索选项会返回认证需求，不伪造空结果。Web adapter 不接收 token 或 Cookie，
也不提供 refresh-token-to-session 转换。认证态 detail/pages/ugoira metadata 均由 App API 提供；App 的页数不完整
或动图资源异常必须显式失败，不能转交 Web 补全。匿名 Web 的原图 ugoira 资源仍是其独立读路径。

### `internal/pixiv/model`

集中 Pixiv response/domain 类型以及 Pixiv 协议枚举 typed const，例如 search target、sort、ranking mode、restrict 和 illust type。MCP delivery 等传输层常量仍留在 `internal/mcpserver`。

### `internal/mcpserver`

负责将 Pixiv 与下载能力注册为 MCP tools。所有 Pixiv 内容、认证、资源和写操作都通过 `SDKService` 使用 public SDK；旧构造器保留的首个 API 参数只是废弃占位，生产路径不会读取。下载由 operation snapshot 对应的 `DownloadManager` 执行。MCP 的 nullable `page`/`limit` 只在本 adapter 解析，逻辑分页遍历由 application 共享引擎执行；旧 offset wire 字段已移除。stdio runtime 由 `internal/bootstrap` 组装和启动。

包内按职责拆分：`server.go` 负责构造与统一 observability wrapper，`registration.go` 只维护 tool 注册，`auth_tools.go` 和 `download_tools.go` 分别承载认证与下载，`legacy_tools.go` 承载文本型读取适配，`formatting.go` 集中文本/output helper，`sdk_runtime.go` 负责分页、operation snapshot、gate 与安全日志，`sdk_tools.go` 承载 SDK typed tools。文本型 handler 的失败结果可使用 `isError=false`，但必须把真实 cause 交给 wrapper；wrapper 把安全分类 metadata 写入 `~/.pixiv-cli/logs`（Windows：`%USERPROFILE%\.pixiv-cli\logs`）的按日纯文本 `YYYY-MM-DD.txt`，不读取参数或原始错误文本，也不把操作日志写到终端。正常空结果不会伪装成失败。

输出目前以中文文本为主，适合直接返回给 LLM/MCP 客户端。其中 `refresh_token` tool 会区分缺少 token、context 取消/deadline、安全 typed SDK 失败与未知失败；其未知底层错误只返回脱敏排查提示，不回显原始原因。完整 wire 语义见 [MCP 工具](../zh-CN/mcp-tools.md#配置认证与下载)。

### `internal/download`

负责下载和本地文件落盘：

- `Download` 会同步下载 ID 列表，并返回每个作品的实际产物路径。
- 单页作品保存到下载目录。
- 多页作品和 ugoira 会建立作品子目录。
- 单页与多页作品从上游 URL path 推导扩展名，并与模板生成的文件名一样规范化跨平台非法字符；扩展名还会替换 ASCII 控制字符并移除 Windows 非法尾随点/空格，但不猜测或静默替换扩展名。
- ugoira 先下载 SDK 验证的 `download_url` zip，再由 Rust FFI encoder 合成为 GIF/APNG；认证态可合法选择 App medium，绝不把它标记成 original。

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
`internal/download/staticlib` 只承载 source digest 与 manifest 的完整性契约，不导入 cgo encoder；
因此 native-evidence 的 **policy** gate 可以在目标库生成前执行。`record` 与 `consolidate` 同样不
触发 cgo 链接，但分别仍需要已生成的 staticlib/binary/archive 与完整六份 evidence 输入；下载运行时
仍只由 `internal/download` 组装 Rust FFI。

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

### `internal/common/constants`

只保存跨包复用、无协议语义的基础设施常量，例如私有文件权限、私有目录权限和安全 operation log 的稳定事件/字段名；`AppDataDirName` 是本地应用数据根目录的路径命名空间例外。Pixiv 协议值、MCP delivery 值、config key/default 等仍留在所属领域包。

### `internal/logging`

集中定义 CLI、MCP、SDK、下载器与 App API 共用的结构化 operation event。`internal/config/logger.go` 仍是 runtime logger 的构造入口；本包只规范 event schema、丢弃 logger 与安全写入方式，不配置全局 logger。事件只允许 component、operation、backend、耗时、结果、安全错误分类、HTTP status、作品/用户 ID 以及限流重试的已解析等待时长；绝不记录 token、URL、原始 header、请求输入或响应 body。

### `internal/utils`

按单一职责拆分文件名清理、模板展开、ID 去重和 refresh token 输入规范化：

- 模板内容及 URL path 推导扩展名中的非法文件名字符替换为 `_`；扩展名额外处理 ASCII 控制字符和 Windows 非法尾随点/空格。
- 支持 `{author}`、`{title}`、`{id}` 模板字段。
- 多页作品追加 `_pN` 后缀。
- 下载 ID 去重时会丢弃小于等于 0 的 ID，并排序。
- refresh token 只接受原始 Pixiv App API token；Cookie 形态（含 `refresh_token=...`）在 SDK、CLI、MCP、环境变量和已存账号边界统一拒绝，绝不提取、转换或发送。

`internal/utils/*` 子包提供无业务语义的通用工具：

- `filename`：下载文件名清理、模板展开和 URL path 派生扩展名。
- `ids`：正整数 ID 的排序去重。
- `credentials`：refresh token 输入规范化，以及 Cookie 形态输入的拒绝。
- `files`：用户配置路径拼接、配置 store 原子写入与任意目标 secret export writer。后者在 Unix-like 将文件设为 `0600` 且不改变既有 parent 权限/ownership；Windows 从创建时就设置 protected DACL 与 owner，只允许当前用户、LocalSystem、builtin Administrators 完全控制，replacement 后重新应用同一 owner/DACL。CI tests 提供该 Windows policy 的覆盖；本地交叉编译留给后续验收，文档不声称已在真实 Windows 主机运行本次验收。
- `text`：字符串默认值和首个非空值。
- `uri`：URL path 提取与 file URI 生成。
- `media`：按文件扩展名推断基础 MIME type。
- `parse`：通用正整数解析。

## 已知约束

- `appapi`、`webapi`、`oauth` 与 resource transport 使用 caller/SDK 注入的 HTTP client；默认 client 专用于当前 SDK client、无整请求固定 timeout，取消与 deadline 由 context 传播。显式 client 保持调用方策略；详见 [ADR 0010](adr/0010-http-client-timeout-and-context.md)。
- `pixiv mcp` 是 MCP stdio server 的显式启动方式；直接执行 `pixiv` 不会启动 MCP。
- 不新增持久账号 import/export MCP tool；既有 session-scoped MCP 认证 tool 与 wire contract 不变。
- CLI 账号文件以明文 JSON 保存 refresh token、user ID 和可选 username，不保存 access token；Unix-like 文件权限为 `0600`，Windows 依赖父目录/既有目标 ACL，当前不主动配置私有 DACL；需要系统钥匙串时再扩展。
- `config.toml` 采用稀疏写入，不会把默认值整份落盘。
- `download_random_from_recommendation` 的 `count` 缺省为 5，显式值须为 1..20，超范围会返回参数错误而非静默钳制。20 限制的是请求作品数：一次请求可触发多个作品下载，每个作品又可展开为多页/多文件，全部产物元数据会进入同一 structured response；该边界避免无界放大下载工作与 JSON-RPC 输出，不截断单个作品的文件。推荐列表不足请求数时下载实际可用数量。
- `download` 只返回本地路径、`file://` URI、`mime_type`、页号与大小；不内嵌 ImageContent 或 base64 缩略图。
- 匿名 `search_user` fallback 语义是“作品搜索结果中的相关作者去重”，不是 Pixiv 官方用户名搜索。
