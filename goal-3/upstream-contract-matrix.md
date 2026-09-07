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
