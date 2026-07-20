## [Unreleased]
### Added
- 所有作品模型、CLI JSON/文本与 MCP 结构化/文本输出增加作品页 `url`（`https://www.pixiv.net/artworks/${id}`），JSON 为 public Illust 首字段，文本输出放在每件作品第一行。

### Changed
- Breaking: 移除 MCP 旧 wire 字段 `search_r18`、`user_id_to_check`、`max_bookmark_id`、`offset`、`include_thumbnail`；列表与搜索统一使用规范字段 `user_id`、`rating`、`page`/`limit`。
- Breaking: 移除 CLI 兼容入口 `--ai-type`、`--r18`、`--profile`、`--offset` 与 `search --type comics`；请分别使用 `--ai-mode`、`--rating r18`、`--uid`、`--page`/`--limit` 与 `--type manga`。

### Fixed
- 搜索在本地筛选产生连续空上游批次时，CLI/MCP 会补拉到首个非空逻辑批次；`--limit N`/`limit` 填满逻辑结果，`--limit 0`/`limit=0` 遍历全部，`--page`/`page` 按过滤后结果分页。
- App 作品 AI 字段优先读取 `illust_ai_type`，并兼容旧 `ai_type`；本地 AI 判定仍固定 `AIType==2`。


## [0.4.5] - 2026-07-20

### Fixed

- Linux Homebrew hosted staging verification now runs the real local staging-tap `brew install` in a short-lived `homebrew/brew` container pinned by immutable digest. This avoids the hosted Linuxbrew `Resource` staging cleanup `EINVAL` while retaining a read-only formula mount, no secrets, a local-only tap and an exact installed-version check. The pinned Homebrew 4.6 image has no `brew trust` or standalone `python3`, so trust remains only in the native macOS path and the container uses its bundled Ruby JSON parser after installation. The Linux container tap is created locally, fed only by the read-only mount and discarded with `--rm`. macOS and end-user Homebrew installs remain unchanged. The GitHub prepublish workflow has passed this verification on all four Homebrew platform/architecture combinations using the published v0.4.4 assets.

## [0.4.4] - 2026-07-19

### Fixed

- Linux Homebrew release validation keeps buildpaths in Homebrew prefix to avoid cross-filesystem FileUtils EINVAL.

## [0.4.3] - 2026-07-19

### Fixed

- 修复 Linux Homebrew Release validation 使用 `/var/tmp` 时可能触发 `EINVAL`、导致 staging formula 无法安装的问题；该验证步骤现仅在 Linux 使用 runner 私有临时目录，macOS 与公开 formula 路径保持不变。

## [0.4.2] - 2026-07-19

### Added

- 新增 `scripts/install.sh` 与不依赖 PowerShell 的 `scripts/install.cmd`：自动选择最新 stable Release 的当前 OS/arch archive，先验证发布 SHA-256 和暂存 binary，再执行无管理员权限的用户级安装；Release 以固定名称发布两个脚本并把它们纳入签名 checksum 集合，现有 locale 的 README 同步提供可复制的人类命令与 Coding Agent 安装 prompt。
- 新增 `pixiv auth import [REFRESH_TOKEN]` 与 `pixiv auth export [UID] [--all] [--output PATH] [--force]`：支持隐藏 TTY/raw stdin direct token import、单账号 raw export、全部账号 versioned bundle export，以及 `--file PATH|-` 离线原子 restore。
- 公开 Go SDK 新增 `AuthExportSelection`、versioned auth bundle model/strict codec、`ExportAuthBundle`、`RestoreAuthBundle`、`AuthRestoreResult` 与 `LocalWriteCommitOutcome`，供调用方实现 point-in-time secret backup 与可分类的离线恢复。
- 插画搜索新增稳定的分级、作品类型、AI、横纵比、分辨率与绘图工具筛选；CLI 新增 `--ai-mode`、`--aspect-ratio`、`--resolution` 与 `--tool`，`--type` 支持 `illust-and-ugoira`/`manga` 并保留 `comics` alias。
- 公开 Go SDK 新增 `SearchIllustFilters`、`SearchIllustOptions` 与 `Illust.Tools`；CLI 新增需要 App 认证的 `pixiv search-options WORD`，动态列出当前可用绘图工具，不引入收藏数或 Cookie 筛选。
- MCP `search_illust` 新增与 SDK 一致的六个筛选字段，并新增需要认证、返回 `{tools,text}` structured output 的 `search_illust_options`；legacy `search_illust` 继续返回 `{text}`，旧 `search_r18` 继续作为兼容字段。
- 公开 Go SDK 的 `upstream_unavailable` 错误新增安全 `TransportKind` 分类，可区分 DNS、TLS、代理连接、拒绝连接、连接重置和无 typed 信号的未知传输失败；诊断仍不暴露 URL、主机、证书或凭据。
- 公开 Go SDK 的本地 snapshot 错误新增安全 `LocalStateKind` 分类，可区分 auth/config 格式、权限、缺失、代理配置、账号身份不匹配、不可用和未知状态；`errors.Unwrap` 仅提供固定脱敏原因，缺失的可选 auth/config 文件仍按空状态正常加载。
- 新增面向 coding agent 的 `pixiv-cli` skill（`skills/pixiv-cli/`，全英文）：SKILL.md 提供预检、凭据安全红线、操作确认分级、输出/token 控制策略与语义陷阱说明，`references/` 收录安装、发现、下载与排障 playbook。
- README 国际化扩展为英文、简体中文与日语入口；新增日语 CLI reference，并补齐英文 SDK/MCP contract。Public docs 现按 `docs/<locale>/` 组织，架构、开发、ADR 与 Agent 规则集中到 `docs/maintainers/`，旧路径保留兼容导航。

### Changed

- Breaking: v0.4.2 删除 `pixiv auth add`、`pixiv auth token` 与 `--token`，不保留 alias/stub；direct token 入口统一为 `auth import`，显式 secret stdout 统一为不带 `--output` 的 `auth export [UID]` 或 `auth export --all`。
- `auth import` 的 direct 与 bundle 成功报告统一使用不含 secret 的 `{user_id,username,status}` account item，其中 `status` 为 `added|updated`；bundle JSON 固定为 `{accounts,default_user_id}` 并按输入 bundle 顺序逐项报告，不再暴露 `default`/`has_token` 或按 added/updated 分组。
- `auth import --file` 严格解码未加密 bundle，并按 UID merge 后原子保存：保留本地已有 default，仅空 store 采用 bundle default；该离线路径拒绝 token/proxy 组合，不刷新、不联网。bundle 是 point-in-time backup 而非 live sync，token rotation 后旧 bundle 与其他机器副本可能 stale。
- 有 refresh token 的搜索始终使用 App API，失败不回落 Web；无 token 的 Web 搜索只执行可靠筛选，R18/R18G/mature 与动态搜索选项会明确要求登录。分级与仅 AI 在 public SDK 筛选，其余新增筛选优先由 App 服务端执行；收藏数筛选仍不提供。
- Deprecated `pixiv search --r18` 现在只作为 `--rating r18` 的 alias，不再向关键词追加 `R-18`；它可与显式 `--rating r18` 同用，与其他显式 rating 冲突。

### Fixed

- 修复 Windows `install.cmd` 在 Release 使用 `core.autocrlf=false` checkout 时仍可借由 Git 属性保留 CRLF 并正常运行。
- 修复 Linux Release 在 Ubuntu 24.04 上通过 cgo 链接后隐式要求 `GLIBC_2.39`、导致 Debian 12 等
  glibc 2.35–2.38 系统无法启动的问题；后续 Linux amd64/arm64 资产统一在 Ubuntu 22.04 构建，
  release、native evidence 与 packaged smoke 在打包前检查 ELF 的 GNU version requirement，任何
  高于公开 `GLIBC_2.35` 基线的依赖都会 fail closed。
- 修复 auth bundle JSON decoder 因 Go struct field 的大小写宽松匹配而接受 `Schema`、`Default_User_ID`、`User_ID`、`Refresh_Token` 等非 canonical alias 的问题；顶层和 account object 现严格要求 exact canonical key，并拒绝 canonical/case alias 冲突。
- 修复 `--ai-type` 的帮助语义与 Pixiv 字段不一致；Pixiv `AIType==2` 现正确识别为 AI，deprecated `--ai-type` 保持 `0=exclude`、`1=only`、`2=all`，并拒绝与显式 `--ai-mode` 同用。
- 修复 legacy MCP handler 把输入、认证、上游读取、下载与资源失败转换为兼容文本后，统一 operation wrapper 仍误记 `result=success` 的问题；wire 继续保持原 Content、structured output、文本与 `isError=false`，stderr 现以 error level 和安全 typed metadata 记录真实失败；事件不记录或输出原始错误文本、tool 输入、query、凭据、URL、path 或 response body。正常空结果仍记为成功。
- 规范化下载 URL path 推导扩展名中的跨平台非法文件名字符、ASCII 控制字符和 Windows 非法尾随点/空格；单页与多页下载共用同一清理边界，既有模板和 ugoira `.gif` 语义不变。
- 修复 MCP 作品列表文本静默只显示前 5 个 tags 的问题；文本现按上游顺序完整输出全部 tags，structured output 不变。
- 修复 MCP `illust_ranking` 依赖 deprecated title-case 转换而产生不稳定英文标题的问题；已知 mode 现使用稳定中文标题，未来 mode 保留原值作为可读 fallback。
- 修复 App API、OAuth 与代理快照隐藏的 60 秒整请求 timeout 可能中断正常资源流，以及 Web/resource 裸用全局 `http.DefaultClient` 导致策略不一致的问题；public SDK 现在默认使用专用零 timeout client，由每次调用的 context 控制总生命周期，显式注入 client 的指针、timeout 与 transport 保持不变。
- 修复写回 `config.toml` 时原地截断可能损坏旧配置，以及 `auth.json` 原子替换前未同步文件的问题；两者现共用同目录临时文件、完整写入、file sync、关闭和原子替换流程。Unix-like 平台主动使用 `0700`/`0600`；替换提交后同步目标目录，首次创建目录时还按 leaf→root 同步每层新目录的外层 parent entry，并合并所有同步错误。Windows 使用 recovery backup 处理 `ReplaceFileW` 部分完成失败；公开替换错误会以 typed contract 告知资源下载、ugoira 发布、更新安装和 release cache 保留新 source，自动恢复也失败时因此保留 old backup 与 new source 两份材料。该路径已有状态模型、classifier 与交叉编译测试，但不声称在真实 Windows 文件系统注入 1177；首次创建继承父目录 ACL、替换既有目标保留其 ACL，不声称主动收紧 DACL 或提供 POSIX directory fsync。
- 修复 MCP `download` 与 `download_random_from_recommendation` 的参数、SDK、推荐、下载、结果整理或文件读取失败被 typed output schema 的 `null` 数组校验错误遮蔽的问题；失败现保留原业务文本与规范化 `delivery`，并返回空 `items`/`files` 数组。
- 修复 MCP `download_random_from_recommendation` 把显式 0、负数或大于 20 的 `count` 静默改写为默认值或边界值的问题；非法值现明确报错，省略时仍默认 5，同时传入非法 `delivery` 时仍优先返回 delivery 参数错误而非 schema 错误。
- 修复 MCP `refresh_token` 在 SDK 初始化、配置或代理失败时误报“未设置 refresh token”的问题；取消、超时和公开 SDK 错误现保留安全分类，未知初始化错误保持脱敏，未知刷新执行错误也不再回显原始错误详情。

### Security

- 安装脚本只使用固定官方 GitHub Release 来源，不接受自定义下载源；checksum 缺失、重复、格式异常、SHA-256 不匹配或暂存 binary 预检失败时均在替换前显式终止。默认用户级目录不提权，PATH 修改必须由 `--add-to-path` 明确请求，也不会读取 Pixiv 认证状态。
- 只有不带 `--output` 的 `auth export [UID]` 与 `auth export --all` 可向 stdout 输出 secret：前者只有 raw token 与换行，后者只有 versioned bundle。export 全程 local-only，不读取环境 token、不联网、不刷新、不修改状态，并跳过 startup update cleanup、自动更新与 operation log；其他 stdout/stderr、JSON、MCP、日志与错误继续禁止泄露 refresh token。
- `auth export --output` 使用独立 secret writer：Unix-like 文件为 `0600` 且不改变既有 parent；Windows 从创建时设置 owner 与 protected DACL，只授权当前用户、LocalSystem、builtin Administrators，并在 replacement 后重新应用。Windows policy 有 CI test 与后续本地交叉编译验收要求，不声称已在真实 Windows filesystem 运行。
- auth restore 写失败以 `LocalWriteCommitOutcome` 准确区分 pre-commit `not_committed`、replacement 后 durability/cleanup 失败 `committed` 与 unresolved recovery `unknown`，不把已提交或未知状态伪装为 rollback。
- 修复 native-evidence 六平台构建只向 runner 默认 Rust 安装 target、导致静态库实际由 movable
  toolchain 生成并与 release test/production 的 `1.96.0`/`1.96.1` provenance 不一致的问题；workflow
  现在逐目标固定版本、绑定 `RUSTUP_TOOLCHAIN` 并以 `--no-self-update` 安装，两个 verifier 共用同一
  fail-closed 映射。原 `1.97.0` 六库已由固定版本的真实六平台 run `29567721284` 成套重建并回填。
- 修复 macOS 登录回调 helper 以固定临时 Swift 源文件路径编译时可能遭受 symlink 覆盖或并发替换的问题；源码现在写入权限为 `0700` 的随机私有目录，并以独占方式创建为 `0600` 普通文件，编译成功或失败都会清理该目录，同时保留真实编译错误。
- 加固 understand-anything 图谱归一化对生成器输入路径的读取：Go scan 与 docs article 现在统一拒绝绝对路径、词法或 symlink 越界及非普通文件，并通过同一已打开文件描述符复核边界与文件身份；路径在校验期间被替换会显式失败，且不会写入四份图谱产物。
- 修复 ugoira ZIP 帧在进入 image decoder 限制前被 `read_to_end` 无界展开的问题；帧源现在以 pinned `image` crate 的默认 `max_alloc` 为同一客观上限，读取前校验 ZIP 声明大小，并在分块读取时校验实际累计字节和响应取消。超限、内存预留失败或取消都会显式失败，不截断或静默降级；image crate 内部正在执行的单帧 decode 仍只能在返回后观察取消。
- 修复 `pixiv config set/unset https_proxy` 在环境变量覆盖配置时把有效代理 URL 原样写入 stderr、可能暴露 userinfo、path 或 query 的问题；命令现在仍提示 effective value 由环境变量控制，但不再回显代理值。配置写入/删除、显式 `config get` 与小写 `https_proxy` 优先于大写 `HTTPS_PROXY` 的语义不变。
- 加固 Release workflow 的 canonical SemVer 门禁：validator 现在是绑定 `RELEASE_TAG` 的独立、单命令 step，workflow policy 固定其位置与精确命令，并拒绝 `if`、`continue-on-error` 或额外 shell command 绕过 tag push/恢复入口的版本校验。更新选择器既有的当前通道非 SemVer fail-closed 语义保持不变。
- 修复 SDK、CLI、MCP、显式更新与自动更新在代理 URL 格式错误或 update 代理不是 absolute HTTP(S) URL 时，错误、unwrap 链或 stderr warning 可能回显代理 userinfo、path 与 query 的问题；非法代理现在保留可分类的安全原因与静态上下文，并继续在联网前明确失败。有效 HTTP(S) 代理、显式空代理、`--no-proxy`、动态配置 snapshot 与代理优先级不变。
- 修复 Release workflow policy 在 GitHub expression 的单引号格式字符串含 `}` 或 `}}` 时可能漏检后续 `secrets` context 的问题；共享 scanner 现在按 expression 边界和单引号转义解析，确保 protected publish job 只能在受审计的签名 metadata step 引用发布签名 secret。

## [0.3.0] - 2026-07-15

### Changed

- Breaking: 公开 Go SDK 已迁移至 `github.com/FlanChanXwO/pixiv-cli/pixiv`；旧导入路径不保留兼容 package。
- Breaking: 认证入口只接受原始 Pixiv App API refresh token；网页 Cookie（包括 `refresh_token=...`）不再被解析、提取或转换。
- Breaking: `pixiv recommended` 现要求 `all|illust|manga|novel|user` kind；`all` 原子返回四类个性化推荐。

### Added

- MCP 新增 `recommended`：以必填 kind 返回插画、漫画、小说或作者推荐；`all` 以每流独立分页的 structured output 返回四类推荐。
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
[Unreleased]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.4.5...HEAD
[0.4.5]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.4.4...v0.4.5
[0.4.4]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.4.3...v0.4.4
[0.4.3]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.3.0...v0.4.2
[0.3.0]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/FlanChanXwO/pixiv-cli/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/FlanChanXwO/pixiv-cli/releases/tag/v0.1.0
