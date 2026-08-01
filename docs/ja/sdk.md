# Pixiv Go SDK

[English](../en/sdk.md) | [简体中文](../zh-CN/sdk.md) | 日本語 | [ドキュメント索引](../index.md)

この guide は旧 HTTP Provider interface を置き換えます。公開 entry point は
`github.com/FlanChanXwO/pixiv-cli/pixiv` の具体的な `*pixiv.Client` です。HTTP endpoint、Provider server、
または discovery 可能な service ではありません。

interface が必要な consumer は、自身の adapter に最小限の method set を定義してください。SDK は `Discover`、
probe、capability negotiation、RSS、crawler behavior を提供しません。

## 構築

```go
// 初心者/local entry point: option は不要です。
local, err := pixiv.OpenDefault()

// 明示 access token または匿名 client: local auth/config field は含みません。
client, err := pixiv.NewClient(pixiv.NewClientOptions{
    AccessToken: accessToken,
})

// 高度な local/default client。
local, err := pixiv.OpenDefaultWith(pixiv.OpenDefaultOptions{
    UserID: 12345678, // optional local account
})
```

`NewClient` は local file を読まず、network authentication も行いません。`OpenDefault` は
`AuthFilePath`、`ConfigFilePath`、`RefreshToken`、`UserID`、または既定の local path と environment から
authentication を選びます。runtime configuration を要する public operation は、その都度新しい
configuration/auth snapshot を取得します。複数の pagination call で同じ snapshot を共有する必要があるときは、
`client.Snapshot(ctx)` を使用してください。明示的な token export だけは例外であり、auth store を直接読みます。

`NewClientOptions` は direct client 用の `AccessToken`、`WebFallbackEnabled`、HTTP、App/Web endpoint、
`ResourcePolicy`、任意の `ResourceCachePath`、`RequestInterval` を持ちます。`OpenDefaultOptions` は local path、OAuth endpoint、
account selection、HTTP/endpoint、resource policy/cache path を持ちます。2 つの options に無効 field は混在しません。

### HTTP client と request lifetime

options の `HTTPClient` を指定しない場合、SDK はその `Client` 専用の `http.Client` を作り、whole-request
`Timeout` は zero です。App API、Web API、OAuth、resource request は `http.DefaultClient` を変更せず、この
client を共有します。zero は SDK が response-body read 全体を覆う固定 deadline を追加しないことを意味します。
connection、TLS handshake、idle connection に対する Go transport の policy は引き続き適用されます。

渡した `context.Context` が operation 全体の lifetime を制御します。caller は operation に適した cancellation または
deadline を設定してください。`context.Canceled` と `context.DeadlineExceeded` は `errors.Is` で検出できます。
`OpenResource` の返却後も context は body read を制御します。stream の利用を終えたら body を close し、context を
cancel してください。

options の `HTTPClient` を指定した場合、constructor は同じ pointer とその timeout、transport、cookie jar、redirect policy
をそのまま保持します。resource request だけは per-request copy で cookie を無効化し、redirect を検証します。
詳細は [ADR 0010](../maintainers/adr/0010-http-client-timeout-and-context.md) を参照してください。

## 読み取りと書き込みの操作

| Category | Method |
| --- | --- |
| Works と recommendation | `SearchIllust`、`SearchNovel`、`SupportedDrawingTools`、`IllustDetail`、`IllustPages`、`IllustRelated`、`IllustRanking`、`IllustRecommended`、`MangaRecommended`、`NovelRecommended`、`UserRecommended`、`FollowingIllusts`、`TrendingTagsIllust`、`IllustSeries`、`UgoiraMetadata`。 |
| Users | `SearchUser`、`UserDetail`、`UserArtworks`、`UserBookmarks`、`UserFollowing`、`CurrentUserID`。 |
| Writes | `AddBookmark`、`RemoveBookmark`、`FollowUser`、`UnfollowUser`。 |
| Accounts/configuration | `ImportAccount`、`ListAccounts`、`SelectAccount`、`RemoveAccount`、`ExportAccountRefreshToken`、`ExportAuthBundle`、`RestoreAuthBundle`、`CheckAccount`、`CheckRefreshToken`、`Refresh`、`RefreshAccount`、`PremiumStatus`、`RefreshPremiumStatus`、`GetConfig`、`SetConfig`、`UnsetConfig`。bundle codec function は package-level です。 |
| Login | `StartLogin`、`CompleteLogin`、`BuildLoginAuthorizationURL`。SDK は browser、loopback server、TTY を起動しません。 |
| Resources | `Download`、`DownloadAll`、`DownloadWith`、`DownloadAllWith`、`ParseResourceRef`、`OpenResource`、`DownloadResource`。 |

request method は `SearchIllustRequest`、`SearchNovelRequest`、
`UserArtworksRequest`、`UserBookmarksRequest`、`UserFollowingRequest`、`AddBookmarkRequest`、
`FollowUserRequest` などの名前付き request type を使用します。`IllustListResult`、`NovelListResult`、
`UserListResult`、`IllustDetail`、`UserDetailResult` などの result model はすべて
top-level `pixiv` package にあります。

すべての public `Illust` は安定した artwork page URL
`https://www.pixiv.net/artworks/{id}` を JSON の先頭 field `url` として含みます。SDK は like-count field を公開しません。
bookmark total を like と表示してはいけません。

### Download: beginner と advanced

`Download(ctx, src)` と `DownloadAll(ctx, srcs)` は初心者 API です。PID、official artwork URL、または
`ResourcePolicy` が許可する CDN URL を受け取り、`./downloads`、`{author} - {title}_{id}`、original、全 page、
`2 × runtime.GOMAXPROCS(0)` 自動並行を使います。

`DownloadWith` / `DownloadAllWith` と `DownloadOptions` では `DownloadPath`、`FilenameTemplate`、`DirectoryTemplate`、closed `Pages` または open range 対応 `PageSelection`、`Quality`、`UgoiraMode`、`Concurrency`、compiled `Filter`、`ArchivePath`、`WriteMetadata`、`RetryPolicy`、observation-only `Progress` callback を指定できます。`UgoiraMode` は既定 `gif` で、`apng`、lossless `zip`、`frames`（frame file と timing manifest）も選べます。template は `{id}`、`{title}`、`{author}`、`{author_id}`、`{date}`、`{tags}`、`{num}` をサポートし、directory template は safe relative path、`{num}` は zero-based です。SQLite archive は選択 output と requested sidecar がすべて成功した artwork だけを記録します。sidecar は public `Illust`、relative artifact path、page、mode、ugoira timing を含む atomic `{artifact}.json` です。CDN 直リンクは metadata-dependent option を拒否します。resource read は valid `Retry-After` を持つ 429、5xx、retryable transport failure を既定で 3 回（1/2/4 秒）retry します。cancel、permanent local error、4xx は replay しません。

`Progress func(DownloadProgress)` は download worker から直接かつ concurrently に呼ばれます。event は input の
`SourceIndex`、1-based `Page`、destination path、利用できる artwork metadata、resource/batch byte counter を持ちます。
callback を設定すると SDK は各 resource を safe HEAD probe します。全 size が分かる場合は `TotalBytesKnown` と
`TotalBytes` が batch を表し、unknown size の場合も resource/batch の transferred byte を報告します。validated partial
byte は最初の event から含まれます。callback は non-blocking にし、transfer は supplied context を cancel して停止します。

`DownloadResource(ctx, ref, destination)` は raw-resource 用の明示 API で、`miss`、`revalidated`、`resumed`、`refreshed`
を持つ `ResourceDownloadResult` を返します。旧 raw `Download(ctx, ResourceRef, path)` を置換します。

configuration も型付きです。`GetConfig`、`SetConfig`、`UnsetConfig` には `ConfigKey` constant を使い、write は `StringConfigInput`、`BoolConfigInput`、`DurationConfigInput` で構築します。CLI/MCP の text boundary は `ParseConfigKey` と `ParseConfigInput` を使います。sensitive relay secret は generic setter では書けず `SetLoginRelaySecret` だけを使い、read は redacted presence marker だけを返します。

`UserArtworksRequest.UserID` のような SDK の user ID は必須です。UID の省略を「current user」の意味にできるのは
CLI/MCP adapter だけです。Go caller は `CurrentUserID(ctx)` を呼び、その ID で request を組み立ててください。

`UserDetail` は常に `UserDetailResult{User, Profile, ProfilePublicity, Workspace}` を返します。upstream envelope が
欠落、`null`、object 以外、または `user.id <= 0` なら、SDK は `OperationUserDetail`、`BackendAppAPI`、要求 UID を持つ
`malformed_upstream_response` を返します。upstream body、URL、credential は公開しません。任意の URL field は missing、
`null`、empty string を `nil` に正規化し、非公開 value は Go の zero value を維持します。

4 種類の personalized recommendation stream はすべて認証済み App API operation です。illustration と manga は
`IllustRecommendedRequest`、novel は `NovelRecommendedRequest`、user は `UserRecommendedRequest` を使い、各々が独立した
opaque cursor を返します。CLI/MCP の `all` は boundary で 4 call を illustration、manga、novel、user の順に組み合わせる
だけであり、SDK の single-stream cursor contract を変更しません。

authentication に使えるのは raw Pixiv App API refresh token だけです。`ImportAccount`、`CheckRefreshToken`、
`OpenDefault`、local account から読み込む token は、`refresh_token=...` のような cookie-shaped value を OAuth request 前に
拒否し、入力を含まない `invalid_argument` を返します。

`ExportAccountRefreshToken(userID int64)` は、保存済み credential を別の信頼できる local integration に渡すための、
明示的な local secret-export operation です。`userID == 0` は `auth.json.default_user_id` を選び、正の ID はその account を
正確に選びます。auth store だけを読み、`PIXIV_REFRESH_TOKEN` と runtime configuration を無視し、refresh、network request、
file modification を行いません。local auth path を持たない `NewClient` は `unsupported` を返します。戻り値は opaque secret
として扱い、log、error、telemetry、MCP/JSON に出してはいけません。

### Authentication bundle と offline restore

`AuthExportSelection{}` は local default account を選択し、`AuthExportSelection{UserID: id}` は 1 account を正確に選び、
`AuthExportSelection{All: true}` はすべての stored account を選びます。`UserID` は negative ではならず、`All` と併用できません。
`Client.ExportAuthBundle` は lock 下の read-only local snapshot です。environment token と runtime account override を無視し、
network/refresh を行わず、state を変更しません。戻り値は `AuthExportBundle{Schema, Version, DefaultUserID, Accounts}` で、
各 `AuthExportSecretAccount` は UID、任意の username、opaque refresh-token secret を持ちます。

`EncodeAuthExportBundle` は末尾 newline を含む安定した indented JSON を出力します。`DecodeAuthExportBundle` は strict であり、
非対応の schema/version、unknown または duplicate field、trailing JSON、empty account list、duplicate または non-positive UID、
empty refresh token、account list に存在しない default UID を拒否します。top-level と account object の key は canonical な
spelling/case と完全に一致しなければなりません。case alias と canonical-plus-alias conflict も拒否します。どちらも bundle
content を含まない redacted typed error を返します。

`Client.RestoreAuthBundle` は decode 済み bundle を検証し、local auth state を lock 下で UID により merge して、1 回の atomic
store write を行います。OAuth と transport は利用しません。既存 account は update、新しい account は add されます。local
default は、空でない限り維持され、空の場合だけ bundle default が採用されます。`AuthRestoreResult` が報告するのは
`DefaultUserID` と secret-free な `Added`、`Updated` account summary だけです。

format は unencrypted な point-in-time backup であり、live sync ではありません。encoded bytes は original token と同じように
保護し、token rotation 後に古い bundle や他 machine の copy が stale になることを考慮してください。

`BuildLoginAuthorizationURL(challenge, state)` は自前で PKCE と state を管理する adapter 向けに、official authorization URL
だけを構築します。SDK に PKCE session を管理させるときは `StartLogin` を使用してください。

### Illustration search filter

`SearchIllustRequest.Filters` は App/Web の wire parameter から独立した安定 domain value を使用します。

| Field | Stable value |
| --- | --- |
| `Rating` | `all`、`sfw`、`r18`、`r18g`、`mature` |
| `ContentType` | `all`、`illust-and-ugoira`、`illust`、`manga`、`ugoira` |
| `AIMode` | `all`、`exclude`、`only`。Pixiv `AIType==2` は AI-generated を意味します。 |
| `AspectRatio` | `all`、`landscape`、`portrait`、`square` |
| `Resolution` | `all`、`high`、`medium`、`low`。両 dimension が順に `>=3000`、`1000..2999`、`<=999` です。 |
| `Tool` | versioned drawing-tool catalog の exact value。unique な 1-edit spelling mistake には suggestion を返し、ambiguous prefix は `invalid_argument`。 |
| `BookmarkMin` / `BookmarkMax` | 任意の包含境界の非負 public bookmark 数。App OAuth と有効な Pixiv Premium 会員資格が必要で、`Min` は `Max` を超えられません。保存済み account の `OpenDefault` は request 前に cached self-profile status を確認し、非 Premium には local `forbidden` を返します。 |

zero enum value は `all` に正規化され、`Tool` は trim されます。unknown value は upstream request 前に
`invalid_argument` を返します。認証済み adapter は resolution、aspect ratio、tool、content type、AI exclusion を App server
parameter と Pixiv Premium 限定の bookmark-count parameter に変換します。rating と AI-only filter は current App batch の正規化 field を使用します。`Illust.Tools []string` は
upstream の order と value を保持し、bookmark-count filter とは関係ありません。

`SupportedDrawingTools() []string` は versioned drawing-tool catalog を documented order で返します。network request は行わず、caller が変更できる defensive copy を返します。`PremiumStatus(ctx)` は保存済み認証 account の cached-or-fresh membership snapshot を返し、`RefreshPremiumStatus(ctx)` は profile を強制取得して結果を保存します。`OpenDefault` は `[premium] status_cache_ttl`（既定 `24h`、`0s` は reuse 無効）を使います。直接 `NewClient` access token には検証可能な account UID がないため、この saved-account precheck はできません。

### Local illustration expression filter

`CompileIllustFilter(expression)` は opaque で side-effect-free な `*IllustFilter` を compile し、`Match(Illust)` で public illustration を判定します。field は `id`、`userId`、`userName`、`type`、`title`、`createDate`、`pageCount`、`bookmarkCount`、`viewCount`、`xRestrict`、`aiType`、`width`、`height`、`tags`、`tools`、`rating`、`aiMode`、`aspectRatio`、`resolution`、`drawTool` だけです。boolean comparison、`and`/`or`/`not`、`in`/`not in`、array literal、Expr native `any`/`all` だけを許可します。例は `any(tags, # in ["miku", "vocaloid"]) and bookmarkCount >= 5000` です。arithmetic、regex、object/map/member access、variable、conditional、pipeline、reflection、その他 function は request 前に拒否されます。compile error は field/type/source column を示し、empty result に偽装しません。

### Novel search と user-search source

`SearchNovel(ctx, SearchNovelRequest{...})` は認証済み App API 専用です。`Target` は illustration search と同じ
`partial_match_for_tags`、`exact_match_for_tags`、`title_and_caption` を受け取り、`Sort` は
`date_desc|date_asc`、`Duration` は empty、`within_last_day`、`within_last_week`、`within_last_month` を受け取ります。
`NovelSearchFilters` は `Rating`、`MinTextLength`、`MaxTextLength`、`OriginalOnly` を含みます。text-length bound が zero の
場合は無効であり、non-zero maximum が minimum 未満なら `invalid_argument` です。

App endpoint には rating、text length、original-only の検証済み wire parameter がありません。SDK は stable result field の
`Novel.XRestrict`、`Novel.TextLength`、`Novel.IsOriginal` で filter します。各 search response は 3 field すべてを含む必要があり、
欠落時は guessed match や unlabeled partial result ではなく `malformed_upstream_response` になります。返される `Novel` は
安定した URL `https://www.pixiv.net/novel/show.php?id={id}` も公開します。

`SearchUser` は結果 semantics を常に `UserListResult.Source` で表します。認証済み App search は `app_search`、匿名 Web
fallback は `related_illust_authors` です。後者は illustration search から deduplicate した author list であり、official
username search ではありません。source は cursor sequence 全体で安定します。

### Illustration ranking

`IllustRankingRequest.Mode` は 16 種類すべての App API mode を受け取ります：`day`、`day_male`、`day_female`、`week`、
`week_original`、`week_rookie`、`month`、`day_manga`、`week_manga`、`month_manga`、`week_rookie_manga`、`day_r18`、
`day_male_r18`、`day_female_r18`、`week_r18`、`week_r18g`。先頭の 7 mode は匿名 Web allowlist でも利用できます。残りの
9 mode は authentication が必須で、refresh token がない場合は Web request 前に `unauthorized` を返します。匿名 daily ranking
へ黙って置き換えることはありません。

## ページネーション

list result は opaque な `pixiv.Cursor` を公開します。同じ operation と query にそのまま渡してください。

```go
result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: uid})
if err != nil { /* handle */ }
next, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{
    UserID: uid,
    Cursor: result.NextCursor,
})
_ = next
```

cursor は versioned で、operation と完全に正規化された query に bound されています。illustration-search cursor は
`Rating`、`ContentType`、`AIMode`、`AspectRatio`、`Resolution`、`Tool` にも bound され、novel-search cursor は target、sort、
duration、rating、text-length bound、original-only condition に bound されます。filter を変えて古い cursor を再利用すると
`invalid_argument` です。cursor の parse/edit、request 間の再利用、upstream offset/page への置換はできません。SDK は
`page` を受け取らず、CLI/MCP が boundary で logical page と limit を変換します。

## ルーティング

refresh token がある場合、illustration search は App API だけを使います。authentication、network、server failure があっても
Web へ自動 fallback しません。token がなく `WebFallbackEnabled=true` の `NewClient` は匿名 Web allowlist を利用できます。
`OpenDefault` は snapshot ごとに local `web_fallback_enabled` を読みます。匿名 search は Web が確実に表現できる filter だけを
使用します。`r18`、`r18g`、`mature` は network 前に `unauthorized` になり、empty result に偽装されません。SDK は cookie を read/inject せず、refresh token を Web session に
変換しません。

`SearchNovel` は App authentication が必要で、Web に fallback しません。`SearchUser` は認証済みなら App search を使い、
匿名 allowlist route は常に `Source=related_illust_authors` を返すため、caller は official operation と取り違えません。

認証済みの `IllustDetail`、`IllustPages`、`UgoiraMetadata` は App API 専用です。`IllustPages` は multi-page work の
App `meta_pages` を使い、single-page work は App の single-page/image field から `meta_pages[0]` を導出します。public JSON
shape は変更しません。page data の欠落または page-count mismatch は `malformed_upstream_response` であり、unlabeled partial
result ではありません。App failure は Web request を行いません。`Illust.Caption` は raw App `caption` または匿名 Web
`description` を保持し、HTML の rendering は public SDK ではなく presentation adapter が決定します。

`UgoiraMetadata.UgoiraMetadata` は、検証済み resource pair である non-empty `download_url` と
`download_quality`（`medium` または `original`）を公開します。`zip_urls.original` は original ZIP を実際に取得できた場合だけ
出力されます。認証済み App response は medium ZIP を選び（`download_quality=medium`）、Web で補完しません。匿名 Web response
が original ZIP を提供するときは `original` を選びます。`Download` は `download_url` を使うため、consumer は
`zip_urls.original` が存在すると仮定してはいけません。匿名 `IllustDetail` は依然として Web detail/pages を読み、いずれかの
stage が失敗すれば atomic に失敗します。詳細は [ADR 0006](../maintainers/adr/0006-original-ugoira-resource-resolution.md) を参照してください。

## Resource と image proxying

```go
ref, err := client.ParseResourceRef(rawURL)
if err != nil { /* reject */ }
response, err := client.OpenResource(ctx, pixiv.OpenResourceRequest{
    Ref: ref, Range: request.Header.Get("Range"), IfRange: request.Header.Get("If-Range"),
})
if err != nil { /* map typed error */ }
defer response.Body.Close()
// response.StatusCode と filter 済み response.Header を io.Copy で stream します。
```

`ResourceRef` は persistent reference にすぎず、`OpenResource` ごとに再検証します。default policy は official Pixiv
resource だけを受け入れます。caller は `ResourcePolicy.Mirrors` に明示的な host/path prefix を追加できます。SDK は
`Range`、`If-None-Match`、`If-Modified-Since`、`If-Range` だけを受け取り、response header を filter し、cookie を無効化して redirect を
検証することで SSRF を低減します。`DownloadResource` は `.pixiv-cache`（または `ResourceCachePath`）に metadata と partial を保存し、ETag/Last-Modified で complete file を再検証し、検証済み partial だけを `Range` + `If-Range` で再開して atomic に publish します。validator がなければ unsafe resume はしません。

idempotent App API JSON read だけでは、最初の HTTP 429 に有効な seconds value または HTTP date の `Retry-After` があれば、
1 回待機して retry します。wait は caller context を監視します。invalid/missing header、2 回目の 429、その他の error は
元の typed error のままです。mutation と resource download は replay しません。

## エラー

public failure は `*pixiv.Error` になりえます。

```go
var pixivErr *pixiv.Error
if errors.As(err, &pixivErr) {
    switch pixivErr.Code {
    case pixiv.CodeArtworkUnavailable:
        // deleted/private/region- or permission-limited item は skip できます。
    case pixiv.CodeRateLimited:
        // caller の policy に従って schedule します。
    }
}
if errors.Is(err, pixiv.ErrUnauthorized) { /* re-authenticate */ }
```

stable code は `invalid_argument`、`artwork_unavailable`、`unauthorized`、`forbidden`、`unsupported`、
`rate_limited`、`upstream_error`、`upstream_unavailable`、`malformed_upstream_response` を含みます。error は stable な
operation、backend、retryable flag、status、検証済み ID を持ちますが、token、cookie、full URL、header、upstream response body
は含みません。

| Call と failure stage | Result | `Operation` | `Backend` |
| --- | --- | --- | --- |
| 認証済み `IllustDetail` の App detail/pages | `nil` | `OperationIllustDetail` | `BackendAppAPI` |
| 認証済み `IllustPages` の App detail/pages | `nil` | `OperationIllustPages` | `BackendAppAPI` |
| 匿名 `IllustDetail` の Web pages | `nil` | `OperationIllustPages` | `BackendWebAPI` |
| 匿名 `IllustDetail` の Web detail | `nil` | `OperationIllustDetail` | `BackendWebAPI` |
| 認証済み `UgoiraMetadata` の App metadata | `nil` | `OperationUgoiraMetadata` | `BackendAppAPI` |
| 匿名 `UgoiraMetadata` の Web metadata | `nil` | `OperationUgoiraMetadata` | `BackendWebAPI` |

例えば、認証済み page retrieval 中の App 403 は `CodeForbidden`、`BackendAppAPI`、`OperationIllustPages`、
`UpstreamStatus=403` になり、Web へ続行しません。

`upstream_unavailable` の transport failure は安全な `Error.TransportKind` として `dns`、`tls`、`proxy`、
`connection_refused`、`connection_reset`、`timeout`、`unknown` を追加で公開します。分類は error text ではなく typed/wrapped Go
cause を使用します。`context.Canceled` と `context.DeadlineExceeded` は引き続き `errors.Is` signal であり、transport kind を
持ちません。

local account/configuration の `invalid_argument` failure は、安全な `Error.LocalStateKind` として `auth_malformed`、
`config_malformed`、`permission_denied`、`not_found`、`invalid_proxy`、`account_mismatch`、`unavailable`、`unknown` を公開できます。
`errors.Unwrap` と `Error()` は redacted のままで、filesystem/parser error、path、local content、proxy userinfo を公開しません。
任意の `auth.json` または `config.toml` がないことは有効な empty state です。

`RestoreAuthBundle` が merge 済み auth store の保存に失敗した場合、error は `Error.LocalWriteCommitOutcome` も公開します。
`not_committed` は replacement が発生しなかったこと、`committed` は replacement 後に durability または cleanup が失敗したため
target を reload する必要があること、`unknown` は recovery で target state を確定できず manual inspection が必要なことを示します。
caller は `committed` または `unknown` を successful rollback と報告してはいけません。

## Caller の責務

caller adapter は collection mode、budget、filter、cursor storage、database transaction、scheduling、retry、外部 HTTP API を
担当します。`atri-setu-api` の random selection、moderation、gallery storage、image proxy policy は SDK の feature ではありません。
integration は normalized model と resource stream をもとに実装できます。

完全な boundary decision は [ADR 0009](../maintainers/adr/0009-public-pixiv-sdk-and-caller-adapter.md) と
[ADR 0010](../maintainers/adr/0010-http-client-timeout-and-context.md) を参照してください。
