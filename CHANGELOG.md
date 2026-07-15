# Changelog

本文件记录项目中值得用户和集成方关注的变化。

格式遵循 [Keep a Changelog 1.1.0]。项目开始切正式版本后，再按
[Semantic Versioning] 维护版本段与比较链接。

## [Unreleased]

## [0.3.0] - 2026-07-15

### Changed

- Breaking: 公开 Go SDK 已迁移至 `github.com/FlanChanXwO/pixiv-cli/pixiv`；旧导入路径不保留兼容 package。
- Breaking: 认证入口只接受原始 Pixiv App API refresh token；网页 Cookie（包括 `refresh_token=...`）不再被解析、提取或转换。

### Added

- MCP 新增 `recommended`：以必填 kind 返回插画、漫画、小说或作者推荐；`all` 以每流独立分页的 structured output 返回四类推荐。
- `pixiv recommended` 现要求 `all|illust|manga|novel|user` kind；`all` 原子返回四类个性化推荐。
- MCP 新增 `user_detail`，以必填 `user_id` 返回完整稳定的用户详情 structured output。
- 新增 `pixiv user detail USER_ID`；可用 `--json` 输出完整、稳定的用户详情 SDK envelope。
- `pixiv search` 新增 `--rating`、`--type` 与 `--ai-type` 本地结果过滤；带 `--limit`/`--page` 时会按匹配结果继续读取 opaque cursor。

### Fixed

- 修复真实 App API 四类推荐的 `next_url` 返回 `offset=0` 时被误判为 malformed 的问题；opaque cursor 继续保持种类、查询与账号来源隔离。
- 修复 App API 用户详情把 `profile_publicity` 的 `public`/`private` wire 值误判为 malformed 的问题；公开 SDK 继续稳定输出 bool。
- `auth login` 的浏览器 callback 页现在明确提示“授权已收到、正在回到 CLI 完成登录”；非 JSON 的登录成功输出精简为单行。
- 默认日志级别改为 `warn`，避免普通 CLI 成功命令把 INFO 操作诊断写入 stderr；显式 `info` 配置和环境覆盖保持不变。
- 修复显式与自动更新检查未向 GitHub Releases API 发送项目识别性 `User-Agent` 而可能收到 HTTP 403 的兼容性问题。

### Security

- `auth login` 不再启动受管 Chromium、连接 DevTools/CDP、读取浏览器历史/会话/存储或扫描活动标签页；只保留本轮 loopback、受控 `pixiv://` helper 和用户显式手动回填。
- SDK、CLI、MCP、环境变量和已存账号在 OAuth 请求前统一拒绝 Cookie 形态凭据，且不回显输入内容。

## [0.2.0] - 2026-07-13

### Added

- 新增公开 Go SDK；提供具体 `*pixiv.Client`、稳定模型、类型化错误、opaque cursor、账号/config 与受策略限制的资源流访问。
- 新增 `pixiv user artworks/bookmarks/following [USER_ID]`；新增 `bookmark add/remove` 与 `follow add/remove`。
- MCP 新增 `user_artworks`、分页用户列表、收藏/关注写操作及 structured output。
- 新增可注入的 `slog` 诊断日志与 `log_level`/`log_format` 配置，支持 `PIXIV_LOG_LEVEL`/`PIXIV_LOG_FORMAT` 覆盖。
- 新增 Linux quality gate 与六平台已打包 binary smoke；它们离线验证 CLI、config 与 MCP stdio，不使用 Pixiv 凭据或真实上游网络。

### Changed

- 列表 CLI 改用 `--limit` 和逻辑 `--page`；`--offset` 已废弃。CLI/MCP 不暴露 SDK cursor。
- 有 refresh token 时 App API 失败不再自动回落 Web；Web 仅用于无 token 的匿名白名单读操作和明确 metadata enrichment。

## [0.1.1] - 2026-07-13

### Fixed

- 修复直接下载的 Release binary 在本机不存在预期 `GOBIN`/`GOPATH/bin/pixiv` 时，把该正常安装来源
  判定为错误而无法执行 `pixiv update --check` 的问题；不存在的 go install 目标现在会正确归类为
  Release，其他路径解析错误仍会原样报告。
- 修复 Release workflow 在 Windows 上以 MinGW GCC 链接 MSVC Rust staticlib 的错误；六平台 Go
  测试、race、vet、pre-commit 与最终构建现在统一使用各自受审计的 cgo linker，Windows 固定为
  LLD-backed Clang。
- 修复登录测试夹具对回调 URL 列表的并发读写，并隔离不应访问真实 macOS URL handler/AppleScript
  的场景，避免 race detector 报错或冷 runner 因系统 helper 副作用耗尽显式测试等待窗口。
- 新增不可变 tag 首次发布在创建 Release 前失败时的受审计恢复入口；恢复仍绑定原 tag，测试门禁与
  生产资产使用独立 runner，后者以 clean checkout 重建工作树和 staticlib，禁止默认分支测试 overlay
  或其进程环境混入 binary、许可证或归档。
- 修复恢复测试门在 Windows runner 上对 ACL、`.exe`、CRLF、文件共享和路径转义的错误假设；覆盖路径
  受静态 policy 限制，生产资产仍只由不可变 tag 源码构建。

## [0.1.0] - 2026-07-13

### Added

- 新增项目级 changelog，集中记录用户可见变化、兼容性说明和发布准备事项。
- 新增 POSIX sh 构建脚本 `sh scripts/build.sh`，默认将二进制输出到 `build/`。
- 新增 `pixiv version [--json]` 与根 `pixiv --version`；JSON 输出包含 `version`、`commit`、`build_date`。
- 新增 `pixiv update [--check] [--prerelease] [--proxy URL]`，支持 Homebrew stable/beta、精确 tag
  `go install` 与签名 Release binary 策略；开发构建拒绝更新。
- 新增可关闭的 `update_check_enabled` 自动 stable 更新提示；普通 CLI 成功后最多每 24 小时检查一次，
  自动检查最多等待 3 秒，且不会污染 JSON/MCP stdout 或改变业务命令退出码。
- 新增内置 Rust ugoira GIF/APNG encoder；生产下载路径不再依赖 `ffmpeg`。
- 新增经六平台 native runner build/smoke 与统一 source digest 验证的 committed Rust staticlibs 和
  `manifest.json`，供受支持的 source build 与精确 tag `go install` 使用。
- 新增固定 six-target Release asset 格式、checksum/Ed25519 签名、Homebrew formula renderer 与六 native
  runner workflow。
- 新增 Homebrew stable `pixiv-cli` 与 beta `pixiv-cli-beta` formula 模板；两者冲突并同装 `pixiv`，
  不依赖 `ffmpeg`。
- Release workflow 现把同一份已发布 checksum 渲染为 stable/beta Homebrew formula，在 macOS/Linux
  双架构真实安装并核对版本后，才以独立 deploy key 向 public tap 推送唯一对应 formula。

### Changed

- Breaking: 本地 auth 账号从自定义账号名改为 Pixiv UID；`auth add/login` 不再接收账号名，`auth use/remove/check` 使用 UID，`--uid` 取代 `--profile` 作为主选择参数。
- Breaking: `auth.json` schema 改为 `default_user_id` 与 `accounts[].user_id/username`；旧 `default_account/accounts[].name` 文件需要重新 `pixiv auth add` 或 `pixiv auth login`。
- `pixiv auth login` 默认打开浏览器时也保留终端粘贴兜底，可直接粘贴 callback URL、`pixiv://...` URL 或原始 authorization code。
- `pixiv auth login` 接受 Pixiv 官方 callback URL 与 `pixiv://account/login` 缺省 OAuth state 的授权码回填，同时继续要求本地 loopback callback 携带正确 state。
- `pixiv auth login` 在 macOS 默认会优先注册本地 `pixiv://` callback helper 并打开默认浏览器，以复用已有 Pixiv 登录态；helper 只把最终 callback URL 转交给本轮 CLI loopback，不安装扩展、不点击页面、不读取 cookie/token。若 helper 不可用，CLI 会退回专用 Chromium/Edge DevTools 捕获；macOS 上仍保留 Edge/Chrome/Chromium/Safari 标签页与 Chromium session/history 只读观察，并在 Pixiv 卡在 `post-redirect` 授权接力页时校验本轮 OAuth 后等待 `pixiv://` handoff，不再自动重开白页；状态不可读或 Pixiv 未生成 callback 时继续保留手动回填路径。

### Fixed

- 修复 Linux 原生 Rust staticlib 的 `libm` 链接，以及 Windows checkout 对 first-party crate、Cargo
  vendor、本地 locked dependency 和生成 license bundle 的文本转换，使六平台保持同一 Rust
  source identity；Rust source digest 也会在筛选 `src/.cargo/vendor` 前规范化 Windows 路径分隔符，
  避免真实输入被静默漏掉；release archive 的 `LICENSE` 固定 LF checkout，避免 Windows/Unix
  许可证成员字节分裂；Windows cgo 现用库搜索参数、Rust std import libraries 和
  LLD-backed Clang 链接 `*-pc-windows-msvc` staticlib，确保六平台 native evidence 能校验并链接真实
  ugoira encoder；Windows release `.zip` 改用 runner 预装的 7-Zip，避免 Git Bash 缺少 `zip` 而中断
  native evidence。

### Security

- 发布基础设施使用公开 source/tap 仓库、受保护 `release` Environment、隔离的 Ed25519 签名与 tap
  deploy credentials；私钥仅位于该 Environment 与 macOS Keychain 恢复副本。
- native-evidence 的 policy command 现与 cgo encoder 解耦，可在目标 staticlib 生成前 fail-closed
  地检查 workflow；避免缺库被误报为 policy 或 runner 配置通过。
- 新增独立、无 secret/发布副作用的 six-target native evidence workflow 与本地 AST policy；每个
  runner 会记录实际 staticlib、版本化 binary、完整许可证 archive 与 source/hash evidence。六个真实
  runner 已针对同一经审计 main SHA 产出并验证对应 artifact，且 source digest 与 `LICENSE` hash 一致。
- Release workflow 现由可解析的 YAML policy 检查所有 action 的 full-SHA pin、最小权限、精确
  trigger/runner matrix；默认分支 trust gate 在独立、无 Environment/secret 的 job 完成后，publish
  才能访问精确的 signing secret。policy 同时拒绝被软失败或条件跳过的质量门禁，并把 SemVer channel
  的 stable/prerelease 分支绑定到实际 `gh release create` 参数，避免排版变化或 action tag/门禁移除
  悄然削弱发布供应链。
- Release workflow policy 现 fail-closed 地拒绝 YAML alias、merge key 与重复 mapping key，并禁止
  required job/ancestry gate 被条件跳过；每项质量检查固定为独立的直接命令 step，validate/build checkout
  显式不持久化凭据，避免工作目录、shell 控制流或 credential 语义悄然改变发布结果。
- Release workflow 的四个 checkout 现固定为 canonical full-SHA source；trust job 只能在完整、无凭据
  checkout 后立即验证 tag ancestry，签名 job 也拒绝 `ref`、`repository`、`path` 等会令源码与发布 asset
  脱钩的 checkout 重定向。
- Homebrew tap secret 仅可进入最终 protected deploy job 的最后 push step；policy 固定 HTTPS clone、
  唯一 formula diff、官方 GitHub ED25519 known_hosts、strict SSH 与 `HEAD:main`，任一前置安装失败均
  不会写 tap。
- Rust ugoira crate 的 crates.io 依赖现以完整 locked `vendor/` 闭包及 Cargo checksum 随源码审计；
  release 验证会在空 Cargo cache 下完成 metadata/build/test 与六 target 许可证检查，缺失 vendor、
  checksum 不匹配或 registry fallback 都会明确失败。
- 受支持 binary 现在内置可轮换的 production Ed25519 key ID→public key trust root，并以已知真实签名和
  SPKI fingerprint 回归验证；私钥仍不进入源码。Release installer 会在替换前验证 Ed25519 签名
  checksum、archive SHA-256 与下载 binary version，且不会因签名私钥或 Release 状态改用其他信任来源。
- 更新检查写入前会把既有 `pixiv-cli` cache 目录在 Unix-like 平台收紧为 `0700`；Windows 保持其 ACL 语义。
- Release installer 会在下载前校验 Releases API 或 ETag cache 中每个选中 asset 的精确 GitHub HTTPS 来源；跨 host、仓库、tag、asset 或含歧义 URL 的记录会明确失败，绝不请求该 URL。
- Release installer 拒绝 archive 原始路径中含有任何 `..` segment 的条目，防止归一化后的路径绕过解包安全校验。
- Release installer 在 archive 解包、版本预检、staging 及最终替换前都会保留调用方取消；取消更新会清理临时文件且不会替换当前 executable。
- Release installer 与 Windows pending-update backup cleanup 在当前 executable 为软链接时保留链接入口，只操作解析后的真实目标；断链会明确失败而不会改写链接或删除备份。
- v0.1.0 不含 Apple notarization 或 Windows Authenticode；直接下载仍可能显示 Gatekeeper/SmartScreen
  提示。

[Keep a Changelog 1.1.0]: https://keepachangelog.com/zh-CN/1.1.0/
[Semantic Versioning]: https://semver.org/spec/v2.0.0.html
