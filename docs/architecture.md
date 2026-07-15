# 架构说明

## 总体流程

`cmd/pixiv/main.go` 是唯一官方二进制入口，它只负责调用 `internal/cli`：

1. `pixiv` 无参数显示 CLI 帮助。
2. `pixiv auth/config/version/update/search/detail/ranking/recommended/user/bookmark/follow/download` 进入 CLI 模式。
3. `pixiv mcp` 委托 `internal/bootstrap` 组装并运行 MCP stdio server。
4. CLI 与 MCP 通过 `internal/bootstrap` 共享生产 wiring：
   - 账号认证来自 `os.UserConfigDir()/pixiv/auth.json`
   - 全局配置来自 `os.UserConfigDir()/pixiv/config.toml`
   - 公开环境变量作为覆盖层参与合并
5. MCP 模式若没有 `PIXIV_REFRESH_TOKEN`，会回退到 `auth.json.default_user_id`；若仍无 refresh token 且 `web_fallback_enabled=true`，支持匿名能力的路径会走 Pixiv web/ajax API。

## 包职责

### `cmd/pixiv`

负责生成 `pixiv` binary 的 `main` package。它不承载业务逻辑，只委托 `internal/cli.Run` 并返回进程退出码。

### `internal/cli`

负责 CLI 用户态的命令分发与输出：

- Cobra 命令树、help 和 flag 解析。
- 文本/JSON 输出。
- `auth login` 的 loopback OAuth、浏览器打开和 TTY 交互。
- `pixiv mcp` 分发。
- `pixiv version`、根 `--version` 与 `pixiv update` 的输入/输出适配。
- 普通 CLI 成功命令后的只读自动更新提示；提示和失败 warning 仅写 stderr。

它不直接拥有账号存储变更、Pixiv client 构造或下载管理器构造；这些职责由 `internal/application` 与 `internal/bootstrap` 承接。

`version` 的 JSON stdout 精确为 `version`、`commit`、`build_date`。自动更新只在普通业务命令
成功后运行，跳过 MCP、help、`version`、`update` 和开发构建；它选择 stable Release、使用用户
cache 的 24 小时节流，并最多等待 3 秒。配置、网络、来源识别失败只作为 stderr warning，不能
改变已成功业务命令的退出码，也不能混入 JSON stdout 或 MCP JSON-RPC。

### `internal/buildinfo`

保存由 Go linker 注入的 `Version`、`Commit`、`BuildDate`。本地默认是 `dev`/`unknown`/`unknown`；
只有 version 为 `dev` 的构建被视为开发构建，并必须拒绝自更新。

### `internal/application`

负责 CLI/MCP 之外的应用用例编排：

- `AccountService`：账号 add/list/remove/use/check。
- `ConfigService`：`config.toml` path/get/set/unset。
- `LoginService`：生成 PKCE/state、authorization-code exchange，并保存账号；Pixiv 登录 URL 构造仍留在 CLI adapter。
- `SDKService`：为 CLI/MCP 打开顶层 `pixiv` client，并把调用方选择的账号/代理/JSON 设置映射到 SDK operation snapshot；作品查询和下载均从该 snapshot 的 public SDK 能力继续执行。
- `DownloadService`：把同一 operation snapshot、本次下载路径和文件名模板交给 bootstrap 注入的窄 factory，并委托下载；应用层不构造具体 manager。

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
- 认证文件路径解析和 `0600` 权限写入。

### `internal/config`

负责 `config.toml` 及运行时配置：

- `config.toml` schema、默认值和配置键定义。
- 运行时配置合并：`config.toml` 与公开环境变量。
- `pixiv config path/get/set/unset` 需要的强类型解析与稀疏写回。

配置拆分如下：

- `auth.json`：只保存 `default_user_id` 与 `accounts[].user_id/username/refresh_token`，文件权限固定为 `0600`。
- `config.toml`：只保存用户显式设置过的全局配置键，文件权限固定为 `0600`。

运行时设置使用 `koanf` 合并 `config.toml` 与公开环境变量；`config set/unset` 使用 `tomledit` 写回，尽量保留注释、顺序和布局。

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

该包不得把签名、checksum、HTTP、archive、替换或权限错误伪装成“无更新”。production trusted key、
签名私钥与 Keychain 恢复副本、受保护 `release` Environment 和公开 remote 已按 Task 20 配置；完整六目标
native evidence 与 staticlib manifest 已回填。v0.3.0 已完成正式 tag、受签名 Release 与 stable tap formula；
Release 安装的失败语义仍是保护边界，而不是临时降级。

### `pixiv`

公开 concrete facade。`NewClient` 只使用显式 options；`OpenDefault` 复用本地 auth/config，并在每个公开操作开始时取得一次 snapshot。它暴露规范化模型、opaque cursor、`*pixiv.Error`、账号/config、登录 session、资源流和下载；CLI/MCP 与外部 Go 程序消费同一契约。

调用方在自身 adapter 中定义 source mode、budget、filter、cursor 持久化和 HTTP presentation。本仓库不提供 HTTP Provider、Discover、Probe、Capabilities、RSS 或 crawler。

### `internal/pixiv/protocol`、`appapi`、`webapi`、`oauth`、`resource`

内部协议包只由 facade 组合：

- `protocol`：上游 base、profile header、endpoint catalog 与脱敏 adapter failure 的唯一来源；不读配置、不发请求，也不保存响应 body、URL、header 或凭据。
- `appapi`：有凭据的 App content API 与 raw DTO/mapper。
- `webapi`：匿名白名单读与明确 metadata enrichment；不接收 SDK Authorization/Cookie。
- `oauth`：PKCE、code exchange、refresh 与 token state。
- `resource`：受 policy 约束的 resource transport、redirect/header/body 边界。

有 token 时 App API 是主路径，失败不自动 Web fallback；无 token 且 `web_fallback_enabled=true` 时才允许明确白名单 Web read。pages/original ugoira enrichment 必须由 operation policy 显式选择。

### `internal/pixiv/model`

集中 Pixiv response/domain 类型以及 Pixiv 协议枚举 typed const，例如 search target、sort、ranking mode、restrict 和 illust type。MCP delivery 等传输层常量仍留在 `internal/mcpserver`。

### `internal/mcpserver`

负责将 Pixiv 与下载能力注册为 MCP tools。所有 Pixiv 内容、认证、资源和写操作都通过 `SDKService` 使用 public SDK；旧构造器保留的首个 API 参数只是废弃占位，生产路径不会读取。下载由 operation snapshot 对应的 `DownloadManager` 执行。stdio runtime 由 `internal/bootstrap` 组装和启动。

输出目前以中文文本为主，适合直接返回给 LLM/MCP 客户端。认证相关工具会显式提示缺少 token、认证失败或自动认证失败的真实原因。

### `internal/download`

负责下载和本地文件落盘：

- `Download` 会同步下载 ID 列表，并返回每个作品的实际产物路径。
- `Enqueue` 会去重、排序并为每个 ID 启动后台任务。
- 内部 semaphore 当前并发为 5。
- 单页作品保存到下载目录。
- 多页作品和 ugoira 会建立作品子目录。
- ugoira 先下载 zip，再由 Rust FFI encoder 合成为 GIF/APNG。

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

只保存跨包复用、无协议语义的基础设施常量，例如私有文件权限、私有目录权限；`AppConfigDirName` 是 config/auth 共同使用的路径命名空间例外。Pixiv 协议值、MCP delivery 值、config key/default 等仍留在所属领域包。

### `internal/utils`

提供文件名清理、模板展开、ID 去重和 refresh token 输入规范化：

- 非法文件名字符替换为 `_`。
- 支持 `{author}`、`{title}`、`{id}` 模板字段。
- 多页作品追加 `_pN` 后缀。
- 下载 ID 去重时会丢弃小于等于 0 的 ID，并排序。
- refresh token 只接受原始 Pixiv App API token；Cookie 形态（含 `refresh_token=...`）在 SDK、CLI、MCP、环境变量和已存账号边界统一拒绝，绝不提取、转换或发送。

`internal/utils/*` 子包提供无业务语义的通用工具：

- `files`：用户配置路径拼接与私有文件写入。
- `text`：字符串默认值和首个非空值。
- `uri`：URL path 提取与 file URI 生成。
- `media`：按文件扩展名推断基础 MIME type。
- `parse`：通用正整数解析。

## 已知约束

- `appapi`、`webapi` 与 resource transport 使用 caller/SDK 注入的 HTTP client；SDK 不新增无依据固定请求超时，取消由 context 传播。
- `pixiv mcp` 是 MCP stdio server 的显式启动方式；直接执行 `pixiv` 不会启动 MCP。
- CLI 账号文件以明文 JSON 保存 refresh token、user ID 和可选 username，不保存 access token，文件权限固定为 `0600`；需要系统钥匙串时再扩展。
- `config.toml` 采用稀疏写入，不会把默认值整份落盘。
- `download_random_from_recommendation` 的 `count` 缺省为 5，显式值须为 1..20，超范围会返回参数错误而非静默钳制。20 限制的是请求作品数：一次请求可触发多个作品下载，每个作品又可展开为多页/多文件，全部产物元数据会进入同一 structured response；该边界避免无界放大下载工作与 JSON-RPC 输出，不截断单个作品的文件。推荐列表不足请求数时下载实际可用数量。
- `download` 默认只返回本地路径和 `file://` URI；当 `delivery=image_content` 时，会把所有下载产物作为 MCP `ImageContent` 一并返回，不做无依据截断。
- `get_thumbnail_base64` 会将缩略图完整编码为 base64 文本返回，调用方需注意输出体积。
- 匿名 `search_user` fallback 语义是“作品搜索结果中的相关作者去重”，不是 Pixiv 官方用户名搜索。
