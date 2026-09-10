# Goal-1：Pixiv API 迁移与稳定性收敛计划

修订日期：2026-09-11。执行分支：`refactor/pixiv-api-stability`。继承基线：`codex/goal-3-vnext-progress` 的 `e40443495981cdaf01215d6711cb24fa618b087a`。

## 1. Goal 定位

本 Goal 不重新发明 Pixiv API 方案，不缩减旧 Goal-3 已承诺的交付范围，也不把历史任务重新做一遍。目标是：

1. 复核并复用已经有可信证据的实现。
2. 精确完成仍缺失或仍未验证的接口迁移、稳定性、SDK、CLI、MCP 与兼容工作。
3. 把执行过程限制为有限、可验证、可恢复、可终止的 Goal Mode 任务图。
4. 让 Luna 级执行模型可以逐轮机械推进，不需要临时设计任务、不需要猜 scope、不需要在宽任务中自行决定 owner。

旧 `goal-3/` 目录只作为 contract、验证证据、兼容决策和风险资料来源，不再作为当前执行状态机。旧 `tasks.md` 的 `verified` 只是证据线索，不能单独证明当前分支仍满足 acceptance。

本 Goal 主要修正旧执行模型的四个结构缺陷：

- 长期总目标与当前轮授权混在一起。
- capability、task、release gate 多套人工状态发生漂移。
- 存在未分解 meta-task、不可达依赖和遗漏 release gate。
- 最终 audit 可以自由产生新 scope，导致 Goal 没有 closure。

## 2. 范围保真：41 项 required capability 全部保留

本 Goal 不进行 scope reduction。旧 `goal-3/capability-admission.md` 中 41 个 `required=yes` capability 全部继续 required。优先级只影响执行顺序，不能改变 requiredness。

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

只有用户后续明确批准 scope change，并记录日期、理由、受影响 capability、兼容影响和验收变化后，才能改变以上 required 集合。

`deferred_nonblocking` 不允许用于以上 41 项。新发现但不属于这 41 项的需求只能进入 `out-of-scope observations`。

## 3. 明确排除与禁止行为

继续继承旧计划确认的 exclusion：

- `/v1/novel/detail`
- `/v1/novel/series`
- `/v1/novel/content`
- WebView/anonymous fallback
- 未经确认的 server-side `x_restrict`

禁止：

- 替代 endpoint 不可用时静默 fallback 到 rejected path。
- 把上游错误伪装为空结果或成功。
- 把 cursor 当作鉴权凭据。
- mutation 不确定结果自动重放。
- 为完成当前 task 顺手升级依赖、换框架、重命名全仓或重写架构。
- 通过新增第 42 个 required capability 来解决当前 acceptance failure。

## 4. Luna 执行契约与 Caveman skill

本 Goal 面向无人值守多轮执行。执行器必须优先遵循确定性和上下文节省原则。

### 4.1 Caveman skill 为执行前置要求

用户明确要求本 Goal 使用 **Caveman skill**。其用途限定为：压缩 Agent 自己的叙述、减少 filler/tool narration、降低跨轮上下文负担；不得改变代码、API/函数名、CLI 命令、flags 或精确错误字符串。

执行前 preflight 必须确认当前 Agent 能加载并启用 Caveman skill。若当前执行环境没有该 skill：

- 不自动安装依赖或修改全局环境。
- 记录 `CAVEMAN_SKILL_UNAVAILABLE`。
- 进入 `blocked_external`，由通用终止任务生成 closure report。
- 不以“手工模仿简短风格”冒充已使用 skill。

Caveman 只负责输出/上下文效率，不替代 TDD、验证或工程判断。

### 4.2 每轮输出预算

每轮完成记录只保留后续 worker 恢复所需事实：

- 实际改动或 no-op 结论。
- Red/Green 或只读 evidence。
- 影响的 capability。
- 剩余风险/blocker。
- 下一 task。

禁止在 `tasks.md` 重复粘贴大段测试日志、diff、旧 plan 文本或相同背景说明。长日志只记录命令、结果摘要和可追溯位置。

### 4.3 执行前 preflight

正式业务 task 之前必须机械确认：

- 当前分支为 `refactor/pixiv-api-stability`。
- HEAD 是 `e404434` 的后代。
- 除 Goal 计划文件外没有来源不明的未提交业务改动。
- `goal-1/input.md`、`plan.md`、`tasks.md` 已被跟踪。
- Go toolchain 与仓库既有测试/build 命令可用。
- Caveman skill 可加载并已启用。
- 需要代码导航时优先使用可用 LSP；不可用时记录 fallback，不能假装执行过语义导航。

Preflight 只检查实施条件，不运行全仓 release gate，不修改业务代码。

## 5. 权威资料与证据优先级

允许引用：

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

证据优先级：

1. 当前分支可重复运行的测试、构建、静态检查和当前 live 结果。
2. 当前分支代码与可追溯 commit/diff。
3. 旧 Goal-3 原始 fixture、wire evidence、pagination/mutation 报告。
4. 旧 contract/compatibility matrices。
5. 旧 `tasks.md` 完成记录。
6. 旧 capability 单字段状态。

后一层不能覆盖前一层冲突事实。历史 `verified` 与当前代码冲突时，以当前事实为准。

## 6. 当前状态模型

`goal-1/current-state.md` 是执行阶段的当前状态权威表。每个 required capability 记录：

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

- `verified`
- `implemented_unverified`
- `missing`
- `blocked_external`
- `blocked_decision`
- `not_applicable`

`not_applicable` 必须有 contract 理由。`public_ready` / `release_ready` 只能派生计算，不再手工维护。

一个 capability 只有同时满足以下条件才 `accepted`：

1. 所有适用 layer 为 `verified` 或有理由的 `not_applicable`。
2. 对应 offline correctness acceptance 通过。
3. 对应 SDK/CLI/MCP compatibility 通过或该 surface 明确不适用。
4. forbidden endpoint / no-fallback 断言通过。
5. release contract 要求 live 时，Live 必须 `verified`。
6. 不存在映射到它的 open P0/P1 correctness finding。

`blocked_external` 不能产生 accepted。

## 7. 防过度设计规则

所有实现 task 默认采用 **minimum sufficient change**：只实现当前 frozen acceptance 所需的最小改动。

禁止：

- 因“以后可能用到”新增 abstraction、framework、configuration layer、plugin system 或通用 retry system。
- 为单个当前用例设计泛化 DSL、通用 registry 或新的跨包架构。
- 在没有当前 acceptance 驱动时做 repo-wide rename、package ownership 移动或公共 API redesign。
- 把局部 bugfix 扩成“顺便清理整个模块”。
- 仅为了让代码看起来统一而改动未触及的稳定路径。

允许抽象的条件：当前 task 已有两个或以上真实调用点需要同一语义，且复用现有 helper 不能合理表达；即使满足，也优先最小局部 helper，不扩大 public surface。

Refactor 阶段只允许：

- 清理本 task 引入的重复或明显可读性问题。
- 复用已有 abstraction。
- 修复与当前 acceptance 直接相关的 ownership 问题。

如果更大重构看起来有价值但不是当前 acceptance 必需，记录到 `out-of-scope observations`，不进入本 Goal。

## 8. 防过度测试规则

测试目标是证明 acceptance，不追求测试数量或覆盖率数字。

### 8.1 代码 leaf task 的最低充分验证

每个生产代码 task：

1. 一个能够行为性证明当前缺口的最小 Red；同一根因不重复堆多个等价 Red。
2. Green 后运行该测试。
3. 运行受影响 package 的现有相关 tests。
4. 只有跨 package/wire/cursor/serialization 行为被触及时，才运行对应 integration/compatibility tests。

不得每张 leaf task 都运行 `go test ./...`、全 MCP replay、全 CLI regression 或全 race。

### 8.2 不要求的测试

除非 frozen contract 或已观察 bug 明确需要，否则不要求：

- 为 trivial pass-through/wrapper 重复添加单元测试。
- 为相同 DTO 语义建立多套等价 fixture。
- 穷举所有 flag/query 组合。
- 为已有稳定 helper 重新补覆盖率。
- 为未改动 package 执行 race test。
- 追求 arbitrary coverage percentage。

### 8.3 集中 gate

全量验证集中执行：

- Phase gate：只跑该阶段相关 regression。
- Offline release candidate：`go test ./...`、`go vet ./...`、`sh scripts/build.sh`、必要 race、兼容 replay、docs、forbidden endpoint、redaction。
- Final closure：不重复运行已经在同一 HEAD 上通过且未被后续改动 invalidated 的相同 gate。

只要 task acceptance 已被最小充分证据关闭，就停止增加测试。

## 9. Live manifest 必须提前冻结

在进入任何 live task 前，必须在 `goal-1/current-state.md` 建立 `Live Manifest`。每一项至少记录：

- capability
- `live_required: yes/no`
- endpoint/path family
- required scenario
- 是否要求 second-page continuation
- 所需账号/目标数据条件
- mutation 时的 read-back/cleanup 要求
- 当前 evidence 或 blocker

Luna 不允许在 live phase 临时决定“哪些 capability 应该 live”。只有 frozen manifest 中 `live_required=yes` 的条目进入 required live gate。

## 10. 执行阶段

### Phase A — Baseline / manifest

目标：完成 preflight，分组盘点 41 capability，复核 correctness，冻结 live manifest 和有限 execution mapping。

出口：

- required=41。
- unmapped=0。
- undecomposed=0。
- live manifest 已冻结。
- 所有内部已知 gap 有具体 task/correction owner。

### Phase B — MCP read convergence

按 user、bookmark typed read、bookmark aggregate、registration/schema/replay 拆分，不再由一个 task同时承担全部 read owner。

出口：required MCP read layer 有专项 evidence。

### Phase C — MCP mutation convergence

按 bookmark、artwork comment/stamp、novel comment/stamp、follow 与 mutation harness 拆分。

出口：offline mutation outcome、uncertain-result、wire compatibility 全部关闭；live 留给 Phase F。

### Phase D — Compatibility / release contract

cursor integrity、SDK compatibility、CLI compatibility/presentation、MCP compatibility、docs/Skill 分开执行。

出口：不存在未处理 breaking decision；forbidden endpoint contract 仍成立。

### Phase E — Offline release candidate

protocol/SDK、CLI、MCP regression 分开，再执行一次 full test/vet/build/必要 race/redaction gate。

出口：内部 `missing=0`、无理由 `implemented_unverified=0`、open P0/P1=0。

### Phase F — Live / closure

按 live manifest 分 read families 和 mutation。最后重新计算 41 capability acceptance 并生成 closure report。

## 11. Leaf task admission

普通 task 进入 `in_progress` 前必须明确：

- 单一 owner/slice。
- capability 集合。
- depends_on。
- 允许修改的层/主要区域。
- acceptance。
- 代码 task 的最小 Red。
- 最小 Green/regression。
- 兼容影响。
- 回滚边界。

禁止 task：

- “按 owner 再拆”。
- “修剩余问题”。
- “让所有测试通过”。
- “完成全部兼容”。
- “继续调查直到没问题”。

若任务开始前仍需要执行器自行设计多个独立 owner，说明任务不是 leaf，必须在 `tasks.md` 预拆后才能执行。

## 12. Correction task 必须抢占后续任务

检查、回归或 live 可以新增 correction，但必须绑定既有 acceptance failure，格式 `G1-CORR-<来源>-NN`，记录：

- Source task/gate
- Capability
- Observed failure
- Expected contract
- Scope boundary
- Red command / expected failure（生产代码变更时）
- Green acceptance
- Compatibility impact
- Rollback boundary

调度规则：

1. 新 correction 必须插入到**当前 task 后、原下一 task 前**，不能简单追加到文件末尾。
2. correction 成为下一张 pending executable task，优先于后续 phase。
3. correction 依赖只能指向已完成必要前置，不得引入新 capability。
4. correction 若让已完成 gate 失效，受影响 gate 与 closure task 重置为 `pending`。
5. correction 完成后回到原执行序列。

这样允许修 bug，但不允许形成无限产品 scope。

## 13. Blocking 与通用终止路径

Task 终态：`verified` / `blocked_external` / `blocked_decision`。

### `blocked_external`

只允许真正环境/外部条件：

- 用户明确要求的 Caveman skill 在执行环境不可用。
- 账号/权限不可用。
- 网络或上游服务不可用。
- 必需目标数据不存在或不可安全构造。

不能用“测试难写”“还没调查”“代码看起来复杂”作为 external blocker。

### `blocked_decision`

只用于必须取得用户授权才能继续的 breaking API、scope change、安全/权限策略变化。

### 通用 `G1-TERM` terminalization

`G1-TERM` 是特殊控制任务，不属于正常 Phase 序列。它可以在**任何阶段**抢占执行，不要求先到 Phase F。

允许执行条件：

```text
runnable_required_tasks == 0
AND (required_external_blockers > 0 OR required_decision_blockers > 0)
```

它只做：

- 汇总 blockers。
- 验证没有仍可执行的内部 task/correction。
- 写 `closure-report.md`。
- 计算 `BLOCKED_EXTERNAL` 或 `BLOCKED_DECISION`。

如果 decision 和 external 同时存在，报告两者，GoalState 使用 `BLOCKED_DECISION`，因为恢复执行首先需要用户决策。

`G1-TERM` **绝不能**把 Goal 标记完成。

如果 blocker 后续解除，相关 task 恢复为 `pending`，从最早未完成 task继续；旧 blocked closure report 标记为 superseded。

## 14. 严格终止状态机

GoalState 只有：

- `ACTIVE`
- `BLOCKED_EXTERNAL`
- `BLOCKED_DECISION`
- `COMPLETED`

### COMPLETED 必要且充分条件

```text
COMPLETED :=
  required_capabilities.count == 41
  AND required_capabilities.all(accepted)
  AND required_tasks.pending == 0
  AND required_tasks.in_progress == 0
  AND required_blockers == 0
  AND undecomposed_tasks == 0
  AND correctness_p0_p1_open == 0
  AND cursor_integrity_gate == PASS
  AND sdk_compat_gate == PASS
  AND cli_compat_gate == PASS
  AND mcp_compat_gate == PASS
  AND protocol_sdk_regression == PASS
  AND cli_regression == PASS
  AND mcp_regression == PASS
  AND full_offline_gate == PASS
  AND required_live_gate == PASS
  AND documentation_gate == PASS
  AND redaction_gate == PASS
  AND final_closure_audit == PASS
```

额外约束：

- `accepted_count` 必须恰好 41。
- 任一 required blocker 存在都不能 COMPLETED。
- 所有 live-required capability 必须有当前 live evidence。
- 最终 closure 不得出现新的既有 required acceptance failure。
- Final closure 不重复运行同一 HEAD 已通过、且之后未被相关改动 invalidated 的昂贵 gate。

### BLOCKED_EXTERNAL

只有在没有可执行内部 task/correction、所有可离线解决的 gap 已关闭、剩余 required acceptance 唯一缺口都是真实 external blocker 时成立。

### BLOCKED_DECISION

只有继续安全执行必须取得用户明确决策时成立。不是完成。

## 15. Compatibility 原则

默认继续旧计划冻结策略：

- public Go SDK 尽量源码兼容。
- CLI 保留必要 canonical/legacy alias 与 deprecation 行为。
- MCP 保留旧 tool/input/output wire compatibility，并以 legacy JSON replay 验证。
- `NovelContent` 等无 App API replacement 的旧 symbol 可以保留明确兼容错误，但不得请求 rejected endpoint。
- cursor version/binding 变化必须显式失败或迁移，不能静默从第一页重启。

必须 breaking 时进入 `blocked_decision`，不由执行 Agent 自行实施。

## 16. 回滚原则

- 不通过大规模 revert 重写旧 WIP 历史。
- 每个 task 只修改其 leaf slice 必需部分。
- 旧实现错误优先最小 correction。
- public API/CLI/MCP 变更记录 blast radius。
- cursor/serialization 变更说明跨版本恢复或明确受控失效。
- mutation/live 只清理本轮可识别副作用。

## 17. 最终产物

至少维护：

- `goal-1/input.md`
- `goal-1/plan.md`
- `goal-1/tasks.md`
- `goal-1/current-state.md`
- `goal-1/closure-report.md`

`closure-report.md` 是唯一允许声明最终 GoalState 的文档，必须能回溯到 `current-state.md`、`tasks.md` 和验证证据。

只有 `GoalState: COMPLETED` 才允许把 Goal 在客户端标记为完成。`BLOCKED_EXTERNAL` / `BLOCKED_DECISION` 只允许停止无人值守推进。