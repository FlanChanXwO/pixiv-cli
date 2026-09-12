# Goal-1 Closure Report

**GoalState:** `ACTIVE`

**Report status:** `CURRENT_AUDIT_READY_FOR_G1-TERM` — 2026-09-13；G1-CHECK-10 已完成当前审计并到 terminal `blocked_decision`，下方历史 terminal evidence 保留用于审计；G1-TERM 尚未执行，GoalState 暂保持 `ACTIVE`。

**Generated:** 2026-09-13

**Branch:** `refactor/pixiv-api-stability`

**Worktree:** `/Users/flanchan/Developer/Projects/GithubProjects/.worktrees/pixiv-cli-refactor-pixiv-api-stability`（专用 linked worktree）

**Previous terminal task:** `G1-TERM`（superseded）

**Current next task:** `G1-TERM`（G1-CHECK-10 已到 terminal `blocked_decision`；GoalState 在 terminal task 执行前保持 `ACTIVE`）

## 0. Resume reason

- blocked closure 的关键前提是 `runnable_required_tasks == 0`。该前提已经失效：当前 comments adapter 从 `created_at` 读取评论时间，而当前 App API 参考模型使用 `date`；这与 live `invalid comment time` 形成直接、可测试的内部根因链。
- 当前 comments adapter 还期待 `access_control:{can_comment,is_locked}`，而当前 App API 参考模型使用 `comment_access_control` 整数。此前“没有显式 CanComment target”因此需要先按真实 wire 重新判定，不能继续无条件归类为 external data blocker；scalar 业务语义不得猜测。
- 已在 `tasks.md` 插入 `G1-CORR-G1-T29-COMMENT-WIRE-02`，并把 G1-T30/G1-CHECK-10 恢复为 pending；该 correction 已完成并记录 endpoint/SDK Red→Green 与当前 live read 证据，旧 G1-TERM checkpoint 只保留历史证据。Goal 当前保持 `ACTIVE`。
- correction 后 #27 当前 live novel comments 候选成功映射 2 条 comments，不再报 `invalid comment time`；`comment_access_control` scalar 语义仍未证实，因此没有映射、没有默认放行，也没有发起 comment mutation。
- `G1-RECOVER-G1-T28-SERIES-TARGET-01` 已完成：显式候选 `1206600` 首页 30 条并成功通过当前 SDK cursor 请求第二页 22 条；#9 novel-series 的 target blocker 解除，#5 artwork-series 仍为 data blocker。
- 恢复启动时，其余 external candidate 中，novel bookmarked=true 目标与 bookmark status-only read-back 暂未发现新的内部根因，继续保持；当时 #5/#9 series 数据仍是 external 条件，因此登记只读 recovery task，先验证公开 PixivPy demo 的 novel-series candidate，artwork-series 不进行任意 ID 扫描。本轮已仅解除 #9，#5 与 public-surface decision blockers 继续保持，直到满足恢复条件或用户明确批准 layer applicability/scope 裁定。

## 1. Terminal eligibility

> **Historical snapshot only.** 本节描述 superseded blocked checkpoint 当时的判定；当前已经因为存在 runnable correction 而不满足 terminal eligibility。

- Required capability scope remains `41/41`; `live_required=yes` 为 36 项，`live_required=no` 为 5 项；`unmapped=0`、`undecomposed=0`。
- G1-CHECK-10 已完成审计并标记 `blocked_decision`。既有 correction `G1-CORR-G1-T28-RECOMMENDED-01`、`G1-CORR-G1-T29-BOOKMARK-DETAIL-01`、`G1-CORR-G1-T29-NOVEL-COMMENTS-DATA-PROBE-01` 均已 `verified`。
- 没有仍可执行的 required task 或 correction。G1-T31 与 G1-FINAL 被 G1-CHECK-10 未通过的依赖条件锁定；因此 `G1-TERM` 是当前唯一可执行的 terminal task。
- Required blocker 同时包含真实 external blocker 与需要明确 public contract/scope 的 decision blocker。按 Goal-1 规则，二者并存时 GoalState 必须是 `BLOCKED_DECISION`。
- 当前 `public_ready=0/41`；required acceptance 尚未全部满足，不能进入 `G1-T31`、`G1-FINAL` 或 `COMPLETED`。

## 2. Live evidence retained

- G1-T28：feed/search/ranking/recommended/detail 的已执行场景保留当前 live evidence；#4/#11 recommended continuation 的内部 P1 已由对应 correction 闭合。
- G1-T29：bookmark/user/relationship/MyPixiv/stamps/aggregate read 场景按 manifest 记录；真实数据不足或上游 DTO 错误的场景没有伪造成功。
- G1-T30：follow mutation 在同一显式 secondary UID `127975236` 上完成 add → read-back → cleanup → read-back；stamps read 返回 40 项。bookmark status-only write 的 read-back 不可确认但 cleanup/reconcile 干净；comments 没有安全 target，因此没有发出写入。
- 所有 live harness 输出仅保留公共 ID、计数、布尔值与安全 reason；没有 token、cookie、refresh token、评论正文或 raw URL 进入日志。

## 3. Required blockers

### External blockers（恢复前历史快照）

| Capabilities | Evidence | Required recovery condition |
|---|---|---|
| #5 artwork-series、#9 novel-series | 当前认证环境没有可安全构造且满足 manifest 第二页要求的真实 series target | 提供合法、可访问且满足分页条件的真实目标数据 |
| #20 novel-bookmark-detail | 账号没有可验证的 `bookmarked=true` novel target | 账号出现可安全读取的 novel bookmark，或提供同等授权目标 |
| #27 novel-comments-read | 当前 search page 候选返回 `invalid comment time`，无法形成 SDK 可映射的合法非空 comments target | 上游返回可合法映射的 comments DTO/目标数据 |
| #17/#21 bookmark mutation | add 成功返回后 detail/list/tags 未确认状态；cleanup 与 read-first reconcile 均确认无残留，未 replay | 上游状态可稳定 read-back，重新执行最小 round-trip |
| #26/#28 comment mutation | 当前没有显式 `CanComment` 且可安全映射的 artwork/novel target；本轮零 comment write | 出现满足权限、access-control、可靠 ID 与 cleanup 条件的目标 |

### Decision / scope blockers

| Capabilities | Current boundary | Required decision |
|---|---|---|
| #6 `ugoira-metadata`、#12 `novel-ranking` | required surface 的 CLI/MCP 或 MCP owner 缺失，只有 correction registry | 是否批准新增 public command/tool、schema、文档与 compatibility contract |
| #23/#24/#41 aggregate | CLI/MCP 离线 aggregate 存在，但 public aggregate SDK、strict aggregate/live contract 未冻结；#41 不能由单路 recommended live 证据代表 | 是否批准 aggregate SDK/API 与完整四流 live contract |
| #38 `bare-id-probe`、#39 `rating-filter` MCP | 当前 frozen boundary 明确禁止隐式 ID probe 与未经确认的 server-side rating；MCP rating surface 未批准 | 是否扩大/改变 public surface，或继续保持 rejected boundary |

这些 decision blockers 不是本轮可在不扩大 scope 的情况下安全实现的内部修复；本轮没有修改生产代码、public API、CLI/MCP schema 或 Goal-3 scope。

## 4. Verification evidence

- `go test ./...`：PASS（当前 HEAD，且 closure commit hook 再次执行）。
- `go vet ./...`：PASS。
- `sh scripts/build.sh`：PASS。
- documentation tests、LSP check、focused mutation tests、live mutation、read-first reconcile、`gofmt` 与 `git diff --check`：PASS。
- G1-CHECK-10 已核对任务状态：无可执行 pending/in-progress required work；未发现新的 live-induced production correction。

## 5. Git and push checkpoint

- G1-CHECK-10 audit base：`a3f8d31`。
- Audit 前远端：`refactor/pixiv-api-stability`=`8589ea1bfa16cef3df3c9dbe2d4fffa8fe9670ec`；`origin/main`=`7ff1e6b4e6177c657876f69cd930d279d2d0bfce`。
- Closure checkpoint commit：`2d60489d5355ac0ed31703fceab47415945be074`。
- Push result：PASS；`8589ea1bfa16cef3df3c9dbe2d4fffa8fe9670ec..2d60489d5355ac0ed31703fceab47415945be074` ordinary fast-forward；未使用 force/rebase。GitHub API 与 `git ls-remote` 均确认 Remote SHA=`2d60489d5355ac0ed31703fceab47415945be074`，与 closure checkpoint Local HEAD 一致。
- Post-push ledger：本报告的 push 结果更新会作为后续 documentation commit 再次普通 fast-forward 推送；发布后再次核验 Remote SHA == 当前 Local HEAD。禁止 force、reset 或删除其他 worktree。

## 6. Resume conditions

若 external 数据/上游条件恢复，或用户明确批准所需 public contract/scope，旧的 blocked closure report 应标记为 superseded；相关 task 从最早未完成 required task 恢复，重新执行受影响的最小 live/compatibility gate。当前报告不代表 Goal 完成。

## 7. Current resumed progress

- `G1-CORR-G1-T29-COMMENT-WIRE-02` 已 verified：novel comment DTO 现在按当前 App API `date` wire 优先映射，旧 `created_at` 合法 fixture 保持兼容；focused endpoint/SDK、novel endpoint regression、LSP、gofmt 与 diff check 均通过。
- #27 live read 已用已授权本机认证、临时代理和既有 manifest harness 复核：候选 `29100695` 返回 2 条 comments，`continuation=false`、`total=false`、`access_control=false`，整个 `TestRealPixivSDKLiveManifestBookmarkUserRead` PASS。日志未包含 token、cookie、refresh token、raw URL 或评论正文。
- `comment_access_control` 的整数业务语义仍没有证据确认；不猜测 `CanComment`/`IsLocked`，不默认放行写入。#26/#28 的受影响 comment target/mutation slice 已完成安全复核并归类为 `blocked_external(data/permission)`；G1-T30 已到 terminal `verified`，G1-CHECK-10 已到 terminal `blocked_decision`。
- G1-T30 只重跑了受 comments wire correction 影响的 comments/access-control slice；既有 follow/stamps/bookmark evidence 没有重放。
- **Next:** `G1-TERM`。

## 8. Current resumed progress — G1-T30 comments/access-control slice

- **TDD / harness：** 新增只读 `comment_access_control` wire probe、独立 comments mutation gate 与无 target 的 `content_unavailable` 分类。decoder/classification focused tests 先 Red 后 Green；malformed/invalid 读取错误仍进入 correction；raw probe 复用既有 `liveManifestPace=1200ms`，不添加新的 retry、timeout、扫描上限或 fallback。
- **Scalar live evidence：** `PIXIV_SDK_E2E_COMMENT_ACCESS_CONTROL_PROBE=1 PIXIV_E2E_MUTATION_USER_ID=127975236 PIXIV_E2E_PROXY=http://127.0.0.1:7890 go test ./e2e -run '^TestRealPixivSDKLiveCommentAccessControlProbe$' -count=1 -v` PASS（约 74s）。artwork 当前 search page `30/30` 响应成功，scalar 为 integer，其中 `0`×29、`1`×1；novel `30/30` 响应成功且全为 `0`。只记录 wire 事实，不将整数映射为业务权限。
- **Mutation live evidence：** `PIXIV_SDK_E2E_COMMENT_MUTATION=1 PIXIV_E2E_MUTATION_USER_ID=127975236 PIXIV_E2E_PROXY=http://127.0.0.1:7890 go test ./e2e -run '^TestRealPixivSDKLiveCommentMutationSlice$' -count=1 -v` PASS（约 77s）。stamps read 成功；artwork text/reply/stamp 与 novel text/stamp 全部在写前 preflight 因无显式 `CanComment` target 阻断，`writes=0`，没有评论/回复/stamp 写入。
- **判定：** #26/#28 在 comments wire correction 后正式归类为 `blocked_external(data/permission)`；#27 的 `date` correction/live read 保留。follow/stamps/bookmark evidence 保留，未重放；没有 token、cookie、refresh token、raw URL 或评论正文进入日志。无 production SDK/endpoint/CLI/MCP/public schema 变化，仅增强 env-gated e2e 证据与安全分类。
- **下一步：** `G1-TERM`；本报告仍为 `ACTIVE`，不代表 Goal 已完成。

## 9. Current G1-CHECK-10 audit（2026-09-13）

- **Task verdict：** `G1-CHECK-10=blocked_decision`（terminal）。manifest 41 行 ID `1..41` 各出现一次，`live_required=yes/no=36/5`，`mapped_to_task=41`、`unmapped=0`、`undecomposed=0`；所有普通 task、correction、recovery 与 G1-T30 已到 terminal，未发现新的内部 correction。G1-T31 依赖本 CHECK `verified`，G1-FINAL 继续锁定。
- **Current live evidence：** #9 novel-series recovery 已完成真实首页 30 → opaque cursor 第二页 22；#27 `date` wire correction 后成功映射 2 条 comments；#36 follow 完成同账号 add/read-back/delete/read-back；#4/#11 recommended continuation P1 已闭合；#29 stamps read 成功。旧 #27 `invalid comment time` blocker 已解除。
- **External blockers：** #5 artwork-series 无可追溯目标；#17/#21 bookmark status-only write 后状态不可确认但 cleanup/reconcile 干净，按 uncertain no-replay；#20 novel bookmark detail 缺 `bookmarked=true` 目标；#26/#28 无显式且可安全判定的 `CanComment` target，comments slice `writes=0`，未发评论/回复/stamp 写请求。
- **Decision / scope blockers：** #6、#12、#23、#24、#41、#38、#39 仍需用户明确批准 layer/public-surface 裁定；不在 terminal audit 中新增 CLI/MCP/SDK contract、aggregate operation、rating surface 或 bare-ID probe。`comment_access_control` 的 scalar 业务语义保持未定义。
- **Safety / verification：** mutation cleanup/reconcile/no-replay evidence 保留；live 输出未包含 token、cookie、refresh token、评论正文或 raw signed/auth URL。文档测试、cursor/SDK route-safety、Pixiv/FANBOX MCP schema/error-canary focused tests PASS；G1-T27 当前 HEAD `e74fcb6` 的 full offline/vet/build/race/LSP/live/reconcile/diff-check 证据未被 docs-only audit invalidated。
- **Git / routing：** 审计起点 Local/Remote=`e74fcb6836781064f755f74a3d8b6c96a99cc63f`，`origin/main=7ff1e6b4e6177c657876f69cd930d279d2d0bfce`，worktree clean、无 PR、无远端分叉。本 audit 文档按普通 fast-forward 推送；因 CHECK verdict 不是 `verified`，该 push 不宣称 Phase F push gate 通过。下一张 `G1-TERM` 需单独写 closure checkpoint 并普通 fast-forward 推送。
- **GoalState：** 仍为 `ACTIVE`；G1-TERM 执行后才可按混合 blocker 计算 `BLOCKED_DECISION`。当前不得进入 G1-T31/G1-FINAL。
