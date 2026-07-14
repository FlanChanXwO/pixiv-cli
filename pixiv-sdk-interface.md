# Pixiv Go SDK 接口

本文件取代旧 HTTP Provider interface。公开入口是 `github.com/FlanChanXwO/pixiv-cli/pixiv` 的具体 `*pixiv.Client`，不是 HTTP endpoint、Provider server 或可发现服务。

调用方若需要接口，应在自身 adapter 内定义最小方法集；SDK 不输出 `Discover`、Probe、Capabilities、RSS 或 crawler。

## 构造

```go
client, err := pixiv.NewClient(pixiv.Options{
    AccessToken: accessToken,
    Logger:      logger, // 可选；nil 时 SDK 静默
})

local, err := pixiv.OpenDefault(pixiv.Options{
    UserID: 12345678, // 可选本地账号
})
```

`NewClient` 不读本地文件，也不网络认证。`OpenDefault` 使用 `AuthFilePath`、`ConfigFilePath`、`RefreshToken`、`UserID` 或现有默认路径和环境选择认证；每个公开操作重新取得一次 configuration/auth snapshot。多次续页若要求同一 snapshot，调用 `client.Snapshot(ctx)`。

`Options` 支持显式 `HTTPClient`、`AppAPIBaseURL`、`WebAPIBaseURL`、`OAuthBaseURL`、`WebFallbackEnabled`、`ResourcePolicy` 与 `Logger`。`AccessToken` 与 `WebFallbackEnabled` 只供 `NewClient`；`OpenDefault` 每次 snapshot 从本地 `web_fallback_enabled` 读取 Web fallback 设置。不要把 refresh token 或 logger 全局化。

## 读取与写入

`Client` 提供以下稳定的公开操作：

| 类别 | 方法 |
| --- | --- |
| 作品与推荐 | `SearchIllust`、`IllustDetail`、`IllustPages`、`IllustRelated`、`IllustRanking`、`IllustRecommended`、`MangaRecommended`、`NovelRecommended`、`UserRecommended`、`FollowingIllusts`、`TrendingTagsIllust`、`UgoiraMetadata`。 |
| 用户 | `SearchUser`、`UserDetail`、`UserArtworks`、`UserBookmarks`、`UserFollowing`、`CurrentUserID`。 |
| 写操作 | `AddBookmark`、`RemoveBookmark`、`FollowUser`、`UnfollowUser`。 |
| 账号/配置 | `ImportAccount`、`ListAccounts`、`SelectAccount`、`RemoveAccount`、`CheckAccount`、`CheckRefreshToken`、`Refresh`、`RefreshAccount`、`GetConfig`、`SetConfig`、`UnsetConfig`。 |
| 登录 | `StartLogin`、`CompleteLogin`、`BuildLoginAuthorizationURL`。SDK 不启动浏览器、loopback server 或 TTY。 |
| 资源 | `ParseResourceRef`、`OpenResource`、`Download`。 |

请求型方法使用命名 request，例如 `SearchIllustRequest`、`UserArtworksRequest`、`UserBookmarksRequest`、`UserFollowingRequest`、`AddBookmarkRequest`、`FollowUserRequest`。返回模型为 `IllustListResult`、`UserListResult`、`IllustDetail`、`UserDetailResult` 等，均来自顶层 `pixiv` package。

`UserArtworksRequest.UserID` 等 SDK 用户 ID 必填；“省略 UID 就是自己”是 CLI/MCP adapter 行为，外部 Go 调用方先调用 `CurrentUserID(ctx)` 后再组装 request。

`UserDetail` 固定返回 `UserDetailResult{User, Profile, ProfilePublicity, Workspace}` 四个 envelope。上游任一 envelope 缺失、`null`、非 object 或 `user.id <= 0` 时，SDK 返回带 `OperationUserDetail`、`BackendAppAPI` 和请求 UID 的 `malformed_upstream_response`；不会暴露上游 body、URL 或凭据。`User.ProfileImageURLs.Medium`、`Profile` 中的网页/背景/社交 URL 以及 `Workspace.WorkspaceImageURL` 均是可选指针，缺失、`null` 与空字符串统一为 `nil`；未公开的文本、计数和字段保持 Go 零值。

四类个性化推荐均是 App API 认证操作：插画/漫画使用 `IllustRecommendedRequest`，小说使用 `NovelRecommendedRequest`，作者使用 `UserRecommendedRequest`；各自返回独立的 opaque `NextCursor`。CLI/MCP 的 `all` 仅是边缘层按插画、漫画、小说、作者顺序组合四次 SDK 调用，不改变 SDK 的单流 cursor 契约。

认证输入只能是原始 Pixiv App API refresh token。`ImportAccount`、`CheckRefreshToken`、`OpenDefault` 和由本地账号读取到的 token 会拒绝 Cookie 形态（包括 `refresh_token=...`），返回不含原始输入的 `invalid_argument`，且不会发起 OAuth 请求。

`BuildLoginAuthorizationURL(challenge, state)` 仅构造官方授权 URL，适合自行持有 PKCE/state 的浏览器 adapter；它不生成或保存凭据。需要 SDK 管理 PKCE/session 时使用 `StartLogin`。

## 分页

列表 result 的 `NextCursor` 类型为 `pixiv.Cursor`。将它原样传到同一个请求的 `Cursor` 字段：

```go
result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: uid})
if err != nil { /* handle */ }
next, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{
    UserID: uid,
    Cursor: result.NextCursor,
})
_ = next
```

cursor 是版本化、不透明、绑定操作和查询的 token；不可解析、编辑、跨请求复用或替换为上游 offset/page。SDK 不以 `page` 为输入；CLI/MCP 在边缘层将逻辑 `page`/`limit` 转为 cursor 遍历。

## 路由

有 refresh token 时，App API 是主路径；App 的认证、网络、服务端失败不自动 Web fallback。`NewClient` 无 refresh token 且 `WebFallbackEnabled=true` 时，匿名白名单读操作使用 Web API；`OpenDefault` 则每次 snapshot 读取本地 `web_fallback_enabled`。`IllustDetail` 的 pages 和原始 ugoira metadata 可能调用 Web 做明确补全，不是失败回退。

## 资源与图片代理

```go
ref, err := client.ParseResourceRef(rawURL)
if err != nil { /* reject */ }
response, err := client.OpenResource(ctx, pixiv.OpenResourceRequest{
    Ref: ref, Range: request.Header.Get("Range"),
})
if err != nil { /* map typed error */ }
defer response.Body.Close()
// 使用 response.StatusCode、response.Header，流式 io.Copy 到下游。
```

`ResourceRef` 只是可持久化引用；每次 `OpenResource` 都重新校验。默认仅官方 Pixiv 资源，调用方可在 `ResourcePolicy.Mirrors` 加入明确 host/path prefix。SDK 只接受 `Range`、`If-None-Match`、`If-Modified-Since` 条件 header，过滤响应 header，禁用 cookie，并验证 redirect，避免 SSRF。`Download` 在本地以流式临时文件加原子替换落盘。

## 错误

所有公开失败可为 `*pixiv.Error`：

```go
var pixivErr *pixiv.Error
if errors.As(err, &pixivErr) {
    switch pixivErr.Code {
    case pixiv.CodeArtworkUnavailable:
        // 删除、私密、地区/权限不可用等可跳过项目。
    case pixiv.CodeRateLimited:
        // 调用方按自身策略调度。
    }
}
if errors.Is(err, pixiv.ErrUnauthorized) { /* re-auth */ }
```

稳定 code 包括 `invalid_argument`、`artwork_unavailable`、`unauthorized`、`forbidden`、`unsupported`、`rate_limited`、`upstream_error`、`upstream_unavailable`、`malformed_upstream_response`。错误带 operation/backend/retryable/status/已验证 ID；不含 token、cookie、完整 URL、header 或上游响应 body。

## 调用方责任

调用方 adapter 决定采集模式、budget、filter、cursor 存储、数据库事务、任务调度、重试与对外 HTTP API。`atri-setu-api` 的随机选图、审查、图库和图片代理不属于 SDK；它可使用 SDK 的规范化模型和资源流实现这些功能。

更多替换边界见 [PRD](pixiv-integration-replacement-prd.md) 与 [ADR 0009](docs/adr/0009-public-pixiv-sdk-and-caller-adapter.md)。
