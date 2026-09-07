# API 迁移 contract 与兼容验证台账

原观测日期：2026-09-05；修订日期：2026-09-07（Asia/Shanghai）。

## 当前门禁来源

本文件是 contract/compatibility 工作清单；当前状态及发布授权只见 [能力准入表](capability-admission.md)。历史 upstream 证据不意味着 migration_ready；该状态必须同时满足 contract freeze 与 T12 兼容决策。T39A 冻结后才能实施 CLI/MCP 迁移。未发布实现允许同步编写导出 SDK、注册、测试和文档，发布仍须 public_ready。

所有 required_scope 均须完成；阻塞能力停止发布但保持 Goal incomplete，不能自行移除范围。

## 已确认的迁移依据

| 能力 | 旧 contract | 目标 contract | Upstream evidence | 当前执行前提 | 结论 |
| --- | --- | --- | --- | --- | --- |
| Novel latest cursor | `offset` | `max_novel_id` | wire/response/两页已确认 | contract + T12 pending | 冻结后 TDD 修复；修复后再过 adapter/SDK gate |
| Novel detail | `/v1/novel/detail` | `/v2/novel/detail` | v1 rejected；v2 wire/response 已确认；无分页 | contract + T12 pending | 冻结后 TDD 迁移；必须保留 v2 series 字段 |
| Comment add/reply/delete wire | 无生产 operation | `/v1/*/comment/add\|delete` | 真实写入、读回和删除已有 evidence | contract + T12 pending | 冻结后写 adapter/SDK red tests；未通过前不公开 |
| Stamps read | 无生产 operation | `/v1/stamps` | wire/response 已确认；无分页 | contract + T12 pending | 冻结后写 adapter/SDK red tests |
| Novel ranking | 无生产 operation | `/v1/novel/ranking` | wire/response/两页已确认 | contract + T12 pending | 冻结后写 adapter/SDK red tests |

## 尚未达到迁移门禁

| 能力 | 候选 contract | 缺失证据 | 当前状态 | 实施规则 |
| --- | --- | --- | --- | --- |
| Novel series | `/v2/novel/series` + `last_order` | 真实第二页；candidate adapter；SDK | inconclusive | 不切生产 path |
| Novel comments v3 | `/v3/novel/comments` | 非空响应；第二页；adapter；SDK | inconclusive | 不从 v2 迁移 |
| Artwork comments v3 DTO | `/v3/illust/comments` | 非空 live DTO；`date`；numeric access control；第二页 | inconclusive / fixture mismatch | 不冻结 DTO |
| Artwork recommended continuation | `/v1/illust/recommended` | 完整 continuation 参数；成功第二页 | inconclusive | 不替换 cursor model |
| Novel search period | `/v1/search/novel` + `start_date/end_date` | 未进入 strict manifest；live 两页；adapter；SDK | not tested | 不删除 `Duration` |
| Novel bookmark tags | `/v1/user/bookmark-tags/novel` | wire/response；空列表；adapter；SDK | not tested | contract snapshot 前不新增 public operation |
| Novel bookmark detail | `/v2/novel/bookmark/detail` | wire/response；public/private 状态；adapter；SDK | not tested | contract snapshot 前不新增 public operation |
| Novel bookmark add/delete | `/v2/novel/bookmark/add` + `/v1/novel/bookmark/delete` | 隔离写入、detail、list、tags、恢复原状态；adapter；SDK | not tested | contract snapshot 前不新增 public mutation |
| Artwork bookmark tags | `/v1/user/bookmark-tags/illust` | wire/response；subtype 语义；adapter；SDK | not tested | 不扩展 subtype tags |
| Artwork bookmark subtype | `/v1/user/bookmarks/illust` + `type/content_type` | 参数是否被忽略；logical pagination | not tested | 默认只能 client-side 候选 |
| Recommended subtype | `/v1/illust/recommended` + `content_type` | illust/manga/ugoira 各两页 | not tested | 不公开 ugoira subtype |
| Latest subtype expansion | `/v1/illust/new` + compound `content_type` | manga/ugoira/组合值各两页 | partial | 只保留已确认的类型 |
| Novel comment total | v2/v3 comments | 非空 `total_comments`；是否需要 include 参数 | inconclusive | total 保持非强保证 |
| Artwork comment total | v3 comments | 非空 `total_comments`；是否需要 include 参数 | inconclusive | total 保持非强保证 |
| Bare ID probe | 多资源 detail endpoint | namespace 冲突；403/404 分类；请求成本 | not tested | 保持显式 `--type` |

## 已拒绝或排除

| 能力 | Contract | 结论 | 处理 |
| --- | --- | --- | --- |
| Novel detail v1 | `/v1/novel/detail` | rejected | 删除依赖，不做 fallback |
| Novel series v1 | `/v1/novel/series` | rejected | 删除依赖，不做 fallback |
| Novel content App API | `/v1/novel/content` | rejected | 不再承诺正文读取 |
| Novel WebView content | `/webview/v2/novel` | excluded | 不进入本 goal 默认实现 |
| Server-side rating | `x_restrict` request filter | rejected | 只允许 client-side filter |


## T12：SDK symbol map 必交付列

T12 的兼容基线是 `sdk`、`sdk/pixiv`、`sdk/fanbox` 当前导出清单及既有 method/request/model/named field type。`go test ./scripts/internal/publicapi` 继续锁定导出清单；本任务不新增 breaking symbol，不删除旧符号，不改 module/version 策略。`TestLegacySDKConsumerCompiles` 以 external test package 直接实现旧 `Client` interface 和 request/model literals，作为旧消费者编译门禁。

### 冻结决定

- `CheckpointSearchArtworks` 与 `SearchArtworksRequest.CursorContext` 是 T23A 已批准的增量入口；旧 `SearchArtworks` method、旧 request 字段、旧 model 保持不变。
- 只有 `SearchArtworks` 的 operation binding version 升为 2；version-1 搜索 cursor 返回 `InvalidCursor`，不能静默从第一页重启。其他 operation 和外层 `sdk.Cursor` 编码保持原版本与格式。
- `SearchArtworks` 属于 identity-scoped operation：`Open/OpenWith` 绑定 verified account，`New/NewWith` 在身份未知时只允许同一 client instance。`SearchNovels` 与 `SearchUsers` 冻结为 public-scoped operation，只绑定 product、operation、version 与 query；相同 query 的 cursor 可以跨 client 恢复，不引入未经批准的 account binding 或 version bump。
- `AddBookmark`、`RemoveBookmark`、`FollowUser`、`UnfollowUser` 与 `SetAIArtworkVisibility` 保留原 method/request 签名和 named types。未来新增显式 artwork/novel mutation 不得借机删除旧 wrapper；wrapper 必须委托同一已冻结的内部语义。
- `NovelContent` 保留 exported method、`NovelContentRequest`、`NovelContent` model 与 DTO，标记为 deprecated；由于 `/v1/novel/content` 已 rejected、WebView 正文已 excluded，入口只返回既有 `sdk.ContentUnavailable`，不发网络请求，不做 fallback。该处理不把旧符号误报为仍可用的 endpoint。

### 旧 public method → request/model → 迁移与错误 map

| 旧 public method | 旧 request / result model | 替代 method 或实施决策 | wrapper / deprecation / 预期错误 | 兼容证据 |
| --- | --- | --- | --- | --- |
| `SearchArtworks` | `SearchArtworksRequest` → `sdk.Page[Artwork]` | 保留原 method；checkpoint 为 additive `CheckpointSearchArtworks` | 不删除、不改参数；query/binding 不匹配返回 `InvalidCursor` | `TestSearchArtworksCheckpointRoundTrip`、`TestSearchArtworksCheckpointRejectsChangedBindings` |
| `Artwork` | `ArtworkRequest` → `Artwork` | 保留；未来 adapter 替换不改变 method | 正数 ID；保留 `InvalidArgument` 与既有 upstream 分类 | `TestLegacySDKConsumerCompiles` |
| `ArtworkPages` | `ArtworkPagesRequest` → `[]ArtworkPage` | 保留；显式页模型继续使用 `ArtworkPage` | 不改 slice/result 形状；malformed resource 真实报错 | `TestLegacySDKConsumerCompiles` |
| `RelatedArtworks` | `RelatedArtworksRequest` → `sdk.Page[Artwork]` | 保留 endpoint-oriented method | cursor/query 失配返回 `InvalidCursor` | `TestLegacySDKConsumerCompiles` |
| `ArtworkSeries` | `ArtworkSeriesRequest` → `sdk.Page[Artwork]` | 保留；series continuation 由后续 adapter/SDK owner 迁移 | 不删除 `SeriesID`/`Cursor`；保留 `InvalidCursor` | `TestLegacySDKConsumerCompiles` |
| `ArtworkRanking` | `ArtworkRankingRequest` → `sdk.Page[Artwork]` | 保留；`Mode`/`Date` 仍是 request 字段 | 非法 mode/date 返回 `InvalidArgument` | `TestLegacySDKConsumerCompiles` |
| `RecommendedArtworks` | `RecommendedArtworksRequest` → `sdk.Page[Artwork]` | 保留；推荐 subtype 由后续 additive surface 处理 | 不把旧 request 改为通用 target；cursor 错误显式返回 | `TestLegacySDKConsumerCompiles` |
| `FollowingArtworks` | `FollowingArtworksRequest` → `sdk.Page[Artwork]` | 保留；`Restrict`/`Cursor` 不改名 | 空 `Restrict` 的既有默认和非法值错误保持 | `TestLegacySDKConsumerCompiles` |
| `LatestArtworks` | `LatestArtworksRequest` → `sdk.Page[Artwork]` | 保留；continuation 修复不得改 method | subtype/cursor 不匹配返回既有分类，不 fallback | `TestLegacySDKConsumerCompiles` |
| `UserArtworks` | `UserArtworksRequest` → `sdk.Page[Artwork]` | 保留；user ID 与 artwork kind 仍分开 | 不删除 `Kind`；非法 ID/续页显式失败 | `TestLegacySDKConsumerCompiles` |
| `UserArtworkBookmarks` | `UserArtworkBookmarksRequest` → `sdk.Page[Artwork]` | 保留；bookmark 聚合由上层组织 | `UserID`、`Restrict`、`Tag`、`Cursor` 保持；未知 restrict `InvalidArgument` | `TestLegacySDKConsumerCompiles` |
| `UserArtworkBookmarkTags` | `UserArtworkBookmarkTagsRequest` → `sdk.Page[BookmarkTag]` | 保留；typed tags 为 additive 目标，不替换旧单流 model | named `BookmarkTag{Name, Count}` 保持 | `TestLegacySDKConsumerCompiles` |
| `MyPixivArtworks` | `MyPixivArtworksRequest` → `sdk.Page[Artwork]` | 保留；继续要求 identity-scoped 语义 | identity/cursor 失配返回 `InvalidCursor` 或 `Unauthorized` | `TestLegacySDKConsumerCompiles` |
| `TrendingArtworkTags` | `TrendingArtworkTagsRequest` → `[]TrendingTag` | 保留；不把无分页结果伪装成 page | malformed upstream 真实返回 `MalformedUpstreamResponse` | `TestLegacySDKConsumerCompiles` |
| `UgoiraMetadata` | `UgoiraMetadataRequest` → `UgoiraMetadata` | 保留；资源/archive/frame model 不改名 | 正数 ID 与 malformed archive 规则保持 | `TestLegacySDKConsumerCompiles` |
| `ArtworkComments` | `ArtworkCommentsRequest` → `CommentPage` | 保留；explicit comment surface 只能 additive | `CommentPage.Total`/`AccessControl` optional 语义保持 | `TestLegacySDKConsumerCompiles` |
| `ArtworkBookmark` | `ArtworkBookmarkRequest` → `ArtworkBookmarkDetail` | 保留；未来 explicit bookmark detail 不删除旧 method | 未收藏仍由空 `Restrict`/tags 表示 | `TestLegacySDKConsumerCompiles` |
| `SearchNovels` | `SearchNovelsRequest` → `sdk.Page[Novel]` | 保留原 method；冻结为 public-scoped cursor | 不绑定账号或 client instance；query 失配返回 `InvalidCursor` | `TestSearchNovelsAndUsersCursorsArePublicScoped` |
| `Novel` | `NovelRequest` → `Novel` | 保留；后续 T14 将同一 method 迁移到 v2 detail | 不删除 ID request；非法 ID/既有 upstream 错误保持 | `TestLegacySDKConsumerCompiles` |
| `NovelSeries` | `NovelSeriesRequest` → `NovelSeriesResult` | 保留；后续 T14 迁移 v2 series 与 metadata | 不 fallback 到 rejected v1；cursor 失配返回 `InvalidCursor` | `TestLegacySDKConsumerCompiles` |
| `NovelContent` | `NovelContentRequest` → `NovelContent` | 保留符号但无可用 replacement body endpoint | `Deprecated`；正数 ID 返回 `ContentUnavailable`，零/负 ID 仍 `InvalidArgument`，全程无网络 | `TestNovelContentDeprecatedEntryPointDoesNotCallRejectedEndpoint` |
| `NovelComments` | `NovelCommentsRequest` → `CommentPage` | 保留；v2/v3 选择由后续 comment owner 冻结 | 不删除旧 read model；continuation/schema 错误显式暴露 | `TestLegacySDKConsumerCompiles` |
| `RecommendedNovels` | `RecommendedNovelsRequest` → `sdk.Page[Novel]` | 保留；推荐 subtype 由后续 additive surface 处理 | 保留 zero-cursor/continuation distinction | `TestLegacySDKConsumerCompiles` |
| `FollowingNovels` | `FollowingNovelsRequest` → `sdk.Page[Novel]` | 保留；`Restrict`/cursor binding 继续生效 | 空值默认与非法值分类不改 | `TestLegacySDKConsumerCompiles` |
| `LatestNovels` | `LatestNovelsRequest` → `sdk.Page[Novel]` | 保留；后续修复 `max_novel_id` continuation | 不把旧 `offset` 修复伪装成 symbol 删除 | `TestLegacySDKConsumerCompiles` |
| `UserNovels` | `UserNovelsRequest` → `sdk.Page[Novel]` | 保留；pagination-exempt 不改变 source API | `UserID`/cursor 保持；失败不 fallback | `TestLegacySDKConsumerCompiles` |
| `UserNovelBookmarks` | `UserNovelBookmarksRequest` → `sdk.Page[Novel]` | 保留；novel bookmark expansion 仅 additive | 不擅自新增 bookmark mutation symbol | `TestLegacySDKConsumerCompiles` |
| `MyPixivNovels` | `MyPixivNovelsRequest` → `sdk.Page[Novel]` | 保留；继续 identity-scoped | 身份未知 `Unauthorized`，cursor 绑定错误 `InvalidCursor` | `TestLegacySDKConsumerCompiles` |
| `SearchUsers` | `SearchUsersRequest` → `sdk.Page[UserPreview]` | 保留原 method；冻结为 public-scoped cursor | 不绑定账号或 client instance；query 失配返回 `InvalidCursor` | `TestSearchNovelsAndUsersCursorsArePublicScoped` |
| `User` | `UserRequest` → `UserDetail` | 保留；后续 v2 detail 迁移不改 symbol | 正数 ID；保留 `InvalidArgument`/upstream 分类 | `TestLegacySDKConsumerCompiles` |
| `RecommendedUsers` | `RecommendedUsersRequest` → `sdk.Page[UserPreview]` | 保留；recommendation aggregate 不改旧 result model | continuation/response 错误不静默为空 | `TestLegacySDKConsumerCompiles` |
| `RelatedUsers` | `RelatedUsersRequest` → `sdk.Page[UserPreview]` | 保留；`UserID` 仍是 target identity | target/cursor 失配返回 `InvalidCursor` | `TestLegacySDKConsumerCompiles` |
| `UserFollowing` | `UserFollowingRequest` → `sdk.Page[UserPreview]` | 保留；relationship target 不改为通用 target | `Restrict`、user ID、cursor 字段保持 | `TestLegacySDKConsumerCompiles` |
| `UserFollowers` | `UserFollowersRequest` → `sdk.Page[UserPreview]` | 保留；relationship target 不改为通用 target | 同上；真实权限/transport 分类不吞掉 | `TestLegacySDKConsumerCompiles` |
| `UserBlockedUsers` | `UserBlockedUsersRequest` → `sdk.Page[UserPreview]` | 保留；blocked scope 继续由 user ID 定义 | 不用另一 user 或匿名 fallback | `TestLegacySDKConsumerCompiles` |
| `MyPixivUsers` | `MyPixivUsersRequest` → `sdk.Page[UserPreview]` | 保留；继续使用 verified current user | 身份未知 `Unauthorized`；cursor 不跨账号复用 | `TestLegacySDKConsumerCompiles` |
| `CurrentUser` | `CurrentUserRequest` → `UserDetail` | 保留；使用 verified identity 的 detail path | 身份未知 `Unauthorized`；不恢复旧 `/v1/user/me` | `TestLegacySDKConsumerCompiles` |
| `AddBookmark` | `AddBookmarkRequest` → `error` | 保留旧 wrapper；未来 explicit artwork bookmark method 委托同一语义 | 空 restrict 默认 `public`；未知值 `InvalidArgument`，不发请求 | `TestLegacySDKConsumerCompiles`、R04 no-network test |
| `RemoveBookmark` | `RemoveBookmarkRequest` → `error` | 保留旧 wrapper；未来 explicit method 不删除它 | 正数 ID；真实 upstream/transport 错误保持 | `TestLegacySDKConsumerCompiles` |
| `FollowUser` | `FollowUserRequest` → `error` | 保留原 mutation symbol | 空 restrict 默认 `public`；未知值 `InvalidArgument`，不发请求 | `TestFollowUserRejectsUnknownRestrictBeforeNetwork` |
| `UnfollowUser` | `UnfollowUserRequest` → `error` | 保留原 mutation symbol | 正数 user ID；不自动 read-back 或重放 | `TestLegacySDKConsumerCompiles` |
| `SetAIArtworkVisibility` | `SetAIArtworkVisibilityRequest` → `error` | 保留原 mutation symbol | 保留显式 upstream/transport 分类 | `TestLegacySDKConsumerCompiles` |

### Named types、字段与非 operation surface

- `SearchArtworksRequest` 的旧字段 `Word`、`Target`、`Sort`、`Duration`、`StartDate`、`EndDate`、`ContentType`、`AIMode`、`AspectRatio`、`Resolution`、`Tool`、`BookmarkMin`、`BookmarkMax`、`Cursor` 全部保留；`CursorContext` 是唯一已批准的 additive 字段，只进入 cursor digest、不发送 upstream。
- 其他 request 的 named fields 保持原样：`SearchNovelsRequest{Word,Target,Sort,Duration,Cursor}`、`SearchUsersRequest{Word,Cursor}`；各类 ID request 保留原 ID 名；paged request 保留 `Cursor`；bookmark/follow request 保留 `UserID`/`ArtworkID`、`Restrict`、`Tag`、`Visible`。不以通用 `Target`、动态 map 或匿名 struct 替换这些字段。
- named enum/type 保持原名与底层语义：`Restrict`、`ArtworkKind`、`SearchTarget`、`SortMode`、`DurationFilter`、`RankingMode`、`SearchContentType`、`SearchAIMode`、`SearchAspectRatio`、`SearchResolution`、`UgoiraQuality`、novel/comment block 与 mark kinds，以及其既有常量。`ArtworkKindIllustration="illustration"` 不改成会破坏源码的 `illust`。
- named result/model 保持原名与字段形状：`Artwork`、`ArtworkPage`、`ImageResource`、`Novel`、`NovelContent`、`NovelSeries`、`NovelSeriesResult`、`User`、`UserDetail`、`UserPreview`、`UserProfile`、`UserProfilePublicity`、`UserWorkspace`、`Comment`、`CommentPage`、`ArtworkBookmarkDetail`、`BookmarkTag`、`TrendingTag`、`UgoiraMetadata` 及其 DTO/显式转换器。新增 normalized DTO 不替换既有 runtime model。
- constructors、login、resource 与 shared error surface 也保持：`Open`/`OpenWith`、`New`/`NewWith`、`BeginLogin`、`Client.OpenResource`、`Client.SaveResource`、`CloseIdleConnections`、`UserID`、`Username`，以及 `sdk.Error`/`Reason`、`sdk.Page`、`sdk.Cursor` 的既有导出符号和 outer encoding。`ContentUnavailable` 是既有 reason，不新增“Unsupported”枚举。

旧消费者编译 fixture 只证明 source compatibility，不把尚未迁移的 v2 adapter、CLI/MCP wire 或 required capability 提升为 `public_ready`；这些仍分别由 T07–T45 的 owner 与最终 gate 验证。

## T39A：MCP compatibility map 必交付列

旧 tool → 新 operation；逐项 input 字段名/必填/default、output 字段/shape、structured error/isError。add_bookmark 的 illust_id 等旧 wire 契约不能被通用 TARGET 替代。每项列出旧 JSON 请求 fixture 与回放断言；CLI alias 验证不能替代它。

## T09A：mutation transport 与结果

现有 PostForm 返回 error，2xx 不交付响应 body。先增加响应可解码的窄能力供 comment adapter 取得本轮 ID；保留 bookmark/follow 的旧 PostForm。request/response/读回共享同账号执行上下文。

明确未成功、已取得 ID 但读回失败、写入结果不确定三类结果必须可区分；后两类禁止自动重放。无法可靠取得 ID 时不能猜测最近评论并删除，清理只接受本轮 ID；保留真实失败与需要人工处理的状态。无匿名 fallback，无通用 mutation 自动重试。
