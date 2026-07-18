# Pixiv Go SDK

English | [简体中文](../zh-CN/sdk.md) | [Documentation index](../index.md)

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
| Works and recommendations | `SearchIllust`, `SearchIllustOptions`, `IllustDetail`, `IllustPages`, `IllustRelated`, `IllustRanking`, `IllustRecommended`, `MangaRecommended`, `NovelRecommended`, `UserRecommended`, `FollowingIllusts`, `TrendingTagsIllust`, `UgoiraMetadata`. |
| Users | `SearchUser`, `UserDetail`, `UserArtworks`, `UserBookmarks`, `UserFollowing`, `CurrentUserID`. |
| Writes | `AddBookmark`, `RemoveBookmark`, `FollowUser`, `UnfollowUser`. |
| Accounts/configuration | `ImportAccount`, `ListAccounts`, `SelectAccount`, `RemoveAccount`, `ExportAccountRefreshToken`, `ExportAuthBundle`, `RestoreAuthBundle`, `CheckAccount`, `CheckRefreshToken`, `Refresh`, `RefreshAccount`, `GetConfig`, `SetConfig`, `UnsetConfig`; bundle codec functions are package-level. |
| Login | `StartLogin`, `CompleteLogin`, `BuildLoginAuthorizationURL`; the SDK does not start a browser, loopback server, or TTY. |
| Resources | `ParseResourceRef`, `OpenResource`, `Download`. |

Request methods use named request types such as `SearchIllustRequest`, `SearchIllustOptionsRequest`,
`UserArtworksRequest`, `UserBookmarksRequest`, `UserFollowingRequest`, `AddBookmarkRequest`, and
`FollowUserRequest`. Result models such as `IllustListResult`, `SearchIllustOptionsResult`, `UserListResult`,
`IllustDetail`, and `UserDetailResult` all live in the top-level `pixiv` package.

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
duplicate or non-positive UIDs, empty refresh tokens, and a default UID that is absent from the account list. Both
functions return redacted typed errors and never include bundle contents.

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

Zero enum values normalize to `all`; `Tool` is trimmed. Unknown values return `invalid_argument` before any
upstream request. The authenticated adapter maps resolution, aspect ratio, tool, content type, and AI exclusion to
App server parameters. Rating and AI-only filtering use normalized fields from the current App batch.
`Illust.Tools []string` preserves upstream order and values and is unrelated to bookmark-count filtering.

`SearchIllustOptions(ctx, SearchIllustOptionsRequest{Word: word})` requires a non-empty word and App
authentication. It returns `SearchIllustOptionsResult{Tools []string}` in upstream order; a missing list becomes a
non-nil empty slice. Premium bookmark tiers are not exposed.

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

Cursors are versioned and bound to the operation and complete normalized query. A search cursor also binds
`Rating`, `ContentType`, `AIMode`, `AspectRatio`, `Resolution`, and `Tool`; changing a filter and reusing the old
cursor returns `invalid_argument`. Never parse, edit, reuse across requests, or replace a cursor with an upstream
offset/page. The SDK does not accept `page`; CLI/MCP translate logical pages and limits at their boundaries.

## Routing

With a refresh token, illustration search uses only App API. Authentication, network, and server failures never
fall back to Web automatically. Without a token, `NewClient` can use the anonymous Web allowlist when
`WebFallbackEnabled=true`; `OpenDefault` reads local `web_fallback_enabled` for each snapshot. Anonymous search
uses only filters Web can express reliably. `r18`, `r18g`, and `mature` fail with `unauthorized` before networking;
they are never disguised as empty results. Anonymous `SearchIllustOptions` returns `unsupported`. The SDK never
reads/injects cookies or converts a refresh token into a Web session.

`IllustDetail` pages and original ugoira metadata use Web for explicit enrichment, not failure fallback, and the
result is atomic:

- Authenticated `IllustDetail` reads App detail and then Web pages. A Web failure returns `nil` and a typed error,
  even if App supplied `MetaPages`; no unlabeled partial result is returned.
- App ugoira metadata contains only the medium zip. If Web cannot provide the original, `UgoiraMetadata` returns
  `nil` and a typed error instead of silently lowering quality.
- Anonymous `IllustDetail` reads Web detail and pages; failure at either stage returns no partial result.

Web enrichment receives no App bearer token or cookie. App `MetaPages` is representable by the wire model and
mapper, but that does not guarantee completeness for every work. See
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
| Authenticated `IllustDetail` App detail | `nil` | `OperationIllustDetail` | `BackendAppAPI` |
| Authenticated/anonymous `IllustDetail` Web pages | `nil` | `OperationIllustPages` | `BackendWebAPI` |
| Anonymous `IllustDetail` Web detail | `nil` | `OperationIllustDetail` | `BackendWebAPI` |
| `UgoiraMetadata` Web enrichment | `nil` | `OperationUgoiraMetadata` | `BackendWebAPI` |

For example, a 403 login wall during page enrichment becomes `CodeForbidden`, `BackendWebAPI`,
`OperationIllustPages`, and `UpstreamStatus=403`. An App detail failure does not continue to Web.

Transport failures with `upstream_unavailable` additionally expose a safe `Error.TransportKind`: `dns`, `tls`,
`proxy`, `connection_refused`, `connection_reset`, or `unknown`. Classification uses typed/wrapped Go causes, not
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
