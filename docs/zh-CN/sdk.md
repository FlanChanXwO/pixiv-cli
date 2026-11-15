# Go SDK

[English](../en/sdk.md) | 简体中文 | [文档索引](../index-zh-CN.md)

模块路径为 `github.com/FlanChanXwO/pixiv-cli`。SDK 提供绑定账号的 API 对象、可恢复的列表与媒体操作。应用传入凭据和连接设置，负责账号选择、配置、输出与凭据持久化。

| import 后缀 | 入口与用途 |
| --- | --- |
| `sdk` | `Page[T]`、不透明的 `Cursor`、分类错误 `Error` 与 `Reason` |
| `sdk/pixiv` | `New(store, options)` 调用已认证的 Pixiv App API；`BeginLogin(options)` 发起 PKCE 登录 |
| `sdk/fanbox` | `New(sessionID, options)` 调用只读 FANBOX API |
| `sdk/saucenao` | `Search(ctx, options)` 从图片字节查询 |
| `sdk/ascii2d` | `Search(ctx, options)` 从图片字节进行色合/BOVW 查询 |
| `sdk/download` | `New(options).Save(ctx, pages...)` 保存选定的 Pixiv 作品页 |

## 快速开始

下面的函数搜索一个上游批次并保存其中的作品页。通过应用自己的私密输入渠道提供 App OAuth refresh token。内存存储仅在当前进程中保留轮换后的 token；长期登录使用持久化的 `CredentialStore`。

```go
package example

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk/download"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func SaveSearch(ctx context.Context, refreshToken, directory string) (download.Report, error) {
	store, err := pixiv.NewMemoryCredentials(pixiv.NewCredentials(refreshToken))
	if err != nil {
		return download.Report{}, err
	}
	client, err := pixiv.New(store, pixiv.Options{})
	if err != nil {
		return download.Report{}, err
	}
	defer client.CloseIdleConnections()
	page, err := client.SearchArtworks(ctx, "landscape", pixiv.ArtworkSearchOptions{})
	if err != nil {
		return download.Report{}, err
	}
	var pages []pixiv.ArtworkPage
	for _, artwork := range page.Items {
		pages = append(pages, artwork.Pages...)
	}
	return download.New(download.Options{DownloadPath: directory}).Save(ctx, pages...)
}
```

`pixiv.New` 构造时无网络与存储 I/O。首次操作加载凭据，必要时自动刷新。`Client.Credentials(ctx)` 可单独准备认证，省去内容请求。App refresh token 校验会拒绝 `PHPSESSID`、`refresh_token=...` 等浏览器 Cookie。

## 引用与对象

`client.Artwork(id)`、`User(id)`、`Novel(id)`、`ArtworkSeries(id)` 和 `NovelSeries(id)` 离线构造绑定引用。作品、用户与小说引用通过 `Get(ctx)` 加载元数据。列表返回相同的绑定对象，可直接调用操作。例如作品嵌入 `*ArtworkRef`，通过 `Author *User` 关联作者，`Pages []ArtworkPage` 中每页通过 `Artwork` 回指作品，并携带原图 `Image`。

| 起点 | 常用操作 |
| --- | --- |
| `ArtworkRef` / `Artwork` | `Get`、`Comments`、`PostComment`、`Bookmark`、`AddBookmark`、`RemoveBookmark`、`UgoiraMetadata` |
| `UserRef` / `User` | `Get`、`Profile`、`Artworks`、`Novels`、`ArtworkBookmarks`、`NovelBookmarks`、关注/粉丝列表与关注修改 |
| `NovelRef` / `Novel` | `Get`、`Comments`、`PostComment`、收藏操作、`Content`、`ParseContent` |
| `CommentRef` / `Comment` | `Reply`、`Delete`；回复目标来自所属作品/小说或评论列表 |
| `ArtworkSeriesRef` | `Artworks(ctx, options)` 列出完整作品 |
| `NovelSeriesRef` | `Get(ctx, options)` 同时返回元数据与 `Novels` 页 |
| `Client` | 作品/小说/用户搜索、排行榜、推荐、关注/最新/MyPixiv 动态、`Stamps` |

列表中的 `User` 可能只含摘要，`Profile(ctx)` 显式读取详细资料。`PostComment(ctx, CommentOptions{Text: "..."})` 返回带上游分配 ID 的引用；`StampID` 支持纯表情评论。修改请求发送一次，省去读回请求。注入的 transport 也应保持此语义，避免重试 POST。

`NovelRef.Content(ctx)` 按 App API 契约返回 `ContentUnavailable`。应用已有小说 webview 文档时，可用 `ParseContent(rawHTML)` 本地解析，媒体仍绑定当前 client。

JSON 保留元数据，移除 client 绑定及页到作品的回指。解码后若需继续操作，通过 client 重新取得引用，例如 `client.Artwork(decoded.ID).Get(ctx)`。`client.ParseURL(raw)` 离线将支持的 Pixiv 页面 URL 解析为绑定引用，`CanonicalURL()` 返回规范页面地址。FANBOX 提供自己的 `Client.ParseURL`。

## 认证

为一个账号实现 `pixiv.CredentialStore`：

- `Load(ctx)` 返回最新 `Credentials`，包含正数 `Revision`、已知时的 `UserID` 与 token。可以省略 access token 状态，client 会按 revision 缓存它。
- `CompareAndSwap(ctx, expectedRevision, replacement)` 仅在 revision 匹配时原子保存。SDK 传入的 `replacement.Revision == expectedRevision+1`。冲突返回 `false, nil`，持久化完成后返回 `true, nil`，保留已建立的账号身份。
- 两个方法返回前释放存储锁，OAuth 在两次调用之间执行。冲突后 SDK 采用较新的已保存凭据；持久化失败在使用轮换后的 token 请求内容前传播。

`pixiv.NewCredentials(refreshToken)` 构造 revision 为 1 的初始凭据，`NewMemoryCredentials` 校验后提供并发安全的内存存储。持久化实现通过 `Credentials.SetTokens` 恢复原始 token。token 字段在 fmt/slog 中脱敏，JSON 省略这些字段；仅在私密持久化或显式凭据传输时读取 `Value()`。

### 以编程方式发起浏览器登录

应用自行打开授权 URL，通过自己的 UI 取得回调 URL 或 code。`Complete` 返回的凭据应先保存，再用于内容请求。

```go
package example

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func Login(ctx context.Context, receive func(string) (string, error)) (pixiv.Credentials, error) {
	session, err := pixiv.BeginLogin(pixiv.Options{})
	if err != nil {
		return pixiv.Credentials{}, err
	}
	defer session.CloseIdleConnections()
	callback, err := receive(session.AuthorizationURL())
	if err != nil {
		return pixiv.Credentials{}, err
	}
	return session.Complete(ctx, callback)
}
```

`AcceptsCallbackURL(raw)` 校验回调，保留会话可用性。有效的完成输入在网络 I/O 前消耗会话，交换失败或取消也会消耗；重试需发起新登录。输入无效时保留会话。

## 连接与日志

会发网络请求的操作均以 `ctx` 为首参数，取消与截止时间通过它传递。构造函数、引用构造、URL 解析和本地 HTML 解析均为离线操作。

Pixiv `Options` 接受 `HTTPClient`、`AcceptLanguage`、`RequestInterval` 与额外的 `ResourceHosts`。HTTP client 为 nil 时创建专用 client，整请求超时由调用方 context 控制；`CloseIdleConnections()` 释放其空闲连接。注入的 client 由调用方拥有，包括代理、transport 与关闭策略。

FANBOX 与 ascii2d 接受显式代理、User-Agent 和可选 FlareSolverr 设置。默认原生 transport 使用浏览器 TLS profile；注入的 `HTTPClient` 需显式提供 `Transport`，Cookie 与重定向策略由产品管理。FANBOX `Close(ctx)` 取消并等待 solver 工作，关闭前先完成原生请求。FlareSolverr 控制请求直连，其浏览器代理单独由 `FlareSolverrProxyURL` 设置，仅挑战恢复使用 solver。

所有 SDK 包使用事件发生时的 `slog` 默认 logger。在应用启动时统一配置，例如导入 `log/slog` 与 `os` 后：

```go
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelDebug,
})))
```

事件记录受控的请求与决策属性，常规节流和重试使用 Debug，限流等待可能使用 Warn。SDK 错误保留 cause 供检查；记录任意错误链前应用自行执行隐私策略。CLI/MCP 在启动时安装 stderr handler，清理后恢复原默认 logger。

## 分页

列表返回 `sdk.Page[T]`，带有 `Items`、`Cursor` 与 `Next(ctx)`。当前页保留账号和查询绑定。空游标表示末批，随后调用 `Next` 会离线返回空页。完整消费一批后保存游标：

```go
package example

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
	"github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func Search(ctx context.Context, client *pixiv.Client, word, saved string,
	consume func([]*pixiv.Artwork) error, checkpoint func(string) error) error {
	cursor, err := sdk.ParseCursor(saved)
	if err != nil {
		return err
	}
	page, err := client.SearchArtworks(ctx, word, pixiv.ArtworkSearchOptions{Cursor: cursor})
	if err != nil {
		return err
	}
	for {
		if err := consume(page.Items); err != nil {
			return err
		}
		if err := checkpoint(page.Cursor.String()); err != nil {
			return err
		}
		if page.Cursor.IsZero() {
			return nil
		}
		page, err = page.Next(ctx)
		if err != nil {
			return err
		}
	}
}
```

示例中的空 `saved` 开始新搜索。另行保存“已经完成”的状态：空的已保存游标也表示列表已耗尽，再次调用 `Search` 前先检查该状态。恢复时使用原查询的全部选项与同一账号。条件不匹配时在内容请求前返回 `InvalidArgument`，认证准备仍可能发生。游标续读实时结果，结果集会随上游变化。

JSON 解码后的页保留数据与游标，移除 `Next` 绑定。非末页通过 `sdk.ParseCursor(saved)` 恢复游标，将其放回原列表操作的 `Cursor` 选项。游标 JSON/text 均为不透明字符串，原样保存。

### 作品搜索批内续读

调用 `client.CheckpointSearchArtworks(ctx, word, options, consumed)` 时，传入取得当前批次时的选项，包括**进入该批次的** `Cursor`。`consumed` 统计 SDK AI 筛选之后、应用自行筛选之前的条目。保存返回游标，使用相同条件经 `SearchArtworks` 恢复。创建 checkpoint 会准备凭据，可能刷新 OAuth；恢复时重新读取实时批次并跳过已消费前缀。

## 资源与下载

Pixiv `Image` 提供 `URL`、`Width`、`Height`、`Quality(...)`、`Open(ctx)`、`OpenResponse(ctx)` 与 `Save(ctx, path)`。`client.Image(rawURL)` 绑定允许的 CDN URL。`Open` 要求 HTTP 200，返回的 `io.ReadCloser` 由调用方关闭。需要自行解释 HTTP 状态时，`OpenResponse` 提供状态、内容类型/长度与 body。`Save` 完整读取成功后原子替换目标，父目录需事先存在。

`sdk/download` 按输入顺序保存传入的 `pixiv.ArtworkPage`。检查页码范围后，对 `artwork.Pages` 切片选页。选项控制根目录、文件名/目录模板、静态图 `Quality` 与 GIF/APNG `UgoiraFormat`。模板字段为 `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}` 和从零开始的 `{num}`。Ugoira 表示为一页，要求 original 质量。

`Save` 同时返回 `Report` 与错误：`Files` 保留已发布文件，`Warnings` 保留诊断，`Failures` 保留逐页 cause。单页失败后继续后续页，取消时停止批次并保留部分结果。错误非 nil 时仍检查报告。FANBOX 媒体通过自己的 `Image.Save` / `File.Save` 保存。

## FANBOX

向 `fanbox.New` 传入纯 `FANBOXSESSID` 值。构造为离线操作，`CurrentUser(ctx)` 或 `ValidateSession(ctx)` 显式验证会话。会话获取与保存由调用方负责。

```go
package example

import (
	"context"
	"errors"

	"github.com/FlanChanXwO/pixiv-cli/sdk/fanbox"
)

func ReadPost(ctx context.Context, sessionID, postID string, consume func(*fanbox.Post) error) (err error) {
	client, err := fanbox.New(sessionID, fanbox.Options{})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, client.Close(context.WithoutCancel(ctx))) }()
	post, err := client.Post(postID).Get(ctx)
	if err != nil {
		return err
	}
	return consume(post)
}
```

`client.Creator(id).Get(ctx)` 加载创作者；`CreatorRef.Posts(ctx, PostsRequest{...})`、`client.Home` 和 `client.Supporting` 返回绑定账号与查询的页。上游隐藏正文时 `Post.Body` 为 nil。可用正文包含文本、有序 block、图片、附件及完整上游 `Payload`，后者保留未知字段与签名媒体地址。输出时自行选择要公开的字段。

绑定的 `Image` / `File` 提供 `Open(ctx, OpenRequest{})` 与原子 `Save(ctx, path)`。`OpenRequest` 支持 GET/HEAD 及 `Range`、`IfNoneMatch`、`IfModifiedSince`、`IfRange`。调用方检查 `MediaResponse.StatusCode`、表示头和 `Filename` 并关闭 `Body`，HEAD/304 的 body 为空。选择本地路径时将上游文件名视为未信任输入。`Save` 要求 HTTP 200 并创建父目录。会话 Cookie 仅发送到经过校验的 FANBOX API/首页身份请求和 `downloads.fanbox.cc`，CDN 读取省略凭据。

## 以图搜图

自行加载来源，将图片字节传给独立 provider 包。查询所需凭据由各 provider 的选项提供。

```go
package example

import (
	"context"

	"github.com/FlanChanXwO/pixiv-cli/sdk/ascii2d"
	"github.com/FlanChanXwO/pixiv-cli/sdk/saucenao"
)

func Sauce(ctx context.Context, image []byte, apiKey string) (saucenao.Result, error) {
	return saucenao.Search(ctx, saucenao.Options{Image: image, APIKey: apiKey})
}

func Ascii(ctx context.Context, image []byte) (ascii2d.Result, error) {
	return ascii2d.Search(ctx, ascii2d.Options{Image: image, Mode: ascii2d.Both})
}
```

SauceNAO 返回有序 `Matches` 与可选 `Quota`。ascii2d 接受 JPEG/PNG/WEBP，大小上限为 `ascii2d.MaxImageBytes`（10 MiB），默认模式为 `Color`。`Both` 上传一次，分别保留 `Color`/`BOVW` 页结果。每个请求页带 `Matches` 与 `Err`，`Search` 同时返回合并错误；处理错误时保留可用的部分结果。文件/URL 加载、合并 provider 结果与补全 Pixiv 信息由调用方负责。

## 错误与迁移

使用 `sdk.ReasonOf(err)`、`errors.As` 或 `errors.Is(err, &sdk.Error{Reason: sdk.NotFound})`。`sdk.Error` 提供 `Reason`、`Operation`、受控 `Message`、`HTTPStatus`、可选 `RetryAfter` 与可解包的 `Err`。取消与持久化 cause 保留在错误链中。

```go
package example

import (
	"context"
	"errors"
	"fmt"

	"github.com/FlanChanXwO/pixiv-cli/sdk"
)

func Explain(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var failure *sdk.Error
	if errors.As(err, &failure) {
		return fmt.Sprintf("%s: %s (HTTP %d)", failure.Operation, failure.Reason, failure.HTTPStatus)
	}
	return fmt.Sprint(err)
}
```

| Reason | 含义 |
| --- | --- |
| `InvalidArgument` | 参数无效、游标不匹配或对象缺少绑定 |
| `Unauthorized`、`CredentialsExpired` | 认证被拒绝，或凭据需要续期/重新登录 |
| `Forbidden`、`NotFound`、`ContentUnavailable` | 访问限制、对象缺失或内容不可用 |
| `ChallengeRequired`、`RateLimited` | 浏览器挑战未解决或上游限流 |
| `UpstreamError`、`UpstreamUnavailable`、`MalformedUpstreamResponse` | 上游失败、传输/可用性问题或无效响应 |
| `ResourceForbidden`、`LocalStateError` | 资源来源被拒绝，或凭据/存储/本地状态失败 |

`RetryAfter` 仅在上游提供有效值时出现，后续调度策略由调用方决定。

从旧 SDK 迁移时，将 `Open`/`OpenWith`/`NewWith` 换为 `pixiv.New(store, options)` 或 `fanbox.New(sessionID, options)`；实体操作迁到绑定引用/对象，网络调用显式传入 `ctx`。构造时传 logger 改为 `slog.SetDefault`，错误与通用资源包装改用 `sdk.Error` 和产品媒体对象。provider 查询移到 `sdk/saucenao` / `sdk/ascii2d`，批量保存移到 `sdk/download`。模块与 import 根路径保持原样。内部归属见[架构说明](maintainers/architecture.md)，CLI 账号管理见[认证](authentication.md)。
