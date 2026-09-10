# Goal-1 Tasks：Pixiv API 迁移与稳定性收敛

修订日期：2026-09-11。执行分支：`refactor/pixiv-api-stability`。继承代码基线：`e40443495981cdaf01215d6711cb24fa618b087a`。

## 执行规则

- `goal-1/plan.md` 定义 required scope、证据规则和终止状态机；本文件定义执行顺序。
- `goal-1/input.md` 保持用户原始启动输入，禁止改写。
- 每轮只执行第一个未完成且可以安全执行的 task；不得合并、跳序或顺手扩 scope。
- 所有生产代码修改必须 Red → Green → Refactor；如果 task 经过当前证据证明已满足 acceptance，可以 no-op `verified`，但必须记录复核证据。
- 旧 `goal-3/tasks.md` 的 `verified` 只能作为调查线索，不能自动提升本文件状态。
- 原 41 个 required capability 全部继续 required；本文件无权把其中任何一个标记为 `deferred_nonblocking`。
- 禁止未分解 meta-task，例如“以后再拆”“按 owner 再拆”“修所有剩余问题”。
- 每完成三个普通 task，下一轮必须执行对应 `CHECK` task。
- 每个 task 完成后必须填写实际改动、验证证据、剩余风险和下一步。
- 任何新增 correction task 必须绑定既有 required acceptance failure，不得新增产品范围。
- 新发现但不属于原 41 项 required scope 的需求只记录到 `goal-1/current-state.md` 的 `out-of-scope observations`，不进入当前 Goal 的 required task graph。

## Task 状态

普通状态：

- `pending`：尚未执行。
- `in_progress`：当前轮正在执行。
- `verified`：acceptance 已由事实证据证明。
- `blocked_external`：仅被账号、权限、网络、目标数据或上游状态阻塞；不能冒充 verified。
- `blocked_decision`：继续需要用户批准 breaking/scope/security 决策；不能冒充 verified。

`blocked_external` / `blocked_decision` 是 task 的终态，但不是 Goal 完成态。

依赖默认要求前置 task 为 `verified`。只有 G1-T18 与 G1-CHECK-06 属于终态计算任务，可以在 G1-T16/G1-T17 已达到任一终态（`verified` / `blocked_external` / `blocked_decision`）后运行，以计算 `COMPLETED` 或阻塞结果。

## Correction task 规则

检查、回归和终审允许追加 correction task，格式必须为 `G1-CORR-<来源>-NN`，并包含：

- Source task/gate
- Capability
- Observed failure
- Expected contract
- Scope boundary
- Red command / expected failure（若改生产代码）
- Green acceptance
- Compatibility impact
- Rollback boundary

Correction 只能修复本 Goal 已冻结的 acceptance failure 或本 Goal 引入的回归。不能借 correction 增加第 42 个 required capability。

如果 correction 使已经完成的 downstream gate 失效，必须把受影响 gate/closure task 重置为 `pending` 并重新验证，禁止沿用旧通过结果。

---

# Phase A — Baseline reconciliation

目标：在继续业务实现前，把 41 个 required capability 的当前真实 layer 状态、已知 correctness 风险和有限剩余工作映射清楚。此阶段不修改业务代码。

## G1-T01 — 建立 41 capability 当前状态与 evidence index

**Status:** pending

**Depends on:** none

**Scope:** `goal-1/current-state.md`、只读代码/测试/history/旧 `goal-3/` 证据。

**目标：** 对旧 41 个 required capability 逐项建立 Contract / Adapter / SDK / Shared / CLI / MCP / Offline / Live / Compatibility / Release 状态。

**验收：**
- 创建 `goal-1/current-state.md`。
- 必须恰好覆盖旧 41 个 required capability；不能少项、合并后丢项或增加 required 项。
- 每个适用 layer 使用 `verified` / `implemented_unverified` / `missing` / `blocked_external` / `blocked_decision` / `not_applicable`。
- 每个 `verified` 必须有证据索引；每个 `not_applicable` 必须有 contract 理由。
- 记录 `e404434` 的 T37D WIP，不能把 WIP 误记为完成。
- 旧 task status 与当前事实冲突时明确登记 drift。
- 记录当前分支相对源分支的基线，不把后续 Goal 文档提交混入旧实现证据。

**禁止：** 修改生产代码；缩减 required scope；仅复制旧 capability 单字段状态。

**完成记录：**
- 实际改动：
- 验证证据：
- 状态漂移：
- 剩余风险：
- 下一步：G1-T02

## G1-T02 — 复核 known correctness 与 forbidden behavior

**Status:** pending

**Depends on:** G1-T01

**Scope:** pagination/replay、novel latest/detail/series、recommended continuation、comments DTO、restrict/rating、rejected endpoint/no-fallback。

**目标：** 把旧风险审计与当前实现/测试逐项对照，确认哪些 correctness 问题已经真实关闭，哪些仅“实现过但未证明”，哪些仍然存在。

**验收：**
- `current-state.md` 有独立 correctness ledger。
- 至少覆盖 logical pagination/checkpoint/replay、novel latest continuation、novel detail/series endpoint、artwork recommended continuation、artwork/novel comments DTO、restrict/rating filter、rejected endpoint no-fallback。
- 每项记录代码路径、测试路径、历史证据和当前 verdict。
- P0/P1 不能以“known limitation”方式绕过。
- 发现内部可修复 failure 时只登记 correction candidate，不在本 task 修改业务代码。
- 发现新需求但不属于旧 41 required 时记为 out-of-scope observation。

**完成记录：**
- 实际改动：
- 验证证据：
- Correction candidates：
- 剩余风险：
- 下一步：G1-T03

## G1-T03 — 编译 finite execution manifest 与 closure mapping

**Status:** pending

**Depends on:** G1-T01,G1-T02

**Scope:** `goal-1/current-state.md`、本 `tasks.md` 的映射核验；不改业务代码。

**目标：** 证明每个非 accepted required capability 都有有限处理路径，并把旧 remaining work 映射到本文件已经列出的 leaf task 或受约束 correction task。

**验收：**
- 41 个 required capability 每项必须属于以下之一：`accepted_by_evidence`、`mapped_to_task`、`blocked_external`、`blocked_decision`；初始化阶段不得出现 `deferred_nonblocking`。
- 每个 `mapped_to_task` 至少有一个具体 task ID。
- MCP read/mutation、cursor integrity、compatibility、docs、offline regression、live validation 都有明确 owner。
- 不存在“以后再拆卡”的任务。
- 若 G1-T01/T02 发现旧已完成区域存在真实 required gap，为每个独立根因追加一个符合规则的 correction task；不得新增 capability。
- 输出 manifest checksum/计数：required=41，unmapped=0，undecomposed=0。

**完成记录：**
- 实际改动：
- Manifest 计数：
- 新增 correction task：
- 剩余风险：
- 下一步：G1-CHECK-01

## G1-CHECK-01 — 集中检查：baseline、correctness、closure

**Status:** pending

**Depends on:** G1-T01,G1-T02,G1-T03

**检查：** input/plan 偏离、41 项完整性、evidence 真伪、状态漂移、P0/P1 correctness、forbidden endpoint、task mapping、未分解任务、scope creep。

**Pass 条件：** `required=41`、`unmapped=0`、`undecomposed=0`，且没有被错误隐藏的内部 correctness gap。

**发现问题：** 只能登记/追加绑定既有 acceptance 的 correction task；不在 CHECK 内顺手修改业务代码。

**完成记录：**
- 检查结论：
- Required/unmapped/undecomposed：
- 新增 correction task：
- 剩余风险：

---

# Phase B — MCP read convergence

## G1-T04 — MCP user / MyPixiv / relationship read 收敛

**Status:** pending

**Depends on:** G1-CHECK-01

**Capabilities:** `user-artworks`、`user-novels`、`user-relationships`、`user-detail`、`user-search`、`mypixiv`，以及其 MCP/Shared 相关 acceptance。

**目标：** 收敛 `e404434` 的 T37D WIP：search user、user detail/artworks/novels、MyPixiv、following/followers/related/blocked 的 schema、resolver/filter、pagination、structured error、legacy replay。

**实现准入：** 若 G1-T01/T03 已证明该 slice 全部满足 acceptance，则允许 no-op verified；否则代码修改必须先得到行为性 Red。

**验收：**
- 相关 MCP schema 与 frozen compatibility 一致。
- identity/resolver/filter/pagination 行为有专项测试。
- structured error 不伪装成功。
- legacy JSON replay 保持兼容。
- rejected endpoint 不可达。
- 不扩展到 bookmark/mutation。

**回滚边界：** 仅 MCP user/MyPixiv/relationship read slice 与必要共享 schema helper。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- 兼容影响：
- 剩余风险：
- 下一步：G1-T05

## G1-T05 — MCP bookmark read 与 required aggregate 收敛

**Status:** pending

**Depends on:** G1-T04

**Capabilities:** artwork/novel bookmark list/tags/detail、`bookmark-subtype`、`bookmark-list-all`、`bookmark-tags-all`。

**目标：** 完成 MCP bookmark read surface，并保留旧 `user_bookmarks` / `bookmark_tags` wire；旧 required aggregate 不能降级为 enhancement。

**验收：**
- artwork/novel list/tags/detail schema、错误和 cursor 行为有专项回归。
- `bookmark-list-all`：artwork 后 novel，统一 Skip/Limit budget，双流 checkpoint，可恢复且不遗漏不重复。
- `bookmark-tags-all`：按内容类型保留同名标签与各自 count。
- 聚合中任一 required 流失败时逻辑页整体失败，不输出部分成功。
- legacy JSON replay 通过。
- 不修改 mutation surface。

**回滚边界：** MCP bookmark read/aggregate 与必要共享聚合调用方。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- 兼容影响：
- 剩余风险：
- 下一步：G1-T06

## G1-T06 — MCP read registration/schema/error/replay harness 收敛

**Status:** pending

**Depends on:** G1-T04,G1-T05

**Capabilities:** 所有 required MCP read layer 的公共 gate。

**目标：** 收敛 read tool registration、legacy exact-set、共享 output/error schema、stdio stdout 边界和全量 read replay harness。

**验收：**
- tool exact-set 与兼容矩阵一致；新增 required operation 只能 additive。
- 旧 tool 不被静默重命名/删除。
- legacy request replay 全通过。
- structured error schema 一致。
- stdout 不混入日志/诊断噪声。
- read tool 的 forbidden endpoint/no-fallback 检查通过。
- 不包含 mutation implementation。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- Tool-set evidence：
- 剩余风险：
- 下一步：G1-CHECK-02

## G1-CHECK-02 — 集中检查：MCP read

**Status:** pending

**Depends on:** G1-T04,G1-T05,G1-T06

复查 MCP read owner 隔离、schema、structured errors、pagination/filter、aggregate 原子性、tool exact-set、legacy replay、forbidden endpoint、测试和 docs drift。

**Pass 条件：** Phase B 对应所有 required MCP read acceptance 均有证据或已有受约束 correction task。

**完成记录：**
- 检查结论：
- 新增 correction task：
- 剩余风险：

---

# Phase C — MCP mutation convergence

## G1-T07 — MCP artwork/novel bookmark mutation vertical slice

**Status:** pending

**Depends on:** G1-CHECK-02

**Capabilities:** `artwork-bookmark-mutation`、`novel-bookmark-mutation` 的 MCP layer。

**目标：** 完成 artwork/novel bookmark add/remove 的 MCP input validation、SDK dispatch、mutation outcome、structured error 和兼容 replay。

**验收：**
- 代码改动前有真实 Red。
- public/private、成功、明确失败、不确定结果均有 offline fixture。
- uncertain mutation 不自动 replay。
- 旧请求 wire 兼容。
- 不把离线成功冒充 live round-trip。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- 兼容影响：
- 剩余风险：
- 下一步：G1-T08

## G1-T08 — MCP artwork/novel comment + stamp mutation vertical slice

**Status:** pending

**Depends on:** G1-T07

**Capabilities:** `artwork-comments-mutation`、`novel-comments-mutation`、`stamps` 的 mutation/MCP acceptance。

**目标：** 完成 create/reply/stamp/delete MCP surface，严格复用已冻结 SDK、comment ID 来源和 mutation outcome 语义。

**验收：**
- create/reply/stamp/delete 各自有输入、成功、错误和 uncertain-result 测试。
- 本轮创建 ID 必须来自可靠响应/contract，禁止通过“最新评论”等启发式猜测。
- 不确定结果不自动重放。
- structured error 与 legacy compatibility 通过。
- 不扩大到未经旧 scope 承诺的新 mutation。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- ID/outcome evidence：
- 剩余风险：
- 下一步：G1-T09

## G1-T09 — MCP follow/unfollow mutation vertical slice

**Status:** pending

**Depends on:** G1-T08

**Capabilities:** `follow-mutation` 的 MCP layer。

**目标：** 完成 user follow/unfollow MCP mutation surface，并保持 restrict 校验和旧 wire。

**验收：**
- invalid restrict 在网络请求前拒绝。
- success / definite failure / uncertain outcome 有离线回归。
- legacy JSON replay 通过。
- 不新增通用 mutation retry。
- 不改变 SDK/CLI 已冻结兼容语义。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- 剩余风险：
- 下一步：G1-CHECK-03

## G1-CHECK-03 — 集中检查：mutation

**Status:** pending

**Depends on:** G1-T07,G1-T08,G1-T09

复查 access control、uncertain outcome、重放安全、同账号语义、本轮 ID、旧 wire、错误传播、敏感信息、无自动 retry、离线/live 证据边界。

**Pass 条件：** 所有 MCP mutation 内部实现 gap 已关闭或有明确 correction；不得用 live blocker 掩盖可修复代码问题。

**完成记录：**
- 检查结论：
- 新增 correction task：
- 剩余风险：

---

# Phase D — Compatibility and release contract convergence

## G1-T10 — Cursor integrity、binding 与跨版本 rollback gate

**Status:** pending

**Depends on:** G1-CHECK-03

**Capabilities:** `logical-pagination` 及所有使用持久化 cursor 的 required capability。

**目标：** 收敛旧 R01：明确 cursor 的不可信边界、版本/binding 行为和跨版本 rollback/失效策略。

**验收：**
- cursor 不包含 credential、cookie、token、signed URL、原始用户内容或未脱敏 next_url。
- query/account/client/subtype binding 与 frozen contract 一致。
- 不兼容版本明确返回 `InvalidCursor` 或已定义迁移结果，禁止静默从第一页重启。
- 批内 checkpoint、末批 checkpoint、Skip/Limit/OneBatch、重复 cursor、取消和 replay 回归通过。
- 有跨版本 rollback/compatibility 测试或明确不可兼容的受控失败测试。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- Integrity evidence：
- 剩余风险：
- 下一步：G1-T11

## G1-T11 — 全局 SDK / CLI / MCP compatibility audit 与差异收敛

**Status:** pending

**Depends on:** G1-T10

**Capabilities:** 41 项的 Compatibility layer。

**目标：** 对照旧 T12/T39A compatibility matrices 和当前实现，只修真实差异，不重新设计 public surface。

**验收：**
- public Go SDK symbol/named type/legacy consumer compilation 通过。
- CLI canonical route、legacy alias、默认值和错误语义通过。
- MCP old tool/input/output wire 和 legacy JSON replay 通过。
- `NovelContent` 等 excluded endpoint 的兼容入口不发 rejected 请求。
- 发现必须 breaking change 时进入 `blocked_decision`，不得擅自实施。

**完成记录：**
- 实际改动：
- Red：
- Green/回归：
- Breaking-change audit：
- 剩余风险：
- 下一步：G1-T12

## G1-T12 — CLI presentation、双语 docs、Skill 与 changelog 收敛

**Status:** pending

**Depends on:** G1-T11

**Capabilities:** 41 项的文档/可发现性公共 gate。

**目标：** 同步 completion/help/deprecation、README、CLI/SDK/MCP docs、`skills/pixiv-cli/` 和必要 changelog，只描述真实已实现 surface。

**验收：**
- CLI help/completion 与实际注册一致。
- 双语文档不宣称 rejected/excluded 能力可用。
- cursor/pagination、mutation uncertainty、兼容/弃用行为有必要说明。
- docs/completion tests 通过。
- 不把未取得 live 证明的能力描述为“已 live 验证”。

**完成记录：**
- 实际改动：
- 验证：
- 文档一致性：
- 剩余风险：
- 下一步：G1-CHECK-04

## G1-CHECK-04 — 集中检查：compatibility / docs / release contracts

**Status:** pending

**Depends on:** G1-T10,G1-T11,G1-T12

复查 cursor integrity、SDK/CLI/MCP compatibility、breaking-change blocker、presentation、docs、Skill、changelog、forbidden endpoint 文档和真实 surface 一致性。

**完成记录：**
- 检查结论：
- 新增 correction task：
- Blocking decision：
- 剩余风险：

---

# Phase E — Offline release candidate

## G1-T13 — Protocol / endpoint / SDK regression gate

**Status:** pending

**Depends on:** G1-CHECK-04

**目标：** 对 adapter、DTO、request、continuation、SDK models/methods/cursor 做系统离线回归。

**验收：**
- required/optional/null/empty/error fixture 通过。
- adapter ↔ SDK 对照通过。
- continuation allowlist 与 query/account/subtype binding 通过。
- old consumer compile/public API checks 通过。
- rejected endpoint 负向测试通过。
- 不以 historical success 替代当前运行结果。

**完成记录：**
- 实际改动：
- 回归命令：
- 结果：
- 新增 correction task：
- 剩余风险：
- 下一步：G1-T14

## G1-T14 — CLI / MCP regression gate

**Status:** pending

**Depends on:** G1-T13

**目标：** 对所有 required CLI/MCP surface 做离线集成和兼容回归。

**验收：**
- CLI JSON/NDJSON/stdin/skip/fail-fast/cursor/alias 行为通过。
- MCP schema、structured errors、exact-set、legacy JSON replay、stdout 边界通过。
- aggregate operation 的顺序、budget、checkpoint、失败原子性通过。
- mutation offline outcome 与 replay safety 通过。
- 不调用 forbidden endpoint。

**完成记录：**
- 实际改动：
- 回归命令：
- 结果：
- 新增 correction task：
- 剩余风险：
- 下一步：G1-T15

## G1-T15 — Full offline release-candidate build / quality / redaction gate

**Status:** pending

**Depends on:** G1-T13,G1-T14

**目标：** 运行本 Goal 最大范围可离线执行的 release-candidate gate。

**必须验证：**
- `go test ./...`
- `go vet ./...`
- `sh scripts/build.sh`
- 必要 race tests
- public API compatibility
- CLI/MCP compatibility replay
- docs/completion tests
- forbidden endpoint/no-fallback 负向检查
- evidence/log 中 token、cookie、signed URL、隐私数据和其他敏感信息检查
- `current-state.md` 中所有内部可解决 layer 不得继续是 `missing` 或无理由的 `implemented_unverified`

任何失败必须映射到具体根因并建立 correction task；禁止创建“修所有测试”任务。

**完成记录：**
- 实际改动：
- Gate 命令：
- Gate 结果：
- 新增 correction task：
- 剩余风险：
- 下一步：G1-CHECK-05

## G1-CHECK-05 — 集中检查：offline release candidate

**Status:** pending

**Depends on:** G1-T13,G1-T14,G1-T15

**目标：** 证明所有代码库内部可解决的 required gap 已关闭，进入 live phase 前只剩真正外部验证或明确用户决策。

**Pass 条件：**
- internal `missing` = 0。
- unjustified `implemented_unverified` = 0。
- open P0/P1 correctness = 0。
- offline gates = PASS。
- required=41、unmapped=0、undecomposed=0。

**完成记录：**
- 检查结论：
- Internal gaps：
- External candidates：
- Decision blockers：
- 新增 correction task：

---

# Phase F — Live validation and terminal closure

## G1-T16 — Live read validation

**Status:** pending

**Depends on:** G1-CHECK-05

**Capabilities:** 所有 contract 明确需要当前 live read/第二页证明的 required capability。

**目标：** 使用明确授权账号/网络/目标数据验证关键 read endpoint、第二页 continuation 以及 adapter/SDK/CLI/MCP 对齐。

**验收：**
- 有授权条件时执行 live read，记录脱敏 evidence。
- 至少覆盖 frozen live manifest 中的 endpoint/path、关键 query、第二页 continuation 和错误边界。
- 不借用旧 historical success 冒充当前验证。
- 缺账号/权限/网络/目标数据/上游条件时，只有满足 plan 的严格条件才可标 `blocked_external`。
- 如果 live 暴露代码内 correctness 问题，则不能标 external blocker；必须登记 correction task。

**完成记录：**
- 实际验证：
- Evidence：
- External blocker（如有）：
- Correction task（如有）：
- 剩余风险：
- 下一步：G1-T17

## G1-T17 — Live mutation validation 与隔离清理

**Status:** pending

**Depends on:** G1-CHECK-05

**Capabilities:** artwork/novel bookmark mutation、artwork/novel comments mutation、follow mutation，以及相关 stamps/read-back contract。

**目标：** 在明确授权的隔离账号与真实目标下验证 mutation round-trip。

**验收：**
- 写前确认 access control 和目标归属。
- create/add/follow 后按 contract read-back。
- reply/stamp/delete/remove/unfollow 按对应 contract 验证。
- 仅清理本轮可识别的副作用；不删除既有用户数据。
- uncertain result 不自动 replay。
- evidence 脱敏。
- 外部条件不足时可 `blocked_external`，但不能记作 mutation verified。
- live 暴露内部 bug 时建立 correction task，而不是 external blocker。

**完成记录：**
- 实际验证：
- Read-back / cleanup：
- Evidence：
- External blocker（如有）：
- Correction task（如有）：
- 剩余风险：
- 下一步：G1-T18

## G1-T18 — 重新计算 41 capability acceptance 与生成 closure report

**Status:** pending

**Depends on:** G1-T16,G1-T17 reached terminal status

**Scope:** 只更新 `goal-1/current-state.md`、`goal-1/closure-report.md` 和必要 task 状态；不修改业务代码。

**目标：** 从当前最终代码和所有 gate 证据机械计算每个 capability 的 acceptance 与 Goal 运行终态。

**验收：**
- 重新核对 41 个 required capability，数量必须恰好为 41。
- 每个 capability 的适用 layer 和 evidence index 最终一致。
- 输出 `accepted_count`、`blocked_external_count`、`blocked_decision_count`、`internal_gap_count`、`pending_task_count`、`undecomposed_task_count`。
- 创建/更新 `goal-1/closure-report.md`。
- 只有满足 plan 中 `COMPLETED` 全部条件时才能写 `GoalState: COMPLETED`。
- 任一 required external blocker 存在时写 `GoalState: BLOCKED_EXTERNAL`。
- 任一 required decision blocker 存在时写 `GoalState: BLOCKED_DECISION`；若同时存在 external blocker，同时列出但 decision blocker 优先决定需要用户输入。
- 若发现内部 gap，GoalState 保持 `ACTIVE` 并建立受约束 correction task；不得伪造终态。

**完成记录：**
- Accepted：
- Blocked external：
- Blocked decision：
- Internal gaps：
- Pending/undecomposed：
- GoalState：
- 下一步：G1-CHECK-06

## G1-CHECK-06 — 最终集中检查-debug 与终态证明

**Status:** pending

**Depends on:** G1-T16,G1-T17,G1-T18 reached terminal status

这是 Goal Mode 的最终最大范围复查，不是新的 feature discovery 阶段。

**必须复查：**
- 41 required capability 是否完整且 acceptance 计算正确。
- tasks 是否存在 pending/in_progress required work。
- 是否有未分解 meta-task。
- P0/P1 correctness 是否全部关闭。
- cursor integrity/rollback gate。
- SDK/CLI/MCP compatibility。
- protocol/SDK、CLI/MCP、full test/vet/build/race gate。
- forbidden endpoint/no-fallback。
- docs/Skill/completion/changelog 一致性。
- live evidence 与 blocker 分类真实性。
- evidence/redaction。
- correction task 是否全部闭合且没有让旧 gate 失效。

**最终判定规则：**

```text
COMPLETED iff
  required_capabilities == 41
  AND accepted_count == 41
  AND pending_required_tasks == 0
  AND in_progress_required_tasks == 0
  AND required_blockers == 0
  AND undecomposed_tasks == 0
  AND open_correctness_p0_p1 == 0
  AND cursor_integrity_gate == PASS
  AND sdk_compat_gate == PASS
  AND cli_compat_gate == PASS
  AND mcp_compat_gate == PASS
  AND protocol_sdk_regression == PASS
  AND cli_mcp_regression == PASS
  AND full_offline_gate == PASS
  AND required_live_gate == PASS
  AND documentation_gate == PASS
  AND redaction_gate == PASS
```

如果只剩真实 external blocker，则最终为 `BLOCKED_EXTERNAL`；如果需要明确用户决策，则最终为 `BLOCKED_DECISION`。两者都允许无人值守 Goal Mode 停止，但**不得调用“标记 goal 完成”的动作**，不得宣称 release/public ready。

若最终检查发现既有 required acceptance failure，只能追加受约束 correction task，并把 G1-T18 / G1-CHECK-06 及受影响 gate 重置为 `pending` 后重新验证。禁止在最终检查新增产品 scope。

**完成记录：**
- 最终检查结论：
- accepted_count：
- external blockers：
- decision blockers：
- pending/in_progress：
- undecomposed：
- gate summary：
- GoalState：

---

# 终点约束

本文件的最后一行不是“所有 task 都做过”就算完成。Goal Mode 只有三种合法停止结果：

- `COMPLETED`：41/41 accepted，全部 required gate 通过，无 blocker。
- `BLOCKED_EXTERNAL`：无内部可执行工作，offline gate 通过，但 required live/external 条件不足。
- `BLOCKED_DECISION`：无安全可继续路径，等待用户批准 breaking/scope/security 决策。

`BLOCKED_*` 只是停止自动推进，不是完成。只有 `COMPLETED` 才能把 Goal 在客户端标记为完成。
