# v1.0.0 PixivPy App API 能力兼容矩阵

## 基线与兼容定义

基线固定为 PixivPy commit
[`4f2e9ea7fff6247d9f5bfe5a862e92c5dfe3b6dd`](https://github.com/upbit/pixivpy/blob/4f2e9ea7fff6247d9f5bfe5a862e92c5dfe3b6dd/pixivpy3/aapi.py)
中的 `AppPixivAPI`。v1 的目标是覆盖其仍有效的产品能力，不复制 Python 方法签名、动态返回 DTO、
query-string continuation 或 transport helper。Go API 使用 typed request、`sdk.Page[T]`、
`sdk.Cursor`、`*sdk.Error` 与显式资源模型；Pixiv 产品 Client 位于 `sdk/pixiv`。

这份矩阵是 release candidate 的契约门禁。基线方法必须映射为“直接”“等价”或“明确排除”，不能
因实现困难静默遗漏。v1 可以提供基线之外的 Go-native 能力，但不得借“兼容 PixivPy”冻结其内部实现。

## 读取能力

| PixivPy `AppPixivAPI` | v1 Pixiv operation | 状态与说明 |
|---|---|---|
| `user_detail` | `User` | 直接 |
| `user_illusts` | `UserArtworks` | 直接；`Artwork.Kind` 区分插画、漫画和 ugoira |
| `user_bookmarks_illust` | `UserArtworkBookmarks` | 直接 |
| `user_bookmarks_novel` | `UserNovelBookmarks` | 直接 |
| `user_related` | `RelatedUsers` | 直接 |
| `user_recommended` | `RecommendedUsers` | 直接 |
| `user_following` | `UserFollowing` | 直接 |
| `user_follower` | `UserFollowers` | 直接 |
| `user_mypixiv` | `MyPixivUsers` | 直接 |
| `user_list` | `UserBlockedUsers` | 等价；用领域名代替含糊的 upstream 名称 |
| `illust_follow` | `FollowingArtworks` | 直接 |
| `illust_detail` | `Artwork` | 直接 |
| `illust_comments` | `ArtworkComments` | 直接；返回 `CommentPage` |
| `illust_related` | `RelatedArtworks` | 直接 |
| `illust_recommended` | `RecommendedArtworks` | 直接 |
| `illust_ranking` | `ArtworkRanking` | 直接 |
| `trending_tags_illust` | `TrendingArtworkTags` | 直接 |
| `search_illust` | `SearchArtworks` | 直接 |
| `illust_bookmark_detail` | `ArtworkBookmark` | 直接 |
| `user_bookmark_tags_illust` | `UserArtworkBookmarkTags` | 直接 |
| `ugoira_metadata` | `UgoiraMetadata` | 等价；archive 使用统一 `Resource` |
| `illust_new` | `LatestArtworks` | 直接 |
| `search_novel` | `SearchNovels` | 直接 |
| `novel_detail` | `Novel` | 直接 |
| `novel_series` | `NovelSeries` | 等价；返回 series metadata 与 `sdk.Page[Novel]` |
| `novel_comments` | `NovelComments` | 直接；返回 `CommentPage` |
| `novel_recommended` | `RecommendedNovels` | 直接 |
| `novel_new` | `LatestNovels` | 直接 |
| `novel_follow` | `FollowingNovels` | 直接 |
| `user_novels` | `UserNovels` | 直接 |
| `webview_novel` | `NovelContent` | 等价；解析为结构化内容，不公开 raw webview HTML |
| `search_user` | `SearchUsers` | 直接 |

`CommentPage` 在 `sdk.Page[Comment]` 之外保留上游确实提供的 total 与访问控制信息；
`NovelSeries` 结果同时包含 series metadata 和分页小说。`NovelContent` 必须完整表示正文语义，若
上游格式无法安全解析则返回 `malformed_upstream_response`，不能用空内容冒充成功。

## Mutation 与连接能力

| PixivPy 能力 | v1 Pixiv operation/构造 | 状态与说明 |
|---|---|---|
| `illust_bookmark_add` | `AddBookmark` | 等价；request 支持 restrict 与 tags |
| `illust_bookmark_delete` | `RemoveBookmark` | 直接 |
| `user_follow_add` | `FollowUser` | 直接 |
| `user_follow_delete` | `UnfollowUser` | 直接 |
| `user_edit_ai_show_settings` | `SetAIArtworkVisibility` | 等价；使用明确领域名与 typed request |
| `auth` / refresh-token login | `Open` / `OpenWith` | 等价；返回 rotation 后凭据，Client 不持有 refresh token |
| `set_auth` | `New` / `NewWith` | 等价；显式 access token，不接受 username/password grant |
| OAuth browser login | `BeginLogin` / `LoginSession.Complete` | Go-native 补充；无状态 PKCE session |
| `set_accept_language` | `Options.AcceptLanguage` | 等价；构造时固定连接选项 |
| `download` | `Resource.URL` 或 `OpenResource` / `SaveResource` | 等价；支持直接流式反代和 SDK 校验读取 |

## 明确排除的基线接口

| PixivPy 接口 | v1 处理 | 排除理由 |
|---|---|---|
| `showcase_article` | 不提供 | 属于 Web API，不是 App API 产品能力；v1 删除 Web backend |
| `novel_text` | 不单独提供 | PixivPy 已标记 deprecated；由结构化 `NovelContent` 覆盖 |
| raw `webview_novel` HTML | 不提供 | 冻结 HTML/脚本会把不稳定 Web 展示层变成公开 SDK 契约 |
| `req_auth=False` | 不提供 | 2026-08-03 无凭据探测显示代表性 App endpoint 拒绝请求；v1 内容操作统一 auth-only |
| username/password grant | 不提供 | 不采用过时凭据流；使用 refresh token 或 `LoginSession` |
| `parse_qs` / `parse_result` / raw response | 不提供 | 分页由 opaque `sdk.Cursor` 表达；协议 DTO 与原始响应保持内部 |
| `set_api_proxy`、SNI、任意 header mutation | 不提供同名 helper | 调用方显式注入 `http.Client`；SDK 仍执行官方 host 与资源安全校验 |
| `_load_model`、`format_bool`、`no_auth_requests_call` 等 helper | 不提供 | 属于 PixivPy 内部实现，不是产品能力 |

## 匿名能力结论

2026-08-03 在不带 token、Cookie 的环境中，以官方 App host 探测 detail、ranking、search、user detail
和 trending tags，均收到 OAuth `invalid_request`；历史 no-login endpoint 返回不存在。该结果只证明
当前代表性核心能力不能匿名使用，不宣称所有未来 endpoint 永远如此。v1 因此把有效 access token
作为所有内容 operation 的前置条件，并以 `unauthorized` 明确失败，不维护匿名分支。
