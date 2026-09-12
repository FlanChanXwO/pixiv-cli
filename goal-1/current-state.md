# Goal-1 当前状态：Capabilities 1–41 baseline inventory

> 本文件覆盖 `G1-T01`、`G1-T02`、`G1-T03`、`G1-CHECK-01`、`G1-T04`、`G1-T05`、`G1-T06`、`G1-CHECK-02`、`G1-T07`、`G1-T08`、`G1-T09`、`G1-CHECK-03`、`G1-T10`、`G1-T11`、`G1-T12`、`G1-CHECK-04`、`G1-T13`、`G1-T14`、`G1-T15`、`G1-CHECK-05`、`G1-T16`、`G1-T17`、`G1-T18`、`G1-CHECK-06`、`G1-T19`、`G1-T20`、`G1-T21`、`G1-CHECK-07`、`G1-T22`、`G1-T23`、`G1-T24`、`G1-CHECK-08`、`G1-T25`、`G1-T26`、`G1-T27`、`G1-CHECK-09`、`G1-T28`、`G1-CORR-G1-T28-RECOMMENDED-01`、`G1-T29` 与 `G1-CORR-G1-T29-BOOKMARK-DETAIL-01`，只记录当前分支的代码、测试、历史和 Goal-3 证据，不授予任何 capability 的发布资格。

## 1. 快照与状态口径

- 执行分支：`refactor/pixiv-api-stability`
- G1-T03 inventory 起始 commit：`35dd0d7efebd16eeda6d5fb8ac3e766b31f8461d`，继承基线：`e40443495981cdaf01215d6711cb24fa618b087a`
- 当前 worktree：`/Users/flanchan/Developer/Projects/GithubProjects/.worktrees/pixiv-cli-refactor-pixiv-api-stability`
- G1-T03 开始时 worktree 干净；G1-CHECK-01 检查时 HEAD 为 `042c0200ce1b3349ab3e08a744fea7802838025c`。G1-T04 审计起始 HEAD 为 `f7cff3fb86dcc5117c634359bedbea5abdfa4d0b`；G1-T05 correctness audit 起始 HEAD 为 `35885b0d9316b4a1f2326bcee0511d8bc5ae29ed`；G1-T06 manifest audit 起始 HEAD 为 `c5e3d679eef4b2b2404984bbd6630588a3c7ac8a`，worktree 干净。G1-CHECK-02 pre-push ledger commit 为 `b17b1775f2ea0783f394f7600d30cafd8ad428c5`，已普通 fast-forward push，远端 SHA 与该 checkpoint 一致。其后 G1-T07 在 `5b45827efc5b553a0ad8caca5289253dfe585ba` 收敛 MCP user identity 行为、测试与双语文档；G1-T08 仅修正双语 MCP contract 文档并同步本状态账本；G1-T09 新增 candidate/offline novel bookmark tags/detail MCP read、补 typed bookmark wire/empty/error/subtype logical-pagination 测试与双语文档；G1-CHECK-03 对 MCP user/typed bookmark 的 owner、schema/error、resolver/filter/pagination、legacy wire、forbidden endpoint 与 abstraction scope 审计通过；G1-T10 新增 `bookmark_list_all` / `bookmark_tags_all` additive MCP aggregate，补双流统一 budget、重放、页原子失败、typed tag 与 exact registration evidence；G1-T11 对 MCP read registration、44 个 client-visible tool exact-set、output schema、structured error 与 forbidden endpoint 做 no-op gate 复核并通过；G1-T12 对 read legacy JSON replay、structured error wire 与 stdio stdout/stderr boundary 做 no-op gate 复核并通过；G1-CHECK-04 已完成 Phase B read 完整性审计并将 `c902b342ddec687a078f6fff99967dfb4689046a` 普通 fast-forward push，Remote SHA 与 Local HEAD 一致；G1-T13 已新增 novel bookmark public SDK mutation 与两个 additive MCP tools，并完成 offline outcome/schema/no-replay evidence；G1-T14 已新增四个 artwork-side comment/stamp mutation tools，直接保留 response `comment_id`，完成 input/schema/outcome/error/no-replay、50-tool exact registration、双语文档与 full offline/build evidence；G1-T15 已新增四个 novel-side comment/stamp mutation tools，直接保留 response `comment_id`，完成 input/schema/outcome/error/no-replay、54-tool exact registration、双语文档与 full offline/build evidenceG1-CHECK-05 已集中复查 bookmark/comment mutation 的可靠 ID、structured outcome/error、legacy wire、uncertain no-replay 与 abstraction scope，结论 PASS、无 correction；实现提交分别为 `41e4b2dc2ced61662e464a0564917e393ce79fa8` 与 `484ffee5629299e644b78da18d24345329479613`；相关 ledger commits 均为 local-only，目标 worktree clean，remote SHA=`6bf64c208719f4e5dff3c0af7bf93e2630d3d340`；严格 live、access-control、写后 read-back/cleanup、public compatibility 与 release 仍未关闭；`goal-3/` 无 diff。G1-T16 对 follow/unfollow mutation 做 no-op verified 并补齐 uncertain 不 replay、invalid input 网络前拒绝与 MCP typed failure 离线回归，未改生产代码。G1-T17 对 shared mutation outcome/uncertainty 语义做 no-op verified：classifyStatus 的 definite/uncertain 分类、pool/facade 的 committed-before-replay 边界与 14 个 mutation tool 对 runtime.Write/RunMutation helper 的全量复用均有实跑证据，未新增 framework。G1-T18 补齐 offline read-back 编排回归并复核旧 mutation wire/structured error 证据，未执行 live 写入。G1-CHECK-06 已完成 Phase C mutation 完整性审计并将 `5a057ad6f7510c7c005c22974012b830152e3ed9` 普通 fast-forward push，Remote SHA 与 Local HEAD 一致。G1-T19 对 cursor integrity/binding/rollback gate 做 no-op verified：封闭 envelope、binding 校验、版本 fail-closed 与 batch/checkpoint/replay 回归均有实跑证据，未改生产代码。G1-T20 对 public SDK compatibility 做 no-op verified：pinned inventory、old consumer 编译、legacy wrapper、NovelContent 零网络负向与冻结 cursor 语义均有实跑证据，无 breaking；aggregate SDK surface 保持显式 missing、不当作已接受。G1-T21 按 frozen map 补齐 `bookmark add/remove` 的 novel namespace dispatch（Red→Green），复核 trending 的真实 CLI surface 为 `search --trending-tags` 并纠正 stale matrix verdict，ugoira CLI 缺口保持显式登记。G1-CHECK-07 已集中复查 cursor+SDK+CLI 三项 gate 并在当前 HEAD 复跑组合回归，结论 PASS、无 correction、无 breaking blocker。G1-T22 对 MCP compatibility 做 no-op verified：T39A 40-tool frozen map + 14 recorded additive 的 exact-set、四组 schema/replay、rejected path 零注册与 rating 冻结语义均有实跑/静态证据，无 breaking。G1-T23 已完成 `bookmark add/remove --type` 的双语 cli-reference、skills/pixiv-cli 与 unreleased changelog 同步，documentation tests PASS。G1-T24 已通过 forbidden endpoint/no-fallback gate：required public paths 无 rejected endpoint/WebView/anonymous 触达，legacy novel resource 路径登记为 out-of-scope observation。G1-CHECK-08 已完成 Phase D exit 审计并将 `c4a99ec4ecb3646bced231d493db94d641405969` 普通 fast-forward push，Remote SHA 与 Local HEAD 一致。G1-T25 已完成 protocol/endpoint/SDK regression：`go test ./internal/services/pixiv/... ./sdk/... -count=1` 40 包全 PASS（含 old consumer 与 rejected endpoint 负向），零失败。G1-T26 已完成 CLI + MCP regression：`go test ./internal/cli/... ./internal/mcpserver/... -count=1` 28+10 包全 PASS（含 legacy replay、stdio 边界、exact-set 与 aggregate/mutation offline safety），零失败。G1-T27 已通过 full offline release gate：`go test ./...` 147 包、`go vet ./...`、`sh scripts/build.sh`、5 包 race 与 redaction focused 全部 PASS。G1-CHECK-09 已完成 Phase E exit 矩阵重算审计（39 missing/150 implemented_unverified 逐类归因、内部缺口=0、离线可闭合 P0/P1=0、P1×1 显式转 live gate）并将 `2b465546067b45d45488d93d275a8def3fe82979` 普通 fast-forward push，Remote SHA 与 Local HEAD 一致。G1-T28 已执行 live manifest artwork/novel/feed 场景（经代理）：11 类场景 PASS；#4/#11 recommended 首页即失败并定位根因（live next_url 多参数续页集 vs adapter 单 offset 解析），已注册抢占式 correction `G1-CORR-G1-T28-RECOMMENDED-01`；#5/#9 因 surface 无法安全构造 series ID 记 blocked_external (data)。G1-CORR-G1-T28-RECOMMENDED-01 已完成：recommended 多参数 continuation 收敛（viewed[] 按 live 证据剔除）、binding v2 fail-closed，#4/#11 live 两页 PASS，P1 闭合。G1-T29 已执行 bookmark/comments/user live 场景（15 类 PASS，#27 数据受限、#16/#20 absent live bug），并注册抢占式 correction `G1-CORR-G1-T29-BOOKMARK-DETAIL-01`。
- G1-CORR-G1-T29-BOOKMARK-DETAIL-01 已完成：artwork/novel bookmark detail 对 `is_bookmarked=false` 统一归一为空 restrict 与 non-nil empty tags；Red→Green focused fixture、endpoint/SDK 回归、`gopls check` 与脱敏 live manifest 均 PASS。#16 Live → verified；#20 absent case PASS，bookmarked=true 因账号无 novel bookmark 数据保持 `blocked_external (data)`；下一任务为 G1-T30。
- Goal-3 的 `goal-3/capability-admission.md` 是 capability 状态唯一权威来源：当前盘点的 41 项均为 `required=yes, state=scope_admitted`；全 41 项中没有一项为 `public_ready`（`goal-3/capability-admission.md:3-15,23-70`；`artwork-series` 在该表后段，仍属于原 required 集合）。
- `scope_admitted` 只表示 capability 属于 required scope；不表示 contract 已冻结、迁移已完成或可以进入正式发布 surface。
- 本文状态：
  - `verified`：当前源码/离线测试/已有证据可以直接核验该层事实。
  - `implemented_unverified`：已有实现或历史证据，但缺少当前 Goal 要求的完整跨层、live、兼容或发布证明。
  - `missing`：当前责任链或必要证据明确不存在。
  - `rejected`：该层被当前发布门禁拒绝；若涉及 endpoint，则同时表示不得调用或 fallback。
  - `not_applicable`：该能力不需要该层；不是缺口，也不授予其他层发布资格。

## 2. 覆盖计数与总览

### 2.1 Required coverage

- Capabilities：41/41，当前盘点能力全部纳入 required scope。
- `public_ready`：0/41。
- `scope_admitted`：41/41。
- 当前没有 capability 可以仅凭 Goal-3 历史 task 的 `verified` 标记直接转为 accepted。
- 当前未发现新的业务/API/evidence drift；Goal-1 分支相对继承基线的 drift 只有执行资料与 G1-T01/G1-T02/G1-T03/G1-CHECK-01/G1-T04/G1-T05/G1-T06/G1-T07/G1-T08/G1-T09/G1-CHECK-03/G1-T10/G1-T11/G1-T12/G1-CHECK-04/G1-T13/G1-T14/G1-T15 记录。

### 2.2 Layer matrix

| # | Capability | Contract | Adapter | SDK | Shared | CLI | MCP | Offline | Live | Compatibility | Release |
|---:|---|---|---|---|---|---|---|---|---|---|---|
| 1 | `artwork-search` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 2 | `artwork-latest` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 3 | `artwork-ranking` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 4 | `artwork-recommended` | implemented_unverified | verified | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | verified | verified | missing | rejected |
| 5 | `artwork-series` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | blocked_external | implemented_unverified | rejected |
| 6 | `ugoira-metadata` | implemented_unverified | verified | implemented_unverified | verified | missing | missing | verified | verified | implemented_unverified | rejected |
| 7 | `novel-search` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | verified | missing |
| 8 | `novel-detail` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 9 | `novel-series` | implemented_unverified | verified | verified | verified | verified | verified | verified | blocked_external | implemented_unverified | missing |
| 10 | `novel-latest` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 11 | `novel-recommended` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 12 | `novel-ranking` | implemented_unverified | verified | verified | verified | verified | missing | verified | verified | implemented_unverified | missing |
| 13 | `novel-follow` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 14 | `artwork-bookmark-list` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 15 | `artwork-bookmark-tags` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 16 | `artwork-bookmark-detail` | implemented_unverified | verified | verified | not_applicable | verified | verified | verified | verified | implemented_unverified | missing |
| 17 | `artwork-bookmark-mutation` | implemented_unverified | verified | verified | not_applicable | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 18 | `novel-bookmark-list` | implemented_unverified | implemented_unverified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 19 | `novel-bookmark-tags` | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | missing |
| 20 | `novel-bookmark-detail` | implemented_unverified | implemented_unverified | implemented_unverified | not_applicable | verified | verified | verified | blocked_external | implemented_unverified | missing |
| 21 | `novel-bookmark-mutation` | implemented_unverified | implemented_unverified | verified | not_applicable | verified | verified | verified | missing | implemented_unverified | missing |
| 22 | `bookmark-subtype` | implemented_unverified | missing | missing | implemented_unverified | verified | verified | verified | missing | implemented_unverified | missing |
| 23 | `bookmark-list-all` | implemented_unverified | verified | missing | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 24 | `bookmark-tags-all` | implemented_unverified | verified | missing | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 25 | `artwork-comments-read` | rejected | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | verified | rejected | implemented_unverified | rejected |
| 26 | `artwork-comments-mutation` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 27 | `novel-comments-read` | implemented_unverified | verified | verified | verified | verified | verified | verified | blocked_external | implemented_unverified | missing |
| 28 | `novel-comments-mutation` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 29 | `stamps` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 30 | `user-artworks` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 31 | `user-novels` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 32 | `user-relationships` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 33 | `user-detail` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 34 | `user-search` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 35 | `trending` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 36 | `follow-mutation` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | missing | implemented_unverified | rejected |
| 37 | `mypixiv` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 38 | `bare-id-probe` | implemented_unverified | missing | missing | verified | implemented_unverified | missing | verified | missing | rejected | rejected |
| 39 | `rating-filter` | implemented_unverified | verified | implemented_unverified | verified | verified | missing | verified | implemented_unverified | implemented_unverified | rejected |
| 40 | `logical-pagination` | implemented_unverified | implemented_unverified | implemented_unverified | verified | implemented_unverified | implemented_unverified | verified | implemented_unverified | implemented_unverified | rejected |
| 41 | `recommended-all` | implemented_unverified | implemented_unverified | missing | implemented_unverified | verified | implemented_unverified | verified | implemented_unverified | implemented_unverified | rejected |

`Release=rejected` / `Release=missing` 均表示当前不能发布，不表示 required scope 可以删除。当前 41 项必须继续沿 Goal-1 Phase A–F 完成各自 gate；G1-T04 已补齐 25–41；矩阵仍只记录事实 verdict，不授予发布资格。

## 3. Capability inventory

### 3.1 `artwork-search`（#1）

- **Contract — `implemented_unverified`**：目标为 `/v1/search/illust`；contract 已记录四种 content type、首请求不带 `offset`、续页使用正 `offset`、query 与 cursor binding（`goal-3/upstream-contract-matrix.md:29-33,51-57,251-254`）。`x_restrict`/rating 不是已确认的 server-side filter，不能伪装成 upstream contract（`goal-3/upstream-contract-matrix.md:33`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/artwork/search/search.go:63-107,116-145` 负责 route、filters、offset、required list 与 continuation；`search_test.go:26-113` 覆盖 query、normalized artwork、空/null list 和非法 continuation。
- **SDK — `implemented_unverified`**：公开 `Client.SearchArtworks` 位于 `sdk/pixiv/ops_artwork_search.go:11-136`，request 位于 `sdk/pixiv/request.go:103-123`；SDK 两页 fixture 位于 `sdk/pixiv/pixiv_test.go:493-543`。typed cursor 已存在，但 query/account/content-type/AI/local filter 的完整兼容 gate 尚未闭包。
- **Shared — `verified`**：使用 typed cursor/pagination，不由 CLI/MCP 解析 raw `next_url`；相关约束见 `goal-3/pagination-validation-report.md:66-68`。
- **CLI — `verified`**：`internal/cli/commands/pixiv/search/search.go:210-243,342-379` 支持 search 与 `--content-type`，调用 SDK；两页 fixture 位于 `internal/cli/commands/pixiv/search/bookmark_test.go:180-255`。
- **MCP — `verified`**：`search_illust` handler 位于 `internal/mcpserver/pixiv/tools/search_illust/search_illust.go:21-36,112-144`，聚合注册位于 `internal/mcpserver/pixiv/pixiv.go:114`。
- **Offline — `verified`**：endpoint、SDK、CLI、MCP 相关 fixture 和 targeted tests 已存在；G1-T01 的 `go test ./...` 也通过。（具体证据索引：本文件:66-70）
- **Live — `verified`（局部 frozen case）**：`search-illust-all/illust/manga/ugoira` 均为 HTTP 200、30→30、`offset`，wire/response/pagination/adapter/SDK confirmed（`goal-3/evidence/appapi-upstream.md:20-23`）。这不覆盖 rating、日期字段或所有未来 filter 语义。
- **Compatibility — `implemented_unverified`**：`SearchArtworks` cursor 必须绑定 query、账号、content type、AI/local filter context；要求见 `goal-3/upstream-contract-matrix.md:251-254`，但 capability 仍未达到 `migration_ready`。
- **Release — `rejected`**：capability authority 仍为 `scope_admitted`，不允许正式 release；不是 upstream endpoint rejection。

### 3.2 `artwork-latest`（#2）

- **Contract — `implemented_unverified`**：目标 continuation 是 `max_illust_id`；当前 evidence 只完整确认基础 `illust`，扩展 subtype 尚未等价确认（`goal-3/upstream-contract-matrix.md:35,59`）。
- **Adapter — `verified`**：timeline adapter 保存 `max_illust_id`；null list、invalid request、subtype route 测试见 `internal/services/pixiv/endpoint/artwork/timeline/timeline_test.go:62-81,148-157`。
- **SDK — `implemented_unverified`**：`LatestArtworks` 位于 `sdk/pixiv/ops_artwork.go:133-230`，绑定 resolved content type；代码仍兼容解析 offset/max ID 两种形式，目标迁移 contract 尚未闭合。
- **Shared — `verified`**：使用 shared cursor/pagination，binding rationale 见 `sdk/pixiv/ops_artwork.go:140-143`。
- **CLI — `verified`**：`timeline` command 支持 content type flag（`internal/cli/commands/pixiv/timeline/timeline.go:58-62`），调用 `LatestArtworks`。
- **MCP — `verified`**：`timeline_illust_latest` 位于 `internal/mcpserver/pixiv/tools/timeline_illust_latest/timeline_illust_latest.go:18-57`，注册于 `internal/mcpserver/pixiv/pixiv.go:118`；当前只允许 `illust`/`manga`。
- **Offline — `verified`**：timeline endpoint、SDK、CLI、MCP fixture 已覆盖基础分页和 continuation 错误。（具体证据索引：本文件:79-83）
- **Live — `implemented_unverified`**：`illust-new` 已有 30→30、`max_illust_id` 的 strict evidence（`goal-3/evidence/appapi-upstream.md:26`），但 `manga`、`ugoira` 和 compound subtype 仍是 partial（`goal-3/upstream-contract-matrix.md:59`）。
- **Compatibility — `implemented_unverified`**：SDK 已绑定 resolved content type，但 offset 兼容形态与 frozen `max_illust_id` 迁移仍需 public compatibility gate。
- **Release — `rejected`**：未完成已承诺 subtype、continuation 和 public gate；不能把基础 `illust` evidence 扩大为完整 capability。

### 3.3 `artwork-ranking`（#3）

- **Contract — `implemented_unverified`**：`mode/date/offset` contract、allowlist 和 invalid input 规则见 `goal-3/upstream-contract-matrix.md:36,60,234`。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/artwork/ranking/ranking_test.go:26-88` 覆盖 route/query/continuation/default day/invalid request。
- **SDK — `implemented_unverified`**：`ArtworkRanking` 位于 `sdk/pixiv/ops_artwork.go:75-99`；默认 day、mode/date 校验和 offset cursor 有实现与测试，但 public compatibility gate 尚未通过。
- **Shared — `verified`**：复用 positive-offset cursor；`mode/date` 作为 base query，不作为任意 continuation 值（`goal-3/upstream-contract-matrix.md:234,251-253`）。
- **CLI — `verified`**：`internal/cli/commands/pixiv/ranking/ranking.go:38-64,96` 支持 mode/date/page/limit 并调用 SDK。
- **MCP — `verified`**：`illust_ranking` handler/schema 位于 `internal/mcpserver/pixiv/tools/illust_ranking/illust_ranking.go:20-48,78`，注册于 `internal/mcpserver/pixiv/pixiv.go:99`。
- **Offline — `verified`**：endpoint、SDK、CLI、MCP tests 覆盖 invalid mode/date 和 continuation。（具体证据索引：本文件:92-96）
- **Live — `verified`（frozen case）**：`illust-ranking` 为 HTTP 200、30→30、`offset`，adapter/SDK/CLI/MCP confirmed（`goal-3/evidence/appapi-upstream.md:27`）。
- **Compatibility — `implemented_unverified`**：mode/date 已进入 query/cursor digest 设计，但 capability 状态仍未越过 `scope_admitted`。
- **Release — `rejected`**：当前只有 frozen case 证据，未完成 public readiness。

### 3.4 `artwork-recommended`（#4）

- **Contract — `implemented_unverified`**：首请求不带 offset；续页保留显式 `offset=0` 特例。contract 见 `goal-3/upstream-contract-matrix.md:34,61,235`。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/artwork/recommended/recommended_test.go:26-68` 覆盖 initial/continuation、zero offset、missing list 和 invalid request。
- **SDK — `implemented_unverified`**：`RecommendedArtworks` 位于 `sdk/pixiv/ops_artwork.go:101-131`，保留 continuation presence；严格 live 第二页仍失败。
- **Shared — `implemented_unverified`**：shared pagination 已接入，但 recommended subtype/filter 尚未进入 SDK request/digest（`goal-3/upstream-contract-matrix.md:252`）。
- **CLI — `implemented_unverified`**：recommended command 与 artwork runner 存在，但 live continuation 未闭合，不能视为 public-ready。
- **MCP — `implemented_unverified`**：`illust_recommended` 位于 `internal/mcpserver/pixiv/tools/illust_recommended/illust_recommended.go:17-50`，支持分页，但同样受 continuation/subtype gate 约束。
- **Offline — `verified`**：endpoint、SDK、CLI、MCP fixture 已通过相关离线验证。（具体证据索引：本文件:105-109）
- **Live — `implemented_unverified`**：首页为 85 items 并返回 continuation，历史第二页为 `second_page_error`；verdict `inconclusive`（`goal-3/evidence/appapi-upstream.md:25`；`goal-3/pagination-validation-report.md:24-31`）。不得静默重放、截断或把首页当完整分页成功。
- **Compatibility — `missing`**：CLI content-type/MCP `illust_filter.type` 尚未进入 `RecommendedArtworksRequest`/SDK digest（`goal-3/upstream-contract-matrix.md:252`）。
- **Release — `rejected`**：required acceptance 要求完整 continuation 与 subtype 两页（`goal-3/capability-admission.md:28`），当前未满足。

### 3.5 `artwork-series`（#5）

- **Contract — `implemented_unverified`**：`GET /v1/illust/series`，正 series ID，continuation=`last_order`，required `illust_series_detail.user.id` 和 `illusts`（`goal-3/upstream-contract-matrix.md:58,232`）。
- **Adapter — `verified`**：series route/query/continuation/required detail user 测试见 `internal/services/pixiv/endpoint/artwork/series/series_test.go:24-44`。
- **SDK — `implemented_unverified`**：`ArtworkSeries` 位于 `sdk/pixiv/ops_artwork.go:58-73`，使用 positive `last_order` cursor；SDK 两页 fixture 位于 `sdk/pixiv/pixiv_test.go:276-320`。
- **Shared — `verified`**：series 使用 typed `last_order` cursor，不由上层解析 raw URL；公共规则见 `goal-3/pagination-validation-report.md:66-68`。
- **CLI — `verified`**：`internal/cli/commands/pixiv/series/series.go:69-99,115-125` 支持 artwork series URL/type identity 与分页。
- **MCP — `verified`**：`illust_series` 位于 `internal/mcpserver/pixiv/tools/illust_series/illust_series.go:17-44`，注册于 `internal/mcpserver/pixiv/pixiv.go:102`。
- **Offline — `verified`**：endpoint、SDK、CLI、MCP 两页合成 fixture 存在并通过。（具体证据索引：本文件:118-122）
- **Live — `implemented_unverified`**：Goal-3 没有独立 artwork series live 第二页；分页报告已确认列表不包含该 capability（`goal-3/upstream-contract-matrix.md:232`；`goal-3/pagination-validation-report.md:5-12`）。
- **Compatibility — `implemented_unverified`**：URL/type resolver contract 已存在，但 live 第二页和 public gate 未完成。
- **Release — `rejected`**：T05 要求独立 series live 第二页，当前不能发布。

### 3.6 `ugoira-metadata`（#6）

- **Contract — `implemented_unverified`**：required metadata、无分页；acceptance 与分页规则见 `goal-3/capability-admission.md:29`、`goal-3/pagination-validation-report.md:5-12`。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/artwork/detail/detail.go:53-58` 调用 `/v1/ugoira/metadata` 并解析 required archive/frames。
- **SDK — `implemented_unverified`**：`Client.UgoiraMetadata` 位于 `sdk/pixiv/ops_artwork.go:278-288`；DTO/mapping 位于 `sdk/pixiv/dto.go:284-289,632-642`、`sdk/pixiv/map_artwork.go:134-169`；valid 与 unsafe filename 测试位于 `sdk/pixiv/pixiv_test.go:1200-1225`。
- **Shared — `verified`**：无分页；resource ref/variant 和 output-safe DTO 见 `sdk/pixiv/ugoira.go:34-41`、`sdk/pixiv/dto.go:272-289`。
- **CLI — `missing`**：当前 `internal/cli` 没有专用 `ugoira metadata` command 或调用；已有 ugoira CLI 语义是 search subtype/download，不是 metadata operation。
- **MCP — `missing`**：当前 `internal/mcpserver` 没有 `ugoira_metadata` tool 或注册；现有 ugoira 相关工具不能替代 metadata tool。
- **Offline — `verified`**：SDK mapping/error 与 public interface compile tests 存在并通过；CLI/MCP 缺失不能被这些测试掩盖。（具体证据索引：本文件:131-135）
- **Live — `implemented_unverified`**：旧 strict evidence 的 `ugoira-metadata` 行声称 HTTP 200、无 continuation、全链 confirmed，但该行与当前源码中缺少 CLI/MCP surface 矛盾；按当前代码不能直接接受旧行（`goal-3/evidence/appapi-upstream.md:33`；当前源码证据见上述 CLI/MCP）。
- **Compatibility — `implemented_unverified`**：SDK DTO/resource contract 已存在，但 CLI/MCP compatibility contract 缺失。
- **Release — `rejected`**：缺少 required CLI/MCP surface，且 capability 仍是 `scope_admitted`。

### 3.7 `novel-search`（#7）

- **Contract — `implemented_unverified`**：`/v1/search/novel`、word、默认 target/sort、duration、首请求无 offset、续页正 offset 见 `goal-3/upstream-contract-matrix.md:91`。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/search/search.go:39-81` 构造请求并解析 novels/next_url/offset；`search_test.go:24-57` 覆盖 query、continuation、null/missing/empty list。
- **SDK — `verified`**：`SearchNovels` 位于 `sdk/pixiv/ops_novel.go:19-64`，校验 target/sort/duration 并绑定 cursor；fixture 位于 `sdk/pixiv/pixiv_test.go:135-182,276-414`。
- **Shared — `verified`**：复用 novel typed cursor/query binding；但 local `novel_filter` 不得伪装成 upstream 字段。（具体证据索引：本文件:143-145）
- **CLI — `verified`**：`internal/cli/commands/pixiv/search/novel.go:26-35,94-105` 支持 canonical `pixiv search --type novel` 与 legacy `pixiv novel search`；测试见 `novel_test.go:56-59,110-156`。
- **MCP — `verified`**：`search_novel` 位于 `internal/mcpserver/pixiv/tools/search_novel/search_novel.go:16-86`，支持 request/filter/page/limit 并经 SDK 分页。
- **Offline — `verified`**：endpoint/SDK/CLI/MCP fixture 与 targeted tests 存在；当前 baseline 全量测试通过。（具体证据索引：本文件:143-148）
- **Live — `implemented_unverified`**：缺独立 strict live 两页及 period/date 字段证据；`goal-3/upstream-contract-matrix.md:91` 与 `goal-3/pagination-validation-report.md:57-64` 明确记录 gap。
- **Compatibility — `verified`（已有 legacy mapping，非 release acceptance）**：CLI migration matrix 要求保留 `pixiv novel search WORD` 并映射到 canonical route（`goal-3/cli-migration-matrix.md:51,111`）；但 period/date 尚未冻结。
- **Release — `missing`**：required acceptance 是 period/date 与两页，当前尚未证明。

### 3.8 `novel-detail`（#8）

- **Contract — `implemented_unverified`**：目标为 `/v2/novel/detail`；旧 `/v1/novel/detail` 已 rejected（`goal-3/upstream-contract-matrix.md:14-15`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/detail/detail.go:31-58` 使用 v2 protocol 并解析 novel/series metadata；测试覆盖 required/missing/null malformed（`detail_test.go:31-47`）。
- **SDK — `verified`（离线实现）**：`sdk/pixiv/ops_novel.go:66-76` 的 `Novel` 校验正 ID并调用 adapter；fixture 位于 `sdk/pixiv/pixiv_test.go:276-414`。
- **Shared — `verified`**：detail 无分页，复用统一 output/error model。（具体证据索引：本文件:156-158）
- **CLI — `verified`**：`internal/cli/commands/pixiv/detail/detail.go:122-152,184-196` 支持 `--type novel`；`--content` 返回显式 `ContentUnavailable`。
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/novel_detail/novel_detail.go:16-42` 注册 tool；schema/结果测试见对应 `novel_detail_test.go` 与 `pixiv_mcp_artwork_novel_read_test.go:38-39,99-104`。
- **Offline — `verified`**：v2 adapter/SDK/CLI/MCP fixtures 和错误边界存在。（具体证据索引：本文件:156-161）
- **Live — `implemented_unverified`**：`/v2/novel/detail` HTTP 200、wire/response confirmed，但 strict evidence 的 Adapter/SDK 为 `not_tested`（`goal-3/evidence/appapi-upstream.md:5-6`）。
- **Compatibility — `implemented_unverified`**：v1 rejection 与 content unavailable 兼容语义已存在，但 v2 full chain 尚未以当前 strict evidence 闭包。
- **Release — `missing`**：必须保持 v1 rejected、完成 v2 detail/series metadata 的完整 acceptance 后再发布。

### 3.9 `novel-series`（#9）

- **Contract — `implemented_unverified`**：目标为 `/v2/novel/series`、`last_order` continuation、required `novel_series_detail` 与 `novels`；旧 v1 因 required detail 缺失而 rejected（`goal-3/upstream-contract-matrix.md:16-17,93-105`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/series/series.go:36-70` 构造 v2 query、解析 last_order 和 required fields；测试见 `series_test.go:24-52`。
- **SDK — `verified`（离线实现）**：`sdk/pixiv/ops_novel.go:78-114` 校验 positive series ID、使用 last_order cursor 并保留 metadata；fixture 位于 `sdk/pixiv/pixiv_test.go:460-543`。
- **Shared — `verified`**：typed last_order cursor，不解析 raw URL。（具体证据索引：本文件:169-171）
- **CLI — `verified`**：`internal/cli/commands/pixiv/series/series.go:39-49,99-100,142-180` 支持 `--type novel`、URL reference、metadata 与分页；测试见 `series_test.go:73-109`。
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/novel_series/novel_series.go:16-62` 注册并返回 metadata/records/pagination。
- **Offline — `verified`**：v2 endpoint/SDK/CLI/MCP fixtures 已存在。（具体证据索引：本文件:169-174）
- **Live — `implemented_unverified`**：v2 首页 HTTP 200、29 records、request 使用 series_id/last_order，但第二页未观察，adapter/SDK 未测试（`goal-3/evidence/appapi-upstream.md:7-8`；`goal-3/pagination-validation-report.md:33-38`）。
- **Compatibility — `implemented_unverified`**：v1 rejection 与 v2 route 已分离，但第二页和 full chain 未确认。
- **Release — `missing`**：不能用 v1 response 或离线两页 fixture 替代 v2 live 两页 acceptance。

### 3.10 `novel-latest`（#10）

- **Contract — `implemented_unverified`**：`/v1/novel/new` 要求 `filter=for_android`，continuation 必须为 `max_novel_id`，禁止 offset（`goal-3/upstream-contract-matrix.md:20,94`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/timeline/timeline.go:47-85,108-117` 发送 max_novel_id；`timeline_test.go:101-165` 拒绝 offset、mixed keys，并接受 max_novel_id。
- **SDK — `verified`（当前 leaf 实现）**：`sdk/pixiv/ops_novel.go:202-213` 使用 max_novel_id；`ops_novel_test.go:195-280` 覆盖两次调用、无 offset 和旧 offset cursor 错误。
- **Shared — `verified`**：latest 使用专用 max_novel_id typed cursor，不把旧 offset 当 fallback。（具体证据索引：本文件:182-184）
- **CLI — `verified`**：`internal/cli/commands/pixiv/timeline/timeline.go:72,157` 支持 `timeline latest --type novel`。
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/timeline_novel_latest/timeline_novel_latest.go:16-63` 注册 tool，并支持 local novel filter。
- **Offline — `verified`**：timeline endpoint、SDK、CLI、MCP 迁移测试存在。（具体证据索引：本文件:182-187）
- **Live — `implemented_unverified`**：upstream 有 30→30、max_novel_id evidence，但 strict row 的 adapter 为 `not_tested`、SDK 为 `inconclusive`、verdict 为 `sdk_call_error`（`goal-3/evidence/appapi-upstream.md:11`）；旧分页报告也记录过 offset mismatch（`goal-3/pagination-validation-report.md:14-20`）。
- **Compatibility — `implemented_unverified`**：当前代码已拒绝旧 offset cursor，但尚需用与当前代码一致的 live adapter/SDK/CLI/MCP evidence 关闭迁移 gate。
- **Release — `missing`**：不能把 upstream HTTP 两页直接提升为完整 public acceptance。

### 3.11 `novel-recommended`（#11）

- **Contract — `implemented_unverified`**：首请求无 query；续页显式带 offset，包括合法 offset=0（`goal-3/upstream-contract-matrix.md:22,95`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/recommended/recommended.go:35-82` 实现 initial/continuation 区分；测试覆盖 initial offset 拒绝、负 offset、显式零 continuation 和 null list（`recommended_test.go:26-74`）。
- **SDK — `verified`**：`sdk/pixiv/ops_novel.go:169-180` 调用 adapter；`ops_novel_test.go:78-119` 覆盖显式零 offset 两页。
- **Shared — `verified`**：使用 shared pagination/cursor 语义，零 offset 仅在该 operation 的 continuation 语境有效。（具体证据索引：本文件:195-200）
- **CLI — `verified`**：recommended command 的 novel 分支调用 `RecommendedNovels`（`internal/cli/commands/pixiv/recommended/recommended.go:247,325`）；fixture 见 `recommended_test.go:146-149,229-277`。
- **MCP — `verified`**：recommended MCP 支持 `kind=novel/all`，经 `CollectPages` 调用 SDK（`internal/mcpserver/pixiv/tools/recommended/recommended.go:160-180`；行为测试见 `pixiv_mcp_feed_read_test.go:51-58,194-228`）。
- **Offline — `verified`**：endpoint/SDK/CLI/MCP 两页 fixture 与 targeted tests 已存在。（具体证据索引：本文件:195-200）
- **Live — `implemented_unverified`（G1-T28 纠正）**：Goal-3 frozen case 曾记录 33→33 offset confirmed（`goal-3/evidence/appapi-upstream.md:13`），但 G1-T28 当前 live 经本仓 adapter 首页即 `malformed_upstream_response`（多参数 next_url vs 单 offset allowlist）；按证据优先级以当前事实为准，由 `G1-CORR-G1-T28-RECOMMENDED-01` 闭合。
- **Compatibility — `implemented_unverified`**：仍需完成 T12/T18/T28/T37 兼容、文档和 release gates；confirmed 不等于 public-ready（`goal-3/upstream-contract-matrix.md:95-97`）。
- **Release — `missing`**：未通过统一 public readiness gate。

### 3.12 `novel-ranking`（#12）

- **Contract — `implemented_unverified`**：`/v1/novel/ranking` 的 filter/mode/positive offset contract 见 `goal-3/upstream-contract-matrix.md:23,97`。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/ranking/ranking.go:36-78` 与 `ranking_test.go:27-77` 覆盖 route/query/default/filter/mode/continuation。
- **SDK — `verified`（离线实现）**：`sdk/pixiv/ops_novel.go:116-137` 实现 `NovelRanking`；`sdk/pixiv/ops_novel_test.go:12-76` 覆盖 mode、两页 cursor 和 invalid mode no-network。
- **Shared — `verified`**：使用 positive offset cursor 和 ranking mode binding。（具体证据索引：本文件:208-210）
- **CLI — `verified`**：`internal/cli/commands/pixiv/ranking/ranking.go:38-86` 支持 `--type novel`；两页 typed route fixture 见 `ranking_test.go:27-129`。
- **MCP — `missing`**：当前没有专门 `novel_ranking` tool；现有 `illust_ranking` 只覆盖 artwork，不能充当 novel ranking MCP owner。
- **Offline — `verified`**：endpoint、SDK、CLI fixtures 已有；MCP 缺失是结构性缺口，不被离线测试掩盖。（具体证据索引：本文件:208-213）
- **Live — `implemented_unverified`**：upstream HTTP 200、30→30、offset，但 strict evidence 的 adapter/SDK 为 `not_tested`、production owner missing（`goal-3/evidence/appapi-upstream.md:14`；`goal-3/wire-adapter-sdk-diff.md:13-14`）。
- **Compatibility — `implemented_unverified`**：CLI/SDK typed route 存在，但缺 MCP 和完整 cross-layer proof。
- **Release — `missing`**：缺少 MCP owner/tool/schema/test，不能发布。

### 3.13 `novel-follow`（#13）

- **Contract — `implemented_unverified`**：`/v1/novel/follow` 支持 restrict public/private；首请求无 offset、续页正 offset，restrict 必须进入 binding（`goal-3/upstream-contract-matrix.md:21,96`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/timeline/timeline.go:94-105,224-244` 校验 restrict、构造 query、解析 offset；测试见 `timeline_test.go:28-88`。
- **SDK — `verified`**：`FollowingNovels` 位于 `sdk/pixiv/ops_novel.go:183-199`；`ops_novel_test.go:121-194` 覆盖两页、restrict 和 invalid value no-network。
- **Shared — `verified`**：shared cursor 绑定 restrict，不把 follow-user mutation 的状态混入 feed cursor。（具体证据索引：本文件:221-226）
- **CLI — `verified`**：`internal/cli/commands/pixiv/timeline/timeline.go:51,123` 支持 `timeline following --type novel`。
- **MCP — `verified`**：`timeline_novel_following` 位于 `internal/mcpserver/pixiv/tools/timeline_novel_following/timeline_novel_following.go:16-70`；认证态边界由 tool 注释和测试覆盖。
- **Offline — `verified`**：endpoint/SDK/CLI/MCP fixture 与 restrict error boundary 存在。（具体证据索引：本文件:221-226）
- **Live — `verified`（frozen case）**：HTTP 200、30→30、offset，wire/response/pagination/adapter/SDK confirmed（`goal-3/evidence/appapi-upstream.md:12`；`goal-3/pagination-validation-report.md:7-9`）。
- **Compatibility — `implemented_unverified`**：必须保持认证态、restrict/account binding 和 CLI/MCP compatibility；不能把 `follow add/remove` mutation 当作该 feed 的证明。
- **Release — `missing`**：统一 compatibility/docs/release gates 尚未完成。

## 4. Bookmark capability inventory（14–24）

> 本节对应 `G1-T03`。11 项 bookmark capability 全部保留 required；`scope_admitted` 只表示纳入范围。当前实现、离线 fixture 与旧 evidence 分开记账，不把离线实现或历史 task `verified` 提升为 `public_ready`。

### 4.1 Bookmark layer matrix 口径

- Required：11/11（14–24）；`public_ready`：0/11。
- Artwork/novel list、tags、detail、mutation 的 leaf surface 与 `list/tags --type all` aggregate surface 必须分别验收。
- `all` 是产品层双流聚合，不是 upstream subtype；不得把 aggregate cursor 继承到 detail/add/remove。
- `not_applicable` 只用于没有 shared orchestration 的 detail/mutation leaf；aggregate、pagination、typed semantics 仍需独立 owner。
- Goal-3 `capability-admission.md:37-47`、`upstream-contract-matrix.md:121-162` 是 requiredness 与 contract 边界；`api-migration-verification.md:30-34` 和 `mutation-validation-report.md:38-60` 保留未测试/未读回状态。

### 4.2 `artwork-bookmark-list`（#14）

- **Contract — `implemented_unverified`**：目标为 `GET /v1/user/bookmarks/illust`；`user_id` 为正数、`restrict` 为 `public|private`、`tag` 可选；首请求不带 `max_bookmark_id`，续页只接受正 `max_bookmark_id`。不发送未经验证的 `type/content_type`（`goal-3/upstream-contract-matrix.md:127`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/artwork/bookmark/bookmark.go:26-69` 要求 `illusts` 字段存在且为合法 list，拒绝非正 artwork ID，按 endpoint allowlist 提取 `max_bookmark_id`；错误页不返回 partial items。测试覆盖空页、缺失/null list、非法/重复/零 cursor 与 transport error（`bookmark_test.go:169-216,218-270`）。
- **SDK — `verified`**：`sdk/pixiv/ops_artwork.go:195-215` 校验用户 ID/restrict，绑定 query 与 `max_bookmark_id` cursor；request shape 在 `sdk/pixiv/request.go:178-184`，wire/continuation 测试在 `sdk/pixiv/pixiv_test.go:1301-1352`。
- **Shared — `verified`**：单流使用 `sdk.Page`/`sdk.Cursor`；aggregate 复用 shared stream 只在本轮执行内提供 checkpoint（`internal/shared/pagination/streams.go:9-38`）。
- **CLI — `verified`**：`internal/cli/commands/pixiv/bookmark/bookmark.go:330-482` 支持 artwork leaf 与 `all` 聚合，沿用 resolver、page plan、JSON/NDJSON/text 输出。
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/user_bookmarks/user_bookmarks.go:16-61` 暴露 `user_bookmarks`，支持 user/restrict/tag/page/limit 与 client-side artwork subtype filter，并复用 `runtime.CollectWith`；G1-T09 覆盖过滤先于 logical pagination。
- **Offline — `verified`**：adapter、SDK、CLI aggregate 测试覆盖 query、continuation、empty page、typed output、页失败原子性；不等于 live upstream 证明。（具体证据索引：本文件:246-251）
- **Live — `implemented_unverified`**：历史 public/private/partial evidence 存在，但 strict second-page 属 data-limited/pagination-exempt，未形成当前 Goal 的完整 live acceptance。
- **Compatibility — `implemented_unverified`**：既有 `user_bookmarks` wire 与 artwork SDK wrapper 需继续保持；aggregate cursor、subtype 与跨版本恢复尚未冻结。
- **Release — `missing`**：尚未通过 contract、compatibility、docs、live 与最终 release gate。

### 4.3 `artwork-bookmark-tags`（#15）

- **Contract — `implemented_unverified`**：目标为 `GET /v1/user/bookmark-tags/illust`；`user_id`、`restrict`、首请求无 `offset`，返回 required `bookmark_tags`，`name` 非空，空 list 合法；续页只接受正 `offset`。`type/content_type` 仍是未验证 candidate（`goal-3/upstream-contract-matrix.md:129`）。
- **Adapter — `verified`**：`bookmark.go:71-109` 解析 required tags、拒绝缺失/null list 或空 tag name，按 allowlist 读取 `offset`；测试 `bookmark_test.go:169-186`、相关 malformed/transport cases 覆盖失败边界。
- **SDK — `verified`**：`sdk/pixiv/ops_artwork.go:218-244` 提供 `UserArtworkBookmarkTags`，校验 restrict、绑定 offset cursor；request 在 `sdk/pixiv/request.go:186-192`，wire/empty-page test 在 `sdk/pixiv/pixiv_test.go:1353-1398`。
- **Shared — `verified`**：CLI single/aggregate tag stream 使用统一 cursor/checkpoint；aggregate 只在一次执行内保存 state。（具体证据索引：本文件:259-264）
- **CLI — `verified`**：`bookmark.go:647-733,764-852` 支持 artwork/novel/all tags，typed tag DTO 保留 `name/count/type`。
- **MCP — `verified`（typed artwork/novel leaf）**：legacy `bookmark_tags` 仍只调用 artwork tags；新增 `internal/mcpserver/pixiv/tools/novel_bookmark_tags/novel_bookmark_tags.go` 注册 `novel_bookmark_tags`，复用 `bookmark_tags` output envelope 与 closed schema；`all` aggregate 留给 G1-T10。
- **Offline — `verified`**：required-list、empty page、cursor/query、CLI output 与 aggregate failure 有 fixture。（具体证据索引：本文件:259-264）
- **Live — `implemented_unverified`**：strict tag wire/subtype/live continuation 尚未完成；pagination exemption 不等于 contract/public-ready。
- **Compatibility — `implemented_unverified`**：legacy artwork tag tool 保持；novel/all typed output 与 public cursor 仍未冻结。
- **Release — `missing`**：未通过完整 capability gate。

### 4.4 `artwork-bookmark-detail`（#16）

- **Contract — `implemented_unverified`**：目标为 `GET /v2/illust/bookmark/detail`，正 `illust_id`，无分页；未收藏归一为空 restrict 与 non-nil empty tags，404/null/明确 absent 只在此 endpoint 归一（`goal-3/upstream-contract-matrix.md:131`）。
- **Adapter — `verified`**：`bookmark.go:112-145` 转换该 endpoint 的 404/null/absent；`is_bookmarked=false` 即使携带 restrict/作品 tags 也归一为空收藏状态，其他错误原样传播。
- **SDK — `verified`**：`sdk/pixiv/ops_artwork.go:308-317` 提供 `ArtworkBookmark`；request 在 `sdk/pixiv/request.go:254-258`，typed model 在 `sdk/pixiv/models.go:258-263`。
- **Shared — `not_applicable`**：detail leaf 无分页/聚合 orchestration；仍需公共 DTO/error gate。
- **CLI — `verified`**：`bookmark.go:136-207` 支持 artwork/novel namespace，artwork detail 输出 text/JSON。
- **MCP — `verified`（typed artwork/novel leaf）**：legacy `bookmark_detail` 保持 positive `illust_id` 与原 envelope；新增 `internal/mcpserver/pixiv/tools/novel_bookmark_detail/novel_bookmark_detail.go` 接受 positive `novel_id`，复用 `{bookmarked,restrict,tags}` envelope 并保留 absent state。
- **Offline — `verified`**：detail absent/bookmarked/malformed/transport cases 有 endpoint/SDK/CLI/MCP coverage。（具体证据索引：本文件:272-277）
- **Live — `verified`**：`TestRealPixivSDKLiveManifestBookmarkUserRead` 实跑已收藏与未收藏 artwork detail；未收藏响应携带作品 tags 时归一为 `bookmarked=false`、non-nil empty tags。
- **Compatibility — `implemented_unverified`**：artwork detail envelope 保持；novel detail 尚无同一 MCP contract，aggregate 不适用。
- **Release — `missing`**：未通过完整 gate。

### 4.5 `artwork-bookmark-mutation`（#17）

- **Contract — `implemented_unverified`**：add/delete endpoint、正 ID、restrict/tag form 已有目标描述；正式 acceptance 还要求写前 access control、写后 read-back、只清理本轮副作用、uncertain 不 replay（`goal-3/upstream-contract-matrix.md:133-134`；`mutation-validation-report.md:38-60`）。
- **Adapter — `verified`（transport）**：`bookmark.go:147-180` 校验 ID/restrict、构造 `tags[]`、调用 add/delete；2xx 只证明 status-only transport。
- **SDK — `verified`（transport）**：`sdk/pixiv/ops_mutation.go:11-68` 提供 explicit artwork operation 与 legacy wrapper；无 read-back。
- **Shared — `not_applicable`**：当前没有 bookmark mutation shared orchestration；不得把 `runtime.Write` 当 read-back。
- **CLI — `verified`（artwork only）**：`bookmark.go:245-321` add/remove 仅调用 artwork SDK，未提供 novel mutation。
- **MCP — `verified`（artwork only）**：`add_bookmark` / `remove_bookmark` schema 只接受 `illust_id`（`internal/mcpserver/pixiv/tools/add_bookmark/add_bookmark.go:14-33`、`remove_bookmark.go:14-31`）。
- **Offline — `verified`（wire/validation only）**：form/path、invalid input no-network、transport error 有测试；不证明收藏状态变化。（具体证据索引：本文件:285-290）
- **Live — `implemented_unverified`**：当前 mutation report 明确 production mutation 尚未按当前 Goal 执行真实 round-trip。
- **Compatibility — `implemented_unverified`**：legacy artwork wrapper 仍需保持；novel mutation 已形成 additive public SDK/MCP API、schema 与 wire，但完整兼容矩阵仍未冻结。
- **Release — `missing`**：无 live read-back/cleanup/uncertain evidence，不得发布为 mutation success。

### 4.6 `novel-bookmark-list`（#18）

- **Contract — `implemented_unverified`**：目标为 `GET /v1/user/bookmarks/novel`，public/private、tag、正 `max_bookmark_id` continuation、required `novels` list（`goal-3/upstream-contract-matrix.md:128`）。
- **Adapter — `implemented_unverified`**：`internal/services/pixiv/endpoint/user/novelbookmarks/novelbookmarks.go:23-80` 已实现 positive novel/nested user ID、required list 与 continuation；但旧 strict table 仍为 `not_tested`，故不提升为 frozen.
- **SDK — `implemented_unverified`**：`sdk/pixiv/ops_novel.go:232-255` 与 `request.go:352-358` 提供 `UserNovelBookmarks`/`max_bookmark_id`；offline wire test 在 `sdk/pixiv/pixiv_test.go:1399-1444`。
- **Shared — `verified`**：单流 cursor 与 aggregate stream 可复用；aggregate checkpoint 仍为执行内 state。（具体证据索引：本文件:298-303）
- **CLI — `verified`**：`bookmark.go:330-337,535-587` 支持 novel bookmark list 与 `all` resolver。
- **MCP — `verified`（read only）**：`user_novel_bookmarks.go:16-53` 复用 `CollectWith`，支持 restrict/tag/page/limit。
- **Offline — `verified`**：endpoint malformed/empty/continuation、SDK query/cursor、CLI/MCP read fixture 已有。（具体证据索引：本文件:298-303）
- **Live — `implemented_unverified`**：legacy partial HTTP evidence 结论为 inconclusive；pagination exemption 不等于 strict live pass（`goal-3/evidence/appapi-read.json:1000-1012`）。
- **Compatibility — `implemented_unverified`**：已有 `user_novel_bookmarks` read wire/docs；aggregate cross-type cursor 与完整 public gate 未闭合。
- **Release — `missing`**：仍为 `scope_admitted`，不可发布。

### 4.7 `novel-bookmark-tags`（#19）

- **Contract — `implemented_unverified`**：candidate `GET /v1/user/bookmark-tags/novel`；T03 不擅自补造 continuation/defaults；必须保留 `name/count` 与 novel kind（`upstream-contract-matrix.md:130,325`）。
- **Adapter — `implemented_unverified`**：`novelbookmarks.go:83-120` 要求 tags list/name，明确拒绝非 null `next_url`，避免丢页；当前 snapshot 尚无 continuation contract。
- **SDK — `implemented_unverified`**：`sdk/pixiv/ops_novel.go:257-280` 支持零 cursor read，非零 cursor 显式 `InvalidCursor`；request 注释在 `sdk/pixiv/request.go:194-202`。
- **Shared — `implemented_unverified`**：CLI single novel tags 可读；aggregate stream 可运行，但 novel tag continuation/aggregate cursor 尚未冻结。
- **CLI — `verified`**：`bookmark.go:647-733` 接受 `--type novel`，并通过 SDK candidate operation。
- **MCP — `verified`（candidate/offline）**：`internal/mcpserver/pixiv/tools/novel_bookmark_tags/novel_bookmark_tags.go` 提供 novel-specific tags read，使用标准 `runtime.CollectWith`；candidate 没有已冻结 continuation，非零 cursor 由 SDK 返回 typed `InvalidCursor`，不猜测或静默重启。
- **Offline — `verified`（candidate）**：endpoint/SDK/CLI tests 覆盖 query、empty、malformed、unsupported continuation；candidate 不代替 strict live。（具体证据索引：本文件:312-315）
- **Live — `implemented_unverified`**：`goal-3/upstream-contract-matrix.md:325` 与 `api-migration-verification.md:30-33` 均记 `not_tested`。
- **Compatibility — `implemented_unverified`**：novel-specific MCP schema/fixture 已补齐且为 additive；candidate 的 public/live wire、continuation 与完整兼容矩阵仍未冻结。
- **Release — `missing`**：未达 public-ready。

### 4.8 `novel-bookmark-detail`（#20）

- **Contract — `implemented_unverified`**：candidate `GET /v2/novel/bookmark/detail`，正 `novel_id`，无分页；absent/404 normalization、tags shape、错误映射需 T08 snapshot（`upstream-contract-matrix.md:132,326`）。
- **Adapter — `implemented_unverified`**：`novelbookmarks.go:123-158` 已有 candidate normalized absent/bookmarked 逻辑；`is_bookmarked=false` 携带 restrict/作品 tags 时已统一归一为空状态，但 candidate 的完整 strict contract 仍未闭合。
- **SDK — `implemented_unverified`**：`sdk/pixiv/ops_novel.go:283-295` 与 `request.go:260-264` 提供 candidate `NovelBookmark`；model 为 `NovelBookmarkDetail`（`sdk/pixiv/models.go:265-270`）。
- **Shared — `not_applicable`**：detail leaf 无分页；公共 error/DTO/compat 仍未闭合。
- **CLI — `verified`**：`bookmark.go:136-207,169-188` 已按 resolver 分发 novel detail，并输出 novel DTO。
- **MCP — `verified`（candidate/offline）**：`internal/mcpserver/pixiv/tools/novel_bookmark_detail/novel_bookmark_detail.go` 提供 positive `novel_id` detail read，使用同一 `BookmarkDetail` envelope；未收藏/absent 仍输出 `bookmarked=false`、空 tags。
- **Offline — `verified`（candidate）**：adapter absent/404/malformed、SDK invalid input/DTO copy 有测试（`novelbookmarks_test.go:228-264`、`sdk/pixiv/pixiv_test.go:114-133,135-182`）。
- **Live — `blocked_external`**：未收藏 absent case 已由 `TestRealPixivSDKLiveManifestBookmarkUserRead` 实跑通过；当前认证账号没有 novel bookmark，无法安全取得 bookmarked=true 目标，按 manifest 记为 `blocked_external (data)`。
- **Compatibility — `implemented_unverified`**：CLI/docs 记录 novel detail，但 MCP legacy contract 仍 artwork-only；未形成 novel public wire。
- **Release — `missing`**：未达 public-ready。

### 4.9 `novel-bookmark-mutation`（#21）

- **Contract — `implemented_unverified`**：candidate add/delete paths 为 `/v2/novel/bookmark/add` 与 `/v1/novel/bookmark/delete`；正式 contract 仍要求 list/tags/detail read-back、删除后恢复与 uncertain 分类（`upstream-contract-matrix.md:133-134,327-328`）。
- **Adapter — `implemented_unverified`**：`novelbookmarks.go:160-198` 提供窄 transport leaf；2xx 只证明 status-only transport，remove 后 read-back/restore 留给后续验证。
- **SDK — `verified`（offline transport）**：`sdk/pixiv/ops_mutation.go:71-99` 暴露 typed `AddNovelBookmark`/`RemoveNovelBookmark`，`request.go:442-455` 暴露 request types；校验正数 ID、`public|private` restrict，并将 empty restrict 归一为 `public`。
- **Shared — `not_applicable`**：没有 novel mutation orchestration；MCP 只通过 public SDK 的窄 port 执行。
- **CLI — `verified`（G1-T21 后）**：`bookmark add/remove` 支持 `--type artwork|novel`（默认 artwork 保持旧行为），novel dispatch 走 public SDK `AddNovelBookmark`/`RemoveNovelBookmark`；`--type all/user` 网络前拒绝；record 消费按 namespace 分离（`bookmark.go` `bookmarkMutationType`/`novelRecordTypes`，tests `TestBookmarkAddSupportsNovelType` 等）。
- **MCP — `verified`（offline）**：新增 `internal/mcpserver/pixiv/tools/add_novel_bookmark` 与 `remove_novel_bookmark`，注册 exact tool/schema，输出 `{success, action, novel_id, text}`，错误为 `isError=true` 的 structured result。
- **Offline — `verified`**：SDK/MCP tests 覆盖 path/form、private/public/default、tags、invalid pre-network、typed success/failure、schema、502 uncertain single-request、legacy artwork wire；不证明状态 round-trip。
- **Live — `missing`**：mutation report 明确 novel add/delete、list/tags/detail read-back、restore 尚未进入 strict mutation manifest（`goal-3/mutation-validation-report.md:45-60`）。
- **Compatibility — `implemented_unverified`**：既有 artwork mutation wire 保持；novel mutation 为 additive SDK/MCP surface，完整 public compatibility/release 仍待后续 gate。
- **Release — `missing`**：必须先完成授权隔离账号、写入、read-back、cleanup、uncertain handling。

### 4.10 `bookmark-subtype`（#22）

- **Contract — `implemented_unverified`**：artwork bookmark upstream `type/content_type` 仍 candidate；`all` 是选择器，不是 subtype（`goal-3/upstream-contract-matrix.md:123-134,246-247`）。
- **Adapter — `missing`**：当前 artwork bookmark adapter 只读取 `illusts`/`id`/内容 DTO，没有已确认可发送的 subtype query binding（`bookmark.go:26-69`）。
- **SDK — `missing`**：`UserArtworkBookmarksRequest` 只有 user/restrict/tag/cursor（`sdk/pixiv/request.go:178-184`），没有冻结的 subtype field。
- **Shared — `implemented_unverified`**：resolver/typed stream 能区分 artwork、novel、all，但不能替代 upstream subtype contract。
- **CLI — `verified`（product selector only）**：`bookmark list/tags --type artwork|novel|all` 与 typed output 已有离线 coverage；`all` 不向 upstream 当 subtype 发送。（具体证据索引：本文件:350-353,389-390）
- **MCP — `missing`**：现有 `user_bookmarks`/`bookmark_tags` 没有 subtype/all input 或 typed aggregate tool。
- **Offline — `verified`（local semantics）**：aggregate order、type field、同名 tag count 分离和 invalid selector 有 tests。（具体证据索引：本文件:350-354,389-390）
- **Live — `missing`**：无 server-side subtype/content_type confirmed evidence。
- **Compatibility — `implemented_unverified`**：既有 single-type tools/CLI 保持；新增 subtype wire/schema 尚未发布。
- **Release — `missing`**：未闭合。

### 4.11 `bookmark-list-all`（#23）

- **Contract — `implemented_unverified`**：artwork stream 先于 novel stream；统一 Skip/Limit/OneBatch；任一 stream 失败整页失败；aggregate cursor 不得携带 raw next URL/token/cookie/query/content（`goal-3/upstream-contract-matrix.md:137,155,245-247`）。
- **Adapter — `verified`（leaf reuse）**：当前 CLI 复用 artwork/novel leaf adapter/SDK page；没有独立 aggregate adapter。（具体证据索引：本文件:247-251,299-303）
- **SDK — `missing`**：没有 public `BookmarkListAll` operation，也没有可跨调用恢复的 aggregate cursor API。
- **Shared — `verified`（offline algorithm）**：`internal/shared/pagination/streams.go:9-38,65-116` 与 `internal/shared/traversal/streams.go:28-53` 覆盖统一 budget/checkpoint/失败丢弃。
- **CLI — `verified`（offline）**：`bookmark.go:343-430,773-808` 建立双流并 staging 输出；`bookmark_test.go:60-190,425-476` 覆盖顺序、limit、typed output、页原子失败。
- **MCP — `verified`（additive aggregate）**：新增 `bookmark_list_all`，使用 closed input、shared stream collector 与 `Records` envelope；旧 `user_bookmarks` / `user_novel_bookmarks` registration 与 wire 保持不变（`internal/mcpserver/pixiv/pixiv.go`、`internal/mcpserver/pixiv/pixiv_bookmark_aggregate_test.go`）。
- **Offline — `verified`**：fixture 能证明 artwork → novel、跨逻辑页的统一 `Skip`/`Limit` 不漏不重、失败重放不泄漏 partial，以及 shared checkpoint seam；MCP 不暴露 aggregate cursor。（具体证据索引：`internal/mcpserver/pixiv/pixiv_bookmark_aggregate_test.go`、`internal/shared/pagination/streams.go`、`internal/shared/traversal/streams.go`）
- **Live — `implemented_unverified`**：无 aggregate live second-page/aggregate cursor evidence。
- **Compatibility — `implemented_unverified`**：新 MCP operation 仅 additive，exact registration 保留旧 tool，聚合 error/empty envelope 与 legacy wire 回归通过；public aggregate SDK、strict live 与完整 compatibility/release gate 仍未闭合。
- **Release — `missing`**：未达 public-ready。

### 4.12 `bookmark-tags-all`（#24）

- **Contract — `implemented_unverified`**：artwork tags stream 先于 novel tags stream；typed `type` 与原始 `count` 保留，同名 tag 不合并；统一 budget、checkpoint 与页原子失败规则同 list all。
- **Adapter — `verified`（leaf reuse）**：复用 artwork/novel tag adapters；无独立 aggregate adapter。（具体证据索引：本文件:260-264,315-317）
- **SDK — `missing`**：没有 public aggregate tags operation 或跨调用 aggregate cursor。
- **Shared — `verified`（offline algorithm）**：复用 `CollectStreams` 与 `bookmarkStreamCursor`。（具体证据索引：本文件:366,390）
- **CLI — `verified`（offline）**：`bookmark.go:647-808,811-856` 实现 artwork → novel typed tag output；`bookmark_test.go:124-190,478-529` 覆盖同名 tag、count/type 保留和 failure atomicity。
- **MCP — `verified`（additive typed aggregate）**：新增 `bookmark_tags_all`，输出 closed typed `{name,count,content_type}` item；旧 `bookmark_tags` / `novel_bookmark_tags` output contract 不变。
- **Offline — `verified`**：CLI 与 MCP 均有 artwork → novel、同名 tag 分离、原始 count/type 与 required-stream failure atomicity evidence；novel candidate continuation 仍不猜测。
- **Live — `implemented_unverified`**：novel tags candidate 与 subtype/continuation strict evidence 缺失；无 aggregate live evidence。
- **Compatibility — `implemented_unverified`**：MCP aggregate schema、exact registration、typed same-name counts 与 structured failure fixture 已有；public aggregate SDK、strict candidate/live compatibility 与 release gate 仍未闭合。
- **Release — `missing`**：未达 public-ready。

### 4.13 Bookmark cross-cutting verdict

- **Typed semantics**：CLI 以 `bookmarkListItem` / `bookmarkTagItem` 保留 artwork/novel kind；`--type all` 固定 artwork 后 novel；同名 tag 不合并，保留各自 count/type。MCP `bookmark_tags_all` 进一步以 `content_type=artwork|novel` 固定来源（`bookmark.go:54-69,773-856`；`bookmark_test.go:60-190`；`internal/mcpserver/pixiv/pixiv_bookmark_aggregate_test.go`）。这是当前离线产品层证据，不是 upstream subtype evidence。
- **Dual-stream checkpoint**：CLI `bookmarkStreamCursor` 与 MCP aggregate-local `bookmarkCursor` 都保存上游输入 cursor 与批内 `consumed`；shared collector 在一条 logical page 中统一执行预算，任一 stream 失败丢弃 partial，traversal 在安全重放时清空失败 attempt 结果。MCP 与 CLI 都不暴露跨调用 public aggregate cursor（`bookmark.go:71-88,433-482,811-852`；`internal/mcpserver/pixiv/tools/bookmark_list_all/bookmark_list_all.go`；`internal/mcpserver/pixiv/tools/bookmark_tags_all/bookmark_tags_all.go`；`internal/shared/pagination/streams.go:9-38,65-116`）。
- **Unified budget**：CLI 与 MCP aggregate 都复用 `CollectStreams` 连接后的单一 Skip/Limit/OneBatch，不按 artwork/novel 分配预算；MCP focused tests 覆盖 limit=1 的 artwork→novel page transition 与无重复恢复。不能据此证明 public cursor 恢复。
- **Page atomicity**：CLI list/tags 先写 staging buffer，所有 stream/serialization 成功后才提交 stdout；MCP aggregate 在 artwork 成功而 novel 失败时返回 `isError=true` 与空 structured records/tags；共享 traversal 也清空失败 attempt 的 partial result。
- **Mutation boundary**：artwork mutation 当前只有 transport success；novel mutation 只有 candidate adapter transport。两者均无当前 Goal 要求的 live read-back、restore、cleanup、uncertain-no-replay evidence。
- **Requiredness/evidence boundary**：11 项在 `capability-admission.md:37-47` 仍为 `scope_admitted`；T15/T19/T23 历史 `verified` 只能证明局部 seam，不能提升 bookmark capability acceptance（`goal-3/tasks.md:649-656,691-733`）。

## 5. G1-T04 inventory：comments/stamps + user/shared（25–41）

本节沿用上文状态口径。`verified` 只表示当前层有可追溯源码/离线测试/evidence；`implemented_unverified` 不等于 `public_ready`；`missing` 与 `rejected` 保持为后续 owner/gate 的明确缺口。

### 5.1 `artwork-comments-read`（#25）

- **Layer verdict：** Contract=`rejected`；Adapter/SDK/Shared/CLI/MCP=`implemented_unverified`；Offline=`verified`；Live=`rejected`；Compatibility=`implemented_unverified`；Release=`rejected`。
- **证据：** 当前 artwork comments 仍走 `/v3/illust/comments`（`internal/services/pixiv/endpoint/artwork/comments/comments.go:60-103`、`sdk/pixiv/ops_artwork.go:292-305`、`internal/cli/commands/pixiv/comment/comment.go:45-64`、`internal/mcpserver/pixiv/tools/illust_comments/illust_comments.go:13-26`），离线 fixture/MCP envelope 可核验（`sdk/pixiv/pixiv_test.go:578-618`、`internal/cli/commands/pixiv/comment/comment_test.go:245-299`、`internal/mcpserver/pixiv/pixiv_mcp_comments_read_test.go:15-132`）。
- **边界：** Goal-3 将该 v3 contract 标为 rejected，要求补齐 `date`、numeric access-control、第二页和非空 live DTO 后才可重新评估（`goal-3/upstream-contract-matrix.md:41`；`goal-3/api-migration-verification.md:26-28`）。不得 fallback 到未经批准的 endpoint。

### 5.2 `artwork-comments-mutation`（#26）

- **Layer verdict：** Contract=`implemented_unverified`；Adapter/SDK/Shared/CLI/Offline=`verified`；MCP=`missing`；Live/Compatibility=`implemented_unverified`；Release=`missing`。
- **证据：** create/reply/stamp/delete adapter 与测试位于 `internal/services/pixiv/endpoint/artwork/comments/comments.go:106-160`、同目录 `comments_test.go:185-296`；SDK/CLI 入口分别位于 `sdk/pixiv/ops_comment.go:11-67`、`sdk/pixiv/ops_stamps.go:30-50`、`internal/cli/commands/pixiv/comment/comment.go:63-123,269-315`，对应离线测试通过。
- **边界：** MCP registry 没有 artwork comment mutation tool（`internal/mcpserver/pixiv/pixiv.go:90-130`）；历史写入证据不等于当前公开 SDK/CLI/MCP 的同账号 read-back/cleanup（`goal-3/mutation-validation-report.md:23-42`；`goal-3/evidence/appapi-upstream.md:35-40`）。

### 5.3 `novel-comments-read`（#27）

- **Layer verdict：** Contract/Live/Compatibility=`implemented_unverified`；Adapter/SDK/Shared/CLI/MCP/Offline=`verified`；Release=`missing`。
- **证据：** novel comments v2 adapter、SDK、MCP read tool 与输出分别见 `internal/services/pixiv/endpoint/novel/comments/comments.go:60-108`、`sdk/pixiv/ops_novel.go:153-166`、`internal/mcpserver/pixiv/tools/novel_comments/novel_comments.go:13-26`、`internal/mcpserver/pixiv/internal/outputs/outputs.go:260-305`；CLI/MCP fixture 见 `internal/cli/commands/pixiv/comment/comment_test.go:245-299`、`internal/mcpserver/pixiv/pixiv_mcp_comments_read_test.go:22-116`。
- **边界：** current production path 无 v3 fallback，但 live 仍缺第二页/完整 strict 回放，不能由离线分页实现升级为 public-ready（`goal-3/api-migration-verification.md:25-27`；`goal-3/wire-adapter-sdk-diff.md:17-19`）。

### 5.4 `novel-comments-mutation`（#28）

- **Layer verdict：** Contract=`implemented_unverified`；Adapter/SDK/Shared/CLI/Offline=`verified`；MCP=`verified`；Live/Compatibility=`implemented_unverified`；Release=`missing`。
- **证据：** novel create/reply/stamp/delete adapter 见 `internal/services/pixiv/endpoint/novel/comments/comments.go:111-160`；SDK request/operation 见 `sdk/pixiv/ops_comment.go:70-126`、`sdk/pixiv/ops_stamps.go:53-74`、`sdk/pixiv/request.go:299-323`；CLI 路由和输入约束见 `internal/cli/commands/pixiv/comment/comment.go:150-315`、`comment_test.go:301-339`；novel-side MCP tools、wire fixture、schema/outcome tests 见 `internal/mcpserver/pixiv/tools/{create_novel_comment,reply_novel_comment,stamp_novel_comment,delete_novel_comment}`、`pixiv_mcp_novel_comment_mutation_test.go` 与 `pixiv_sdk_wire_test.go`。
- **边界：** MCP mutation 只经 public `sdk/pixiv`，create/reply/stamp 取 v2 写入 response 的 `comment_id`，delete 只使用调用方提供的 ID；不读取最新评论猜 ID，不回退 candidate v3，不自动 replay。strict live、access-control、同账号 read-back/cleanup 与 public acceptance 仍由后续 task 负责。

### 5.5 `stamps`（#29）

- **Layer verdict：** Contract=`implemented_unverified`；Adapter/SDK/Shared/CLI/Offline=`verified`；MCP=`verified`；Live/Compatibility=`implemented_unverified`；Release=`missing`。
- **证据：** `/v1/stamps` adapter 与验证见 `internal/services/pixiv/endpoint/stamps/stamps.go`、`internal/services/pixiv/endpoint/stamps/stamps_test.go:26-113`；SDK resource/mutation 见 `sdk/pixiv/ops_stamps.go:11-74`、`ops_stamps_test.go:153-222`；CLI surface 见 `internal/cli/commands/pixiv/comment/comment.go:125-145,317-361`。；artwork/novel comment stamp 的 MCP registration、public SDK 调用、wire response ID 与 schema/no-replay tests 已分别由 G1-T14/G1-T15 覆盖。
- **边界：** legacy MCP registry 明确不新增 standalone `stamps` tool（`internal/mcpserver/pixiv/pixiv_mcp_comments_read_test.go:119-132`；`docs/en/mcp-tools.md:37-38`）。MCP stamp mutation 只在 artwork/novel comment add 上暴露；strict live、access-control、同账号 read-back/cleanup 与 release 仍未关闭。

### 5.6 `user-artworks`（#30）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Release=`rejected`。
- **证据：** adapter 使用 `UserArtworks` timeline kind、正数 user ID、type 与 offset continuation（`internal/services/pixiv/endpoint/artwork/timeline/timeline.go:33-85,117-137`）；SDK 入口/LSP symbol 位于 `sdk/pixiv/ops_artwork.go:175-194`，MCP/CLI 与测试见 `internal/mcpserver/pixiv/tools/user_artworks/user_artworks.go:17-63`、`internal/cli/commands/pixiv/user/user.go:266-280`、`internal/cli/commands/pixiv/user/user_test.go:175-182`。
- **边界：** 历史 live 只有首请求，`second_page_not_observed`（`goal-3/evidence/appapi-upstream.md:28-29`）；不允许把同类 SDK continuation fixture（`sdk/pixiv/ops_user_test.go:12-77`）当作 live acceptance。

### 5.7 `user-novels`（#31）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Release=`rejected`。
- **证据：** adapter 固定 `filter=for_android`、正数 user ID 与 offset continuation（`internal/services/pixiv/endpoint/user/novels/novels.go:17-76,199-205`）；SDK/CLI/MCP 分别见 `sdk/pixiv/ops_novel.go:216`、`internal/cli/commands/pixiv/user/user.go:318-331`、`internal/mcpserver/pixiv/tools/user_novels/user_novels.go:17-60`，测试见 `internal/cli/commands/pixiv/user/user_test.go:184-189`。
- **边界：** historical row 仍 `second_page_not_observed`（`goal-3/evidence/appapi-upstream.md:15`）；兼容 fixture 不替代 live continuation。

### 5.8 `user-relationships`（#32）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Release=`rejected`。
- **证据：** following/followers/related/blocked adapter 分别位于 `internal/services/pixiv/endpoint/user/following`、`internal/services/pixiv/endpoint/user/followers`、`internal/services/pixiv/endpoint/user/related`、`internal/services/pixiv/endpoint/user/blocked`；SDK operations 见 `sdk/pixiv/ops_user.go:70-145`；MCP read tools 与 fixture 见 `internal/mcpserver/pixiv/tools/user_following`、`internal/mcpserver/pixiv/tools/user_followers`、`internal/mcpserver/pixiv/tools/related_users`、`internal/mcpserver/pixiv/tools/blocked_users`、`internal/mcpserver/pixiv/pixiv_mcp_user_read_test.go:198-231,378-385`。
- **边界：** 当前没有 strict live rows，关系列表的离线 continuation 不能替代 authenticated live evidence（`goal-3/upstream-contract-matrix.md:287,314`）。

### 5.9 `user-detail`（#33）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Release=`rejected`。
- **证据：** detail adapter 严格校验 user/profile/profile_publicity/workspace（`internal/services/pixiv/endpoint/user/detail/detail.go:23-42`）；SDK/CLI/MCP 入口与测试见 `sdk/pixiv/ops_user.go:35-45`、`internal/cli/commands/pixiv/user/user.go:250-263`、`internal/mcpserver/pixiv/tools/user_detail/user_detail.go:17-48`、`internal/cli/commands/pixiv/user/user_test.go:168-174`。
- **边界：** Goal-3 未登记该 capability 的 strict live row；不能把 DTO fixture 当 current-account live proof（`goal-3/upstream-contract-matrix.md:283,314`）。

### 5.10 `user-search`（#34）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Release=`rejected`。
- **证据：** search adapter 负责 word、offset 和 required users list（`internal/services/pixiv/endpoint/user/search/search.go:20-55`）；SDK/CLI/MCP 入口见 `sdk/pixiv/ops_user.go:19-33`、`internal/cli/commands/pixiv/user/user.go:236-247`、`internal/mcpserver/pixiv/tools/search_user/search_user.go:17-50`；SDK query/cursor test 见 `sdk/pixiv/pixiv_test.go:219-265`。
- **边界：** 无 strict live row，且 account binding/anonymous fallback gate 未闭合（`goal-3/upstream-contract-matrix.md:282,314`）。

### 5.11 `trending`（#35）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/MCP/CLI/Offline=`verified`（G1-T21 纠正：CLI surface 为 `pixiv search --trending-tags`，frozen row 110）；Release=`rejected`。
- **证据：** SDK artwork trending operation 位于 `sdk/pixiv/ops_artwork.go:262-279`；MCP owner/handler 与 tests 位于 `internal/mcpserver/pixiv/tools/trending_tags_illust/trending_tags_illust.go:17-55`、`internal/mcpserver/pixiv/pixiv_mcp_feed_read_test.go:34-62`；CLI user owner 中没有 trending command（`internal/cli/commands/pixiv/user/user.go:187-370`）。
- **边界：** `goal-3/upstream-contract-matrix.md:288,314` 无 strict live row；CLI owner 为 `search --trending-tags`（`internal/cli/commands/pixiv/search/search.go`，含 `--type/--content-type` 冲突拒绝与 wire tests），G1-T07 时代记录的 user-owner 缺失结论已被当前源码纠正。

### 5.12 `follow-mutation`（#36）

- **Layer verdict：** Contract/SDK/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Live=`missing`；Release=`rejected`。
- **证据：** follow adapter 与测试在 `internal/services/pixiv/endpoint/user/follow/follow.go`、`follow_test.go`；SDK `FollowUser`/`UnfollowUser` 在 `sdk/pixiv/ops_mutation.go:70-95`；CLI/MCP owners 与 tests 见 `internal/cli/commands/pixiv/follow/follow.go:29-107`、`internal/mcpserver/pixiv/tools/{follow_user,unfollow_user}`、`internal/mcpserver/pixiv/pixiv_sdk_wire_test.go:323-334`。
- **边界：** 当前没有同账号 read-back/cleanup 的 strict live evidence；2xx 只说明请求被接受，不能证明关系状态变化（`goal-3/mutation-validation-report.md:28-45`；`goal-3/upstream-contract-matrix.md:298`）。

### 5.13 `mypixiv`（#37）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Release=`rejected`。
- **证据：** user MyPixiv adapter 在 `internal/services/pixiv/endpoint/user/mypixiv/mypixiv.go:30-115`；SDK artwork/novel/user operations 见 `sdk/pixiv/ops_artwork.go:248-260`、`sdk/pixiv/ops_novel.go:298-309`、`sdk/pixiv/ops_user.go:148-169`；CLI/MCP owners 与 tests 见 `internal/cli/commands/pixiv/mypixiv/mypixiv.go:34-68,133-214`、`internal/mcpserver/pixiv/tools/{mypixiv_users,mypixiv_illusts,mypixiv_novels}`、`internal/mcpserver/pixiv/pixiv_mcp_user_read_test.go:174-197,292-302`。
- **边界：** contract 要求 verified current identity、禁止外部 UID/跨账号 cursor；当前 G1-T08 已验证 MCP/Shared offline acceptance，但 strict live 第二页仍未登记（`goal-3/upstream-contract-matrix.md:289-290,314`），因此不提升 live/compatibility/release。

### 5.14 `bare-id-probe`（#38）

- **Layer verdict：** Contract=`implemented_unverified`；Adapter/SDK/MCP/Live=`missing`；Shared/Offline=`verified`；CLI=`implemented_unverified`；Compatibility/Release=`rejected`。
- **证据：** shared resolver 定义 `BareIDPolicy`、probe status 与多命中/403/404/network 分类（`internal/shared/resolver/resolver.go:100-118,235-294`；LSP `BareIDPolicy` symbol 同样定位于 `resolver.go:103`），测试覆盖 typed requirement、probe 分类与不 fallback（`internal/shared/resolver/resolver_test.go:56-74,164-223,240-340`）。
- **边界：** 没有 production adapter 注入、SDK `Probe/ResolveBareID` 或 MCP probe tool；迁移台账仍要求保持显式 `--type`（`goal-3/api-migration-verification.md:35-40`）。

### 5.15 `rating-filter`（#39）

- **Layer verdict：** Contract/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/Offline=`verified`；SDK=`implemented_unverified`；MCP=`missing`；Release=`rejected`。
- **证据：** local canonical rating 和 client-side matching 位于 `internal/shared/searchfilter/filter.go:10-39,55-137`，测试见 `filter_test.go:11-116`；mapper 保留 `XRestrict`，但 search 不把 rating 写入 upstream query（`sdk/pixiv/map_artwork.go:30-37`、`sdk/pixiv/ops_artwork_search.go:13-128`）；CLI tests 见 `internal/cli/commands/pixiv/search/search_test.go:439-480,522-535`。
- **边界：** MCP search schema 尚未暴露 rating，且历史 server-side `x_restrict` evidence 不能充当完整 live dataset（`internal/mcpserver/pixiv/tools/search_novel/search_novel.go:31-34`；`goal-3/api-migration-verification.md:41-49`）。

### 5.16 `logical-pagination`（#40）

- **Layer verdict：** Contract/Adapter/SDK/CLI/MCP/Live/Compatibility=`implemented_unverified`；Shared/Offline=`verified`；Release=`rejected`。
- **证据：** shared pagination/traversal 与测试见 `internal/shared/pagination/{pagination.go,pagination_test.go}`、`internal/shared/traversal/{traversal.go,streams.go,streams_test.go}`；MCP runtime LSP `CollectWith` 定位在 `internal/mcpserver/pixiv/internal/runtime/runtime.go:224`；SDK search checkpoint 见 `sdk/pixiv/ops_artwork_search.go:39-67,139-156`，CLI listing 见 `internal/cli/commands/pixiv/internal/listing/listing.go:22-76,110-113`。
- **边界：** T23A/R02 只证明 shared/checkpoint/replay engine（`goal-3/pagination-validation-report.md:79-100,136-145`），不证明所有 Pixiv endpoint continuation；latest/recommended/novel continuation 的 live rows 仍有 gaps（`goal-3/api-migration-verification.md:20-34`）。

### 5.17 `recommended-all`（#41）

- **Layer verdict：** Contract/Adapter/Shared/MCP/Live/Compatibility=`implemented_unverified`；SDK=`missing`；CLI/Offline=`verified`；Release=`rejected`。
- **证据：** individual SDK operations 存在但没有 `RecommendedAll` aggregate（`sdk/pixiv/ops_artwork.go:102-127`、`sdk/pixiv/ops_novel.go:170-177`、`sdk/pixiv/ops_user.go:48-70`）；CLI `runAll`/spool/atomic output 见 `internal/cli/commands/pixiv/recommended/all.go:16-72,102-115,190-210`；MCP four-stream handler 见 `internal/mcpserver/pixiv/tools/recommended/recommended.go:94-209`，LSP 定位 `handleRecommended` 为 line 94。
- **边界：** `kind=all` 的四路 pagination、任一必需流失败即整体错误的 compatibility contract 已记录，但没有真实四流 live evidence；T37B 只通过 offline replay，不能提升 public-ready（`goal-3/mcp-compatibility-matrix.md:94`；`goal-3/tasks.md:906-909`；`goal-3/pagination-validation-report.md:24-31`）。

### 5.18 G1-T04 汇总

- 17/17 项已完成 layer inventory；全 required scope 为 `41/41`，全部 `scope_admitted`，`public_ready=0/41`。
- 离线闭合面：comments/stamps 的 adapter/SDK/CLI seams、novel comments read MCP、user read/relationship/MyPixiv MCP、resolver policy、local rating、shared logical pagination、recommended-all CLI/MCP aggregate 均有源码和 focused tests。
- 未闭合面：artwork comments v3 contract rejected；comment mutation MCP surface 缺失；user artwork/novel 及 MyPixiv 缺 strict second page；大多数 user/trending/detail/search/relationship rows 无 strict live；follow mutation 缺 read-back；bare-ID 无 production probe；rating 缺 MCP；logical pagination 非 endpoint-global；recommended-all 缺 SDK aggregate/live 四流证明。
- 关键审计规则：历史 `verified`、离线 fixture、T23A/R02 replay、T37D WIP 均不得改写为 `public_ready` 或 MCP user layer verified。

## 6. G1-T05 Correctness ledger 与 forbidden behavior 复核

> 本节逐项把 code path、test path、历史 evidence 和当前 verdict 放在同一 ledger 中。`offline verified` 只证明当前可复核的实现/适配 seam，不等于 strict live、跨层 continuation 或 `public_ready`；P0/P1 以显式 open 状态记录。

| 复核项 | Code path + test path | 历史 evidence | 当前 verdict |
|---|---|---|---|
| logical pagination / checkpoint / replay | `internal/shared/pagination/pagination.go:37-180`、`internal/shared/traversal/traversal.go:37-105`；`internal/shared/pagination/pagination_test.go:376-538`、`internal/shared/traversal/traversal_test.go:13-169` | `goal-3/tasks.md:205-214,724-733`（T23A/R02） | **Shared engine verified**；只闭合 generic traversal、checkpoint/replay seam，不闭合各 endpoint 的 next-page wire/live。无 P0/P1；保留 endpoint-specific correction candidate。 |
| novel latest | `internal/services/pixiv/endpoint/novel/timeline/timeline.go:101-120,185-197`；`.../timeline_test.go:101-165`；`sdk/pixiv/ops_novel.go:201-212`、`ops_novel_test.go:195-245` | `goal-3/tasks.md:547-556,680-689`；`goal-3/evidence/appapi-upstream.md:11-12` | **Adapter/SDK offline verified**：`max_novel_id` 语义已落到 v2 leaf；CLI/MCP second-page 与 strict live 尚未独立闭合。无 P0/P1；列为 G1-T06 correction/evidence candidate。 |
| novel detail / series | `internal/services/pixiv/endpoint/novel/detail/{detail.go,detail_test.go}`、`novel/series/{series.go,series_test.go}`；`sdk/pixiv/ops_novel.go:66-114`；MCP novel detail/series owners | `goal-3/evidence/appapi-upstream.md:5-8`；`goal-3/api-migration-verification.md:134-139`；`goal-3/pagination-validation-report.md:33-38` | **Offline v2 path verified**；v1 detail/series rejected，live second-page/release evidence 未闭合。无 P0/P1。发现一个 P2 tracking-doc drift：`goal-3/upstream-contract-matrix.md:17` 的“生产仍为 v1”与当前 v2 code path 冲突，留给后续 correction，不在本 task 改 Goal-3。 |
| artwork recommended continuation | `internal/services/pixiv/endpoint/artwork/recommended/recommended.go:24-80,185-197`、`recommended_test.go:26-80`；`sdk/pixiv/ops_artwork.go:101-112`、`ops_artwork_test.go:11-52`；CLI/MCP recommended tests | `goal-3/api-migration-verification.md:28`；`goal-3/wire-adapter-sdk-diff.md:21`；`goal-3/tasks.md:53-54`；`goal-3/pagination-validation-report.md:24-31` | **Open P1**：second-page continuation 当前仍为 `inconclusive/second_page_error`。shared pagination 或 zero-cursor fake test 不能关闭该风险，不得标作 known limitation。Correction candidate 需用 strict live non-empty two-page fixture，保留完整 continuation params，并贯穿 endpoint→SDK→CLI/MCP；G1-T05 不改业务 code。 |
| artwork / novel comments DTO | artwork `/v3/illust/comments` adapter `internal/services/pixiv/endpoint/artwork/comments/comments.go:60-104,231-299`、tests `comments_test.go:52-139`；novel adapter `internal/services/pixiv/endpoint/novel/comments/comments.go:206-306`；SDK DTO mapping `sdk/pixiv/map_artwork.go:247-260`、`map_extra.go:39-52`；MCP read tests | `goal-3/evidence/appapi-upstream.md:18-19`；`goal-3/upstream-contract-matrix.md:262-263`；`goal-3/pagination-validation-report.md:57-63` | **Offline DTO/adapter/SDK/MCP verified**；live pagination inconclusive。无 P0/P1；未达到 public-ready。 |
| restrict / rating / cursor binding | local restrict validation `sdk/pixiv/validation.go:24-40`；cursor envelope/binding `sdk/pixiv/cursor.go:15-58,84-142`；`cursor_test.go:273-331`、`sdk/pixiv/pixiv_test.go:754-834` | `goal-3/upstream-contract-matrix.md:33,252-255` | **Offline validation/binding verified**；没有把 server-side rating 伪装成 upstream filter，也没有把 cursor 当鉴权凭据。无 P0/P1；account-pool switch/live contract 仍是 evidence candidate，不能泛化为“所有 operation 必须 account-bound”。 |
| comments mutation outcome | artwork/novel mutation adapters 与 tests；`sdk/pixiv/ops_comment.go:16-124` | `goal-3/mutation-validation-report.md:5-26,38-42`；`goal-3/evidence/appapi-mutation.md:3-10` | **Transport/DTO offline verified；outcome 未闭合**：production mutation 仍 `not_tested`，无同账号 read-back/cleanup。无隐藏 P1（release 已阻断）；correction candidate 需同账号 create/reply/stamp/delete、response ID read-back、清理确认，并对 uncertain 禁止自动 replay。 |
| rejected endpoint / no-fallback | `sdk/pixiv/ops_novel.go:140-150` 的 `NovelContent`；`internal/mcpserver/pixiv/tools/novel_content/novel_content.go:26-37`；SDK/MCP no-network tests | `goal-3/evidence/appapi-upstream.md:3-10`；`goal-3/api-migration-verification.md:134-144` | **Forbidden behavior verified offline**：v1 detail/series/content 与 WebView fallback 不可达；`ContentUnavailable` 保持显式结果；upstream error 不转空成功，uncertain mutation 不自动重放。无 P0/P1。 |

### 6.1 Open P0/P1

- **P0：无。** 当前没有证据显示已发生数据破坏、凭据泄露、不可逆错误或所有调用方都会命中的阻断性错误。
- **P1：0 项（G1-CORR-G1-T28-RECOMMENDED-01 后闭合）。** `artwork-recommended` continuation 的 open P1 已由 G1-T28 live 证据 + correction 定位根因（多参数 next_url vs 单 offset allowlist）并修复；live 两页 89→89 实证通过。

### 6.2 Constrained correction candidates

以下只建立受约束候选，不在 G1-T05 修改业务代码或扩展 scope：

1. **CAND-G1-T06-REC-RECOMMENDED**：用非空 two-page strict fixture 验证 recommended continuation，覆盖 endpoint/SDK/CLI/MCP 与完整 cursor/next-url 参数。
2. **CAND-G1-T06-REC-LATEST**：补 novel-latest CLI/MCP second-page 与 live evidence；保留 `max_novel_id`，禁止回退 offset 或混合 key。
3. **CAND-G1-T06-REC-SERIES-DOC**：修正 novel-series tracking-doc drift，并把 live second-page evidence 与当前 v2 contract 对齐。
4. **CAND-G1-T06-REC-COMMENTS-MUTATION**：同账号 mutation round-trip/read-back/cleanup；结果区分 confirmed、accepted-not-read-back、uncertain，uncertain 不 replay。
5. **CAND-G1-T06-REC-ACCOUNT-RATING**：验证 account-pool switch、filter digest cursor invalidation 和 local `x_restrict` 语义；不新增未经证实的 server-side rating 或 all-ops account binding。

### 6.3 Out-of-scope observations

- 本轮没有新增产品需求；未来 MCP rating surface、SDK aggregate、更多 strict live fixtures 只作为现有 gap/candidate 记录，不改变 required scope 41 项。
- 当前 branch 仍无业务/API/Goal-3 资料 diff；本轮只更新 Goal-1 tracking 文档。

## 7. Rejected endpoint 与 no-fallback

以下路径和行为必须在所有层保持显式拒绝或不可达，不能以兼容为由 fallback：

- `/v1/novel/detail`：live 404，`upstream_contract_rejected`（`goal-3/evidence/appapi-upstream.md:5`）。目标只允许 v2 detail。
- `/v1/novel/series`：虽返回 HTTP 200，但缺 required detail，`required_field_missing`，不可作为 v2 fallback（`goal-3/evidence/appapi-upstream.md:7`）。
- `/v1/novel/content`：live 404，`upstream_contract_rejected`；`detail --content` 只返回显式 `ContentUnavailable`，不发送 rejected request 或 WebView fallback（`goal-3/evidence/appapi-upstream.md:9-10`；`goal-3/api-migration-verification.md:136-144`）。
- `/webview/v2/novel`：历史 HTTP 200 仅作为排除项，不是 App API fallback。
- server-side `x_restrict` / rating：服务端忽略或不支持时，不得伪装成 upstream filter；只能保留已确认的本地语义与 cursor binding（`goal-3/upstream-contract-matrix.md:33,252-255`）。
- 不得把 cursor 当鉴权凭据；不得把 upstream error 变成空成功结果；不得把不确定 mutation 自动重放。

## 8. 历史 evidence 与当前代码的边界

- Goal-3 历史 tasks 中 T01/T02、T07A/T07B、T10A–T10G、T13/T14/T18、T24–T32、T37A/B 等 `verified` 只证明对应历史 task 的实现或审计，不授予 capability `public_ready`（`goal-3/tasks.md:19-39,51-62,75-87`；`goal-3/capability-admission.md:5-15`）。
- `ugoira-metadata` strict evidence 声称 CLI/MCP confirmed，但当前源码没有专用 CLI/MCP metadata surface；本 inventory 以当前源码为准，将 live 标为 `implemented_unverified`，把该 evidence 冲突留给后续 correction/owner task。
- `novel-latest` strict upstream evidence 已显示 `max_novel_id`；当前 adapter/SDK max-ID leaf 与离线测试可核验，但 CLI/MCP second-page 与 strict live 尚未闭合，不得把历史 evidence 自动提升为完整 cross-layer acceptance。
- 当前分支相对 `e404434` 没有 `goal-3/` diff；上述 gaps 是继承状态，不是本轮业务/API 改动引入。

## 9. 查阅范围与验证命令

### 查阅范围

已检查：

- `goal-1/plan.md`、`goal-1/tasks.md`、`goal-3/capability-admission.md`
- `goal-3/upstream-contract-matrix.md`
- `goal-3/api-migration-verification.md`
- `goal-3/cli-migration-matrix.md`
- `goal-3/mcp-compatibility-matrix.md`
- `goal-3/current-surface-risk-audit.md`
- `goal-3/pagination-validation-report.md`
- `goal-3/mutation-validation-report.md`
- `goal-3/wire-adapter-sdk-diff.md`
- `goal-3/shaft-protocol-diff.md`
- `goal-3/tasks.md`
- `goal-3/evidence/appapi-upstream.md` 及其 JSON/相关 evidence
- capabilities 1–41 的 endpoint、SDK、CLI、MCP source/test 文件及 Git history
- bookmark aggregate 的 `internal/shared/pagination/*`、`internal/shared/traversal/*` 与 CLI regression tests
- G1-T04 comments/stamps、user/relationship、resolver、searchfilter、recommended-all 的 endpoint、SDK、CLI、MCP source/test 文件

### LSP 证据

目标 worktree 已启动 `gopls`；已通过 `open_document`/`list_symbols`/`find_symbol` 核验 protocol、SDK public operation、novel ranking 和 ugoira metadata symbols。关键结果包括：

- `sdk/pixiv.Client.SearchArtworks`、`LatestArtworks`、`ArtworkRanking`、`RecommendedArtworks`、`ArtworkSeries`、`UgoiraMetadata` 存在。
- `sdk/pixiv.Client.SearchNovels`、`Novel`、`NovelSeries`、`LatestNovels`、`RecommendedNovels`、`NovelRanking`、`FollowingNovels` 存在。
- bookmark public symbols `UserArtworkBookmarks`、`UserArtworkBookmarkTags`、`ArtworkBookmark`、`UserNovelBookmarks`、`UserNovelBookmarkTags`、`NovelBookmark` 与 artwork mutation wrappers 存在；novel mutation public symbols、aggregate public operation 不存在。
- `NovelRankingRequest`、`AppNovelRanking`、`UgoiraMetadataRequest` 和 public DTO 存在。
- 当前未发现 `ugoira_metadata` CLI/MCP owner，也未发现专用 `novel_ranking` MCP owner。

### Offline evidence

G1-T01 已在干净目标 worktree 执行 `go test ./...` 并通过；G1-CHECK-01 在 HEAD `042c0200ce1b3349ab3e08a744fea7802838025c` 再次执行 `go test ./...` 并通过。G1-T04 focused command 通过：`go test ./internal/services/pixiv/endpoint/artwork/comments ./internal/services/pixiv/endpoint/novel/comments ./internal/services/pixiv/endpoint/stamps ./sdk/pixiv ./internal/cli/commands/pixiv/comment ./internal/mcpserver/pixiv/... -count=1`。G1-T05 correctness focused commands 通过：shared pagination/traversal；novel latest；artwork recommended endpoint/SDK/CLI/MCP；novel detail/series；artwork/novel comments；SDK validation/cursor；MCP read/no-fallback。具体执行均为 `-count=1`，未执行真实 Pixiv live API；本文件引用的 source/test/evidence 均为当前分支可追溯资料。

## 10. G1-T05、G1-T04、G1-T03 与 G1-CHECK-01 结论

- 覆盖：41/41；G1-T04 补齐 25–41 共 17 项；无 capability 漏项；全 required scope 为 `41/41 scope_admitted`，`public_ready=0/41`。
- 当前可核验实现仍主要集中在 offline adapter/SDK/CLI/MCP leaf 与局部 aggregate；strict live、mutation read-back/cleanup、MCP mutation、SDK aggregate、endpoint-global continuation 等未闭合面保持为明确 verdict。
- `logical-pagination` 的 T23A/R02 只证明 shared/checkpoint/replay engine；不能替代 user/comments/recommended/latest 等 endpoint continuation evidence。
- G1-T07/G1-T08/G1-T09 已完成 MCP/Shared offline acceptance：user identity、collections、relationships、MyPixiv，以及 artwork/novel typed bookmark list/tags/detail 的 schema、resolver、filter、pagination、structured error、legacy JSON replay 与 additive registration 均由当前目标分支回归覆盖；G1-T10 又补齐 bookmark aggregate 的双流预算、checkpoint、typed tag 与页原子失败；G1-T11 复核 exact registration、schema/error 与 forbidden endpoint gate。上述证据不替代 strict live、candidate contract、compatibility 或 release gate。
- G1-CHECK-01：历史集中检查仍 PASS，覆盖 1–24；G1-T04 inventory 与 G1-T05 correctness focused tests 均 PASS，未改生产代码、API、scope 或 Goal-3 资料；artwork-recommended second-page continuation 的 P1 仍 open。
- G1-CHECK-03：PASS。静态/语义审计确认 MCP leaf 仅经 public `sdk/pixiv` 与 owner-local `internal/mcpserver/pixiv/internal/{runtime,filters,records,outputs}`；无 `internal/services/pixiv|fanbox` 直连；生产 MCP 无 `/v1/novel/detail`、`/v1/novel/series`、`/v1/novel/content` 调用。既有 user legacy replay、typed error/empty、resolver/filter/logical pagination、bookmark legacy wire、candidate novel read 与 exact registration 回归均通过；未发现额外 abstraction 或需本轮 correction。该证据不替代 strict live、candidate contract、compatibility 或 release gate。
- GoalState：保持 `ACTIVE`。G1-T05 的 1 个 open P1 已在 G1-T06 映射到具体 correction owner；G1-T09 只关闭 typed bookmark 的 MCP/offline 责任链，G1-CHECK-03 审计未扩大 scope；G1-T10/G1-T11 只补齐 aggregate 与 read gate 的离线/注册证据，未把缺失的 ugoira metadata、novel ranking、rating owner 或 public SDK aggregate 静默视为完成；novel candidate strict live/compatibility 仍保持未验证；无新增 external/decision blocker。G1-CHECK-02 已完成 Phase A 检查并通过普通 fast-forward push，checkpoint Local/Remote SHA 均为 `b17b1775f2ea0783f394f7600d30cafd8ad428c5`；下一任务为 `G1-T12`。


## 11. G1-T06 Live Manifest 与 finite execution mapping

> **冻结口径：** 本 manifest 是进入 Phase F 前的唯一 live scope。`live_required=yes` 只表示该 capability 最终需要真实场景；不表示当前已经 live verified。`mapped_to_task` 是本阶段的执行映射状态，不等于最终 `accepted` 或 `public_ready`。数据不足、账号/权限、网络或上游不可用时，只能按本表条件记录 `blocked_external`，不得伪造第二页或成功。

### 11.1 Manifest：artwork / novel feed（1–13）

| # / capability | live_required | endpoint / path family | required scenario；second-page | 账号 / 目标数据条件 | mutation read-back / cleanup | 当前 evidence / blocker | mapping status / owner |
|---:|---|---|---|---|---|---|---|
| 1 `artwork-search` | yes | `GET /v1/search/illust` | 稳定 query；覆盖 `all/illust/manga/ugoira`；首页+续页；**required** | 已认证账号；每个代表性 content type 需非空两页 | N/A（read-only） | HTTP 200、30→30、offset 与 adapter/SDK wire 已有；rating/filter/account binding 未闭合 | `mapped_to_task`：G1-T28；cursor/compat：G1-T19–T27；rating candidate：`CAND-G1-T06-REC-ACCOUNT-RATING` |
| 2 `artwork-latest` | yes | `GET /v1/illust/new` | latest；验证承诺 subtype 与 `max_illust_id`；首页+续页；**required** | 已认证账号；每个承诺 subtype 需稳定非空两页 | N/A | `illust-new` 有 strict 30→30；manga/ugoira/compound subtype 与 SDK offset/max 双形态仍未闭合 | `mapped_to_task`：G1-T28；correction：`CAND-G1-T06-REC-LATEST` |
| 3 `artwork-ranking` | yes | `GET /v1/illust/ranking` | 固定 `mode`、日期/默认日期；offset 首页+续页；核验无重复；**required** | 已认证账号；指定 mode/date 需两页 | N/A | HTTP 200、30→30、offset 已有；compat/release 未闭合 | `mapped_to_task`：G1-T28；compat/regression：G1-T19–T27 |
| 4 `artwork-recommended` | yes | `GET /v1/illust/recommended` | 非空首页；保留完整 continuation 参数；真实第二页；**required** | 已认证账号；推荐结果必须非空且返回 continuation | N/A | G1-T28+correction：live 首页 89 项 + 真实第二页 89 项（多参数 continuation 收敛后），Open P1 闭合 | `mapped_to_task`：G1-T28；correction：`CAND-G1-T06-REC-RECOMMENDED`（已由 G1-CORR-G1-T28-RECOMMENDED-01 关闭） |
| 5 `artwork-series` | yes | `GET /v1/illust/series` | 有效 series ID；`last_order` 首页+续页；required metadata；**required** | 可访问且至少两页的 artwork series | N/A | 当前无独立 live 第二页证据；offline 两页不替代 live | `mapped_to_task`：G1-T28；correction：`CAND-G1-T06-ARTWORK-SERIES-LIVE` |
| 6 `ugoira-metadata` | yes | `GET /v1/ugoira/metadata` | 有效 ugoira artwork；读取 archive/frames；**no** | 已认证且可访问的真实 ugoira ID | N/A | 历史 upstream evidence 存在；当前缺专用 CLI/MCP owner，不能只当 external blocker | `mapped_to_task`：G1-T28、G1-T11、G1-T21、G1-T22；correction：`CAND-G1-T06-UGOIRA-SURFACE` |
| 7 `novel-search` | yes | `GET /v1/search/novel` | 固定 query；覆盖 period/date；正 offset 首页+续页；**required** | 已认证账号；query 需非空两页，period/date 可验证 | N/A | strict 两页与 period/date evidence 未闭合 | `mapped_to_task`：G1-T28、G1-T25；correction：`CAND-G1-T06-NOVEL-SEARCH-CONTRACT` |
| 8 `novel-detail` | yes | `GET /v2/novel/detail`；旧 `/v1/novel/detail` 必须 rejected | 有效 novel detail；required novel/series metadata；**no** | 已认证账号；可靠正 novel ID | N/A | v2 wire/response 可见；adapter/SDK strict full-chain 未测试；v1 404/rejected | `mapped_to_task`：G1-T20、G1-T25、G1-T28；correction：`CAND-G1-T06-NOVEL-DETAIL-SERIES-V2` |
| 9 `novel-series` | yes | `GET /v2/novel/series`；旧 v1 必须 rejected | 有效 series ID；`last_order` 首页+续页；**required** | 已认证账号；series 至少两页 | N/A | v2 首页有记录，第二页未观察；tracking-doc 与当前 v2 path 有 drift | `mapped_to_task`：G1-T20、G1-T23、G1-T28；correction：`CAND-G1-T06-REC-SERIES-DOC` |
| 10 `novel-latest` | yes | `GET /v1/novel/new`；`filter=for_android`；`max_novel_id` | latest 首页+`max_novel_id` 续页；禁止 offset fallback/mixed key；**required** | 已认证账号；feed 必须非空两页 | N/A | upstream 30→30；当前 strict row 为 `sdk_call_error`，CLI/MCP second-page 未闭合 | `mapped_to_task`：G1-T28；correction：`CAND-G1-T06-REC-LATEST` |
| 11 `novel-recommended` | yes | `GET /v1/novel/recommended` | 推荐首页+offset 续页；核验无重复；**required** | 已认证账号；推荐 feed 需两页 | N/A | HTTP 200、33→33、offset 与 adapter/SDK wire 已有；compat/release 未闭合 | `mapped_to_task`：G1-T28；compat/regression：G1-T19–T27 |
| 12 `novel-ranking` | yes | `GET /v1/novel/ranking` | 固定 filter/mode；offset 首页+续页；**required** | 已认证账号；指定 ranking 场景需两页 | N/A | upstream 30→30；adapter/SDK `not_tested`；当前无专用 MCP owner | `mapped_to_task`：G1-T11、G1-T20、G1-T22、G1-T28；correction：`CAND-G1-T06-NOVEL-RANKING-SURFACE` |
| 13 `novel-follow` | yes | `GET /v1/novel/follow`；`restrict` + offset | following 首页+续页；至少一个明确 restrict；**required** | 已认证账号；following feed 需两页 | N/A | HTTP 200、30→30、offset 已有；strict contract/compat 未闭合 | `mapped_to_task`：G1-T28；compat/regression：G1-T19–T27 |

### 11.2 Manifest：bookmark（14–24）

| # / capability | live_required | endpoint / path family | required scenario；second-page | 账号 / 目标数据条件 | mutation read-back / cleanup | 当前 evidence / blocker | mapping status / owner |
|---:|---|---|---|---|---|---|---|
| 14 `artwork-bookmark-list` | yes | `GET /v1/user/bookmarks/illust` | public/private representative read；**no：data-limited exception**；若自然有 continuation 只记录，不伪造 | 同一认证账号；有可访问 artwork bookmarks；正 user ID | N/A（read-only） | adapter/SDK/CLI/MCP/offline 有；strict live 未闭合；pagination exemption 不等于 live pass | `mapped_to_task`：G1-T09、G1-T29；cursor/compat：G1-T19–T27 |
| 15 `artwork-bookmark-tags` | yes | `GET /v1/user/bookmark-tags/illust` | tags name/count、空列表、restrict；**conditional：continuation contract 未冻结** | 账号需有 tags；允许合法空 tags | N/A | strict tag wire/live continuation `not_tested`；null-as-empty 不得冒充 strict contract | `mapped_to_task`：G1-T09、G1-T25、G1-T29 |
| 16 `artwork-bookmark-detail` | yes | `GET /v2/illust/bookmark/detail` | 已收藏与未收藏/absent；tags/restrict/error；**no** | 可靠正 illust ID；同账号可访问 | N/A | offline fixture 与 strict live absent/bookmarked evidence 已有；未收藏作品 tags 归一为空 | `mapped_to_task`：G1-T09、G1-T29、G1-CORR-G1-T29-BOOKMARK-DETAIL-01 |
| 17 `artwork-bookmark-mutation` | yes | `POST /v2/illust/bookmark/add`；`POST /v1/illust/bookmark/delete` | add 或 remove 一次；**no**（read-back 不是第二页） | 明确授权隔离账号；可靠正 illust ID；目标可写 | 必须保存原状态；同账号 detail/list/tags read-back；只清理本轮副作用；uncertain 不 replay | 当前只有 transport/form/validation；2xx 不证明状态变化 | `mapped_to_task`：G1-T13、G1-T17、G1-T18、G1-T30 |
| 18 `novel-bookmark-list` | yes | `GET /v1/user/bookmarks/novel` | restrict、required novels、空结果；**no：data-limited exception**；不得伪造第二页 | 同一认证账号；有可访问 novel bookmarks；正 user ID | N/A | endpoint/SDK/CLI/MCP/offline 有；strict live 仍 `implemented_unverified` | `mapped_to_task`：G1-T09、G1-T29；cursor/compat：G1-T19–T27 |
| 19 `novel-bookmark-tags` | yes | candidate `GET /v1/user/bookmark-tags/novel` | 先冻结 strict candidate，再读 name/count/empty/error；**conditional：不得猜 continuation** | 账号可有 novel tags，也允许合法空结果 | N/A | Goal-3 `not_tested`；candidate adapter/SDK/MCP/offline 已有，MCP 不发明 continuation，strict live/public compatibility 仍未冻结 | `mapped_to_task`：G1-T09、G1-T25、G1-T29 |
| 20 `novel-bookmark-detail` | yes | candidate `GET /v2/novel/bookmark/detail` | 已收藏/未收藏；absent/404、public/private、tags/error；**no** | 可靠正 novel ID；同账号可访问 | N/A | absent strict live 已通过且作品 tags 归一为空；账号无 novel bookmark，bookmarked=true 数据为 `blocked_external (data)`；candidate compatibility 仍未闭合 | `mapped_to_task`：G1-T09、G1-T20、G1-T25、G1-T29、G1-CORR-G1-T29-BOOKMARK-DETAIL-01` |
| 21 `novel-bookmark-mutation` | yes | candidate `POST /v2/novel/bookmark/add`；`POST /v1/novel/bookmark/delete` | add/delete 一次；**no**（read-back 不是第二页） | 明确授权隔离账号；可靠正 novel ID；先保存原状态 | detail/list/tags 同账号 read-back；删除后恢复原 bookmark/restrict/tags；只清理本轮副作用；uncertain 不 replay | offline public SDK/MCP transport/schema/error/no-replay evidence 已有；当前 live `missing`，read-back/cleanup 未执行 | `mapped_to_task`：G1-T13、G1-T17、G1-T18、G1-T20、G1-T22、G1-T30
| 22 `bookmark-subtype` | no | artwork bookmark family 的 client-side selector；无已批准 server subtype path | 不执行 server-side subtype live；offline 验证 `artwork\|novel\|all`、client-side filter、logical limit；**no** | synthetic fixture 覆盖多种 subtype | N/A | MCP client-side subtype filter 与 logical pagination 已 offline verified；server-side subtype 无 evidence；`all` 不是 upstream subtype | `mapped_to_task`：G1-T09、G1-T19、G1-T21、G1-T22 |
| 23 `bookmark-list-all` | yes | artwork bookmark list → novel bookmark list aggregate | 同一账号执行 `--type all`；artwork 后 novel；统一 budget/typed output/failure atomicity；**conditional：仅自然 continuation** | 同一账号可访问两类 bookmarks；最好两流非空，允许一流为空 | N/A（read-only aggregate） | CLI/MCP offline aggregate 有；无 public SDK/MCP aggregate cursor、strict live 或 aggregate live evidence | `mapped_to_task`：G1-T10、G1-T19、G1-T20、G1-T22、G1-T29 |
| 24 `bookmark-tags-all` | yes | artwork tags → novel tags candidate aggregate | `--type all`；同名 tag 不合并；count/type/atomicity；**conditional：novel continuation 未冻结** | 同一账号可读两类 tags；至少一侧有 tags，另一侧可空 | N/A | CLI/MCP offline aggregate 有；novel candidate continuation、strict compatibility 与 aggregate live evidence 缺失 | `mapped_to_task`：G1-T10、G1-T19、G1-T20、G1-T22、G1-T29 |

### 11.3 Manifest：comments / user / relationship（25–37）

| # / capability | live_required | endpoint / path family | required scenario；second-page | 账号 / 目标数据条件 | mutation read-back / cleanup | 当前 evidence / blocker | mapping status / owner |
|---:|---|---|---|---|---|---|---|
| 25 `artwork-comments-read` | no | 当前 `/v3/illust/comments` contract rejected；不得 fallback | 不发 rejected request；执行 no-fallback negative guard；**no** | 若未来批准新 contract，才需可评论且有 comments 的 artwork；当前不要求 live data | N/A（read-only） | 当前 v3 缺 required `date`/numeric access-control/strict DTO/second-page；不能把 rejection 当成功 | `mapped_to_task`：G1-T24；correction：`CAND-G1-T06-ARTWORK-COMMENTS-REJECTED`；只有 approved replacement 才能再映射 T29 |
| 26 `artwork-comments-mutation` | yes | `POST /v1/illust/comment/add`、`POST /v1/illust/comment/delete`；reply/stamp 共用 add | text/reply/stamp 各最小一条并 delete 本轮 comment；**no** | 明确授权隔离账号；目标 artwork 可评论；必须取得可靠 comment ID | 同账号 read-back；只删除本轮 comment ID；ID 不确定不得猜删/重放 | adapter/SDK/offline transport/MCP mutation 有；production live/read-back/cleanup 未闭合 | `mapped_to_task`：G1-T14、G1-T17、G1-T18、G1-T30；correction：`CAND-G1-T06-REC-COMMENTS-MUTATION` |
| 27 `novel-comments-read` | yes | 当前生产 `GET /v2/novel/comments`；v3 为 candidate | 非空 comments；DTO/access-control；有 continuation 时完成第二页；**required if returned** | 已认证账号；目标 novel 可访问且有 comments；需可取得跨页数据，缺数据按 external blocker 记录 | N/A | v2 strict row HTTP 200 但 second page 未观察；v3 single-page/inconclusive | `mapped_to_task`：G1-T29；若切换 v3，追加 correction/contract owner |
| 28 `novel-comments-mutation` | yes | `POST /v1/novel/comment/add`、`POST /v1/novel/comment/delete` | text/stamp/delete 本轮 comment；**no** | 明确授权隔离账号；目标 novel 可评论；保存响应 ID | 同账号 read-back；只删除本轮 ID；uncertain 不 replay | adapter/SDK/offline transport/MCP mutation 有；production live/read-back/cleanup 未闭合 | `mapped_to_task`：G1-T15、G1-T17、G1-T18、G1-T30；correction：`CAND-G1-T06-REC-COMMENTS-MUTATION` |
| 29 `stamps` | yes | `GET /v1/stamps`；写入通过 artwork/novel comment add stamp | 至少一次 stamps read；#26/#28 各覆盖一条 stamp；**no** | read 需真实 stamp target；mutation 需隔离账号和可评论目标 | 同 comment mutation：read-back、只清理本轮 comment/stamp ID | strict wire/response 与 artwork/novel MCP stamp mutation offline evidence 有；当前无 standalone stamps public surface，live/read-back 未闭合 | `mapped_to_task`：G1-T14、G1-T15、G1-T30；correction：`CAND-G1-T06-REC-COMMENTS-MUTATION` |
| 30 `user-artworks` | yes | `GET /v1/user/illusts`；`type=illust\|manga` | representative user read；**no：pagination_exempt**；若自然有 continuation 只记录 | 正数 public user ID；允许合法空/单页，但目标必须有效 | N/A | adapter/SDK/CLI/MCP/offline 有；历史仅首请求，second page 未观察 | `mapped_to_task`：G1-T08、G1-T29 |
| 31 `user-novels` | yes | `GET /v1/user/novels`；`filter=for_android` | required novels/nested IDs；**no：pagination_exempt** | 正数 public user ID；数据受限可接受空/单页 | N/A | fixture/adapter/SDK 有；strict live second page 未观察 | `mapped_to_task`：G1-T08、G1-T29 |
| 32 `user-relationships` | yes | following/follower/related families；blocked path `/v2/user/list` remains blocked | representative following/follower/related/blocked read；**conditional：有 continuation 才验证，不伪造** | 已认证；private relationship 需权限；related 需正 seed user ID；403/跨账号不得 fallback | N/A | offline continuation 有；strict live rows 未登记 | `mapped_to_task`：G1-T08、G1-T29 |
| 33 `user-detail` | yes | `GET /v1/user/detail`；CurrentUser identity path | user detail + CurrentUser identity；**no** | 正数 user ID；CurrentUser 只用已验证 client identity 和 `filter=for_android` | N/A | fixture/SDK/MCP 有；strict live row 缺失；不得从 token 猜 UID | `mapped_to_task`：G1-T07、G1-T29 |
| 34 `user-search` | yes | `GET /v1/search/user` | non-empty word；首页+有 continuation 时第二请求；**conditional** | 已认证或明确 public-scoped；word 非空；user IDs 正数 | N/A | fixture/SDK 有；strict live/account-cursor binding 未闭合 | `mapped_to_task`：G1-T07、G1-T29 |
| 35 `trending` | yes | `GET /v1/trending-tags/illust` | non-empty trending tags；sample artwork 合法；**no** | 需取得带合法 sample artwork 的响应 | N/A | fixture/SDK/MCP 有；strict live 未登记；CLI owner 缺失 | `mapped_to_task`：G1-T07、G1-T21、G1-T29 |
| 36 `follow-mutation` | yes | `POST /v1/user/follow/add`；`POST /v1/user/follow/delete` | 保存原关系；add/read-back true；delete/read-back false；**no** | 明确授权隔离账号；正数目标 user；同账号执行上下文 | 强制同账号 read-back；只恢复本轮可识别关系；uncertain 不 retry/replay | endpoint/SDK/MCP/offline wire 有；Live `missing`；2xx 不证明关系变化 | `mapped_to_task`：G1-T16、G1-T17、G1-T18、G1-T30 |
| 37 `mypixiv` | yes | `GET /v1/user/mypixiv`；`GET /v2/illust/mypixiv`；`GET /v1/novel/mypixiv` | current identity users/artworks/novels representative read；**conditional：若有 continuation 才验证** | 只允许 verified current user/client identity；不接受外部 UID、匿名或跨账号 cursor | N/A | fixtures 与当前 MCP/Shared offline acceptance 已验证；strict live second page 未登记，不据 synthetic continuation 升格 | `mapped_to_task`：G1-T08、G1-T29 |

### 11.4 Manifest：shared semantics / aggregate（38–41）

| # / capability | live_required | endpoint / path family | required scenario；second-page | 账号 / 目标数据条件 | mutation read-back / cleanup | 当前 evidence / blocker | mapping status / owner |
|---:|---|---|---|---|---|---|---|
| 38 `bare-id-probe` | no | 无已批准 production endpoint；resolver 仅 shared policy | 不执行 live probe；先保持显式 `--type`；**no** | 若未来冻结 namespace，需 single-ID 多命中/403/404/network 分类 | N/A | resolver tests 有；Adapter/SDK/MCP/Live missing；不得隐式 fallback | `mapped_to_task`：G1-T21、G1-T24；correction：`CAND-G1-T06-BARE-ID-SURFACE` |
| 39 `rating-filter` | no | 本地 normalized `x_restrict` / search filter；不得宣称 upstream rating | local fixture 验证不同 `XRestrict`；若伴随 live，只验证本地过滤；**no：rating 本身不进 live gate** | fixture 必须含不同 XRestrict；禁止发未经确认的 upstream rating/x_restrict | N/A | local canonical rating/filter offline verified；MCP missing；server-side evidence inconclusive | `mapped_to_task`：G1-T19、G1-T20、G1-T21、G1-T22；correction：`CAND-G1-T06-REC-ACCOUNT-RATING`、`CAND-G1-T06-RATING-MCP-SURFACE` |
| 40 `logical-pagination` | no | `internal/shared/pagination`、`internal/shared/traversal`；endpoint continuation 由各 owner 负责 | 不单独做 live API；shared checkpoint/replay/Skip/Limit/OneBatch/duplicate/cancel offline；**generic fixture second-page yes，live N/A** | synthetic pages、合法 continuation、失败/取消/replay；generic evidence 不替代 endpoint live | N/A | shared engine offline verified；T23A/R02 不覆盖 endpoint-global continuation | `mapped_to_task`：G1-T19；endpoint owners：G1-T28、G1-T29 |
| 41 `recommended-all` | yes | artwork `/v1/illust/recommended`、novel `/v1/novel/recommended`、user `/v1/user/recommended`，CLI/MCP `kind=all` 四分区 | 四流 aggregate；每个可分页流实际 continuation；统一 budget、任一路失败整体失败；**required** | 已认证；四流均需非空或按 manifest 记录 data blocker；不得输出 partial success | N/A（read-only aggregate） | CLI/MCP offline aggregate 有；SDK `RecommendedAll` missing；四流 strict live 缺；artwork continuation P1 open | `mapped_to_task`：G1-T11、G1-T20、G1-T22、G1-T28、G1-T29；correction：`CAND-G1-T06-RECOMMENDED-ALL-SURFACE` |

### 11.5 Finite execution mapping

| execution owner | frozen responsibility | capability / evidence coverage |
|---|---|---|
| G1-T07 | MCP user identity read、schema、resolver、pagination、error | #33–35；#35 CLI gap转 G1-T21 |
| G1-T08 | MCP user collections、relationships、MyPixiv read | #30–32、#37 |
| G1-T09 | MCP typed bookmark list/tags/detail、client-side subtype | #14–16、#18–20、#22 |
| G1-T10 | bookmark list/tags aggregate | #23–24 |
| G1-T11 | read registration/exact-set；required missing read owners 不得静默跳过 | #6、#12、#39、#41 的 registration/schema owner；缺 surface 进入对应 correction |
| G1-T12 | read legacy JSON replay、stdio stdout/stderr boundary、structured error wire | read capabilities #1–16、#18–20、#22–25、#27、#30–35、#37、#39–41；mutation read-back output仍由 G1-T18/G1-T30 负责 |
| G1-T13 | bookmark mutation MCP layer | #17、#21 |
| G1-T14 | artwork comment/stamp MCP mutation | #26、#29 |
| G1-T15 | novel comment/stamp MCP mutation | #28、#29 |
| G1-CHECK-05 | 集中复查 bookmark/comment mutation 的 ID、outcome/error、legacy wire、uncertain no-replay 与 abstraction scope | #17、#21、#26、#28–29；不改变 capability 状态 |
| G1-T16 | follow/unfollow MCP mutation | #36 |
| G1-T17 / G1-T18 | shared mutation outcome、read-back/cleanup、uncertain no replay、legacy contract | #17、#21、#26、#28、#29、#36 |
| G1-T19 | cursor integrity、filter/account/subtype binding、generic logical pagination | #1–5、#7、#10–13、#14–24、#27、#30–34、#37、#39–41 |
| G1-T20 | exported SDK symbols/wrappers、v2 contract、aggregate SDK owner | #8–12、#20–24、#39、#41 |
| G1-T21 | CLI canonical/legacy surface、presentation、explicit-type/no implicit fallback、local rating | #6、#21、#35、#38–39 |
| G1-T22 | MCP compatibility、exact-set、missing/additive read surface、MCP rating boundary | #6、#12、#21–25、#39、#41 |
| G1-T23 | docs/Skill/changelog synchronization、v2/rejected/no-fallback wording | #8–9、#25、#38–41 |
| G1-T24 | forbidden endpoint/no-fallback negative gate、rejected artwork comments、bare-ID boundary | #25、#38–39；并保护 #8–9 的 v1 rejection |
| G1-T25 | protocol/endpoint/SDK regression、continuation allowlist、negative/rejected fixtures | 全 41 项的 contract/adapter/SDK regression |
| G1-T26 | CLI/MCP regression、JSON/NDJSON/cursor/aggregate/mutation safety | 全 41 项的 public presentation/registration regression |
| G1-T27 | full offline/build/vet/race-as-needed/redaction/docs gate | 全 41 项最终 offline/release gate |
| G1-T28 | manifest feed live read、query、continuation、错误边界 | #1–13；#41 的 artwork/novel/user feed streams |
| G1-T29 | manifest bookmark/comments/user/MyPixiv/relationship read；data-limited policy | #14–16、#18–20、#23–25、#27、#30–35、#37；#41 aggregate read |
| G1-T30 | manifest mutation round-trip、read-back、cleanup、uncertain no replay | #17、#21、#26、#28–29、#36 |
| G1-CHECK-02 | 冻结 manifest、检查 counts、P0/P1 显式暴露、Phase A push gate | 41/41 mapped；36 live-required；5 no-live；0 unmapped；0 undecomposed |

### 11.6 Correction owner registry

本轮不新增 required capability，也不增加新的 live scope。以下候选均已有具体后续 owner；是否转为代码 correction 由对应 task 的 Red/evidence 决定：

| candidate | bounded correction / evidence | owner |
|---|---|---|
| `CAND-G1-T06-REC-RECOMMENDED` | strict non-empty two-page recommended；保留完整 continuation；贯穿 endpoint→SDK→CLI/MCP | G1-T28、G1-T29、G1-T20、G1-T22 |
| `CAND-G1-T06-REC-LATEST` | novel-latest CLI/MCP second-page + live；保留 `max_novel_id`，禁止 offset/mixed key | G1-T20、G1-T28 |
| `CAND-G1-T06-REC-SERIES-DOC` | v2 series live/second-page evidence + tracking-doc correction | G1-T20、G1-T23、G1-T28 |
| `CAND-G1-T06-REC-COMMENTS-MUTATION` | same-account create/reply/stamp/delete；response ID read-back；cleanup；uncertain no replay | G1-T14、G1-T15、G1-T17、G1-T18、G1-T30 |
| `CAND-G1-T06-REC-ACCOUNT-RATING` | pool switch、filter digest cursor invalidation、local restrict；不伪造 server-side rating | G1-T19、G1-T20、G1-T21、G1-T22 |
| `CAND-G1-T06-ARTWORK-SERIES-LIVE` | artwork series reliable ID + strict second-page | G1-T25、G1-T28 |
| `CAND-G1-T06-UGOIRA-SURFACE` | additive CLI/MCP owner 或明确 correction/blocker；不把缺 surface 静默当作 live success | G1-T11、G1-T21、G1-T22 |
| `CAND-G1-T06-NOVEL-SEARCH-CONTRACT` | period/date contract、query binding、strict continuation | G1-T19、G1-T25、G1-T28 |
| `CAND-G1-T06-NOVEL-RANKING-SURFACE` | adapter/SDK contract 与 MCP owner；registration additive | G1-T11、G1-T20、G1-T22、G1-T28 |
| `CAND-G1-T06-NOVEL-DETAIL-SERIES-V2` | v2 detail/series adapter/SDK strict evidence；v1 rejection regression | G1-T20、G1-T23、G1-T25、G1-T28 |
| `CAND-G1-T06-ARTWORK-COMMENTS-REJECTED` | 保持 rejected endpoint/no-fallback；只有批准 replacement 才允许新 read path | G1-T24；后续若批准再映射 G1-T29 |
| `CAND-G1-T06-BARE-ID-SURFACE` | explicit `--type` 与 no implicit probe/fallback；不凭空新增 resolver surface | G1-T21、G1-T24 |
| `CAND-G1-T06-RATING-MCP-SURFACE` | 仅在 frozen contract 需要时补 MCP local rating semantics；不宣称 upstream rating | G1-T21、G1-T22 |
| `CAND-G1-T06-RECOMMENDED-ALL-SURFACE` | SDK aggregate、MCP/CLI exact-set、四流 live 与 failure atomicity | G1-T11、G1-T20、G1-T22、G1-T28、G1-T29 |

### 11.7 G1-T06 counts / blockers / state

- **Required：** 41。
- **Live-required：** 36；`live_required=no`：#22、#25、#38、#39、#40，共 5 项。
- **Mapping：** `mapped_to_task=41`；`accepted_by_evidence=0`（本 task 只冻结执行映射，不提前宣称最终 acceptance）；`blocked_external=0`；`blocked_decision=0`。
- **Unmapped / undecomposed：** `unmapped=0 / undecomposed=0`。每个 capability 都有主 task、配套 gate 或 correction owner；缺失 public surface 已显式登记，不再留在“以后再看”。
- **Open correctness：** P0=0；P1=1（#4 `artwork-recommended` continuation），已映射至 G1-T28/G1-T20/G1-T22，未隐藏。
- **Mutation：** #17、#21、#26、#28、#29、#36 必须执行写前授权、可靠 ID、同账号 read-back、仅清理本轮副作用；uncertain 不 replay。
- **External/decision blocker：** 当前无 blocker。未来 live 数据/账号/权限/网络不足时，严格按 manifest 记录 `blocked_external`；不得把 internal bug 归类为 external blocker。
- **Freeze boundary：** 后续 live task 只能执行本表 `live_required=yes` 的 scenario；不得临时增加 query、subtype、第二页或 mutation scope。G1-CHECK-02 已复核 manifest、counts、P0/P1、owner mapping 并完成普通 fast-forward push；Phase B 已完成 G1-T07、G1-T08、G1-T09、G1-CHECK-03、G1-T10、G1-T11、G1-T12 与 G1-CHECK-04，Phase B push-exit checkpoint `c902b342ddec687a078f6fff99967dfb4689046a` 已普通 fast-forward 推送且 Remote SHA == Local HEAD；G1-T13、G1-T14 与 G1-T15 已完成 offline mutation MCP gate；G1-CHECK-05 已复查 bookmark/comment mutation contract 且无 correction；G1-T16 已补齐 follow/unfollow 的 uncertain no-replay、invalid input 网络前拒绝与 MCP typed failure 回归，下一任务为 G1-T17；G1-T17 已核验 shared mutation outcome/uncertainty 语义（definite/uncertain 分类、committed 边界、helper 复用）；G1-T18 已完成 mutation legacy wire/structured error 复核并补齐 offline read-back 编排回归；G1-CHECK-06 已通过 Phase C exit 审计并完成普通 fast-forward push（Local/Remote SHA 均为 `5a057ad6f7510c7c005c22974012b830152e3ed9`）；G1-T19 已核验 cursor integrity/binding/rollback gate；G1-T20 已核验 public SDK compatibility 且无 breaking decision；G1-T21 已按 frozen map 补齐 bookmark add/remove novel CLI surface 并复核 trending/ugoira CLI 裁定；G1-CHECK-07 已集中复查 cursor+SDK+CLI（PASS、无 correction）；G1-T22 已核验 MCP compatibility exact-set/schema/replay/rejected-path 且无 breaking decision；G1-T23 已同步双语文档/Skill/changelog；G1-T24 已通过 forbidden endpoint/no-fallback negative gate（PASS、无 correction）；G1-CHECK-08 已通过 Phase D exit 审计并完成普通 fast-forward push（Local/Remote SHA 均为 `c4a99ec4ecb3646bced231d493db94d641405969`）；G1-T25 已完成 protocol/endpoint/SDK regression（40 包全 PASS）；G1-T26 已完成 CLI + MCP regression（28+10 包全 PASS）；G1-T27 已通过 full offline build/quality/race/redaction gate；G1-CHECK-09 已通过 Phase E exit 审计并完成普通 fast-forward push（Local/Remote SHA 均为 `2b465546067b45d45488d93d275a8def3fe82979`）；G1-T28 已执行 live read 场景并注册抢占式 correction；G1-CORR-G1-T28-RECOMMENDED-01 已完成（P1 闭合、#4/#11 live verified）；G1-T29 已执行 bookmark/comments/user live 场景；G1-CORR-G1-T29-BOOKMARK-DETAIL-01 已完成（#16 Live verified、#20 absent pass 且 bookmarked=true 数据 blocked_external），下一任务为 G1-T30。


## 12. G1-T11 MCP read registration/schema/error gate

- **结论：** PASS，no-op verified。现有实现和离线证据已满足本 task 的 registration/schema/error acceptance，因此没有修改生产代码、tool 名称、schema 或 API。
- **Exact registration：** `TestServerListsExpectedTools` 对 MCP client-visible tool 做 exact-set 核验，当前集合为 44 个名称；legacy read/mutation/download/reverse-search 名称均保留，G1-T10 的 `bookmark_list_all` / `bookmark_tags_all` 作为 additive operations 保留。
- **Schema：** `TestFeedRecommendationSchemasMatchLegacyContracts`、`TestArtworkNovelReadOutputSchemasMatchWireEnvelopes`、`TestUserReadSchemasMatchLegacyContracts` 通过；`recommended.kind` 仍是 `all|illust|manga|novel|user`，`recommended` output 仍是 `records` + 四路 `pagination`。`TestEveryToolOutputSchemaOmitsTransportAndCredentialFields` 通过，未发现 nil output schema 或 transport/credential property。
- **Structured error：** recommended、ranking、user、typed bookmark、artwork/novel read 的 invalid-input、SDK-failure、empty/partial-failure tests 通过；`TestToolErrorResultPreservesStructuredContent` 与 `TestToolErrorOutputDoesNotLeakCanary` 通过，错误保持 `isError=true`、structured envelope 安全且不泄漏 transport/credential。
- **Manifest correction routing：** `ugoira-metadata`、`novel-ranking`、`rating-filter` 缺失 owner 仍分别由 `CAND-G1-T06-UGOIRA-SURFACE`、`CAND-G1-T06-NOVEL-RANKING-SURFACE`、`CAND-G1-T06-RATING-MCP-SURFACE` 登记并交给后续 task；`recommended-all` 的现有 MCP aggregate 已核验，但 SDK aggregate、四流 strict live、compatibility/release 仍未闭合。缺失 surface 未被静默跳过或伪造为 accepted。
- **Forbidden endpoint：** 生产 `internal/mcpserver/pixiv` 源码无 `internal/services/{pixiv,fanbox}` 直连，也无 `/v1/novel/detail`、`/v1/novel/series`、`/v1/novel/content` 调用；`/v1/novel/content` 只在 rejected-path test fixture 中用于证明不可达。
- **Focused evidence：** `go test ./internal/mcpserver/pixiv -count=1 -run '^(TestServerListsExpectedTools|TestEveryToolOutputSchemaOmitsTransportAndCredentialFields|TestFeedRecommendationSchemasMatchLegacyContracts|TestArtworkNovelReadOutputSchemasMatchWireEnvelopes|TestUserReadSchemasMatchLegacyContracts|TestToolErrorResultPreservesStructuredContent|TestToolErrorOutputDoesNotLeakCanary|TestSDKRecommendedAllReturnsEveryStreamAndPagination|TestSDKRecommendedSingleKindsAndInputFailures|TestSDKRecommendedAllFailureDoesNotExposePartialStructuredOutput|TestIllustRankingRejectsInvalidInputBeforeSDKExecution|TestRecommendedKindSelectsArtworkSubtype|TestRecommendedRejectsKindConflictingFiltersBeforeSDKExecution|TestTypedBookmarkSchemasKeepLegacyFieldsClosed|TestTypedBookmarkReadsPreserveEmptyAndTypedErrors|TestBookmarkListAllFailureDoesNotExposePartialRecords|TestBookmarkTagsAllFailureDoesNotExposePartialTags|TestSearchUserSDKFailureRemainsStructured|TestUserDetailSDKFailureRemainsStructured|TestBlockedUsersSDKFailureRemainsStructuredAndDoesNotFallback|TestArtworkNovelReadSDKFailuresPreserveSafeStructuredEnvelopes)$' -v`：PASS。按 task 预算未跑全 legacy replay。
- **下一步：** G1-T12。


## 13. G1-T12 MCP read legacy replay + stdout boundary

- **结论：** PASS，no-op verified。现有 read replay 与 stdio boundary 实现已满足本 task acceptance，没有修改生产代码、tool schema、wire 或日志策略。
- **Legacy replay：** `TestArtworkNovelReadLegacyJSONReplayPreservesStructuredContracts`、`TestCommentReadLegacyJSONReplayPreservesStructuredContracts`、`TestFeedRecommendationLegacyJSONReplayPreservesStructuredContracts`、`TestUserReadLegacyJSONReplayPreservesStructuredContracts` 全部 PASS；覆盖 artwork/novel、feed/recommendation、comments、user/Mypixiv/relationship read，保留 legacy request/structured output/empty/error 契约，并验证 rejected `novel_content` 不触发 `/v1/novel/content`。
- **Stdout/stderr：** `TestMCPStdioKeepsJSONRPCOnStdout` PASS；CLI diagnostics、reverse-search close 与 MCP reverse-search lifetime tests PASS。JSON-RPC 独占 stdout，diagnostics 仅写 stderr，close error 不污染 stdout。
- **Structured error wire：** `TestToolErrorResultPreservesStructuredContent` 与 `TestToolErrorOutputDoesNotLeakCanary` PASS；错误仍为 `isError=true` 且 structured content 安全。
- **Correction/risk：** 无 correction、无新增 blocker；offline replay/stdio evidence 不能替代 strict live、public compatibility、release 或 mutation read-back。
- **Focused evidence：** `go test ./internal/mcpserver/pixiv ...` replay/error/stdout subset 与 `go test ./internal/cli ./internal/cli/commands/pixiv/mcp ...` stdout subset 均 PASS；未重复无关 package 功能测试。
- **下一步：** G1-CHECK-04。

## 14. G1-CHECK-04 Phase B exit：MCP read 完整性 + push

- **检查结论：** PASS。专用 linked worktree、branch=`refactor/pixiv-api-stability`、Phase B G1-T07–G1-T12 evidence、44 个 client-visible tool exact registration、read owner/correction mapping、schema/error、legacy replay 与 stdout/stderr boundary 均已复核；无未分解 read owner、无多余 generalization、无来源不明 diff。
- **Coverage/correction：** user read、typed bookmark read、dual-stream aggregate read 与 legacy/stdout read gate 已有 offline evidence；`ugoira-metadata`、`novel-ranking`、`rating-filter` 继续保留明确 correction owner，`recommended-all` 的 SDK aggregate/strict live/compatibility/release 继续 open；未新增 capability、task 或 scope，缺失 required surface 未被静默跳过或伪造为 accepted。
- **Push gate：** Phase B push-exit ledger checkpoint Local HEAD=`c902b342ddec687a078f6fff99967dfb4689046a`；Remote SHA=`c902b342ddec687a078f6fff99967dfb4689046a`，`gh api` 与 `git ls-remote` 一致；`aa80415f17c49d274b672cf256e63e109ee4ff9e..c902b342ddec687a078f6fff99967dfb4689046a` ordinary fast-forward，未使用 force/rebase。
- **风险：** offline MCP/read evidence 不能替代 strict live、public compatibility、release 或 mutation read-back；无新增 internal/external/decision blocker。
- **下一步：** G1-T14。

## 15. G1-T13 MCP bookmark mutation

- **结论：** PASS。已完成 artwork bookmark mutation 的 MCP 回归复核，并将既有 novel bookmark add/delete leaf 通过 public SDK 暴露为 additive MCP tools；旧 artwork tool 名称、schema 与 wire 保持兼容。
- **Surface：** `add_novel_bookmark` 接受必填正数 `novel_id`、可选 `restrict` 与重复 `tags`；`remove_novel_bookmark` 接受必填正数 `novel_id`。两者输出 `novel_id`，失败保留 `success=false` 与 `isError=true`。
- **Outcome / uncertainty：** success、typed upstream failure、invalid input 与 502 uncertain outcome 均有 offline evidence；`PostForm` 对非 401/403 的 uncertain failure 不自动 replay，未新增 read-back 或猜测性恢复。
- **Focused evidence：** `go test ./sdk/pixiv ./internal/mcpserver/pixiv ./internal/services/pixiv/endpoint/user/novelbookmarks -count=1`、`go test ./scripts/internal/publicapi -count=1`、`go test ./scripts/tests/documentation -count=1` 与 `go test ./... -count=1` 均 PASS；`git diff --check` PASS。
- **风险：** offline wire/status-only evidence 不能替代 strict live、access-control、同账号 read-back/cleanup、release compatibility；无新增 blocker。
- **下一步：** G1-T14。

## 16. G1-T14 MCP artwork comment/stamp mutation

- **结论：** PASS。新增四个 additive artwork-side MCP mutation tools：`create_artwork_comment`、`reply_artwork_comment`、`stamp_artwork_comment`、`delete_artwork_comment`；exact client-visible registry 从 46 增至 50。
- **Input/schema：** 四个 tool 均为 closed object；artwork/comment/parent/stamp/comment ID 按 contract 校验正数或非空 comment；invalid input 在 SDK execution 前拒绝，且不产生 wire call。legacy `illust_comments`/`novel_comments` read contract 未变，也没有新增 standalone `stamps` read tool。
- **Outcome/ID：** create/reply/stamp 直接把 public SDK/upstream response 的 `comment_id` 写入 structured `Mutation` envelope；delete 只使用调用方提供的可靠 `comment_id`。没有“读取最新评论再猜 ID”的逻辑；typed upstream failure 返回 `success=false`、`isError=true` 与安全错误文本。
- **Uncertain/no replay：** 502 upstream failure fixture 验证单次 wire request；handler 不做 mutation replay、换账号重试或写后猜测性恢复。
- **验证：** `go test ./internal/mcpserver/pixiv -run 'TestArtworkCommentMutation' -count=1 -race`、`go test ./internal/mcpserver/pixiv -count=1`、`go test ./sdk/pixiv ./internal/services/pixiv/endpoint/artwork/comments -count=1`、`go test ./scripts/tests/documentation -count=1`、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh` 与 commit hook 均 PASS；`git diff --check` PASS。
- **提交/边界：** implementation commit=`41e4b2dc2ced61662e464a0564917e393ce79fa8`；ledger commits 均为 local-only，worktree clean，remote SHA=`6bf64c208719f4e5dff3c0af7bf93e2630d3d340`，本 task 未 push。当前证据仍是 offline/wire；strict live、access-control、同账号 read-back/cleanup、public compatibility 与 release gate 继续 open，无新增 blocker。
- **下一步：** G1-T15。

## 17. G1-T15 MCP novel comment/stamp mutation

- **结论：** PASS。新增四个 additive novel-side MCP mutation tools：`create_novel_comment`、`reply_novel_comment`、`stamp_novel_comment`、`delete_novel_comment`；exact client-visible registry 从 50 增至 54。
- **Input/schema：** 四个 tool 均为 closed object；novel/comment/parent/stamp/comment ID 按 frozen contract 校验正数或非空 comment；invalid input 在 SDK execution 前拒绝且不产生 wire call。novel comments v2 read surface 未变，也没有新增 standalone `stamps` read tool。
- **Outcome/ID：** create/reply/stamp 使用 `/v1/novel/comment/add` 并直接把 public SDK/upstream response 的 `comment_id` 写入 structured `Mutation` envelope；delete 使用 `/v1/novel/comment/delete`，只接受并返回调用方提供的可靠 `comment_id`。没有“读取最新评论再猜 ID”的逻辑，也不回退 candidate v3。
- **Uncertain/no replay：** 502 typed upstream failure fixture 验证单次 wire request；handler 不做 mutation replay、换账号重试或写后猜测性恢复。失败返回 `success=false`、`isError=true` 与安全错误文本。
- **验证：** Red 的 focused registry/mutation command 在实现前按预期失败；随后 `go test ./internal/mcpserver/pixiv -run 'Test(NovelCommentMutation|ServerListsExpectedTools)' -count=1 -v`、`go test ./internal/mcpserver/pixiv -run 'TestNovelCommentMutation' -count=1 -race`、`go test ./internal/mcpserver/pixiv -count=1`、`go test ./sdk/pixiv ./internal/services/pixiv/endpoint/novel/comments -count=1`、`go test ./scripts/tests/documentation -count=1`、`go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh`、commit hook 与 `git diff --check` 均 PASS。
- **提交/边界：** implementation commit=`484ffee5629299e644b78da18d24345329479613`；ledger commits 均为 local-only，worktree clean，remote SHA=`6bf64c208719f4e5dff3c0af7bf93e2630d3d340`，本 task 未 push。当前证据仍是 offline/wire；strict live、access-control、同账号 read-back/cleanup、public compatibility 与 release gate 继续 open，无新增 blocker。
- **下一步：** G1-CHECK-05。

## 18. G1-CHECK-05 bookmark/comment mutation concentration check

- **结论：** PASS，no-op verified。G1-T13、G1-T14、G1-T15 的 bookmark、artwork comment/stamp、novel comment/stamp mutation 已按 frozen contract 集中复查；未发现需要修正的生产代码、公开 SDK、MCP registration、schema 或文档。
- **ID 来源：** bookmark add/remove 只传递调用方提供的正数 artwork/novel ID；artwork/novel comment create/reply/stamp 的 `comment_id` 直接来自 public SDK/upstream response，并由 endpoint 对正数响应字段做校验；delete 只使用调用方提供的正数 `comment_id`。没有读取最新评论猜 ID。
- **Outcome/error：** 所有 mutation 复用现有 `outputs.Mutation` envelope 与 `RunMutation`/`RunMutationInPlace`；typed upstream failure 保留 `success=false`、MCP `isError=true` 与安全诊断；invalid input 在 SDK/wire 前拒绝。未新增 candidate v3 fallback 或无关 mutation framework。
- **Uncertain/no replay：** 502 typed failure fixtures 对 artwork comment、novel comment 与 novel bookmark 均验证单次 wire request；`runtime.Write` 将 mutation attempt 标为 committed，`Facade.Use` 在账号池边界提交后不再切换账号，符合“不确定结果不自动 retry/replay”约束。相关 focused race tests PASS。
- **Legacy wire/边界：** artwork bookmark legacy wrappers 与四组 bookmark path/form 保持不变；comment/stamp 使用 `/v1/illust/comment/add`、`/v1/illust/comment/delete`、`/v1/novel/comment/add`、`/v1/novel/comment/delete`；MCP mutation tools 仅依赖 public `sdk/pixiv`，未导入 `internal/services/pixiv`。exact client-visible registry 为 54，双语 MCP 文档与 documentation tests 一致。
- **验证：** `go test ./internal/mcpserver/pixiv -run 'Test(SDKMutationTypedErrorIsMCPError|NovelBookmarkMutationTypedErrorIsMCPError|SDKMutationToolsReturnStructuredSuccess|ArtworkCommentMutation|NovelCommentMutation|ServerListsExpectedTools)' -count=1 -race`、SDK mutation focused race tests、`go test ./internal/services/pixiv/endpoint/artwork/bookmark ./internal/services/pixiv/endpoint/artwork/comments ./internal/services/pixiv/endpoint/novel/comments ./internal/services/pixiv/endpoint/stamps ./internal/services/pixiv/endpoint/user/novelbookmarks -count=1`、`go test ./scripts/tests/documentation -count=1` 与 `git diff --check` 均 PASS；此前同一实现 HEAD 的 `go test ./... -count=1`、`go vet ./...`、`sh scripts/build.sh` 也均已 PASS，之后仅变更 ledger。
- **Correction：** 无。
- **风险：** 当前仍只有 offline MCP/wire evidence；strict live、写前 access-control、同账号写后 read-back/cleanup、public compatibility、release 与 Phase C push gate 继续 open。无新增 internal/external/decision blocker。
- **下一步：** G1-T16。

## 19. G1-T16 MCP follow/unfollow mutation

- **结论：** PASS，no-op verified。既有 MCP `follow_user`/`unfollow_user`、public SDK `FollowUser`/`UnfollowUser` 与 `/v1/user/follow/add|delete` wire 已满足 frozen contract；本轮只补齐 acceptance 要求但此前缺失的离线回归，未改生产代码、tool schema、registration 或文档。
- **Uncertain/no replay：** 新增 SDK `TestFollowMutationsDoNotReplayUncertainFailure`：follow/unfollow 对上游 502 各只发一次 wire request 且原样返回错误，不自动 retry/replay，与 `runtime.Write` 提交后不切账号的既有约束一致。
- **Invalid input：** 新增 SDK `TestFollowMutationsRejectInvalidInputBeforeNetwork`：非法 `user_id` 与 unknown `restrict` 在网络前返回 typed `InvalidArgument`（calls=0）；unknown restrict 的网络前拒绝继续由既有 `TestFollowUserRejectsUnknownRestrictBeforeNetwork` 覆盖。
- **MCP typed failure：** 新增 `TestFollowMutationTypedErrorIsMCPError`：typed upstream failure 保持 `isError=true`、`success=false`、`user_id` 回显与 `upstream_error` reason 文本；success/旧 wire 继续由 `TestSDKMutationToolsReturnStructuredSuccess` 与 follow wire handlers 覆盖。
- **验证：** `go test ./sdk/pixiv ./internal/mcpserver/pixiv ./internal/services/pixiv/endpoint/user/follow -count=1`、`go test ./internal/mcpserver/pixiv -run 'TestFollowMutationTypedErrorIsMCPError|TestSDKMutationTypedErrorIsMCPError|TestSDKMutationToolsReturnStructuredSuccess' -count=1 -race`、`go vet ./sdk/pixiv ./internal/mcpserver/pixiv`、`gofmt` 与 `git diff --check` 均 PASS。
- **风险：** 仍只有 offline wire 证据；strict live、写前 access-control、同账号写后 read-back/cleanup、public compatibility、release 与 Phase C push gate 继续 open（G1-T30 及后续 owner）。无新增 internal/external/decision blocker。
- **下一步：** G1-T17。

## 20. G1-T17 shared mutation outcome / uncertainty harness

- **结论：** PASS，no-op verified。definite failure 与 uncertain 分离、不自动 replay、helper 复用三项 acceptance 均由既有 shared 载体与测试满足；本轮零代码改动，未新增通用 mutation framework。
- **Definite/uncertain 分类：** `sdk/pixiv/errors.go classifyStatus` 将 400/403/404/410 映射为 definite typed reason（`InvalidArgument`/`Forbidden`/`NotFound`/`ContentUnavailable`），401/429 映射为 `CredentialsExpired`/`RateLimited` 且只携带显式 `RetryAdvice`，5xx 与 transport error 统一映射为 uncertain 的 `UpstreamError`；各 mutation family 的 502 回归直接证明 uncertain 分类与单次请求语义。
- **Committed 边界：** `internal/mcpserver/pixiv/internal/runtime/runtime.go` 的 `Write` 以 `committed=true` 提交 mutation attempt（`CollectWith` 读路径为 false）；`internal/services/pixiv/facade.go` 的 `Facade.Use` 只在 attempt 未 commit 时允许 pool 切换，`TestSchedulerFailsOverOnlyBeforeCommit`、`TestFacadeUseDoesNotReplayCommittedAttempt`、`TestSchedulerRequiresSafeFutureRetryAfter` 全部 PASS。
- **Helper 复用：** rg 全量核对 14 个 mutation tool（bookmark add/remove ×2、comment create/reply/stamp/delete ×2、follow/unfollow）全部经 `runtime.Write` 与 `outputs.RunMutation`/`RunMutationInPlace`，无 tool 自建 outcome 映射、无绕过账号池边界、无 candidate v3 fallback。
- **验证：** `go test ./internal/services/pixiv ./internal/services/pixiv/pool -run 'TestScheduler|TestFacadeUse' -count=1 -v`、`go test ./sdk/pixiv ./internal/mcpserver/pixiv ./internal/services/pixiv/endpoint/user/follow ./internal/services/pixiv/endpoint/user/novelbookmarks -count=1` 均 PASS。
- **风险：** 仍只有 offline shared 语义证据；strict live、写后 read-back/cleanup、public compatibility 与 release 继续 open（G1-T30 及后续 owner）。无新增 internal/external/decision blocker。
- **下一步：** G1-T18。

## 21. G1-T18 MCP mutation legacy replay / offline read-back contract gate

- **结论：** PASS。旧 mutation request wire、structured error 与 offline read-back contract 全部有当前 HEAD 可复跑证据；未执行任何 live 写入，未新增 abstraction 或自动 read-back。
- **Replay：** MCP wire responder 覆盖全部 mutation endpoint 的精确 path/form（artwork/novel bookmark、artwork/novel comment、follow 的 add/delete）；`TestSDKMutationToolsReturnStructuredSuccess` 保持 legacy tool 名与输入字段；五组 typed-error no-replay 测试（SDK mutation、novel bookmark、follow、artwork/novel comment）证明 structured error 保持 `isError=true`/`success=false` 且单次请求。
- **Read-back contract：** 新增 `TestBookmarkMutationReadBackOrchestrationOffline`：同一 public client 上 add→detail read-back 确认→remove→detail 确认恢复，精确断言 wire 序列与读取到的状态（`tags` 出现/消失）；证明 read-back 编排可离线测试，且状态确认来自 read 结果而非 mutation 2xx。编排仍由调用方组合既有 public read 操作，live read-back/cleanup 留给 G1-T30。
- **Uncertain 边界：** 四个 mutation family 的 502 单次请求回归继续成立；offline fixture 成功不宣称 live success。
- **验证：** `go test ./internal/mcpserver/pixiv -run '^(TestSDKMutationToolsReturnStructuredSuccess|TestSDKMutationTypedErrorIsMCPError|TestNovelBookmarkMutationTypedErrorIsMCPError|TestFollowMutationTypedErrorIsMCPError|TestArtworkCommentMutationTypedErrorIsMCPErrorAndDoesNotReplay|TestNovelCommentMutationTypedErrorIsMCPErrorAndDoesNotReplay)$' -count=1`、`go test ./sdk/pixiv ./internal/services/pixiv/endpoint/artwork/bookmark ./internal/services/pixiv/endpoint/user/novelbookmarks ./internal/services/pixiv/endpoint/user/follow -count=1`、`go vet ./sdk/pixiv`、`gofmt` 与 `git diff --check` 均 PASS；commit hook 全量 `go test ./...` PASS。
- **Correction：** 无。
- **风险：** 仍只有 offline 证据；strict live、写前 access-control、同账号写后 read-back/cleanup、public compatibility 与 release 继续 open。无新增 internal/external/decision blocker。
- **下一步：** G1-CHECK-06。

## 22. G1-CHECK-06 Phase C exit：MCP mutation 完整性 + push

- **检查结论：** PASS。G1-T13–T18 全部 verified；novel/artwork bookmark mutation、artwork/novel comment/stamp mutation、follow/unfollow mutation 的 MCP/offline 责任链关闭，无未分解 mutation owner、无抢占式 correction、无来源不明 diff。专用 linked worktree 与 branch=`refactor/pixiv-api-stability` 有效。
- **Verification：** push-exit HEAD 复跑 shared harness（`TestScheduler|TestFacadeUse`）、8 个 mutation 相关 package 回归与 MCP focused mutation/registration `-race` set，全部 PASS；各实现提交时 commit hook 全量 `go test ./...` PASS。
- **Live boundary：** Phase C 只关闭 offline MCP/wire/编排 contract；strict live、access-control、同账号 read-back/cleanup 按 manifest 留给 G1-T30，未被 offline fixture 伪装为已验证。
- **Push gate：** Local HEAD=`5a057ad6f7510c7c005c22974012b830152e3ed9`；Remote SHA=`5a057ad6f7510c7c005c22974012b830152e3ed9`（`git ls-remote` 与 `gh api` 双重核验一致）；`6bf64c2..5a057ad` ordinary fast-forward，未使用 force/rebase。
- **风险：** offline 证据不替代 public compatibility、release、cursor/docs gate 与 live；Phase D–F owner 继续 open。无新增 internal/external/decision blocker。
- **下一步：** G1-T19（Phase D）。

## 23. G1-T19 cursor integrity / binding / rollback gate

- **结论：** PASS，no-op verified。`sdk` 通用 Cursor、`sdk/pixiv` 产品绑定与 shared pagination/traversal 的既有实现与测试已满足全部 acceptance；本轮零代码改动。
- **Integrity：** `cursorEnvelope` 为封闭 typed 结构，无凭据/cookie/token/signed URL/raw next_url/原始内容承载字段；payload 唯一来源为 `continuationEnvelope{Key,Value,Consumed}`；`TestSearchArtworksCheckpointRejectsChangedBindings` 行为级断言 payload 不含查询词与 CursorContext；`TestCursorTextIsRouteSafe` 保证文本可持久化/可日志。
- **Binding/rollback：** product/op/binding version/query digest 全量校验，mismatch 一律显式 `InvalidCursor`；identity-scoped ops 绑定 verified identity 或 ephemeral instance；binding version 篡改的受控失败测试（b=2→1 → InvalidCursor）即跨版本 rollback gate；format version 不识别同样 fail-closed；不存在静默第一页重启路径。
- **Checkpoint/batch：** shared pagination（skip/OneBatch/exact limit/repeated cursor/cycle/cancel/filtered continuation replay）与 traversal（commit boundary、uncommitted replay 清空）、MCP runtime safe-replay filter reset、search checkpoint round trip/changed bindings/verified account 均有针对性回归。
- **验证：** `go test ./sdk ./sdk/pixiv ./internal/shared/pagination ./internal/shared/traversal ./internal/mcpserver/pixiv/internal/runtime ./internal/cli/commands/pixiv/internal/listing -count=1`、`go vet ./sdk ./sdk/pixiv ./internal/shared/pagination ./internal/shared/traversal`、`git diff --check` 均 PASS。
- **风险：** 离线证据不替代 endpoint live continuation（Phase F）；aggregate cursor 不对外暴露。无新增 internal/external/decision blocker。
- **下一步：** G1-T20。

## 24. G1-T20 public Go SDK compatibility

- **结论：** PASS，no-op verified。对照旧 T12 symbol map 无实际 compatibility 差异需要收敛；无 breaking，未触发 `blocked_decision`。
- **Surface：** `TestRepositoryPublicAPIInventoryIsPinned` 钉死 digest PASS；自 T12 起 exported surface 唯一变化为 G1-T13 additive novel bookmark operations（当时已随 public API review 更新 digest）；无 symbol/model/named field/enum 删除或重命名。
- **Old consumer：** `TestLegacySDKConsumerCompiles` PASS（旧 interface + request 字面量编译）；`TestAddBookmarkWiresMutation` 保留 legacy wrapper；`TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint` 证明 excluded endpoint 兼容入口零网络且返回 `ContentUnavailable`。
- **Cursor/filter：** `TestSearchNovelsAndUsersCursorsArePublicScoped`、`TestSearchArtworksCheckpointRoundTrip` 与 G1-T19 证据承接已冻结 cursor/filter 语义。
- **Aggregate SDK surface：** #23/#24/#41 的 aggregate 在旧 T12 symbol map 中无 public SDK operation，contract 权威为 cli-migration-matrix 产品层聚合命令；SDK aggregate 仍如实记录为 `missing`，不因 CLI/MCP 聚合已实现而当作已接受，capability verdict 由 G1-FINAL 重算裁决。
- **验证：** `go test ./scripts/internal/publicapi -count=1`、SDK focused compat set、`go test ./sdk ./sdk/pixiv -count=1`、`go vet ./sdk/pixiv` 均 PASS。
- **风险：** live/compatibility/release gate 继续 open。无新增 internal/external/decision blocker。
- **下一步：** G1-T21。

## 25. G1-T21 CLI compatibility + presentation

- **结论：** PASS。Red→Green 补齐 frozen map row 35 的 `bookmark add/remove` 双 namespace；其余 CLI surface 经复核与 frozen map 一致；无 breaking、无重设计。
- **实现：** `bookmark add/remove` 新增 `--type/-t`（默认 `artwork`）；novel dispatch 走 public SDK；`--type all/user/unknown` 在网络前以 resolver 同措辞拒绝；record 消费按 namespace 分离；help/usage 改 typed 名。Red 阶段 5 个新测试实跑全部因 `unknown flag: --type` 失败。
- **Trending（#35）：** CLI surface 复核为 `pixiv search --trending-tags`（frozen row 110；`search.go` 含 exactArgs(0)、`--type/--content-type` 冲突拒绝、`/v1/trending-tags/illust` wire tests）；state matrix #35 的 stale `CLI=missing` 已纠正为 `verified`。
- **Ugoira metadata（#6）：** frozen cli-migration-matrix 无 metadata CLI 路由（Goal-3 T07A 仅 adapter/SDK）；CLI 缺口保持显式登记于 `CAND-G1-T06-UGOIRA-SURFACE`，不静默当作已解决，是否新增 additive 命令留 G1-FINAL 裁决。
- **Bare-ID/rating（#38/#39）：** explicit `--type` 边界与无隐式 probe 由 resolver/CLI 负向 tests 覆盖；`search --rating` 为本地过滤并绑定 cursor digest，不发 server-side rating。
- **验证：** `go test ./internal/cli/commands/pixiv/{bookmark,search,follow,comment} ./internal/cli ./sdk/pixiv -count=1`、`go test ./scripts/tests/documentation -count=1`、`go vet`、`gofmt` 均 PASS；commit hook 全量 `go test ./...` PASS。
- **文档：** `--type` 的双语 cli-reference 与 `skills/pixiv-cli` 同步 pending，由 G1-T23 统一执行。
- **风险：** 无新增 internal/external/decision blocker。
- **下一步：** G1-T22。

## 26. G1-CHECK-07 cursor + SDK + CLI concentration check

- **检查结论：** PASS。G1-T19（cursor safety/rollback）、G1-T20（source compatibility）、G1-T21（CLI presentation）在当前 HEAD 复核并复跑组合回归通过；无 over-refactor、无 debug 残留、无 breaking blocker。
- **Cursor：** 封闭 envelope、payload canary、fail-closed 版本、无静默重启、checkpoint/replay/cancel 回归（T19 证据）。
- **SDK：** pinned inventory、old consumer 编译、legacy wrapper、NovelContent 零网络、additive-only（T20 证据）。
- **CLI：** frozen row 35 双 namespace 闭合、默认 artwork 旧行为保持、namespace 冲突网络前拒绝、trending verdict 纠正、ugoira 缺口显式登记、local rating 本地语义（T21 证据）；T21 diff 仅 bookmark.go 68 行 + 测试 162 行。
- **验证：** 8 包组合回归（shared pagination/traversal、sdk、sdk/pixiv、bookmark、search、internal/cli、publicapi）全部 PASS。
- **Correction：** 无。**Decision blocker：** 无。
- **下一步：** G1-T22。

## 27. G1-T22 MCP compatibility audit

- **结论：** PASS，no-op verified。对照旧 T39A 无真实 wire 差异需要修复；无 breaking，未触发 `blocked_decision`。
- **Exact-set：** 当前 54 个 client-visible tool = T39A 冻结 40（download/reverse-search/全部 read 与 legacy mutation 名称）+ 14 个有账本记录的 additive（T09/T10/T13/T14/T15）；`TestServerListsExpectedTools` PASS，旧 tool 无删除/静默改名。
- **Schema/replay：** output schema 四组 legacy match、四组 read legacy JSON replay harness、mutation structured success 与 typed-error no-replay 测试、transport/credential 字段防泄漏 schema walk——focused 19 项全 PASS。
- **Additive owner：** `ugoira-metadata`、`novel-ranking` surface 仍缺失并继续由 correction candidate 登记所属（静态核查 registry 无此二 tool，未被静默跳过）；`recommended-all` 四流 aggregate 存在且有页原子失败 tests。
- **Rating/rejected path：** MCP 无 rating input（frozen local semantics 仅 CLI）；生产 MCP 无 rejected endpoint 调用；`illust_comments` 保持 legacy v3 wire、无 fallback 注册；`novel_content` structured unsupported 零网络。
- **验证：** `go test ./internal/mcpserver/pixiv -count=1 -run '^(19 项 compat/replay focused set)$'` 与 7 项核心 subset 两轮实跑 PASS。
- **风险：** missing additive surface 与 live/compatibility/release gates 继续 open。无新增 internal/external/decision blocker。
- **下一步：** G1-T23。

## 28. G1-T23 docs / Skill / changelog synchronization

- **结论：** PASS。仅同步真实已实现 surface（G1-T21 的 `bookmark add/remove --type` CLI namespace 扩展），未文档化任何未实现或未 live-verified 的能力。
- **文档改动：** 双语 cli-reference（quick examples、Record 消费 namespace 说明、命令表、`--type/-t` flags 行）；`skills/pixiv-cli/SKILL.md` 示例改 typed 名；`changelog/unreleased/{en,zh-CN}.md` Added 条目（双语一致，说明默认 artwork 行为保持、novel 走 public SDK wire、网络前 namespace 拒绝、Record namespace 过滤）。
- **未改：** README、MCP docs、SDK docs（T13–T15 已同步且本轮无 MCP/SDK 变化）；ugoira/novel-ranking/rating 缺失 surface 继续保持文档沉默。
- **验证：** `go test ./scripts/tests/documentation -count=1`、`TestBookmarkHelpUsesTypedTargetNames`、`ILLUST_ID` 残留扫描（空）、`git diff --check` 均 PASS。
- **风险：** 无新增 internal/external/decision blocker。
- **下一步：** G1-T24。

## 29. G1-T24 forbidden endpoint / no-fallback release contract gate

- **结论：** PASS。rejected endpoint 与 fallback 禁令在 required public paths 上成立；静态扫描与负向回归均有当前 HEAD 证据；未新增重复 fixture。
- **v1 detail/series（#8/#9）：** 生产树无 `/v1/novel/detail|series`；protocol 常量与 endpoint tests 锁定 `/v2/novel/detail`、`/v2/novel/series`。
- **`/v1/novel/content` + WebView（#25/legacy）：** SDK `NovelContent` deprecated 零网络、MCP `novel_content` structured unsupported 零网络（focused tests PASS）；唯一消费点为 legacy `novel_image`/`novel_file` resource resolve（不属于 41 required capabilities surface，ref 铸造链自闭合，CLI download 不接受 novel 来源），上游 404 受控失败；登记 out-of-scope observation，移除该兼容面属 breaking 需用户决策。
- **bare-ID（#38）**：resolver explicit-type 边界与无隐式 probe 回归 PASS。**rating/x_restrict（#39）**：searchfilter 本地规范化 PASS，rating 不写入 upstream query。
- **验证：** 6 组负向/package 回归实跑 PASS（sdk/pixiv focused、internal/mcpserver/pixiv focused、novel detail/series endpoint、resolver、searchfilter）+ 生产树静态扫描。
- **Correction：** 无。**下一步：** G1-CHECK-08（Phase D exit + push）。

## 30. G1-CHECK-08 Phase D exit：compatibility/docs/release contract + push

- **检查结论：** PASS。G1-T19–T24 与 G1-CHECK-07 全部 verified；cursor/SDK/CLI/MCP/docs/forbidden 六项 contract 在 push-exit HEAD 复跑代表回归全部 PASS；无未决内部差异、无新增 correction/blocker；专用 linked worktree 干净。
- **Push gate：** Local HEAD=`c4a99ec4ecb3646bced231d493db94d641405969`；Remote SHA=`c4a99ec4ecb3646bced231d493db94d641405969`（`git ls-remote` 与 `gh api` 双重核验一致）；`5bfb29c..c4a99ec` ordinary fast-forward，未使用 force/rebase。
- **风险：** offline/compat 证据不替代 live（Phase F）；missing additive surface（ugoira/novel-ranking/rating）与 legacy novel resource observation 继续 open。无新增 internal/external/decision blocker。
- **下一步：** G1-T25（Phase E）。

## 31. G1-T25 protocol / endpoint / SDK regression

- **结论：** PASS。`go test ./internal/services/pixiv/... ./sdk/... -count=1`：40 个有测试 package 全部 ok，0 FAIL；覆盖 required/optional/null/empty/error fixtures、adapter↔SDK 映射、continuation allowlist/binding、old consumer 编译与 rejected endpoint 负向回归（focused 复跑确认）。
- **预算遵守：** 未重复 full CLI/MCP（G1-T26 owner）；race 留给 G1-T27 按需执行。
- **Correction：** 无。**下一步：** G1-T26。

## 32. G1-T26 CLI + MCP regression

- **结论：** PASS。`go test ./internal/cli/... -count=1` 28/28、`go test ./internal/mcpserver/... -count=1` 10/10 全部 ok，0 FAIL；focused replay/stdout/exact-set subset 6/6 PASS。
- **覆盖：** CLI JSON/NDJSON/stdin 补值/`--on-error` skip|fail-fast/typed cursor/legacy alias/route 组合校验；MCP closed input schema/structured error/exact 54-tool registration/四组 read legacy JSON replay/stdio JSON-RPC stdout 独占；bookmark 双流 aggregate 页原子失败与全部 14 个 mutation tool 的 offline safety（502 单请求、namespace record 过滤、uncertain 不 replay）。
- **预算遵守：** 未重复 protocol/SDK suite（G1-T25）；race 留给 G1-T27。
- **Correction：** 无。**下一步：** G1-T27。

## 33. G1-T27 full offline build / quality / race-as-needed / redaction gate

- **结论：** PASS。`go test ./...` 147 包 PASS（0 FAIL）；`go vet ./...` clean；`sh scripts/build.sh` 产出 `build/pixiv`；race 5 包（mcpserver/pixiv、runtime、sdk/pixiv、services/pixiv、pool）PASS；redaction focused（error canary、schema credential walk、cursor route-safe、payload canary）PASS；`goal-1` 账本静态 secret 扫描 clean。
- **Race rationale：** 仅 Goal 修改过且具并发语义的 package（MCP mutation/runtime、SDK mutation、facade/pool attempt-commit），沿 release contract 既有 race 要求集，未为未改动 package 追加。
- **引用证据：** docs/completion、compatibility replay、forbidden endpoint gates 在同一代码 HEAD 已通过且未被 invalidated，未重复运行昂贵 gate。
- **Correction：** 无。**下一步：** G1-CHECK-09（Phase E exit + push）。

## 34. G1-CHECK-09 Phase E exit：offline release candidate + push

- **检查结论：** PASS。required=41、unmapped=0、undecomposed=0、mapped_to_task=41；matrix 39 个 `missing` 与 150 个 `implemented_unverified` 单元逐类归因（Live pending / Release 终局 gate / correction registry / 契约裁决 / G1-FINAL 重算），内部可离线闭合缺口=0、无理由未验证单元=0；offline gates（T25/T26/T27）全 PASS；离线可闭合 open P0=0、P1=0，P1×1（#4 recommended continuation）live-dependent、显式保留于 live manifest 并绑定 G1-T28/G1-CHECK-10。
- **Push gate：** Local HEAD=`2b465546067b45d45488d93d275a8def3fe82979`；Remote SHA=`2b465546067b45d45488d93d275a8def3fe82979`（`git ls-remote` 与 `gh api` 双重核验一致）；`740a09c..2b46554` ordinary fast-forward，未使用 force/rebase。
- **风险：** live manifest 36 项 required scenario 未执行（Phase F）；missing surface 的 scope 裁决留 G1-FINAL。无新增 internal/external/decision blocker。
- **下一步：** G1-T28。

## 35. G1-T28 live read：artwork / novel / feed

- **执行环境：** 已认证本地账号（凭据仅进程内读取/回写轮换）；直连超时，经 `PIXIV_E2E_PROXY=http://127.0.0.1:7890` 执行（AGENTS.md §2.2 允许的临时代理）；新增 `e2e/sdk_pixiv_live_manifest_test.go` manifest 场景 harness（env 门控、默认 skip）。
- **Live PASS（脱敏计数）：** #1 search all/illust/manga/ugoira 均 30→30 两页零重复；#2 latest illust/manga 两页；#3 ranking/day 两页零重复；#6 ugoira metadata（公共 artwork 149551069：frames=87、archives=1）；#7 novel search 30→30；#8 novel detail ok；#10 novel latest 两页；#12 novel ranking 两页；#13 novel follow 9→0（continuation 存在、第二页空）；#41 user recommended stream 30 项有 continuation。
- **Live 失败 → correction：** #4/#11 recommended 首页即 `malformed_upstream_response`；脱敏诊断确认响应结构合法（illusts 85 项、novels 31 项、id/user.id 全正、next_url 非空），根因为 live `next_url` 多参数续页集（artwork：min_bookmark_id_for_recent_illust/max_bookmark_id_for_recommend/offset=0/viewed[]；novel：offset/already_recommended/max_bookmark_id_for_recommend）vs adapter 单 offset allowlist。注册 `G1-CORR-G1-T28-RECOMMENDED-01`（含 Red fixture 方案、binding version fail-closed、兼容影响与回滚边界）；P1 根因落定并升级为首页失败。
- **blocked_external (data)：** #5 artwork-series、#9 novel-series——production surface 无 series 引用可安全构造目标 ID；correction candidate 保留。
- **Live verdict 更新：** #1/#2/#3/#6/#7/#8/#10/#12/#13 Live → verified；#4/#11/#41(artwork/novel 流) Live 保持未验证（correction owner）；#5/#9 Live=blocked_external (data)。
- **Correction：** G1-CORR-G1-T28-RECOMMENDED-01（pending，抢占下一轮）。**下一步：** G1-CORR-G1-T28-RECOMMENDED-01。

## 36. G1-CORR-G1-T28-RECOMMENDED-01 recommended 多参数 continuation 收敛

- **结论：** PASS。Red→Green→live 三段证据齐备；P1（#4 artwork-recommended continuation）正式闭合；#4/#11 Live verdict → verified。
- **根因与修复：** live `next_url` 为多参数续页集；`continuation.ParseParams`（精确键 + `IgnoredKeyPrefixes`）整体提取；adapter Request/Result 参数化；cursor envelope `Params` 结构化存储（无 raw next_url/凭据）；`RecommendedArtworks`/`RecommendedNovels` binding v2 fail-closed（legacy cursor fixture → InvalidCursor PASS）。
- **viewed[] 裁定：** live 实验矩阵（dropviewed 200/90、onlyoffset 200/85、含 viewed 各态 400、顺序无关）证明 viewed[] 回放被上游 400 拒绝，按 `IgnoredKeyPrefixes` 剔除；novel `already_recommended` 回放成立。
- **Live：** `TestRealPixivSDKLiveManifestRead` 全场景 PASS——#4 89→89、#11 31→31 零重复、#41 user 流 30 项；搜索/排行/最新等确定性序列跨页零重复。
- **验证：** 35 包 focused 回归 PASS（sdk、sdk/pixiv、全部 endpoint、mcpserver、cli recommended、shared pagination/traversal）；vet/gofmt/diff-check PASS；commit hook 全量 `go test ./...` PASS。
- **风险：** recommended cursor 较大（含 viewed 以外的参数集与 already_recommended csv），仍在 opaque cursor 预算内；无新增 internal/external/decision blocker。
- **下一步：** G1-T29。

## 37. G1-T29 live read：bookmark / comments / user

- **执行环境：** 与 G1-T28 相同（同账号、代理、env 门控 harness 扩展）。
- **Live PASS：** #14 bookmarks/illust public 30(continuation)/private 1；#15 tags illust 30、novel 0（空合法）；#18 novel bookmarks 空（合法）；#29 stamps 40；#30/#31 user artworks/novels self 0（合法空）；#32 following 29→29 零重复、followers/blocked 空（合法）；#33 user detail + CurrentUser 身份一致；#34 search user 18；#35 trending 40 tags + sample 合法；#37 mypixiv 三流 0（合法空）；#23/#24 CLI `--type all` 聚合 live（真实 binary 子进程）：list records=2 artwork_side=true、tags typed 输出，novel 流空为 manifest 允许。
- **blocked_external (data)：** #27 novel comments——扫描前 3 本搜索小说均 0 comments（目标数据不存在）；#20 bookmarked case——账号无 novel bookmarks。
- **Live 失败 → correction：** #16/#20 absent case live `malformed_upstream_response`（响应 200 + is_bookmarked:false + 作品 tags；adapter 过严与冻结契约冲突）；注册并完成 `G1-CORR-G1-T29-BOOKMARK-DETAIL-01`。
- **Live verdict 更新：** #14/#15/#16/#18/#19/#23/#24/#29/#30/#31/#32/#33/#34/#35/#37 Live → verified；#20 absent case PASS、bookmarked=true 因账号无 novel bookmark 数据为 `blocked_external (data)`；#27 Live=blocked_external (data)。
- **Correction：** `G1-CORR-G1-T29-BOOKMARK-DETAIL-01` 已完成；两 endpoint 对 false 状态忽略 restrict/作品 tags 并返回 non-nil empty tags；bookmarked=true、null、404 与其他错误路径回归保持。**下一步：** G1-T30。

## 38. G1-CORR-G1-T29-BOOKMARK-DETAIL-01 bookmark detail absent 归一修正

- **结论：** PASS。live 观察到的 `is_bookmarked:false` + 作品自身 `tags` 不再被误判为 malformed；artwork 与 novel adapter 均按冻结契约归一为 `Restrict:""`、non-nil empty `Tags`。
- **验证：** Red focused fixture 在修正前实跑失败；Green focused/endpoint/SDK tests、`gopls check`、`gofmt`、`git diff --check` 全 PASS；`TestRealPixivSDKLiveManifestBookmarkUserRead` 经 `127.0.0.1:7890` 代理 PASS，artwork/novel absent 均 `bookmarked=false,tags=0`。
- **边界：** 无 wire/public symbol/CLI/MCP schema 变化；novel bookmarked=true 目标数据不存在，保留 `blocked_external (data)`，不升级为 live verified。无新 correction。
