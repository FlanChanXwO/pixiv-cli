# Goal-1：Pixiv API 迁移与稳定性收敛计划

修订日期：2026-09-11。执行分支：`refactor/pixiv-api-stability`。继承基线：`codex/goal-3-vnext-progress` 的 `e40443495981cdaf01215d6711cb24fa618b087a`。

## 1. Goal 定位

本 Goal 不重新发明一套 Pixiv API 方案，也不缩减旧 Goal-3 已经明确承诺的交付范围。它的职责是把旧分支中已经完成的大量接口迁移、SDK/CLI/MCP 改造、分页修复、兼容工作和验证证据重新整理为一个**有限、可验证、可终止**的 Goal Mode 执行图，然后只完成真实剩余工作。

旧 `goal-3/` 目录继续作为历史 contract、验证证据、兼容决策和风险材料来源，但不再作为当前执行状态机。旧 `tasks.md` 的 `verified` 只能作为证据线索，不能单独证明当前分支仍满足对应验收。

本 Goal 主要修正旧执行模型的四个结构缺陷：

1. 长期总目标与“当前轮允许做什么”混在一起，导致局部修复完成后仍自动向整个任务图推进。
2. capability 状态、task 状态和 release gate 是多套人工维护真相，已经发生漂移。
3. 存在未分解 meta-task、不可达依赖和未接入最终终点的 gate。
4. 最终 audit 可以继续自由发现新范围，形成没有 closure 的开放式任务生成器。

## 2. 范围保真：旧 41 项 required capability 全部保留

本 Goal **不进行 scope reduction**。旧 `goal-3/capability-admission.md` 中 41 个 `required=yes` capability 全部继续 required；优先级只影响执行顺序，不能改变 requiredness，也不能把原 required 项降级为 `deferred_nonblocking`。

### Artwork / feed

1. `artwork-search`
2. `artwork-latest`
3. `artwork-ranking`
4. `artwork-recommended`
5. `artwork-series`
6. `ugoira-metadata`

### Novel / feed

7. `novel-search`
8. `novel-detail`
9. `novel-series`
10. `novel-latest`
11. `novel-recommended`
12. `novel-ranking`
13. `novel-follow`

### Bookmark

14. `artwork-bookmark-list`
15. `artwork-bookmark-tags`
16. `artwork-bookmark-detail`
17. `artwork-bookmark-mutation`
18. `novel-bookmark-list`
19. `novel-bookmark-tags`
20. `novel-bookmark-detail`
21. `novel-bookmark-mutation`
22. `bookmark-subtype`
23. `bookmark-list-all`
24. `bookmark-tags-all`

### Comments / stamps

25. `artwork-comments-read`
26. `artwork-comments-mutation`
27. `novel-comments-read`
28. `novel-comments-mutation`
29. `stamps`

### User / relationship

30. `user-artworks`
31. `user-novels`
32. `user-relationships`
33. `user-detail`
34. `user-search`
35. `trending`
36. `follow-mutation`
37. `mypixiv`

### Shared semantics / aggregation

38. `bare-id-probe`
39. `rating-filter`
40. `logical-pagination`
41. `recommended-all`

只有用户后续明确批准 scope change，且在本计划中记录日期、理由、受影响 capability、兼容影响和验收变化后，才能改变以上 required 集合。

## 3. 明确排除与禁止行为

继续继承旧计划已经确认的 exclusion：

- `/v1/novel/detail`
- `/v1/novel/series`
- `/v1/novel/content`
- WebView/anonymous fallback
- 未经确认的 server-side `x_restrict`

禁止因为替代 endpoint 不可用而静默回退到 rejected path；禁止把上游错误伪装为空结果；禁止把 cursor 当鉴权凭据；禁止在 mutation 不确定结果上自动重放。

## 4. 权威资料与证据优先级

允许引用的旧资料包括：

- `goal-3/plan.md`
- `goal-3/tasks.md`
- `goal-3/capability-admission.md`
- `goal-3/current-surface-risk-audit.md`
- `goal-3/api-migration-verification.md`
- `goal-3/upstream-contract-matrix.md`
- `goal-3/cli-migration-matrix.md`
- `goal-3/mcp-compatibility-matrix.md`
- `goal-3/pagination-validation-report.md`
- `goal-3/mutation-validation-report.md`
- `goal-3/feasibility-report.md`
- `goal-3/wire-adapter-sdk-diff.md`
- `goal-3/shaft-protocol-diff.md`
- `goal-3/evidence/`

证据可信度按以下顺序判断，后一层不能覆盖前一层的冲突事实：

1. 当前分支可重复运行的测试、构建、静态检查和真实 live 结果。
2. 当前分支代码与明确可追溯的 commit/diff。
3. 旧 Goal-3 的原始 fixture、wire evidence、pagination/mutation 报告。
4. 旧 contract/compatibility matrices。
5. 旧 `tasks.md` 完成记录。
6. 旧 capability 单字段状态。

任何 capability 或 layer 只能在证据足以证明时标记 `verified`。历史 `verified` 与当前代码冲突时，以当前代码和可重复验证结果为准，并建立 correction task。

## 5. 当前状态模型：一份真相，不再手写 public_ready

`goal-1/current-state.md` 是本 Goal 开始执行后生成的当前状态权威表。每个 required capability 逐层记录：

- Contract
- Adapter
- SDK
- Shared semantics
- CLI
- MCP
- Offline / fixture regression
- Live validation
- Compatibility
- Release readiness

layer 状态只允许：

- `verified`：有当前或可复核证据证明满足目标 contract。
- `implemented_unverified`：实现看起来存在，但当前 Goal 尚未取得足够证据。
- `missing`：实现或必要验证不存在。
- `blocked_external`：实现侧无已知缺口，但验证依赖当前不可用的账号、数据、网络或外部服务状态。
- `blocked_decision`：继续必须做未经授权的 breaking/scope/security 决策。
- `not_applicable`：该 layer 按 capability contract 明确不适用；必须写理由。

`public_ready` / `release_ready` 不再作为手工维护状态；只能由上述 layer 和 gate **派生计算**。

## 6. Required capability 的完成定义

一个 required capability 只有同时满足以下条件才算 `accepted`：

1. 其适用 layer 均为 `verified` 或 contract 明确允许的 `not_applicable`。
2. 对应离线/fixture correctness 验收通过。
3. 对应 SDK/CLI/MCP 兼容要求通过，或者 contract 明确不要求该 surface。
4. 所有 forbidden endpoint / no-fallback 负向断言通过。
5. 若该 capability 的 release contract 明确要求 live 证明，则 live layer 必须 `verified`；`blocked_external` 只能形成 Goal 阻塞，不能形成 accepted。
6. 不存在映射到该 capability 的未解决 P0/P1 correctness finding。

仅仅“代码已经写了”“旧 tasks 标 verified”“单元测试通过”或“没有 pending task”都不足以让 capability accepted。

## 7. 优先级：只决定顺序，不决定是否交付

### P0/P1 correctness 优先

优先确认并关闭：

- logical pagination / checkpoint / replay correctness
- novel latest continuation
- novel detail / series endpoint migration
- artwork recommended continuation
- artwork / novel comments contract 与 DTO
- restrict / rating filter 语义
- rejected endpoint 不可达与 no-fallback

### 核心 public surface

随后收敛已经接近完成但仍缺 MCP/read/mutation/compatibility 的能力：

- user / MyPixiv / relationships
- bookmark read 与 aggregate
- bookmark mutation
- comment/stamp mutation
- follow mutation
- MCP registration/schema/error/replay

### 最终 convergence

最后执行：

- cursor integrity / rollback gate
- SDK/CLI/MCP compatibility audit
- CLI presentation / docs / Skill
- protocol/SDK regression
- CLI/MCP regression
- full build/release-candidate gate
- live read/mutation validation
- capability closure audit

`bookmark-*-all`、`recommended-all`、`bare-id-probe` 等即使优先级较低，也仍属于 required scope，不能因为是 enhancement 风格能力而自动延期。

## 8. 执行结构与阶段出口

### Phase A — Baseline reconciliation

目标：从 `e404434` 重新建立真实 capability/layer 状态，并证明任务图 closure。

出口条件：

- 41 个 required capability 全部出现在 `current-state.md`。
- 每个非 verified layer 都映射到具体 task、外部 blocker 或明确 `not_applicable`。
- 没有“以后再拆”“按 owner 再拆”的 meta-task。
- 不允许改变 required scope。

### Phase B — MCP read convergence

目标：完成 user/MyPixiv/relationship、bookmark read/aggregate 和 read harness。

出口条件：相关 MCP schema、structured error、pagination/filter、legacy JSON replay 和 registration exact-set 全部有证据。

### Phase C — MCP mutation convergence

目标：完成 bookmark、comment/stamp、follow mutation surface。

出口条件：input validation、outcome、uncertain-result、legacy compatibility 和 offline fixture 全部通过；live 证明留给专门 live phase。

### Phase D — Compatibility and release contract convergence

目标：关闭 cursor integrity/rollback、SDK/CLI/MCP compatibility、presentation/docs/Skill 差异。

出口条件：不存在未决 breaking change；若必须 breaking change，进入 `blocked_decision`，不得由 Agent 自行实施。

### Phase E — Offline release candidate

目标：完成最大范围离线回归、构建、负向 endpoint 检查和敏感数据审计。

出口条件：所有可在代码库内解决的 required gap 已关闭；只允许真正的 live/external blocker 留到 Phase F。

### Phase F — Live and terminal closure

目标：执行可授权的 live read/mutation 验证，并机械计算最终 Goal 状态。

出口条件只能是 `COMPLETED`、`BLOCKED_EXTERNAL` 或 `BLOCKED_DECISION`，不能是含糊的“基本完成”。

## 9. Task admission：所有执行项必须是 leaf task

任何普通 task 在进入 `in_progress` 前必须具备：

- 唯一 owner/slice。
- 明确涉及的 capability。
- 明确 depends_on。
- 明确允许修改的层和主要文件区域。
- 可执行验收断言。
- 代码 task 的真实 Red 方式。
- Green 后的最小相关回归。
- 兼容影响。
- 回滚边界。
- 完成后要回写的证据位置。

以下描述不能作为 task：

- “按 owner 再拆”。
- “修剩余问题”。
- “让测试都通过”。
- “完成所有兼容”。
- “根据情况继续”。

如果一个 task 同时跨越多个可独立测试的 owner，必须在**开始实现前**拆成 leaf task；拆分不能新增 capability 或扩大 scope。

## 10. TDD 与验证层级

所有生产代码修改必须 Red → Green → Refactor。Red 必须行为性失败，不能用语法错误、故意破坏 fixture 或无关失败冒充。

### Leaf task

- 目标 Red 测试。
- 最小实现。
- 目标 Green 测试。
- 相关 package tests。
- 必要的 integration/compatibility test。

### 每三个普通 task 后的集中检查

复查：

- 是否偏离 `input.md` 和本 plan。
- capability/task 映射是否漂移。
- bug、死代码、调试残留。
- 类型、构建、相关测试。
- 兼容性、错误传播、安全与敏感信息。
- pagination/cursor/mutation invariants。
- 文档和测试是否同步。

集中检查不允许顺手修业务代码；发现 required acceptance failure 时创建受约束的 correction task。

### Phase / final gate

最终才运行最大范围：

- `go test ./...`
- `go vet ./...`
- `sh scripts/build.sh`
- 必要 race tests
- public API compatibility tests
- CLI canonical/legacy route tests
- MCP exact-set / legacy JSON replay
- docs/completion tests
- forbidden endpoint / fallback 负向检查
- evidence / secret / token / cookie / URL redaction audit

## 11. Correction task：允许修复，不允许扩 scope

集中检查、regression 或终审可以新增 correction task，但必须满足全部条件：

1. 来源必须是本 Goal 已冻结的 required capability、compatibility rule、regression gate 或本 Goal 自己引入的回归。
2. 必须记录 `Source task/gate`、`Capability`、`Observed failure`、`Expected contract`、`Acceptance`。
3. 一个 correction task 只处理一个可独立验证的根因；多个根因必须拆开。
4. 不允许借 correction 引入新产品能力、依赖升级、无关重构或新的 public surface。
5. 发现真正的新需求时只记入 `out-of-scope observations`，不进入本 Goal task graph。
6. correction task 必须有有限验收；禁止“继续调查直到没问题”式任务。

因此本 Goal 允许 task 数量因**已有 acceptance 失败**而增加，但不允许 required capability 集合增长。

## 12. Live validation 与外部阻塞

Live read/mutation 只在具备明确授权的账号、网络和目标数据时执行。

### 可以标记 `blocked_external` 的条件

必须同时满足：

- 当前代码与 offline/fixture 层没有已知可修复缺口。
- blocker 确实来自账号、权限、网络、目标数据或上游服务，而不是“测试难写”或“尚未调查”。
- 记录缺失条件、受影响 capability、已完成的离线覆盖和 release 风险。
- 不借用旧历史 live success 冒充当前 live 验证。

Mutation live 额外要求：

- 隔离账号和明确授权。
- 写前 access control。
- 本轮创建/修改资源可识别。
- 写后 read-back。
- 仅清理本轮副作用。
- uncertain result 不自动 replay，不通过“最新评论”等启发式猜测创建 ID。

## 13. 兼容原则

默认继续旧计划已经冻结的策略：

- public Go SDK 尽量源码兼容；已有 exported method/type 不随 endpoint 迁移随意删除。
- CLI 保留必要 canonical/legacy alias 与 deprecation 行为。
- MCP 保留旧 tool/input/output wire compatibility，并以 legacy JSON replay 证明。
- `NovelContent` 等无可用 App API replacement 的旧 symbol 可以保留明确兼容错误，但不得调用 rejected endpoint。
- cursor version/binding 变化必须显式失败或迁移，禁止静默从第一页重启。

若发现必须 breaking change，当前 task 只能进入 `blocked_decision` 并写清 blast radius、替代方案和需要的用户决策；不能自行实施。

## 14. 回滚原则

- 本分支从现有 API 迁移 WIP 分出，不通过大规模 revert 重写历史。
- 每个新 task 只修改其 vertical slice 必需部分。
- 发现旧实现错误时优先最小 correction，而不是回滚整段已经验证的工作。
- 公共 API/CLI/MCP 变更必须记录 blast radius。
- cursor/serialization 变更必须说明跨版本恢复或明确失效策略。
- mutation/live 验证不得留下无法归属本轮的远端副作用。

## 15. 严格终止状态机

本 Goal 的运行状态只有：

- `ACTIVE`：仍存在可执行的 required task/correction task。
- `BLOCKED_EXTERNAL`：不存在可执行内部任务，但至少一个 required acceptance 仅被真实外部条件阻塞。
- `BLOCKED_DECISION`：不存在可继续安全执行的路径，且至少一个 required acceptance 需要用户批准 breaking/scope/security 决策。
- `COMPLETED`：所有 required capability accepted，所有 gate 通过，无 blocker。

### `COMPLETED` 的必要且充分条件

只有以下条件**全部同时成立**才能标记 Goal 完成：

1. 41 个 required capability 全部 `accepted`。
2. `tasks.md` 不存在 `pending` / `in_progress` 的 required ordinary task 或 correction task。
3. 不存在未分解 meta-task。
4. 不存在 `blocked_external` 或 `blocked_decision` 的 required task/capability。
5. `current-state.md` 与最终代码、测试、兼容和 live evidence 一致。
6. 所有 P0/P1 correctness finding 已关闭；不存在以“已知限制”方式隐藏的 correctness defect。
7. cursor/pagination integrity 与 rollback gate 通过。
8. SDK compatibility gate 通过。
9. CLI compatibility/presentation gate 通过。
10. MCP exact-set/schema/error/legacy replay gate 通过。
11. protocol/SDK regression 通过。
12. CLI/MCP regression 通过。
13. `go test ./...` 通过。
14. `go vet ./...` 通过。
15. `sh scripts/build.sh` 通过。
16. 必要 race tests 通过。
17. forbidden endpoint/no-fallback 负向检查通过。
18. docs/Skill/completion 与真实 surface 一致。
19. evidence 和日志不存在 token/cookie/signed URL/隐私数据泄漏。
20. 所有需要 live 证明的 required capability 已取得当前 live evidence。
21. 最终 closure audit 没有发现新的**既有 required acceptance failure**。

形式化表示：

```text
COMPLETED :=
  required_capabilities.count == 41
  AND required_capabilities.all(accepted)
  AND required_tasks.pending == 0
  AND required_tasks.in_progress == 0
  AND required_blockers == 0
  AND undecomposed_tasks == 0
  AND correctness_p0_p1_open == 0
  AND compatibility_gates == PASS
  AND offline_release_gate == PASS
  AND required_live_gate == PASS
  AND final_closure_audit == PASS
```

### `BLOCKED_EXTERNAL`

只有在以下条件全部成立时才允许停止为 external blocked：

- 没有任何当前可执行的内部 task/correction task。
- 所有离线 gate 已通过。
- 每个未 accepted required capability 的唯一剩余缺口都能映射到真实 external blocker。
- 没有未调查的 `implemented_unverified` 或 `missing` layer。
- 没有 P0/P1 内部 correctness defect。

此状态允许 Goal Mode 停止自动推进，但**不得把 Goal 标记完成，也不得宣称 public/release ready**。

### `BLOCKED_DECISION`

只有在安全继续必须得到用户明确决策时使用，例如 breaking public API、required scope change、敏感账号/权限策略变化。此状态同样不是完成。

## 16. 防止无限 Goal 的硬规则

- 最终 audit 不能新增产品 scope。
- 原 41 项之外的新需求一律写入 `out-of-scope observations`，不追加为本 Goal required task。
- audit 只能为已冻结 acceptance failure 生成 correction task。
- correction task 必须绑定根因和验收，不能再产生开放式“调查全部问题”。
- `blocked_external` / `blocked_decision` 不能被当作 verified。
- `deferred_nonblocking` 不允许用于原 41 个 required capability。
- 如果所有 task 都完成但任一 required capability 未 accepted，则 Goal **不能**结束为 COMPLETED；必须回溯到具体 layer，创建受约束 correction task或进入合法 blocker 状态。
- 如果某个 task 被证明无需代码修改，只有在现有代码和验证证据已满足其 acceptance 时才能直接标 `verified`，并记录 no-op evidence。

## 17. 最终产物

Goal 结束前至少维护：

- `goal-1/input.md`：用户原始启动输入，保持逐字不改。
- `goal-1/plan.md`：本计划和后续明确批准的 scope/decision 变更。
- `goal-1/tasks.md`：有限执行任务和每轮完成证据。
- `goal-1/current-state.md`：41 个 capability 的当前 layer 状态与 evidence index。
- `goal-1/closure-report.md`：最终 `COMPLETED` / `BLOCKED_EXTERNAL` / `BLOCKED_DECISION` 判定、gate 结果、残余风险和 blocker。

只有 `closure-report.md` 能声明最终运行终态；它必须能够从 `current-state.md`、`tasks.md` 和验证输出追溯到证据。
