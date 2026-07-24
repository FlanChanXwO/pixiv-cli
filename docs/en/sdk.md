# Pixiv Go SDK

English | [简体中文](../zh-CN/sdk.md) | [日本語](../ja/sdk.md) | [Documentation index](../index.md)

This guide replaces the former HTTP Provider interface. The public entry point is the concrete `*pixiv.Client`
from `github.com/FlanChanXwO/pixiv-cli/pixiv`, not an HTTP endpoint, Provider server, or discoverable service.

Consumers that need an interface should define the smallest method set in their own adapter. The SDK does not
provide `Discover`, probes, capability negotiation, RSS, or crawler behavior.

## Construction

```go
client, err := pixiv.NewClient(pixiv.Options{
    AccessToken: accessToken,
    Logger:      logger, // optional; nil keeps the SDK quiet
})

local, err := pixiv.OpenDefault(pixiv.Options{
    UserID: 12345678, // optional local account
})
```

`NewClient` never reads local files or performs network authentication. `OpenDefault` selects authentication from
`AuthFilePath`, `ConfigFilePath`, `RefreshToken`, `UserID`, or the default local paths and environment. Public
operations that require runtime configuration obtain a fresh configuration/auth snapshot. Use
`client.Snapshot(ctx)` when several pagination calls must share one snapshot. Explicit token export is the only
exception and reads the auth store directly.

`Options` accepts an explicit `HTTPClient`, `AppAPIBaseURL`, `WebAPIBaseURL`, `OAuthBaseURL`,
`WebFallbackEnabled`, `ResourcePolicy`, and `Logger`. `AccessToken` and `WebFallbackEnabled` apply only to
`NewClient`; each `OpenDefault` snapshot reads local `web_fallback_enabled`. Do not make refresh tokens or loggers
global mutable state.

### HTTP client and request lifetime

Without `Options.HTTPClient`, the SDK creates a dedicated `http.Client` for that `Client` with a zero whole-request
`Timeout`. App API, Web API, OAuth, and resource requests share it instead of mutating `http.DefaultClient`. Zero
means the SDK adds no fixed deadline covering response-body reads; Go transport policies for connection, TLS
handshake, and idle connections still apply.

The supplied `context.Context` controls the total operation lifetime. Callers should add cancellation or a
deadline appropriate to the operation. `context.Canceled` and `context.DeadlineExceeded` remain detectable with
`errors.Is`. After `OpenResource` returns, the context also governs body reads; close the body and cancel the
context when the stream is no longer needed.

When `Options.HTTPClient` is provided, the constructor preserves the same pointer and its timeout, transport,
cookie jar, and redirect policy. Resource requests still use per-request copies that disable cookies and validate
redirects. See [ADR 0010](../maintainers/adr/0010-http-client-timeout-and-context.md).

## Read and write operations

| Category | Methods |
| --- | --- |
| Works and recommendations | `SearchIllust`, `SearchNovel`, `SearchIllustOptions`, `IllustDetail`, `IllustPages`, `IllustRelated`, `IllustRanking`, `IllustRecommended`, `MangaRecommended`, `NovelRecommended`, `UserRecommended`, `FollowingIllusts`, `TrendingTagsIllust`, `UgoiraMetadata`. |
| Users | `SearchUser`, `UserDetail`, `UserArtworks`, `UserBookmarks`, `UserFollowing`, `CurrentUserID`. |
| Writes | `AddBookmark`, `RemoveBookmark`, `FollowUser`, `UnfollowUser`. |
| Accounts/configuration | `ImportAccount`, `ListAccounts`, `SelectAccount`, `RemoveAccount`, `ExportAccountRefreshToken`, `ExportAuthBundle`, `RestoreAuthBundle`, `CheckAccount`, `CheckRefreshToken`, `Refresh`, `RefreshAccount`, `GetConfig`, `SetConfig`, `UnsetConfig`; bundle codec functions are package-level. |
| Login | `StartLogin`, `CompleteLogin`, `BuildLoginAuthorizationURL`; the SDK does not start a browser, loopback server, or TTY. |
| Resources | `ParseResourceRef`, `OpenResource`, `Download`. |

Request methods use named request types such as `SearchIllustRequest`, `SearchNovelRequest`, `SearchIllustOptionsRequest`,
`UserArtworksRequest`, `UserBookmarksRequest`, `UserFollowingRequest`, `AddBookmarkRequest`, and
`FollowUserRequest`. Result models such as `IllustListResult`, `SearchIllustOptionsResult`, `UserListResult`,
`IllustDetail`, and `UserDetailResult` all live in the top-level `pixiv` package.
Every public `Illust` includes a stable artwork page URL
`https://www.pixiv.net/artworks/{id}` as the first JSON field `url`. The SDK does not
expose a like-count field; bookmark totals must not be labeled as likes.
`Download` accepts `DownloadOptions` with `ParsePageSpec` page selection and
`DownloadQuality` (`original|regular|small|thumb|mini`); ugoira rejects page selection
or non-original quality as unsupported.

### Local Pixiv references

`ParseReference(raw)` performs no I/O and accepts either a positive artwork ID or a
strict official Pixiv HTTPS URL. It returns `Reference{Kind, ID}`, where `Kind` is
`artwork` for an ID or `/artworks/{id}`, and `user` for `/users/{id}` or
`/users/{id}/artworks`. The URL host must be `pixiv.net` or `www.pixiv.net`; an
optional locale, query, and fragment are allowed. `ParseArtworkReference(raw)` is the
artwork-only variant, and `Reference.URL()` returns the canonical artwork or user page
URL. The parser neither follows redirects nor fetches HTML, rejects all other Pixiv
properties and URL forms, and its validation errors do not reproduce the supplied URL.

SDK user IDs such as `UserArtworksRequest.UserID` are required. Omitting a UID to mean “the current user” is a
CLI/MCP adapter feature; Go callers should call `CurrentUserID(ctx)` and then build the request.

`UserDetail` always returns `UserDetailResult{User, Profile, ProfilePublicity, Workspace}`. If an upstream envelope
is missing, `null`, not an object, or has `user.id <= 0`, the SDK returns `malformed_upstream_response` with
`OperationUserDetail`, `BackendAppAPI`, and the requested UID, without exposing upstream bodies, URLs, or
credentials. Optional URL fields normalize missing, `null`, and empty strings to `nil`; undisclosed values retain
their Go zero values.

All four personalized recommendation streams are authenticated App API operations. Illustrations and manga use
`IllustRecommendedRequest`, novels use `NovelRecommendedRequest`, and users use `UserRecommendedRequest`; each
returns its own opaque cursor. CLI/MCP `all` combines the four calls in illustration, manga, novel, user order and
does not change the SDK's one-stream cursor contract.

Authentication accepts only a raw Pixiv App API refresh token. `ImportAccount`, `CheckRefreshToken`, `OpenDefault`,
and locally loaded accounts reject cookie-shaped values such as `refresh_token=...` before any OAuth request and
return a redacted `invalid_argument` error.

`ExportAccountRefreshToken(userID int64)` is an explicit local secret-export operation for handing a stored
credential to another trusted local integration. `userID == 0` selects `auth.json.default_user_id`; a positive ID
selects that exact account. It reads only the auth store, ignores `PIXIV_REFRESH_TOKEN` and runtime configuration,
performs no refresh or network request, and does not modify files. `NewClient` without a local auth path returns
`unsupported`. Treat the returned string as an opaque secret: never log it, format it into an error, send it to
telemetry, or expose it through MCP/JSON.

### Authentication bundles and offline restore

`AuthExportSelection{}` selects the local default account, `AuthExportSelection{UserID: id}` selects one exact
account, and `AuthExportSelection{All: true}` selects every stored account. `UserID` must not be negative and cannot
be combined with `All`. `Client.ExportAuthBundle` is a locked, read-only local snapshot: it ignores environment
tokens and runtime account overrides, performs no network/refresh, and does not mutate state. It returns
`AuthExportBundle{Schema, Version, DefaultUserID, Accounts}`; each `AuthExportSecretAccount` contains UID, optional
username, and an opaque refresh-token secret.

`EncodeAuthExportBundle` emits the stable indented JSON form with a final newline. `DecodeAuthExportBundle` is
strict: it rejects unsupported schema/version, unknown or duplicate fields, trailing JSON, empty account lists,
duplicate or non-positive UIDs, empty refresh tokens, and a default UID that is absent from the account list.
Top-level and account-object keys must exactly match the documented canonical spelling and case; case aliases and
canonical-plus-alias conflicts are rejected. Both functions return redacted typed errors and never include bundle
contents.

`Client.RestoreAuthBundle` validates an already decoded bundle, locks the local auth state, merges accounts by UID,
and performs one atomic store write without OAuth or any transport. Existing accounts are updated, new accounts are
added, and the local default is preserved unless it was empty, in which case the bundle default is adopted.
`AuthRestoreResult` reports only `DefaultUserID`, secret-free `Added`, and secret-free `Updated` account summaries.

The format is an unencrypted point-in-time backup, not live sync. Callers must protect the encoded bytes like the
original tokens, and account for an old bundle or another machine's copy becoming stale after token rotation.

`BuildLoginAuthorizationURL(challenge, state)` only constructs the official authorization URL for adapters that
manage their own PKCE and state. Use `StartLogin` when the SDK should manage the PKCE session.

### Illustration search filters

`SearchIllustRequest.Filters` uses stable domain values independent of App/Web wire parameters:

| Field | Stable values |
| --- | --- |
| `Rating` | `all`, `sfw`, `r18`, `r18g`, `mature` |
| `ContentType` | `all`, `illust-and-ugoira`, `illust`, `manga`, `ugoira` |
| `AIMode` | `all`, `exclude`, `only`; Pixiv `AIType==2` means AI-generated |
| `AspectRatio` | `all`, `landscape`, `portrait`, `square` |
| `Resolution` | `all`, `high`, `medium`, `low`; both dimensions must respectively be `>=3000`, `1000..2999`, or `<=999` |
| `Tool` | Exact upstream drawing-tool value; no fuzzy matching |
| `BookmarkMin` / `BookmarkMax` | Optional inclusive non-negative public bookmark-count bounds; `Min` cannot exceed `Max` |

Zero enum values normalize to `all`; `Tool` is trimmed. Unknown values return `invalid_argument` before any
upstream request. `SearchIllustRequest.Target` also accepts `keyword` for tags, titles, and captions; `Duration`
accepts `within_last_day|within_last_week|within_last_month`; `StartDate` and `EndDate`
are optional inclusive `YYYY-MM-DD` bounds. A date range cannot be combined with `Duration`, and a supplied start
cannot be later than end. The authenticated adapter maps resolution, aspect ratio, tool, content type, AI exclusion,
date bounds, and bookmark bounds to App server parameters. Rating and AI-only filtering use normalized fields from
the current App batch.
`Illust.Tools []string` preserves upstream order and values and is unrelated to bookmark-count filtering.

`SearchIllustOptions(ctx, SearchIllustOptionsRequest{Word: word})` requires a non-empty word and App
authentication. It returns `SearchIllustOptionsResult{Tools []string}` in upstream order; a missing list becomes a
non-nil empty slice. Premium bookmark tiers are not exposed.

### Novel search and user-search source

`SearchNovel(ctx, SearchNovelRequest{...})` is authenticated App API only. `Target` accepts the same stable
`partial_match_for_tags`, `exact_match_for_tags`, and `title_and_caption` values as illustration search; `Sort`
accepts `date_desc|date_asc`; `Duration` is empty or `within_last_day|within_last_week|within_last_month`.
`NovelSearchFilters` contains `Rating`, `MinTextLength`, `MaxTextLength`, and `OriginalOnly`. Zero text-length
bounds are disabled; a non-zero maximum below the minimum is `invalid_argument`.

The App endpoint has no verified wire parameters for rating, text length, or original-only. The SDK applies those
filters to stable result fields instead: `Novel.XRestrict`, `Novel.TextLength`, and `Novel.IsOriginal`. Each search
response must contain all three fields; missing data is a typed `malformed_upstream_response`, never a guessed match
or an unlabeled partial result. Returned `Novel` values also expose the stable `URL` form
`https://www.pixiv.net/novel/show.php?id={id}`.

`SearchUser` always labels the result semantics in `UserListResult.Source`. Authenticated App search returns
`app_search`; anonymous Web fallback returns `related_illust_authors`, which is a de-duplicated author list from
illustration search rather than an official username search. The source is stable across a cursor sequence.

### Illustration rankings

`IllustRankingRequest.Mode` accepts all 16 App API modes: `day`, `day_male`, `day_female`, `week`,
`week_original`, `week_rookie`, `month`, `day_manga`, `week_manga`, `month_manga`, `week_rookie_manga`,
`day_r18`, `day_male_r18`, `day_female_r18`, `week_r18`, and `week_r18g`. The first seven remain available to the
anonymous Web allowlist. The other nine require authentication and return `unauthorized` before a Web request when
no refresh token is available; they never silently become the daily ranking.

## Pagination

List results expose an opaque `pixiv.Cursor`. Pass it back to the same operation and query:

```go
result, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{UserID: uid})
if err != nil { /* handle */ }
next, err := client.UserArtworks(ctx, pixiv.UserArtworksRequest{
    UserID: uid,
    Cursor: result.NextCursor,
})
_ = next
```

Cursors are versioned and bound to the operation and complete normalized query. An illustration-search cursor also
binds target, duration, date bounds, `Rating`, `ContentType`, `AIMode`, `AspectRatio`, `Resolution`, `Tool`, and bookmark bounds; a novel-search cursor binds its
target, sort, duration, rating, text-length bounds, and original-only condition. Changing a filter and reusing the
old cursor returns `invalid_argument`. Never parse, edit, reuse across requests, or replace a cursor with an upstream
offset/page. The SDK does not accept `page`; CLI/MCP translate logical pages and limits at their boundaries.

## Routing

With a refresh token, illustration search uses only App API. Authentication, network, and server failures never
fall back to Web automatically. Without a token, `NewClient` can use the anonymous Web allowlist when
`WebFallbackEnabled=true`; `OpenDefault` reads local `web_fallback_enabled` for each snapshot. Anonymous search
uses only filters Web can express reliably. `r18`, `r18g`, `mature`, `Target=keyword`, and bookmark bounds fail
with `unauthorized` before networking; they are never disguised as empty results. Anonymous `SearchIllustOptions` returns `unsupported`. The SDK never
reads/injects cookies or converts a refresh token into a Web session.

`SearchNovel` requires App authentication and never falls back to Web. `SearchUser` uses App search when
authenticated; its anonymous allowlist route is exposed only with `Source=related_illust_authors`, so callers cannot
mistake it for the official operation.

Authenticated `IllustDetail`, `IllustPages`, and `UgoiraMetadata` use App API only. `IllustPages` takes multi-page
data from App `meta_pages`; for a single-page work it derives `meta_pages[0]` from the App single-page/image fields
without changing the public JSON shape. Missing page data or a page-count mismatch is a typed
`malformed_upstream_response`, never an unlabeled partial result. An App failure never makes a Web request.
`Illust.Caption` preserves the raw App `caption` or anonymous Web `description`; presentation adapters, rather than
the public SDK, decide whether to render its HTML.

`UgoiraMetadata.UgoiraMetadata` exposes a verified resource pair: non-empty `download_url` and
`download_quality` (`medium` or `original`). `zip_urls.original` is omitted unless that original ZIP was actually
obtained. An authenticated App response selects its medium ZIP (`download_quality=medium`) and does not ask Web to
fill it in; an anonymous Web response that provides an original ZIP selects `original`. `Download` uses
`download_url`, so consumers must not assume `zip_urls.original` exists. Anonymous `IllustDetail` still reads Web
detail/pages and fails atomically if either stage fails. See
[ADR 0006](../maintainers/adr/0006-original-ugoira-resource-resolution.md).

## Resources and image proxying

```go
ref, err := client.ParseResourceRef(rawURL)
if err != nil { /* reject */ }
response, err := client.OpenResource(ctx, pixiv.OpenResourceRequest{
    Ref: ref, Range: request.Header.Get("Range"),
})
if err != nil { /* map typed error */ }
defer response.Body.Close()
// Stream response.StatusCode and filtered response.Header with io.Copy.
```

`ResourceRef` is only a persistent reference; every `OpenResource` revalidates it. The default policy accepts only
official Pixiv resources. Callers may add explicit host/path prefixes through `ResourcePolicy.Mirrors`. The SDK
accepts only `Range`, `If-None-Match`, and `If-Modified-Since`, filters response headers, disables cookies, and
validates redirects to reduce SSRF risk. `Download` streams to a temporary file and atomically replaces the target.

For idempotent App API JSON reads only, the first HTTP 429 is retried once when its `Retry-After` is a valid
seconds value or HTTP date. The wait observes the caller context. Invalid or missing headers, a second 429, and all
other errors remain the original typed error; mutations and resource downloads are never replayed. The optional
`info` logger records only the retry attempt and parsed wait duration, never a URL, header value, credential, or
response body.

## Errors

Public failures may be `*pixiv.Error`:

```go
var pixivErr *pixiv.Error
if errors.As(err, &pixivErr) {
    switch pixivErr.Code {
    case pixiv.CodeArtworkUnavailable:
        // A deleted/private/region- or permission-limited item may be skipped.
    case pixiv.CodeRateLimited:
        // Schedule according to the caller's policy.
    }
}
if errors.Is(err, pixiv.ErrUnauthorized) { /* re-authenticate */ }
```

Stable codes include `invalid_argument`, `artwork_unavailable`, `unauthorized`, `forbidden`, `unsupported`,
`rate_limited`, `upstream_error`, `upstream_unavailable`, and `malformed_upstream_response`. Errors carry a stable
operation, backend, retryable flag, status, and validated IDs; they never include tokens, cookies, full URLs,
headers, or upstream response bodies.

| Call and failure stage | Result | `Operation` | `Backend` |
| --- | --- | --- | --- |
| Authenticated `IllustDetail` App detail/pages | `nil` | `OperationIllustDetail` | `BackendAppAPI` |
| Authenticated `IllustPages` App detail/pages | `nil` | `OperationIllustPages` | `BackendAppAPI` |
| Anonymous `IllustDetail` Web pages | `nil` | `OperationIllustPages` | `BackendWebAPI` |
| Anonymous `IllustDetail` Web detail | `nil` | `OperationIllustDetail` | `BackendWebAPI` |
| Authenticated `UgoiraMetadata` App metadata | `nil` | `OperationUgoiraMetadata` | `BackendAppAPI` |
| Anonymous `UgoiraMetadata` Web metadata | `nil` | `OperationUgoiraMetadata` | `BackendWebAPI` |

For example, an App 403 during authenticated page retrieval becomes `CodeForbidden`, `BackendAppAPI`,
`OperationIllustPages`, and `UpstreamStatus=403`; it does not continue to Web.

Transport failures with `upstream_unavailable` additionally expose a safe `Error.TransportKind`: `dns`, `tls`,
`proxy`, `connection_refused`, `connection_reset`, `timeout`, or `unknown`. Classification uses typed/wrapped Go causes, not
error text. `context.Canceled` and `context.DeadlineExceeded` remain `errors.Is` signals and have no transport kind.

Local account/configuration `invalid_argument` failures may expose `Error.LocalStateKind`: `auth_malformed`,
`config_malformed`, `permission_denied`, `not_found`, `invalid_proxy`, `account_mismatch`, `unavailable`, or
`unknown`. `errors.Unwrap` and `Error()` remain redacted and never expose filesystem/parser errors, paths, local
contents, or proxy userinfo. Missing optional `auth.json` or `config.toml` remains a valid empty state.

When `RestoreAuthBundle` fails while saving the merged auth store, its error additionally exposes
`Error.LocalWriteCommitOutcome`. `not_committed` means the replacement did not happen; `committed` means replacement
happened but a subsequent durability or cleanup step failed, so callers must reload the target; `unknown` means
recovery could not establish the target state and manual inspection is required. Callers must not report
`committed` or `unknown` as a successful rollback.

## Caller responsibilities

The caller adapter owns collection mode, budgets, filters, cursor storage, database transactions, scheduling,
retries, and its external HTTP API. `atri-setu-api` random selection, moderation, gallery storage, and image proxy
policy are not SDK features; an integration may build them from normalized models and resource streams.

See [ADR 0009](../maintainers/adr/0009-public-pixiv-sdk-and-caller-adapter.md) and
[ADR 0010](../maintainers/adr/0010-http-client-timeout-and-context.md) for the complete boundary decisions.
