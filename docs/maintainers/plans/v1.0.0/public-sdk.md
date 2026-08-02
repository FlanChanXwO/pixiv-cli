# v1.0.0 公开 SDK 契约

## Package 边界

- `sdk`：跨产品的 `Page[T]`、opaque `Cursor`、安全 `Error`、`ResourceRef` 与 `Resource`。
- `sdk/pixiv`（package `pixiv`）：Pixiv App API、OAuth、Artwork/Novel/User、mutation 与资源读取。
- `sdk/fanbox`（package `fanbox`）：FANBOX session、Creator/Post、入口解析与资源读取。
- 两个产品 package 都只依赖 `sdk` 和允许的 internal adapter；彼此不 import，不共享 credential，
  不自动 fallback。

公开 package 不 import `internal/storage`、`internal/config`、浏览器 provider、Cobra 或 MCP。
协议 adapter 继续留在 `internal/services/<product>`，不得公开。

`sdk` 保持小型、协议无关，不承载产品 operation 或内容模型。源码按职责拆成多个文件；文件名不是
公开兼容契约，可以随内部维护调整。

## Go documentation 语言

`sdk`、`sdk/pixiv` 与 `sdk/fanbox` 是面向外部开发者的公共产品面，其 package comment 和全部导出
declaration 的说明注释统一使用英文。每个导出 function/method 都必须说明用途、关键参数、返回值、
资源 ownership、认证前置条件以及非显然的错误/取消语义；不能只把函数名改写成一句无信息注释。

导出 type、const、var 和需要独立理解的 struct field 同样使用英文 GoDoc，并遵循 Go convention，
以被说明的 identifier 开头。example 的解释文字使用英文；测试数据、搜索词和上游原始枚举不因本规则
翻译。internal package、函数体中的实现注释和测试内部说明仍可使用中文。本规则是项目“注释默认中文”
对公开 SDK 的明确例外。

同一 public symbol 的英文 GoDoc 是源码中的 canonical API summary；`docs/en/sdk.md` 负责完整契约，
`docs/zh-CN/sdk.md` 提供对应中文说明。GoDoc 不塞入迁移历史或长篇协议细节，而应链接稳定文档。

## `sdk.Page[T]` 与 `sdk.Cursor`

```go
type Page[T any] struct {
	Items []T
	Next  Cursor
}
```

`Items` 的空成功结果是非 `nil` 空 slice；`Next.IsZero()` 表示没有下一页。所有可能
增长参数的列表操作使用 request struct，request 含 `Cursor`。调用方续页时重复原始
查询参数；产品 SDK 校验 cursor 的 product、operation、版本与查询摘要，不匹配时返回
`invalid_cursor`。

`Cursor` 不允许 struct literal 构造，提供 `IsZero`、Text/JSON marshal 与 parse。其编码：

- 不包含 token、Cookie、签名 URL、原始搜索文本或本地路径；
- 可在进程间传递，并在同一 major 的后续版本中保持可解码；
- 不保证上游 continuation 永久有效；上游失效必须显式报错；
- 从不因为跨产品、跨 operation 或查询不匹配而静默从第一页重启；
- 对 identity-scoped operation 绑定经过验证的非 secret identity；无法验证 identity 时，
  该 cursor 只允许同一 Client 实例继续使用，并在文档中明确这一点。

## `sdk.Error`

`*sdk.Error` 提供稳定的 `Product`、`Operation`、`Code`、安全 upstream status、
transport kind 与 retry advice。它实现 `errors.Is/As`，并保留 `context.Canceled` 和
`context.DeadlineExceeded` 链。

v1 基础 code 固定为：

```text
invalid_argument
invalid_cursor
unauthorized
credentials_expired
forbidden
not_found
content_unavailable
challenge_required
rate_limited
upstream_error
upstream_unavailable
malformed_upstream_response
resource_forbidden
local_state_error
removed_setting
```

产品差异通过 `Product`、`Operation` 和受控 detail kind 表达，不临时创造同义 code。后续新增
code 只能是 additive change，并必须同步 SDK、CLI/MCP 映射和三语文档。

重试信息使用 `RetryAdvice{Safe, After, HasAfter}`。`Safe` 只表达从操作提交语义看可安全
重试，不代表 SDK 会自动重试；`After` 只来自已验证的上游信息。错误链不得包含原始 URL、
header、response body、token、Cookie、代理 userinfo、浏览器路径或配置内容。

## Pixiv 构造与凭据

```go
func Open(
	ctx context.Context,
	refreshToken string,
) (*Client, Credentials, error)

func OpenWith(
	ctx context.Context,
	refreshToken string,
	options Options,
) (*Client, Credentials, error)

func New(accessToken string) (*Client, error)

func NewWith(accessToken string, options Options) (*Client, error)
```

- `Open` 执行一次 OAuth refresh，返回只持有 access token 的 Client 以及 rotation 后的
  `Credentials`；Client 不自动 refresh。
- 调用方必须在使用 Client 前持久化返回的 rotated credentials。CLI/MCP 保存失败时不得
  发起内容请求。
- access token 过期后返回 `credentials_expired`；调用方重新 `Open`。
- `New` 不联网、不读取文件、不推断 refresh token，也不声称能从 access token 安全恢复 UID。
- `Credentials` 固定包含经过验证的 user ID、username、access token、refresh token 与 expiry；
  secret 字段禁止 JSON marshal，`String`、`GoString` 与格式化路径统一脱敏。
- `Options` 只持有显式 HTTP client、`AcceptLanguage`、request pacing 与 resource policy 等连接级选项。
  不包含账号路径、config、浏览器、Web fallback 或公开 base URL 覆写。测试使用受控
  `RoundTripper` 拦截官方 host，避免把凭据发往调用方指定地址。
- 注入的 HTTP client 归调用方所有；SDK 不关闭它。SDK 自建 transport 时提供
  `CloseIdleConnections`，不设置无依据的整请求固定 timeout。

## Pixiv OAuth login

```go
func BeginLogin(options LoginOptions) (*LoginSession, error)

func (s *LoginSession) AuthorizationURL() string

func (s *LoginSession) Complete(
	ctx context.Context,
	callbackURL string,
) (Credentials, error)
```

`LoginSession` 是 self-contained、one-shot 的 PKCE/state 会话，不绑定 Client 或本地账号库。
复制、并发完成或二次完成都返回稳定错误；格式化永不暴露 verifier、state、code 或 callback。

## Pixiv 模型与方法规则

- 使用 `Artwork` 统一插画、漫画与 ugoira，并以 `ArtworkKind` 区分。
- common fields 与 variant fields 分组；未知 kind 保留原始类型标识，不伪装成插画。
- `Artwork`、`Novel`、`User` 的 list summary 与 detail 完整度在模型注释中固定。
- operation 名必须表达完整领域，例如 `UserArtworkBookmarks`，不使用含糊的 `Bookmarks`。
- 搜索、排行、推荐、时间线、用户作品/收藏/关注、小说、ugoira metadata、mutation 和
  resource operation 都只走 App API。
- 有 access token 时 App API 失败不 fallback；没有 token 时内容 operation 返回
  `unauthorized`。
- 普通列表 operation 直接返回 `sdk.Page[T]`；comments、novel series 等带额外 metadata 的结果
  使用包含 `sdk.Page[T]` 的显式 wrapper。所有分页 operation 使用独立 request type；后续增加筛选
  字段通过 request struct 的 additive field 完成。
- v1 内容 operation inventory 固定如下；实现可以补充 request/model 类型，但不得在未修订设计时
  增加同义入口：

```text
CurrentUser
SearchArtworks
Artwork
ArtworkPages
RelatedArtworks
ArtworkSeries
ArtworkRanking
RecommendedArtworks
FollowingArtworks
LatestArtworks
UserArtworks
UserArtworkBookmarks
UserArtworkBookmarkTags
MyPixivArtworks
TrendingArtworkTags
UgoiraMetadata
ArtworkComments
ArtworkBookmark

SearchNovels
Novel
NovelSeries
NovelContent
NovelComments
RecommendedNovels
FollowingNovels
LatestNovels
UserNovels
UserNovelBookmarks
MyPixivNovels

SearchUsers
User
RecommendedUsers
RelatedUsers
UserFollowing
UserFollowers
UserBlockedUsers
MyPixivUsers

AddBookmark
RemoveBookmark
FollowUser
UnfollowUser
SetAIArtworkVisibility
OpenResource
SaveResource
```

所有 list operation 使用独立 request struct；detail 和 mutation 也优先使用 request struct，避免
将未来可能增长的参数冻结为位置参数。所有导出 symbol 必须有英文 GoDoc，并有 example 或
contract test。

完整能力基线和排除项见 [PixivPy App API 能力兼容矩阵](pixivpy-parity.md)。兼容目标是固定版本
`AppPixivAPI` 的产品能力，而非 Python 方法名、参数布局、返回 DTO 或低层 transport helper。

comments 与 novel series 在普通分页外还有不可丢失的上游 metadata，使用显式 wrapper：

```go
type CommentPage struct {
	Page          sdk.Page[Comment]
	Total         *int64
	AccessControl *CommentAccessControl
}

type NovelSeriesResult struct {
	Series NovelSeries
	Novels sdk.Page[Novel]
}
```

只有上游明确提供 total/access control 时对应指针才非 `nil`，不能把未知值伪造为零值。
`NovelContent` 使用结构化 block/mark 模型完整表达正文，不返回 raw HTML；遇到未知但可保留的 block
保存其类型和安全 payload，无法在不丢失正文的情况下解析时显式失败。`AddBookmarkRequest` 固定支持
visibility/restrict 与 tags；`Options.AcceptLanguage` 只影响语言协商，不改变模型 schema。

## Pixiv URL 引用

公开 package 提供纯本地、强类型的 Pixiv URL 解析，不要求调用方用正则猜测资源类型：

```go
type ReferenceKind string

const (
	ReferenceKindArtwork      ReferenceKind = "artwork"
	ReferenceKindNovel        ReferenceKind = "novel"
	ReferenceKindUser         ReferenceKind = "user"
	ReferenceKindUserBookmarks ReferenceKind = "user_bookmarks"
	ReferenceKindArtworkSeries ReferenceKind = "artwork_series"
	ReferenceKindNovelSeries   ReferenceKind = "novel_series"
)

type Reference struct {
	Kind        ReferenceKind
	ID          int64
	OwnerUserID int64
}

func ParseURL(rawURL string) (Reference, error)
func (r Reference) CanonicalURL() (string, error)
```

`ID` 始终是 `Kind` 所指实体的 ID；`OwnerUserID` 只在 artwork series URL 明确包含作者 ID 时
非零。所有 ID 必须是正整数。`ParseURL` 不联网、不跟随 redirect，只接受 HTTPS、无 userinfo、
无显式 port 的 `pixiv.net` 或 `www.pixiv.net` URL。v1 至少接受以下官方形态及其 locale path
前缀：

```text
/artworks/{artwork_id}
/novel/show.php?id={novel_id}
/users/{user_id}
/users/{user_id}/artworks
/users/{user_id}/bookmarks/artworks
/user/{owner_user_id}/series/{artwork_series_id}
/novel/series/{novel_series_id}
```

parser 只读取识别资源所需的 path/query，绝不把其他 query 或 fragment 保存在 `Reference`；
`CanonicalURL` 输出无 tracking query/fragment 的规范 URL。未知 path、缺失/重复 ID、非十进制 ID、
非正 ID 和 host 混淆一律返回 `invalid_argument`。裸整数因无法区分 artwork、novel、user 或 series，
不得由 `ParseURL` 猜测；具体 operation 的 request 可以继续明确接受对应实体 ID。

新增等价官方 URL 形态属于 additive change，但既有形态的 `Kind`、`ID` 和 canonical output 在同一
major 内不得改变。成功解析只表示识别了引用，不表示当前 inventory 必然有对应读取 operation；
不支持的引用必须显式报错，不能改用其他资源类型。`Reference` 的 JSON/Text 表达若对外提供，也必须
固定版本并拒绝未知或不一致字段。

## 稳定 identity 与时间

- Pixiv 的 artwork、novel、user、artwork series、novel series，以及 FANBOX 的 creator、post、tag
  和原生 asset，均保留上游稳定 identity；两产品的 ID 类型与 namespace 不合并，也不由标题、URL、
  列表位置或抓取时间合成 ID。
- 所有可发布内容统一暴露 `PublishedAt time.Time`；上游提供修改时间时暴露
  `UpdatedAt *time.Time`。当前必须覆盖 Pixiv `Artwork`、`Novel` 与 FANBOX `Post`。
- 时间解析后统一为 UTC，但表示的 instant 不变。必需的发布时间缺失或非法时返回
  `malformed_upstream_response`；未知的可选修改时间使用 `nil`，不得填零值、当前时间、首次观察时间
  或发布时间。
- 同步程序以 `(product, entity kind, ID)` 作为 identity key；时间只用于排序和变更提示。SDK 不承诺
  上游时间单调、唯一或足以实现无遗漏增量同步，cursor 也不等同于长期 sync token。
- list summary 与 detail 对同一实体必须返回相同 ID 和时间语义；上游未提供的 detail-only 字段保持
  明确的缺失状态，不能伪造完整模型。

## FANBOX 构造与凭据

```go
type SessionCredentials struct {
	FANBOXSESSID string `json:"-"`
}

func Open(credentials SessionCredentials) (*Client, error)

func OpenWith(
	credentials SessionCredentials,
	options Options,
) (*Client, error)
```

- 构造器不联网、不读取浏览器或本地文件。
- SDK 只接受明确的 `FANBOXSESSID`，不接受完整 Cookie header。
- `SessionCredentials` 的所有格式化路径脱敏。
- `ValidateSession(ctx)` 显式验证当前身份；session 失效返回 `credentials_expired`。
- Cookie 只发送给精确允许的 FANBOX host；资源请求和 redirect 后续跳转不携带 Cookie。
- 只有精确识别的 challenge 响应返回 `challenge_required`；普通 403 保持 `forbidden` 或
  upstream 分类。

## FANBOX 模型与入口

v1 覆盖 creator profile、following/supporting creator、creator posts、tag posts、single post、
home、supporting、旧 Pixiv FANBOX redirect、分页与详情。

Post 模型覆盖 text、image、file、article、cover、video/embed metadata 和受限摘要。未知
block/embed 保留结构化类型标识；第三方 embed 只返回 provider、content identity、canonical
link 与安全 metadata，不递归请求或下载。comments 与 plan detail 明确不属于 v1 公共能力，
不能靠 DTO 偶然暴露。

FANBOX operation inventory 固定为：

```text
ValidateSession
CurrentUser
Creator
Creators
CreatorTags
CreatorPosts
TaggedPosts
Post
Home
Supporting
ResolveURL
OpenResource
SaveResource
```

`Creators` 以 request kind 区分 following/supporting；所有 list operation 返回 `sdk.Page[T]`。

## 原生媒体模型

公开模型中的所有第一方可读取媒体都通过所属产品的 `Resource` 暴露：

```go
type Resource struct {
	Ref                 ResourceRef       `json:"ref"`
	URL                 string            `json:"url"`
	RequestHeaders      map[string]string `json:"request_headers,omitempty"`
	ExpiresAt           *time.Time        `json:"expires_at,omitempty"`
	RequiresCredentials bool              `json:"requires_credentials,omitempty"`
}
```

`URL` 是当前可用的上游定位符，允许调用方直接流式读取或交给自己实现的无落盘反代；`Ref` 是稳定、
opaque 的资源 identity，供 `OpenResource`/`SaveResource` 在 SDK 内重新解析和安全读取。二者不是互斥
方案，也不能互相推导。覆盖范围至少包括：

- Pixiv：artwork 每页的所有可用尺寸、缩略图、novel/series cover、user profile image 与
  ugoira ZIP archive；
- FANBOX：post/creator cover、profile image、image block、file block 与 article 中引用的 FANBOX
  image/file asset。

两产品分别定义自己的 `ImageResource`、`FileResource` 或领域等价类型，不建立跨产品通用媒体 DTO。
每个媒体项包含 `Resource Resource`，并保留上游存在的安全 metadata，例如 asset ID、role/variant、
零基 page index、width、height、media type、extension、filename 与 size；上游未提供的 metadata
保持明确缺失，不能从签名 URL 猜测后冻结为事实。集合顺序保持上游定义的展示顺序。

Pixiv `ArtworkPages` 必须能仅凭其返回模型取得每一页至少一个可用的 image `Resource`；其他
detail/list 模型若包含媒体，也直接给出 resource，不要求调用方再访问 internal adapter。FANBOX
article block 对原生 asset 使用 resource；第三方 embed 只保留 canonical link，不能转换成
`ResourceRef`，也不能被 `OpenResource` 递归请求。

`RequestHeaders` 只允许资源读取所需、可交给终端调用方的非 secret header，例如 Pixiv image
referer；禁止 Cookie、Authorization、proxy credentials 及浏览器 session。若资源仍需调用方不可见
的产品凭据，`RequiresCredentials` 必须为 `true`，调用方应使用 `OpenResource`，不能把该 URL 当成
独立公开链接。`ExpiresAt` 只在上游提供或协议能可靠确定时设置；调用方不得把 URL 当作永久 identity。

public API inventory 和反射测试必须阻止 `DownloadURL`、`ZipURLs`、`OriginalURL`、`SignedURL` 等
重复、散落的原生媒体 URL 字段重新进入公开模型；媒体 URL 只能出现在 `Resource.URL`。普通
Pixiv/FANBOX 页面 canonical link 不属于媒体资源，可以作为链接 metadata 暴露，但不得被资源读取
接口接受。

## Ugoira metadata

```go
type UgoiraQuality string

const (
	UgoiraQualityMedium   UgoiraQuality = "medium"
	UgoiraQualityOriginal UgoiraQuality = "original"
)

type UgoiraArchive struct {
	Quality  UgoiraQuality
	Resource Resource
}

type UgoiraFrame struct {
	Filename          string
	DelayMilliseconds int
}

type UgoiraMetadata struct {
	ArtworkID int64
	Archives  []UgoiraArchive
	Frames    []UgoiraFrame
}
```

`UgoiraMetadata` 只接受 `ArtworkKindUgoira` 的 artwork ID；其他 kind 返回 `invalid_argument`。
成功结果必须有正数 `ArtworkID`、至少一个 archive 和至少一个 frame。archive 按上游可用质量返回，
同一 quality 不重复；original 不存在时不得用 medium 冒充。未知 quality 保留原始非空标识，以允许
additive 上游能力。

frame 顺序就是播放和 ZIP 解包映射顺序；`DelayMilliseconds` 原样保留上游毫秒整数，不换算为
帧率或固定时基。`Filename` 必须是相对、非空、无 traversal 的 archive entry name，并在同一结果中
唯一；非法 metadata 返回 `malformed_upstream_response`。公开模型不保留 `DownloadURL` 或
`ZipURLs`，调用方按所需 `Quality` 选择 `Archive.Resource.URL` 直接读取，或把
`Archive.Resource.Ref` 交给 `OpenResource`/`SaveResource`。

## Resource 契约

共享类型位于 package `sdk`：

```go
type ResourceMethod string

const (
	ResourceMethodGet  ResourceMethod = "GET"
	ResourceMethodHead ResourceMethod = "HEAD"
)

type OpenResourceRequest struct {
	Ref             ResourceRef
	Method          ResourceMethod
	Range           string
	IfNoneMatch     string
	IfModifiedSince string
	IfRange         string
}

```

两个产品 Client 都提供同名方法，签名使用共享资源契约：

```text
(*pixiv.Client).OpenResource(context.Context, sdk.OpenResourceRequest) (*sdk.ResourceResponse, error)
(*pixiv.Client).SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
(*fanbox.Client).OpenResource(context.Context, sdk.OpenResourceRequest) (*sdk.ResourceResponse, error)
(*fanbox.Client).SaveResource(context.Context, sdk.ResourceRef, sdk.SaveOptions) (sdk.SavedResource, error)
```

`ResourceRef` 是 product-scoped opaque identity；它不承诺编码或永久保存 `Resource.URL`。它的
Text codec 使用 route-safe 的 unpadded base64url alphabet，适合放入 URL path segment；解析后仍
必须由产品 SDK 重新验证。打开资源时重新解析或刷新
上游位置，并再次验证 scheme、host、path、redirect、header 与 resource identity。成功返回的
Body 由调用方关闭；任何错误和进度事件都不得出现 credential、Cookie 或完整签名 query。
`ResourceRef` 提供稳定的 v1 Text/JSON codec 和对应 parse，不允许调用方把任意 raw URL 伪装成 ref。

`Resource.URL` 必须在进入公开模型前验证 scheme、host、userinfo 与允许的资源路径；
`RequestHeaders` 必须使用新 map 返回，调用方修改不得改变 Client 内部状态。直接 URL 路径由调用方
负责传递规定 header、处理过期与重新获取模型；SDK 安全读取路径仍只接受 `ResourceRef`，不另设
`ResolveResourceURL`，避免形成第二套 URL 生命周期接口。

`Method` 只接受 `GET` 与 `HEAD`；零值等同 `GET`。request header 字段拒绝控制字符，SDK 不接受
任意 header map。`ResourceResponse` 只暴露状态码、经过 allowlist 的 `Content-Type`、
`Content-Length`、`Content-Range`、`Accept-Ranges`、`ETag`、`Last-Modified`、`Cache-Control`
与响应流，不把 upstream `Location`、`Set-Cookie`、认证 header 或内部 URL 交给调用方。
`Body` 在 `HEAD`、`204` 与 `304` 时也必须是可安全关闭的非 `nil` 空流。

`SaveResource` 只负责单资源条件读取、进度和原子落盘，不承担 creator 批量展开、模板、archive、
sidecar 或 ugoira 批处理。

使用方式见 [SDK 使用伪代码](sdk-usage-pseudocode.md)。
