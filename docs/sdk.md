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

### HTTP client 与请求生命周期

未提供 `Options.HTTPClient` 时，SDK 为该 `Client` 创建专用的 `http.Client`，其整请求 `Timeout` 为零；App API、Web API、OAuth 与资源读取复用这一个 client，不依赖全局可变的 `http.DefaultClient`。零值只表示 SDK 不添加覆盖 response body 读取的固定总时限；Go 默认 transport 的连接、TLS handshake 与 idle connection 等阶段策略保持不变。

每次操作的总生命周期由传入的 `context.Context` 控制。调用方应按操作建立 cancel 或 deadline；`context.Canceled` 与 `context.DeadlineExceeded` 可继续通过 `errors.Is` 判断。`OpenResource` 返回后，context 也覆盖后续 body 读取，调用方须关闭 body，并在不再消费流时取消 context。

显式提供 `Options.HTTPClient` 时，SDK constructor 保留同一指针及其 `Timeout`、`Transport`、cookie jar 与 redirect policy，不修改调用方对象。需要 client-wide timeout 的集成方可在该 client 上自行设置；SDK 不另加默认 timeout。资源请求仍按下文安全契约在逐请求副本上禁用 cookie 并包装 redirect 校验。完整决策见 [ADR 0010](adr/0010-http-client-timeout-and-context.md)。

## 读取与写入

`Client` 提供以下稳定的公开操作：

| 类别 | 方法 |
| --- | --- |
| 作品与推荐 | `SearchIllust`、`SearchIllustOptions`、`IllustDetail`、`IllustPages`、`IllustRelated`、`IllustRanking`、`IllustRecommended`、`MangaRecommended`、`NovelRecommended`、`UserRecommended`、`FollowingIllusts`、`TrendingTagsIllust`、`UgoiraMetadata`。 |
| 用户 | `SearchUser`、`UserDetail`、`UserArtworks`、`UserBookmarks`、`UserFollowing`、`CurrentUserID`。 |
| 写操作 | `AddBookmark`、`RemoveBookmark`、`FollowUser`、`UnfollowUser`。 |
| 账号/配置 | `ImportAccount`、`ListAccounts`、`SelectAccount`、`RemoveAccount`、`CheckAccount`、`CheckRefreshToken`、`Refresh`、`RefreshAccount`、`GetConfig`、`SetConfig`、`UnsetConfig`。 |
| 登录 | `StartLogin`、`CompleteLogin`、`BuildLoginAuthorizationURL`。SDK 不启动浏览器、loopback server 或 TTY。 |
| 资源 | `ParseResourceRef`、`OpenResource`、`Download`。 |

请求型方法使用命名 request，例如 `SearchIllustRequest`、`SearchIllustOptionsRequest`、`UserArtworksRequest`、`UserBookmarksRequest`、`UserFollowingRequest`、`AddBookmarkRequest`、`FollowUserRequest`。返回模型为 `IllustListResult`、`SearchIllustOptionsResult`、`UserListResult`、`IllustDetail`、`UserDetailResult` 等，均来自顶层 `pixiv` package。

`UserArtworksRequest.UserID` 等 SDK 用户 ID 必填；“省略 UID 就是自己”是 CLI/MCP adapter 行为，外部 Go 调用方先调用 `CurrentUserID(ctx)` 后再组装 request。

`UserDetail` 固定返回 `UserDetailResult{User, Profile, ProfilePublicity, Workspace}` 四个 envelope。上游任一 envelope 缺失、`null`、非 object 或 `user.id <= 0` 时，SDK 返回带 `OperationUserDetail`、`BackendAppAPI` 和请求 UID 的 `malformed_upstream_response`；不会暴露上游 body、URL 或凭据。`User.ProfileImageURLs.Medium`、`Profile` 中的网页/背景/社交 URL 以及 `Workspace.WorkspaceImageURL` 均是可选指针，缺失、`null` 与空字符串统一为 `nil`；未公开的文本、计数和字段保持 Go 零值。

四类个性化推荐均是 App API 认证操作：插画/漫画使用 `IllustRecommendedRequest`，小说使用 `NovelRecommendedRequest`，作者使用 `UserRecommendedRequest`；各自返回独立的 opaque `NextCursor`。CLI/MCP 的 `all` 仅是边缘层按插画、漫画、小说、作者顺序组合四次 SDK 调用，不改变 SDK 的单流 cursor 契约。

认证输入只能是原始 Pixiv App API refresh token。`ImportAccount`、`CheckRefreshToken`、`OpenDefault` 和由本地账号读取到的 token 会拒绝 Cookie 形态（包括 `refresh_token=...`），返回不含原始输入的 `invalid_argument`，且不会发起 OAuth 请求。

`BuildLoginAuthorizationURL(challenge, state)` 仅构造官方授权 URL，适合自行持有 PKCE/state 的浏览器 adapter；它不生成或保存凭据。需要 SDK 管理 PKCE/session 时使用 `StartLogin`。

### 插画搜索筛选

`SearchIllustRequest.Filters` 是独立于 App/Web wire 参数的稳定 `SearchIllustFilters`：

| 字段 | 稳定值 |
| --- | --- |
| `Rating` | `all`、`sfw`、`r18`、`r18g`、`mature` |
| `ContentType` | `all`、`illust-and-ugoira`、`illust`、`manga`、`ugoira` |
| `AIMode` | `all`、`exclude`、`only`；Pixiv `AIType==2` 表示 AI 生成 |
| `AspectRatio` | `all`、`landscape`、`portrait`、`square` |
| `Resolution` | `all`、`high`、`medium`、`low`；三档分别要求宽高均 `>=3000`、均在 `1000..2999`、均 `<=999` |
| `Tool` | 上游绘图工具原值；不做模糊匹配 |

枚举零值规范化为 `all`，`Tool` 会去除首尾空白；未知枚举返回 `invalid_argument`，不会发起上游
请求。认证路径把分辨率、横纵比、工具、作品类型和 `exclude` AI 翻译为 App 服务端参数；分级与
`only` AI 再基于当前 App 批次的规范化字段筛选。`Illust.Tools []string` 保留 App 返回的工具顺序和
原值；该字段不是收藏数筛选。

`SearchIllustOptions(ctx, SearchIllustOptionsRequest{Word: word})` 需要非空关键词和 App 认证，返回
`SearchIllustOptionsResult{Tools []string}`。工具列表保持上游顺序与原值；上游未提供列表时返回非
`nil` 空切片。该操作不公开 Premium 收藏数档位。

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

cursor 是版本化、不透明、绑定操作和完整查询的 token；`SearchIllust` cursor 同时绑定规范化后的
`Rating`、`ContentType`、`AIMode`、`AspectRatio`、`Resolution` 与 `Tool`。改变任一筛选字段后复用
旧 cursor 会返回 `invalid_argument`。cursor 不可解析、编辑、跨请求复用或替换为上游 offset/page。
SDK 不以 `page` 为输入；CLI/MCP 在边缘层将逻辑 `page`/`limit` 转为 cursor 遍历。

## 路由

有 refresh token 时，插画搜索只走 App API；App 的认证、网络、服务端失败不自动 Web fallback。
`NewClient` 无 refresh token 且 `WebFallbackEnabled=true` 时，匿名白名单读操作使用 Web API；
`OpenDefault` 则每次 snapshot 读取本地 `web_fallback_enabled`。匿名 `SearchIllust` 只执行 Web 能可靠
表达的筛选；`Rating` 为 `r18`、`r18g` 或 `mature` 时在联网前返回 `unauthorized`，不会伪装为空结果。
匿名 `SearchIllustOptions` 返回 `unsupported`，不会请求 Web。SDK 不读取或注入 Cookie，也不把 refresh
token 转换为 Web session。

`IllustDetail` 的 pages 和 original ugoira metadata 会调用 Web 做明确补全，不是失败回退，并采用原子结果契约：

- 认证 `IllustDetail` 先读取 App detail，再读取 Web pages。即使 App 响应自带 `MetaPages`，Web pages 失败也返回 `nil` 与 typed error，不返回无标记的 App partial result。
- `UgoiraMetadata` 的 App metadata 只有 medium zip；Web metadata 未能提供 original 时返回 `nil` 与 typed error，不暗中降级质量。
- 匿名 `IllustDetail` 依次读取 Web detail 与 pages；任一阶段失败都不返回 partial result。

SDK 不向 Web 补全请求注入 App bearer 或 Cookie。App `MetaPages` 可被 wire model 表达和 mapper 保留，但 SDK 不把这一能力解释为上游对所有作品的完整性保证。完整决策与未来引入显式 partial-result 状态的门槛见 [ADR 0006](adr/0006-original-ugoira-resource-resolution.md)。

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

补全失败的阶段可直接从 typed error 观察：

| 调用与失败阶段 | 返回结果 | `Operation` | `Backend` |
| --- | --- | --- | --- |
| 认证 `IllustDetail` 的 App detail 失败 | `nil` | `OperationIllustDetail` | `BackendAppAPI` |
| 认证或匿名 `IllustDetail` 的 Web pages 失败 | `nil` | `OperationIllustPages` | `BackendWebAPI` |
| 匿名 `IllustDetail` 的 Web detail 失败 | `nil` | `OperationIllustDetail` | `BackendWebAPI` |
| `UgoiraMetadata` 的 Web metadata 补全失败 | `nil` | `OperationUgoiraMetadata` | `BackendWebAPI` |

例如登录墙返回 HTTP 403 时，pages 补全错误为 `CodeForbidden`、`BackendWebAPI`、`OperationIllustPages`，并保留 `UpstreamStatus=403`；App detail 失败时不会继续请求 Web。调用方应按这些字段处理失败，不应从结果中猜测补全是否完成。

`upstream_unavailable` 的网络传输失败还可通过 `Error.TransportKind` 区分安全子类：`dns`、`tls`、`proxy`、`connection_refused`、`connection_reset`、`unknown`。分类只依据 Go 标准库的 typed/wrapped cause，不解析错误文本；例如没有 typed 信号的 HTTPS proxy CONNECT 非 200 文本错误会保持 `unknown`。`Error()` 只输出稳定枚举，不输出 DNS name、代理 userinfo、证书内容或原始 cause。`context.Canceled` 与 `context.DeadlineExceeded` 不设置 transport 子类，继续通过 `errors.Is` 判断。

`OpenDefault` 和本地账号/配置操作的 `invalid_argument` 还可通过 `Error.LocalStateKind` 区分安全子类：`auth_malformed`、`config_malformed`、`permission_denied`、`not_found`、`invalid_proxy`、`account_mismatch`、`unavailable`、`unknown`。顶层 code、operation、backend、user ID 与 retryable 语义保持不变；`account_mismatch` 仍带 `oauth` backend 和所选 user ID。`errors.Unwrap` 只返回固定的脱敏原因，不返回原始 filesystem/parser 错误、路径、配置/auth 内容或含 userinfo 的代理 URL；`Error()` 也只输出白名单枚举。正常加载时缺失的可选 `auth.json` 或 `config.toml` 继续视为空状态并成功，不会产生 `not_found`。

## 调用方责任

调用方 adapter 决定采集模式、budget、filter、cursor 存储、数据库事务、任务调度、重试与对外 HTTP API。`atri-setu-api` 的随机选图、审查、图库和图片代理不属于 SDK；它可使用 SDK 的规范化模型和资源流实现这些功能。

更多边界说明见 [ADR 0009](adr/0009-public-pixiv-sdk-and-caller-adapter.md) 与 [ADR 0010](adr/0010-http-client-timeout-and-context.md)。
