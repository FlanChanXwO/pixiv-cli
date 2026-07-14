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
- 实际：recovery PR #4 与 overlay 修复 PR #5 已合并至 main。run `29306391898` 已在六个原生 test job 通过 overlay、Go/race/vet/package/pre-commit gate，并在六个隔离 production job 从 immutable tag 成功重建资产；但 publish job 在 runner 分配前被 GitHub Environment branch policy 拒绝，未创建 Release。
- 证据：`29304765177` 失败由 17/4 overlay 断言差异引起，已由 T03a 修复；PR #5 的 quality 与六平台 CI 全绿。run `29306391898` 的 validate、六个 build 和六个 build_production 均成功；`Sign and publish the GitHub Release`（job `87002181764`）无 steps、无 runner、failure。GitHub API 显示 `release` environment custom branch policy 唯一为 tag `v*`，而 workflow dispatch 的安全约束要求 `main`，形成确定性冲突。
- 风险/下一步：新增 T03b，以最小 Environment 配置变更允许受保护 `main` 进入已有人审的 release environment；随后重新 dispatch 同一 immutable tag。不得移动 tag、删除 release policy 或绕过 required reviewer。

## T03a — 收敛 recovery overlay 至实际最小审计差异并复验 policy

- 状态：完成
- 实际：将 test-only recovery overlay 的 archive、工作树 diff 断言和 Go canonical verifier 同步收敛为 tag 与当前默认分支实际不同的四条路径；移除遗留的两条 `git add -N`，并把聚焦策略测试改为精确四路径命令、逐条缺失和额外路径拒绝。
- 证据：提交 `59105ed`；TDD RED `go test ./scripts/releaseworkflow -run '^TestCheckRecoveryPolicyRequiresExactFourPathOverlay$' -count=1` 在旧 17 条命令下失败，GREEN 后通过；`go test ./scripts/releaseworkflow -count=1`、`sh scripts/test-release-workflow.sh`、`go test ./...`、`git diff --check origin/main..HEAD` 和 pre-commit 均通过。临时 detached `v0.2.0` worktree 从 `origin/main` archive 四条路径后，`git diff --name-only` 精确等于该集合且 cached diff 为空。规格审查与质量审查均批准。
- 风险/下一步：仍需把修复推送、通过六平台 PR CI，并从默认分支重新 dispatch `release_tag=v0.2.0`；tag 保持不可变，生产构建隔离仍待远端实际 run 验证。

## T03b — 对齐 protected release Environment 与受审计 main recovery dispatch

- 状态：未完成
- 实际：
- 证据：
- 风险/下一步：

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
