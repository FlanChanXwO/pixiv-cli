# Pixiv SDK (v1)

English | [简体中文](../zh-CN/sdk.md) | [日本語](../ja/sdk.md) | [Documentation index](../index.md)

The v1 SDK exposes three public packages:

- `github.com/FlanChanXwO/pixiv-cli/sdk` — protocol-agnostic primitives shared by
  both products: paginated pages, opaque cursors, classified errors, and the
  resource contract.
- `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv` — the Pixiv App API client,
  models, URL references, and mutations.
- `github.com/FlanChanXwO/pixiv-cli/sdk/fanbox` — the Pixiv FANBOX client,
  models, and URL resolution.

All exported declarations carry English GoDoc; the package source is the
canonical API summary.

## Authentication

Pixiv is App-only. Every content operation requires a valid access token.

```go
client, creds, err := pixiv.Open(ctx, refreshToken) // OAuth rotation
// persist creds.RefreshToken() before issuing content requests

client, err := pixiv.New(accessToken) // static token, no network I/O
```

`Open` returns a `Client` holding only the access token; it never refreshes on
its own. When the token expires, operations return `CodeCredentialsExpired`.
There is no anonymous or Web fallback.

FANBOX is authenticated with an explicit `FANBOXSESSID` value:

```go
client, err := fanbox.Open(fanbox.SessionCredentials{FANBOXSESSID: session})
```

Pixiv refresh tokens and FANBOX sessions are independent and never convert.

## Pagination

List operations return `sdk.Page[T]` with an opaque `Cursor`:

```go
page, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "miku"})
for {
    for _, artwork := range page.Items { /* ... */ }
    if page.Next.IsZero() { break }
    request.Cursor = page.Next
    page, err = client.SearchArtworks(ctx, request)
}
```

Cursors are bound to the product, operation, binding version, and query digest;
reusing one with a different query returns `CodeInvalidCursor`.

## Errors

All failures are `*sdk.Error` with a stable `Code`:

```text
invalid_argument, invalid_cursor, unauthorized, credentials_expired, forbidden,
not_found, content_unavailable, challenge_required, rate_limited, upstream_error,
upstream_unavailable, malformed_upstream_response, resource_forbidden,
local_state_error, removed_setting
```

`errors.Is`/`errors.As` work, and `context.Canceled`/`DeadlineExceeded` are
preserved. The error chain never contains URLs, headers, tokens, cookies, or
config content.

## Resources

First-party media is exposed through `sdk.Resource` with two parallel paths:

- `Resource.URL` + `Resource.RequestHeaders` — stream directly or proxy without
  buffering to disk.
- `Resource.Ref` — hand back to `OpenResource`/`SaveResource` for SDK-validated
  reads (scheme/host/path revalidation, cookie-free, redirect-safe).

```go
page, _ := client.ArtworkPages(ctx, pixiv.ArtworkPagesRequest{ArtworkID: id})
image := page[0].Image.Resource
resp, err := client.OpenResource(ctx, sdk.OpenResourceRequest{Ref: image.Ref})
```

`Resource` never carries tokens or cookies; `RequiresCredentials` reports when a
resource still needs product credentials invisible to the caller.

## URL references

`pixiv.ParseURL` and `fanbox.ResolveURL` turn page URLs into typed references
without network I/O, and `Reference.CanonicalURL` returns the tracking-free
canonical form.

## FANBOX

`sdk/fanbox` provides creator profiles, posts, tags, home and supporting feeds,
URL resolution, and the shared resource contract. Post bodies are structured
blocks; third-party embeds keep only their canonical link. Restricted posts
carry a summary with a nil body.

## Migrating from v0

See the [migration guide](v1.0.0-migration.md) for the v0 `pixiv` to v1
`sdk/pixiv` transition.
