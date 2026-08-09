# Pixiv SDK (v1)

English | [简体中文](../zh-CN/sdk.md) | [Documentation index](../index.md)

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

Pixiv refresh tokens and FANBOX sessions are independent and never convert.

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
    if page.Next.IsZero() { break }
    request.Cursor = page.Next
    page, err = client.SearchArtworks(ctx, request)
}
```

Cursors are bound to the product, operation, binding version, and query digest;
reusing one with a different query returns `InvalidCursor`.

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

## Resources

First-party media is exposed through `sdk.Resource` with two parallel paths:

- `Resource.URL` + `Resource.RequestHeaders` — stream directly or proxy without
  buffering to disk.
- `Resource.Ref` — hand back to `OpenResource`/`SaveResource` for SDK-validated
  reads (scheme/host/path revalidation and redirect-safe handling). A
  `Resource` never stores a cookie; a bound FANBOX client may use its session
  only for the authenticated FANBOX API and `downloads.fanbox.cc` request
  policy, never for Pixiv/CDN or third-party hosts.

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
URL resolution, and the shared resource contract. The verified native routes
use the `api.fanbox.cc` root (`post.info`, `post.listHome`,
`post.listSupporting`, `post.listTagged`, and `tag.getFeatured`); creator
pagination follows the server-provided `pageUrls`. Post bodies are structured
blocks; image and file blocks are joined with their resource indexes, including
responses that provide attachments only through `imageMap` or `fileMap`.
Third-party embeds keep only their canonical link. Restricted posts carry a
summary with a nil body.

## Migrating from v0

See the [migration guide](v1.0.0-migration.md) for the v0 `pixiv` to v1
`sdk/pixiv` transition.
