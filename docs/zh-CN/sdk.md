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
返回 `CodeCredentialsExpired`。不存在匿名或 Web fallback。

FANBOX 使用显式 `FANBOXSESSID` 值认证：

```go
client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: session})
```

Pixiv refresh token 与 FANBOX session 相互独立，永不转换。

## 分页

列表操作返回 `sdk.Page[T]` 与不透明 `Cursor`。游标绑定 product、operation、
binding version 与查询摘要；用不同查询复用游标会返回 `CodeInvalidCursor`。

## 错误

所有失败都是带稳定 `Code` 的 `*sdk.Error`：

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
  （scheme/host/path 复验、无 Cookie、redirect 安全）。

`Resource` 不携带 token 或 Cookie；`RequiresCredentials` 表示资源仍需要调用方
不可见的产品凭据。

## URL 引用

`pixiv.ParseURL` 与 `fanbox.ResolveURL` 在无网络的情况下把页面 URL 转为类型化
引用，`Reference.CanonicalURL` 返回无 tracking 的规范形式。

## FANBOX

`sdk/fanbox` 提供 creator 资料、帖子、标签、home 与 supporting 流、URL 解析与
共享资源契约。帖子正文是结构化 block；第三方 embed 只保留 canonical link。
受限帖只带摘要、Body 为 nil。

## 从 v0 迁移

见[迁移指南](../en/v1.0.0-migration.md)了解 v0 `pixiv` 到 v1 `sdk/pixiv` 的切换。
