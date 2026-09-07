# Goal-3 MCP wire compatibility matrix（T39A）

修订日期：2026-09-07（Asia/Shanghai）。当前 capability 状态和发布授权只见
[能力准入表](capability-admission.md)；本文件只冻结 MCP 的公开 wire contract，
不把未完成的 endpoint/SDK 实现写成已发布能力。

## 权威来源与边界

本表以当前注册表、handler 类型和共享输出 helper 为准：

- 注册集合：`internal/mcpserver/pixiv/pixiv.go` 与
  `internal/mcpserver/pixiv/pixiv_search_server_test.go` 的 exact-set 回归。
- 输入 schema：各 `internal/mcpserver/pixiv/tools/<tool>/` 的 input struct；
  `search_illust`、`search_novel`、`reverse_search` 的显式 closed schema；
  其余 struct schema 同样拒绝未知字段。
- 输出 schema/error：`internal/mcpserver/pixiv/internal/runtime`、`outputs`、
  `records`，以及现有 `pixiv_*_test.go` 的 structured-content 断言。
- locale 公开说明：[English MCP reference](../docs/en/mcp-tools.md)、
  [简体中文 MCP reference](../docs/zh-CN/mcp-tools.md)。

T39A 冻结的是现有 tool 名、JSON 字段、默认值、structured output 和
`isError` 语义。后续 T24–T38 可以新增明确命名的 operation，但不能为了统一
CLI `TARGET` 而改写旧 MCP 字段；特别是 `add_bookmark.illust_id` 必须继续存在。
MCP 不暴露 SDK opaque cursor、`next_url`、Cookie、token、签名 URL 或请求头。

## 共享输入语义

以下规则适用于表中带有对应字段的 tool：

| 字段 | Frozen contract |
| --- | --- |
| `page` | 可选、1-based logical page；传入时必须为正数，并且必须同时传入正数 `limit`。默认不传。 |
| `limit` | 可选；省略表示只读一个 upstream batch，正数填充一页，`0` 读取到 upstream cursor 结束；负数拒绝。 |
| `illust_filter` | 可选 object：`id`（正数）、`type`（`illust`/`manga`/`ugoira`）、`tags`、`min_views`、`min_pages`；所有数值边界在 handler 前校验。 |
| `novel_filter` | 可选 object：`id`（正数）、`tags`、`min_views`。 |
| `user_filter` | 可选 object：`id`（正数）。 |
| `restrict` | 可选 `public`/`private`；空值沿既有 operation 的兼容默认（follow/timeline 为 `public`），未知值不得被静默改写。 |
| `page`/`limit` 输出 | `pagination` 至少包含 `page`、`limit`、`returned`、`has_more`、`next_page`；SDK cursor 只留在 server 内。 |

Schema 级无效输入在打开 SDK operation 前被拒绝；handler 运行后的失败保留该
tool 的 structured output、设置 `isError=true`，并将集合置为空或报告置为对应
的空失败形状。正常空页仍是 `isError=false`。这条规则不允许把失败伪装成空成功。

## 输出与错误 envelope

表格使用以下短名，短名只减少重复，不改变 wire：

| 名称 | Structured output |
| --- | --- |
| `Records` | `{records: Record[], pagination: {page, limit, returned, has_more, next_page}, filter?}`；每条 record 至少有 `id`、`type`、`url`。 |
| `UserDetail` | `{records: Record[]}`。 |
| `NovelDetail` | `{records: Record[]}`。 |
| `Comments` | `{comments: CommentDTO[], pagination, total?, access_control?}`。 |
| `NovelSeries` | `{series: NovelSeriesDTO, records: Record[], pagination}`。 |
| `BookmarkTags` | `{bookmark_tags: BookmarkTagDTO[], pagination}`。 |
| `BookmarkDetail` | `{bookmarked: boolean, restrict?, tags: string[]}`。 |
| `Mutation` | `{success: boolean, action: string, illust_id? / user_id?, text: string}`；失败仍返回 envelope 且 `isError=true`。 |
| `Trending` | `{tags: TrendingTagDTO[], text: string}`。 |
| `Recommended` | `{records: Record[], pagination: {illust?, manga?, novel?, user?}}`；`kind=all` 的流分别保留分页。 |
| `NovelContent` | `{content: NovelContentDTO}`；兼容入口失败时 `content.blocks` 为空。 |
| `Reverse` | `{input, providers, results, records, provider_errors, partial}`；部分 provider 成功时 `partial=true` 且 `isError=false`。 |
| `Download` | `{delivery, items, failures, files, text}`；失败项保留，handler 失败时仍返回该报告形状并 `isError=true`。 |
| `Error` | 对应成功 envelope 的空值/安全默认值 + 稳定错误文本；所有 handler error 均 `isError=true`。 |

错误正文沿 SDK typed error 的稳定 code 脱敏。旧调用方只能依赖
`isError`、structured 空形状和稳定 code/安全 message，不能依赖上游原始 JSON。
`novel_content` 是特例：它保留 wire/schema 和 structured shape，但正数 ID
固定返回既有 `content_unavailable`，不请求已排除的 `/v1/novel/content`。

## 逐 tool compatibility map 与旧 JSON replay fixture

`Fixture` 是旧 JSON-RPC `tools/call.params.arguments` 的原始 JSON object；回放
必须使用表中原 tool name，不得先转换成 CLI 命令。`Assertion` 是每个 fixture
至少要锁定的断言：schema 接受/拒绝、默认值、请求映射、structured shape 和
`isError`。正数 ID、关键词和路径均为合成值，不代表 live API 目标。

| Tool name（wire 保留） | 新 operation / port | Input fields、必填与默认 | Fixture | Output | Error / replay assertion |
| --- | --- | --- | --- | --- | --- |
| `download` | download pipeline | `src` 与非空 `srcs` 二选一；`pages?` 默认全部；`quality?` 使用 application default；`ugoira_mode?=gif`；`delivery?=local_path` | `{"src":"101"}` | `Download` | 输入互斥/选项错误为 `isError=true`，报告 shape 不变；不发布未接入的 retry/concurrency 字段。 |
| `download_random_from_recommendation` | recommendation + download pipeline | `count?=5`，显式值 `1..20`；`pages?`、`quality?`、`ugoira_mode?=gif`、`delivery?=local_path` | `{}` | `Download` | 默认 count 必须为 5；越界在 handler 前失败，不静默截断。 |
| `search_illust` | `SearchArtworks(SearchArtworksRequest)` | `word` 必填；`search_target?=partial_match_for_tags`、`sort?=date_desc`；`duration/start_date/end_date/content_type/ai_mode/aspect_ratio/resolution/tool/bookmark_min/bookmark_max/bookmark_strategy/illust_filter/page/limit` 可选 | `{"word":"cat"}` | `Records` | closed schema 拒绝未知字段；日期/enum/filter/range 错误不打开 SDK；bookmark strategy 失败不变成空成功。 |
| `search_novel` | `SearchNovels(SearchNovelsRequest)` | `word` 必填；`search_target?=partial_match_for_tags`、`sort?=date_desc`；`duration/novel_filter/page/limit` 可选；不接受 rating/text-length/original-only | `{"word":"cat"}` | `Records` | closed schema 与 unsupported-field negative replay；query/cursor 错误为 `Records` 空结果 + `isError=true`。 |
| `illust_detail` | `Artwork(ArtworkRequest)` | `illust_id` 与 `url` 必须二选一；ID 为正数；URL 只接受本地解析支持的 artwork URL | `{"illust_id":101}` | `UserDetail` | 双填/双空/非正 ID 先拒绝；成功只有一条 artwork record。 |
| `illust_related` | `RelatedArtworks(RelatedArtworksRequest)` | `illust_id` 必填正数；`illust_filter/page/limit` 可选 | `{"illust_id":101}` | `Records` | ID/filter/list plan 映射保持；SDK/上游失败保留 error。 |
| `illust_ranking` | `ArtworkRanking(ArtworkRankingRequest)` | `mode?=day`、`date?`、`illust_filter/page/limit` 可选 | `{}` | `Records` | omitted mode 必须为 `day`；非法 mode/date/filter 不能静默改为 day。 |
| `search_user` | `SearchUsers(SearchUsersRequest)` | `word` 必填；`user_filter/page/limit` 可选 | `{"word":"miku"}` | `Records` | user filter 与分页映射保持；错误为结构化 error。 |
| `illust_recommended` | `RecommendedArtworks(RecommendedArtworksRequest)` | `illust_filter/page/limit` 可选 | `{}` | `Records` | filter 在逻辑分页前应用；不新增旧 schema 字段。 |
| `novel_detail` | `Novel(NovelRequest)` | `novel_id` 必填正数 | `{"novel_id":201}` | `NovelDetail` | metadata 读取错误为 `NovelDetail` 空 records + `isError=true`；不 fallback 到 rejected v1 path。 |
| `novel_content` | 保留 `NovelContent(NovelContentRequest)` symbol 的兼容入口 | `novel_id` 必填正数 | `{"novel_id":201}` | `NovelContent` | 正数 ID 固定 `content_unavailable`、`isError=true`、空 blocks、零 rejected-endpoint 请求；0/负数为 input error。 |
| `illust_series` | `ArtworkSeries(ArtworkSeriesRequest)` | `series_id` 必填正数；`page/limit` 可选 | `{"series_id":301}` | `Records` | series ID/list plan 映射保持；cursor 不出 MCP。 |
| `novel_series` | `NovelSeries(NovelSeriesRequest)` | `series_id` 必填正数；`page/limit` 可选 | `{"series_id":302}` | `NovelSeries` | series metadata 与 records 同时保留；上游失败不返回部分成功。 |
| `illust_comments` | `ArtworkComments(ArtworkCommentsRequest)` | `id` 必填正数；`page/limit` 可选 | `{"id":101}` | `Comments` | `comments/pagination` 必有；`total/access_control` 只在上游提供时出现。 |
| `novel_comments` | `NovelComments(NovelCommentsRequest)` | `id` 必填正数；`page/limit` 可选 | `{"id":201}` | `Comments` | 与 artwork comments 同 envelope；不把 content endpoint 当作 comments fallback。 |
| `recommended` | kind-specific `Recommended*` requests | `kind` 必填：`all/illust/manga/novel/user`；对应 typed filter 与 `page/limit` 可选 | `{"kind":"all"}` | `Recommended` | all 的四条流分别记录 pagination；不接受与 kind 冲突的 filter；任一必需流失败则整体 `isError=true`。 |
| `trending_tags_illust` | `TrendingArtworkTags(TrendingArtworkTagsRequest)` | 无字段 | `{}` | `Trending` | 无输入；空列表仍可成功，SDK/上游错误为 `isError=true`。 |
| `timeline_illust_following` | `FollowingArtworks(FollowingArtworksRequest)` | `restrict?=public`；`illust_filter/page/limit` 可选 | `{}` | `Records` | 默认 public；未知 restrict 在 SDK 边界拒绝，不换匿名/Web path。 |
| `timeline_novel_following` | `FollowingNovels(FollowingNovelsRequest)` | `restrict?=public`；`novel_filter/page/limit` 可选 | `{}` | `Records` | 同上；App/OAuth 失败保持 typed error。 |
| `timeline_illust_latest` | `LatestArtworks(LatestArtworksRequest)` | `content_type` 必填：`illust` 或 `manga`；`illust_filter/page/limit` 可选 | `{"content_type":"illust"}` | `Records` | 其他 subtype 先拒绝；不把 search 的 `all` 带入 latest。 |
| `timeline_novel_latest` | `LatestNovels(LatestNovelsRequest)` | `novel_filter/page/limit` 可选 | `{}` | `Records` | cursor 由 SDK 保留；后续 max-novel-id 修复不得改 tool wire。 |
| `mypixiv_users` | `MyPixivUsers(MyPixivUsersRequest)` | `user_filter/page/limit` 可选 | `{}` | `Records` | 需要认证；身份错误不伪造空成功。 |
| `mypixiv_illusts` | `MyPixivArtworks(MyPixivArtworksRequest)` | `illust_filter/page/limit` 可选 | `{}` | `Records` | identity-scoped operation；output 与普通 records 相同。 |
| `mypixiv_novels` | `MyPixivNovels(MyPixivNovelsRequest)` | `novel_filter/page/limit` 可选 | `{}` | `Records` | identity-scoped operation；失败保留 `isError=true`。 |
| `user_detail` | `User(UserRequest)` | `user_id` 必填正数 | `{"user_id":401}` | `UserDetail` | 非正 ID 先拒绝；只返回 safe public record。 |
| `user_artworks` | `UserArtworks(UserArtworksRequest)` | `user_id?` 默认认证用户；`type?` 为 `illust/manga/ugoira`；`illust_filter/page/limit` 可选 | `{}` | `Records` | 省略 ID 的 resolver 语义保留；不接受已删除的 `user_id_to_check`/`offset`。 |
| `user_novels` | `UserNovels(UserNovelsRequest)` | `user_id?` 默认认证用户；`novel_filter/page/limit` 可选 | `{}` | `Records` | current-user default 与 novel filter 保持；错误不 fallback。 |
| `user_bookmarks` | `UserArtworkBookmarks(UserArtworkBookmarksRequest)` | `user_id?` 默认认证用户；`restrict?`、`tag?`、`illust_filter/page/limit` 可选 | `{"user_id":401,"restrict":"private","tag":"tag-a"}` | `Records` | exact fields `user_id/restrict/tag` 继续存在；旧 `user_id_to_check/max_bookmark_id/offset` 作为 negative replay 拒绝。 |
| `user_novel_bookmarks` | `UserNovelBookmarks(UserNovelBookmarksRequest)` | `user_id?` 默认认证用户；`restrict?`、`tag?`、`page/limit` 可选 | `{}` | `Records` | 保留 novel bookmark read wire；错误不切 artwork path。 |
| `user_following` | `UserFollowing(UserFollowingRequest)` | `user_id?` 默认认证用户；`restrict?`、`user_filter/page/limit` 可选 | `{"user_id":401,"restrict":"private"}` | `Records` | identity/user filter 映射保持；unknown legacy fields reject。 |
| `user_followers` | `UserFollowers(UserFollowersRequest)` | `user_id?` 默认认证用户；`restrict?`、`page/limit` 可选 | `{}` | `Records` | relation scope 与 error 语义保持；不返回 transport credential。 |
| `related_users` | `RelatedUsers(RelatedUsersRequest)` | `user_id?` 默认认证用户；`restrict?`（当前 relation request 不使用）、`page/limit` 可选 | `{"user_id":401}` | `Records` | 不把 relation target 变成通用 TARGET；上游错误可见。 |
| `blocked_users` | `UserBlockedUsers(UserBlockedUsersRequest)` | `user_id?` 默认认证用户；`restrict?` 兼容字段、`page/limit` 可选 | `{}` | `Records` | App API failure final；禁止 Web fallback 或空成功。 |
| `bookmark_tags` | `UserArtworkBookmarkTags(UserArtworkBookmarkTagsRequest)` | `user_id?` 默认认证用户；`restrict?`、`page/limit` 可选 | `{"user_id":401}` | `BookmarkTags` | output key 必须是 `bookmark_tags`；不改成 generic `tags`，all 聚合由后续 owner additive 实现。 |
| `bookmark_detail` | `ArtworkBookmark(ArtworkBookmarkRequest)` | `illust_id` 必填正数 | `{"illust_id":101}` | `BookmarkDetail` | 未收藏仍 `bookmarked=false`；请求/上游错误为 `isError=true`。 |
| `add_bookmark` | `AddBookmark(AddBookmarkRequest)` | `illust_id` 必填正数；`restrict?`、重复 `tags?` 可选，空 restrict 默认 public | `{"illust_id":101,"restrict":"private","tags":["tag-a"]}` | `Mutation` | `illust_id` 不得改名为 `target`；成功 `success=true`，失败 `success=false` + `isError=true`；不自动重放未知结果。 |
| `remove_bookmark` | `RemoveBookmark(RemoveBookmarkRequest)` | `illust_id` 必填正数 | `{"illust_id":101}` | `Mutation` | 保留 `action=remove_bookmark` 与 `illust_id`；失败不猜测状态。 |
| `follow_user` | `FollowUser(FollowUserRequest)` | `user_id` 必填正数；`restrict?`，空值默认 public | `{"user_id":401}` | `Mutation` | unknown restrict 在 SDK 边界 `InvalidArgument` 且零网络；不自动换账号重放。 |
| `unfollow_user` | `UnfollowUser(UnfollowUserRequest)` | `user_id` 必填正数 | `{"user_id":401}` | `Mutation` | 保留 user_id/action；提交后未知状态不重放。 |
| `reverse_search` | reverse-search facade | `source` 必填 regular local file/HTTP(S) URL；`provider?` 默认启动配置（通常 `saucenao`） | `{"source":"/fixtures/pixiv.png"}` | `Reverse` | closed schema；provider partial semantics、稳定 error code 和 source 脱敏必须保持。 |

## 回放清单与当前证据

上表的 40 个 fixture 是 T39A 冻结的完整旧 JSON 清单。实现 owner 必须将每行接入
离线 fake SDK/HTTP fixture；不以 CLI alias 测试替代 MCP replay。当前已存在并在本轮
复跑的证据如下：

| Evidence | 覆盖的冻结项 |
| --- | --- |
| `TestSDKMutationToolsReturnStructuredSuccess` | `add_bookmark`、`remove_bookmark`、`follow_user`、`unfollow_user` 的旧字段、成功 `Mutation` shape。 |
| `TestSDKMutationTypedErrorIsMCPError` | mutation typed error → `success=false`、`isError=true`、安全 code。 |
| `TestMCPStdioKeepsJSONRPCOnStdout` | 旧 JSON-RPC `search_illust`、`add_bookmark`、`reverse_search` 请求以及 stdout/`isError` 边界。 |
| `TestSDKUserListToolsSchemaRejectsRemovedLegacyFields` | user list schema 对 `user_id_to_check`、`max_bookmark_id`、`offset` 等已删除字段的拒绝。 |
| `TestNovelContentReportsUnsupportedWithoutCallingRejectedEndpoint` | `novel_content` 旧 JSON 名/`novel_id` 保留，structured `content_unavailable` 与零 rejected-endpoint 请求。 |
| `TestSDKUserListToolsUseCanonicalUserIDAndFilters` | `user_bookmarks`/`user_following` 的 `user_id`、`restrict`、`tag`、分页字段映射。 |
| `TestServerListsExpectedTools`（exact set） | 40 个 tool 名称不增删；schema 检查覆盖 search 字段和 unsupported novel search 字段。 |

本轮不执行真实账号 API，也不宣称尚未有 fake fixture 的 endpoint 已
`public_ready`。T39A 的完成含义是 wire 冻结和 replay 清单就绪；T37/T38/T39B/T43
仍必须逐项补齐离线回放、实现后审计和全量 CLI/MCP 回归。

## 回滚与兼容承诺

本文件、CLI route map 与双语 locale 修正均为 contract/documentation 变更；回滚时
必须一起撤销 T39A 状态、matrix 链接和 `novel_content` 的过时成功承诺修正。没有
数据库、账号、token、下载内容、endpoint 请求或依赖变化。后续实现若已引用某一
行的新 request，回滚应先撤销引用或提供同名旧 wire wrapper，不能只删除 map。
