# 任务清单

每个任务完成后填写“实际/证据/风险”。每三个实现任务后执行集中检查。

## T01 — 建立隔离 worktree 与干净基线

- 状态：完成
- 实际：创建 `codex/goal-setup` 元数据分支，提交 goal 文件与 `.worktrees/` ignore；在 `.worktrees/v030` 创建 `codex/v030` 隔离工作区。
- 证据：Go 1.26.3；`go test ./...` 退出成功。
- 风险/下一步：基线来自 main 的 v0.2.0 后续提交；下一任务只修改 recovery test overlay，保持 v0.2.0 tag 不变。

## T02 — 修复 v0.2.0 Windows recovery 测试门并扩展白名单

- 状态：完成
- 实际：Windows 下把 config 私有文件 mode 断言改为既有 ACL `0666` 表示，Unix 保持 `0600`；将精确测试文件加入 workflow 与 canonical policy 的 archive/diff allowlist；新增遗漏/额外路径拒绝测试，并修正文档计数。
- 证据：提交 `9a41124`、`8c5739f`；`go test ./pkg/pixiv ./scripts/releaseworkflow`、`sh scripts/test-release-workflow.sh`、`git diff --check 3d59741..HEAD` 均通过；规格与质量复审均批准。
- 风险/下一步：本机非 Windows，实际 Windows gate 留待恢复 workflow 的原生 runner 证实；下一任务只审查并触发 v0.2.0 recovery，不移动 tag。

## T03 — 审查 recovery 修改并 dispatch/验收 v0.2.0 Release

- 状态：待修复
- 实际：recovery PR #4 与 overlay 修复 PR #5 已合并至 main；T03b 对齐 Environment 后，run `29307406643` 完成并成功公开 v0.2.0 Release、四平台验证 Homebrew formula 与 tap 部署。发布产物、tag 与公式均已验收，但实际 CLI `update --check --json` 仍被 GitHub API 403 拒绝。
- 证据：`29304765177` 失败由 17/4 overlay 差异引起，已由 T03a 修复；`29306391898` 的 publish job 由 Environment branch policy 拒绝，已由 T03b 修复。`29307406643` 的 validate、六个 build、六个 build_production、release publish、四个 Homebrew verify 与 tap deploy 均为 success；Release `v0.2.0` 为非 draft/non-prerelease，含六平台 archive 与 `checksums.{txt,json}`，tag commit 仍为 `329711121588d9f054fb3d15540bb0fd6c134e42`。`homebrew-tap` 的 `Formula/pixiv-cli.rb` version/URL/SHA 与 Release 对齐，commit `710f997`。但以 v0.2.0 buildinfo 运行 `pixiv update --check --json` 得 `GitHub Releases ... HTTP 403 Forbidden`；同端点带 User-Agent curl 返回 HTTP 200、匿名配额剩余 57，且 `internal/update/releases.go` 的请求仅设 ETag，未设 User-Agent。
- 风险/下一步：新增 T03c，补齐 GitHub Releases 请求 User-Agent 的聚焦回归与实现；修复后需要完整 CI 和对已发布 v0.2.0 的 update-check 兼容性策略评估，不能重写 immutable tag 或 Release。

## T03a — 收敛 recovery overlay 至实际最小审计差异并复验 policy

- 状态：完成
- 实际：将 test-only recovery overlay 的 archive、工作树 diff 断言和 Go canonical verifier 同步收敛为 tag 与当前默认分支实际不同的四条路径；移除遗留的两条 `git add -N`，并把聚焦策略测试改为精确四路径命令、逐条缺失和额外路径拒绝。
- 证据：提交 `59105ed`；TDD RED `go test ./scripts/releaseworkflow -run '^TestCheckRecoveryPolicyRequiresExactFourPathOverlay$' -count=1` 在旧 17 条命令下失败，GREEN 后通过；`go test ./scripts/releaseworkflow -count=1`、`sh scripts/test-release-workflow.sh`、`go test ./...`、`git diff --check origin/main..HEAD` 和 pre-commit 均通过。临时 detached `v0.2.0` worktree 从 `origin/main` archive 四条路径后，`git diff --name-only` 精确等于该集合且 cached diff 为空。规格审查与质量审查均批准。
- 风险/下一步：仍需把修复推送、通过六平台 PR CI，并从默认分支重新 dispatch `release_tag=v0.2.0`；tag 保持不可变，生产构建隔离仍待远端实际 run 验证。

## T03b — 对齐 protected release Environment 与受审计 main recovery dispatch

- 状态：完成
- 实际：在 GitHub `release` Environment 的 custom deployment branch policies 中新增精确 `main` branch 规则；保留既有 `v*` tag 规则、required reviewer 及 `prevent_self_review=false`，未更改 secrets、环境名或 workflow。
- 证据：GitHub API 创建 policy `54575656`（`main` / `branch`）；复查显示 policy 集合仅为 `main` branch 与 `v*` tag，Environment 仍含 required_reviewers 和 branch_policy protection rules；`main` 的 GitHub branch 元数据为 `protected=true`。
- 风险/下一步：需重新从 main dispatch immutable `v0.2.0`；publish 应进入 required reviewer gate，而非 runner 前 branch policy 拒绝。随后验收签名 Release、Homebrew 与更新检查；不得删除这两条 policy 或绕过 reviewer。

## T03c — 修复 GitHub Releases update check 的 User-Agent 兼容性

- 状态：完成
- 实际：给 GitHub Releases 每个分页请求加入固定、非敏感 `User-Agent: pixiv-cli`，保持 ETag、缓存、分页、proxy、timeout、错误和 fallback 语义不变；新增经公开 `GitHubReleaseClient.Check` 的首页/next-page 回归，并更新 Unreleased changelog。
- 证据：提交 `3b46b9f`；TDD RED 显示默认 Go User-Agent/403，GREEN `go test ./internal/update -run '^TestGitHubReleaseClientUsesStableUserAgentForEveryReleasePage$' -count=1`、`go test ./internal/update -count=1`、`go test ./internal/cli -count=1`、`go test ./...`、pre-commit 和 diff check 全通过；规格与质量审查批准。将正式 v0.2.0 buildinfo 编入临时 binary，在隔离 HOME/cache 下连续两次 `pixiv update --check --json` 均返回 source release、latest `v0.2.0`、`update_available=false`，第二次覆盖 ETag cache 重验证。
- 风险/下一步：不可变 v0.2.0 资产不包含该源码修复；必须把修复经 PR CI 合并，使后续 v0.3.0 包含。T03 的 release/Homebrew/tag 已完成，更新修复的公开分发留待 v0.3.0 Release。

## C01 — 集中检查：恢复链路、tag 不变性、文档与外部发布证据

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T04 — 将公开 SDK 迁移到顶层 pixiv 并更新全仓 import

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T05 — 集中协议 profile、endpoint catalog 与脱敏 adapter failure

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T06 — 扩展 User Detail 正规模型与 SDK contract

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## C02 — 集中检查：SDK 路径、协议边界、公开模型稳定性

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T07 — 新增 CLI user detail 与 SDK 单链路调用

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T08 — 新增 MCP user_detail、更新 tool skill 与文档

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T09 — 新增小说/作者/漫画推荐 SDK 与稳定模型

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## C03 — 集中检查：用户详情和推荐模型、认证、错误与分页

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T10 — 改造 CLI recommended 子命令与原子输出

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T11 — 新增 MCP recommended(kind) 并保留旧 tool 兼容

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T12 — 迁移剩余 CLI/MCP/download 到 SDK，删除 legacy Source 双栈

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## C04 — 集中检查：能力矩阵、架构导入门禁、完整回归

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T13 — 同步 README、MCP 文档、ADR、CHANGELOG 和知识图谱

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T14 — v0.3.0 Release 候选门禁与 opt-in API canary

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## T15 — 创建并发布不可变 v0.3.0，验收 Release/Homebrew/更新

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

## C05 — 终审

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：
