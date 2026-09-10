# Goal-1 Tasks：Pixiv API 迁移与稳定性收敛

## 执行规则

- 本文件是 Goal-1 的执行顺序权威来源；`goal-1/plan.md` 定义范围与终止条件。
- 每轮只执行第一个 `pending` 且未阻塞的 task。
- 任何代码 task 必须 Red → Green → Refactor；Red 必须先真实失败。
- 旧 `goal-3/tasks.md` 的 `verified` 只能作为调查线索，不能自动提升本文件状态。
- 禁止出现“后续再拆卡”“按 owner 再拆”等未分解 meta-task。
- 每完成三个普通 task，下一项必须执行集中检查-debug。
- 每个 task 完成后填写：实际改动、验证证据、剩余风险、下一步。
- 外部条件不足的 live task 使用 `blocked_external`；不得伪装为 verified。
- `deferred_nonblocking` 只允许用于 plan 明确列为 enhancement 且不阻塞 correctness/API migration/release 的事项。

状态：`pending` / `in_progress` / `verified` / `blocked_external` / `deferred_nonblocking`。

## 有限任务集

### G1-T01 — 重建当前 capability/layer 状态

**Status:** pending

**目标：** 从当前分支 HEAD、git history、测试和 `goal-3/` 已验证资料重建真实状态表，逐 capability 记录 Contract / Adapter / SDK / Shared / CLI / MCP / Offline / Live / Release。不得直接复制旧 `scope_admitted` 或 task verified 状态。

**验收：**
- 形成 `goal-1/current-state.md`。
- 每个旧 required capability 都有明确 layer 状态和证据索引。
- 明确区分 `verified`、`implemented_unverified`、`missing`、`blocked_external`、`deferred_nonblocking`。
- 列出当前 HEAD `e40443495981cdaf01215d6711cb24fa618b087a` 的 WIP 内容和未闭合测试/实现。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T02 — 校正 known correctness 风险清单

**Status:** pending

**Depends on:** G1-T01

**目标：** 对旧风险审计与当前实现逐项对照，确认 pagination/replay、novel latest、novel detail/series、recommended continuation、comments DTO、restrict/rating 等生产 correctness 项哪些已真实修复、哪些仍有缺口。

**验收：**
- 更新 `goal-1/current-state.md` 的 correctness section。
- 每个 P0/P1 风险都有当前代码路径、测试证据和结论。
- 若发现代码内可修复缺口，为其追加明确 correction leaf task；不得以 audit task 自己顺手修业务代码。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T03 — 冻结剩余 surface 与兼容边界

**Status:** pending

**Depends on:** G1-T01,G1-T02

**目标：** 从现有 CLI/MCP/SDK compatibility matrices 和当前实现中冻结“仍需完成”的 public surface，删除已经由证据证明完成的重复工作，明确 breaking-change blocker 规则。

**验收：**
- 在 `goal-1/current-state.md` 形成 finite remaining-surface 表。
- 明确哪些 enhancement 为 `deferred_nonblocking`。
- 明确剩余 MCP read/mutation tool exact set、CLI alias、SDK compatibility 要求。
- 后续所有实现 task 均能映射到这里的具体缺口，不允许出现未知 owner。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-CHECK-01 — 集中检查：状态、风险与剩余范围

**Status:** pending

**Depends on:** G1-T01,G1-T02,G1-T03

检查 input/plan 偏离、状态证据真实性、遗漏 capability、错误优先级、兼容边界、是否存在未分解任务。运行必要静态检查和只读验证。发现本 Goal required 缺口时只能追加明确 correction leaf task。

**完成记录：**
- 检查结论：
- 新增 correction task：
- 剩余风险：

### G1-T04 — 完成 MCP user / MyPixiv / relationship read WIP

**Status:** pending

**Depends on:** G1-CHECK-01

**目标：** 仅在 G1-T03 确认仍缺失时，收敛当前 `e404434` 的 T37D WIP：search user、user detail/artworks/novels、MyPixiv、following/followers/related/blocked 的 schema、resolver/filter、pagination、structured error 和 legacy replay。

**验收：**
- 目标 Red 测试先失败。
- 相关 MCP read 测试通过。
- 旧 JSON wire 保持兼容。
- 不扩展到 bookmark/mutation。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T05 — 完成 MCP bookmark read 与 aggregate

**Status:** pending

**Depends on:** G1-T04

**目标：** 完成 artwork/novel bookmark list/tags/detail 的 MCP read，以及已确认仍 required 的 aggregate operation；保留旧 `user_bookmarks` / `bookmark_tags` wire。

**验收：**
- 双流 checkpoint 与统一 budget 有针对性 Red/Green 测试。
- 页原子失败，不输出部分成功结果。
- 旧 JSON replay 通过。
- aggregate enhancement 若 G1-T03 已判 `deferred_nonblocking`，本 task 相应缩减且记录，不自行恢复范围。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T06 — 收敛 MCP registration/schema/error/replay harness

**Status:** pending

**Depends on:** G1-T04,G1-T05

**目标：** 完成 read tool registration、legacy exact-set、共享 output/error schema、stdio stdout 边界及全量 read replay harness；不得新增未在 G1-T03 冻结的 tool。

**验收：**
- tool set 与 frozen compatibility 表一致。
- legacy request replay 全通过。
- structured errors 与 stdout/stderr 边界有回归。
- 不包含 mutation implementation。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-CHECK-02 — 集中检查：MCP read 收敛

**Status:** pending

**Depends on:** G1-T04,G1-T05,G1-T06

复查 MCP read owner 隔离、schema、error、pagination/filter、aggregate 原子性、旧 wire replay、禁止 endpoint、测试与文档漂移。发现回归时追加具体 correction leaf task。

**完成记录：**
- 检查结论：
- 新增 correction task：
- 剩余风险：

### G1-T07 — MCP bookmark mutation vertical slice

**Status:** pending

**Depends on:** G1-CHECK-02

**目标：** 对 artwork/novel bookmark add/remove 建立完整 MCP mutation slice，包括 input validation、SDK dispatch、outcome、structured error 和旧请求回放。

**验收：**
- Red 测试证明缺失/错误行为。
- Offline fixture 覆盖成功、明确失败和不确定结果。
- 不自动重放 uncertain mutation。
- 若 live 条件不可用，不在本 task 伪造 live success。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T08 — MCP comment/stamp mutation vertical slice

**Status:** pending

**Depends on:** G1-T07

**目标：** 完成 artwork/novel comment create/reply/stamp/delete 的 MCP mutation surface，严格使用已冻结 SDK 和 mutation outcome 语义。

**验收：**
- create/reply/stamp/delete 各自有明确输入与结果测试。
- 本轮 ID 与 uncertain result 可观测。
- 不通过“最新评论”猜测创建 ID。
- structured error 与 legacy compatibility 按 G1-T03 冻结要求通过。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T09 — MCP follow/unfollow mutation vertical slice

**Status:** pending

**Depends on:** G1-T08

**目标：** 完成 user follow/unfollow MCP mutation surface 与 restrict validation，保持既有 SDK/CLI 兼容策略。

**验收：**
- invalid restrict 在网络前拒绝。
- follow/unfollow success/failure outcome 有离线回归。
- 旧 wire replay 通过。
- 不引入通用 mutation retry。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-CHECK-03 — 集中检查：mutation surface

**Status:** pending

**Depends on:** G1-T07,G1-T08,G1-T09

复查 mutation access control、uncertain outcome、重放安全、同账号语义、旧 wire、错误传播、敏感信息、无自动 retry。发现问题追加具体 correction leaf task。

**完成记录：**
- 检查结论：
- 新增 correction task：
- 剩余风险：

### G1-T10 — 全局 SDK/CLI/MCP compatibility convergence

**Status:** pending

**Depends on:** G1-CHECK-03

**目标：** 对照 G1-T03 frozen surface，完成 SDK symbol、CLI alias/deprecation、MCP tool/input/output wire 的最终一致性收敛；只修实际差异。

**验收：**
- public SDK compatibility tests 通过。
- CLI canonical/legacy route tests 通过。
- MCP legacy exact-set/replay 通过。
- rejected endpoint 不可达且无 fallback。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T11 — Presentation / docs / Skill 收敛

**Status:** pending

**Depends on:** G1-T10

**目标：** 同步 completion/help/deprecation、双语 README/CLI/SDK/MCP 文档和 `skills/pixiv-cli/`，只描述已实现且已验证的 surface。

**验收：**
- 文档与当前 CLI/MCP/SDK surface 一致。
- excluded/deprecated 能力不被描述为可用。
- docs tests / completion tests 通过。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T12 — Offline full regression 与 build gate

**Status:** pending

**Depends on:** G1-T10,G1-T11

**目标：** 执行本 Goal 最大范围 offline regression，修复本 Goal 暴露或引入的真实回归。

**验收：**
- `go test ./...`
- `go vet ./...`
- `sh scripts/build.sh`
- 必要 race tests
- public API compatibility tests
- CLI/MCP replay tests
- documentation tests
- 禁止 endpoint/fallback 负向检查
- evidence/sensitive-data audit

如失败，追加按失败根因划分的 correction leaf task；不得创建模糊“修所有测试”任务。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-CHECK-04 — 集中检查：release candidate offline gate

**Status:** pending

**Depends on:** G1-T10,G1-T11,G1-T12

复查当前状态表与代码一致性、所有 P0/P1 correctness、兼容面、文档、测试、build、回滚、敏感信息与未分解任务。确认只剩明确 live/external gate 或 nonblocking deferred 项。

**完成记录：**
- 检查结论：
- 新增 correction task：
- 剩余风险：

### G1-T13 — Live read validation

**Status:** pending

**Depends on:** G1-CHECK-04

**目标：** 在存在已授权账号/网络/目标数据时验证关键 read endpoint、第二页 continuation、adapter/SDK/CLI/MCP 对齐。

**验收：**
- 能执行时记录真实 live evidence，并脱敏。
- 无授权账号、数据或环境时标记 `blocked_external`，列出缺失条件和 release 影响。
- 不借用旧 historical success 冒充本轮 live 验证。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T14 — Live mutation validation 与隔离清理

**Status:** pending

**Depends on:** G1-T13

**目标：** 在存在明确授权的隔离账号与真实目标时验证 bookmark/comment/follow mutation round-trip。

**验收：**
- 写前 access control。
- 写后 read-back。
- 仅清理本轮明确创建/修改的资源。
- uncertain result 不自动 replay。
- 无所需外部条件时标记 `blocked_external` 并说明 release blocker。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- 下一步：

### G1-T15 — 最终 closure audit

**Status:** pending

**Depends on:** G1-T13,G1-T14

**目标：** 证明 Goal-1 已达到 `goal-1/plan.md` 的明确终止条件。

**验收：**
- tasks 中无 `pending` executable leaf task。
- 无未分解 meta-task。
- correction tasks 全部完成或合法 blocked/deferred。
- `current-state.md` 与最终代码/测试一致。
- 所有 P0/P1 correctness 已 verified，或仅存在真正 external blocker。
- offline release gate 全通过。
- live blocker 若存在，明确标注 release readiness，不误报 public_ready。
- 输出最终剩余 `blocked_external` / `deferred_nonblocking` 清单；不得在此阶段扩张新的产品 scope。

**完成记录：**
- 实际改动：
- 验证证据：
- 剩余风险：
- Goal 终态：

### G1-CHECK-05 — 最终集中检查-debug

**Status:** pending

**Depends on:** G1-T13,G1-T14,G1-T15

执行 Goal Mode 最终最大范围复查。若仅剩 external blockers 或明确 nonblocking deferred 项，记录后结束；若发现本 Goal 引入的回归或既有 required correctness 未真实完成，只追加具体 correction leaf task并继续，禁止新增产品范围。

**完成记录：**
- 最终检查结论：
- External blockers：
- Deferred nonblocking：
- Goal completion：
