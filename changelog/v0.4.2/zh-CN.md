# v0.4.2 — 2026-07-19

## 新增

- 新增 `scripts/install.sh` 与不依赖 PowerShell 的 `scripts/install.cmd`：自动选择最新 stable Release 的当前 OS/arch archive，先验证发布 SHA-256 和暂存 binary，再执行无管理员权限的用户级安装；Release 以固定名称发布两个脚本并把它们纳入签名 checksum 集合，现有 locale 的 README 同步提供可复制的人类命令与 Coding Agent 安装 prompt。
- 新增 `pixiv auth import [REFRESH_TOKEN]` 与 `pixiv auth export [UID] [--all] [--output PATH] [--force]`：支持隐藏 TTY/raw stdin direct token import、单账号 raw export、全部账号 versioned bundle export，以及 `--file PATH|-` 离线原子 restore。
- 公开 Go SDK 新增 `AuthExportSelection`、versioned auth bundle model/strict codec、`ExportAuthBundle`、`RestoreAuthBundle`、`AuthRestoreResult` 与 `LocalWriteCommitOutcome`，供调用方实现 point-in-time secret backup 与可分类的离线恢复。
- 插画搜索新增稳定的分级、作品类型、AI、横纵比、分辨率与绘图工具筛选；CLI 新增 `--ai-mode`、`--aspect-ratio`、`--resolution` 与 `--tool`，`--type` 支持 `illust-and-ugoira`/`manga` 并保留 `comics` alias。SDK 新增 `SearchIllustFilters`、`SearchIllustOptions` 与 `Illust.Tools`；需 App 认证的 `pixiv search-options WORD` 动态列出绘图工具，不引入收藏数或 Cookie 筛选。
- MCP `search_illust` 新增同样的六个筛选字段，并新增需认证、返回 `{tools,text}` 的 `search_illust_options`；公开 SDK 的 `upstream_unavailable` 与本地 snapshot 错误新增安全的 transport/local-state 分类，诊断不暴露 URL、主机、证书或凭据。
- 新增面向 coding agent、全英文的 `pixiv-cli` skill；README 扩展为英文、简体中文与日语入口，公共文档按 `docs/<locale>/` 组织，架构、开发、ADR 与 Agent 规则集中到 `docs/maintainers/`。

## 变更

- Breaking: 删除 `pixiv auth add`、`pixiv auth token` 与 `--token`，不保留 alias/stub；direct token 入口统一为 `auth import`，显式 secret stdout 统一为不带 `--output` 的 `auth export [UID]` 或 `auth export --all`。
- `auth import` 的 direct 与 bundle 成功报告统一使用不含 secret 的 `{user_id,username,status}`；bundle JSON 固定为 `{accounts,default_user_id}`。`auth import --file` 严格解码未加密 bundle、按 UID merge 并原子保存，保留本地已有 default，仅空 store 采用 bundle default；拒绝 token/proxy 组合，不刷新、不联网。
- 有 refresh token 的搜索始终使用 App API，失败不回落 Web；无 token 的 Web 搜索只执行可靠筛选，R18/R18G/mature 与动态搜索选项明确要求登录。分级与仅 AI 在 SDK 筛选，其余新增筛选优先由 App 服务端执行；不提供收藏数筛选。
- Deprecated `pixiv search --r18` 在本版本仅作为 `--rating r18` 的 alias，不再向关键词追加 `R-18`；可与相同显式 rating 同用，与其他 rating 冲突。

## 修复

- 修复 Windows `install.cmd` 在 `core.autocrlf=false` checkout 时的 CRLF 问题；Linux amd64/arm64 后续统一在 Ubuntu 22.04 构建，并在 release、native evidence 与 packaged smoke 中检查 ELF GNU version requirement，任何高于公开 `GLIBC_2.35` 基线的依赖均 fail closed。
- 严格限制 auth bundle JSON 只接受 canonical key；修复 `AIType==2` 的 AI 语义；统一 legacy MCP 的脱敏 operation diagnostics；清理下载 URL 衍生扩展名；完整输出 MCP tags；稳定已知排行榜 mode 标题。
- 移除 App API/OAuth/代理快照中的隐藏 60 秒请求 timeout，改由调用 context 控制；统一专用 transport；加固 `config.toml`/`auth.json` 原子写入、同步、权限/ACL 与 typed commit outcome。
- MCP 下载失败保留原业务错误并返回规范空数组；随机下载不再静默改写无效 count；refresh-token 初始化与执行失败保留安全分类。

## 安全

- 安装器只使用固定官方 GitHub Release 来源，checksum 缺失、重复、格式异常、SHA-256 不匹配或暂存 binary 预检失败均在替换前终止；默认用户级安装不提权，PATH 修改必须显式请求，且不读取 Pixiv 认证状态。
- 只有不带 `--output` 的 `auth export [UID]` 与 `auth export --all` 可以输出 secret；export 全程 local-only，文件导出使用独立 secret writer，restore 精确报告 `not_committed`、`committed` 或 `unknown`。
- 加固 native-evidence toolchain provenance、macOS OAuth helper 临时源码、图谱输入路径、ugoira ZIP 帧内存上限、代理错误脱敏，以及 Release workflow SemVer 与 secret-reference policy 解析。
