# Goal-1 当前状态：Capabilities 1–41 baseline inventory

> 本文件覆盖 `G1-T01`、`G1-T02`、`G1-T03`、`G1-CHECK-01` 与 `G1-T04`，只记录当前分支的代码、测试、历史和 Goal-3 证据，不授予任何 capability 的发布资格。

## 1. 快照与状态口径

- 执行分支：`refactor/pixiv-api-stability`
- G1-T03 inventory 起始 commit：`35dd0d7efebd16eeda6d5fb8ac3e766b31f8461d`，继承基线：`e40443495981cdaf01215d6711cb24fa618b087a`
- 当前 worktree：`/Users/flanchan/Developer/Projects/GithubProjects/.worktrees/pixiv-cli-refactor-pixiv-api-stability`
- G1-T03 开始时 worktree 干净；G1-CHECK-01 检查时 HEAD 为 `042c0200ce1b3349ab3e08a744fea7802838025c`。G1-T04 审计起始 HEAD 为 `f7cff3fb86dcc5117c634359bedbea5abdfa4d0b`，相对 `origin/refactor/pixiv-api-stability` 领先 5 个 Goal-1 tracking commit；worktree 干净。与继承基线相比，当前分支只新增 Goal-1 tracking 文件，`goal-3/` 无 diff。
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
- 当前未发现新的业务/API/evidence drift；Goal-1 分支相对继承基线的 drift 只有执行资料与 G1-T01/G1-T02/G1-T03/G1-CHECK-01/G1-T04 记录。

### 2.2 Layer matrix

| # | Capability | Contract | Adapter | SDK | Shared | CLI | MCP | Offline | Live | Compatibility | Release |
|---:|---|---|---|---|---|---|---|---|---|---|---|
| 1 | `artwork-search` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 2 | `artwork-latest` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 3 | `artwork-ranking` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | verified | implemented_unverified | rejected |
| 4 | `artwork-recommended` | implemented_unverified | verified | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | verified | implemented_unverified | missing | rejected |
| 5 | `artwork-series` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 6 | `ugoira-metadata` | implemented_unverified | verified | implemented_unverified | verified | missing | missing | verified | implemented_unverified | implemented_unverified | rejected |
| 7 | `novel-search` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | verified | missing |
| 8 | `novel-detail` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 9 | `novel-series` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 10 | `novel-latest` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 11 | `novel-recommended` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 12 | `novel-ranking` | implemented_unverified | verified | verified | verified | verified | missing | verified | implemented_unverified | implemented_unverified | missing |
| 13 | `novel-follow` | implemented_unverified | verified | verified | verified | verified | verified | verified | verified | implemented_unverified | missing |
| 14 | `artwork-bookmark-list` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 15 | `artwork-bookmark-tags` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 16 | `artwork-bookmark-detail` | implemented_unverified | verified | verified | not_applicable | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 17 | `artwork-bookmark-mutation` | implemented_unverified | verified | verified | not_applicable | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 18 | `novel-bookmark-list` | implemented_unverified | implemented_unverified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 19 | `novel-bookmark-tags` | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | verified | missing | verified | implemented_unverified | missing | missing |
| 20 | `novel-bookmark-detail` | implemented_unverified | implemented_unverified | implemented_unverified | not_applicable | verified | missing | verified | implemented_unverified | implemented_unverified | missing |
| 21 | `novel-bookmark-mutation` | implemented_unverified | implemented_unverified | missing | not_applicable | missing | missing | verified | missing | missing | missing |
| 22 | `bookmark-subtype` | implemented_unverified | missing | missing | implemented_unverified | verified | missing | verified | missing | implemented_unverified | missing |
| 23 | `bookmark-list-all` | implemented_unverified | verified | missing | verified | verified | missing | verified | implemented_unverified | missing | missing |
| 24 | `bookmark-tags-all` | implemented_unverified | verified | missing | verified | verified | missing | verified | implemented_unverified | missing | missing |
| 25 | `artwork-comments-read` | rejected | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | implemented_unverified | verified | rejected | implemented_unverified | rejected |
| 26 | `artwork-comments-mutation` | implemented_unverified | verified | verified | verified | verified | missing | verified | implemented_unverified | implemented_unverified | missing |
| 27 | `novel-comments-read` | implemented_unverified | verified | verified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | missing |
| 28 | `novel-comments-mutation` | implemented_unverified | verified | verified | verified | verified | missing | verified | implemented_unverified | implemented_unverified | missing |
| 29 | `stamps` | implemented_unverified | verified | verified | verified | verified | missing | verified | implemented_unverified | implemented_unverified | missing |
| 30 | `user-artworks` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 31 | `user-novels` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 32 | `user-relationships` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 33 | `user-detail` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 34 | `user-search` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 35 | `trending` | implemented_unverified | verified | implemented_unverified | verified | missing | verified | verified | implemented_unverified | implemented_unverified | rejected |
| 36 | `follow-mutation` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | missing | implemented_unverified | rejected |
| 37 | `mypixiv` | implemented_unverified | verified | implemented_unverified | verified | verified | verified | verified | implemented_unverified | implemented_unverified | rejected |
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
- **Live — `verified`（upstream/adapter/SDK case）**：HTTP 200、33→33、offset、wire/response/pagination/adapter/SDK confirmed（`goal-3/evidence/appapi-upstream.md:13`）。
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
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/user_bookmarks/user_bookmarks.go:16-61` 暴露 `user_bookmarks`，支持 user/restrict/tag/page/limit，并复用 `runtime.CollectWith`。
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
- **MCP — `verified`（artwork leaf only）**：`internal/mcpserver/pixiv/tools/bookmark_tags/bookmark_tags.go:15-53` 只注册 artwork `bookmark_tags`；不能横向证明 novel tags 或 all tags。
- **Offline — `verified`**：required-list、empty page、cursor/query、CLI output 与 aggregate failure 有 fixture。（具体证据索引：本文件:259-264）
- **Live — `implemented_unverified`**：strict tag wire/subtype/live continuation 尚未完成；pagination exemption 不等于 contract/public-ready。
- **Compatibility — `implemented_unverified`**：legacy artwork tag tool 保持；novel/all typed output 与 public cursor 仍未冻结。
- **Release — `missing`**：未通过完整 capability gate。

### 4.4 `artwork-bookmark-detail`（#16）

- **Contract — `implemented_unverified`**：目标为 `GET /v2/illust/bookmark/detail`，正 `illust_id`，无分页；未收藏归一为空 restrict 与 non-nil empty tags，404/null/明确 absent 只在此 endpoint 归一（`goal-3/upstream-contract-matrix.md:131`）。
- **Adapter — `verified`**：`bookmark.go:112-145` 仅转换该 endpoint 的 404/null/absent；absent 携带 restrict/tag 时判 malformed，其他错误原样传播。
- **SDK — `verified`**：`sdk/pixiv/ops_artwork.go:308-317` 提供 `ArtworkBookmark`；request 在 `sdk/pixiv/request.go:254-258`，typed model 在 `sdk/pixiv/models.go:258-263`。
- **Shared — `not_applicable`**：detail leaf 无分页/聚合 orchestration；仍需公共 DTO/error gate。
- **CLI — `verified`**：`bookmark.go:136-207` 支持 artwork/novel namespace，artwork detail 输出 text/JSON。
- **MCP — `verified`（artwork leaf only）**：`internal/mcpserver/pixiv/tools/bookmark_detail/bookmark_detail.go:15-37` 仅接受 positive `illust_id`，返回 `BookmarkDetail` envelope。
- **Offline — `verified`**：detail absent/bookmarked/malformed/transport cases 有 endpoint/SDK/CLI/MCP coverage。（具体证据索引：本文件:272-277）
- **Live — `implemented_unverified`**：现有 wire fixture 不等于当前 strict live/public evidence。
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
- **Compatibility — `implemented_unverified`**：legacy artwork wrapper 仍需保持；novel mutation public API/wire 尚未冻结。
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
- **MCP — `missing`**：现有 `bookmark_tags` 只调用 artwork tags；`user_novel_bookmarks` 不返回 tags（`bookmark_tags.go:15-53`、`user_novel_bookmarks.go:38-53`）。
- **Offline — `verified`（candidate）**：endpoint/SDK/CLI tests 覆盖 query、empty、malformed、unsupported continuation；candidate 不代替 strict live。（具体证据索引：本文件:312-315）
- **Live — `implemented_unverified`**：`goal-3/upstream-contract-matrix.md:325` 与 `api-migration-verification.md:30-33` 均记 `not_tested`。
- **Compatibility — `missing`**：无 novel-specific MCP schema/fixture；CLI/SDK additive candidate 尚未纳入公共兼容矩阵。
- **Release — `missing`**：未达 public-ready。

### 4.8 `novel-bookmark-detail`（#20）

- **Contract — `implemented_unverified`**：candidate `GET /v2/novel/bookmark/detail`，正 `novel_id`，无分页；absent/404 normalization、tags shape、错误映射需 T08 snapshot（`upstream-contract-matrix.md:132,326`）。
- **Adapter — `implemented_unverified`**：`novelbookmarks.go:123-158` 已有 candidate normalized absent/bookmarked/malformed 逻辑，代码注释明确尚未通过 live wire/SDK gate。
- **SDK — `implemented_unverified`**：`sdk/pixiv/ops_novel.go:283-295` 与 `request.go:260-264` 提供 candidate `NovelBookmark`；model 为 `NovelBookmarkDetail`（`sdk/pixiv/models.go:265-270`）。
- **Shared — `not_applicable`**：detail leaf 无分页；公共 error/DTO/compat 仍未闭合。
- **CLI — `verified`**：`bookmark.go:136-207,169-188` 已按 resolver 分发 novel detail，并输出 novel DTO。
- **MCP — `missing`**：`bookmark_detail` schema 只接受 `illust_id`，无 novel-specific tool。
- **Offline — `verified`（candidate）**：adapter absent/404/malformed、SDK invalid input/DTO copy 有测试（`novelbookmarks_test.go:228-264`、`sdk/pixiv/pixiv_test.go:114-133,135-182`）。
- **Live — `implemented_unverified`**：`upstream-contract-matrix.md:326` 与 `api-migration-verification.md:31-32` 明确未测试。
- **Compatibility — `implemented_unverified`**：CLI/docs 记录 novel detail，但 MCP legacy contract 仍 artwork-only；未形成 novel public wire。
- **Release — `missing`**：未达 public-ready。

### 4.9 `novel-bookmark-mutation`（#21）

- **Contract — `implemented_unverified`**：candidate add/delete paths 为 `/v2/novel/bookmark/add` 与 `/v1/novel/bookmark/delete`；正式 contract 必须包含 list/tags/detail read-back、删除后恢复与 uncertain 分类（`upstream-contract-matrix.md:133-134,327-328`）。
- **Adapter — `implemented_unverified`**：`novelbookmarks.go:160-198` 只提供 candidate transport leaf；注释明确 2xx 不证明状态改变，remove 后 read-back/restore 留给后续验证。
- **SDK — `missing`**：当前 `sdk/pixiv` 没有 `AddNovelBookmark`/`RemoveNovelBookmark` public operation；只有 read candidate。
- **Shared — `not_applicable`**：没有 novel mutation orchestration。
- **CLI — `missing`**：`bookmark add/remove` 仍 artwork-only（`bookmark.go:245-321`）。
- **MCP — `missing`**：`add_bookmark`/`remove_bookmark` schema 与 handler 只支持 artwork。
- **Offline — `verified`（transport only）**：candidate endpoint tests 覆盖 form/path/validation/error（`novelbookmarks_test.go:90-153`）；不证明状态 round-trip。
- **Live — `missing`**：mutation report 明确 novel add/delete、list/tags/detail read-back、restore 尚未进入 strict mutation manifest（`goal-3/mutation-validation-report.md:45-60`）。
- **Compatibility — `missing`**：无 novel mutation public symbol/wire/schema 可回放。
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
- **MCP — `missing`**：`internal/mcpserver/pixiv/pixiv.go:90-129` 仅注册单类 bookmark tools，无 all aggregate route。
- **Offline — `verified`**：当前 fixture 能证明一次调用内 artwork → novel、统一预算、failure atomicity；不能证明跨调用 cursor。（具体证据索引：本文件:364-367,389-392）
- **Live — `implemented_unverified`**：无 aggregate live second-page/aggregate cursor evidence。
- **Compatibility — `missing`**：无 public aggregate SDK/MCP wire 或 replay fixture；CLI docs 不能覆盖完整 gate。
- **Release — `missing`**：未达 public-ready。

### 4.12 `bookmark-tags-all`（#24）

- **Contract — `implemented_unverified`**：artwork tags stream 先于 novel tags stream；typed `type` 与原始 `count` 保留，同名 tag 不合并；统一 budget、checkpoint 与页原子失败规则同 list all。
- **Adapter — `verified`（leaf reuse）**：复用 artwork/novel tag adapters；无独立 aggregate adapter。（具体证据索引：本文件:260-264,315-317）
- **SDK — `missing`**：没有 public aggregate tags operation 或跨调用 aggregate cursor。
- **Shared — `verified`（offline algorithm）**：复用 `CollectStreams` 与 `bookmarkStreamCursor`。（具体证据索引：本文件:366,390）
- **CLI — `verified`（offline）**：`bookmark.go:647-808,811-856` 实现 artwork → novel typed tag output；`bookmark_test.go:124-190,478-529` 覆盖同名 tag、count/type 保留和 failure atomicity。
- **MCP — `missing`**：无 all tags MCP registration。
- **Offline — `verified`**：只证明 CLI offline aggregate。（具体证据索引：本文件:379-380）
- **Live — `implemented_unverified`**：novel tags candidate 与 subtype/continuation strict evidence 缺失；无 aggregate live evidence。
- **Compatibility — `missing`**：无 aggregate public wire/schema/fixture。
- **Release — `missing`**：未达 public-ready。

### 4.13 Bookmark cross-cutting verdict

- **Typed semantics**：CLI 以 `bookmarkListItem` / `bookmarkTagItem` 保留 artwork/novel kind；`--type all` 固定 artwork 后 novel；同名 tag 不合并，保留各自 count/type（`bookmark.go:54-69,773-856`；`bookmark_test.go:60-190`）。这是当前离线产品层证据，不是 upstream subtype evidence。
- **Dual-stream checkpoint**：`bookmarkStreamCursor` 保存上游输入 cursor 与批内 `consumed`；shared collector 在一条 logical page 中统一执行预算，任一 stream 失败丢弃 partial（`bookmark.go:71-88,433-482,811-852`；`internal/shared/pagination/streams.go:9-38,65-116`）。但 `runAllList`/`runAllTags` 当前丢弃返回 `StreamState`，没有跨调用可恢复的 public aggregate cursor。
- **Unified budget**：`CollectStreams` 连接后只应用一次 Skip/Limit/OneBatch，不按 artwork/novel 分配预算；CLI tests 已覆盖 limit=2 与双流请求。不能据此证明 public cursor 恢复。
- **Page atomicity**：CLI list/tags 先写 staging buffer，所有 stream/serialization 成功后才提交 stdout；novel stream 失败时 output 保持空。共享 traversal 也清空失败 attempt 的 partial result。
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

- **Layer verdict：** Contract=`implemented_unverified`；Adapter/SDK/Shared/CLI/Offline=`verified`；MCP=`missing`；Live/Compatibility=`implemented_unverified`；Release=`missing`。
- **证据：** novel create/reply/stamp/delete adapter 见 `internal/services/pixiv/endpoint/novel/comments/comments.go:111-160`；SDK request/operation 见 `sdk/pixiv/ops_comment.go:70-126`、`sdk/pixiv/ops_stamps.go:53-74`、`sdk/pixiv/request.go:299-323`；CLI 路由和输入约束见 `internal/cli/commands/pixiv/comment/comment.go:150-315`、`comment_test.go:301-339`。
- **边界：** `novel_comments` MCP 只读，未提供 mutation surface（`internal/mcpserver/pixiv/tools/novel_comments/novel_comments.go:13-26`）；历史 mutation row 仍是 upstream-only/production `not_tested`（`goal-3/mutation-validation-report.md:7-14,23-42`；`goal-3/evidence/appapi-upstream.md:37-40`）。

### 5.5 `stamps`（#29）

- **Layer verdict：** Contract=`implemented_unverified`；Adapter/SDK/Shared/CLI/Offline=`verified`；MCP=`missing`；Live/Compatibility=`implemented_unverified`；Release=`missing`。
- **证据：** `/v1/stamps` adapter 与验证见 `internal/services/pixiv/endpoint/stamps/stamps.go`、`internal/services/pixiv/endpoint/stamps/stamps_test.go:26-113`；SDK resource/mutation 见 `sdk/pixiv/ops_stamps.go:11-74`、`ops_stamps_test.go:153-222`；CLI surface 见 `internal/cli/commands/pixiv/comment/comment.go:125-145,317-361`。
- **边界：** legacy MCP registry 明确不新增 standalone `stamps` tool（`internal/mcpserver/pixiv/pixiv_mcp_comments_read_test.go:119-132`；`docs/en/mcp-tools.md:37-38`）。strict live 只证明 `/v1/stamps` wire/response，尚未证明当前 adapter/SDK/CLI public path 对齐（`goal-3/evidence/appapi-upstream.md:34`；`goal-3/wire-adapter-sdk-diff.md:27`）。

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

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/MCP/Offline=`verified`；CLI=`missing`；Release=`rejected`。
- **证据：** SDK artwork trending operation 位于 `sdk/pixiv/ops_artwork.go:262-279`；MCP owner/handler 与 tests 位于 `internal/mcpserver/pixiv/tools/trending_tags_illust/trending_tags_illust.go:17-55`、`internal/mcpserver/pixiv/pixiv_mcp_feed_read_test.go:34-62`；CLI user owner 中没有 trending command（`internal/cli/commands/pixiv/user/user.go:187-370`）。
- **边界：** `goal-3/upstream-contract-matrix.md:288,314` 无 strict live row；MCP surface 不能替代缺失的 CLI owner。

### 5.12 `follow-mutation`（#36）

- **Layer verdict：** Contract/SDK/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Live=`missing`；Release=`rejected`。
- **证据：** follow adapter 与测试在 `internal/services/pixiv/endpoint/user/follow/follow.go`、`follow_test.go`；SDK `FollowUser`/`UnfollowUser` 在 `sdk/pixiv/ops_mutation.go:70-95`；CLI/MCP owners 与 tests 见 `internal/cli/commands/pixiv/follow/follow.go:29-107`、`internal/mcpserver/pixiv/tools/{follow_user,unfollow_user}`、`internal/mcpserver/pixiv/pixiv_sdk_wire_test.go:323-334`。
- **边界：** 当前没有同账号 read-back/cleanup 的 strict live evidence；2xx 只说明请求被接受，不能证明关系状态变化（`goal-3/mutation-validation-report.md:28-45`；`goal-3/upstream-contract-matrix.md:298`）。

### 5.13 `mypixiv`（#37）

- **Layer verdict：** Contract/SDK/Live/Compatibility=`implemented_unverified`；Adapter/Shared/CLI/MCP/Offline=`verified`；Release=`rejected`。
- **证据：** user MyPixiv adapter 在 `internal/services/pixiv/endpoint/user/mypixiv/mypixiv.go:30-115`；SDK artwork/novel/user operations 见 `sdk/pixiv/ops_artwork.go:248-260`、`sdk/pixiv/ops_novel.go:298-309`、`sdk/pixiv/ops_user.go:148-169`；CLI/MCP owners 与 tests 见 `internal/cli/commands/pixiv/mypixiv/mypixiv.go:34-68,133-214`、`internal/mcpserver/pixiv/tools/{mypixiv_users,mypixiv_illusts,mypixiv_novels}`、`internal/mcpserver/pixiv/pixiv_mcp_user_read_test.go:174-197,292-302`。
- **边界：** contract 要求 verified current identity、禁止外部 UID/跨账号 cursor；strict live 第二页未登记（`goal-3/upstream-contract-matrix.md:289-290,314`）。`T37D` 的 WIP/旧 verified 不升级为 MCP user layer verified。

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

## 6. Rejected endpoint 与 no-fallback

以下路径和行为必须在所有层保持显式拒绝或不可达，不能以兼容为由 fallback：

- `/v1/novel/detail`：live 404，`upstream_contract_rejected`（`goal-3/evidence/appapi-upstream.md:5`）。目标只允许 v2 detail。
- `/v1/novel/series`：虽返回 HTTP 200，但缺 required detail，`required_field_missing`，不可作为 v2 fallback（`goal-3/evidence/appapi-upstream.md:7`）。
- `/v1/novel/content`：live 404，`upstream_contract_rejected`；`detail --content` 只返回显式 `ContentUnavailable`，不发送 rejected request 或 WebView fallback（`goal-3/evidence/appapi-upstream.md:9-10`；`goal-3/api-migration-verification.md:136-144`）。
- `/webview/v2/novel`：历史 HTTP 200 仅作为排除项，不是 App API fallback。
- server-side `x_restrict` / rating：服务端忽略或不支持时，不得伪装成 upstream filter；只能保留已确认的本地语义与 cursor binding（`goal-3/upstream-contract-matrix.md:33,252-255`）。
- 不得把 cursor 当鉴权凭据；不得把 upstream error 变成空成功结果；不得把不确定 mutation 自动重放。

## 7. 历史 evidence 与当前代码的边界

- Goal-3 历史 tasks 中 T01/T02、T07A/T07B、T10A–T10G、T13/T14/T18、T24–T32、T37A/B 等 `verified` 只证明对应历史 task 的实现或审计，不授予 capability `public_ready`（`goal-3/tasks.md:19-39,51-62,75-87`；`goal-3/capability-admission.md:5-15`）。
- `ugoira-metadata` strict evidence 声称 CLI/MCP confirmed，但当前源码没有专用 CLI/MCP metadata surface；本 inventory 以当前源码为准，将 live 标为 `implemented_unverified`，把该 evidence 冲突留给后续 correction/owner task。
- `novel-latest` strict upstream evidence 已显示 `max_novel_id`，但 adapter/SDK 状态为 not_tested/inconclusive；当前代码已有 max ID leaf，不得把历史 evidence 自动提升为完整 cross-layer acceptance。
- 当前分支相对 `e404434` 没有 `goal-3/` diff；上述 gaps 是继承状态，不是本轮业务/API 改动引入。

## 8. 查阅范围与验证命令

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

G1-T01 已在干净目标 worktree 执行 `go test ./...` 并通过；G1-CHECK-01 在 HEAD `042c0200ce1b3349ab3e08a744fea7802838025c` 再次执行 `go test ./...` 并通过。G1-T04 focused command 通过：`go test ./internal/services/pixiv/endpoint/artwork/comments ./internal/services/pixiv/endpoint/novel/comments ./internal/services/pixiv/endpoint/stamps ./sdk/pixiv ./internal/cli/commands/pixiv/comment ./internal/mcpserver/pixiv/... -count=1`。本轮未执行真实 Pixiv live API；本文件引用的 source/test/evidence 均为当前分支可追溯资料。

## 9. G1-T04、G1-T03 与 G1-CHECK-01 结论

- 覆盖：41/41；G1-T04 补齐 25–41 共 17 项；无 capability 漏项；全 required scope 为 `41/41 scope_admitted`，`public_ready=0/41`。
- 当前可核验实现仍主要集中在 offline adapter/SDK/CLI/MCP leaf 与局部 aggregate；strict live、mutation read-back/cleanup、MCP mutation、SDK aggregate、endpoint-global continuation 等未闭合面保持为明确 verdict。
- `logical-pagination` 的 T23A/R02 只证明 shared/checkpoint/replay engine；不能替代 user/comments/recommended/latest 等 endpoint continuation evidence。
- T37D WIP/旧 task `verified`、离线 fixture、历史 upstream mutation evidence 均未提升为 capability acceptance 或 MCP user layer verified。
- G1-CHECK-01：历史集中检查仍 PASS，覆盖 1–24；G1-T04 本轮 focused tests PASS，新增 inventory 覆盖 25–41，未改生产代码、API、scope 或 Goal-3 资料。
- GoalState：保持 `ACTIVE`。无新增 external/decision blocker；下一任务为 `G1-T05`。
