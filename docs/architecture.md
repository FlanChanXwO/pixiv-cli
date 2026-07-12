# 架构说明

## 总体流程

`cmd/pixiv/main.go` 是唯一官方二进制入口，它只负责调用 `internal/cli`：

1. `pixiv` 无参数显示 CLI 帮助。
2. `pixiv auth/config/version/update/search/detail/ranking/recommended/download` 进入 CLI 模式。
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
- `ArtworkService`：search/detail/ranking/recommended。
- `DownloadService`：按 ID 下载作品。
- `LoginService`：生成 PKCE/state、authorization-code exchange，并保存账号；Pixiv 登录 URL 构造仍留在 CLI adapter。

### `internal/bootstrap`

生产 composition root，负责把 `internal/config`、`internal/storage/auth`、`internal/pixiv`、`internal/download`、`internal/mcpserver`、更新 release client/installer 和 application services 组装起来。测试可以替换 service 里的小接口或 factory，不需要复制生产 wiring。

`NewUpdateCoordinator` 通过 `productionReleaseInstallerOptions` 为 Release installer 注入随受支持
binary 提交的 Ed25519 key ID→public key 映射，并在每次组装时复制 map 与 key bytes，避免调用方污染
production trust root。该公开 key 的 SPKI fingerprint 与已知签名 fixture 由同包测试验证；私钥不在
bootstrap、源码或运行时配置中。只读更新检查不需要该 key，且该 wiring 不代表存在一个可安装的公开
Release。

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

该包不得把签名、checksum、HTTP、archive、替换或权限错误伪装成“无更新”。当前 production
trusted key 为空，故 Release 安装的失败语义是保护边界，而不是临时降级。

### `internal/pixiv`

Pixiv 领域 facade。对 CLI/MCP 暴露稳定的 `Source`、`NewSource`、`NewOAuthClient`、HTTP client wiring 和常用模型 type alias。

source 策略只有一条：refresh token 为空且 `web_fallback_enabled=true` 时，`search/detail/ranking/search_user/download/ugoira metadata` 使用 web；只要存在 refresh token，就优先 app API，app API 的认证、网络或服务端错误不会自动 fallback。

### `internal/pixiv/api`

封装 Pixiv app API、OAuth refresh flow 和 authorization-code token exchange。当前实现使用 `resty` 作为底层 HTTP transport，主要职责：

- 保存 refresh token、access token、user ID 和可从认证/用户详情接口获取到的 username。
- 用 Pixiv Android app 风格 header 访问 API。
- 在认证错误时 refresh token 后只重放一次原请求。
- 将非 2xx 响应暴露为 `APIError`，保留状态码和响应体。

当前已实现接口包括搜索作品、作品详情、相关作品、排行榜、用户搜索、推荐、热门标签、关注动态、用户收藏、用户关注、ugoira metadata 和直接下载 URL。

### `internal/pixiv/web`

封装匿名 Pixiv web/ajax API。它复用 CLI/MCP 的 HTTP proxy 配置，当前用于无 refresh token fallback：

- `/ajax/search/artworks/{word}`：匿名作品搜索。
- `/ajax/illust/{id}` 与 `/ajax/illust/{id}/pages`：作品详情和原图 URL。
- `ranking.php?format=json`：排行榜。
- `/ajax/illust/{id}/ugoira_meta`：ugoira zip 与 frames。
- pximg 下载时使用 Pixiv web Referer。

web API 字段缺失时不伪造 App API 数据；仅映射可从 web 响应确认的字段。

### `internal/pixiv/model`

集中 Pixiv response/domain 类型以及 Pixiv 协议枚举 typed const，例如 search target、sort、ranking mode、restrict 和 illust type。MCP delivery 等传输层常量仍留在 `internal/mcpserver`。

### `internal/mcpserver`

负责将 Pixiv 与下载能力注册为 MCP tools。它定义了较窄的 `PixivAPI` 和 `DownloadManager` interface，便于测试和隔离；stdio runtime 由 `internal/bootstrap` 组装和启动。

输出目前以中文文本为主，适合直接返回给 LLM/MCP 客户端。认证相关工具会显式提示缺少 token、认证失败或自动认证失败的真实原因。

### `internal/download`

负责下载和本地文件落盘：

- `Download` 会同步下载 ID 列表，并返回每个作品的实际产物路径。
- `Enqueue` 会去重、排序并为每个 ID 启动后台任务。
- 内部 semaphore 当前并发为 5。
- 单页作品保存到下载目录。
- 多页作品和 ugoira 会建立作品子目录。
- ugoira 先下载 zip，再由 Rust FFI encoder 合成为 GIF/APNG。

Rust crate 以 target 专用 staticlib 接入 cgo：darwin/linux/windows 各有 amd64/arm64 selector。受支持
的 release/source build 必须从同一 Rust source digest 的六目标 `manifest.json` 选择并链接真实库；
无 cgo、无 target library 或无 C linker 时应在编译/构建期明确失败，不能回退到 `ffmpeg` 或 runtime
stub。Rust `target/` 是机器产物，staticlib/manifest 是经过 native 验证后才可提交的发布输入。

当前仅有 Darwin/arm64 library，完整 manifest 尚缺；因此这一架构约束已经由构建脚本实施，但尚未得到
六平台 native runner 证明。`ffmpeg` 仅保留给显式启用的开发质量对照，不在生产下载路径中。

## Release assets 与信任边界

`scripts/releaseassets` 以固定六目标封装 archive：darwin/linux 为 `.tar.gz`，Windows 为 `.zip`；
每个 archive 包含一个 `pixiv`/`pixiv.exe`、`LICENSE`、`THIRD_PARTY_LICENSES.md` 与完整
`third_party/licenses`。finalize 阶段收集这六个 archive 的 SHA-256 到 `checksums.txt`，并为原始
checksum bytes 生成带 key ID 的 Ed25519 `checksums.json`。

`.github/workflows/release.yml` 将签名/发布放在受保护的 `release` Environment 中；它使用最小权限和
full-SHA Actions，并在草稿 Release 上传后核对 asset 集合才发布。文件和本地 fixture 已存在，但尚未
部署 production signing 私钥、Environment、remote 或实际 GitHub runner 运行；受支持 binary 的公开
trust root 已在 `internal/bootstrap/release_trust.go` 配置。同时 staticlib/manifest、workflow policy 和
native artifact 证据仍是正式发布阻断项。Rust crates.io 依赖已由 crate 内 source replacement 固定到完整
vendor 闭包，并以空 Cargo cache 离线 metadata/build/test 与六 target 许可证检查验证。

Homebrew formula 模板由已验证的六目标 `checksums.txt` 生成，仅使用 macOS/Linux asset；stable
`pixiv-cli` 与 beta `pixiv-cli-beta` 相互冲突且同装 `pixiv`。tap credential 与发布 key 是不同的
信任域：tap 私钥只允许进入受保护 `release` Environment，不能代替 Release Ed25519 trust root。当前
没有 tap 或可安装 formula。

v0.1.0 的 archive 不计划包含 Apple notarization 或 Windows Authenticode。用户收到 Gatekeeper 或
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
- refresh token 输入可从包含 `refresh_token=...` 的 Cookie 字符串中提取真实 token。

`internal/utils/*` 子包提供无业务语义的通用工具：

- `files`：用户配置路径拼接与私有文件写入。
- `text`：字符串默认值和首个非空值。
- `uri`：URL path 提取与 file URI 生成。
- `media`：按文件扩展名推断基础 MIME type。
- `parse`：通用正整数解析。

## 已知约束

- `internal/pixiv/api.Client` 默认 HTTP timeout 为 60 秒，`internal/pixiv` facade 创建带代理的 HTTP client 时也保留该客户端级保护。
- `pixiv mcp` 是 MCP stdio server 的显式启动方式；直接执行 `pixiv` 不会启动 MCP。
- CLI 账号文件以明文 JSON 保存 refresh token、user ID 和可选 username，不保存 access token，文件权限固定为 `0600`；需要系统钥匙串时再扩展。
- `config.toml` 采用稀疏写入，不会把默认值整份落盘。
- `download_random_from_recommendation` 默认下载 5 个，当前代码将输入数量限制在最多 20 个。
- `download` 默认只返回本地路径和 `file://` URI；当 `delivery=image_content` 时，会把所有下载产物作为 MCP `ImageContent` 一并返回，不做无依据截断。
- `get_thumbnail_base64` 会将缩略图完整编码为 base64 文本返回，调用方需注意输出体积。
- 匿名 `search_user` fallback 语义是“作品搜索结果中的相关作者去重”，不是 Pixiv 官方用户名搜索。
