# Goal-1 当前状态：Capabilities 1–13 baseline inventory

> 本文件对应 `G1-T02`，只记录当前分支的代码、测试、历史和 Goal-3 证据，不授予任何 capability 的发布资格。

## 1. 快照与状态口径

- 执行分支：`refactor/pixiv-api-stability`
- Inventory baseline HEAD：`691b173de4f975d77ddf9d2f9a08a5e4b5465619`，继承基线：`e40443495981cdaf01215d6711cb24fa618b087a`
- 当前 worktree：`/Users/flanchan/Developer/Projects/GithubProjects/.worktrees/pixiv-cli-refactor-pixiv-api-stability`
- 当前 worktree 干净；与远端分支相比本地仅包含 G1-T01 的 tracking commit。与继承基线相比，当前分支只新增 Goal-1 tracking 文件，`goal-3/` 无 diff。
- Goal-3 的 `goal-3/capability-admission.md` 是 capability 状态唯一权威来源：13 项均为 `required=yes, state=scope_admitted`，没有一项为 `public_ready`（`goal-3/capability-admission.md:3-15,23-36`）。
- `scope_admitted` 只表示 capability 属于 required scope；不表示 contract 已冻结、迁移已完成或可以进入正式发布 surface。
- 本文状态：
  - `verified`：当前源码/离线测试/已有证据可以直接核验该层事实。
  - `implemented_unverified`：已有实现或历史证据，但缺少当前 Goal 要求的完整跨层、live、兼容或发布证明。
  - `missing`：当前责任链或必要证据明确不存在。
  - `rejected`：该层被当前发布门禁拒绝；若涉及 endpoint，则同时表示不得调用或 fallback。

## 2. 覆盖计数与总览

### 2.1 Required coverage

- Capabilities：13/13，全部纳入 required scope。
- `public_ready`：0/13。
- `scope_admitted`：13/13。
- 当前没有 capability 可以仅凭 Goal-3 历史 task 的 `verified` 标记直接转为 accepted。
- 当前未发现新的业务/API/evidence drift；Goal-1 分支相对继承基线的 drift 只有执行资料和 G1-T01 记录。

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

`Release=rejected` / `Release=missing` 均表示当前不能发布，不表示 required scope 可以删除。所有 13 项必须继续沿 Goal-1 Phase A–F 完成各自 gate。

## 3. Capability inventory

### 3.1 `artwork-search`（#1）

- **Contract — `implemented_unverified`**：目标为 `/v1/search/illust`；contract 已记录四种 content type、首请求不带 `offset`、续页使用正 `offset`、query 与 cursor binding（`goal-3/upstream-contract-matrix.md:29-33,51-57,251-254`）。`x_restrict`/rating 不是已确认的 server-side filter，不能伪装成 upstream contract（`goal-3/upstream-contract-matrix.md:33`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/artwork/search/search.go:63-107,116-145` 负责 route、filters、offset、required list 与 continuation；`search_test.go:26-113` 覆盖 query、normalized artwork、空/null list 和非法 continuation。
- **SDK — `implemented_unverified`**：公开 `Client.SearchArtworks` 位于 `sdk/pixiv/ops_artwork_search.go:11-136`，request 位于 `sdk/pixiv/request.go:103-123`；SDK 两页 fixture 位于 `sdk/pixiv/pixiv_test.go:493-543`。typed cursor 已存在，但 query/account/content-type/AI/local filter 的完整兼容 gate 尚未闭包。
- **Shared — `verified`**：使用 typed cursor/pagination，不由 CLI/MCP 解析 raw `next_url`；相关约束见 `goal-3/pagination-validation-report.md:66-68`。
- **CLI — `verified`**：`internal/cli/commands/pixiv/search/search.go:210-243,342-379` 支持 search 与 `--content-type`，调用 SDK；两页 fixture 位于 `internal/cli/commands/pixiv/search/bookmark_test.go:180-255`。
- **MCP — `verified`**：`search_illust` handler 位于 `internal/mcpserver/pixiv/tools/search_illust/search_illust.go:21-36,112-144`，聚合注册位于 `internal/mcpserver/pixiv/pixiv.go:114`。
- **Offline — `verified`**：endpoint、SDK、CLI、MCP 相关 fixture 和 targeted tests 已存在；G1-T01 的 `go test ./...` 也通过。
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
- **Offline — `verified`**：timeline endpoint、SDK、CLI、MCP fixture 已覆盖基础分页和 continuation 错误。
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
- **Offline — `verified`**：endpoint、SDK、CLI、MCP tests 覆盖 invalid mode/date 和 continuation。
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
- **Offline — `verified`**：endpoint、SDK、CLI、MCP fixture 已通过相关离线验证。
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
- **Offline — `verified`**：endpoint、SDK、CLI、MCP 两页合成 fixture 存在并通过。
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
- **Offline — `verified`**：SDK mapping/error 与 public interface compile tests 存在并通过；CLI/MCP 缺失不能被这些测试掩盖。
- **Live — `implemented_unverified`**：旧 strict evidence 的 `ugoira-metadata` 行声称 HTTP 200、无 continuation、全链 confirmed，但该行与当前源码中缺少 CLI/MCP surface 矛盾；按当前代码不能直接接受旧行（`goal-3/evidence/appapi-upstream.md:33`；当前源码证据见上述 CLI/MCP）。
- **Compatibility — `implemented_unverified`**：SDK DTO/resource contract 已存在，但 CLI/MCP compatibility contract 缺失。
- **Release — `rejected`**：缺少 required CLI/MCP surface，且 capability 仍是 `scope_admitted`。

### 3.7 `novel-search`（#7）

- **Contract — `implemented_unverified`**：`/v1/search/novel`、word、默认 target/sort、duration、首请求无 offset、续页正 offset 见 `goal-3/upstream-contract-matrix.md:91`。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/search/search.go:39-81` 构造请求并解析 novels/next_url/offset；`search_test.go:24-57` 覆盖 query、continuation、null/missing/empty list。
- **SDK — `verified`**：`SearchNovels` 位于 `sdk/pixiv/ops_novel.go:19-64`，校验 target/sort/duration 并绑定 cursor；fixture 位于 `sdk/pixiv/pixiv_test.go:135-182,276-414`。
- **Shared — `verified`**：复用 novel typed cursor/query binding；但 local `novel_filter` 不得伪装成 upstream 字段。
- **CLI — `verified`**：`internal/cli/commands/pixiv/search/novel.go:26-35,94-105` 支持 canonical `pixiv search --type novel` 与 legacy `pixiv novel search`；测试见 `novel_test.go:56-59,110-156`。
- **MCP — `verified`**：`search_novel` 位于 `internal/mcpserver/pixiv/tools/search_novel/search_novel.go:16-86`，支持 request/filter/page/limit 并经 SDK 分页。
- **Offline — `verified`**：endpoint/SDK/CLI/MCP fixture 与 targeted tests 存在；当前 baseline 全量测试通过。
- **Live — `implemented_unverified`**：缺独立 strict live 两页及 period/date 字段证据；`goal-3/upstream-contract-matrix.md:91` 与 `goal-3/pagination-validation-report.md:57-64` 明确记录 gap。
- **Compatibility — `verified`（已有 legacy mapping，非 release acceptance）**：CLI migration matrix 要求保留 `pixiv novel search WORD` 并映射到 canonical route（`goal-3/cli-migration-matrix.md:51,111`）；但 period/date 尚未冻结。
- **Release — `missing`**：required acceptance 是 period/date 与两页，当前尚未证明。

### 3.8 `novel-detail`（#8）

- **Contract — `implemented_unverified`**：目标为 `/v2/novel/detail`；旧 `/v1/novel/detail` 已 rejected（`goal-3/upstream-contract-matrix.md:14-15`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/detail/detail.go:31-58` 使用 v2 protocol 并解析 novel/series metadata；测试覆盖 required/missing/null malformed（`detail_test.go:31-47`）。
- **SDK — `verified`（离线实现）**：`sdk/pixiv/ops_novel.go:66-76` 的 `Novel` 校验正 ID并调用 adapter；fixture 位于 `sdk/pixiv/pixiv_test.go:276-414`。
- **Shared — `verified`**：detail 无分页，复用统一 output/error model。
- **CLI — `verified`**：`internal/cli/commands/pixiv/detail/detail.go:122-152,184-196` 支持 `--type novel`；`--content` 返回显式 `ContentUnavailable`。
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/novel_detail/novel_detail.go:16-42` 注册 tool；schema/结果测试见对应 `novel_detail_test.go` 与 `pixiv_mcp_artwork_novel_read_test.go:38-39,99-104`。
- **Offline — `verified`**：v2 adapter/SDK/CLI/MCP fixtures 和错误边界存在。
- **Live — `implemented_unverified`**：`/v2/novel/detail` HTTP 200、wire/response confirmed，但 strict evidence 的 Adapter/SDK 为 `not_tested`（`goal-3/evidence/appapi-upstream.md:5-6`）。
- **Compatibility — `implemented_unverified`**：v1 rejection 与 content unavailable 兼容语义已存在，但 v2 full chain 尚未以当前 strict evidence 闭包。
- **Release — `missing`**：必须保持 v1 rejected、完成 v2 detail/series metadata 的完整 acceptance 后再发布。

### 3.9 `novel-series`（#9）

- **Contract — `implemented_unverified`**：目标为 `/v2/novel/series`、`last_order` continuation、required `novel_series_detail` 与 `novels`；旧 v1 因 required detail 缺失而 rejected（`goal-3/upstream-contract-matrix.md:16-17,93-105`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/series/series.go:36-70` 构造 v2 query、解析 last_order 和 required fields；测试见 `series_test.go:24-52`。
- **SDK — `verified`（离线实现）**：`sdk/pixiv/ops_novel.go:78-114` 校验 positive series ID、使用 last_order cursor 并保留 metadata；fixture 位于 `sdk/pixiv/pixiv_test.go:460-543`。
- **Shared — `verified`**：typed last_order cursor，不解析 raw URL。
- **CLI — `verified`**：`internal/cli/commands/pixiv/series/series.go:39-49,99-100,142-180` 支持 `--type novel`、URL reference、metadata 与分页；测试见 `series_test.go:73-109`。
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/novel_series/novel_series.go:16-62` 注册并返回 metadata/records/pagination。
- **Offline — `verified`**：v2 endpoint/SDK/CLI/MCP fixtures 已存在。
- **Live — `implemented_unverified`**：v2 首页 HTTP 200、29 records、request 使用 series_id/last_order，但第二页未观察，adapter/SDK 未测试（`goal-3/evidence/appapi-upstream.md:7-8`；`goal-3/pagination-validation-report.md:33-38`）。
- **Compatibility — `implemented_unverified`**：v1 rejection 与 v2 route 已分离，但第二页和 full chain 未确认。
- **Release — `missing`**：不能用 v1 response 或离线两页 fixture 替代 v2 live 两页 acceptance。

### 3.10 `novel-latest`（#10）

- **Contract — `implemented_unverified`**：`/v1/novel/new` 要求 `filter=for_android`，continuation 必须为 `max_novel_id`，禁止 offset（`goal-3/upstream-contract-matrix.md:20,94`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/timeline/timeline.go:47-85,108-117` 发送 max_novel_id；`timeline_test.go:101-165` 拒绝 offset、mixed keys，并接受 max_novel_id。
- **SDK — `verified`（当前 leaf 实现）**：`sdk/pixiv/ops_novel.go:202-213` 使用 max_novel_id；`ops_novel_test.go:195-280` 覆盖两次调用、无 offset 和旧 offset cursor 错误。
- **Shared — `verified`**：latest 使用专用 max_novel_id typed cursor，不把旧 offset 当 fallback。
- **CLI — `verified`**：`internal/cli/commands/pixiv/timeline/timeline.go:72,157` 支持 `timeline latest --type novel`。
- **MCP — `verified`**：`internal/mcpserver/pixiv/tools/timeline_novel_latest/timeline_novel_latest.go:16-63` 注册 tool，并支持 local novel filter。
- **Offline — `verified`**：timeline endpoint、SDK、CLI、MCP 迁移测试存在。
- **Live — `implemented_unverified`**：upstream 有 30→30、max_novel_id evidence，但 strict row 的 adapter 为 `not_tested`、SDK 为 `inconclusive`、verdict 为 `sdk_call_error`（`goal-3/evidence/appapi-upstream.md:11`）；旧分页报告也记录过 offset mismatch（`goal-3/pagination-validation-report.md:14-20`）。
- **Compatibility — `implemented_unverified`**：当前代码已拒绝旧 offset cursor，但尚需用与当前代码一致的 live adapter/SDK/CLI/MCP evidence 关闭迁移 gate。
- **Release — `missing`**：不能把 upstream HTTP 两页直接提升为完整 public acceptance。

### 3.11 `novel-recommended`（#11）

- **Contract — `implemented_unverified`**：首请求无 query；续页显式带 offset，包括合法 offset=0（`goal-3/upstream-contract-matrix.md:22,95`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/recommended/recommended.go:35-82` 实现 initial/continuation 区分；测试覆盖 initial offset 拒绝、负 offset、显式零 continuation 和 null list（`recommended_test.go:26-74`）。
- **SDK — `verified`**：`sdk/pixiv/ops_novel.go:169-180` 调用 adapter；`ops_novel_test.go:78-119` 覆盖显式零 offset 两页。
- **Shared — `verified`**：使用 shared pagination/cursor 语义，零 offset 仅在该 operation 的 continuation 语境有效。
- **CLI — `verified`**：recommended command 的 novel 分支调用 `RecommendedNovels`（`internal/cli/commands/pixiv/recommended/recommended.go:247,325`）；fixture 见 `recommended_test.go:146-149,229-277`。
- **MCP — `verified`**：recommended MCP 支持 `kind=novel/all`，经 `CollectPages` 调用 SDK（`internal/mcpserver/pixiv/tools/recommended/recommended.go:160-180`；行为测试见 `pixiv_mcp_feed_read_test.go:51-58,194-228`）。
- **Offline — `verified`**：endpoint/SDK/CLI/MCP 两页 fixture 与 targeted tests 已存在。
- **Live — `verified`（upstream/adapter/SDK case）**：HTTP 200、33→33、offset、wire/response/pagination/adapter/SDK confirmed（`goal-3/evidence/appapi-upstream.md:13`）。
- **Compatibility — `implemented_unverified`**：仍需完成 T12/T18/T28/T37 兼容、文档和 release gates；confirmed 不等于 public-ready（`goal-3/upstream-contract-matrix.md:95-97`）。
- **Release — `missing`**：未通过统一 public readiness gate。

### 3.12 `novel-ranking`（#12）

- **Contract — `implemented_unverified`**：`/v1/novel/ranking` 的 filter/mode/positive offset contract 见 `goal-3/upstream-contract-matrix.md:23,97`。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/ranking/ranking.go:36-78` 与 `ranking_test.go:27-77` 覆盖 route/query/default/filter/mode/continuation。
- **SDK — `verified`（离线实现）**：`sdk/pixiv/ops_novel.go:116-137` 实现 `NovelRanking`；`sdk/pixiv/ops_novel_test.go:12-76` 覆盖 mode、两页 cursor 和 invalid mode no-network。
- **Shared — `verified`**：使用 positive offset cursor 和 ranking mode binding。
- **CLI — `verified`**：`internal/cli/commands/pixiv/ranking/ranking.go:38-86` 支持 `--type novel`；两页 typed route fixture 见 `ranking_test.go:27-129`。
- **MCP — `missing`**：当前没有专门 `novel_ranking` tool；现有 `illust_ranking` 只覆盖 artwork，不能充当 novel ranking MCP owner。
- **Offline — `verified`**：endpoint、SDK、CLI fixtures 已有；MCP 缺失是结构性缺口，不被离线测试掩盖。
- **Live — `implemented_unverified`**：upstream HTTP 200、30→30、offset，但 strict evidence 的 adapter/SDK 为 `not_tested`、production owner missing（`goal-3/evidence/appapi-upstream.md:14`；`goal-3/wire-adapter-sdk-diff.md:13-14`）。
- **Compatibility — `implemented_unverified`**：CLI/SDK typed route 存在，但缺 MCP 和完整 cross-layer proof。
- **Release — `missing`**：缺少 MCP owner/tool/schema/test，不能发布。

### 3.13 `novel-follow`（#13）

- **Contract — `implemented_unverified`**：`/v1/novel/follow` 支持 restrict public/private；首请求无 offset、续页正 offset，restrict 必须进入 binding（`goal-3/upstream-contract-matrix.md:21,96`）。
- **Adapter — `verified`**：`internal/services/pixiv/endpoint/novel/timeline/timeline.go:94-105,224-244` 校验 restrict、构造 query、解析 offset；测试见 `timeline_test.go:28-88`。
- **SDK — `verified`**：`FollowingNovels` 位于 `sdk/pixiv/ops_novel.go:183-199`；`ops_novel_test.go:121-194` 覆盖两页、restrict 和 invalid value no-network。
- **Shared — `verified`**：shared cursor 绑定 restrict，不把 follow-user mutation 的状态混入 feed cursor。
- **CLI — `verified`**：`internal/cli/commands/pixiv/timeline/timeline.go:51,123` 支持 `timeline following --type novel`。
- **MCP — `verified`**：`timeline_novel_following` 位于 `internal/mcpserver/pixiv/tools/timeline_novel_following/timeline_novel_following.go:16-70`；认证态边界由 tool 注释和测试覆盖。
- **Offline — `verified`**：endpoint/SDK/CLI/MCP fixture 与 restrict error boundary 存在。
- **Live — `verified`（frozen case）**：HTTP 200、30→30、offset，wire/response/pagination/adapter/SDK confirmed（`goal-3/evidence/appapi-upstream.md:12`；`goal-3/pagination-validation-report.md:7-9`）。
- **Compatibility — `implemented_unverified`**：必须保持认证态、restrict/account binding 和 CLI/MCP compatibility；不能把 `follow add/remove` mutation 当作该 feed 的证明。
- **Release — `missing`**：统一 compatibility/docs/release gates 尚未完成。

## 4. Rejected endpoint 与 no-fallback

以下路径和行为必须在所有层保持显式拒绝或不可达，不能以兼容为由 fallback：

- `/v1/novel/detail`：live 404，`upstream_contract_rejected`（`goal-3/evidence/appapi-upstream.md:5`）。目标只允许 v2 detail。
- `/v1/novel/series`：虽返回 HTTP 200，但缺 required detail，`required_field_missing`，不可作为 v2 fallback（`goal-3/evidence/appapi-upstream.md:7`）。
- `/v1/novel/content`：live 404，`upstream_contract_rejected`；`detail --content` 只返回显式 `ContentUnavailable`，不发送 rejected request 或 WebView fallback（`goal-3/evidence/appapi-upstream.md:9-10`；`goal-3/api-migration-verification.md:136-144`）。
- `/webview/v2/novel`：历史 HTTP 200 仅作为排除项，不是 App API fallback。
- server-side `x_restrict` / rating：服务端忽略或不支持时，不得伪装成 upstream filter；只能保留已确认的本地语义与 cursor binding（`goal-3/upstream-contract-matrix.md:33,252-255`）。
- 不得把 cursor 当鉴权凭据；不得把 upstream error 变成空成功结果；不得把不确定 mutation 自动重放。

## 5. 历史 evidence 与当前代码的边界

- Goal-3 历史 tasks 中 T01/T02、T07A/T07B、T10A–T10G、T13/T14/T18、T24–T32、T37A/B 等 `verified` 只证明对应历史 task 的实现或审计，不授予 capability `public_ready`（`goal-3/tasks.md:19-39,51-62,75-87`；`goal-3/capability-admission.md:5-15`）。
- `ugoira-metadata` strict evidence 声称 CLI/MCP confirmed，但当前源码没有专用 CLI/MCP metadata surface；本 inventory 以当前源码为准，将 live 标为 `implemented_unverified`，把该 evidence 冲突留给后续 correction/owner task。
- `novel-latest` strict upstream evidence 已显示 `max_novel_id`，但 adapter/SDK 状态为 not_tested/inconclusive；当前代码已有 max ID leaf，不得把历史 evidence 自动提升为完整 cross-layer acceptance。
- 当前分支相对 `e404434` 没有 `goal-3/` diff；上述 gaps 是继承状态，不是 G1-T02 新引入。

## 6. 查阅范围与验证命令

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
- capabilities 1–13 的 endpoint、SDK、CLI、MCP source/test 文件及 Git history

### LSP 证据

目标 worktree 已启动 `gopls`；已通过 `open_document`/`list_symbols`/`find_symbol` 核验 protocol、SDK public operation、novel ranking 和 ugoira metadata symbols。关键结果包括：

- `sdk/pixiv.Client.SearchArtworks`、`LatestArtworks`、`ArtworkRanking`、`RecommendedArtworks`、`ArtworkSeries`、`UgoiraMetadata` 存在。
- `sdk/pixiv.Client.SearchNovels`、`Novel`、`NovelSeries`、`LatestNovels`、`RecommendedNovels`、`NovelRanking`、`FollowingNovels` 存在。
- `NovelRankingRequest`、`AppNovelRanking`、`UgoiraMetadataRequest` 和 public DTO 存在。
- 当前未发现 `ugoira_metadata` CLI/MCP owner，也未发现专用 `novel_ranking` MCP owner。

### Offline evidence

G1-T01 已在干净目标 worktree 执行 `go test ./...` 并通过。G1-T02 本身只读，不重新运行全量测试；本文件引用的 targeted tests/fixtures 是当前分支文件中的可追溯证据，后续 owner task 仍需按当前代码重新运行相关 gate。

## 7. G1-T02 结论

- 覆盖：13/13，无漏项。
- `verified` 层均有 source/test/evidence index；未把历史 task 的 `verified` 直接升级为 capability acceptance。
- 当前主要内部 gaps：
  1. artwork recommended 第二页失败且 subtype binding 未闭合。
  2. artwork series 缺独立 live 第二页。
  3. ugoira metadata 缺 CLI/MCP surface，旧 live evidence 与当前源码冲突。
  4. novel search 缺 period/date 与独立 strict 两页证据。
  5. novel detail/series 缺当前 v2 adapter→SDK→CLI/MCP live 闭环。
  6. novel latest 的 max_novel_id 迁移缺当前完整 live 闭环。
  7. novel ranking 缺 MCP owner，且 strict adapter/SDK evidence 未测试。
  8. 统一 contract/compatibility/docs/release gates 尚未把任何项提升为 `public_ready`。
- GoalState：保持 `ACTIVE`。无新 external/decision blocker；下一任务为 `G1-T03`。
