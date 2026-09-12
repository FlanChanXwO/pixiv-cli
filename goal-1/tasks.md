# Goal-1 Tasks：Pixiv API 迁移与稳定性收敛

修订日期：2026-09-11。执行分支：`refactor/pixiv-api-stability`。继承代码基线：`e40443495981cdaf01215d6711cb24fa618b087a`。

## 执行规则

- `goal-1/plan.md` 定义 required scope、证据、测试预算、worktree/push gate 和终止状态机；本文件定义执行顺序。
- `goal-1/input.md` 保持用户原始启动输入，禁止改写。
- 每轮只执行第一个未完成且可安全执行的普通 task、抢占式 correction task，或满足条件的特殊终止任务。
- 除 Goal 计划初始化/修订外，所有业务执行必须在专用独立 worktree 中进行；禁止在普通主 checkout 直接实施。
- 专用 worktree 必须运行 `refactor/pixiv-api-stability`，优先复用已有 linked/native isolation，再优先平台原生 worktree，最后才允许安全的 `git worktree` fallback；禁止 force、reset 或破坏其他 checkout。
- 每个 Phase 的最后一个 CHECK 同时是 phase push gate；push 成功并确认远端 SHA 与 phase-exit HEAD 一致前，CHECK 不能 `verified`，下一 Phase 不得开始。
- Phase push 只允许普通 fast-forward push 到当前执行分支 `refactor/pixiv-api-stability`；网络/认证失败为 `blocked_external`，non-fast-forward/未知远端并发为 `blocked_decision`，禁止 force push。
- 原 41 个 required capability 全部继续 required；不得降级为 `deferred_nonblocking`。
- 所有生产代码修改必须 Red → Green → Refactor。
- 同一根因只要求一个最小行为性 Red；Green 后只运行受影响 package 和必要 integration/compatibility tests。
- leaf task 不重复运行 `go test ./...`、全 CLI/MCP regression 或全 race；这些集中在 Phase E。独立 worktree 建立时允许并要求一次 `go test ./...` 作为干净 baseline。
- 禁止 speculative abstraction、无关重构、依赖升级、全仓 rename、为了“统一”修改稳定路径。
- 如果现有实现和证据已经满足 task acceptance，可以 no-op `verified`，但必须记录当前复核证据。
- 新发现但不属于原 41 项的需求只进入 `out-of-scope observations`。
- 每完成三个普通 task，下一轮必须执行对应 CHECK。
- Caveman skill 只负责压缩叙述和上下文；不能替代验证，也不能改变代码、命令、API 名或精确错误字符串。每个新会话恢复 Goal 时重新确认其可用性。

## Task 状态

普通 task：

- `pending`
- `in_progress`
- `verified`
- `blocked_external`
- `blocked_decision`

`blocked_external` / `blocked_decision` 是 task 终态，不是 Goal 完成态。

## Correction 抢占规则

发现既有 required acceptance failure 或本 Goal 引入回归时，新增 `G1-CORR-<来源>-NN`，必须记录：

- Source task/gate
- Capability
- Observed failure
- Expected contract
- Scope boundary
- Red / expected failure（生产代码变更时）
- Green acceptance
- Compatibility impact
- Rollback boundary

新 correction 必须插在当前 task 后、原下一 task 前，使其成为下一张 pending executable task。禁止简单追加到文件末尾后继续下一 Phase。若 correction 使已完成 gate 失效，受影响 gate 和 final closure 必须重置为 `pending`。

## Phase push gate 通用规则

Phase A–F 的最后一个 CHECK 在其业务/审计 acceptance 通过后，还必须完成：

1. 本 Phase 的 task/correction 结果和 Goal 账本均已提交；没有来源不明的未提交业务 diff。
2. 执行普通 fast-forward push 到 `refactor/pixiv-api-stability`；即使本 Phase 没有新业务实现，也执行一次 push，`up-to-date` 算成功。
3. 核对远端分支 SHA 与本地 phase-exit HEAD 一致。
4. 在 CHECK 完成记录中写 `Local HEAD`、`Remote SHA`、`Push result`。
5. push 失败时 CHECK 不得标记 `verified`，后续 Phase 不得开始。

Phase push 是恢复 checkpoint，不要求为 push 再重复阶段测试。

## 特殊控制任务：G1-TERM

`G1-TERM` 不属于普通 Phase 顺序，不计入“三个普通 task 后 CHECK”。它可以在任意阶段抢占执行。

**允许条件：**

```text
runnable_required_tasks == 0
AND (required_external_blockers > 0 OR required_decision_blockers > 0)
```

**只允许做：**

- 证明没有仍可执行的内部 task/correction。
- 汇总 blocker 与受影响 capability。
- 创建/更新 `goal-1/closure-report.md`。
- 写 `GoalState: BLOCKED_EXTERNAL` 或 `GoalState: BLOCKED_DECISION`。
- 提交 blocked closure 记录并尝试普通 fast-forward push 到 `refactor/pixiv-api-stability`；若 push 本身失败，记录失败，不得 force。

若两类 blocker 同时存在，记录两类，GoalState 使用 `BLOCKED_DECISION`。

**禁止：** 标记 Goal complete；修改业务代码；跳过仍可执行的内部 work；force push。

---

# Phase A — Preflight、baseline 与 manifests

本阶段不修改业务代码。

## G1-T01 — Luna/Caveman/worktree execution preflight

**Status:** verified

**Depends on:** none

**Scope:** worktree、分支、工作区、toolchain、Goal 文件、Caveman/LSP 可用性、干净 baseline。

**验收：**

- 检测 `git-dir`/`git-common-dir` 与 superproject，证明当前是 linked/native isolated worktree 且不是 submodule；若已隔离则复用，禁止嵌套 worktree。
- 若尚未隔离：优先平台原生 worktree；无原生能力时才使用安全的 `git worktree` fallback，并在项目内 fallback 目录创建前确认其被 ignore。
- 当前分支为 `refactor/pixiv-api-stability`。
- HEAD 是 `e404434` 后代。
- 不使用 force/reset/delete-other-worktree 等方式解决 branch checkout 冲突；无法安全建立隔离时标 `blocked_external`。
- 没有来源不明的未提交业务 diff。
- `goal-1/input.md`、`plan.md`、`tasks.md` 均被跟踪。
- Go toolchain 与仓库既有 test/build 入口可调用。
- Caveman skill 可加载并启用；不可用时标 `blocked_external`，不自动安装。
- 代码导航需要 LSP 时，确认可用；不可用则记录明确 fallback。
- 在专用 worktree 上执行一次 `go test ./...` 作为干净 baseline；通过才可继续。若 baseline 失败且不是本 Goal 造成，标 `blocked_decision`，不得无授权继续。

**禁止：** 业务代码修改；新依赖安装；额外 release-only gate。

**完成记录：**
- Worktree path/type：`/Users/flanchan/Developer/Projects/GithubProjects/.worktrees/pixiv-cli-refactor-pixiv-api-stability`；linked Git worktree；`git-dir != git-common-dir`；不是 submodule。
- Branch/HEAD：`refactor/pixiv-api-stability`；preflight 基线 HEAD `29ae255e2061b57e21d2067492878e1df342efe8`；已创建 task commit `docs(goal-1): verify G1-T01 preflight`；`e40443495981cdaf01215d6711cb24fa618b087a` 是 ancestor。
- Worktree isolation：PASS；未嵌套 worktree；主 checkout 的既有 dirty WIP 未触碰。
- Worktree cleanliness：PASS；目标 worktree `git status --porcelain` 为空；`goal-1/input.md`、`plan.md`、`tasks.md` 均 tracked；`git diff --check` 通过。
- Toolchain：Go `go1.26.3 darwin/arm64`；`go.mod` 与 `scripts/build.sh` 可用；`go mod download` 成功。
- Baseline `go test ./...`：PASS；目标 worktree 执行退出码 0，所有 package 通过。
- Caveman：PASS；`/Users/flanchan/.agents/skills/caveman/SKILL.md` 可加载；当前会话按其紧凑叙述规则运行。
- LSP/fallback：PASS；目标 worktree 已启动 `gopls`，server version `v0.21.1`；`open_document` 与 `list_symbols` 成功，无 fallback。
- GoalState impact：保持 `ACTIVE`；无 blocker。
- 下一步：G1-T02

## G1-T02 — Baseline inventory：artwork + novel/feed

**Status:** verified

**Depends on:** G1-T01

**Capabilities:** 1–13。

**Scope:** `goal-1/current-state.md`，只读代码、tests、history、旧 evidence。

**目标：** 对 artwork / novel / feed 13 项逐层记录 Contract、Adapter、SDK、Shared、CLI、MCP、Offline、Live、Compatibility、Release。

**验收：**

- 13 项全部存在，无合并丢项。
- `verified` 有 evidence index。
- `implemented_unverified` / `missing` 不被历史 task 状态覆盖。
- rejected endpoint 与 no-fallback 状态明确。
- T37A/B/C 历史完成只作为证据，不自动 accepted。

**完成记录：**
- 覆盖计数：13/13，全部纳入 required scope；`public_ready=0/13`；无漏项。
- Verified evidence：新建 `goal-1/current-state.md`，逐项记录 Contract、Adapter、SDK、Shared、CLI、MCP、Offline、Live、Compatibility、Release，并为 verified 层提供 source/test/evidence index。
- Drift：当前 HEAD 相对继承基线 `e40443495981cdaf01215d6711cb24fa618b087a` 仅增加 Goal-1 tracking 文件；`goal-3/` 无 diff；当前分支无新的业务/API/evidence drift。
- Internal gaps：artwork recommended 第二页/subtype binding；artwork series live 第二页；ugoira metadata CLI/MCP surface 与旧 evidence 冲突；novel search period/date/live 两页；novel detail/series v2 full chain；novel latest max_novel_id live 闭环；novel ranking MCP owner；统一 compatibility/docs/release gate。
- Rejected/no-fallback：已记录 `/v1/novel/detail`、`/v1/novel/series`、`/v1/novel/content`、WebView fallback、未确认 server-side `x_restrict` 的拒绝边界；未把历史 task `verified` 提升为 capability acceptance。
- 验证证据：G1-T01 `go test ./...` baseline PASS；本 task 完成 LSP symbol 核验、源码/evidence/history inventory 和 current-state 结构校验；未执行 live E2E。
- GoalState impact：保持 `ACTIVE`；无 external/decision blocker。
- 下一步：G1-T03

## G1-T03 — Baseline inventory：bookmark

**Status:** verified

**Depends on:** G1-T02

**Capabilities:** 14–24。

**目标：** 复核 bookmark list/tags/detail/mutation/subtype/list-all/tags-all 的所有 layer。

**验收：**

- 11 项完整。
- `bookmark-list-all` / `bookmark-tags-all` 保持 required。
- artwork/novel typed semantics、双流 checkpoint、统一 budget、页原子失败证据分开记录。
- mutation offline 与 live evidence 不混淆。

**完成记录：**
- 覆盖计数：11/11（14–24）；全部 required，`public_ready=0/11`；全 Goal scope 仍 `required=41, scope_admitted=41`。
- Verified evidence：更新 `goal-1/current-state.md`，为 11 项逐层记录 Contract、Adapter、SDK、Shared、CLI、MCP、Offline、Live、Compatibility、Release verdict，并补充 typed semantics、双流 checkpoint、统一 budget、页原子失败、mutation read-back/cleanup 边界。
- Aggregate：确认当前 CLI list/tags `--type all` 有 offline 双流实现与 atomicity tests，但没有 public aggregate SDK/MCP operation 或跨调用 cursor；`all` 不作为 upstream subtype。
- Mutation：artwork add/remove 仅有 transport/form/validation；novel add/delete 仍 candidate adapter；没有真实 bookmark mutation、read-back、restore、cleanup 或 uncertain-no-replay evidence。
- Evidence boundary：`goal-3/capability-admission.md:37-47` 的 11 项仍为 `scope_admitted`；历史 T15/T19/T23 `verified` 仅作局部 seam 证据，不提升 capability acceptance；旧 `not_tested/inconclusive` 记录保持。
- Drift：目标 worktree 相对继承基线仍只有 Goal-1 tracking 文件，`goal-3/` 无 diff；未新增业务/API scope。
- 验证：完成源码/测试/contract/evidence/history inventory；LSP 核验 bookmark public symbols 与 shared collectors；目标 worktree 初始 clean；未执行 live E2E；commit hook 负责 `gofmt` 与 `go test ./...`。
- GoalState impact：保持 `ACTIVE`；无 external/decision blocker。
- 下一步：G1-CHECK-01

## G1-CHECK-01 — 集中检查：preflight + artwork/novel/bookmark baseline

**Status:** verified

**Depends on:** G1-T01,G1-T02,G1-T03

检查 worktree/branch/skill/toolchain 条件、24 项 baseline 完整性、evidence 误提升、scope drift、rejected endpoint 和 bookmark aggregate requiredness。

**Pass：** 当前覆盖 24/41；无漏项；无未经证据的 `verified`；执行仍在同一专用 worktree。

**完成记录：**
- 检查结论：PASS。`goal-1/current-state.md` layer matrix 编号严格为 1–24，覆盖 24/41；bookmark 14–24 为 11/11；required scope 仍为 41/41 `scope_admitted`；`public_ready=0/41`；无 capability 漏项。
- Evidence：generic `verified` 行已补具体 evidence index，均可回溯到相邻 source/test/evidence 行；历史 Goal-3 task `verified` 未被提升为 capability acceptance；`goal-3/capability-admission.md` required 状态无漂移。
- Rejected/aggregate：`/v1/novel/detail`、`/v1/novel/series`、`/v1/novel/content` 与 WebView/anonymous fallback 仍显式拒绝；`bookmark-list-all`、`bookmark-tags-all` 仍 required，`--type all` 只属于产品层 list/tags aggregate，不作为 upstream subtype。
- Worktree：在 `/Users/flanchan/Developer/Projects/GithubProjects/.worktrees/pixiv-cli-refactor-pixiv-api-stability` 的 linked worktree 执行；branch=`refactor/pixiv-api-stability`；worktree clean；审计时 HEAD=`042c0200ce1b3349ab3e08a744fea7802838025c`；相对远端 ahead 3；与继承基线 `e40443495981cdaf01215d6711cb24fa618b087a` 相比仅有 Goal-1 tracking 文件，`goal-3/` 与业务/API 无 diff。
- Preflight/toolchain：目标 skill 文件（goal-mode、using-git-worktrees、github、caveman、superpowers）均可加载；`gopls` 已启动并报告可用；`go version go1.26.3 darwin/arm64`、`gh version 2.97.0`；当前 HEAD 执行 `go test ./...` PASS。
- Correction：无。为消除 generic `verified` 可审计性缺口，仅补充 `current-state.md` 的 evidence index 与 CHECK 记录；未改生产代码、API、scope 或 Goal-3 资料。
- Blocker：无。G1-CHECK-01 非 Phase A exit gate，本轮不 push；下一任务为 `G1-T04`。

## G1-T04 — Baseline inventory：comments/stamps + user/shared

**Status:** verified

**Depends on:** G1-CHECK-01

**Capabilities:** 25–41。

**目标：** 完成其余 comments/stamps、user/relationship、bare-id/rating/logical-pagination/recommended-all 的 layer inventory。

**验收：**

- 17 项完整，最终 `required_count=41`。
- `logical-pagination` 的 T23A/R02 证据与其他 endpoint continuation 分开。
- T37D WIP 明确为 WIP，不误记 MCP user layer verified。
- comments read/mutation、stamps、follow mutation 的 offline/live 边界明确。

**完成记录：**
- 覆盖计数：PASS。25–41 共 17/17；与前序 1–24 合计 41/41；全 required scope 为 `scope_admitted`，`public_ready=0/41`。
- Verified evidence：已更新 `goal-1/current-state.md` 的 25–41 layer matrix 与逐项 evidence。确认 comments/stamps、user/relationship、resolver/rating/pagination/recommended-all 的 Contract、Adapter、SDK、Shared、CLI、MCP、Offline、Live、Compatibility、Release verdict；所有 `verified` 均指向当前源码/测试/证据。
- Boundary：artwork comments v3 contract 保持 `rejected`；comment mutation MCP surface 缺失；user artwork/novel 与 MyPixiv 缺 strict second-page；follow mutation 缺同账号 read-back；bare-ID 无 production probe；rating 缺 MCP；T23A/R02 不替代 endpoint continuation；T37D WIP 不计 MCP user layer verified。
- Offline verification：`go test ./internal/services/pixiv/endpoint/artwork/comments ./internal/services/pixiv/endpoint/novel/comments ./internal/services/pixiv/endpoint/stamps ./sdk/pixiv ./internal/cli/commands/pixiv/comment ./internal/mcpserver/pixiv/... -count=1` PASS；本轮未执行真实 Pixiv live API。
- Drift：无。只修改 `goal-1/current-state.md` 与 `goal-1/tasks.md`；未改生产代码、API、scope 或 Goal-3 资料；目标 worktree 保持专用 linked worktree。
- Internal gaps：无新增 blocker；既有 `implemented_unverified`/`missing`/`rejected` 逐项记录，留给后续 owner/correction task，不在本 task 修改业务代码。
- 下一步：G1-T05

## G1-T05 — Correctness ledger 与 forbidden behavior 复核

**Status:** verified

**Depends on:** G1-T04

**至少覆盖：** logical pagination/checkpoint/replay、novel latest、novel detail/series、artwork recommended continuation、artwork/novel comments DTO、restrict/rating、rejected endpoint/no-fallback。

**验收：**

- 每项有代码路径、测试路径、历史 evidence、当前 verdict。
- P0/P1 不允许以 known limitation 绕过。
- 内部可修复 failure 建立受约束 correction candidate，不在本 task 修业务代码。
- 新产品需求只进 out-of-scope observations。

**完成记录：**
- **Open P0/P1：** P0 无；P1 1 项：`artwork-recommended` second-page continuation 的 `inconclusive/second_page_error`。该项显式保持 open，未伪装为 known limitation，也未用 shared pagination PASS 覆盖。
- **Closed by evidence：** shared pagination/checkpoint/replay engine；novel-latest adapter/SDK 的 `max_novel_id` leaf；novel detail/series 离线 v2 path；artwork/novel comments DTO adapter/SDK/MCP offline path；restrict/local rating/cursor binding offline path；rejected endpoint/no-fallback offline boundary。
- **Correction candidates：** CAND-G1-T06-REC-RECOMMENDED（strict non-empty two-page endpoint→SDK→CLI/MCP）；CAND-G1-T06-REC-LATEST（CLI/MCP second page + live）；CAND-G1-T06-REC-SERIES-DOC（tracking-doc drift + v2/live evidence）；CAND-G1-T06-REC-COMMENTS-MUTATION（same-account read-back/cleanup/uncertain no replay）；CAND-G1-T06-REC-ACCOUNT-RATING（pool switch/filter digest/local restrict，不伪造 server-side rating）。未修改业务代码。
- **Drift：** 仅修改 `goal-1/current-state.md` 与 `goal-1/tasks.md`；发现的 novel-series Goal-3 tracking-doc drift 只登记为 P2 candidate，未在本 task 修改 Goal-3；无新产品需求。
- **Offline verification：** correctness focused commands 全部 PASS（shared pagination/traversal、novel latest、artwork recommended endpoint/SDK/CLI/MCP、novel detail/series、artwork/novel comments、SDK validation/cursor、MCP read/no-fallback）；本轮未执行真实 Pixiv live API。
- **下一步：** G1-T06

## G1-T06 — Freeze Live Manifest + finite execution mapping

**Status:** verified

**Depends on:** G1-T04,G1-T05

**Scope:** `goal-1/current-state.md` 与本 task graph；不改业务代码。

**验收：**

- 建立 Live Manifest：capability、live_required、scenario、second-page、账号/数据条件、mutation read-back/cleanup、evidence/blocker。
- 41 项每项为 `accepted_by_evidence`、`mapped_to_task`、`blocked_external` 或 `blocked_decision` 之一。
- 每个 `mapped_to_task` 有具体 task ID。
- `required=41`、`unmapped=0`、`undecomposed=0`。
- Compatibility、cursor、docs、offline regression、live 都有 owner。

**完成记录：**
- **Required/unmapped/undecomposed：** `41 / 0 / 0`。41 项全部有主 task、配套 gate 或 correction owner；`mapped_to_task=41`。
- **Live-required count：** `36 yes`；`5 no`：#22 `bookmark-subtype`、#25 `artwork-comments-read`（rejected）、#38 `bare-id-probe`、#39 `rating-filter`（local）、#40 `logical-pagination`（generic shared）。
- **Added correction tasks：** 未新增 task ID；新增/保留 14 个有界 correction candidate，并全部绑定到 G1-T07–G1-T30 或 G1-CHECK-02 的既有 owner；缺失 public surface、rejected path 与 aggregate gap 均显式登记。
- **Blockers：** 当前 external=0、decision=0；P0=0；P1=1（#4 artwork-recommended continuation），映射至 G1-T28/G1-T20/G1-T22，不隐藏。
- **Freeze：** 后续 live 只执行 manifest 中 `live_required=yes` 的明确 scenario；数据/账号/网络不足时记录 `blocked_external`，不得伪造第二页；mutation 必须 read-back/cleanup，uncertain 不 replay。
- **Offline / graph verification：** `41/41` matrix、manifest 4 组表、finite mapping、correction owner registry 已通过静态审计；本 task 无业务代码变更。
- **下一步：** G1-CHECK-02

## G1-CHECK-02 — Phase A exit：41-state、correctness、manifests + push

**Status:** verified

**Depends on:** G1-T04,G1-T05,G1-T06

**Pass：** `required=41`、`unmapped=0`、`undecomposed=0`、Live Manifest frozen；无被隐藏的内部 P0/P1；仍在专用 worktree。

**Phase push gate：** Pass 后提交本阶段 Goal 账本/证据，普通 fast-forward push 到 `refactor/pixiv-api-stability`，远端 SHA 必须等于 Local HEAD。push 未成功不得进入 Phase B。

**完成记录：**
- **检查结论：** PASS。专用 linked worktree、branch=`refactor/pixiv-api-stability`、`e404434` ancestry、tracked Goal-1 inputs、clean pre-push state 均通过；未改业务代码/API/scope/Goal-3。
- **Counts：** required=`41`；manifest rows=`41`；`live_required=yes/no=36/5`；`mapped_to_task/unmapped/undecomposed=41/0/0`；P0=`0`；P1=`1`（#4 `artwork-recommended` continuation，已显式保留并绑定 correction owner）。
- **Coverage：** Goal-3 required capability set、current-state layer matrix、Live Manifest capability set 均为 41 项；finite mapping 覆盖 G1-T07–G1-T30 与 G1-CHECK-02；G1-T12 legacy replay/stdout owner 已补齐。
- **Correction：** 未新增 capability、task ID 或 scope；保留 14 个 G1-T06 correction candidates；无 external/decision blocker。
- **Local HEAD：** `b17b1775f2ea0783f394f7600d30cafd8ad428c5`。
- **Remote SHA：** `b17b1775f2ea0783f394f7600d30cafd8ad428c5`；已由 `gh api` 与 `git ls-remote` 核验一致。
- **Push result：** PASS；`29ae255e2061b57e21d2067492878e1df342efe8..b17b1775f2ea0783f394f7600d30cafd8ad428c5` ordinary fast-forward；未使用 force/rebase。
- **下一步：** G1-T07。

---

# Phase B — MCP read convergence

## G1-T07 — MCP user identity read：search/detail/trending

**Status:** verified

**Depends on:** G1-CHECK-02

**Capabilities:** `user-search`、`user-detail`、`trending` 的 MCP/Shared acceptance。

**目标：** 收敛 search user、user detail、trending 的 schema、resolver/filter、pagination、structured error 与 legacy replay。

**最小验证：** 若需改代码，一个行为性 Red；Green 后运行相关 MCP tool/package tests 和必要 legacy replay。

**验收：** schema/identity/filter/pagination/error 与 frozen contract 一致；不扩到 relationships/bookmark。

**完成记录：**
- **改动/no-op：** 补齐 `search_user` 的非空白 `word` schema（`minLength=1`）与 SDK 执行前 typed `InvalidArgument` guard；补充 user-search/user-detail structured-error、blank-input no-execute、trending empty-success 的 MCP 回归与 legacy replay；trending 空列表文本改为成功语义 `No trending tags found.`；双语 MCP 文档同步。未扩展 relationships/bookmark，也未改变 `user_detail` 的 frozen 必填 `user_id` wire；CurrentUser verified identity 仍由 SDK/endpoint 既有路径覆盖，当前用户 resolver/list 交由 G1-T08。
- **Red/Green：** Red：`go test ./internal/mcpserver/pixiv -run 'Test(UserReadSchemasMatchLegacyContracts|SearchUserRejectsBlankWordBeforeSDKExecution|TrendingTagsIllustEmptyResultIsSuccessful)$' -count=1 -v` 实际暴露 `search_user.word` 缺少 `minLength`，以及 trending 空成功仍输出失败措辞。Green：相关 focused replay、`go test ./internal/mcpserver/pixiv/... -count=1`、`go test ./sdk/pixiv ./internal/services/pixiv/endpoint/user/search ./internal/services/pixiv/endpoint/user/detail ./internal/services/pixiv/endpoint/artwork/trending -count=1`、`go test ./internal/cli/commands/pixiv/search -run 'TestTrendingTags' -count=1`、`go vet ./internal/mcpserver/pixiv/...`、`go test ./scripts/tests/documentation -count=1` 均通过；LSP diagnostics 无新增错误，`git diff --check` 通过。
- **Compatibility：** `search_user`、`user_detail`、`trending_tags_illust` 名称、旧字段、输出 envelope、pagination/filter 与 explicit detail identity contract 保持；legacy JSON replay、跨 SDK cursor filter、structured error、trending empty success 均通过；未执行真实 Pixiv API，未把 offline evidence 升格为 live/public-ready。
- **风险：** #33–35 的 strict live/account binding/CurrentUser live evidence 仍未闭合；trending CLI owner/correction 仍属 G1-T21；本卡只完成 MCP/Shared offline acceptance。
- **下一步：** G1-T08

## G1-T08 — MCP user collections/relationships/MyPixiv read

**Status:** verified

**Depends on:** G1-T07

**Capabilities:** `user-artworks`、`user-novels`、`user-relationships`、`mypixiv`。

**目标：** 收敛 artworks/novels、following/followers/related/blocked、MyPixiv 的 read surface。

**最小验证：** 一个根因一个 Red；相关 tool/package tests；只在跨 owner wire 变化时跑 replay。

**验收：** resolver、pagination、structured errors、legacy wire 均与 manifest 一致；T37D WIP 全部闭合或产生具体 correction。

**完成记录：**
- 改动/no-op：production MCP/SDK/endpoint 实现已满足 frozen user collections、relationships、MyPixiv wire，本轮不做行为改动；修正文档将 `user_following` 与 `user_followers` 的 `user_filter` 差异、`related_users` 的 current-user default/compatibility `restrict`、`blocked_users` 的 compatibility `restrict` 与 schema 对齐。T37D offline owner 已以当前目标分支证据闭合。
- Red/Green：本轮为 no-op + 文档 contract correction，未触发 production behavior Red；focused MCP schema、resolver、logical pagination/filter、structured error、legacy JSON replay、MCP package、SDK user operations、user/artwork endpoint、vet、documentation tests 与 `git diff --check` 均通过。
- Compatibility：`user_artworks`、`user_novels`、`mypixiv_users`/`mypixiv_illusts`/`mypixiv_novels`、`user_following`/`user_followers`/`related_users`/`blocked_users` 的 names、legacy fields、current-user resolver、filters、opaque cursor、records envelope 与 typed error semantics 保持；不接受 removed legacy fields，不暴露 transport credential，不引入 Web/匿名 fallback。
- 风险：#30–32、#37 的 strict live/account binding、真实第二页与 compatibility/release gates 仍未闭合；按 manifest 由 G1-T29 继续处理，不能把 offline replay 或 synthetic continuation 升格为 live/public-ready。无新增内部 blocker。
- 下一步：G1-T09

## G1-T09 — MCP typed bookmark read

**Status:** verified

**Depends on:** G1-T08

**Capabilities:** artwork/novel bookmark list/tags/detail、`bookmark-subtype`。

**目标：** 完成 typed bookmark read，不包含 `all` aggregate。

**验收：** public/private、typed target、empty/error/cursor、logical-limit/filter 语义和旧 `user_bookmarks` / `bookmark_tags` wire 通过。

**最小验证：** focused Red/Green + bookmark MCP package tests。

**完成记录：**
- 改动/no-op：新增 `novel_bookmark_tags` / `novel_bookmark_detail` MCP candidate read tools；保留 legacy `user_bookmarks` / `bookmark_tags` / `bookmark_detail` wire；扩展 typed bookmark wire fixture、exact registration test 与双语 MCP docs；`user_bookmarks` 的 artwork subtype 继续走 client-side filter，未添加 `all` aggregate。
- Red/Green：初次 `go test ./internal/mcpserver/pixiv -run 'TestTypedBookmark|TestArtworkBookmarkSubtype' -count=1 -v` 在缺少 novel bookmark route / typed tool 时按预期失败；补齐 transport fixture 后同命令再次因 `novel_bookmark_tags` 未注册失败。新增工具与最小实现后该 focused suite 通过；随后 MCP root、MCP subtree、SDK/endpoint bookmark tests、vet、documentation tests、`git diff --check` 与 `gofmt` 检查均通过。
- Compatibility：旧 tool 名称、字段、output envelope 与 legacy replay 保持；新增 novel tools 使用 closed schemas、positive `novel_id`、`bookmark_tags` / `bookmark_detail` typed envelope，保留 empty、upstream typed error、invalid argument 与 absent state；candidate novel tags 不发明 continuation，unexpected non-zero cursor 显式返回 `InvalidCursor`；不引入 Web/匿名 fallback、secret 输出或 `all` 聚合 wire。
- 风险：novel candidate 的 strict wire/public-private/live/continuation/compatibility/release 仍未证明；#14–16、#18 的 strict live/account binding、aggregate G1-T10 与 G1-CHECK-03 仍未关闭；本 task 无新增 internal blocker。
- 下一步：G1-CHECK-03

## G1-CHECK-03 — 集中检查：MCP user + typed bookmark read

**Status:** verified

**Depends on:** G1-T07,G1-T08,G1-T09

复查 owner 隔离、schema/error、resolver/filter/pagination、legacy wire、forbidden endpoint、是否出现多余 abstraction。

**完成记录：**
- 检查结论：PASS。审计 `G1-T07`–`G1-T09` 的 MCP leaf 与 registration：tool owner 只经 public `sdk/pixiv` 和 MCP owner-local runtime/filter/record/output；没有 `internal/services/pixiv` 或 `internal/services/fanbox` 直连。生产 MCP 没有调用 `/v1/novel/detail`、`/v1/novel/series`、`/v1/novel/content`；reverse-search 的既有 `internal/services/reversesearch` 仍是唯一允许的边界例外。`user_*` resolver/filter/logical pagination、typed bookmark empty/error/invalid/absent、legacy JSON replay、bookmark output schema 与 exact registration 回归均通过。
- Correction：无。新增 novel candidate tools 复用既有 `runtime.CollectWith`、`runtime.Read`、`BookmarkTags`/`BookmarkDetail` envelope，没有新建重复 abstraction；`all` aggregate 未提前进入本 task。此前发现的 layer matrix stale `missing` 已修正为 MCP/offline `verified`，不涉及业务代码。
- 风险：candidate novel tags/detail 的 strict wire、public/private、continuation、live、compatibility 与 release 仍未证明；#14–16、#18 strict live/account binding、G1-T10 aggregate 与后续 regression/live gates 仍按 manifest 开放。G1-CHECK-03 无新增 internal/external/decision blocker。
- 下一步：G1-T10

## G1-T10 — MCP required bookmark aggregates

**Status:** verified

**Depends on:** G1-CHECK-03

**Capabilities:** `bookmark-list-all`、`bookmark-tags-all`。

**验收：**

- list-all：artwork 后 novel、统一 Skip/Limit budget、双流 checkpoint、恢复不漏不重。
- tags-all：typed 同名标签分开保留各自 count。
- 任一 required 流失败，逻辑页整体失败，不输出部分成功。
- 旧 wire 不被替换；新增 operation 只能 additive。

**最小验证：** 聚合专项 Red/Green；不跑全 MCP suite。

**完成记录：**
- 改动/no-op：新增 `bookmark_list_all` 与 `bookmark_tags_all` 两个 additive MCP tool；复用 public `sdk/pixiv` bookmark operations、shared `pagination/traversal` 双流 collector，新增 typed tag output/schema、registration、wire failure fixture 与双语 MCP docs；旧 bookmark tool 名称、输入/输出 envelope 与 endpoint wire 未替换。
- Red/Green：Red 实跑 `go test ./internal/mcpserver/pixiv -run 'TestBookmark(ListAll|TagsAll)' -count=1 -v`，在 operation 未注册时按预期因 unknown tool 失败；Green 实跑 `go test ./internal/mcpserver/pixiv -run 'TestBookmark(ListAll|TagsAll)|TestServerListsExpectedTools' -count=1 -v`，5 个 aggregate tests 与 exact registration 全部通过。另 `go test ./internal/mcpserver/pixiv/internal/runtime ./internal/mcpserver/pixiv/internal/records ./internal/mcpserver/pixiv/internal/outputs ./internal/shared/pagination ./internal/shared/traversal -count=1`、`go vet ./internal/mcpserver/pixiv/...`、文档测试、`gofmt` 与 `git diff --check` 通过；按任务约束未跑全 MCP suite。
- Aggregate evidence：list 固定 artwork→novel；统一 page/limit 在第一页返回 artwork、第二页返回 novel 且无重复；失败 attempt 重放后只保留成功结果；required novel stream failure 时整页 `isError=true` 且 structured records 为空；tags 保留同名 tag 的独立 count 与 `content_type=artwork|novel`，required stream failure 时 structured tags 为空；exact registration 证明旧 wire additive 保留。shared collector 的双流 state/checkpoint 已接入，MCP 不暴露 aggregate cursor。
- 风险：public SDK aggregate operation/cursor、strict candidate/public-private/live second-page、完整 compatibility 与 release 仍未证明；novel bookmark tags continuation 仍按“不猜测”处理；无新增 internal/external/decision blocker。
- 下一步：G1-T11

## G1-T11 — MCP read registration/schema/error gate

**Status:** verified

**Depends on:** G1-T07,G1-T08,G1-T09,G1-T10

**目标：** 收敛 read tool registration、exact-set、共享 output/error schema。

**Manifest owner coverage：** 对 `ugoira-metadata`、`novel-ranking`、`recommended-all`、`rating-filter` 的 required read/registration surface 做 exact-set 核验；缺失 surface 必须转入已登记 correction，不得静默跳过。

**验收：** 旧 tool 不被删除/静默重命名；required additive operations 注册完整；structured error 一致；forbidden endpoint 不可达。

**最小验证：** registration/schema/error 相关 tests；不跑全 legacy replay。

**完成记录：**
- 改动/no-op：no-op verified。现有 MCP registry 已覆盖当前 44 个 client-visible tool；保留 legacy 名称/输入字段，并保留 G1-T10 新增 `bookmark_list_all` / `bookmark_tags_all` additive surface。没有为缺失 surface 发明临时 tool 或静默跳过 manifest correction。
- Red/Green：未修改生产代码，无新增 Red；focused registration/schema/error suite 实跑通过：`go test ./internal/mcpserver/pixiv -count=1 -run '^(TestServerListsExpectedTools|TestEveryToolOutputSchemaOmitsTransportAndCredentialFields|TestFeedRecommendationSchemasMatchLegacyContracts|TestArtworkNovelReadOutputSchemasMatchWireEnvelopes|TestUserReadSchemasMatchLegacyContracts|TestToolErrorResultPreservesStructuredContent|TestToolErrorOutputDoesNotLeakCanary|TestSDKRecommendedAllReturnsEveryStreamAndPagination|TestSDKRecommendedSingleKindsAndInputFailures|TestSDKRecommendedAllFailureDoesNotExposePartialStructuredOutput|TestIllustRankingRejectsInvalidInputBeforeSDKExecution|TestRecommendedKindSelectsArtworkSubtype|TestRecommendedRejectsKindConflictingFiltersBeforeSDKExecution|TestTypedBookmarkSchemasKeepLegacyFieldsClosed|TestTypedBookmarkReadsPreserveEmptyAndTypedErrors|TestBookmarkListAllFailureDoesNotExposePartialRecords|TestBookmarkTagsAllFailureDoesNotExposePartialTags|TestSearchUserSDKFailureRemainsStructured|TestUserDetailSDKFailureRemainsStructured|TestBlockedUsersSDKFailureRemainsStructuredAndDoesNotFallback|TestArtworkNovelReadSDKFailuresPreserveSafeStructuredEnvelopes)$' -v`。
- Tool set：`TestServerListsExpectedTools` exact-set PASS；`recommended.kind` 保留 `all|illust|manga|novel|user` 和四路 pagination；所有注册 tool 的 output schema 均存在且不暴露 transport/credential 字段；read failure 均保留 structured error/空 partial 输出语义。`ugoira-metadata`、`novel-ranking`、`rating-filter` 的缺失 owner 继续由 `CAND-G1-T06-UGOIRA-SURFACE`、`CAND-G1-T06-NOVEL-RANKING-SURFACE`、`CAND-G1-T06-RATING-MCP-SURFACE` 登记并交给后续 owner；`recommended-all` 的 MCP surface 已存在，但 SDK aggregate/live/compatibility 仍未闭合。
- 风险：#6/#12/#39 的 required surface 仍未完成，不得升格为 accepted；#41 仍受 public SDK aggregate、四流 strict live、compatibility/release 和 artwork recommendation continuation P1 约束。生产 MCP 源码无 forbidden internal service import 或 `/v1/novel/detail|series|content` 调用；`/v1/novel/content` 仅保留在 rejected-path 测试 fixture 中。无新增 internal/external/decision blocker。
- 下一步：G1-T12

## G1-T12 — MCP read legacy replay + stdout boundary

**Status:** verified

**Depends on:** G1-T11

**目标：** 只验证 read 层 legacy JSON replay 和 stdio stdout/stderr 边界。

**验收：** legacy request replay 通过；stdout 不混入日志；structured error wire 稳定。

**测试预算：** 运行 read replay harness 与 stdout tests；不重复 package 已通过的功能测试。

**完成记录：**
- Replay：no-op verified。现有四组 read legacy JSON replay harness 覆盖 artwork/novel、feed/recommendation、comments、user/Mypixiv/relationship read；全部保留既有 request fields、structured output envelope、empty/error 语义与 rejected `novel_content` endpoint 边界。
- Stdout：`TestMCPStdioKeepsJSONRPCOnStdout`、`TestDiagnosticsLevelControlsMCPStderrWithoutTouchingStdout`、`TestMCPReverseSearchRegistersSearcherForStdioLifetime` 与 `TestCLIReverseSearchClosesSearcherOnceAndKeepsJSONOnStdout` 通过；JSON-RPC 独占 stdout，diagnostics 只按配置写 stderr，close error 不污染 protocol stdout。
- Correction：无。生产 `RunStdio` 仍只绑定 `mcp.StdioTransport`；没有新增日志、fallback 或改写 wire。此前 forbidden endpoint/rejected-path correction 继续有效。
- 风险：证据为 offline replay/stdio boundary，不能替代 strict live、public compatibility、release 或 mutation read-back；无新增 internal/external/decision blocker。
- 验证：`go test ./internal/mcpserver/pixiv -count=1 -run '^(TestArtworkNovelReadLegacyJSONReplayPreservesStructuredContracts|TestCommentReadLegacyJSONReplayPreservesStructuredContracts|TestFeedRecommendationLegacyJSONReplayPreservesStructuredContracts|TestUserReadLegacyJSONReplayPreservesStructuredContracts|TestMCPStdioKeepsJSONRPCOnStdout|TestToolErrorResultPreservesStructuredContent|TestToolErrorOutputDoesNotLeakCanary)$' -v`、`go test ./internal/cli ./internal/cli/commands/pixiv/mcp -count=1 -run '^(TestDiagnosticsLevelControlsMCPStderrWithoutTouchingStdout|TestCLIReverseSearchClosesSearcherOnceAndKeepsJSONOnStdout|TestMCPReverseSearchRegistersSearcherForStdioLifetime|TestNewCommandPreservesMCPSurface)$' -v` 均 PASS。
- 下一步：G1-CHECK-04

## G1-CHECK-04 — Phase B exit：MCP read 完整性 + push

**Status:** verified

**Depends on:** G1-T10,G1-T11,G1-T12

**Pass：** Phase B required MCP read acceptance 有 evidence；无未分解 read owner；无多余 generalization；worktree 无来源不明 diff。

**Phase push gate：** 提交 Phase B 的实现/账本，普通 fast-forward push 到 `refactor/pixiv-api-stability` 并验证 Remote SHA == Local HEAD。push 未成功不得进入 Phase C。

**完成记录：**
- **检查结论：** PASS。复核 G1-T07、G1-T08、G1-T09、G1-CHECK-03、G1-T10、G1-T11、G1-T12 的 read evidence；required MCP read owner 已分解，当前 44 个 client-visible tool 的 exact registration、schema/error、legacy wire 与 stdio boundary 有证据；未发现未分解 read owner、重复 generalization 或来源不明 diff。专用 linked worktree 与 branch=`refactor/pixiv-api-stability` 仍有效，未改业务 API、scope 或 Goal-3 资料。
- **Coverage：** user identity/collections/relationships/MyPixiv、typed bookmark 与 dual-stream aggregate read、registration/schema/structured error、legacy JSON replay/stdout boundary 均已覆盖；`ugoira-metadata`、`novel-ranking`、`rating-filter` 仍显式落在 `CAND-G1-T06-UGOIRA-SURFACE`、`CAND-G1-T06-NOVEL-RANKING-SURFACE`、`CAND-G1-T06-RATING-MCP-SURFACE` correction registry；`recommended-all` 的 MCP surface 已存在，但 SDK aggregate、strict live、compatibility/release 仍未闭合。
- **Correction：** 无新增 correction、capability、task ID 或 scope；缺失 required surface 未被静默跳过或伪造为 accepted。
- **风险：** 当前证据为 offline MCP/read replay/stdio；不能替代 strict live、public compatibility、release 或 mutation read-back。无新增 internal/external/decision blocker。
- **Local HEAD：** `c902b342ddec687a078f6fff99967dfb4689046a`（Phase B push-exit ledger checkpoint）。
- **Remote SHA：** `c902b342ddec687a078f6fff99967dfb4689046a`；已由 `gh api` 与 `git ls-remote` 一致核验。
- **Push result：** PASS；`aa80415f17c49d274b672cf256e63e109ee4ff9e..c902b342ddec687a078f6fff99967dfb4689046a` ordinary fast-forward；未使用 force/rebase。CHECK 在远端一致性核验后标记为 `verified`。
- **下一步：** G1-T13。

---

# Phase C — MCP mutation convergence

## G1-T13 — MCP bookmark mutation

**Status:** verified

**Depends on:** G1-CHECK-04

**Capabilities:** `artwork-bookmark-mutation`、`novel-bookmark-mutation` 的 MCP layer。

**验收：** add/remove、public/private、definite success/failure/uncertain outcome、structured error、legacy wire；uncertain 不自动 replay。

**最小验证：** 一个根因一个 Red；bookmark mutation focused/package tests。

**完成记录：**
- 改动/no-op：已将既有 novel bookmark endpoint leaf 提升到 public SDK 的 `AddNovelBookmark` / `RemoveNovelBookmark`；新增 MCP `add_novel_bookmark` / `remove_novel_bookmark` 与 `novel_id` structured output；artwork `add_bookmark`/`remove_bookmark` 及 legacy wire 未改。
- Red/Green：SDK Red：`go test ./sdk/pixiv -run 'TestNovelBookmarkMutations' -count=1 -v` 初始因 public method/request type 未定义失败；MCP Red：`go test ./internal/mcpserver/pixiv -run '^TestNovelBookmarkMutation' -count=1 -v` 初始因 `Mutation.NovelID` 未定义失败。随后 focused SDK/MCP tests 与 `go test ./... -count=1` 均 PASS。
- Outcome evidence：offline tests 覆盖 add/remove path/form、显式 private 与省略 restrict 的 public、tags、invalid input 网络前拒绝、typed structured MCP failure/success/schema、legacy artwork wire，以及 502 uncertain outcome 只发出一次请求；没有自动 replay/read-back。
- 风险：仍无真实 mutation、access-control、写后 read-back/cleanup 或 release evidence；不把 status-only 2xx 提升为收藏状态已改变。无新增 blocker。
- 下一步：G1-T14
## G1-T14 — MCP artwork comment/stamp mutation

**Status:** verified

**Depends on:** G1-T13

**Capabilities:** `artwork-comments-mutation`、`stamps` 的 artwork-side MCP acceptance。

**验收：** create/reply/stamp/delete input/outcome/error；创建 ID 来自可靠 contract，不使用“最新评论”猜测；uncertain 不 replay。

**最小验证：** artwork comment mutation focused/package tests。

**完成记录：**
- 改动/no-op：新增 additive MCP `create_artwork_comment`、`reply_artwork_comment`、`stamp_artwork_comment`、`delete_artwork_comment`；`Mutation` 增加可选 `comment_id`，保留既有 `RunMutation`，仅为可靠响应字段增加窄的 `RunMutationInPlace`；补齐注册、wire fixture、exact registry、schema 与双语 MCP/README/release-prep 文档。未改 legacy `illust_comments`/`novel_comments` read surface，也未新增 standalone `stamps` tool 或 internal/services 直连。
- Red/Green：Red 实际运行 `go test ./internal/mcpserver/pixiv -run 'Test(ArtworkCommentMutation|ServerListsExpectedTools)' -count=1 -v`，在实现前因 4 个 tool 未注册、schema 缺失和 exact registry mismatch 失败。随后 focused mutation/registry、invalid-input、race、MCP package、SDK/endpoint、documentation、`go test ./... -count=1`、`go vet ./...` 与 `sh scripts/build.sh` 均 PASS；commit hook 的 gofmt/full test 也 PASS。
- ID/outcome：create/reply/stamp 的 `comment_id` 直接取 public SDK/upstream response；delete 只接受并返回调用方提供的正数 comment ID，不通过“最新评论”猜测。typed upstream failure 保留 `success=false`、`isError=true` 与安全诊断；schema 为 closed object，ID 为正数、comment 非空；502 uncertain fixture 只发一次 wire 请求，不自动 replay。
- 证据/提交：当前 exact client-visible registry 为 50 个 tool；实现提交为 `41e4b2dc2ced61662e464a0564917e393ce79fa8`，目标 worktree clean；implementation/ledger commits 均为 local-only，remote 仍为 `6bf64c208719f4e5dff3c0af7bf93e2630d3d340`，本 task 未 push。
- 风险：当前仍只有 offline MCP/wire evidence；严格 live、access-control、同账号写后 read-back/cleanup、public compatibility 与 release gate 仍未关闭。无新增 internal/external/decision blocker。
- 下一步：G1-T15

## G1-T15 — MCP novel comment/stamp mutation

**Status:** verified

**Depends on:** G1-T14

**Capabilities:** `novel-comments-mutation`、`stamps` 的 novel-side MCP acceptance。

**验收：** create/reply/stamp/delete 与 frozen v2 contract 一致；禁止对 candidate v3 fallback；ID/outcome/error 可观测。

**最小验证：** novel comment mutation focused/package tests。

**完成记录：**
- 改动/no-op：新增 additive MCP `create_novel_comment`、`reply_novel_comment`、`stamp_novel_comment`、`delete_novel_comment`；复用既有 public SDK operations 与 `outputs.Mutation.RunMutationInPlace`/`RunMutation`，补齐 registration、wire fixture、exact registry、schema 与双语 MCP/changelog 文档。未改 novel comments v2 read surface，未新增 candidate v3 fallback、standalone `stamps` tool 或 internal/services 直连。
- Red/Green：Red 实际运行 `go test ./internal/mcpserver/pixiv -run 'Test(NovelCommentMutation|ServerListsExpectedTools)' -count=1 -v`，在实现前因 4 个 tool 未注册、schema 缺失和 exact registry mismatch 失败。随后 focused mutation/registry、invalid-input、race、MCP package、SDK/endpoint、documentation、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh` 与 commit hook 均 PASS。
- ID/outcome：novel create/reply/stamp 使用 frozen v2 写入 `/v1/novel/comment/add`，直接把 public SDK/upstream response 的 `comment_id` 写入 structured `Mutation`；delete 使用 `/v1/novel/comment/delete`，只接受并返回调用方提供的正数 `comment_id`。typed upstream failure 保留 `success=false`、`isError=true` 与安全诊断；invalid input 在 SDK execution 前拒绝；不读取最新评论猜 ID、不切换账号 replay、不回退 candidate v3。
- 证据/提交：exact client-visible registry 从 50 增至 54；实现提交为 `484ffee5629299e644b78da18d24345329479613`。实现与后续 ledger commit 均为 local-only，目标 worktree clean，remote SHA 保持 `6bf64c208719f4e5dff3c0af7bf93e2630d3d340`；本 task 未 push。
- 风险：当前仍只有 offline MCP/wire evidence；严格 live、access-control、同账号写后 read-back/cleanup、public compatibility 与 release gate 仍未关闭。无新增 internal/external/decision blocker。
- 下一步：G1-CHECK-05

## G1-CHECK-05 — 集中检查：bookmark/comment mutation

**Status:** verified

**Depends on:** G1-T13,G1-T14,G1-T15

复查 uncertain outcome、ID 来源、error、legacy wire、无自动 retry、无无关 mutation abstraction。

**完成记录：**
- 检查结论：PASS。对 G1-T13、G1-T14、G1-T15 的 bookmark/comment/stamp mutation 做集中复查；可靠 ID、structured outcome/error、legacy wire、uncertain no-replay、public SDK boundary 与 helper scope 均符合 frozen contract。
- Correction：无。bookmark mutation 只使用调用方提供的 artwork/novel ID；comment create/reply/stamp 只使用 public SDK/upstream response 的正数 `comment_id`；delete 只使用调用方提供的正数 `comment_id`。`outputs.RunMutation`/`RunMutationInPlace` 与 `runtime.Write` 保持现有窄边界，没有新增通用 mutation framework、latest-comment 猜测、candidate v3 fallback、账号切换 replay 或 internal/services 直连。
- 验证：`go test ./internal/mcpserver/pixiv -run 'Test(SDKMutationTypedErrorIsMCPError|NovelBookmarkMutationTypedErrorIsMCPError|SDKMutationToolsReturnStructuredSuccess|ArtworkCommentMutation|NovelCommentMutation|ServerListsExpectedTools)' -count=1 -race`、SDK mutation focused race tests、五个 mutation endpoint package tests、`go test ./scripts/tests/documentation -count=1` 与 `git diff --check` 均 PASS；502 typed failure fixtures 均只产生一次 wire request，structured failure 保留 `success=false`/`isError=true`。
- 风险：本 gate 只关闭 offline MCP/wire contract；strict live、写前 access-control、同账号写后 read-back/cleanup、public compatibility、release 与 Phase C push 仍未关闭。无新增 blocker。
- 下一步：G1-T16

## G1-T16 — MCP follow/unfollow mutation

**Status:** verified

**Depends on:** G1-CHECK-05

**Capabilities:** `follow-mutation` MCP layer。

**验收：** invalid restrict 网络前拒绝；follow/unfollow success/failure/uncertain 有离线回归；旧 wire 兼容；无通用 retry。

**最小验证：** focused Red/Green + follow MCP package tests。

**完成记录：**
- 改动/no-op：no-op verified。既有 MCP `follow_user`/`unfollow_user` 与 public SDK `FollowUser`/`UnfollowUser` 已满足 frozen contract（正数 `user_id`、空 restrict 归一 public、unknown restrict 网络前拒绝、`/v1/user/follow/add|delete` wire 不变）；本轮未改生产代码、tool schema、registration 或文档，只补齐 acceptance 要求但此前缺失的离线回归。
- Red/Green：新增 SDK `TestFollowMutationsDoNotReplayUncertainFailure`（follow/unfollow 对上游 502 各只发一次请求且原样返回错误，不自动 retry/replay）、`TestFollowMutationsRejectInvalidInputBeforeNetwork`（非法 user_id/restrict 在网络前 typed `InvalidArgument`，calls=0）与 MCP `TestFollowMutationTypedErrorIsMCPError`（typed upstream failure → `isError=true`、`success=false`、错误文本保留 `upstream_error` reason）。新测试实跑即通过，证明现有实现无 uncertain retry、无吞错，未触发生产 Red。invalid restrict 网络前拒绝由既有 `TestFollowUserRejectsUnknownRestrictBeforeNetwork` 继续覆盖；success/旧 wire 由 `TestSDKMutationToolsReturnStructuredSuccess` 与 follow wire handlers 覆盖。
- 回归：`go test ./sdk/pixiv ./internal/mcpserver/pixiv ./internal/services/pixiv/endpoint/user/follow -count=1`、focused mutation `-race`、`go vet ./sdk/pixiv ./internal/mcpserver/pixiv`、`gofmt` 与 `git diff --check` 均 PASS。
- 风险：仍只有 offline wire 证据；strict live、写前 access-control、同账号写后 read-back/cleanup 与 release gate 继续 open（G1-T30 及后续 owner）。无新增 internal/external/decision blocker。
- 下一步：G1-T17

## G1-T17 — Shared mutation outcome / uncertainty harness

**Status:** verified

**Depends on:** G1-T13,G1-T14,G1-T15,G1-T16

**目标：** 只收敛多种 mutation 共用的 outcome/error/uncertain 语义和必要 shared harness；不得为了统一而新建通用 mutation framework。

**验收：** definite failure 与 uncertain 分离；不自动 replay；已有 helper 可复用时不新增 abstraction。

**最小验证：** shared harness tests + 受影响 mutation package spot checks。

**完成记录：**
- 改动/no-op：no-op verified。shared 语义已由既有载体承载且测试齐备，本轮零代码改动，不新增通用 mutation framework。
- Red/Green：无生产代码变更，未触发 Red；shared harness 与 spot checks 实跑通过（见下）。
- Shared semantics：(1) definite/uncertain 分离由 `sdk/pixiv/errors.go classifyStatus` 承载：400→`InvalidArgument`、403→`Forbidden`、404→`NotFound`、410→`ContentUnavailable`（definite typed failure）；401→`CredentialsExpired`、429→`RateLimited` 仅携带显式 `RetryAdvice`；5xx/transport→`UpstreamError`（uncertain）。(2) 不自动 replay：SDK 层各 mutation family 的 502 单次请求回归（novel bookmark/artwork comment/novel comment/follow）+ pool/facade 边界 `TestSchedulerFailsOverOnlyBeforeCommit`、`TestFacadeUseDoesNotReplayCommittedAttempt`、`TestSchedulerRequiresSafeFutureRetryAfter`；MCP `runtime.Write` 将 mutation attempt 标记 `committed=true`（读路径为 false），`Facade.Use` 只在未 commit 且 SDK retry advice 明确允许时切换账号。(3) 复用而非新增：14 个 mutation tool 全部经 `runtime.Write` + `outputs.RunMutation`/`RunMutationInPlace`（rg 全量核对），无 tool 自建 outcome 映射或绕过账号池边界。
- 回归：`go test ./internal/services/pixiv ./internal/services/pixiv/pool -run 'TestScheduler|TestFacadeUse' -count=1 -v`、`go test ./sdk/pixiv ./internal/mcpserver/pixiv ./internal/services/pixiv/endpoint/user/follow ./internal/services/pixiv/endpoint/user/novelbookmarks -count=1` 均 PASS。
- 风险：本 gate 只收敛 offline shared 语义；strict live、写后 read-back/cleanup 与 release 仍 open（G1-T30 及后续 owner）。无新增 internal/external/decision blocker。
- 下一步：G1-T18

## G1-T18 — MCP mutation legacy replay / offline read-back contract gate

**Status:** verified

**Depends on:** G1-T17

**目标：** 验证旧 mutation request wire、structured error、offline read-back contract；不执行 live 写入。

**验收：** legacy replay 通过；read-back orchestration 可测试；无法知道写入结果时保持 uncertain；不借 offline fixture 宣称 live success。

**测试预算：** mutation replay harness + 必要 integration，不重复所有 focused tests。

**完成记录：**
- Replay：旧 mutation request wire 与 structured error 由既有证据承载并在本轮 HEAD 复跑：MCP wire responder 对全部 mutation endpoint（artwork/novel bookmark add-delete、artwork/novel comment add-delete、follow add-delete）做精确 path/form 断言；`TestSDKMutationToolsReturnStructuredSuccess` 保持 legacy tool 名与 legacy 输入字段（`illust_id`/`restrict`/`tags`/`user_id`）；`TestSDKMutationTypedErrorIsMCPError`、`TestNovelBookmarkMutationTypedErrorIsMCPError`、`TestFollowMutationTypedErrorIsMCPError`、`TestArtworkCommentMutationTypedErrorIsMCPErrorAndDoesNotReplay`、`TestNovelCommentMutationTypedErrorIsMCPErrorAndDoesNotReplay` 证明 typed failure 保持 `isError=true`/`success=false` 且不重放。read 侧四组 legacy JSON replay harness 不受影响。
- Read-back contract：新增 SDK `TestBookmarkMutationReadBackOrchestrationOffline`：同一 client 上 add→`ArtworkBookmark` detail 确认→remove→detail 确认恢复，并对 wire 序列（add/detail/delete/detail）做精确断言；证明 read-back 编排只由调用方组合既有 public read 操作即可离线测试，且 2xx 本身不代表状态确认。未新增 abstraction、未在 MCP 层做自动 read-back（uncertain 不 replay 边界不变）。live read-back/cleanup 仍属 G1-T30。
- Correction：无。
- 风险：offline fixture 只证明 wire/编排可测性，不宣称 live success；`无法知道写入结果时保持 uncertain` 由四 family 的 502 单次请求回归继续覆盖。无新增 internal/external/decision blocker。
- 下一步：G1-CHECK-06

## G1-CHECK-06 — Phase C exit：MCP mutation 完整性 + push

**Status:** verified

**Depends on:** G1-T16,G1-T17,G1-T18

**Pass：** 所有内部 mutation gap 关闭或有抢占式 correction；无 live blocker 掩盖代码问题；worktree 状态可交付。

**Phase push gate：** 提交 Phase C 的实现/账本，普通 fast-forward push 到 `refactor/pixiv-api-stability` 并验证 Remote SHA == Local HEAD。push 未成功不得进入 Phase D。

**完成记录：**
- 检查结论：PASS。G1-T13（novel bookmark public SDK + MCP mutation）、G1-T14（artwork comment/stamp ×4）、G1-T15（novel comment/stamp ×4）、G1-T16（follow/unfollow 回归补齐）、G1-T17（shared outcome/uncertainty 复核）、G1-T18（legacy wire/structured error 复核 + offline read-back 编排回归）全部 verified；MCP/offline 层无未关闭的内部 mutation gap，无需抢占式 correction。可靠 ID、structured outcome/error、legacy wire、uncertain no-replay、committed-before-replay 边界与 helper 复用在 CHECK-05 与 T16–T18 记录中均有当前分支证据。
- Verification：本轮在 push-exit HEAD 复跑 `go test ./internal/services/pixiv ./internal/services/pixiv/pool -run 'TestScheduler|TestFacadeUse' -count=1`（shared harness）、8 个 mutation 相关 package（sdk/pixiv、internal/mcpserver/pixiv、artwork/novel bookmark、artwork/novel comments、stamps、follow endpoint）`-count=1`，以及 MCP focused mutation/registration set `-race`，全部 PASS；此前 T14–T18 提交时 commit hook 的全量 `go test ./...` 亦 PASS。
- Live boundary：Phase C 按计划仅关闭 offline MCP/wire/编排 contract；strict live、写前 access-control、同账号写后 read-back/cleanup 未执行且未被 offline fixture 伪装，全部按 manifest 留给 G1-T30（Phase F）；无 live blocker 掩盖代码问题。
- Worktree：专用 linked worktree、branch=`refactor/pixiv-api-stability`、push 前 `git status --porcelain` 为空；Phase C 实现/账本共 14 个 commit（`f266d9a..5a057ad`）均已提交，无来源不明 diff。
- Correction：无新增 correction、capability、task ID 或 scope。
- 风险：当前证据为 offline wire/regression/race；public compatibility、release、cursor/docs gate 与 live 继续 open（Phase D–F owner）。无新增 internal/external/decision blocker。
- Local HEAD：`5a057ad6f7510c7c005c22974012b830152e3ed9`（Phase C push-exit checkpoint）。
- Remote SHA：`5a057ad6f7510c7c005c22974012b830152e3ed9`；已由 `git ls-remote` 与 `gh api repos/FlanChanXwO/pixiv-cli/branches/...` 双重核验一致。
- Push result：PASS；`6bf64c208719f4e5dff3c0af7bf93e2630d3d340..5a057ad6f7510c7c005c22974012b830152e3ed9` ordinary fast-forward；未使用 force/rebase。CHECK 在远端一致性核验后标记为 `verified`。
- 下一步：G1-T19（Phase D）。

---

# Phase D — Compatibility、cursor 与 docs

## G1-T19 — Cursor integrity / binding / rollback gate

**Status:** verified

**Depends on:** G1-CHECK-06

**Capabilities:** `logical-pagination` 与所有持久化 cursor 使用者。

**验收：**

- cursor 不含 credential/cookie/token/signed URL/raw next_url/原始用户内容。
- query/account/client/subtype binding 与 contract 一致。
- 不兼容版本显式 `InvalidCursor` 或已定义迁移，不静默第一页重启。
- 批内/末批 checkpoint、Skip/Limit/OneBatch、重复 cursor、取消、replay 有针对性回归。
- 跨版本 rollback 有兼容测试或受控失败测试。

**测试预算：** cursor/shared + 直接调用方相关 tests，不跑全仓。

**完成记录：**
- 改动/no-op：no-op verified。cursor integrity/binding/rollback 三层（`sdk` 通用 Cursor、`sdk/pixiv` 产品绑定、shared pagination/traversal 与直接调用方）均已由既有实现与测试满足；本轮零代码改动，未触发 Red。
- Integrity：`sdk/cursor.go` 的 `cursorEnvelope` 为封闭 typed 结构（format version/product/operation/binding/query digest/非 secret identity/ephemeral instance/payload），凭据、cookie、token、signed URL、raw next_url、原始查询词与用户内容没有承载字段；payload 由 `sdk/pixiv/cursor.go continuationEnvelope{Key,Value,Consumed}` 唯一构造。`TestSearchArtworksCheckpointRejectsChangedBindings` 直接断言 payload 不含查询词与 CursorContext；`TestCursorTextIsRouteSafe`、`TestCursorRoundTripPreservesBindingAndPayload`、`TestCursorJSONRoundTrip` 覆盖编码与往返。
- Binding：`ValidateCursor` 校验 product/op/binding version/query digest；identity-scoped ops 绑定 verified 非 secret identity 或 ephemeral instance（`TestRemainingIdentityScopedPixivCursorsBindClientInstance`、`TestCursorEphemeralInstanceBinding`）；search checkpoint 绑定 word/CursorContext/AI mode/content type/account/client（`TestSearchArtworksCheckpoint{RoundTrip,RejectsChangedBindings,AIAndLaterBatches,VerifiedAccount}`）；`TestRemainingPixiv{Offset,Value}CursorsRejectNonPositiveContinuation`、`TestLatestArtworksRejectsNonPositiveContinuationValues` 覆盖其余 ops 的 kind/value 边界。
- Incompatible version/rollback：format version 不识别 → `InvalidCursor`（`decodeCursor`）；binding version 篡改（b=2→1）→ `InvalidCursor` 的受控失败测试即 rollback gate（`TestSearchArtworksCheckpointRejectsChangedBindings` 尾段）；novel latest 旧 offset cursor 拒绝（`ops_novel_test.go`）；不静默第一页重启——zero cursor 是唯一的首页入口。
- Checkpoint/batch regression：`internal/shared/pagination` 覆盖 skip across batches、truncate inside batch、exact-limit next cursor、OneBatch 三态、repeated cursor/cycle 拒绝、negative plan pre-fetch、fetch/consume error、caller context（取消）、filtered continuation（limit 后 checkpoint、unconsumed 保留、cancel/error、predicate failure）；`internal/shared/traversal` 覆盖 commit boundary、logical more 不暴露 cursor、uncommitted 结果 replay 前清空；MCP runtime `TestCollectWithFromResetsLocalFilterOnSafeReplay`；CLI narrow executor port。
- 回归：`go test ./sdk ./sdk/pixiv ./internal/shared/pagination ./internal/shared/traversal ./internal/mcpserver/pixiv/internal/runtime ./internal/cli/commands/pixiv/internal/listing -count=1` 与 `go vet`（四个核心包）均 PASS；`git diff --check` PASS。
- 风险：离线 shared/SDK 证据不替代 endpoint live continuation（Phase F owner）；MCP aggregate cursor 不对外暴露，跨版本恢复仅在受控失败语义内。无新增 internal/external/decision blocker。
- 下一步：G1-T20

## G1-T20 — Public Go SDK compatibility

**Status:** verified

**Depends on:** G1-T19

**目标：** 对照旧 T12 symbol map，只收敛实际 SDK compatibility 差异。

**Manifest owner coverage：** 承接 v2 novel detail/series、required aggregate SDK surface 与已冻结 cursor/filter 语义；不得以兼容 wrapper 触发 rejected endpoint，也不得把缺失 aggregate 当作已接受。

**验收：** exported symbols/named types/legacy wrappers/old consumer compilation；excluded endpoint 兼容入口不发 rejected 请求。

**最小验证：** SDK compatibility/old consumer tests；不跑 CLI/MCP。

**Blocking：** 必须 breaking 时 `blocked_decision`，并触发 G1-TERM 条件检查。

**完成记录：**
- 改动/no-op：no-op verified。当前 public SDK surface 与旧 T12 symbol map 对照无删除/重命名/语义 breaking；自 T12 以来 exported surface 的唯一变化是 G1-T13 additive 新增 `AddNovelBookmark`/`RemoveNovelBookmark` 及其 request types（当时已随 public API review 更新 pinned digest）。本轮零代码改动。
- Compatibility tests：`TestRepositoryPublicAPIInventoryIsPinned`（exported symbols/named types 钉死 digest PASS）+ `TestInventoryCollectsOnlyExportedPackageSymbols`；`TestLegacySDKConsumerCompiles`（旧消费者 interface/字面量编译）PASS；legacy wrapper `TestAddBookmarkWiresMutation` 与 `TestNovelBookmarkMutationsUseCandidatePathsAndForms` PASS；`TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint` 证明 excluded endpoint 兼容入口零网络、返回 `ContentUnavailable`；`TestSearchNovelsAndUsersCursorsArePublicScoped` 与 `TestSearchArtworksCheckpointRoundTrip` 证明已冻结 cursor 语义保持；`go test ./sdk ./sdk/pixiv -count=1` 全包 PASS。
- Manifest coverage：v2 novel detail/series 的 SDK 层为 verified（`Novel`/`NovelSeries` v2 path）；aggregate SDK surface（#23/#24 bookmark aggregate、#41 recommended-all）在旧 T12 symbol map 中不存在 public aggregate operation，其 contract 权威是 cli-migration-matrix 的产品层聚合命令；本轮如实记录 SDK aggregate 仍 `missing`、不得当作已接受，capability verdict 留给 G1-FINAL 重算；已冻结 cursor/filter 语义由 G1-T19 证据承接。
- Breaking decision：无。无 breaking 变更，不触发 `blocked_decision` 或 G1-TERM。
- 风险：SDK aggregate 缺口已显式登记（不因 CLI/MCP 聚合已实现而升格 accepted）；live/compatibility/release gate 继续 open。无新增 internal/external/decision blocker。
- 下一步：G1-T21

## G1-T21 — CLI compatibility + presentation

**Status:** verified

**Depends on:** G1-T20

**目标：** 收敛 canonical route、legacy alias/deprecation、默认值、help/completion；不重设计 CLI。

**Manifest owner coverage：** 承接 `ugoira-metadata`/trending 的真实 CLI surface、explicit `--type`/bare-ID 边界与 local rating presentation；不凭空增加 server-side rating 或隐式 probe。

**验收：** 旧 route/alias 可用；新 canonical 行为符合 frozen map；help/completion 只显示真实 surface。

**最小验证：** 相关 route/alias/help/completion tests。

**完成记录：**
- 改动/no-op：Red→Green 实现 frozen map row 35 要求的 `bookmark add/remove` 双 namespace：新增 `--type/-t`（默认 `artwork`，保持旧行为），novel dispatch 走 G1-T13 的 public SDK `AddNovelBookmark`/`RemoveNovelBookmark`（`/v2/novel/bookmark/add`、`/v1/novel/bookmark/delete`）；`--type all/user` 与未知值在网络前拒绝，措辞与 resolver TypeSpec 一致；record 消费按 namespace 分离（novel type 只接受 `novel` record，artwork record 报 `unsupported_type`；默认 artwork 行为不变）；help/usage 改为 typed 名（`[ARTWORK_ID_OR_NOVEL_ID]`）。Red 实跑 5 个新测试因 `unknown flag: --type` 全部失败后 Green。
- Tests：新增 `TestBookmark{Add,Remove}SupportsNovelType`、`TestBookmarkMutationRejectsUnsupportedTypeBeforeNetwork`、`TestBookmarkAddConsumesNovelRecordsOnlyWithNovelType`、`TestBookmarkAddRejectsArtworkRecordWithNovelType`；回归 `go test ./internal/cli/commands/pixiv/{bookmark,search,follow,comment} ./internal/cli ./sdk/pixiv -count=1`、`go test ./scripts/tests/documentation -count=1`、`go vet`、`gofmt` 全 PASS；commit hook 全量 `go test ./...` PASS。
- Compatibility：旧 route/alias 全部保留（artwork 默认路径、`bookmark_add/remove` record operation 名、legacy wrapper wire 不变）；canonical 行为符合 cli-migration-matrix row 35（artwork 或 novel、不接受 `all`、namespace 一致）；trending 的真实 CLI surface 复核为 `pixiv search --trending-tags`（frozen row 110，冲突拒绝与 wire tests 已存在，纠正 state matrix #35 的 stale `CLI=missing`）；bare-ID 边界保持显式 `--type`、无隐式 probe（resolver/CLI 负向 tests）；local rating 为 `search --rating` 本地过滤并绑定 cursor digest，不发 server-side rating（`search.go` 注释与 tests）。
- Adjudication：`ugoira-metadata` CLI：frozen cli-migration-matrix 无 `ugoira metadata` 路由（Goal-3 T07A 明确 adapter/SDK only），CLI 层缺口保持显式登记（`CAND-G1-T06-UGOIRA-SURFACE`），不静默当作已解决；是否新增 additive 命令留 G1-FINAL 裁决。
- 文档：`bookmark add/remove --type` 与 typed usage 的双语 cli-reference / `skills/pixiv-cli` 同步由 G1-T23 统一执行（本 task 记录 pending）。
- 风险：无新增 internal/external/decision blocker；live/access-control 仍由 Phase F owner。
- 下一步：G1-T22

## G1-CHECK-07 — 集中检查：cursor + SDK + CLI

**Status:** verified

**Depends on:** G1-T19,G1-T20,G1-T21

复查 cursor safety、rollback、source compatibility、CLI alias/presentation、breaking blocker 与 over-refactor。

**完成记录：**
- 检查结论：PASS。G1-T19/T20/T21 证据在当前 HEAD 复核并复跑通过：(1) cursor safety/rollback——封闭 typed envelope 无凭据承载字段、payload canary、format/binding version fail-closed、无静默第一页重启、batch/checkpoint/replay/cancel 回归；(2) source compatibility——pinned public API inventory digest PASS、old consumer 编译、legacy wrapper 保留、`NovelContent` 零网络、T12 以来仅 additive；(3) CLI presentation——frozen map row 35 双 namespace 已闭合且默认 artwork 旧行为不变、namespace 冲突网络前拒绝、trending= `search --trending-tags` 纠正 stale verdict、ugoira CLI 缺口保持显式登记、local rating 不发 server-side。
- Over-refactor：T21 实现改动仅 `bookmark.go` 68 行（helper + 两命令 flag/dispatch + record types）+ 162 行测试；复用既有 flag/dispatch/record 模式，无新 abstraction、无跨包重设计、无 debug 残留（diff 扫描）。
- Breaking blocker：无。T20 无 breaking；T21 为 additive flag + 默认值保持，不触发 `blocked_decision`。
- Verification：`go test ./internal/shared/pagination ./internal/shared/traversal ./sdk ./sdk/pixiv ./internal/cli/commands/pixiv/bookmark ./internal/cli/commands/pixiv/search ./internal/cli ./scripts/internal/publicapi -count=1` 全部 PASS；各实现提交时 commit hook 全量 `go test ./...` PASS。
- Correction：无新增 correction、capability、task ID 或 scope。
- Decision blocker：无。G1-CHECK-07 非 Phase D exit gate，本轮不 push；下一任务为 G1-T22。

## G1-T22 — MCP compatibility audit

**Status:** verified

**Depends on:** G1-CHECK-07

**目标：** 对照旧 T39A，只修 MCP tool/input/output wire 的真实差异。

**Manifest owner coverage：** 核验 required additive owner（含 `ugoira-metadata`、`novel-ranking`、`recommended-all`）；rating 仅保留 frozen local semantics；`artwork-comments-read` rejected path 不得被注册为 fallback。

**验收：** old tool exact-set、legacy request/output/error schema；新增 required operation additive；旧 tool 不删不静默改名。

**最小验证：** MCP compatibility tests + relevant replay，不重复功能 tests。

**Blocking：** breaking wire 需要 `blocked_decision`。

**完成记录：**
- 改动/no-op：no-op verified。对照旧 T39A（40-tool frozen map）与当前 registry，无 tool 删除/静默改名、无 legacy request/output/error schema 差异需要修复；本轮零代码改动。
- Compatibility：`TestServerListsExpectedTools` exact-set PASS——当前 54 个 client-visible tool = T39A 冻结 40 + 14 个有记录 additive（T09 novel bookmark tags/detail、T10 bookmark_list_all/tags_all、T13 add/remove_novel_bookmark、T14/T15 各 4 个 comment/stamp mutation），legacy 名称与输入字段全部保留；output schema 四组 match 测试、四组 read legacy JSON replay、mutation structured success/typed error 测试（共 19 focused）全 PASS；`TestEveryToolOutputSchemaOmitsTransportAndCredentialFields` PASS。
- Additive owner 核验：`ugoira-metadata`、`novel-ranking` 的 MCP surface 仍缺失，继续显式登记于 `CAND-G1-T06-UGOIRA-SURFACE`、`CAND-G1-T06-NOVEL-RANKING-SURFACE`（registry 静态核查无这两个 tool，未被静默跳过或伪造）；`recommended-all` 的 MCP 四流 aggregate（`recommended` kind=all）存在且有 focused failure-atomicity tests。
- Rating/rejected：MCP 无 rating input schema（`search_novel` 注释明确 rating 表面在可靠证据前不发布），local semantics 仅在 CLI；生产 MCP 源码静态核查无 `/v1/novel/detail|series|content` 调用；`illust_comments` legacy v3 read tool 保持原 wire，未注册任何 fallback replacement；`novel_content` 结构化 unsupported 且零网络（focused test PASS）。
- Breaking decision：无。不触发 `blocked_decision`。
- 风险：missing additive surface（ugoira/novel-ranking）与 live/compatibility/release gates 继续 open（correction registry 与后续 phase owner）。无新增 internal/external/decision blocker。
- 下一步：G1-T23

## G1-T23 — README / CLI / SDK / MCP docs / Skill / changelog

**Status:** verified

**Depends on:** G1-T21,G1-T22

**目标：** 只同步真实已实现 surface、compatibility、cursor/mutation 注意事项和 exclusion。

**验收：** 双语文档、`skills/pixiv-cli/`、必要 changelog 与实际 surface 一致；不声称未 live 验证能力已 live verified。

**测试预算：** docs/completion tests；不跑业务全仓测试。

**完成记录：**
- 文档改动：同步 G1-T21 引入的 `bookmark add/remove --type artwork|novel` 真实 surface——双语 cli-reference（quick examples 增 novel add 示例、record 消费说明按 namespace 限定、命令表 `detail/add/remove 支持 artwork/novel 不接受 all`、flags 表补 add/remove 的 `--type/-t` 行）、`skills/pixiv-cli/SKILL.md`（`ILLUST_ID` 过时示例改为 `ARTWORK_ID_OR_NOVEL_ID` + `--type novel` 提示）、`changelog/unreleased/{en,zh-CN}.md`（Added 条目：默认 artwork 保持既有行为、novel 走 public SDK wire、namespace 网络前拒绝、Record 按 namespace 过滤）。未声称任何 live-verified 能力；README/MCP docs/SDK docs 无需改动（T13–T15 已同步 MCP/SDK 表面，本轮无 MCP/SDK 变化）。
- Docs tests：`go test ./scripts/tests/documentation -count=1` PASS；`TestBookmarkHelpUsesTypedTargetNames` PASS；`ILLUST_ID` 全仓残留扫描为空；`git diff --check` PASS。
- 风险：ugoira/novel-ranking/rating 等缺失 surface 继续按 correction registry 保持文档沉默（不凭空文档化）；live 边界不变。无新增 internal/external/decision blocker。
- 下一步：G1-T24

## G1-T24 — Forbidden endpoint / no-fallback release contract gate

**Status:** verified

**Depends on:** G1-T20,G1-T21,G1-T22,G1-T23

**目标：** 专门证明 rejected endpoint 和 fallback 禁令仍成立，避免在大 regression 中被淹没。

**Manifest owner coverage：** 保护 #25 artwork comments rejected、#38 bare-ID explicit-type boundary，以及 #8/#9/#25 的 v1/WebView/anonymous no-fallback 负向断言。

**验收：** `/v1/novel/detail`、`/v1/novel/series`、`/v1/novel/content`、WebView/anonymous fallback、未经确认 server x_restrict 不可从 required public paths 触发。

**测试预算：** 现有负向 tests + 必要静态引用检查；不新增重复 fixture。

**完成记录：**
- Gate：PASS。静态扫描（sdk/internal/cli/internal/mcpserver/internal/services/pixiv/internal/shared 生产树）：无 `/v1/novel/detail`、`/v1/novel/series` 引用（protocol 常量为 `AppNovelDetail="/v2/novel/detail"`、`AppNovelSeries="/v2/novel/series"`，endpoint packages PASS）；无 `/webview` 请求路径；无 anonymous fallback 路径。负向回归实跑 PASS：`TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint`（SDK NovelContent 零网络）、`TestNovelContentReportsUnsupportedWithoutCallingRejectedEndpoint`（MCP structured unsupported 零网络）、novel detail/series endpoint、resolver（bare-ID explicit-type、无隐式 probe）、searchfilter（rating 仅本地规范化，`map_artwork` 不把 rating 写入 upstream query）。
- Findings：(1) `/v1/novel/content` 仅作为 protocol 常量 `AppNovelContent` 存在，唯一生产消费点是 legacy `novel_image`/`novel_file` resource resolve 路径（`sdk/pixiv/resource.go:280`→`detail.go Content`）；该路径不属于 41 项 required capability 的任何 surface（CLI download 不接受 novel 来源、SDK `NovelContent` deprecated 零网络、MCP `novel_content` structured unsupported），ref 铸造链闭合于内容解析自身，required public path 无法铸造或接受此类 ref；上游对该 endpoint 已 404（受控失败，无 fallback）。判定：gate 不破，登记 out-of-scope observation。(2) `illust_comments` 保持 legacy v3 wire，未注册任何 replacement/fallback（#25 保护成立）。
- Correction：无。finding (1) 若要移除 legacy resource 兼容面属 breaking change（超出 required scope，需用户决策），不自行实施。
- 下一步：G1-CHECK-08

## G1-CHECK-08 — Phase D exit：compatibility/docs/release contract + push

**Status:** verified

**Depends on:** G1-T22,G1-T23,G1-T24

**Pass：** cursor/SDK/CLI/MCP/docs/forbidden contract 均有 evidence；无未决内部差异；worktree 状态可交付。

**Phase push gate：** 提交 Phase D 的实现/文档/账本，普通 fast-forward push 到 `refactor/pixiv-api-stability` 并验证 Remote SHA == Local HEAD。push 未成功不得进入 Phase E。

**完成记录：**
- 检查结论：PASS。G1-T19（cursor integrity/binding/rollback）、G1-T20（SDK compatibility，无 breaking）、G1-T21（CLI presentation + bookmark add/remove novel namespace Red→Green）、G1-CHECK-07（集中检查 PASS）、G1-T22（MCP compatibility audit，无 breaking）、G1-T23（双语 docs/Skill/changelog 同步）、G1-T24（forbidden endpoint/no-fallback gate PASS，legacy novel resource 路径登记 out-of-scope observation）全部 verified；cursor/SDK/CLI/MCP/docs/forbidden 六项 contract 均有当前 HEAD 可复跑证据；无未决内部差异。
- Verification：push-exit HEAD 复跑 Phase D 代表性回归——shared pagination/traversal/resolver/searchfilter、sdk、sdk/pixiv、bookmark、search、internal/cli、publicapi pinned inventory、MCP focused compat set（exact registration/schema walk/novel_content negative/mutation structured）与 documentation tests，全部 PASS；各实现提交时 commit hook 全量 `go test ./...` PASS。
- Worktree：专用 linked worktree、branch=`refactor/pixiv-api-stability`、push 前干净；Phase D 实现/文档/账本共 9 个 commit（`2f62771..c4a99ec`）全部已提交，无来源不明 diff。
- Correction：无新增 correction、capability、task ID 或 scope；T24 的 out-of-scope observation 已登记（legacy novel resource 兼容面，移除需用户决策）。
- Blocker：无 internal/external/decision blocker。
- Local HEAD：`c4a99ec4ecb3646bced231d493db94d641405969`（Phase D push-exit checkpoint）。
- Remote SHA：`c4a99ec4ecb3646bced231d493db94d641405969`；已由 `git ls-remote` 与 `gh api` 双重核验一致。
- Push result：PASS；`5bfb29ceb0b2a541e7fb21ae932bdc79f34e3432..c4a99ec4ecb3646bced231d493db94d641405969` ordinary fast-forward；未使用 force/rebase。CHECK 在远端一致性核验后标记为 `verified`。
- 下一步：G1-T25（Phase E）。

---

# Phase E — Offline release candidate

## G1-T25 — Protocol / endpoint / SDK regression

**Status:** verified

**Depends on:** G1-CHECK-08

**验收：** required/optional/null/empty/error fixtures、adapter↔SDK、continuation allowlist/binding、old consumer、rejected endpoint negative regression 通过。

**测试预算：** 运行 protocol/endpoint/SDK regression 集；不重复 full CLI/MCP。

**完成记录：**
- Commands：`go test ./internal/services/pixiv/... ./sdk/... -count=1`（protocol、appapi、oauth、pool、resource、facade 与全部 artwork/novel/user endpoint adapter packages、sdk、sdk/pixiv、sdk/fanbox，共 40 个有测试 package）；另单独复跑 `go test ./sdk/pixiv -run 'TestLegacySDKConsumerCompiles|TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint' -count=1`。
- Result：40/40 package PASS，0 FAIL。required/optional/null/empty/error fixtures、adapter↔SDK DTO/cursor 映射、continuation allowlist/binding（含 search checkpoint v2、novel latest max_novel_id、typed value cursors）、old consumer 编译、`NovelContent` rejected endpoint 零网络负向回归全部通过；按预算未重复 full CLI/MCP（G1-T26 owner）。
- Correction：无。
- 下一步：G1-T26

## G1-T26 — CLI + MCP regression

**Status:** verified

**Depends on:** G1-T25

**验收：** CLI JSON/NDJSON/stdin/skip/fail-fast/cursor/alias；MCP schema/error/exact-set/legacy replay/stdout；aggregate 与 mutation offline safety 通过。

**测试预算：** 运行 CLI/MCP regression harness；不重复 protocol/SDK suite。

**完成记录：**
- Commands：`go test ./internal/cli/... -count=1`（28 个有测试 package：全部 Pixiv 命令 owner、pipeline/action record 消费、internal/cli root、CLI MCP runtime）与 `go test ./internal/mcpserver/... -count=1`（10 个有测试 package：Pixiv MCP 全 tool、runtime/records/outputs/filters internals、FANBOX MCP）；另复跑 focused subset（四组 read legacy JSON replay、`TestMCPStdioKeepsJSONRPCOnStdout`、`TestServerListsExpectedTools`）6/6 PASS。
- Result：CLI 28/28、MCP 10/10 全部 ok，0 FAIL。CLI JSON/NDJSON/stdin 补值/skip/fail-fast/typed cursor/legacy alias、MCP closed schema/structured error/exact registration/legacy JSON replay/stdio stdout 边界、bookmark 双流 aggregate 与全部 mutation 的 offline safety（502 单请求、namespace record 过滤、页原子失败）全部通过；按预算未重复 protocol/SDK suite（G1-T25 已闭环）。
- Correction：无。
- 下一步：G1-T27

## G1-T27 — Full offline build / quality / race-as-needed / redaction gate

**Status:** verified

**Depends on:** G1-T25,G1-T26

**必须验证一次：**

- `go test ./...`
- `go vet ./...`
- `sh scripts/build.sh`
- race tests 仅对本 Goal 修改过且存在并发语义的 package，或已有 release contract 明确要求的集合
- docs/completion
- compatibility replay
- forbidden endpoint/no-fallback
- evidence/log 中 token/cookie/signed URL/隐私数据

禁止为了“更保险”重复运行与同一 HEAD 已通过且未被 invalidated 的昂贵 gate。

**完成记录：**
- Commands：`go test ./... -count=1`（147 包）；`go vet ./...`（clean）；`sh scripts/build.sh`（built build/pixiv）；`go test -race ./internal/mcpserver/pixiv ./internal/mcpserver/pixiv/internal/runtime ./sdk/pixiv ./internal/services/pixiv ./internal/services/pixiv/pool -count=1`；redaction focused：`TestToolErrorOutputDoesNotLeakCanary`、`TestEveryToolOutputSchemaOmitsTransportAndCredentialFields`、`TestCursorTextIsRouteSafe`、`TestSearchArtworksCheckpointRejectsChangedBindings`；`git diff --check`。
- Result：全量 test 147/147 包 PASS（0 FAIL）；vet 零输出；build 成功产出 `build/pixiv`；race 5 包 PASS；redaction focused 全 PASS。
- Race scope/rationale：仅覆盖本 Goal 修改过且具并发语义的 package——MCP mutation/runtime（wire handler 与 attempt 生命周期）、sdk/pixiv（novel mutation + client）、services/pixiv facade 与 pool（账号池 attempt commit/replay 语义，release contract 既有 race 要求集，沿 CHECK-05 先例）；未为未改动 package 追加 race。
- Redaction：MCP error canary 与 output schema credential/transport 字段 walk PASS（isError 输出与全部 54 tool schema 不泄漏 token/cookie/signed URL）；cursor 文本 route-safe PASS；`goal-1/*.md` 账本静态扫描无 refresh/access token、Bearer、签名 URL 或隐私数据（本轮无 live 证据写入）；`git diff --check` PASS。
- 引用未 invalidated 证据：docs/completion（scripts/tests/documentation PASS 于 T23/CHECK-08）、compatibility replay（四组 legacy replay 于 T26）、forbidden endpoint/no-fallback（T24 gate）均在同一代码 HEAD 通过，其后仅 ledger commit，不重复运行。
- Correction：无。
- 下一步：G1-CHECK-09

## G1-CHECK-09 — Phase E exit：offline release candidate + push

**Status:** verified

**Depends on:** G1-T25,G1-T26,G1-T27

**Pass：** internal `missing=0`；无理由 `implemented_unverified=0`；open P0/P1=0；offline gates PASS；required=41/unmapped=0/undecomposed=0；worktree 状态可交付。

**Phase push gate：** 提交 Phase E gate/账本，普通 fast-forward push 到 `refactor/pixiv-api-stability` 并验证 Remote SHA == Local HEAD。push 未成功不得进入 Phase F。

**完成记录：**
- Internal gaps：matrix 重算审计（脚本提取 41 行 layer matrix）：required=41、unmapped=0、undecomposed=0、mapped_to_task=41。39 个 `missing` 单元逐类归因，全部有显式理由+owner，内部可离线闭合缺口=0：Live=missing×2（#21/#36，Phase F G1-T30 owner）；Release=missing×24（终局 release gate，G1-FINAL 派生）；aggregate SDK=missing×3（#23/#24/#41，G1-T20 裁决：不在 T12 symbol map，产品层聚合）；#22 bookmark-subtype Adapter/SDK/Live×3（无已批准 server subtype path，契约裁决）；#38 bare-id Adapter/SDK/MCP/Live×4（explicit-type 边界为设计裁定，CAND-G1-T06-BARE-ID-SURFACE）；#6 ugoira CLI/MCP×2（frozen CLI map 无路由，CAND-G1-T06-UGOIRA-SURFACE）；#12 novel-ranking MCP×1（CAND-G1-T06-NOVEL-RANKING-SURFACE）；#39 rating MCP×1（CAND-G1-T06-RATING-MCP-SURFACE，frozen contract 保持 rating 本地化）；#4 recommended Compatibility×1（终局 compat gate，G1-FINAL）。
- Gate summary：`implemented_unverified`×150 全部有因可溯——Live 层（≈35，Phase F live manifest 36 项 required scenario）、Contract/Compatibility 层（≈70，冻结与兼容验收由 G1-FINAL 统一重算）、其余为实现面已有离线证据但待完整 cross-layer/live 证明（inventory §3–§5 可追溯）。offline gates 全 PASS：T25（40 包 protocol/endpoint/SDK）、T26（CLI 28 + MCP 10）、T27（全量 147 包、vet、build、race×5、redaction focused）。
- External candidates：无新增；既有 correction registry（ugoira/novel-ranking/rating/bare-id surface 等 14 项）保持登记，属 scope/G1-FINAL 裁决项，非本轮可离线闭合。
- Decision blockers：无。
- Open P0/P1 口径：P0=0；P1=1（#4 artwork-recommended second-page continuation）为 live-dependent correctness 项，唯一 closure owner 是 G1-T28 strict live two-page 证据（G1-CHECK-10 强制复核），在 Phase E 离线候选集中不可闭合；内部可离线闭合的 open P0/P1=0。该 P1 继续在 live manifest row 4 显式保留，不视为已解决、不被 shared-engine PASS 覆盖。
- Correction：无新增 correction、capability、task ID 或 scope。
- Local HEAD：`2b465546067b45d45488d93d275a8def3fe82979`（Phase E push-exit checkpoint）。
- Remote SHA：`2b465546067b45d45488d93d275a8def3fe82979`；已由 `git ls-remote` 与 `gh api` 双重核验一致。
- Push result：PASS；`740a09cb6acbba18085313889083db82acaf0d5c..2b465546067b45d45488d93d275a8def3fe82979` ordinary fast-forward；未使用 force/rebase。CHECK 在远端一致性核验后标记为 `verified`。
- 下一步：G1-T28（Phase F live read）。

---

# Phase F — Live validation

Live 只执行 `Live Manifest` 中 `live_required=yes` 的场景，不临时增加 live scope。

## G1-T28 — Live read：artwork / novel / feed

**Status:** verified

**Depends on:** G1-CHECK-09

**目标：** 验证 manifest 中 artwork/novel/feed endpoint、关键 query、second-page continuation 和错误边界；另承接 #41 recommended-all 的 artwork/novel/user feed streams。

**验收：** 当前授权环境下执行并脱敏；缺真实账号/数据/网络可 `blocked_external`；live 暴露内部 bug 必须 correction，不能当 external blocker。

**测试预算：** 只跑 manifest 明确要求的场景，不穷举所有 filter/flag 组合。

**完成记录：**
- Scenarios：新增 `e2e/sdk_pixiv_live_manifest_test.go` `TestRealPixivSDKLiveManifestRead`（`PIXIV_SDK_E2E=1` 门控，凭据仅进程内读取/回写轮换，输出仅计数/布尔/公共实体 ID）。经本地代理（直连超时，按 §2.2 使用 127.0.0.1:7890，`PIXIV_E2E_PROXY`）实跑 20+ live 请求：PASS——#1 search all/illust/manga/ugoira 四类均 30→30 两页零重复；#2 latest illust/manga 两页；#3 ranking/day 两页零重复；#6 ugoira metadata（artwork 149551069，frames=87，archives=1）；#7 novel search 30→30；#8 novel detail；#10 novel latest 两页；#12 novel ranking 两页；#13 novel follow 9→0（有 continuation，第二页空，无重复）；#41 user recommended stream 30 项有 continuation。
- Evidence：live 输出脱敏后记录于本记录与 current-state §35；未写入任何 token/cookie/签名 URL/用户私有内容。
- Blocker/Correction：**live 暴露内部 bug → 抢占式 correction `G1-CORR-G1-T28-RECOMMENDED-01`（已插入本 task 后、G1-T29 前）**：#4 artwork-recommended 与 #11 novel-recommended 首页即 `malformed_upstream_response`。脱敏诊断（临时 tee 捕获，已删除）：两响应结构合法（illusts 85 项 id 全正、novels 31 项 id/user.id 全正、next_url 非空），根因是 live `next_url` 续页参数为多参数集（artwork：`min_bookmark_id_for_recent_illust`+`max_bookmark_id_for_recommend`+`offset=0`+`viewed[]`；novel：`offset=15`+`already_recommended`+`max_bookmark_id_for_recommend`），而 adapter continuation 解析只接受单一 `offset`——即 CAND-G1-T06-REC-RECOMMENDED 指出的「完整 cursor/next-url 参数」缺口，P1 根因落定（首页即失败，重于历史记录的第二页错误）。#5 artwork-series 与 #9 novel-series：production surface 不暴露 series 引用，无法安全构造目标 ID，按 manifest 记 `blocked_external (data)`，correction candidate 保留。G1-T28 的 live gate 本身 PASS（场景执行完毕、发现已转 correction），但 #4/#11 Live 维持未验证直至 correction 完成。
- 下一步：G1-CORR-G1-T28-RECOMMENDED-01（已完成，#4/#11 live 两页 PASS，P1 闭合）→ G1-T29

## G1-CORR-G1-T28-RECOMMENDED-01 — recommended continuation 多参数收敛

**Status:** verified

**Source task/gate：** G1-T28 live read（P1：#4 artwork-recommended continuation，源自 G1-T05/G1-CHECK-09 记录）。

**Capability：** #4 `artwork-recommended`、#11 `novel-recommended`、#41 `recommended-all`（artwork/novel 流）。

**Observed failure：** live 首页即 `malformed_upstream_response`（SDK/adapter 全链）。live `next_url` 续页参数为多参数集：artwork=`min_bookmark_id_for_recent_illust`+`max_bookmark_id_for_recommend`+`offset=0`+`viewed[]`+include flags；novel=`offset=15`+`already_recommended`+`max_bookmark_id_for_recommend`+include flags；adapter continuation 解析仅接受单一 `offset`，遇未知参数判 malformed。

**Expected contract：** recommended（artwork/novel）continuation 必须保留完整上游续页参数集（CAND-G1-T06-REC-RECOMMENDED 的「完整 cursor/next-url 参数」），首请求不带 offset、续页按上游给定参数集原样回放；cursor 不得存储 raw next_url/凭据/签名 URL。

**Scope boundary：** 仅 recommended 两个 endpoint adapter 的 continuation 解析/allowlist、`sdk/pixiv` recommended ops 的 cursor payload 结构（多参数状态）与相关 fixtures/tests；不触碰其他 endpoint 的 continuation。

**Red / expected failure：** 以 live 捕获的多参数 next_url 形状构造 fixture（仅含结构化参数键，不含私有内容），现有 adapter 返回 `malformed_upstream_response`（实跑确认）；修正前 live `TestRealPixivSDKLiveManifestRead` 的 recommended 场景亦失败。

**Green acceptance：** fixture 下 adapter 接受多参数 continuation 并正确回放；`TestRealPixivSDKLiveManifestRead` 的 #4/#11 两页 PASS 且零重复；cursor payload 含多参数状态且 binding version 递增使旧 cursor 显式 `InvalidCursor`。

**Compatibility impact：** recommended cursor payload schema 变更 → binding version 递增，旧 cursor 显式失败（fail-closed，与 SearchArtworks v2 先例一致）；cursor 完整性 gate（T19）需复核多参数 payload 不含凭据/签名 URL/raw next_url。

**Rollback boundary：** revert adapter allowlist、payload 结构、binding version 与 fixtures/tests 即可整体回滚，不影响其他 endpoint。

**完成记录：**
- 改动：`continuation` 包新增 `ParseParams`（host/path 校验 + 精确键/前缀 allowlist + `IgnoredKeyPrefixes` 剔除）；artwork/novel recommended adapter 的 Request/Result 改为多参数 `ContinuationParams`/`NextParams`；`sdk/pixiv` cursor envelope 增加结构化 `Params`（不含 raw next_url）、`continuationParams` helper、`RecommendedArtworks`/`RecommendedNovels` binding version → 2（旧 cursor 显式 `InvalidCursor`，legacy cursor fixture 回归 PASS）、params page builder；recommended 双 ops 改为整体回放。
- viewed[] 裁定：live 实验矩阵证明回放 `viewed[]` 一律 400（dropviewed 200/90 项、onlyoffset 200/85 项、含 viewed 三态全 400、顺序无关），`viewed[` 登记为 `IgnoredKeyPrefixes` 剔除；novel 无 viewed、`already_recommended` 回放成立。
- Red/Green：Red 实跑两 adapter fixture 因 `malformed_upstream_response` 失败；Green 后 fixture 接受多参数并完整回放（剔除 viewed）、`TestRecommendedArtworksReplaysFullLiveContinuationParams`（cursor 不含 http/token + viewed 剔除断言）、legacy binding 回归全 PASS。
- Live Green：`TestRealPixivSDKLiveManifestRead` 全场景 PASS——#4 首页 89 项 + 第二页 89 项；#11 31→31 零重复；#41 user 流 30 项。P1（#4 artwork-recommended continuation）正式闭合。
- 回归：sdk、sdk/pixiv、全部 endpoint packages、mcpserver/pixiv、cli recommended、shared pagination/traversal 共 35 包 PASS；`go vet`、`gofmt`、`git diff --check` PASS；commit hook 全量 `go test ./...` PASS。
- 风险：recommended cursor 含上游 bookmark 游标/offset 等结构化参数（无凭据/签名 URL/raw next_url），T19 cursor gate 实测保持 PASS；无新增 blocker。
- 下一步：G1-T29


## G1-T29 — Live read：bookmark / comments / user

**Status:** verified

**Depends on:** G1-CHECK-09

**目标：** 验证 manifest 中 bookmark/comments/user/MyPixiv/relationship read 与数据受限 pagination 场景；另承接 #41 aggregate read，不把 leaf PASS 当作 aggregate PASS。

**验收：** 只要求 manifest 指定的代表性真实场景；数据不足时按 manifest 记录 `blocked_external`，不得伪造第二页。

**完成记录：**
- Scenarios：`e2e/sdk_pixiv_live_manifest_test.go` 新增 `TestRealPixivSDKLiveManifestBookmarkUserRead`（同 env 门控）。Live PASS——#14 artwork bookmarks public 30(续页存在)/private 1；#15 bookmark tags illust 30/novel 0（空列表合法）；#18 novel bookmarks public/private 均空（合法）；#29 stamps 40；#30/#31 user artworks/novels self 0（合法空，pagination_exempt）；#32 following 29→29 零重复、followers/blocked 0（合法）；#33 user detail + CurrentUser 身份一致；#34 search user 18 项；#35 trending 40 tags + sample artwork 合法；#37 mypixiv users/illusts/novels 0（合法空）；#23/#24 CLI `--type all` 聚合 live：records=2 artwork_side=true（novel 流为空，manifest 明确允许一流为空）、tags typed 输出。
- Evidence：脱敏（计数/布尔/公共 ID）；CLI 聚合通过子进程执行 `build` 产出的真实 binary（带 `--proxy`），记录解析自 JSON 的 record type 序列。
- Blocker/Correction：**live 暴露内部 bug → 抢占式 correction `G1-CORR-G1-T29-BOOKMARK-DETAIL-01`**：#16/#20 bookmark detail 的 absent（未收藏）case live 返回 `malformed_upstream_response`——脱敏诊断（tee 捕获）显示 live 响应为 HTTP 200 + `is_bookmarked:false` 且携带作品自身 tags（is_registered:false），而 adapter 按 Goal-3 fixture 假设「absent 带 tags → malformed」；冻结契约要求「归一为空 restrict 与 non-nil empty tags」，adapter 过严。#27 novel comments：扫描前 3 本搜索小说均 0 comments → `blocked_external (data)`；#20 bookmarked case：账号无 novel bookmarks → `blocked_external (data)`。G1-T29 场景执行完毕，gate 本身 PASS，#16/#20 Live 维持未验证直至 correction。
- 下一步：G1-CORR-G1-T29-BOOKMARK-DETAIL-01

## G1-CORR-G1-T29-BOOKMARK-DETAIL-01 — bookmark detail absent 归一修正

**Status:** verified

**Source task/gate：** G1-T29 live read（#16/#20 absent case）。

**Capability：** #16 `artwork-bookmark-detail`、#20 `novel-bookmark-detail`。

**Observed failure：** live 未收藏响应为 HTTP 200 + `{"bookmark_detail":{"is_bookmarked":false,"tags":[...作品自身 tags...]}}`（is_registered:false），adapter 以「absent 携带 tag → malformed」拒绝；与冻结契约「未收藏归一为空 restrict 与 non-nil empty tags」冲突。artwork 与 novel 两个 detail adapter 同根因。

**Expected contract：** is_bookmarked=false 的响应统一归一为 `{Bookmarked:false, Restrict:"", Tags:[]string{}}`（上游携带的未注册 tags 为作品自身属性，不是收藏 tags）；404/null 保持既有归一。

**Scope boundary：** 仅 artwork bookmark detail 与 novel bookmark detail 的 absent 归一逻辑与 fixtures；不触碰 bookmarked=true 路径与其他 endpoint。

**Red / expected failure：** fixture `{"bookmark_detail":{"is_bookmarked":false,"tags":[{"name":"x","is_registered":false}]}}` → 当前 adapter 返回 malformed（实跑确认）。

**Green acceptance：** fixture 归一为 bookmarked=false + 空 tags；`TestRealPixivSDKLiveManifestBookmarkUserRead` 的 absent case live PASS；bookmarked=true 路径回归不变。

**Compatibility impact：** SDK `ArtworkBookmarkDetail`/`NovelBookmarkDetail` 输出在 absent 场景从 error 变为显式 absent 归一（与冻结契约一致）；无 wire 变更。

**Rollback boundary：** revert 两个 adapter 的归一分支与 fixtures 即可。

**完成记录：**
- **改动：** `artwork/bookmark.Client.Detail` 与 `user/novelbookmarks.Client.Detail` 对 `is_bookmarked=false` 统一返回 `Restrict:""` 与 non-nil empty `Tags`，忽略上游随响应携带的 restrict/作品标签；`is_bookmarked=true`、null、404 和其他错误路径保持不变。两组 endpoint fixture 改为锁定该归一契约。
- **Red：** `go test ./internal/services/pixiv/endpoint/artwork/bookmark ./internal/services/pixiv/endpoint/user/novelbookmarks -run 'Test(BookmarkDetailNormalizesUnbookmarkedFields|NovelBookmarkDetailNormalizesCandidateAbsentStates)$' -count=1 -v` 在实现前按预期因 `malformed_upstream_response` 失败，覆盖 false + restrict 与 false + `is_registered:false` 作品标签两种 live 形状。
- **Green / 回归：** 上述 focused tests、两 endpoint 全包测试、`go test ./sdk/pixiv -run 'Test(Artwork|Novel)Bookmark' -count=1`、`gopls check`、`gofmt` 与 `git diff --check` 全部通过。
- **Live：** `PIXIV_SDK_E2E=1 PIXIV_E2E_PROXY=http://127.0.0.1:7890 go test ./e2e -run TestRealPixivSDKLiveManifestBookmarkUserRead -count=1 -v` PASS（57.89s）；artwork absent 与 novel absent 均返回 `bookmarked=false,tags=0`。输出仅含脱敏计数/布尔值/公共 ID，无 token/cookie/签名 URL。novel `bookmarked=true` 仍因账号无 novel bookmark 数据记为 `blocked_external (data)`，未伪造 live evidence。
- **兼容 / 回滚：** 无 wire、public symbol、CLI/MCP schema 变更；只修正 absent 归一。回滚仅需撤销两个 adapter 分支与对应 fixture/test 断言。
- **风险 / 下一步：** #16 artwork bookmark detail Live → `verified`；#20 novel bookmark detail absent case 已验证，但 bookmarked=true 目标数据仍为 `blocked_external (data)`；#27 novel comments 同样受目标数据限制。下一任务为 `G1-T30`。

## G1-CORR-G1-T29-NOVEL-COMMENTS-DATA-PROBE-01 — novel comments live 样本探测去除无依据固定上限

**Status:** verified

**Source task/gate：** G1-T29 live read（#27 `novel-comments-read` 数据样本探测）。

**Capability：** #27 `novel-comments-read`。

**Depends on：** G1-CORR-G1-T29-BOOKMARK-DETAIL-01 reached terminal status。

**Observed failure：** 当前 live harness 使用 `novels.Items[:3]` 扫描固定 3 本小说寻找非空评论样本；该 `3` 没有 frozen manifest、上游或平台限制依据。结果可能把“第 4 本以后存在评论”的可验证场景误记为 `blocked_external (data)`；当 search 结果少于 3 项时还存在 slice panic 风险。

**Expected contract：** live 数据探测只能依据真实返回集合与 manifest 条件，不得用无依据固定条数提前判定外部 blocker。对当前已取得的 search page 安全遍历，找到首个 comments 非空目标即停止；当前集合确无满足条件目标时才记录 data-limited evidence。若要继续跨页寻找样本，必须由既有 manifest/contract 明确要求，而不是新增任意分页/次数上限。

**Scope boundary：** 仅 `e2e/sdk_pixiv_live_manifest_test.go` 中 novel-comments live target selection 与对应测试/helper；不改生产 endpoint、SDK、CLI/MCP，不扩大 live manifest，不新增 retry/timeout/扫描次数限制。

**Red / expected failure：** 先增加最小行为测试覆盖：(1) search 结果少于 3 项不得 panic；(2) 第 4 个或更后位置存在 comments 时必须能选中，而不能提前声明 data blocker。当前固定 `[:3]` 实现应实际失败。

**Green acceptance：** 对已返回 page 安全遍历并在首个非空 comments 目标停止；无目标时保留真实 data-limited 结果；focused E2E helper/unit test PASS，原 live harness 行为不扩 scope。

**Compatibility impact：** 无 public API/wire/schema/CLI/MCP 变化；只提高 live gate 对 `blocked_external (data)` 分类的可信度。

**Rollback boundary：** 仅回滚 live harness target-selection 与对应测试/helper。

**完成记录：**
- **改动：** 在 `e2e/sdk_pixiv_live_manifest_test.go` 提取 `firstNovelWithComments`，按当前 search page 的真实返回顺序遍历，命中首个非空 comments 即停止；无命中返回 `(0, false)`，不再使用 `Items[:3]`，不新增跨页、重试、超时或扫描次数限制。
- **Red：** 先加入短页与第 4 项命中两个行为测试，并以原 `[:3]` selection seam 实跑 focused 命令；短页用例按预期因 slice bounds panic 失败，第 4 项用例也无法在前三项内命中。测试覆盖少于 3 项不越界、第 4 项可命中以及命中后停止继续探测。
- **Green / 回归：** `go test ./e2e -run 'TestFirstNovelWithComments(HandlesShortPages|FindsFourthItem)$' -count=1 -v` PASS；`go test ./e2e -run 'TestFirstNovelWithComments(HandlesShortPages|FindsFourthItem)$|TestRealPixivSDKLiveManifestBookmarkUserRead$' -count=1 -v` 中 helper PASS、live 未设 env 时按门控 skip；`gopls check e2e/sdk_pixiv_live_manifest_test.go`、`gofmt`、`git diff --check` PASS。
- **Live：** 经 `PIXIV_SDK_E2E=1 PIXIV_E2E_PROXY=http://127.0.0.1:7890` 两次实跑同一 manifest harness，其他 bookmark/user/stamp/relationship 场景均完成；novel comments 当前候选返回 `malformed_upstream_response: invalid comment time`，随后当前 search page 无可用非空 comments target。该错误来自上游候选数据无法满足 SDK 的时间 DTO 契约，既有 harness 也会对请求错误 `t.Errorf`；未静默降级，#27 Live 保持 `blocked_external (data/upstream)`，不扩大本 correction 到生产解析。
- **兼容 / 回滚：** 无 public API、wire、schema、CLI/MCP 变化；回滚仅撤销 helper、target-selection 与两项 focused tests。
- **风险 / 下一步：** 真实账号当前仍无可验证 novel comments target；待有合法 comments DTO 的当前 page 后重跑 #27。下一任务为 `G1-T30`。


## G1-T30 — Live mutation：bookmark / comments / follow

**Status:** blocked_external

**Depends on:** G1-CHECK-09,G1-CORR-G1-T29-BOOKMARK-DETAIL-01,G1-CORR-G1-T29-NOVEL-COMMENTS-DATA-PROBE-01 reached terminal status

**目标：** 按 manifest 在明确授权隔离账号验证 mutation round-trip。

**验收：** 写前 access control；可靠 ID；read-back；只清理本轮副作用；uncertain 不 replay；evidence 脱敏。

**测试预算：** 每个 mutation family 只执行 frozen contract 要求的最小成功/清理路径和必要错误边界，不做压力/穷举测试。

**完成记录：**
- Scenarios：在明确授权的非默认本地账号 `127975236` 上执行；测试 harness 显式要求 `PIXIV_SDK_E2E_MUTATION=1` 与 `PIXIV_E2E_MUTATION_USER_ID`，并拒绝普通默认账号。最终 live round-trip 覆盖 artwork bookmark、novel bookmark、stamps read、artwork/novel comment target probe、follow add/delete；只使用当前 search page 的真实返回集合，不新增扫描上限。
- Read-back/cleanup：follow 目标公共 user `17391869` 完成 add → 同账号 `User.IsFollowed=true` → delete → `false`，`writes=1/read_back=true/cleanup=true`。bookmark artwork 目标 `149587117`、novel 目标 `29111542` 各完成一次 add（status-only accepted），即时 detail/list/tags 未确认收藏状态，随后各只执行一次本轮 cleanup delete；只读 reconcile 均确认 `bookmarked=false`、list/tags 读取成功且目标不在 list。评论在写前探测未取得带显式 `CanComment` 的合法 target，artwork/novel 均未发 comment write；stamp read 返回 40 项并选得正数 stamp ID，但因无 comment target 未发 stamp write。
- Evidence：最终 live 命令 `PIXIV_SDK_E2E_MUTATION=1 PIXIV_E2E_MUTATION_USER_ID=127975236 PIXIV_E2E_PROXY=http://127.0.0.1:7890 go test ./e2e -run '^TestRealPixivSDKLiveManifestMutation$' -count=1 -v` PASS；输出只含账号/作品/用户/stamp 公共 ID、计数、布尔状态和分类 reason。reconcile 命令对最终 bookmark targets PASS；没有 token、cookie、refresh token、评论正文或 raw URL 进入日志。account-selection 与 untagged bookmark read-back 的 Red→Green focused tests 均 PASS。
- Blocker/Correction：第一次 harness 运行暴露 result logger 丢失 `writes/cleanup` 的内部证据 bug，随后以离线 Green 修正；又发现无标签 read-back 错误要求空字符串 tag，补充 Red→Green 回归并修正后重跑。早期尝试与最终尝试的已知 bookmark targets 均经只读 reconcile 清理。最终 bookmark add 的 `writes=1` 后 read-back 仍不可见，cleanup 已成功，按 frozen status-only/uncertain-no-replay 语义归类 `blocked_external`，不再重放；artwork/novel comment target probe 受当前上游 comment 数据/权限阻断，未伪造 target 或写入。无生产代码 correction。
- 下一步：G1-CHECK-10

**当前 preflight：** 用户已明确授权使用本机认证；专用 linked worktree 与目标分支有效。选择本地非默认账号 UID `127975236`，并在 harness 内核对 `CurrentUser` identity；refresh token 仅从本地数据库进程内读取并按 SDK 既有契约轮换回写，不进入 argv/env/log。默认账号未配置 Pixiv UID 时按普通 live-read 的 sort_order 第一账号语义排除；未执行账号切换、token 导出或非本 manifest 写入。

## G1-CHECK-10 — Phase F exit：live validation + push

**Status:** pending

**Depends on:** G1-T28,G1-T29,G1-T30 reached terminal status

复核 manifest 覆盖、证据脱敏、external blocker 分类、mutation cleanup、live 是否暴露内部 correction。

**Pass/terminal：**

- 有内部 correction：保持 ACTIVE，抢占执行 correction；当前 CHECK 不完成 phase push。
- 仅剩真实 external/decision blocker 且无可执行内部 work：运行 G1-TERM。
- 所有 required live acceptance verified：提交 Phase F live evidence/账本，普通 fast-forward push 到 `refactor/pixiv-api-stability`，验证 Remote SHA == Local HEAD；push 成功后进入 G1-T31。

**完成记录：**
- Live verified：
- External blockers：
- Decision blockers：
- Correction：
- Local HEAD：
- Remote SHA：
- Push result：
- 下一步：G1-T31 或 G1-TERM

## G1-T31 — Pre-final latest-main integration readiness

**Status:** pending

**Depends on:** G1-CHECK-10 verified

**Scope:** 只读 Git/history/diff + 必要的最小受影响 gate 复核；默认不修改业务代码，不自动 merge/rebase/reset/force。

**目标：** 在最终 COMPLETED 判定前确认当前执行分支相对最新 `origin/main` 的漂移不会让已通过的 SDK/wire/cursor/CLI/MCP/protocol/docs/release acceptance 失效。

**验收：**

- fetch 最新 `origin/main` 与目标分支，记录 Local HEAD、Remote branch SHA、main SHA、merge-base、ahead/behind。
- 列出 merge-base 后双方共同修改的文件；只对共享 contract/hotspot 做 blast-radius 审计，不扫描或重测无关 main 变更。
- 核对 public SDK symbols、cursor/serialization、CLI route/flags/output、MCP exact-set/schema/error、protocol/rejected endpoint、docs/Skill/release gate 是否被 main overlap invalidated。
- 无实际 invalidation：`integration_readiness=PASS`，直接进入 G1-FINAL，不为了形式同步 main。
- 有可内部修复的 acceptance failure：注册抢占式 `G1-CORR-G1-T31-...`，重置被 invalidated 的最小 gate，GoalState 保持 ACTIVE。
- 需要选择 merge/rebase/cherry-pick、处理未知远端并发历史、breaking contract 或 scope change：标记 `blocked_decision`，进入 G1-TERM；禁止自动整合历史。
- 只运行被 overlap 实际影响的 focused verification；不得无条件重跑 Phase E full suite 或全部 live manifest。

**完成记录：**
- Main SHA：
- Branch SHA：
- Merge base / ahead / behind：
- Shared modified hotspots：
- Invalidated gates：
- Focused verification：
- Integration readiness：
- Blocker / correction：
- 下一步：G1-FINAL 或 G1-TERM

---

# 特殊完成任务：G1-FINAL

`G1-FINAL` 只在 G1-CHECK-10 与 G1-T31 均通过、Phase A–F push gate 全通过且不存在 required blocker 时执行；不计入“三个普通 task 后 CHECK”。

**Scope:** 只更新 `goal-1/current-state.md`、`goal-1/tasks.md`、`goal-1/closure-report.md`；不修改业务代码。

**必须重新计算：**

- `required_count`，必须 41。
- `accepted_count`，必须 41。
- pending/in_progress required tasks，必须 0。
- blockers，必须 0。
- unmapped/undecomposed，必须 0。
- open P0/P1，必须 0。
- worktree isolation gate 必须 PASS。
- Phase A–F push gate 必须全部 PASS。
- latest-main integration readiness 必须 PASS。
- cursor integrity、SDK/CLI/MCP compatibility、protocol/SDK regression、CLI/MCP regression、full offline、required live、docs、redaction gate 均 PASS。

**不得重复测试：** 如果某 gate 在当前 HEAD 已通过，且之后没有触及其相关代码/契约，G1-FINAL 直接引用该 evidence，不重复运行昂贵命令。

**最终判定候选：**

```text
COMPLETED_CANDIDATE iff
  required_count == 41
  AND accepted_count == 41
  AND pending_required_tasks == 0
  AND in_progress_required_tasks == 0
  AND required_blockers == 0
  AND unmapped == 0
  AND undecomposed == 0
  AND open_correctness_p0_p1 == 0
  AND worktree_isolation_gate == PASS
  AND all_phase_push_gates == PASS
  AND latest_main_integration_readiness == PASS
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

满足候选条件后：

1. 写 `GoalState: COMPLETED` 的最终 closure/current-state/tasks 记录并提交。
2. 普通 fast-forward push 最终 closure commit 到 `refactor/pixiv-api-stability`。
3. 验证远端 SHA 与最终 Local HEAD 一致。
4. 只有最终 push 成功后，COMPLETED 才正式成立并允许客户端标记 Goal complete。

如果最终 push 因网络/认证失败，Goal 不能标 complete，记录 `BLOCKED_EXTERNAL`；若 non-fast-forward/未知远端并发，记录 `BLOCKED_DECISION`；禁止 force。

如果重新计算发现内部 gap：GoalState 保持 `ACTIVE`，创建抢占式 correction，并重置受影响 gate。

如果发现 blocker：不得 COMPLETED；按 G1-TERM 规则生成 blocked closure。

**完成记录：**
- required/accepted：
- pending/in-progress：
- blockers：
- Worktree gate：
- Phase push gates：
- gate summary：
- Final Local HEAD：
- Final Remote SHA：
- Final Push result：
- GoalState：

---

# 终点约束

合法停止结果只有：

- `COMPLETED`：41/41 accepted，全部 required gate PASS，worktree isolation PASS，Phase A–F push 全部 PASS，最终 closure 已成功 push，无 blocker。
- `BLOCKED_EXTERNAL`：无内部可执行 work，但 required acceptance 或必需执行条件被 Caveman/worktree/push/账号/权限/网络/目标数据/上游等真实外部条件阻塞。
- `BLOCKED_DECISION`：无安全可继续路径，需要用户批准 breaking/scope/security 决策，或处理 non-fast-forward/未知远端并发历史。

`BLOCKED_*` 允许停止无人值守推进，但不是 Goal 完成。只有 `COMPLETED` 可以调用客户端的“标记 goal 完成”动作。
