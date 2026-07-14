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
- 实际：recovery PR #4 已合并至 main；审计 recovery run 已从不可变 `v0.2.0` tag 启动，但六个平台均在覆盖层步骤失败，未创建 Release。
- 证据：GitHub Actions `29304765177`（failure）。逐字复现其 overlay 后，实际 diff 仅为 `.github/workflows/release.yml`、`pkg/pixiv/account_external_test.go`、`scripts/releaseworkflow/main.go`、`scripts/releaseworkflow/main_test.go` 四项，而 workflow 的严格断言仍要求历史遗留的 17 项全集；失败发生在 Go 测试前，故与平台或 Windows ACL 无关。
- 风险/下一步：新增 T03a 以把审计白名单和运行时断言改为同一个、基于实际 tag-to-main 差异的最小集合；修复必须继续不移动 tag，且经实现/规格/质量三阶段审查和多平台 CI 后才可重新 dispatch。

## T03a — 收敛 recovery overlay 至实际最小审计差异并复验 policy

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
