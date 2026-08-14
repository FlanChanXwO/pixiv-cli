# Pixiv SDK (v1)

[English](../en/sdk.md) | 简体中文 | [文档索引](../index.md)

v1 SDK 暴露三个公开包：

- `github.com/FlanChanXwO/pixiv-cli/sdk` — 两个产品共享的协议无关原语：分页、
  不透明游标、分类错误与资源契约。
- `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv` — Pixiv App API 客户端、模型、
  URL 引用与 mutation。
- `github.com/FlanChanXwO/pixiv-cli/sdk/fanbox` — Pixiv FANBOX 客户端、模型与
  URL 解析。

所有导出声明均带英文 GoDoc；源码是 API 的权威摘要。

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

Pixiv refresh token 与 FANBOX session 相互独立，永不转换。

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

列表操作返回 `sdk.Page[T]` 与不透明 `Cursor`。游标绑定 product、operation、
binding version 与查询摘要；用不同查询复用游标会返回 `InvalidCursor`。

对 identity-scoped operation，`pixiv.New` 创建的 Client 没有已验证的账号 ID，
因此其续页 cursor 是 ephemeral，并携带只用于绑定该 Client 实例的非敏感标识；
同一 Client 可以继续，其他 Client 或进程会返回 `InvalidCursor`。通过
`pixiv.Open` 创建的 Client 则把 cursor 绑定到已验证的账号 identity。

## Pixiv 读取操作

公开 Pixiv Client 将 artwork、novel、user 搜索分开，同时使用相同的 typed 分页契约：

- `SearchArtworks` 支持关键词、target、排序、时间/日期边界、作品类型、AI、横纵比、
  分辨率、绘图工具和可选收藏数边界。省略 target、sort 时分别默认为
  `partial_match_for_tags`、`date_desc`。未知枚举、非法日期和非法收藏数范围会在发请求前返回
  `InvalidArgument`。
- `SearchNovels` 只接受关键词、target、排序和 duration；`SearchUsers` 只接受关键词。
  作品专用字段不会静默复制到其他实体的请求。
- `SearchAIModeOnly` 按规范化后的 `Artwork.AIType == 2` 对当前返回批次做本地筛选；该 mode
  会进入 cursor 绑定，因此不能把另一种 AI mode 的续页 cursor 复用过来。
- `ArtworkRanking` 省略 mode 时默认为 `day`，并校验排行 mode 和可选的 `YYYY-MM-DD` 日期。
  `Artwork`、`Novel`、`User` 是使用正数 typed ID 的详情操作。`ArtworkSeries` 与 `NovelSeries`
  保留各自的系列分页续接；后者同时返回系列 metadata。
- `ArtworkComments` 与 `NovelComments` 返回 `CommentPage`。只有上游明确提供时才填充评论总数和
  访问控制 metadata。成功的空列表使用非 nil 的空 `Items` slice 表示，不伪造错误或总数。
- `UserArtworkBookmarks`、`UserArtworkBookmarkTags` 与 `UserNovelBookmarks` 会在请求 App API
  前校验 `UserID`、`Restrict` 和 opaque continuation；`tag` 与服务端提供的收藏续页值按原语义传递。
  成功的空页仍是空 `Items`。`ArtworkBookmark` 用空 `Restrict` 与空 tags 表示当前作品未收藏；
  `AddBookmark` 校验可见性值，不把未知值静默交给服务端默认处理。
- `BookmarkMin` 与 `BookmarkMax` 是可选、闭区间、非负的 App API 候选边界。public SDK 只负责
  校验并转发为 `bookmark_num_min`/`bookmark_num_max`，不做 Premium 前置探测，不宣称全局完备，
  也不静默切换候选策略。application 若做本地精确复核，应另行报告已解析的策略与结果完备性。

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

## 资源

程序化 SDK 调用方通过 `sdk.Resource` 获取第一方媒体；它有两条 runtime 路径：

- `Resource.URL` + `Resource.RequestHeaders` — 直接流式读取或无落盘反代。
- `Resource.Ref` — 交回 `OpenResource`/`SaveResource` 做 SDK 校验读取
  （scheme/host/path 复验与 redirect 安全）。`Resource` 本身不保存 Cookie；绑定的 FANBOX
  client 只可按策略把 session 发送给 FANBOX API 与 `downloads.fanbox.cc`，不会发送给 Pixiv/CDN
  或第三方 host。

`Resource` 不携带 token 或 Cookie；`RequiresCredentials` 表示资源仍需要调用方
不可见的产品凭据。

Pixiv 的 `Resource.Ref` 只包含资源 kind、稳定 ID、page 和可选 variant，绝不嵌入当前或签名媒体 URL。
SDK 会优先复用当前 Client 保存的 locator，或重新读取对应 artwork、novel、user、ugoira 或小说正文
metadata 后再打开；解析出的 URL 与每次 redirect 都会再次通过 allowlist 校验。`SaveResource` 通过
原子目标写入；上游提供 `Content-Length` 时，`SaveProgress.Total` 会报告该值。资源请求只使用显式
允许的 header，绝不发送调用方 Cookie jar。

### Runtime model 与输出 DTO

运行时 product model 与 CLI/MCP JSON 边界的值是有意分离的。`sdk.Resource` 在进程内
streaming 操作中可以带当前可用的 `URL`、转发所需的 `RequestHeaders` 和 `ExpiresAt`；
这些字段绝不进入输出 DTO。

序列化结果时使用显式的逐字段转换器：Pixiv 使用
`pixiv.ToArtworkDTO`、`pixiv.ToNovelDTO`、`pixiv.ToUserDTO`、
`pixiv.ToUserDetailDTO`、`pixiv.ToUserPreviewDTO`、`pixiv.ToCommentDTO`、
`pixiv.ToNovelContentDTO`、`pixiv.ToUgoiraMetadataDTO` 及其相关转换器；FANBOX
使用对应的 `fanbox.To*DTO` 转换 creator、post、block、asset、user 与 tag。
`sdk.ToResourceDTO` 只输出 opaque `ref` 与可选的 `requires_credentials` metadata。
CLI/MCP 只编码这些 DTO、管道 `Record` 与 typed envelope，不反射遍历或直接 JSON 编码
运行时 product model。

## URL 引用

`pixiv.ParseURL` 与 `fanbox.ResolveURL` 在无网络的情况下把页面 URL 转为类型化
引用，`Reference.CanonicalURL` 返回无 tracking 的规范形式。

### 可选 DTO 字段

输出 DTO 对上游响应未提供的字段采用**省略**而不是发 `null` 或空值：例如
`ArtworkDTO` 在 SDK 没有更新时间、没有工具列表或没有页面列表时省略
`updated_at`、`tools` 与 `pages`（pages 只在 detail 路径填充）。调用方应把
缺失的 key 视为未知值；MCP tool 发布的 JSON schema 相应把这些字段标为可选。

## FANBOX

`sdk/fanbox` 提供 creator 资料、帖子、标签、home 与 supporting 流、URL 解析与
共享资源契约。已验证的 native route 使用 `api.fanbox.cc` root 下的 `post.info`、
`post.listHome`、`post.listSupporting`、`post.listTagged` 与 `tag.getFeatured`；creator
分页跟随服务端返回的 `pageUrls`。帖子正文是结构化 block；图片和文件 block 会与资源索引
关联，即使上游只通过 `imageMap` 或 `fileMap` 提供附件也会暴露可用资源。第三方 embed 只保留
canonical link。受限帖只带摘要、Body 为 nil。

## 从 v0 迁移

见[迁移指南](../en/v1.0.0-migration.md)了解 v0 `pixiv` 到 v1 `sdk/pixiv` 的切换。
