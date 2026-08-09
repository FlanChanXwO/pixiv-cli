# v1.0.0 计划与验收资料

本目录是 v1.0.0 SDK、CLI、MCP、认证、网络、FANBOX、native 和发布门禁的唯一计划资料入口。
产品配置仍使用 TOML；YAML 只用于 workflow/tooling，不属于运行时配置格式。

## 主题路由

- 目标与公开 SDK：`design.md`、`public-sdk.md`、`sdk-package-layout-versioning.md`、`sdk-usage-pseudocode.md`、`pixivpy-parity.md`。
- CLI、MCP、下载与 web API：`cli-mcp-download.md`、`web-api-removal.md`、`strict-cli-argument-parsing.md`。
- 认证、浏览器与数据库：`auth-browser-storage.md`、`authdb-design-review-2026-08-04.md`、`account-pool-scheduling.md`。
- 网络与 FANBOX challenge：`network-routing.md`、`fanbox-challenge-routing.md`。
- 实施与架构：`implementation-plan.md`、`architecture-reorganization-plan.md`、`rc-follow-up-index.md`、`rc-follow-up-implementation-plan.md`。
- 发布与环境：`environment-readiness.md`、`verification-release.md`、`release-prep-runbook.md`。
- 过程记录与证据：`session-status-2026-08-03.md`、`debug-diagnostics.md`、`code-review-2026-08-08.md`、`final-verification-2026-08-08.md`。

## 当前执行状态

- SDK、认证契约、账号池、网络路由、FANBOX challenge、诊断、记录、下载器/native 路径和文档同步已在本 worktree 完成回归。
- `architecture-reorganization-plan.md` 的内部重组已完成本地代码、LSP、测试、构建、release-path 与文档审计；其验证记录见该计划第 8 节及 [最终验证记录](final-verification-2026-08-08.md)。
- RC-11 的真实 Pixiv public SDK 与一次性 real-solver evidence 已在 2026-08-08 的授权环境通过；本轮又取得两个指定 FANBOX target 的 headed browser `post.info`/资源 evidence，并修正 Edge modern Chromium cookie 解密；Go SDK direct transport 的真实 FANBOX 闭环与 credential-free `browser-evidence.yml` 六目标 runner 仍作为独立发布门禁，不以单一 host evidence 或离线测试冒充全平台通过。
- 旧 `auth.json` 不再自动读取或迁移。跨版本迁移必须由旧 CLI 执行
  `pixiv auth export --all --output <private bundle>`，再由新 CLI 执行
  `pixiv auth import --file <bundle>`。
- `account_pool.accounts` 到 `internal/persistence/authdb` 的数据库状态迁移仍是启动时一次性、幂等且可观测的兼容流程；历史 `data/account-pool.json` scheduler 不读取、不迁移、不删除。

## 完成门禁

完成前必须在合法 worktree 记录以下本地门禁：

```text
GOPROXY=off go test ./... -count=1
GOPROXY=off go test -race ./... -count=1
GOPROXY=off go vet ./...
sh scripts/build.sh
GOPROXY=off go test ./scripts/documentation -count=1
GOPROXY=off pre-commit run --all-files
git diff --check
```

需要真实凭据、真实浏览器 profile、FlareSolverr 容器或 Rust staticlib 的门禁必须保留为明确的 PASS、SKIP（含原因）或 FAIL，不能用无凭据的本地测试冒充真实 evidence。
