# Command-to-upstream contract matrix

观测日期：2026-09-05（Asia/Shanghai）。

说明：

- `P1/P2` 是两页 item count。
- `—` 表示本次不要求或未测试。
- `pagination_exempt` 是用户确认的数据受限例外。
- `confirmed` 是当前 evidence verdict；完整 contract 以 `contract_frozen` 为准。

| Case | 命令 | Method | Path | 参数 | P1 | P2 | Continuation | Adapter | SDK | Verdict | 备注 |
| --- | --- | --- | --- | --- | ---: | ---: | --- | --- | --- | --- | --- |
| novel-detail-v1 | detail novel | GET | /v1/novel/detail | novel_id | — | — | none | — | — | rejected | live v1 不可用 |
| novel-detail-v2 | detail novel | GET | /v2/novel/detail | novel_id | — | — | none | not_tested | not_tested | not_tested | upstream migration-ready；public 未验证 |
| novel-series-v1 | series novel | GET | /v1/novel/series | series_id | 28 | — | none | — | — | rejected | required detail 缺失 |
| novel-series-v2 | series novel | GET | /v2/novel/series | series_id,last_order | — | — | last_order | — | — | inconclusive | 生产仍为 v1 |
| novel-content-app | detail novel --content | GET | /v1/novel/content | novel_id | — | — | none | — | — | rejected | live v1 不可用 |
| novel-content-webview | detail novel --content | GET | /webview/v2/novel | id | — | — | none | — | — | not_tested | 排除 |
| novel-new | timeline latest novel | GET | /v1/novel/new | filter,max_novel_id | 30 | 30 | max_novel_id | rejected | inconclusive | inconclusive | 当前使用 offset |
| novel-follow | timeline following novel | GET | /v1/novel/follow | restrict | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| novel-recommended | recommended novel | GET | /v1/novel/recommended | — | 33 | 33 | offset | confirmed | confirmed | confirmed | |
| novel-ranking | candidate novel ranking | GET | /v1/novel/ranking | filter,mode | 30 | 30 | offset | not_tested | not_tested | not_tested | production owner missing |
| user-novels | user novels | GET | /v1/user/novels | filter,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-novel-bookmarks-public | bookmark novel public | GET | /v1/user/bookmarks/novel | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-novel-bookmarks-private | bookmark novel private | GET | /v1/user/bookmarks/novel | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | private data verified |
| novel-comments-v2 | comment novel | GET | /v2/novel/comments | novel_id,offset | — | — | offset | confirmed | confirmed | inconclusive | access-control risk |
| novel-comments-v3 | comment novel candidate | GET | /v3/novel/comments | novel_id,offset | — | — | offset | candidate | candidate | inconclusive | candidate fixture passed |
| search-illust-all | search artwork | GET | /v1/search/illust | filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-illust | search artwork illust | GET | /v1/search/illust | content_type,filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-manga | search artwork manga | GET | /v1/search/illust | content_type,filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-ugoira | search artwork ugoira | GET | /v1/search/illust | content_type,filter,search_target,sort,word | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| search-illust-rating | search artwork rating | GET | /v1/search/illust | x_restrict | — | — | — | rejected | rejected | rejected | server ignores parameter |
| illust-recommended | recommended artwork | GET | /v1/illust/recommended | filter,continuation state | 83 | — | offset+state | confirmed | confirmed | inconclusive | second page error |
| illust-new | timeline latest artwork | GET | /v1/illust/new | content_type,filter | 30 | 30 | max_illust_id | confirmed | confirmed | confirmed | |
| illust-ranking | ranking artwork | GET | /v1/illust/ranking | mode | 30 | 30 | offset | confirmed | confirmed | confirmed | |
| user-illusts-illust | user artworks illust | GET | /v1/user/illusts | filter,type,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-illusts-manga | user artworks manga | GET | /v1/user/illusts | filter,type,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-illust-bookmarks-public | bookmark artwork public | GET | /v1/user/bookmarks/illust | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | data-limited |
| user-illust-bookmarks-private | bookmark artwork private | GET | /v1/user/bookmarks/illust | restrict,user_id | — | — | pagination_exempt | confirmed | confirmed | confirmed | private data verified |
| illust-comments-v3 | comment artwork | GET | /v3/illust/comments | illust_id,offset | — | — | offset | rejected | rejected | rejected | live date/access-control mismatch |
| ugoira-metadata | ugoira metadata | GET | /v1/ugoira/metadata | illust_id | 1 | — | none | confirmed | confirmed | confirmed | |
| stamps | candidate stamps | GET | /v1/stamps | — | 40 | — | none | not_tested | not_tested | not_tested | production owner missing |
| illust-comment-text | comment artwork text | POST | /v1/illust/comment/add | illust_id,comment | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| illust-comment-reply | reply artwork comment | POST | /v1/illust/comment/add | illust_id,comment,parent_comment_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| novel-comment-stamp | comment novel stamp | POST | /v1/novel/comment/add | novel_id,comment,stamp_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| novel-comment-text | comment novel text | POST | /v1/novel/comment/add | novel_id,comment | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| illust-comment-delete | delete artwork comment | POST | /v1/illust/comment/delete | comment_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |
| novel-comment-delete | delete novel comment | POST | /v1/novel/comment/delete | comment_id | — | — | read_back | not_tested | not_tested | not_tested | production owner missing |

## T01 artwork 基础 contract 冻结（2026-09-07）

本节冻结 artwork 六类 operation 的基础 request、normalized entity/DTO、subtype 与异常边界；它不是把不同 endpoint 压成一个通用 request。历史 evidence 的 `confirmed`/`inconclusive` 仍保持原样，能力发布状态仍只见 [能力准入表](capability-admission.md)。

| Operation | Method / path | Base request | Normalized response | Continuation / subtype boundary | Evidence boundary |
| --- | --- | --- | --- | --- | --- |
| artwork search | `GET /v1/search/illust` | `word` required；`search_target` 默认 `partial_match_for_tags`；`sort` 默认 `date_desc`；`duration` 或 `start_date/end_date` optional；`offset` 只来自 continuation | required `illusts` list → `Artwork`；`next_url` 只在 adapter 内解析 | search selector 为 `all`、`illust-and-ugoira`、`illust`、`manga`、`ugoira`；`AIModeOnly` 是本地 filter，不是 subtype；wire `illust` 映射为 semantic `illust` / public legacy `illustration` | 四种 search case 均已有两页 wire/response/adapter/SDK evidence；rating 不进入 server-side contract |
| artwork series | `GET /v1/illust/series` | `illust_series_id` required positive；首请求不带 `last_order`，续页只带 positive `last_order` | `illust_series_detail.user.id` 与 required `illusts` list；series user ID 是成功条件；item 仍 normalized 为 `Artwork` | continuation key 固定 `last_order`；series request 不承载 subtype filter，item subtype 仍按 `illust/manga/ugoira` 映射 | 现有 endpoint/SDK fixture 已确认；独立 live T01 matrix/第二页证据缺失，保留给 T05 |
| artwork latest | `GET /v1/illust/new` | wire 必有 `content_type` 与 `filter=for_android`；public empty value 默认 `illust`；续页使用服务端返回的 continuation | required `illusts` list → `Artwork` | base subtype 先冻结 `illust`；`max_illust_id` 优先，兼容解析 `offset`；`manga`/`ugoira`/compound expansion 需独立证据 | `illust` 两页 confirmed；扩展 subtype 仍为 partial，不由 T01 提升 |
| artwork ranking | `GET /v1/illust/ranking` | `mode` default `day`；`date` optional；续页为 positive `offset` | required `illusts` list → `Artwork` | subtype 不是独立 request 维度；`day_manga` 等 mode 由 ranking contract 表达，不映射成全局 `ArtworkKind`；合法 mode 由 `RankingMode` allowlist 冻结 | artwork ranking 两页 confirmed；mode/date 非法为 `InvalidArgument` |
| artwork recommended | `GET /v1/illust/recommended` | base request 无 query；首次不发送 `offset`；续页即使 offset 为 `0` 也必须显式发送 `offset=0`；endpoint 的 optional `content_type` 先保留为 candidate | required `illusts` list → `Artwork` | continuation key 为 `offset`，且必须保留 initial/continuation distinction；recommended subtype 不在 T01 宣告支持 | 首页/基础 mapping confirmed；第二页失败根因与 subtype 两页未确认，保留给 T05 |
| ugoira metadata | `GET /v1/ugoira/metadata` | `illust_id` required positive；无分页 | required metadata → `UgoiraMetadataDTO{artwork_id, archives, frames}`；至少一个 archive 与非空 frames | 无 continuation；这是 ugoira metadata operation，不是把所有 artwork 都标为 ugoira | wire/response/adapter/SDK mapping confirmed；unsafe/duplicated frame filename 为 malformed |

### T01 normalized artwork DTO 与 null/empty/error 规则

- public `ArtworkDTO` 的稳定字段为 `id/title/caption/kind/raw_kind/tags/user/published_at/total_bookmarks/total_views/width/height/page_count/x_restrict/ai_type/cover`；`updated_at`、`tools`、`pages` 是 optional `omitempty` 字段。`UpdatedAt=nil` 表示 upstream 未提供，list operation 不伪造 `Pages`，detail 才在有可用 media metadata 时填充页面。
- `ArtworkKind` public legacy spelling 保持 `illustration/manga/ugoira/unknown`；`RawKind` 保留 upstream 原值。T20 的 semantic subtype `illust` 与 wire `illust` 的映射不能通过删除既有 SDK named value 来实现。
- artwork list/series/latest/ranking/recommended 的 `illusts` 是 required list：缺失或 JSON `null` 为 `MalformedUpstreamResponse`，空数组合法且 normalized `Items` 必须为 non-nil empty slice。`next_url=null` 是正常结束；非 null 空字符串、缺少唯一合法 continuation key、重复 key、超出该 operation allowlist/range 或不可解析值均为 malformed（recommended 的 continuation `offset=0` 是允许的特例）。
- `PublishedAt` 不能由无效/缺失 `create_date` 猜测；不能生成伪造 media URL。cover 按 `original → large → medium → square_medium` 选择，页面必须有可用 image URL；这些 malformed/error mapping 由 adapter/SDK owner 落地，不由本任务改变。
- ugoira metadata 要求 `zip_urls` 至少有一个可用 archive、frames 非空；frame filename 必须非空、相对、安全且不重复。请求 ID/mode/date 等调用方错误为 `InvalidArgument`，upstream 结构错误为 `MalformedUpstreamResponse`，取消与 transport/upstream 错误保持真实分类。

### T01 evidence index

- search：历史两页四种 selector 的 evidence 为 `goal-3/evidence/appapi-upstream.md:20-23`，endpoint/SDK 回归分别见 `internal/services/pixiv/endpoint/artwork/search/search_test.go:24-94`、`sdk/pixiv/pixiv_test.go:493-543`。
- series：首批/续页 `last_order` 与 series user required fixture 见 `sdk/pixiv/pixiv_test.go:276-320`、`internal/services/pixiv/endpoint/artwork/series/series_test.go:24-42`；独立 live evidence 尚缺。
- latest/ranking：两页 evidence 见 `goal-3/evidence/appapi-upstream.md:26-27`；wire/continuation regression 见 `internal/services/pixiv/endpoint/artwork/timeline/timeline_test.go:24-84`、`internal/services/pixiv/endpoint/artwork/ranking/ranking_test.go:24-42`。
- recommended：initial/continuation offset distinction 见 `internal/services/pixiv/endpoint/artwork/recommended/recommended_test.go:24-49`；历史第二页失败与 subtype 未确认见 `goal-3/evidence/appapi-upstream.md:25`、`goal-3/pagination-validation-report.md:24-31,57-61`。
- DTO/ugoira：public optional-field shape 见 `sdk/pixiv/dto_test.go:93-144`；ugoira archive/frame mapping 与 unsafe filename 见 `sdk/pixiv/pixiv_test.go:890-918`。

### T01 与后续任务的边界

- T01 已冻结基础 operation contract，不把历史 evidence 直接提升为 capability `contract_frozen`/`public_ready`。T05 仍必须补 artwork series live 第二页、latest 扩展 subtype、recommended 完整 continuation/subtype 两页及各 operation 的 query/account/subtype binding。
- T07/T10 负责 endpoint leaf、DTO/error mapping 和 ranking/latest/recommended owner 实现；T13 负责 public SDK、旧签名、series metadata 是否公开及 ugoira resource mapping。T01 不新增 production path、public symbol 或依赖。

## T02 novel 基础 contract 冻结（2026-09-07）

本节冻结 novel 七类 read operation 的基础 request、normalized entity/DTO、continuation 和 rejection 边界。历史表中的 `confirmed`、`inconclusive`、`not_tested` 与 `rejected` 不被改写；本节的 target contract 不等于 adapter/SDK 已实现，也不把任何 capability 提升为 `migration_ready` 或 `public_ready`。Novel 正文是独立 operation，不属于本节的 metadata contract；已被拒绝或排除的正文路径见“后续边界”。

| Operation | Method / path | Base request | Normalized response | Continuation / subtype / rejection boundary | Evidence boundary |
| --- | --- | --- | --- | --- | --- |
| novel search | `GET /v1/search/novel` | `word` required；`search_target` 默认 `partial_match_for_tags`；`sort` 默认 `date_desc`；`duration` optional；首请求不带 `offset`，续页只带正 `offset` | required `novels` list → normalized `Novel` / public `NovelDTO`；`next_url` 只在 adapter 内解析 | search target、sort、duration 由各自 allowlist 校验；`start_date/end_date` 尚未进入 strict manifest，不在 T02 冻结；novel filter 仍是本地语义，不伪装成 upstream subtype | 现有 SDK/adapter request、DTO 和单元 fixture 已核验；独立 strict live 两页与日期字段证据尚缺，保持 `scope_admitted` |
| novel detail | `GET /v2/novel/detail` | `novel_id` required positive；无分页 | required `novel` → `Novel`；保留 `series_next`、`series_prev` 的可选引用及 ID/title | `series_next`/`series_prev` 不产生 continuation；`/v1/novel/detail` 已 rejected，禁止 fallback 或继续请求旧 path | v2 wire/response 已确认；现有生产 adapter/SDK 与旧 v1 fixture 尚未迁移，留给 T07/T14 |
| novel series | `GET /v2/novel/series` | `series_id` required positive；首请求只带 `series_id`，续页只带正 `last_order` | required `novel_series_detail` 与 `novels` list；保留 series ID/title/caption/user/is_concluded 及 novels | continuation key 固定为 `last_order`；不承载全局 subtype；`/v1/novel/series` 已 rejected，禁止 fallback | v2 wire/response 首批已确认，真实第二页与 adapter/SDK 尚缺；现有 v1 生产路径不得据此放行 |
| novel latest | `GET /v1/novel/new` | wire 固定发送 `filter=for_android`；首请求无 continuation | required `novels` list → `Novel` | continuation 必须使用 upstream `max_novel_id`，不得把 `offset` 当作兼容替代；novel content subtype 不在本 operation 声明 | live wire/response 两页已确认；当前 adapter/SDK 仍使用 offset，修复留给 T10/T14/T18 |
| novel recommended | `GET /v1/novel/recommended` | 首请求不带 query；续页显式发送 `offset`，包括合法的 `offset=0` | required `novels` list → `Novel` | continuation key 为 `offset`，必须区分“没有 cursor”和“cursor 值为 0”；不额外宣告 subtype | 两页 wire/response/adapter/SDK 已确认；后续仍需按 T05 补 binding 与完整回归 |
| novel follow | `GET /v1/novel/follow` | `restrict` 为可选 public/private 选择，CLI/MCP 默认 public；首请求不带 offset | required `novels` list → `Novel` | continuation key 为正 `offset`；`restrict` 必须进入 query 与 cursor binding；这是 authenticated following read，不用匿名 Web fallback | 两页 wire/response/adapter/SDK 已确认；仍须经 T12、T18、T29/T37 的兼容与发布门禁 |
| novel ranking | `GET /v1/novel/ranking` | `filter`、`mode` 按 ranking contract 传递；初始请求无 offset | required `novels` list → `Novel` | continuation key 为正 `offset`；ranking `mode` 是 operation 参数，不建立全局 novel subtype；当前没有 production owner | 两页 live wire/response 已确认；adapter、SDK、CLI/MCP owner 尚缺，保持 `not_tested`/`scope_admitted` |

### T02 normalized novel DTO 与 null/empty/error 规则

- normalized `Novel` 与 public `NovelDTO` 只承载已存在的稳定字段：`id/title/caption/user/tags/published_at/updated_at/x_restrict/text_length/is_original/total_bookmarks/total_views/cover`。`PublishedAt` 只能由有效 upstream 时间映射，不能用当前时间、零值或猜测值补齐；detail 的 series 引用和 series operation 的 series metadata 另行保留，不能塞入普通 novel 字段。
- detail response 的 `novel`、series target response 的 `novel_series_detail` 与 `novels`、各 list operation 的 `novels` 都是 required。缺失或 JSON `null` 为 `MalformedUpstreamResponse`；空数组合法，normalized/public page 的 `Items` 必须是 non-nil empty slice。每一个 item 的 `novel.id` 和 `novel.user.id` 必须为正数；缺失、null 或非正身份字段均为 malformed。
- wire scalar 的 optional 规则按 operation 保持明确差异：detail 当前允许 `x_restrict`、`text_length`、`is_original` 指针缺失并映射为 `0/false`，但不伪造业务值；search list 当前 adapter 对这三个字段要求非 null，缺失/null 必须报 malformed。其他已确认 list adapter 的普通字段可落 Go 零值，但后续 owner 不得把零值重新解释为 upstream 明示值。
- `series_next`、`series_prev` 可以缺失或为 null；一旦存在，引用 ID 必须为正数，非法引用为 malformed。`novel_series_detail` 的 series ID 与 user ID 同样必须为正数，并保留 `caption`、`is_concluded` 等字段；不能因当前 v1 adapter 的宽松 list 解析而删除 v2 target 的 required 约束。
- `next_url=null` 是正常终止。非 null 但为空、无法解析、缺少本 operation 唯一 continuation key、与上一页重复、违反 allowlist/range 或把另一 operation 的 token 当作本 operation continuation，均为 `MalformedUpstreamResponse`。search/follow/ranking 的 `offset` 与 series 的 `last_order` 必须为正；recommended 允许 continuation `offset=0`；latest 的 `max_novel_id` 必须为正。
- 调用方参数错误（空 `word`、非法 search target/sort/duration、非正 novel/series ID、非法 ranking 参数或 restrict）返回 `InvalidArgument`，不发送 upstream request。传输、取消、鉴权、状态码和 upstream schema 错误保留真实分类；不得把 rejected path、匿名 fallback 或空列表伪装成成功。

### T02 evidence index

- 历史 novel operation 表与 verdict：本文件开头的 `novel-detail-v1` 至 `novel-ranking` 行；strict evidence 为 [`evidence/appapi-upstream.md`](evidence/appapi-upstream.md) 的 novel detail/series/content/latest/follow/recommended/ranking 行。
- 迁移目标、已拒绝路径和 T12 兼容边界：[`api-migration-verification.md`](api-migration-verification.md) 的“已确认的迁移依据”“尚未达到迁移门禁”“已拒绝或排除”与 T12 小节。
- 当前 normalized/SDK 对照：`internal/services/pixiv/endpoint/novel/{search,detail,series,timeline,recommended}`、`sdk/pixiv/{models.go,dto.go,request.go,ops_novel.go,validation.go}`；现有 endpoint/SDK/MCP fixture 仍按各自当前 v1/offset 行为记录，不能倒推目标实现已完成。
- 当前明确缺口：detail/series 的 v2 adapter/SDK、latest 的 `max_novel_id`、novel ranking owner、search period 和所有 operation 的完整 query/account/subtype binding，分别留给 T05/T07/T10/T12/T14/T18/T25/T29/T30/T37/T42 等任务。

### T02 与后续任务的边界

- T05 负责第二页 fixture、continuation allowlist、query/account/subtype binding，以及 latest `max_novel_id`、series v2 `last_order` 和 search period 的补证；T06 负责跨 operation error、鉴权和脱敏规则。
- T07/T10 负责 endpoint leaf、v2 path、DTO 和 error mapping；T14 负责 novel public SDK、series metadata、cursor 与源码兼容；T18 负责 latest/recommended/ranking/follow SDK 集成。T02 不改 protocol、adapter、SDK、CLI、MCP、依赖或默认值。
- T25/T29/T30/T31/T32/T37 在 T39A 后分别处理 novel search、timeline、ranking、detail、series 及 MCP 路由；`NovelContent`/`/v1/novel/content` 已 rejected，WebView 正文已 excluded，后续只允许按 T12 冻结的 deprecated/explicit-unsupported 兼容方案处理，不得 fallback。

## T03 bookmark 基础 contract 冻结（2026-09-07）

本节冻结 artwork/novel bookmark 的 list、tags、detail、add、remove，以及 `list/tags --type all` 的产品层 contract。目标 contract、历史 evidence 和当前生产实现严格分层：历史表中的 verdict 不改写；`not_tested`/`inconclusive` 的 operation 只能登记目标边界，不能新增 public route 或提升 capability 状态。`all` 是跨两个 endpoint 的聚合选择，不是 upstream subtype，也不能被 detail/add/remove 继承。

| Operation | Method / path | Base request | Normalized response | Continuation / subtype / mutation boundary | Evidence boundary |
| --- | --- | --- | --- | --- | --- |
| artwork bookmark list | `GET /v1/user/bookmarks/illust` | `user_id` required positive；`restrict` 为 `public/private`，产品层默认 `public`；`tag` optional；首请求不带 `max_bookmark_id` | required `illusts` list → normalized `Artwork`；空列表合法 | 续页只接受正 `max_bookmark_id`；不发送未经验证的 `type/content_type`；单类 cursor 不能用于 all | public/private wire、response、现有 adapter/SDK fixture 已有，但 strict live 第二页是 `pagination_exempt`/未观察，不能宣告完整分页发布 |
| novel bookmark list | `GET /v1/user/bookmarks/novel` | `user_id` required positive；`restrict` 为 `public/private`，产品层默认 `public`；`tag` optional；首请求不带 `max_bookmark_id` | required `novels` list → normalized `Novel`；空列表合法 | 续页只接受正 `max_bookmark_id`；novel list 不承载 artwork subtype；单类 cursor 不能用于 all | public/private wire、response、现有 adapter/SDK fixture 已有，但 strict live 第二页是 `pagination_exempt`/未观察 |
| artwork bookmark tags | `GET /v1/user/bookmark-tags/illust` | `user_id` required positive；`restrict` 为 `public/private`，产品层默认 `public`；首请求不带 `offset` | target required `tags` list → `BookmarkTag{name,count}`；`name` 非空；空列表合法 | 续页只接受正 `offset`；`type/content_type` 仍是未验证 candidate，不进入已发布 wire；single-stream tag cursor 不能用于 all | 当前 adapter 将缺失/null tags 宽松视为空结果，属于旧行为而非目标 contract；现有 fixture 仅覆盖单流，subtype/live evidence 尚缺 |
| novel bookmark tags | candidate `GET /v1/user/bookmark-tags/novel` | `user_id`、`restrict` 是已知 request 维度；其他参数、默认值与 continuation 不在 T03 擅自补造 | target 结果仍需保留 `name/count` 与 novel content kind；wire shape、null/empty 及分页需 T08 snapshot | 未冻结可发送 subtype 或 offset allowlist；在 T08 evidence/adapter/SDK 前不得新增 public operation | candidate 表明确 wire/response/空列表/adapter/SDK `not_tested`；保持 `scope_admitted` |
| artwork bookmark detail | `GET /v2/illust/bookmark/detail` | `illust_id` required positive；无分页 | `ArtworkBookmarkDetail{restrict,tags}`；未收藏是合法空状态（空 restrict、non-nil empty tags）；已收藏保留 restrict 与全部 tag name | 404、`bookmark_detail:null` 或明确 `is_bookmarked:false` 只在该 endpoint 归一为空状态；其他错误真实传播；不接受 all | 当前 wire/adapter/SDK fixture 已覆盖 bookmarked/unbookmarked/404；这是当前单类实现证据，不是 all 或 mutation read-back 证据 |
| novel bookmark detail | target `GET /v2/novel/bookmark/detail` | `novel_id` required positive；无分页 | target 需提供与 artwork detail 等价的 bookmark state（restrict、tags、明确的 absent 状态），不得把 Novel metadata/detail 混入 | absent/404 的归一化、tags shape 和错误映射必须先由 T08 snapshot 冻结；不接受 all | wire、response、public/private 状态、adapter/SDK 均 `not_tested`；T03 只冻结不 fallback、不公开未验证路径 |
| artwork bookmark add | `POST /v2/illust/bookmark/add` | `illust_id` required positive；`restrict` 为 `public/private`，空值仅由兼容层默认 `public`；`tags[]` optional、可重复、可为空 | mutation 不以 2xx/nil 单独证明状态；需 detail/list/tags read-back | 无 continuation；写前确认当前账号与目标权限，保存原状态；结果分为确定失败、写入后读回失败、结果不确定，后两类禁止自动重放 | 当前仅有窄 `PostForm`/单测，无 bookmark 真实 mutation/read-back evidence；T09A/T08/T15/T38 必须补齐 |
| novel bookmark add | target `POST /v2/novel/bookmark/add` | `novel_id` required positive；`restrict`、`tags` 的具体 wire optional/default 以 T08 strict snapshot 为准 | target 与 artwork mutation 同样要求 detail/list/tags read-back 和可观测 outcome | 无 continuation；不能因为旧 symbol 存在而调用未确认 path；不确定结果禁止重放 | novel bookmark mutation 当前未进入 strict manifest；保持 `not_tested` |
| artwork bookmark remove | `POST /v1/illust/bookmark/delete` | `illust_id` required positive；无 continuation | 通过 detail/list/tags read-back 确认删除状态，或明确暴露不确定结果 | 写前保存并校验原状态；只恢复/清理本轮目标，不删除既有其他 bookmark；不接受 all | 当前仅验证 form method/path/status，无真实隔离账号生命周期证据 |
| novel bookmark remove | target `POST /v1/novel/bookmark/delete` | `novel_id` required positive；无 continuation | 通过 novel detail/list/tags read-back 确认删除状态，删除后恢复原状态 | 无 fallback；不确定写入禁止猜测或重放；detail/list/tags read-back 是发布前必要条件 | path 是 candidate；wire、adapter、SDK、真实写后恢复均 `not_tested` |
| bookmark list --type all | product aggregate over artwork list → novel list | 输入身份为 user/user bookmarks；`--type all` 与 user URL 合法；显式 artwork URL 与 novel 类型冲突；统一 `tag/restrict/local filter` | 连接后的 normalized Artwork/Novel sequence，保留各自 content kind | 固定 artwork → novel；各流 upstream 顺序不变；Skip/Limit 只应用一次，不按类型分配配额；OneBatch 可跳过空流但不为凑类型额外抓批；aggregate cursor 记录当前流、两端 checkpoint、完成状态、query/account binding | 目前无 aggregate SDK/CLI/MCP handler、DTO、cursor 或 all fixture；目标为 required，仍 `scope_admitted` |
| bookmark tags --type all | product aggregate over artwork tags → novel tags | 输入身份与 list 相同；`all` 只适用于 tags；单类 `restrict`/user binding 必须同时作用于两流 | 每项必须带 `content_type=artwork|novel`、`name`、该流原始 `count`；同名 tag 不合并、不相加、不去重，流内顺序保留 | 固定 artwork → novel；统一 Skip/Limit/OneBatch；aggregate cursor 记录当前流、两个 tags checkpoint、完成状态和 binding；不持久化 next_url/token | novel tags 与 typed count 尚未有 upstream/adapter/SDK evidence；目前无 all output/atomicity fixture，仍 `scope_admitted` |

### T03 subtype、鉴权与聚合错误边界

- T20 的三层语义继续生效：bookmark list/tags 的 Target kind 是 user/user bookmarks，Result kind 是 artwork 或 novel，`illust/manga/ugoira` 只属于 artwork subtype；`all` 不是 subtype。现有 artwork bookmark endpoint 没有已验证的 `type/content_type` wire，因此在 evidence 前只允许将 subtype 作为目标 contract/candidate，不得静默发送或宣告 server-side filtering。若后续改为 client-side filter，subtype 与 local filter 必须进入 cursor binding，并按逻辑分页计数。
- `restrict` 的产品选择只有 `public/private`，新 CLI/MCP/aggregate route 默认绑定 `public` 并把实际选择写入每个流的 query/cursor。`private` 只允许 verified authenticated account 的自身 bookmark scope；跨账号 private、缺少认证或上游 403 必须显式拒绝/保留真实鉴权分类，不得改请求为 public、匿名 Web fallback 或换另一个 user 重试。既有 SDK 空 `Restrict` 的源码兼容与默认处理留给 T12；不因此删除旧 named type 或改变旧 wire。
- 已冻结的 list `illusts`/`novels` 与 artwork tags 的目标 `tags` list 都是 required：缺失或 JSON `null` 为 `MalformedUpstreamResponse`，`[]` 合法且 public page `Items` 为 non-nil empty slice。现有 artwork tags 的 null-as-empty 仅记录为 legacy adapter 行为，不能成为 all 成功的默认值；novel tags 的 required/null/empty 仍待 T08 snapshot，不得从 candidate path 推断。`next_url=null` 正常结束；非 null 空值、重复/缺失/非本 operation allowlist 的 continuation、非正 `max_bookmark_id`/`offset` 均 malformed。detail 的明确 absent state 是合法结果，但矛盾字段或未验证 novel detail shape 不能猜测归一。
- 所有显式 ID 必须为正数；URL/structured record/`--type` namespace 冲突、非法 restrict/subtype 或 detail/add/remove 使用 all 返回 `InvalidArgument`。upstream 401/403、transport、取消、429、非成功状态和 schema 错误保留真实分类；不得把错误流、空 response 或 mutation 的 2xx 当作成功空列表。
- 一次 `--type all` 逻辑页需要的任一流失败，整页失败，不返回已收集的成功子集；JSON/NDJSON 必须在两个流都完成该逻辑页收集后再提交。aggregate cursor 不得携带 next_url、signed URL、token、cookie、原始查询或用户内容；现有 `sdk.Cursor` 的 binding 也不是鉴权或密码学防篡改证明。

### T03 evidence index

- 历史 list rows 与 verdict：本文件开头的 `user-illust-bookmarks-public/private`、`user-novel-bookmarks-public/private`；strict evidence 为 [`evidence/appapi-upstream.md`](evidence/appapi-upstream.md) 对应行，pagination 仍按 data-limited/未观察第二页处理。
- 已确认/候选迁移目标：[`api-migration-verification.md`](api-migration-verification.md) 的“尚未达到迁移门禁”与“已拒绝或排除”表；novel tags/detail/add/delete、artwork tags/subtype 均不得在 snapshot 前新增 public operation。
- 当前 artwork leaf 与 normalized 规则：`internal/services/pixiv/endpoint/artwork/bookmark/bookmark.go` 及 `sdk/pixiv/ops_artwork.go`、`sdk/pixiv/ops_mutation.go`；当前 novel list 对照为 `internal/services/pixiv/endpoint/user/novelbookmarks/novelbookmarks.go` 与 `sdk/pixiv/ops_novel.go`。这些 fixture 证明既有单流行为，不证明 all/mutation contract。
- 聚合、类型和错误目标：[`cli-migration-matrix.md`](cli-migration-matrix.md) 的 Target/Result/Subtype 与 `bookmark list/tags --type all` 段；鉴权、写前检查、读回、恢复和不确定 outcome 见 [`mutation-validation-report.md`](mutation-validation-report.md)。

### T03 与后续任务的边界

- T05 补两类 list/tags 的第二页或合成两页 fixture、continuation allowlist、query/account/restrict/subtype binding；T06 冻结跨账号 private、鉴权错误、mutation outcome 与脱敏分类。
- T08 实现并验证 novel tags/detail/add/delete 及 artwork tags/subtype 所需 endpoint leaf、DTO、null/empty/error；T09A 提供可解码 mutation response 的窄 form transport，保留既有 `PostForm`。
- T12/T15 冻结 explicit artwork/novel SDK symbol map、旧 `AddBookmark`/`RemoveBookmark` wrapper、named type 与空 restrict 兼容；T19/T23 负责 aggregate cursor、双流 checkpoint、统一 Skip/Limit/OneBatch 与失败原子性。T03 不改 production code、protocol、SDK、CLI、MCP 或依赖。
- T21 负责 user URL/user bookmarks URL/显式类型 resolver；T27/T37/T38 分别实现 CLI/MCP 的 all、typed tags、页原子输出、旧 wire 回放和 mutation read-back/outcome；T39A/T41/T42/T43/T44/T45 负责 compatibility、双语文档、回归、隔离账号 live 和最终发布门禁。

## T04 comment 与 stamp 基础 contract 冻结（2026-09-07）

本节冻结 artwork/novel comments 的 read、text/reply/stamp/delete mutation、stamps read 与 `total` 元数据 contract。目标 contract、当前 adapter/SDK fixture、历史 live mutation 和 strict upstream evidence 严格分层：既有 `rejected`/`inconclusive`/`not_tested` verdict 不改写，历史 HTTP 200/read-back 不直接授予 public route。当前 comments v3/v2 版本选择、日期/access-control wire 与 mutation response ID 未完全证实时，不得以 fallback、空成功或自动重放掩盖缺口。

| Operation | Method / path candidate | Base request | Normalized response / result | Continuation / mutation boundary | Evidence boundary |
| --- | --- | --- | --- | --- | --- |
| artwork comments read | current `GET /v3/illust/comments` | `illust_id` required positive；`offset` 只作为 continuation；历史曾带 `include_total_comments`，是否需要不得从旧记录推断 | target required `comments` list → `Comment{id,user,body,CreatedAt,parent}`；可选 `total` 与 `access_control` | `next_url=null` 正常结束；非 null 只允许经过 adapter 验证的正 `offset`；不保存 raw URL | 当前 adapter/SDK fixture 与 strict wire/response 有覆盖，但 strict 第二页未观察；历史 matrix 将 `illust-comments-v3` 标为 `rejected`，strict row 为 `inconclusive`（date/numeric access-control/分页风险），均不能放行 release |
| novel comments read | current `GET /v2/novel/comments`；`/v3/novel/comments` 仅 candidate | `novel_id` required positive；`offset` 只作为 continuation；不发送未经 snapshot 确认的额外 total 参数 | target required `comments` list → 同一 normalized comment shape；可选 `total` 与 `access_control` | 只接受本 operation 的正 `offset`；v2/v3 不自动 fallback 或混合 cursor | v2 有当前 adapter/SDK fixture，但 strict overall `inconclusive`（空首批/未观察第二页）；v3 wire/response 仅 candidate/inconclusive，adapter/SDK 未测试 |
| artwork comment text create | candidate `POST /v1/illust/comment/add` | `illust_id`、`comment` required；无 `parent_comment_id`/`stamp_id` | 必须从严格冻结的 response 解码本轮正 `comment_id`，再由 read-back 确认；2xx/nil 不是状态证明 | 无 continuation；写前检查可评论/权限与目标 namespace；结果区分明确失败、已得 ID 但读回失败、结果不确定 | 历史非主账号 wire/read-back 200 仍为当前 production `not_tested`；现有 `PostForm` 不交付 body/ID，T09A/T09/T16 必须补齐 |
| artwork comment reply | candidate `POST /v1/illust/comment/add` | `illust_id`、`comment` required；`parent_comment_id` required positive；parent 必须属于同一 artwork scope | 返回并读回新 comment ID；parent chain 只作为 comment result 的递归关系，不把 parent ID 当作品 ID | 无 continuation；写前确认 parent/权限；不确定结果禁止自动重放或猜测最近评论 | 历史 wire/read-back 记录确认额外 `parent_comment_id`，但 adapter/SDK/CLI/MCP 未测试 |
| artwork comment stamp | candidate `POST /v1/illust/comment/add` | `illust_id`、`comment` required；`stamp_id` required positive；stamp ID 不得猜测 | 同 text/reply：response ID + 同账号 read-back；stamp 引用字段需由 stamps snapshot 冻结 | 无 continuation；只接受目标 artwork namespace；不确定结果禁止重放 | 旧 `comment-stamp-write` 有单条 read-back 记录，但未进入当前 strict comment mutation manifest；不提升为 production capability |
| novel comment text create | candidate `POST /v1/novel/comment/add` | `novel_id`、`comment` required；无 parent/stamp | 必须取得 response 中冻结的正 comment ID 并 read-back；2xx/nil 不等于成功 | 无 continuation；同账号写入/读回/清理；明确失败与不确定结果分开 | 历史 wire/read-back 200，但 current adapter/SDK/CLI/MCP 全部 `not_tested` |
| novel comment reply | candidate `POST /v1/novel/comment/add` | `novel_id`、`comment` required；`parent_comment_id` required positive；parent 必须属于同一 novel scope | response ID + novel comments read-back；parent scope 必须与 novel 一致 | 无 continuation；Shaft 仅提供形态参考，不凭此补 live response/schema | 只有候选/Shaft 依据，当前 strict mutation manifest 无独立 case；保持 `candidate/not_tested` |
| novel comment stamp | candidate `POST /v1/novel/comment/add` | `novel_id`、`comment` required；`stamp_id` required positive；stamp 不能脱离 stamps contract 猜测 | response ID + read-back | 无 continuation；结果不确定禁止重放 | 历史 wire/read-back 200，但当前 production adapter/SDK/CLI/MCP `not_tested` |
| artwork comment delete | candidate `POST /v1/illust/comment/delete` | `comment_id` required positive；comment namespace 必须可证明属于 artwork | read-back 确认目标消失/状态变化；不能用 2xx 单独证明删除 | 写前 access check；只清理本轮确认 ID，不删除既有评论；不确定结果禁止猜测或重放 | 历史删除/read-back 200；当前 production owner、adapter/SDK/CLI/MCP 仍 `not_tested` |
| novel comment delete | candidate `POST /v1/novel/comment/delete` | `comment_id` required positive；comment namespace 必须可证明属于 novel | read-back 确认删除；不把空列表或网络失败当删除成功 | 同 artwork delete；同账号 execution context；不跨 namespace fallback | 历史删除/read-back 200；当前 production owner、adapter/SDK/CLI/MCP 仍 `not_tested` |
| stamps read | `GET /v1/stamps` | 当前 strict evidence 无 query 参数；不发送未验证 selector | target required `stamps` list，但 item 字段/ID/资源引用必须先由 T17 snapshot 冻结；不凭 schema fingerprint 设计 public DTO | strict evidence 为 no continuation；若后续发现 continuation，必须另行 snapshot，不把它混入当前无分页 contract | wire/response/page=40/no continuation confirmed；adapter/SDK/CLI/MCP 与字段级 DTO 均 `not_tested`，production owner missing |
| artwork comments total metadata | 同 artwork comments read endpoint | `total_comments` 是否要求 `include_total_comments` 未冻结；默认不向 public surface 承诺请求该参数 | `CommentPage.Total *int64` 仅在 upstream 显式给出时非 nil；缺失/null 不变为 0；total 不是分页完整性证明 | total 不产生独立 cursor，也不保证与 comments/items 实时一致 | 历史 `include_total_comments` 请求与 strict 记录均为 item_count=0/`inconclusive`；非空 total、参数必要性、两页一致性待补 |
| novel comments total metadata | 同 novel comments read endpoint（v2/v3 选择尚未定） | `total_comments`/include 参数同上；不把 v3 candidate 当默认 | 同上，保持 optional pointer/non-strong guarantee | 不单独续读；不得以 missing total 推断无评论 | novel v2/v3 total 均缺非空/第二页 evidence，状态 `inconclusive`；T05/T06/T09/T16 冻结后续语义 |

### T04 normalized comment、空值与错误边界

- 目标 comments response 的 `comments` 是 required list：缺失或 JSON `null` 为 `MalformedUpstreamResponse`，`[]` 合法且 normalized `Items` 为 non-nil empty slice。当前 artwork/novel adapter 对缺失/null 通过 Go nil slice 生成空结果，只能登记为 legacy 宽松行为，不能成为新 contract 的静默成功。每个 comment 与 parent chain 的 ID 必须为正数；不增加无依据的递归深度或正文长度截断。
- normalized comment 必须保留正 `ID`、用户、正文、有效创建时间和可选 parent chain。当前 fixture 在 `comment` 为空时回退 `caption`，而 Shaft/live 风险记录使用 `date`、当前 adapter 使用 `created_at`；字段别名、正文空字符串/null、用户字段 requiredness 和日期解析必须由 strict snapshot/owner fixture 冻结，不能把当前 fallback 当作对未知 wire 的通用兜底。无效时间、矛盾 parent 或已确认的 schema 错误真实报错。
- `access_control` 是可选元数据：缺失/null 保持 nil，不等价于 `false`；当前 adapter 的 bool shape 与历史 live numeric access-control 观察不一致，必须先冻结 wire 到 normalized mapping，不能丢字段或把“不能评论”伪装成“没有评论”。读取权限错误、403 与合法空列表保持不同分类。
- `total_comments` 只映射为 optional `*int64`；缺失/null/未验证不能写成 0，total 也不作为“已取完所有 comments”的证明。`include_total_comments` 是历史请求字段，不在 T04 默认 public wire 中擅自保证；T05/T06 负责非空 total、绑定和错误 evidence。
- `next_url=null` 是正常结束；非 null 空字符串、解析失败、缺少/重复/非本 operation allowlist 的 `offset`、非正或溢出 offset 均 malformed。continuation 只由 adapter 转成 sanitized state；SDK/CLI/MCP 不保存 next_url、token、cookie、签名 URL、原始 query 或评论正文。

### T04 mutation、类型与输出边界

- comments read/create 的父 Target 是 artwork 或 novel，Result kind 是 comment；reply/delete 的 Target kind 是 comment；`text`、`reply`、`stamp` 是 operation type，不是 artwork subtype。所有作品、评论、parent、stamp ID 必须为正数并保持 namespace；`all` 不适用于 comment detail/mutation，URL、structured record 与显式类型冲突返回 `InvalidArgument`，不做隐式换类型重试。
- comment body 是 mutation 的 required input；JSON null/missing 必须拒绝。空字符串/空白的业务接受性、服务端长度边界与 response body 字段不能凭空硬编码，必须由 T09/T16 snapshot/测试冻结；不截断合法正文。`parent_comment_id`、`stamp_id` 的存在关系与目标 scope 必须在写前验证。
- mutation 必须先确认同一账号 execution context 对目标具备写/删权限，保存所需原状态；创建必须从可解码 response 取得本轮正 comment ID，再做同账号 read-back；读回失败、已得 ID 但无法证明状态和无可靠 ID 的不确定结果分别暴露，禁止自动重放、猜测最近评论或删除既有数据。delete 只允许清理本轮确认 ID，并验证删除后的目标状态。
- 当前 CLI `comment ID --type artwork|novel`、MCP `illust_comments`/`novel_comments` 的输入字段、JSON/NDJSON/structured envelope 与 `total`/`access_control` optional 形状继续保留；新 create/reply/stamp/delete/stamps surface 必须经 T39A 逐项冻结，不得把旧 read tool 的 `id` 或旧 DTO 静默改成通用 TARGET。read 逻辑页任一 continuation/adapter 错误时不输出已收集成功子集。
- 401/403、取消、transport、429、非成功状态和 schema 错误保持真实分类；不得把 error、空 response、mutation 2xx 或缺失 total 当成功空列表。MCP stdout 继续只承载 JSON-RPC，comment DTO、stamp DTO 和 cursor 不得泄露 token、cookie、header、原始 URL 或用户凭据。

### T04 evidence index

- 当前 comments adapter/SDK 与 fixture：`internal/services/pixiv/endpoint/artwork/comments/comments.go`、`internal/services/pixiv/endpoint/artwork/comments/comments_test.go`、`internal/services/pixiv/endpoint/novel/comments/comments.go`、`internal/services/pixiv/endpoint/novel/comments/comments_test.go`、`sdk/pixiv/ops_artwork.go`、`sdk/pixiv/ops_novel.go`、`sdk/pixiv/models.go`、`sdk/pixiv/dto.go`。
- 历史 comments/stamps strict rows：本文件开头 `novel-comments-v2`、`novel-comments-v3`、`illust-comments-v3`、`stamps`、六个 comment mutation rows；strict evidence 为 [`evidence/appapi-upstream.md`](evidence/appapi-upstream.md) 对应行，其中 comments 第二页/total 与 artwork date/access-control 风险不能省略。
- mutation live/read-back 与当前生产门禁：[`mutation-validation-report.md`](mutation-validation-report.md) 的既有六类 comment 写入/删除、[`evidence/appapi-mutation.md`](evidence/appapi-mutation.md) 的 adapter/SDK `not_tested` rows，以及 [`api-migration-verification.md`](api-migration-verification.md) 的 comment add/reply/delete、stamps 和 T09A 条款；历史 `comment-stamp-write` 仅保持其原 verdict。
- total、旧读取和候选字段：[`evidence/appapi-read.md`](evidence/appapi-read.md) 的 `comments-total`/`comment-stamp-write`、[`evidence/appapi-read.json`](evidence/appapi-read.json) 的 `include_total_comments` 记录、[`shaft-protocol-diff.md`](shaft-protocol-diff.md) 的 date/access-control/add/reply/stamps 对照；这些只提供候选/历史依据，不能替代 strict snapshot。
- CLI/MCP 兼容与类型边界：[`cli-migration-matrix.md`](cli-migration-matrix.md) 的 comments Target/Result/type 与旧 `comment` surface；现有实现/fixture 为 `internal/cli/commands/pixiv/comment`、`internal/mcpserver/pixiv/tools/{illust_comments,novel_comments}`、`internal/mcpserver/pixiv/internal/outputs/outputs.go`，只证明当前 read surface。

### T04 与后续任务的边界

- T05 补 comments 两页/非空目标、offset allowlist、query/account binding、重复 cursor 和 total continuation 证据；T06 冻结 401/403、numeric access-control、null/empty、脱敏与 mutation outcome 分类。
- T09A 提供可解码 form response 的窄 transport，保留旧 `PostForm`；T09 实现 artwork/novel comments read/create/reply/stamp/delete endpoint leaves、DTO 和错误映射，不能用当前 error-only mutation 当成功证明。
- T12/T16 冻结 explicit comment SDK symbol、旧 read request/model/DTO 与 wrapper/compatibility；T17 实现 stamps model/DTO/SDK；T19/T23 接入 comment continuation、logical pagination 与页原子性。
- T21 负责 comment URL/structured record/显式类型 resolver；T33 实现 CLI read/mutation/stamps 迁移与输出；T37/T38 分别实现 MCP read/mutation tools、旧 JSON 回放、structured error 与同账号 read-back。
- T39B/T40/T41/T42/T43/T44/T45 负责实施后兼容、completion/docs、协议/SDK/CLI/MCP 回归、隔离账号 live 与最终发布门禁；T04 不修改 production code、protocol、SDK、CLI、MCP 或依赖。

## T05 continuation, binding, and page-2 fixture contract freeze (2026-09-07)

本节在 T01–T04 的 operation contract 之上冻结 continuation allowlist、初始页/续页差异、query/account/subtype binding 与第二页 fixture 规则。它复用现有 `sdk.Cursor`、`sdk/pixiv` 的 product/operation/version/query/account/client binding，以及 `internal/shared/pagination` / `internal/shared/traversal`；不新增 cursor 编码、通用 checkpoint API、CLI/MCP 自行解析 `next_url` 或 production behavior。历史 evidence 的 `confirmed`、`pagination_exempt`、`inconclusive`、`not_tested` 和 `rejected` verdict 均保持原样。

### T05 continuation allowlist 与初始页/续页差异

| Operation family | Target sanitized continuation allowlist | 初始页与续页 | 第二页 fixture / evidence boundary |
| --- | --- | --- | --- |
| artwork search | `offset`，正整数；四种已验证 content type 共用该 operation key | zero cursor 首请求不发送 `offset`；续页只发送正 `offset`，重复原始 search query | live `search-illust-{all,illust,manga,ugoira}` 均 confirmed、30/30、跨页无重复；SDK/CLI/MCP 两页 fixture 见 `goal-3/evidence/appapi-upstream.md:20-23`、`sdk/pixiv/pixiv_test.go:493-543`、`internal/cli/commands/pixiv/search/bookmark_test.go:180-255` |
| artwork series | `last_order`，正 `int64` | 首请求只带 series ID；续页只带正 `last_order` | endpoint/SDK 两页合成 fixture 已有；独立 live 第二页未观察，保持 `inconclusive`，见 `sdk/pixiv/pixiv_test.go:276-320`、`goal-3/pagination-validation-report.md:5-12` |
| artwork latest | target key 为 `max_illust_id`；当前 adapter 仍兼容解析 `offset`，但新 target request 不得以 `offset` 替代 | 首请求不带 continuation；续页沿用服务端返回的唯一 typed key；`content_type` 属于 base binding | `illust` live 30/30 confirmed；扩展 subtype 仍 partial，见 `goal-3/evidence/appapi-upstream.md:26`、`internal/services/pixiv/endpoint/artwork/timeline/timeline_test.go:65-81` |
| artwork ranking / following / MyPixiv / user artworks / related / comments | `offset`，正整数；ranking 的 `mode/date`、user 的 `user_id/type`、follow 的 `restrict` 不属于 continuation | 首请求省略 `offset`；续页只发送正 `offset` | ranking live 30/30 confirmed；related/comments、user/feed 的 live 第二页不据 fixture 推为 confirmed，保留各自历史 verdict |
| artwork recommended | `offset`，允许续页值 `0`；“无 cursor”与“cursor=0”必须可区分 | 无 cursor 时不发送 `offset`；合法续页即使为 `offset=0` 也显式发送 | initial/continuation endpoint fixture 已有；live 85/0、第二页错误，状态 `inconclusive`，见 `internal/services/pixiv/endpoint/artwork/recommended/recommended_test.go:24-49`、`goal-3/evidence/appapi-upstream.md:25` |
| novel search / follow / MyPixiv / user novels / user relations | `offset`，正整数；`word/search_target/sort/duration`、`restrict`、`user_id` 等 base query 不得被续页替换 | 首请求省略 `offset`；续页只发送正 `offset` 并复用 base query | novel search 只有 SDK 合成两页，live period/两页尚缺；follow live 30/30 confirmed；user novels 是 `pagination_exempt`，仍须保留合成逻辑两页 |
| novel recommended / user recommended | `offset`，允许续页值 `0`；同 artwork recommended 的 presence 规则 | 无 cursor 时不发送 `offset`；续页 `offset=0` 必须显式发送 | novel recommended live 33/33 confirmed；user recommended 只有 endpoint/SDK 级 fixture，不能提升 live verdict |
| novel latest | target key 为 `max_novel_id`；不得把当前实现的 `offset` 当兼容替代 | 首请求无 continuation；续页必须按 upstream `max_novel_id` 重建请求 | live 30/30 只证明 upstream continuation；当前 adapter/SDK 仍用 `offset`，续页 rejected，保持 `inconclusive`，见 `goal-3/evidence/appapi-upstream.md:11`、`goal-3/pagination-validation-report.md:14-20` |
| novel series v2 | target key 为 `last_order` | 首请求只带 series ID；续页只带正 `last_order`；不得从 v1 fallback 或混用 cursor | v2 path/首批证据存在，第二页未观察，生产仍 v1，保持 `inconclusive`，见 `goal-3/evidence/appapi-upstream.md:7-8` |
| novel comments | `offset`，正整数；v2/v3 cursor 不互换 | 首请求只带 novel ID；续页只发送正 `offset` | endpoint fixture 与 artwork comments SDK 两页 fixture 不能替代 novel comments live 第二页；当前 strict verdict `inconclusive` |
| artwork / novel bookmark list | `max_bookmark_id`，正 `int64` | 首请求不带 key；续页只发送正 `max_bookmark_id`，保留 `user_id/restrict/tag` | 两类真实数据均 `pagination_exempt`/未观察第二页；SDK 两页 fixture 见 `sdk/pixiv/pixiv_test.go:991-1041,1089-1133`，不得改写 live verdict |
| artwork bookmark tags | `offset`，正整数 | 首请求不带 key；续页只发送正 `offset`，保留 `user_id/restrict` | SDK 两页 fixture 已有；wire/subtype/live evidence 尚缺，仍按 T03 `not_tested`/`pagination_exempt` 边界处理 |
| novel bookmark tags | continuation allowlist 未冻结 | 不猜测 candidate path 的 query、key、range 或 null/empty 行为 | 留给 T08 strict snapshot；T05 不新增 public operation 或 candidate fallback |
| detail、ugoira metadata、stamps、comment/bookmark mutation | none | 不产生 continuation；mutation 的 read-back 是 outcome，不是分页 cursor | stamps live no-continuation 已确认，但 adapter/SDK/DTO 仍 `not_tested`；其他 operation 继续沿用 T01–T04 rejection boundary |
| bookmark `--type all` / tags `--type all` | product aggregate cursor 由每个已冻结流的 typed checkpoint 组成；不能直接复用单流 cursor | 固定 artwork → novel；统一 Skip/Limit/OneBatch；单流完成状态与两端 binding 必须进入 aggregate state | 当前无 aggregate SDK/CLI/MCP fixture；不得携带 raw `next_url`、token、cookie、原始 query 或用户内容，留给 T19/T23 |

规则解释：`next_url=null` 或缺失表示正常结束；非 null 空字符串、无法解析、缺少/重复 key、非本 operation allowlist、值越界或另一 operation 的 key 均为 malformed target。recommended 的 `offset=0` 是唯一已冻结的零值续页特例。当前多数单 key adapter 会忽略未知额外 query key，且 endpoint parser 不保存上一页状态；这两项是实现缺口，不得在 T05 记录中伪称已由 leaf parser 完成，后续 owner/T23 必须用测试补齐。

### T05 query、account 与 subtype binding

- 外层 `sdk.Cursor` 固定携带 `product`、`operation`、product binding version 与 query digest；identity-scoped operation 额外携带 verified identity 或 ephemeral client binding，public-scoped operation 不携带账号/实例 binding；payload 只保存 typed continuation `Key`、数值 `Value`，`SearchArtworks` 额外保存批内 `Consumed`。query digest 排序后计算，排除 continuation 本身；改变 product、operation、binding version、query 或 payload kind 都返回 `InvalidCursor`，不得静默从第一页重启。证据为 `sdk/cursor.go:9-35,177-207`、`sdk/pixiv/cursor.go:56-171`。
- query digest 必须覆盖所有会改变结果序列的 base/query/local semantics：artwork search 的 `word/target/sort/duration/date bounds/content type`、novel search 的 `word/target/sort/duration`、`SearchArtworks` 的 AI mode、aspect/resolution/tool/bookmark bounds 与 `CursorContext`；尚未进入 strict manifest 的 novel search `start_date/end_date` 不得被假定为已冻结字段；paged series/comments 的 target ID；ranking 的 `mode/date`；follow/bookmark 的 `restrict`；bookmark 的 `user_id/tag`；user artworks 的 `user_id/type`。`CursorContext` 只参与 digest、不发送给 upstream；本地 subtype/filter 改变时必须改变 context。现有字段核对见 `sdk/pixiv/request.go:103-123`、`sdk/pixiv/ops_artwork_search.go:74-136`、`sdk/pixiv/ops_artwork.go:58-230`、`sdk/pixiv/ops_novel.go:1-225`。
- account binding 以 operation 的真实账号语义为准：`identityScopedOps` 中的 verified `Open/OpenWith` cursor 绑定正 user ID，未验证 `New/NewWith` 只能由同一 client instance 续读；pool attempt 换账号时必须丢弃旧结果和 cursor，不能把旧账号中间页交给新账号。T12 冻结 `SearchArtworks`、CurrentUser、following/recommended/related 等为 identity-scoped；`SearchNovels`、`SearchUsers` 则明确为 public-scoped，只绑定 product、operation、version 与 query，不绑定账号或 client instance。双语 SDK 文档已按此决定修正，并由跨 client 合成回归锁定，不能再把“所有搜索均已账号绑定”作为契约。证据为 `sdk/pixiv/cursor.go:37-54`、`sdk/pixiv/pixiv.go:131-201`、`sdk/pixiv/pixiv_test.go:229-274`。
- subtype binding 只允许已证实的语义进入 cursor：`SearchArtworks` 的 `ContentType`、AI/local filter context，latest 的 resolved `content_type`，user artworks 的 `type`；ranking `mode` 保持 ranking 参数，不伪装成全局 artwork subtype。CLI recommended 的 `content-type` 与 MCP `illust_filter.type` 当前没有进入 `RecommendedArtworksRequest`/SDK digest，虽然单次 MCP 在 runtime 做本地过滤，未来任何可持久化/跨请求 cursor 前必须补 subtype binding 与两页 fixture；artwork bookmark subtype 仍是 candidate，不得静默发 wire。
- cursor 编码是 JSON + Base64URL，不是 MAC/签名，也不是鉴权凭据；不可信边界仍须重新校验上述 binding，错误和输出不得包含 token、cookie、signed URL、raw `next_url`、原始 query 或用户内容。现有 envelope 禁止项与安全警告见 `sdk/cursor.go:15-28`、`docs/zh-CN/sdk.md:173-178`。

### T05 第二页 fixture 规则与现有证据

每个需要分页的 owner 至少要保留以下合成回归：首响应带 required list 与合法 continuation，第二请求只携带本 operation allowlist key 并复用不可变 base query，第二响应再次具备 required list，终止响应为合法空 list + `next_url=null`；另有缺失/null list、空 continuation、重复 key、未知 key、非法 range、upstream error 与 query/account/subtype changed-binding 失败样例。fixture 只验证当前 operation 的行为，不把 endpoint 的 `next_url` 文本泄露到 public cursor。

- live second-page confirmed 仅包括 `novel-follow`、`novel-recommended`、artwork search 四种 content type、`artwork latest` 与 `artwork ranking`；这些 case 的 item count、`page_overlap=0` 和 adapter/SDK verdict 见 `goal-3/evidence/appapi-upstream.md:11-14,20-27`。`page_overlap=0` 对没有真正第二页的 case 不构成成功证明。
- 已有合成两页覆盖包括 `SearchArtworks`、artwork/novel series、artwork/novel bookmarks、artwork bookmark tags、artwork comments、novel search，以及 CLI/MCP search 的逻辑页/批内 checkpoint；入口见 `sdk/pixiv/pixiv_test.go:135-182,276-414,493-543,991-1133`、`internal/shared/pagination/pagination_test.go:87-289,361-419`。novel comments 当前只有 endpoint 单次 continuation fixture，不得写成 SDK 两页已覆盖。
- `user novels`、`user artworks`、public/private bookmarks 是数据受限的 `pagination_exempt`，只放宽真实 live 第二页观测，不放宽合成逻辑分页、binding、错误和 checkpoint 回归。`illust-recommended`、`novel-series-v2`、comments 与 `novel-new` 的第二页/迁移状态仍按 `inconclusive` 或真实 failure class 保留，见 `goal-3/pagination-validation-report.md:14-54`。
- shared pagination 在再次 fetch 前按 opaque cursor 文本检测重复/环路，并在 fetch/consume 错误时丢弃已收集结果；它不解码 SDK cursor。`CheckpointSearchArtworks` 是当前唯一明确的批内 checkpoint API，其他 operation 只复用 upstream continuation；不得为补 fixture 临时发明通用 `Checkpoint*`。

### T05 未关闭项与后续 owner

- `novel-new` 必须由 T10/T14/T18 按 `max_novel_id` 完成 adapter/SDK 迁移；当前 `offset` 续读失败不能通过 fallback、重试或空结果隐藏。
- recommended artwork 的 subtype/filter binding 与两页证据由 T07/T13/T18/T23 关闭；在此之前 CLI/MCP 不得把 `illust`/`manga` 的本地过滤描述成 upstream subtype continuation contract。
- `SearchNovels`/`SearchUsers` 的 account binding 由 T12 冻结为 public-scoped；保留跨 client 恢复回归，不新增 identity binding 或 cursor version bump。若未来产品语义要求改为账号作用域，必须先重新提交兼容决策并单独提升 operation binding version，不能静默改变现有 cursor 行为。
- 非本 operation 的额外 continuation query key 与跨页重复 upstream cursor 的拒绝，需要 leaf parser/共享 traversal 的专门回归；当前仅有同一 URL 内重复参数拒绝和 shared 层重复 cursor 检测，不能把两者混为一谈。
- T05 只冻结 contract/fixture，不改变 capability admission；所有 required capability 继续由 `capability-admission.md` 授权，当前 Goal 仍 incomplete。

## T06 error 与其他 read contract 冻结（2026-09-07）

本节冻结 user、trending、MyPixiv 与 follow mutation 的 request/DTO/分页、鉴权、错误、结果和脱敏边界；不把当前 endpoint/SDK fixture 或历史 live evidence 直接提升为 `public_ready`。user artworks/novels 的真实数据受限仍沿用 `pagination_exempt`，但不能豁免合成两页、binding、错误和输出原子性回归。T06 不新增 endpoint、public symbol、CLI/MCP wire 或匿名 fallback。

### T06 user、trending 与 MyPixiv read contract

| Operation family | Method / path | Base request | Normalized response | Continuation / access boundary | Evidence boundary |
| --- | --- | --- | --- | --- | --- |
| user search | `GET /v1/search/user` | `word` 必须非空；首请求不带 `offset`，续页只带正 `offset` | required `user_previews` list → `UserPreview{User}`；每个 user ID 必须为正数；空数组是合法成功结果 | `next_url=null` 正常结束；非 null 只接受本 operation 的唯一正 `offset`；搜索账号 binding 当前仍是 T05 登记的 P1 gap | endpoint/SDK fixture 已有（`internal/services/pixiv/endpoint/user/search`、`sdk/pixiv/pixiv_test.go`）；strict live case 未进入 `evidence/appapi-upstream.md`，不据 fixture 提升 verdict |
| user detail / current user | `GET /v1/user/detail` | 目标 user ID 必须为正数；`CurrentUser` 只能使用已验证 client identity 并附 `filter=for_android` | `user`、`profile`、`profile_publicity`、`workspace` 四个 envelope 都是 required object；缺失、null、结构非法或 user ID 非正数为 malformed；profile publicity 只接受 bool 或 `public/private` 形式 | 无 continuation；当前用户身份未知返回 `Unauthorized`，不得从 token 猜 ID 或切换账号 | detail endpoint、SDK DTO、MCP structured fixture 已有；strict live row 未登记，profile optional/visibility 语义以 owner fixture 为准 |
| user artworks | `GET /v1/user/illusts` | user ID 必须为正数；`type` 是 command-specific artwork subtype，public `artwork/illustration` 映射 upstream `illust`；不支持的 subtype 返回 `InvalidArgument`；首请求不带 `offset` | required `illusts` list → `Artwork`；item ID 必须为正数；空数组合法且保持 non-nil `Items` | continuation 为正 `offset`，`user_id/type` 是 immutable base binding；不能把 user target 当成 artwork subtype | wire/adapter/SDK fixture 与历史 `user-illusts-{illust,manga}` row 已有；live 第二页未观察，继续 `inconclusive`/`pagination_exempt` 边界 |
| user novels | `GET /v1/user/novels` | user ID 必须为正数；adapter 固定 `filter=for_android`；首请求不带 `offset` | required `novels` list → `Novel`；novel ID 与 nested user ID 必须为正数；空数组合法且保持 non-nil `Items` | continuation 为正 `offset`，`user_id` 不得被续页替换；这是 public user read，不把 private bookmark scope 混入 | wire/adapter/SDK fixture 与历史 `user-novels` row 已有；数据受限，live 第二页未观察，继续 `inconclusive`/`pagination_exempt` |
| user relationships | `GET /v1/user/following`, `GET /v1/user/follower`, `GET /v1/user/related`, `GET /v2/user/list` | following/follower 使用正 `user_id` + `restrict=public|private`；related 使用正 `seed_user_id`；blocked 使用正 `user_id` + `filter=for_android`；首请求不带 `offset` | required user list（`user_previews`，blocked 兼容 `users`）→ `UserPreview{User}`；每个 user ID 必须为正数；空数组合法 | 所有 list continuation 只接受正 `offset`；restrict、target/seed ID 和 blocked scope 是 binding；跨账号 private、403、缺少认证不得改为 public、匿名或另一 user 重试 | endpoint leaf tests、SDK/MCP request/output fixtures 已有；strict live rows 未登记，related/followers/blocked 的第二页不得从其他 list 推断 |
| user recommended | `GET /v1/user/recommended` | 首请求不发送 `offset`；合法续页即使为 `offset=0` 也必须显式发送 | required `user_previews`；user ID、nested artwork/novel ID 与 nested owner ID 必须为正数；nested list 可为空但不能以 malformed 响应伪装成功 | continuation 为 `offset`，区分 zero cursor 与 explicit `offset=0`；推荐 aggregate 的 user/artwork/novel result kind 不混为 subtype | endpoint fixture 已覆盖 nested mapping 与 `offset=0`；strict live case 未登记，不授予 recommended-all 发布权限 |
| trending artwork tags | `GET /v1/trending-tags/illust` | 无 query、无用户 ID、无 continuation | required `trend_tags` list；每项 `tag` 非空且必须有合法 sample artwork；sample artwork ID 为正数并按 artwork DTO 规则映射；空数组合法 | 无分页；缺失/null list、空 tag、缺失/null sample artwork 或 sample ID 非正数为 `MalformedUpstreamResponse` | endpoint/SDK/MCP fixture 已有；strict live row 未登记，不能把当前 fixture 当作 upstream 两页或发布 evidence |
| MyPixiv users | `GET /v1/user/mypixiv` | 只使用 verified current user ID，并固定 `filter=for_android`；首请求不带 `offset` | required `user_previews` list → `UserPreview{User}`；user ID 必须为正数；空数组合法 | continuation 为正 `offset`；不得接受调用方注入其他 user ID，不得匿名 fallback；身份未知返回 `Unauthorized` | endpoint/SDK/MCP fixture 已有；strict live row 未登记；账号 binding 和 pool replay 由 T12/T19/T23 owner 回归 |
| MyPixiv artworks / novels | `GET /v2/illust/mypixiv`, `GET /v1/novel/mypixiv` | 由当前 authenticated client 执行；首请求不带 `offset`；不接受 user ID 或跨账号 scope | artwork path 的 required `illusts` → `Artwork`；novel path 的 required `novels` → `Novel`；ID 与 nested owner ID 必须为正数；空数组合法 | 两者 continuation 均为正 `offset`；各自 operation/cursor 不互用；无匿名、Web 或另一账号 fallback | artwork/novel timeline endpoint tests、SDK/MCP fixtures 已有；strict live 第二页未登记，不据 MyPixiv fixture 提升 verdict |

所有 T06 list 的 required envelope 缺失或 JSON `null` 都是 `MalformedUpstreamResponse`，合法空数组必须保持 non-nil empty `Items`。`next_url` 只在 adapter 内解析；空字符串、不可解析、缺失唯一 allowlist key、重复 key、非正数或跨 operation key 均为 malformed，不把 raw URL 写入 public cursor。`SearchUsers` 的 verified account binding 不再作为未关闭缺口：T12 已冻结该 operation 为 public-scoped，现有 cross-client compatibility fixture 证明其不绑定账号或 client instance。

### T06 follow mutation、错误与结果脱敏

| Operation | Method / path | Request / precondition | Outcome contract | Evidence boundary |
| --- | --- | --- | --- | --- |
| follow user | `POST /v1/user/follow/add` | `user_id` 正数；`restrict` 仅 `public|private`，空值由兼容层默认 `public`；写前须在同一账号 execution context 验证目标和权限 | 当前 `PostForm` 不解码 response body，2xx/nil 只表示请求被接受，不能单独宣告关系已改变；必须用同账号 user/relationship read-back 证明 `is_followed=true`。明确拒绝/确定失败、读回失败和 dispatch 后不确定结果分开，不自动重放 | endpoint path/form、SDK/MCP wire fixture 已有；当前生产层没有 follow mutation read-back strict evidence，仍由 T07/T35/T38 完成 |
| unfollow user | `POST /v1/user/follow/delete` | `user_id` 正数；写前保存原关系状态并使用同一账号 execution context | 2xx/nil 不是删除证明；同账号 read-back 必须证明 `is_followed=false`，并恢复测试前原状态。transport/cancel/deadline 或已接受但无法读回时保留不确定状态，不猜测、不搜索最近关系、不自动重试 | endpoint path/form、SDK/MCP wire fixture 已有；当前 production mutation/read-back 未测试，不升级 capability |

T06 统一采用以下 SDK error 分类，调用方不能把错误流、空列表或 mutation 的 2xx 伪装成成功：

- `InvalidArgument`：本地 ID/word/restrict/subtype/namespace 冲突或不支持的输入；不发送 upstream request。`InvalidCursor`：operation、query、account/client binding、版本或 continuation 不匹配。
- `Unauthorized`：当前用户身份不可用的 identity-scoped read；`CredentialsExpired`：401 或无效/过期凭据；`Forbidden`：403/access-control 拒绝；`NotFound`：404；`ContentUnavailable`：410；`RateLimited`：429，并且只使用验证过的 `Retry-After` 提供 retry advice。
- `MalformedUpstreamResponse`：required envelope/list/object 缺失、null、ID/continuation/schema 非法；`UpstreamError`：其他已分类的 upstream rejection/5xx；`UpstreamUnavailable`：DNS/TLS/proxy/connection/timeout 等传输失败。`context.Canceled` 与 `context.DeadlineExceeded` 保留为可识别的取消原因，不改写成成功或空结果。
- 未有独立证据时不新增 `ChallengeRequired` 或其他 Pixiv-specific reason；403、网络错误、429、malformed 和不确定 mutation 不能触发“换一种资源类型再试”的隐式 bare-ID probe。bare-ID probe 只有在 T21 冻结候选 namespace 后才允许，无法判定或多个候选成功必须显式 `InvalidArgument`。

错误与输出必须只交付受控分类：`sdk.Error` 的 detail/cause 不含 response body、raw URL、header、token、cookie、proxy userinfo、浏览器路径、配置内容、用户输入正文或上游 Web envelope message；protocol failure 只保留 status、transport、取消/deadline 和已验证 retry advice。CLI/MCP 不输出敏感诊断，MCP runtime error 保留 structured result 并设置 `isError=true`，stdout 继续只承载 JSON-RPC；cursor 只保存 binding/query digest/typed continuation，不保存 raw query、next_url、用户内容或凭据。

### T06 evidence index 与边界

- user/search/detail/relationship/trending/MyPixiv 的当前 adapter 与 SDK 对照分别见 `internal/services/pixiv/endpoint/user/{search,detail,related,followers,following,blocked,mypixiv,recommended}`、`internal/services/pixiv/endpoint/artwork/trending`、`internal/services/pixiv/endpoint/artwork/timeline`、`internal/services/pixiv/endpoint/novel/timeline` 及 `sdk/pixiv/{ops_user.go,ops_artwork.go,ops_novel.go,pixiv_test.go}`；MCP route/structured output 见 `internal/mcpserver/pixiv/pixiv_user_test.go`、`pixiv_sdk_wire_test.go`。
- error/redaction 的现有安全边界见 `sdk/error.go`、`sdk/pixiv/errors.go`、`internal/services/pixiv/protocol/failure.go` 及其测试；双语 SDK 对外错误契约见 `docs/en/sdk.md`、`docs/zh-CN/sdk.md`。这些是当前实现与 fixture evidence，不替代 T12/T35/T37/T38 的兼容和发布回归。
- strict live evidence 只认 `goal-3/evidence/appapi-upstream.md`/`.json` 的已登记 rows；T06 新增的 user search/detail/relationships/recommended/trending/MyPixiv/follow mutation 没有被历史 live 记录覆盖，user artworks/novels 的已有 rows 继续保持 `inconclusive`/`pagination_exempt`。本节不改写任何历史 verdict，不修改 capability admission；required capabilities 继续为 `scope_admitted`。

## 迁移准入规则（2026-09-07 更正）

上表保留历史观测与原 verdict；包括备注中的 migration-ready 也仅是当时的证据标签，不是当前实施状态。当前实施与发布授权只来自 [能力准入表](capability-admission.md)，confirmed 不放行 CLI/MCP/docs。contract、T12/T39A、adapter/SDK、回归与文档按 tasks 依赖推进。

## 原始计划中尚未进入 strict manifest 的候选

| Case | Method | Path | 参数/语义 | Wire | Response | Pagination | Adapter | SDK | 状态 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| search-novel-period | GET | /v1/search/novel | start_date,end_date | not_tested | not_tested | not_tested | not_tested | not_tested | 必须补测 |
| novel-bookmark-tags | GET | /v1/user/bookmark-tags/novel | restrict,user_id | not_tested | not_tested | pagination_exempt | not_tested | not_tested | 必须补测 |
| novel-bookmark-detail | GET | /v2/novel/bookmark/detail | novel_id | not_tested | not_tested | none | not_tested | not_tested | 必须补测 |
| novel-bookmark-add | POST | /v2/novel/bookmark/add | novel_id,restrict,tags | not_tested | not_tested | read_back | not_tested | not_tested | 必须补测 |
| novel-bookmark-delete | POST | /v1/novel/bookmark/delete | novel_id | not_tested | not_tested | read_back | not_tested | not_tested | 必须补测 |
| illust-bookmark-tags | GET | /v1/user/bookmark-tags/illust | restrict,user_id | not_tested | not_tested | pagination_exempt | not_tested | not_tested | 必须补测 |
| illust-bookmark-subtype | GET | /v1/user/bookmarks/illust | type/content_type | not_tested | not_tested | logical | not_tested | not_tested | 必须补测 |
| illust-recommended-subtype | GET | /v1/illust/recommended | content_type | not_tested | not_tested | not_tested | not_tested | not_tested | 必须补测 |
| illust-new-subtype-expansion | GET | /v1/illust/new | manga/ugoira/compound | partial | partial | partial | partial | partial | 必须补测 |
| novel-comment-total | GET | /v2或v3/novel/comments | total_comments/include_total | partial | inconclusive | inconclusive | partial | partial | 必须补测 |
| illust-comment-total | GET | /v3/illust/comments | total_comments/include_total | partial | inconclusive | inconclusive | partial | partial | 必须补测 |


## Goal-3 状态语义

本矩阵的历史 verdict 用于描述 evidence、fixture 与当前生产覆盖，不等同于 capability 不存在。Goal-3 内按 `contract_frozen`、`migration_ready`、`public_ready` 逐层推进；`inconclusive` / `not_tested` 需要补 snapshot 或实现证据，但不产生新的 Goal。
