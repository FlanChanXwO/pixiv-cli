# 领域上下文

## 领域

本项目是 Pixiv CLI、MCP stdio server 与公开 Go SDK。它提供作品搜索、详情、排行、推荐、用户/收藏流程、下载、缩略图、本地账号/配置管理，以及面向正式二进制的版本与更新协议；不提供 HTTP Provider service。

## 架构词汇

- **CLI controller**：`internal/cli`；Cobra commands、flags、prompts、loopback browser interaction，以及 stdout/stderr presenter。
- **Account services**：`internal/account/{pixiv,fanbox}`；Pixiv/FANBOX 本地账号模型、登录完成与持久化端口。SDK operation 的一次性打开与账号池重放边界在 `internal/session`，逻辑分页遍历在 `internal/pagination`，配置 schema 与 immutable snapshot 在 `internal/storage/config`。
- **Composition root**：`internal/cli/cli.go` 的私有、per-run lazy resource graph，由 `internal/cli/root.go` 的命令树驱动；在解析与输入校验后按 typed requirements（`internal/cli/requirements`）组装 config snapshot、auth storage、Pixiv/FANBOX SDK client、OAuth client、download manager 与 update dependency，并逆序关闭资源。
- **Auth storage**：`internal/storage/database`；以 UID 为 key 的 SQLite `pixiv-cli.db`、migration ledger、私有路径与权限。旧 `auth.json` 不自动读取；跨版本迁移使用显式 `auth export --all --output` / `auth import < bundle.json` bundle。
- **Pixiv SDK 与协议适配**：`sdk/pixiv`（App-only）与 `sdk/fanbox` 提供唯一公开调用链；`internal/services/pixiv` 由 appapi transport、artwork/novel/user endpoint families、oauth、protocol 与 resource 子包封装上游协议；`internal/services/fanbox/protocol` 只提供 FANBOX 专属 session/cookie/challenge 与窄 transport，creator/post 父包和各 endpoint family 分离 normalized entity 与 wire conversion。v1 已删除 `webapi`、FANBOX wide service 与共享 model owner。
- **SDK client**：`sdk/pixiv.Client` / `sdk/fanbox.Client`；外部 Go 程序通过它们访问规范化 Pixiv/FANBOX 原子能力。
- **Caller adapter**：调用方自己的窄接口与业务层；它拥有 source mode、budget、filter、cursor 持久化、入库与调度。
- **Operation snapshot**：`pixiv.Open` 每个公开操作读取一次 auth/config/OAuth 快照；`Snapshot(ctx)` 可显式固定一个高层操作。
- **Opaque cursor**：SDK 绑定 operation/query/source 的版本化 continuation；CLI/MCP 不暴露它。
- **MCP server**：`internal/mcpserver/{pixiv,fanbox}` 是产品聚合根包，仅保留薄构造转发；每个 tool 一个独立 package（`internal/mcpserver/{pixiv,fanbox}/<tool>`），stdio transport 与 runtime lifecycle 归 `internal/mcpserver/stdio`。
- **Build information**：`internal/buildinfo`；Go linker 注入的 `version`、`commit`、`build_date`；`dev` 是不可自更新的开发构建。
- **Update domain**：`internal/update`；安装来源识别、GitHub Releases 查询/cache、SemVer channel、Homebrew/Go/Release 更新策略与 Release installer。
- **Release trust root**：Release installer 认可的 Ed25519 public key 与 key ID；私钥永不属于 CLI、源码、formula 或 archive。
- **Release asset**：固定名称的 `pixiv-cli_<version>_<os>_<arch>` archive，加上 `checksums.txt` 与签名 `checksums.json`。
- **Rust staticlib**：ugoira encoder 的 target 专用 cgo 输入；完整 manifest 绑定 source digest、六 target、path 与 SHA-256。
- **Utility packages**：`internal/utils/{parse,text,uri}`；本地状态路径/权限归 `internal/platform/localstate`，文件机制归 `internal/storage/file/{atomic,lock,replace,secret}`，下载 filename、本地获取编排与媒体 MIME 类型归 `internal/media/downloader`。
- **Infrastructure constants**：`internal/platform/localstate`（本地状态路径/权限）；按责任归属，避免通用 constants 包。

## 安装来源与更新通道

- **Development**：version 为 `dev` 的构建；不读取安装来源或网络，也不允许自更新。
- **Homebrew stable**：由 `pixiv-cli` keg/receipt 识别；目标是 stable GitHub Release。
- **Homebrew beta**：由 `pixiv-cli-beta` keg/receipt 识别；只有显式 `--prerelease` 保持 beta，切回 stable 时必须报告可能的卸载/安装/回滚结果。
- **Go install**：Go build info 与实际 `GOBIN`/`GOPATH/bin` 一致的二进制；升级使用精确 Git tag。
- **Release binary**：非上述来源的正式二进制；安装前必须验证签名 checksum、平台 archive 与下载 binary version，并原子替换目标文件。
- **Automatic check**：普通 CLI 成功后对 stable channel 的只读提示；24 小时节流、最多 3 秒、只写 stderr，绝不改变业务退出码或污染 JSON/MCP stdout。

这只是领域模型，不代表所有渠道已公开可用：受支持 binary 的 production Ed25519 public key、key ID
与 fingerprint 已随 [`internal/cli/cli.go`](internal/cli/cli.go) 提交；公开
source/tap remote、受保护 `release` Environment、私钥恢复副本与完整六目标 staticlib manifest 已按
Task 20 的审计流程配置或回填。v0.3.0 已发布为正式 Release，公开 tap 已有对应 stable formula；后续版本
仍必须完成相同的 tag、签名、资产和安装门禁，不能只因 trust root 存在就声称可安装。

## 边界规则

- `internal/cli` 的私有 graph 只拥有 production wiring 与生命周期；命令 owner 不得自行构造持久状态、Pixiv/download/update 网络依赖或签名信任根。
- `auth login` 的 loopback HTTP server、browser opening 和 terminal prompt 留在 CLI，因为它们是本地 UI adapter。
- `auth login` 可注册本地 macOS `pixiv://` URL handler，只转交最终 callback URL 给当前 CLI loopback；不得读取 cookie/token、自动化 browser UI、安装 extension 或伪造登录成功。
- `auth login` 不读取浏览器历史、会话文件、标签页、存储或网络流量，也不启动受管 Chromium、DevTools/CDP；helper 不可用时只能等待 loopback 或用户手动回填。
- `internal/account/pixiv.LoginService` 拥有 PKCE/state 创建、OAuth code exchange 与账号保存。
- `internal/bootstrap` 已随 v1 迁移删除，不再是 CLI/MCP composition root；`internal/cli/cli.go` 的私有 graph 为 Release installer 注入已提交的
  production public key/key ID，私钥仍只应存在于受保护 `release` Environment secret 或受控 macOS
  Keychain 恢复副本。
- `internal/storage/config` 只处理 `config.toml` schema、default、effective value、immutable snapshot 与 sparse write；`[update].check_enabled` 只控制自动检查，且只可手工维护 TOML。
- `internal/update` 负责来源与更新策略，但不得把权限、HTTP、asset、签名、checksum、archive 或替换错误伪装成无更新。
- `internal/media/ugoira` 的生产编码路径使用 Rust staticlib，不得在 runtime 回退 `ffmpeg`；完整六目标 manifest 是 source/release 可用的前置条件。
- `sdk/pixiv` 与 `sdk/fanbox` 是公开 SDK；内部协议实现物理拆分为 appapi transport、Pixiv endpoint families、oauth、resource 与 FANBOX adapter，不得反向 import public package。
- 没有匿名 Web fallback：App API 是唯一 Pixiv 内容路径，失败直接返回规范化错误，不自动切换协议；已删除的 `web_fallback_enabled` 若仍显式存在则返回 `removed_setting`。
- 不新增 Discover、Probe、Capabilities、RSS、crawler、通用 Provider interface 或 HTTP server；这些属于调用方 adapter。
- 不再保留通用 `internal/common/constants`；`AppDataDirName` 与文件权限归 `internal/platform/localstate`。
- CLI/MCP/OAuth loopback adapter helper 留在 adapter package，除非它们是 protocol-free parsing helper。

## 行为约束

- **Local account**：以 Pixiv UID 为 key 的已保存身份，含 refresh token 与可选 username。
- CLI data command 使用 `auth use` 的 local account，或在启用 `[account_pool]` 后使用 `pixiv-cli.db` 中可调度的账号；不接受 `--uid`/`--refresh-token`。public SDK 必须由调用方显式传入 credential。
- MCP credential selection 由其 runtime config 决定；不得把 CLI 的已移除 flag priority 套用于 MCP。
- Runtime proxy priority：Pixiv command override > `[pixiv.network].proxy_url` > `https_proxy`/`HTTPS_PROXY` > `[network].https_proxy`；FANBOX 使用独立 `[fanbox.network]` 设置，FlareSolverr 只使用显式 solver 配置。CLI proxy flag 只影响本次网络命令，绝不持久化。
- JSON/text output shape 应保持稳定；refresh token、Ed25519 private key 与 tap deploy key 绝不打印。
- OAuth URL callback 必须校验 `state`；Pixiv official callback/code input 与 authorization relay URL 是 browser flow 未到达 loopback 时的显式 fallback。
- 不得新增无依据 timeout、truncation、retry、item limit、silent fallback 或 hidden downgrade；自动更新的 24 小时/3 秒是既定产品约束，只适用于自动检查。
