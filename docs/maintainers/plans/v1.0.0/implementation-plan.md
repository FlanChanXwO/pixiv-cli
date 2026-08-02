# v1.0.0 实施顺序与检查点

本文件把设计分卷转换为可执行顺序；契约细节仍以 [总设计](design.md) 及其分卷为准。每个阶段保持
仓库可构建、补齐聚焦测试并完成 LSP diagnostics 后才能进入下一阶段。正式实施不得直接在 `main`
工作树进行。

## 0. 隔离工作区与冻结基线

- 将整个 `docs/maintainers/plans/v1.0.0/` 纳入实施分支，使独立 worktree 能读取同一计划。
- 从当前 `main` 创建 `codex/v1-sdk-rewrite` 独立 worktree；不复用已有 `codex/fanbox` worktree。
- 记录 `go test ./...`、race、vet、build、pre-commit、LSP diagnostics 的干净基线。
- 用 LSP blast-radius 记录旧 `pixiv/`、webapi、auth storage、CLI/MCP public caller，作为删除检查输入。

检查点：工作树只包含计划文件和明确属于 v1 的改动；基线门禁无失败。

## 1. 共享 `sdk` 基础契约

- 新增 `sdk` package，实现 `Page[T]`、`Cursor`、`Error`、`ResourceRef`、`Resource` 与资源 request/
  response/save 类型。
- 先完成 Text/JSON codec、错误链、context、脱敏、cursor binding 与资源 header/URL policy 测试。
- 增加 public inventory 与英文 GoDoc 检查，但此阶段不删除旧 `pixiv/`。

检查点：`sdk` 不 import 产品模型或本地状态；新旧代码并存仅用于实施过渡，不构成 v1 compatibility
承诺。

## 2. Pixiv App-only SDK

- 在 `sdk/pixiv` 实现构造器、OAuth `LoginSession`、credentials rotation、模型、PixivPy parity
  operation、URL reference、ugoira 与资源读取。
- 复用并收敛 `internal/services/pixiv/appapi|oauth|resource`，不复制 protocol DTO 到公开模型。
- 先让 `sdk/pixiv` external contract tests 全部通过，再切换 `internal/application` caller。

检查点：全部 Pixiv operation 只走 App API；无 token 明确 `unauthorized`；尚未删除的旧 package 不再
承载新增功能。

## 3. SQLite 鉴权与配置迁移

- 新增 `internal/storage/authdb` 与 embedded `migrations/*.sql`，实现 schema、权限、repository 和
  `auth.json` 一次性迁移。
- 将 Pixiv rotation、默认账号与 account-pool selection 切换到 DB/config 契约。
- 完成 crash/re-entry、checksum、并发、权限、config 跨文件失败和 secret-output tests。

检查点：逻辑导入对比通过后才删除 legacy secret；任何失败都显露 commit outcome。

## 4. FANBOX SDK

- 以 `codex/fanbox` worktree 只读参考协议知识和 fixtures，在 `internal/services/fanbox` 与
  `sdk/fanbox` 重新实现 session、creator/tag/post/home/supporting、pagination 与资源模型。
- 覆盖受限帖、article block、cover/image/file、第三方 embed metadata、challenge 与 session expiry。
- FANBOX SDK 不读取浏览器、DB 或 Pixiv credentials，也不 import `sdk/pixiv`。

检查点：公开 inventory、英文 GoDoc、fixture contracts、资源安全和脱敏测试通过。

## 5. 浏览器 Cookie provider

- 实现 `internal/platform/browsercookies` core、Chromium、Firefox、Safari 与各 OS secret backend。
- FANBOX adapter 只查询 `fanbox.cc` 所需 session，保存前使用 `sdk/fanbox.Client` 验证身份。
- 先完成 synthetic fixtures，再在可用的 native host 验证浏览器运行锁、snapshot 与系统凭据边界。
- Firefox 不要求预装到开发机。维护脱敏的真实 schema fixture，并在最终 native job 中临时解包固定
  Firefox 版本、创建隔离 profile、运行 provider contract 后清理。

检查点：macOS Chrome/Edge/Firefox/Safari、Windows Chrome/Edge/Firefox、Linux Chrome/Edge/Firefox
均有 native CI evidence；fixture 与临时 Firefox job 可以共同提供 evidence，但不能用 Chromium
provider 的成功替代 Firefox provider。

## 6. Application、CLI、MCP 与下载编排

- 分别实现 `internal/application/pixiv|fanbox`，让 CLI/MCP 只调用对应公开 SDK。
- 增加 FANBOX auth/creators/posts/tags/home/supporting/download 与独立 MCP server。
- 把高级下载、archive、sidecar、progress、atomic destination 放入 internal downloader；公共 SDK
  只保留单资源操作。

检查点：CLI text/JSON/NDJSON、stdout isolation、两个 MCP registry、账号选择和完整成功边界通过。

## 7. 最终切换与删除

- 用 LSP references 确认生产 caller 已切换后删除旧顶层 `pixiv/`、`internal/services/pixiv/webapi`、
  Web route/fallback/backend/base URL、旧 storage 和废弃测试。
- 保留配置迁移墓碑、`internal/services/pixiv/resource` 和纯本地 URL parser 对应的新实现。
- 运行 import/package golden，证明公开面只有 `sdk`、`sdk/pixiv`、`sdk/fanbox`。

检查点：不存在 dormant compatibility layer、隐藏 fallback、internal adapter 直连或旧 package import。

## 8. 文档、迁移与兼容冻结

- 先更新 canonical English SDK/CLI/MCP contract，再同步现有中文、日文入口与产品 Skill。
- 完成 v0→v1 import/symbol/setting/auth migration guide、release notes、最终 architecture ADR 和
  development guide。
- 冻结 v1.0 public inventory；beta/RC 的每次公开变更都重新运行 compatibility review。

检查点：文档链接、locale 语义、GoDoc、examples、apidiff baseline 和 workflow policy 全部通过。

## 9. 真实 E2E 与发布准备

- 严格按 [最终验证操作手册](release-prep-runbook.md) 执行并生成脱敏 evidence；开工阶段不提前探测
  凭据联网有效性。
- 从本机 `pixiv-cli` 私有状态取得授权 RFT，完成一次真实 Pixiv SDK identity/detail/resource 读取并
  安全持久化 rotation。
- 从 macOS Keychain 取得授权 FANBOX session，完成 ValidateSession、detail/list 与 Resource 读取。
- 启动一次固定 digest、loopback-only 的本机 FlareSolverr Docker 容器，单独记录辅助 evidence；不得
  替代 SDK 直连结果。
- 运行最终 full test、race、vet、build、pre-commit、docs、workflow、native browser 与 API
  compatibility gates。

检查点：所有 [发布条件](verification-release.md#v100-发布条件) 满足后才能创建不可变 v1.0.0 tag。

## 停止条件

遇到以下情况立即停止当前阶段并显露真实原因：公开契约存在未决选择；migration 无法证明数据一致；
凭据 rotation 无法安全提交；LSP 显示未知生产 caller；测试需要静默 fallback/固定无据限制才能通过；
真实 E2E 只能依赖 FlareSolverr 成功；或 native 平台 evidence 缺失。不得用后续阶段掩盖前一检查点失败。
