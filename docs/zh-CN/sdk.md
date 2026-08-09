# Pixiv SDK (v1)

[English](../en/sdk.md) | 简体中文 | [日本語](../ja/sdk.md) | [文档索引](../index.md)

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

空 `UserAgent` 使用内置 Firefox 148 baseline；自定义值不会改变 TLS profile，也不保证能绕过 Cloudflare。
`FlareSolverr` 为 nil 时完全关闭，只有 native 请求被严格识别为 Cloudflare challenge 后才会调用。
solver service URL 与 upstream proxy 独立于 native proxy；public constructor 不联网。

## 分页

列表操作返回 `sdk.Page[T]` 与不透明 `Cursor`。游标绑定 product、operation、
binding version 与查询摘要；用不同查询复用游标会返回 `InvalidCursor`。

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

第一方媒体通过 `sdk.Resource` 以两条并行路径暴露：

- `Resource.URL` + `Resource.RequestHeaders` — 直接流式读取或无落盘反代。
- `Resource.Ref` — 交回 `OpenResource`/`SaveResource` 做 SDK 校验读取
  （scheme/host/path 复验与 redirect 安全）。`Resource` 本身不保存 Cookie；绑定的 FANBOX
  client 只可按策略把 session 发送给 FANBOX API 与 `downloads.fanbox.cc`，不会发送给 Pixiv/CDN
  或第三方 host。

`Resource` 不携带 token 或 Cookie；`RequiresCredentials` 表示资源仍需要调用方
不可见的产品凭据。

## URL 引用

`pixiv.ParseURL` 与 `fanbox.ResolveURL` 在无网络的情况下把页面 URL 转为类型化
引用，`Reference.CanonicalURL` 返回无 tracking 的规范形式。

## FANBOX

`sdk/fanbox` 提供 creator 资料、帖子、标签、home 与 supporting 流、URL 解析与
共享资源契约。已验证的 native route 使用 `api.fanbox.cc` root 下的 `post.info`、
`post.listHome`、`post.listSupporting`、`post.listTagged` 与 `tag.getFeatured`；creator
分页跟随服务端返回的 `pageUrls`。帖子正文是结构化 block；图片和文件 block 会与资源索引
关联，即使上游只通过 `imageMap` 或 `fileMap` 提供附件也会暴露可用资源。第三方 embed 只保留
canonical link。受限帖只带摘要、Body 为 nil。

## 从 v0 迁移

见[迁移指南](../en/v1.0.0-migration.md)了解 v0 `pixiv` 到 v1 `sdk/pixiv` 的切换。
