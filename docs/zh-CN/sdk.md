# Pixiv SDK (v1)

[English](../en/sdk.md) | 简体中文 | [文档索引](../index-zh-CN.md)

v1 SDK 暴露三个公开包：

- `github.com/FlanChanXwO/pixiv-cli/sdk` — 两个产品共享的协议无关原语：分页、不透明游标、分类错误与资源契约。
- `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv` — Pixiv App API 客户端、模型、URL 引用与 mutation。
- `github.com/FlanChanXwO/pixiv-cli/sdk/fanbox` — Pixiv FANBOX 客户端、模型与 URL 解析。

所有导出声明均带英文 GoDoc；源码是 API 的权威摘要。

## 快速开始

一个完整的 Pixiv 流程：认证、搜索、取详情、保存图片。

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func main() {
	ctx := context.Background()

	// 1. 通过 OAuth rotation 认证。在发起内容请求前持久化新的 refresh token；
	//    client 不会自动 refresh。
	client, creds, err := pixiv.Open(ctx, os.Getenv("PIXIV_REFRESH_TOKEN"))
	if err != nil {
		panic(err)
	}
	_ = creds // 持久化 creds.RefreshToken() 到可靠存储

	// 2. 搜索作品并按 typed page cursor 迭代。
	page, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "miku"})
	if err != nil {
		panic(err)
	}
	if len(page.Items) == 0 {
		return
	}
	first := page.Items[0]

	// 3. 取作品页面（图片资源）。
	pages, err := client.ArtworkPages(ctx, pixiv.ArtworkPagesRequest{ArtworkID: first.ID})
	if err != nil {
		panic(err)
	}

	// 4. 通过 SDK 校验的资源路径保存第一张图。
	_, err = client.SaveResource(ctx, sdk.SaveOptions{
		Ref:  pages[0].Image.Resource.Ref,
		Dest: "./first.png",
	})
	if err != nil {
		panic(err)
	}
	fmt.Println("saved", first.ID)
}
```

一个 FANBOX 流程：用 session 打开、列出 supporting 帖子、解析某帖资源。

```go
client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: sess})
if err != nil {
    panic(err)
}
page, err := client.Supporting(ctx, fanbox.SupportingRequest{})
if err != nil {
    panic(err)
}
if len(page.Items) == 0 {
    return
}
post, err := client.Post(ctx, fanbox.PostRequest{PostID: page.Items[0].ID})
if err != nil {
    panic(err)
}
_ = post // post.Body 的 block 携带 image/file asset 及其 Resource ref
```

## 认证

Pixiv 只走 App API。每个内容操作都需要有效 access token。

```go
client, creds, err := pixiv.Open(ctx, refreshToken) // OAuth rotation
// 在发起内容请求前持久化 creds.RefreshToken()

client, err := pixiv.New(accessToken) // 静态 token，无网络 I/O
```

`Open` 返回只持有 access token 的 Client，它不会自动 refresh。token 过期后操作
返回 `CredentialsExpired`。OAuth 成功响应必须包含正数 account user ID；缺少身份时
`Open` 返回 `MalformedUpstreamResponse`，不返回 Client 或 credentials。不存在匿名或
Web fallback。

### 以编程方式发起浏览器登录

`BeginLogin` 创建 self-contained、one-shot 的 PKCE session。它不会打开浏览器或
启动 loopback listener；调用方自行打开 `AuthorizationURL()`，再把 callback URL 或
bare code 交给 `Complete`。

```go
session, err := pixiv.BeginLogin(pixiv.LoginOptions{HTTPClient: httpClient})
if err != nil { /* 处理错误 */ }
if !session.AcceptsCallbackURL(callbackURL) {
    // 在消耗 one-shot session 前拒绝 callback。
}
credentials, err := session.Complete(ctx, callbackURL)
```

`AcceptsCallbackURL` 是非消耗性的，不联网。官方 HTTPS callback 必须包含本 session 的
`state`；支持的 `pixiv://account/login` callback 可以省略 `state`，但如果提供则必须匹配。
`IsOfficialOAuthCallbackURL` 与 `IsOfficialOAuthStartURL` 会校验精确 origin/path，且不访问 Pixiv。
session 的格式化输出不会暴露 verifier、state、authorization code 或 callback URL。

FANBOX 使用显式 `FANBOXSESSID` 值认证：

```go
client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: session})
```

> [!IMPORTANT]
> Pixiv refresh token 与 FANBOX session 相互独立，永不转换。

FANBOX 的连接选项均为显式可选项：

```go
client, err := fanbox.OpenWith(credentials, fanbox.Options{
    ProxyURL:  "https://proxy.example:8443", // 仅 native HTTP(S) CONNECT
    UserAgent: "my-native-agent/1.0",          // 仅修改 native header
    FlareSolverr: &fanbox.FlareSolverrOptions{
        URL:      "http://127.0.0.1:8191",
        ProxyURL: "socks5://solver-upstream.example:1080",
    },
})
```

生产 native transport 使用 tls-client 的 Chrome 146 TLS profile。空 `UserAgent` 使用内置 Firefox 148 HTTP header baseline；自定义值不会改变 TLS profile，也不保证能绕过 Cloudflare。
`FlareSolverr` 为 nil 时完全关闭，只有 native 请求被严格识别为 Cloudflare challenge 后才会调用。
solver service URL 与 upstream proxy 独立于 native proxy；public constructor 不联网。

## 分页

列表操作返回 `sdk.Page[T]` 与不透明 `Cursor`：

```go
page, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "miku"})
for {
    for _, artwork := range page.Items { /* ... */ }
    if page.Next.IsZero() { break }   // 没有剩余 cursor 时停止
    request.Cursor = page.Next
    page, err = client.SearchArtworks(ctx, request)
}
```

> [!NOTE]
> 游标绑定 product、operation、binding version 与查询摘要；用不同查询复用游标会返回 `InvalidCursor`。

对 identity-scoped operation，`pixiv.New` 创建的 Client 没有已验证的账号 ID，
因此其续页 cursor 是 ephemeral，并携带只用于绑定该 Client 实例的非敏感标识；
同一 Client 可以继续，其他 Client 或进程会返回 `InvalidCursor`。通过
`pixiv.Open` 创建的 Client 则把 cursor 绑定到已验证的账号 identity。

> [!WARNING]
> Cursor 编码只提供续读状态和绑定校验，不提供密码学意义上的真实性校验。
> Cursor 不是鉴权凭据；不要把 secret 放进 identity、context 或 payload，也不要在
> 不可信边界接收 cursor 时依赖其防篡改能力。`SearchArtworks` 的 binding version
> 变更还要求 shared collector、SDK 和调用方作为整体回滚；已经发出的 version-2
> cursor 不能由 version-2 之前的搜索实现消费。

### 作品搜索批内续读

`SearchArtworks` 的普通批次 cursor 与 checkpoint 均使用 binding version **2**。
旧版本 1 搜索 cursor 返回 `InvalidCursor`；清除旧 cursor 后重新查询。
其他 operation 的 binding version 与 `sdk.Cursor` 外层编码不变。

在批次内停止时，使用**取得该批次的原请求**调用
`client.CheckpointSearchArtworks(request, consumed)`，不能传入该批次的 `page.Next`。
`consumed` 为正数，按 SDK 规范化及 AI 筛选后的条目计数，包含调用方随后过滤或 Skip
消费的项目。从已恢复请求再次建立 checkpoint 时累计消费位置；批次全部消费后使用
`page.Next`。checkpoint 构造不联网；超出批次的位置在恢复请求时返回 `InvalidCursor`，
非正数或整数溢出返回 `InvalidArgument`。

将返回 cursor 通过同一 `SearchArtworksRequest.Cursor` 恢复，可用 Text/JSON codec 持久化。
重复所有查询字段和可选 `CursorContext`；后者由调用方表达本地筛选语义，只进入摘要，
不发送上游。本地筛选语义改变时必须改变该 context。CLI/MCP 收藏过滤搜索共用实际策略
和收藏上下界的摘要；本轮不新增 CLI flag 或 MCP 字段。

`SearchArtworks` cursor 绑定 `Open/OpenWith` 的已验证账号；没有 verified identity 的
`New/NewWith` 只能同一 client 实例恢复。跨账号或跨实例返回 `InvalidCursor`。
恢复顺序为上游批次 → SDK 规范化及 AI 筛选 → 已消费前缀 → 调用方筛选与逻辑 limit。
稳定源序列下可避免遗漏和重复；重新请求实时批次不构成快照，无法保证上游插入、删除、
重排时的数据稳定性。保存的位置超出当前批次时返回 `InvalidCursor`，不静默重启。

`SearchNovels` 与 `SearchUsers` 刻意采用 public-scoped cursor：cursor 绑定
product、operation、binding version 与 query，不绑定 verified account 或 client
instance。相同查询的 cursor 可以交给另一个 client 恢复；这是冻结的源码兼容策略，
不表示所有搜索 operation 都属于账号作用域。

接受上游 continuation 的分页 operation，其所有非零 continuation 都必须是正数的
typed position。offset 形式会在发起 transport 前拒绝零值、负值和溢出值；显式 value
形式会拒绝 `last_order`、`max_bookmark_id`、`max_illust_id` 等非正数。上文所述的
recommended feed 仍保留显式 `offset=0` 的续读例外。

## Pixiv 读取操作

| 操作 | 入参要点 | 返回 | 常见错误 |
| --- | --- | --- | --- |
| `SearchArtworks` | 关键词、target、排序、日期边界、类型、AI、横纵比、分辨率、工具、收藏数边界 | `Page[Artwork]` | `InvalidArgument`（未知枚举、非法日期、非法收藏范围） |
| `SearchNovels` | 关键词、target、排序、duration | `Page[Novel]` | `InvalidArgument` |
| `SearchUsers` | 关键词 | `Page[User]` | `InvalidArgument` |
| `ArtworkRanking` | mode（默认 `day`）、可选 `YYYY-MM-DD` | `Page[Artwork]` | `InvalidArgument` |
| `RecommendedArtworks` | cursor | `Page[Artwork]` | `InvalidCursor` |
| `FollowingArtworks` | `restrict`（`public`/`private`）、cursor | `Page[Artwork]` | `InvalidArgument`、`InvalidCursor` |
| `LatestArtworks` | content type（默认 `illust` 或 `manga`）、cursor | `Page[Artwork]` | `InvalidArgument`、`InvalidCursor` |
| `NovelRanking` | mode（默认 `day`）、cursor | `Page[Novel]` | `InvalidArgument`、`InvalidCursor` |
| `RecommendedNovels` | cursor | `Page[Novel]` | `InvalidCursor` |
| `FollowingNovels` | `restrict`（`public`/`private`）、cursor | `Page[Novel]` | `InvalidArgument`、`InvalidCursor` |
| `LatestNovels` | cursor | `Page[Novel]` | `InvalidCursor` |
| `Stamps` | 无 query 或 cursor 字段 | `[]Stamp` | `MalformedUpstreamResponse`、已分类的上游/传输错误 |
| `Artwork` / `Novel` / `User` | 正数 typed ID | 详情记录 | `NotFound`、`InvalidArgument` |
| `ArtworkSeries` / `NovelSeries` | 正数 series ID、cursor | 系列分页（novel 还返回系列 metadata） | `InvalidCursor` |
| `ArtworkComments` / `NovelComments` | 正数 ID、cursor | `CommentPage` | `NotFound` |
| `PostArtworkComment` / `ReplyArtworkComment` / `DeleteArtworkComment` | 正数 artwork ID；reply 还要求正数 parent comment ID | post/reply 返回 `CommentMutationResult`；delete 返回 `error` | `InvalidArgument`、`MalformedUpstreamResponse`、已分类的上游/传输错误 |
| `StampArtworkComment` | 正数 artwork ID、非空 comment、正数 stamp ID | `CommentMutationResult` | `InvalidArgument`、`MalformedUpstreamResponse`、已分类的上游/传输错误 |
| `PostNovelComment` / `ReplyNovelComment` / `DeleteNovelComment` | 正数 novel ID；reply 还要求正数 parent comment ID | post/reply 返回 `CommentMutationResult`；delete 返回 `error` | `InvalidArgument`、`MalformedUpstreamResponse`、已分类的上游/传输错误 |
| `StampNovelComment` | 正数 novel ID、非空 comment、正数 stamp ID | `CommentMutationResult` | `InvalidArgument`、`MalformedUpstreamResponse`、已分类的上游/传输错误 |
| `UserArtworkBookmarks` / `UserArtworkBookmarkTags` / `UserNovelBookmarks` / `UserNovelBookmarkTags` | `UserID`、`Restrict`、`tag`、cursor | typed 分页 | `InvalidArgument`、`InvalidCursor` |
| `ArtworkBookmark` / `NovelBookmark` | 正数 artwork 或 novel ID | 收藏详情状态 | `InvalidArgument`、`MalformedUpstreamResponse`、已分类的上游/传输错误 |
| `AddArtworkBookmark` / `RemoveArtworkBookmark`（旧 `AddBookmark` / `RemoveBookmark`） | 正数 artwork ID；add 接受 `Restrict` 与 tags | `error` | `InvalidArgument`、已分类的上游/传输错误 |

`NovelContent` 为兼容旧 v1 调用方而保留导出符号，但已标记为 deprecated：已
rejected 的 `/v1/novel/content` App API endpoint 与被排除的 WebView path 都不会
调用，方法不产生网络请求并返回 `ContentUnavailable`。当前 v1 contract 没有正文
endpoint 替代入口。

关键语义：

- `CurrentUser` 通过 `/v1/user/detail`、已验证的正数账号 UID 和 Android App API filter 读取认证账号；不再调用已失效的 `/v1/user/me`。
- `SearchAIModeOnly` 按规范化后的 `Artwork.AIType == 2` 对当前返回批次做本地筛选；该 mode 会进入 cursor 绑定，因此不能把另一种 AI mode 的续页 cursor 复用过来。
- ranking cursor 会绑定所选 `mode`（artwork ranking 还绑定 `date`）。`NovelRanking` 固定发送 App API filter `for_android`，首页不发送 `offset`，续页只使用上游返回的正数 `offset`。
- `RecommendedArtworks` 与 `RecommendedNovels` 保留“未提供 continuation”和显式 `offset=0` 的区别；只有续读 cursor 时才发送后者。recommended artwork subtype 不属于当前 public SDK request，不能根据调用方本地 filter 推断或补发。
- `RelatedUsers`、`UserFollowing`、`UserFollowers` 与 `UserBlockedUsers` 都是 identity-scoped cursor operation。存在已验证账号时，cursor 绑定该账号；否则只允许同一 client instance 继续使用。`UserFollowing` 与 `UserFollowers` 会在 transport 和 cursor 绑定前都把空 `restrict` 归一为 `public`。
- `FollowingArtworks` 与 `FollowingNovels` 会把空 `restrict` 归一为 `public`，在 transport 前拒绝其他值，并把解析后的值绑定到 cursor query。
- `LatestArtworks` 将空 content type 解析为 `illust`，并接受已冻结的 `manga` subtype；解析后的 subtype 会进入 cursor binding。该操作拒绝 `all`、`illust-and-ugoira` 与 `ugoira`，其兼容的 `offset` 与 `max_illust_id` cursor 形式都必须携带正数。
- `LatestNovels` 始终发送固定的 App API filter `for_android`。其 cursor 携带上游正数 `max_novel_id`，SDK 不 fallback 到 offset continuation；续读时应重复原始 request 字段。
- 只有上游明确提供时才填充评论总数和访问控制 metadata。成功的空列表使用非 nil 的空 `Items` slice 表示，不伪造错误或总数。
- `Stamps` 按认证态 `/v1/stamps` read contract 请求，不发送 query 或 continuation。每个 `Stamp` 只公开正数稳定 ID 与 `ImageResource`；当前 contract 不宣称尺寸或其他未冻结的 wire 字段。stamp 图片使用与其他 Pixiv 媒体相同的 opaque resource 边界，新 client 打开时会重新从 `/v1/stamps` 解析 locator。
- `PostArtworkComment`/`ReplyArtworkComment` 与
  `PostNovelComment`/`ReplyNovelComment` 使用各自 namespace 的 comment add
  endpoint；只有上游响应包含正数 `comment_id` 时，才返回
  `CommentMutationResult.CommentID`。该 ID 不是读回确认；SDK 不猜测最新评论，
  不执行 read-back，也不自动重放不确定的 mutation。
- `DeleteArtworkComment` 与 `DeleteNovelComment` 使用各自 namespace 的
  delete endpoint，转发调用方提供的正数 `comment_id`。SDK 只负责本地形状校验
  与上游结果分类；归属、namespace 证明、read-back 与清理由 application 负责。
- `StampArtworkComment` 与 `StampNovelComment` 使用对应 namespace 的
  comment add endpoint，把 `stamp_id` 作为与操作 `comment` 文本并列的独立字段。
  不会把 stamp 编码为 reply parent，也不会静默退化为 text/reply 语义；返回 ID
  同样遵循直接响应、无 read-back 规则。
- `ArtworkBookmark` 与 `NovelBookmark` 用空 `Restrict` 与空 tags 表示当前对象未收藏。`NovelBookmark` 与
  `UserNovelBookmarkTags` 当前遵循 candidate upstream read contract：小说收藏 tags 暂无续页，非零 cursor
  会被拒绝，直到该 contract 完成验证。小说收藏 mutation 仍按 strict/live evidence 要求保持不导出。
- `AddArtworkBookmark` 与 `RemoveArtworkBookmark` 是显式 artwork mutation method。旧的 `AddBookmark` 与
  `RemoveBookmark` wrapper 保留原签名和 error-operation label，并委托同一套校验与 wire 语义。add 的空
  `Restrict` 默认 `public`，不支持的值在本地拒绝。
- `BookmarkMin` 与 `BookmarkMax` 是可选、闭区间、非负的 App API 候选边界。public SDK 只负责校验并转发为 `bookmark_num_min`/`bookmark_num_max`，不做 Premium 前置探测，不宣称全局完备，也不静默切换候选策略。application 若做本地精确复核，应另行报告已解析的策略与结果完备性。

## 错误

所有失败都是带稳定 `Reason` 的 `*sdk.Error`：

```text
invalid_argument, invalid_cursor, unauthorized, credentials_expired, forbidden,
not_found, content_unavailable, challenge_required, rate_limited, upstream_error,
upstream_unavailable, malformed_upstream_response, resource_forbidden,
local_state_error, removed_setting
```

支持 `errors.Is`/`errors.As`，并保留 `context.Canceled`/`DeadlineExceeded`。
错误链不包含 URL、header、token、Cookie 或配置内容。

```go
if errors.Is(err, sdk.Unauthorized{}) {
    // 重新认证
} else if sdk.ReasonOf(err) == sdk.RateLimited {
    // 用 RetryAdvice 退避
}
```

## 资源

程序化 SDK 调用方通过 `sdk.Resource` 获取第一方媒体；它有两条 runtime 路径：

- `Resource.URL` + `Resource.RequestHeaders` — 直接流式读取或无落盘反代。
- `Resource.Ref` — 交回 `OpenResource`/`SaveResource` 做 SDK 校验读取（scheme/host/path 复验与 redirect 安全）。

```go
// 直接流式读取，不落盘。
page, _ := client.ArtworkPages(ctx, pixiv.ArtworkPagesRequest{ArtworkID: id})
image := page[0].Image.Resource
resp, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: image.Ref})
if err != nil { /* 处理 */ }
defer resp.Body.Close()
// 按需用 image.URL + image.RequestHeaders 从 resp.Body 读取
```

```go
// 通过 SDK 校验路径保存（复验 URL/redirect，原子写入）。
_, err := client.SaveResource(ctx, sdk.SaveOptions{
    Ref:  image.Ref,
    Dest: "./out.png",
})
```

> [!IMPORTANT]
> `Resource` 本身不保存 Cookie；绑定的 FANBOX client 只可按策略把 session 发送给 FANBOX API 与 `downloads.fanbox.cc`，不会发送给 Pixiv/CDN 或第三方 host。`Resource` 不携带 token 或 Cookie；`RequiresCredentials` 表示资源仍需要调用方不可见的产品凭据。

### Runtime model 与输出 DTO

运行时 product model 与 CLI/MCP JSON 边界的值是有意分离的。`sdk.Resource` 在进程内 streaming 操作中可以带当前可用的 `URL`、转发所需的 `RequestHeaders` 和 `ExpiresAt`；这些字段绝不进入输出 DTO。

序列化结果时使用显式的逐字段转换器：Pixiv 使用 `pixiv.ToArtworkDTO`、`pixiv.ToNovelDTO`、`pixiv.ToUserDTO`、`pixiv.ToUserDetailDTO`、`pixiv.ToUserPreviewDTO`、`pixiv.ToCommentDTO`、`pixiv.ToStampDTO`、`pixiv.ToNovelContentDTO`、`pixiv.ToUgoiraMetadataDTO` 及其相关转换器；FANBOX 使用对应的 `fanbox.To*DTO` 转换 creator、post、block、asset、user 与 tag。`sdk.ToResourceDTO` 只输出 opaque `ref` 与可选的 `requires_credentials` metadata。CLI/MCP 只编码这些 DTO、管道 `Record` 与 typed envelope，不反射遍历或直接 JSON 编码运行时 product model。

Pixiv 的 `Resource.Ref` 只包含资源 kind、稳定 ID、page 和可选 variant，绝不嵌入当前或签名媒体 URL。SDK 会优先复用当前 Client 保存的 locator，或重新读取对应 artwork、novel、user、ugoira、小说正文或 stamp metadata 后再打开；解析出的 URL 与每次 redirect 都会再次通过 allowlist 校验。`SaveResource` 通过原子目标写入；上游提供 `Content-Length` 时，`SaveProgress.Total` 会报告该值。资源请求只使用显式允许的 header，绝不发送调用方 Cookie jar。

FANBOX 的 `Resource.Ref` 只包含稳定 identity（资源 kind、所属 creator 或 post，以及 attachment id），绝不嵌入当前可用或签名媒体 URL，因此 locator 轮换不会改变缓存键，存储的 ref 可跨 session 重新打开。`OpenResource` 与 `SaveResource` 优先复用 session 内 locator，否则通过重新拉取所属 creator 或 post 并按稳定 id 定位附件来重新解析出新鲜且经 allowlist 校验的 locator。session cookie 只发送给需要凭据的 `downloads.fanbox.cc` host，绝不发送给公开 CDN 或第三方 host；`RequiresCredentials` 表示该 locator 仍需要 session。

## URL 引用

`pixiv.ParseURL` 与 `fanbox.ResolveURL` 在无网络的情况下把页面 URL 转为类型化引用，`Reference.CanonicalURL` 返回无 tracking 的规范形式。

```go
ref, err := pixiv.ParseURL(pageURL)
if err != nil { /* 处理 */ }
canonical := ref.CanonicalURL()
```

### 可选 DTO 字段

输出 DTO 对上游响应未提供的字段采用**省略**而不是发 `null` 或空值：例如 `ArtworkDTO` 在 SDK 没有更新时间、没有工具列表或没有页面列表时省略 `updated_at`、`tools` 与 `pages`（pages 只在 detail 路径填充）。调用方应把缺失的 key 视为未知值；MCP tool 发布的 JSON schema 相应把这些字段标为可选。

## FANBOX

`sdk/fanbox` 提供 creator 资料、帖子、标签、home 与 supporting 流、URL 解析与共享资源契约。已验证的 native route 使用 `api.fanbox.cc` root 下的 `post.info`、`post.listHome`、`post.listSupporting`、`post.listTagged` 与 `tag.getFeatured`；creator 分页跟随服务端返回的 `pageUrls`。帖子正文是结构化 block；图片和文件 block 会与资源索引关联，即使上游只通过 `imageMap` 或 `fileMap` 提供附件也会暴露可用资源。第三方 embed 只保留 canonical link。受限帖只带摘要、Body 为 nil。

Home、Supporting 与 Creators 流属于 identity-scoped 操作：其续页 cursor 会绑定到已验证的 FANBOX 账号 ID（每个 client 经 session identity 解析一次的非敏感值），因此在一个账号下生成的 cursor 不能被重放到另一个账号的流上，会返回 `InvalidCursor`。CreatorPosts 与 TaggedPosts 是公开作用域，不携带账号绑定。与 Pixiv 一致，cursor 同样绑定 product、operation、binding version 与查询摘要。

## 从 v0 迁移

见[迁移指南](../en/v1.0.0-migration.md)了解 v0 `pixiv` 到 v1 `sdk/pixiv` 的切换。
