# Pixiv SDK (v1)

English | [简体中文](../zh-CN/sdk.md) | [Documentation index](../index.md)

The v1 SDK exposes three public packages:

- `github.com/FlanChanXwO/pixiv-cli/sdk` — protocol-agnostic primitives shared by both products: paginated pages, opaque cursors, classified errors, and the resource contract.
- `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv` — the Pixiv App API client, models, URL references, and mutations.
- `github.com/FlanChanXwO/pixiv-cli/sdk/fanbox` — the Pixiv FANBOX client, models, and URL resolution.

All exported declarations carry English GoDoc; the package source is the canonical API summary.

## Quickstart

A complete Pixiv flow: authenticate, search, fetch detail, and save an image.

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

	// 1. Authenticate via OAuth rotation. Persist the rotated refresh token
	//    before issuing content requests; the client never refreshes itself.
	client, creds, err := pixiv.Open(ctx, os.Getenv("PIXIV_REFRESH_TOKEN"))
	if err != nil {
		panic(err)
	}
	_ = creds // persist creds.RefreshToken() to durable storage

	// 2. Search artworks and iterate the typed page cursor.
	page, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "miku"})
	if err != nil {
		panic(err)
	}
	if len(page.Items) == 0 {
		return
	}
	first := page.Items[0]

	// 3. Fetch artwork pages (image resources).
	pages, err := client.ArtworkPages(ctx, pixiv.ArtworkPagesRequest{ArtworkID: first.ID})
	if err != nil {
		panic(err)
	}

	// 4. Save the first image through the SDK-validated resource path.
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

A FANBOX flow: open with a session, list supporting posts, resolve one post's resource.

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
_ = post // post.Body blocks carry image/file assets with their Resource refs
```

## Authentication

Pixiv is App-only. Every content operation requires a valid access token.

```go
client, creds, err := pixiv.Open(ctx, refreshToken) // OAuth rotation
// persist creds.RefreshToken() before issuing content requests

client, err := pixiv.New(accessToken) // static token, no network I/O
```

`Open` returns a `Client` holding only the access token; it never refreshes on
its own. When the token expires, operations return `CredentialsExpired`.
The OAuth response must contain a positive account user ID; otherwise `Open`
returns `MalformedUpstreamResponse` without a client or credentials. There is
no anonymous or Web fallback.

### Programmatic browser login

`BeginLogin` creates a self-contained, one-shot PKCE session. It does not open a
browser or start a loopback listener; the caller opens `AuthorizationURL()` and
delivers the resulting callback or bare code to `Complete`.

```go
session, err := pixiv.BeginLogin(pixiv.LoginOptions{HTTPClient: httpClient})
if err != nil { /* handle error */ }
if !session.AcceptsCallbackURL(callbackURL) {
    // Reject callbacks before consuming the one-shot session.
}
credentials, err := session.Complete(ctx, callbackURL)
```

`AcceptsCallbackURL` is non-consuming and performs no network I/O. Official
HTTPS callbacks require the session `state`; the supported `pixiv://account/login`
callback may omit `state`, but a supplied value must match. The exact-origin
helpers `IsOfficialOAuthCallbackURL` and `IsOfficialOAuthStartURL` validate
browser input without contacting Pixiv. The session never exposes its verifier,
state, authorization code, or callback URL through formatting.

FANBOX is authenticated with an explicit `FANBOXSESSID` value:

```go
client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: session})
```

> [!IMPORTANT]
> Pixiv refresh tokens and FANBOX sessions are independent and never convert.

FANBOX connection options are explicit and optional:

```go
client, err := fanbox.OpenWith(credentials, fanbox.Options{
    ProxyURL:  "https://proxy.example:8443", // native HTTP(S) CONNECT only
    UserAgent: "my-native-agent/1.0",          // native header only
    FlareSolverr: &fanbox.FlareSolverrOptions{
        URL:      "http://127.0.0.1:8191",
        ProxyURL: "socks5://solver-upstream.example:1080",
    },
})
```

The production native transport uses tls-client's Chrome 146 TLS profile. An
empty `UserAgent` uses the built-in Firefox 148 HTTP header baseline; a custom value
is disabled when the option is nil and is consulted only after a strict native
Cloudflare challenge. Its service URL and upstream proxy are independent from
the native proxy. The public constructor performs no network I/O.

## Pagination

List operations return `sdk.Page[T]` with an opaque `Cursor`:

```go
page, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "miku"})
for {
    for _, artwork := range page.Items { /* ... */ }
    if page.Next.IsZero() { break }   // stop when no cursor remains
    request.Cursor = page.Next
    page, err = client.SearchArtworks(ctx, request)
}
```

> [!NOTE]
> Cursors are bound to the product, operation, binding version, and query digest.
> Reusing one with a different query returns `InvalidCursor`.

For identity-scoped operations, a client created with `pixiv.New` has no
verified account ID. Its continuation cursor is ephemeral and carries a
non-secret binding for that Client instance; the same Client may continue it,
while another Client or process receives `InvalidCursor`. A client opened
through `pixiv.Open` binds the cursor to the verified account identity instead.

## Pixiv read operations

| Operation | Input highlights | Returns | Common errors |
| --- | --- | --- | --- |
| `SearchArtworks` | word, target, sort, date bounds, type, AI mode, aspect ratio, resolution, tool, bookmark bounds | `Page[Artwork]` | `InvalidArgument` (unknown enum, bad dates, bad bookmark range) |
| `SearchNovels` | word, target, sort, duration | `Page[Novel]` | `InvalidArgument` |
| `SearchUsers` | word | `Page[User]` | `InvalidArgument` |
| `ArtworkRanking` | mode (default `day`), optional `YYYY-MM-DD` | `Page[Artwork]` | `InvalidArgument` |
| `Artwork` / `Novel` / `User` | positive typed ID | detail record | `NotFound`, `InvalidArgument` |
| `ArtworkSeries` / `NovelSeries` | positive series ID, cursor | series page (novel also returns metadata) | `InvalidCursor` |
| `ArtworkComments` / `NovelComments` | positive ID, cursor | `CommentPage` | `NotFound` |
| `UserArtworkBookmarks` / `UserArtworkBookmarkTags` / `UserNovelBookmarks` | `UserID`, `Restrict`, `tag`, cursor | typed page | `InvalidArgument`, `InvalidCursor` |

Key semantics:

- `CurrentUser` reads the authenticated account through `/v1/user/detail` with the verified positive account user ID and the Android App API filter; the removed `/v1/user/me` route is not used.
- `SearchAIModeOnly` is a local result-batch filter over `Artwork.AIType == 2`.
  Its mode is included in the cursor binding, so a continuation cannot be reused
  for another AI mode.
- Comment totals and access-control metadata remain nil unless the upstream
  response supplied them. A successful empty list is a non-nil empty `Items`
  slice, not an invented error or total.
- `ArtworkBookmark` represents an absent bookmark with an empty `Restrict` and
  empty tags; `AddBookmark` validates visibility and never treats an unsupported
  value as a server default.
- `BookmarkMin` and `BookmarkMax` are optional, inclusive, non-negative App API
  candidate bounds. The SDK validates and forwards them as
  `bookmark_num_min`/`bookmark_num_max` but performs no Premium preflight, claims
  no global completeness, and never silently falls back to another candidate
  strategy. Application-level search may recheck `Artwork.TotalBookmarks` and
  must report its resolved strategy and completeness separately.

## Errors

All failures are `*sdk.Error` with a stable `Reason`:

```text
invalid_argument, invalid_cursor, unauthorized, credentials_expired, forbidden,
not_found, content_unavailable, challenge_required, rate_limited, upstream_error,
upstream_unavailable, malformed_upstream_response, resource_forbidden,
local_state_error, removed_setting
```

`errors.Is`/`errors.As` work, and `context.Canceled`/`DeadlineExceeded` are
preserved. The error chain never contains URLs, headers, tokens, cookies, or
config content.

```go
if errors.Is(err, sdk.Unauthorized{}) {
    // re-authenticate
} else if sdk.ReasonOf(err) == sdk.RateLimited {
    // back off using RetryAdvice
}
```

## Resources

Programmatic SDK callers receive first-party media through `sdk.Resource` with
two runtime paths:

- `Resource.URL` + `Resource.RequestHeaders` — stream directly or proxy without
  buffering to disk.
- `Resource.Ref` — hand back to `OpenResource`/`SaveResource` for SDK-validated
  reads (scheme/host/path revalidation and redirect-safe handling).

```go
// Stream directly without buffering to disk.
page, _ := client.ArtworkPages(ctx, pixiv.ArtworkPagesRequest{ArtworkID: id})
image := page[0].Image.Resource
resp, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: image.Ref})
if err != nil { /* handle */ }
defer resp.Body.Close()
// read from resp.Body using image.URL + image.RequestHeaders as needed
```

```go
// Save through the SDK-validated path (revalidates URL/redirects, atomic write).
_, err := client.SaveResource(ctx, sdk.SaveOptions{
    Ref:  image.Ref,
    Dest: "./out.png",
})
```

> [!IMPORTANT]
> A `Resource` never stores a cookie; a bound FANBOX client may use its session
> only for the authenticated FANBOX API and `downloads.fanbox.cc` request policy,
> never for Pixiv/CDN or third-party hosts. `Resource` never carries tokens or
> cookies; `RequiresCredentials` reports when a resource still needs product
> credentials invisible to the caller.

### Runtime models and output DTOs

Runtime product models are intentionally separate from values that cross a CLI
or MCP JSON boundary. `sdk.Resource` may contain the current usable `URL`,
forwarding `RequestHeaders`, and `ExpiresAt` for an in-process streaming
operation; these fields are never part of an output DTO.

Use the explicit field-by-field converters when serializing a result:
`pixiv.ToArtworkDTO`, `pixiv.ToNovelDTO`, `pixiv.ToUserDTO`,
`pixiv.ToUserDetailDTO`, `pixiv.ToUserPreviewDTO`, `pixiv.ToCommentDTO`,
`pixiv.ToNovelContentDTO`, `pixiv.ToUgoiraMetadataDTO`, and their related
Pixiv converters; or the corresponding `fanbox.To*DTO` converters for creators,
posts, blocks, assets, users, and tags. `sdk.ToResourceDTO` emits only the
opaque `ref` and optional `requires_credentials` metadata. The CLI and MCP
servers encode only these DTOs, pipeline `Record` values, and typed envelopes;
they never reflect over or JSON-marshal runtime product models.

For Pixiv, `Resource.Ref` contains only the resource kind, stable ID, page, and
optional variant. It never embeds the current or signed media URL. The SDK can
reuse the current locator held by the client, or re-fetch the corresponding
artwork, novel, user, ugoira, or novel-content metadata before opening it; every
resolved URL and redirect is allowlisted again. `SaveResource` writes through an
atomic destination and reports the response `Content-Length` in
`SaveProgress.Total` when upstream supplies it. Resource requests use an
explicit header allowlist and never send the caller's Cookie jar.

For FANBOX, `Resource.Ref` contains only stable identity (the resource kind,
the owning creator or post, and the attachment id) and never embeds the
currently-usable or signed media URL, so locator rotation never changes the
cache key and a stored ref can be reopened across sessions. `OpenResource` and
`SaveResource` reuse the in-session locator when present, otherwise they
re-resolve a fresh, allowlisted locator by re-fetching the owning creator or
post and locating the attachment by its stable id. The session cookie is sent
only on the credentialed `downloads.fanbox.cc` host; public CDN and third-party
hosts never receive it. `RequiresCredentials` reports when a locator still
needs the session.

## URL references

`pixiv.ParseURL` and `fanbox.ResolveURL` turn page URLs into typed references
without network I/O, and `Reference.CanonicalURL` returns the tracking-free
canonical form.

```go
ref, err := pixiv.ParseURL(pageURL)
if err != nil { /* handle */ }
canonical := ref.CanonicalURL()
```

### Optional DTO fields

Output DTOs omit fields that the upstream response does not provide instead of
emitting `null` or empty values: for example `ArtworkDTO` omits `updated_at`,
`tools` and `pages` when the SDK has no update time, no tool list, or no page
list (pages are populated on the detail path only). Consumers must treat a
missing key the same as an unknown value; the JSON schema published for MCP
tools marks these fields optional accordingly.

## FANBOX

`sdk/fanbox` provides creator profiles, posts, tags, home and supporting feeds,
URL resolution, and the shared resource contract. The verified native routes
use the `api.fanbox.cc` root (`post.info`, `post.listHome`,
`post.listSupporting`, `post.listTagged`, and `tag.getFeatured`); creator
pagination follows the server-provided `pageUrls`. Post bodies are structured
blocks; image and file blocks are joined with their resource indexes, including
responses that provide attachments only through `imageMap` or `fileMap`.
Third-party embeds keep only their canonical link. Restricted posts carry a
summary with a nil body.

The Home, Supporting, and Creators feeds are identity-scoped: their continuation
cursors bind to the verified FANBOX account id (a non-secret value resolved once
per client via the session identity), so a cursor minted under one account
cannot be replayed against another account's feed and returns `InvalidCursor`.
CreatorPosts and TaggedPosts are public-scoped and carry no account binding.
As with Pixiv, cursors are also bound to the product, operation, binding
version, and query digest.

## Migrating from v0

See the [migration guide](v1.0.0-migration.md) for the v0 `pixiv` to v1
`sdk/pixiv` transition.
