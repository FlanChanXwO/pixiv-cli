# Go SDK

English | [简体中文](../zh-CN/sdk.md) | [Documentation index](../index.md)

Use the SDK from module `github.com/FlanChanXwO/pixiv-cli`. It provides account-bound API objects, resumable lists and media operations. Your application supplies credentials and connection settings and owns account selection, configuration, output and credential persistence.

| Import suffix | Entry and purpose |
| --- | --- |
| `sdk` | `Page[T]`, opaque `Cursor`, classified `Error` and `Reason` |
| `sdk/pixiv` | `New(store, options)` for authenticated Pixiv App API operations; `BeginLogin(options)` for PKCE |
| `sdk/fanbox` | `New(sessionID, options)` for read-only FANBOX operations |
| `sdk/saucenao` | `Search(ctx, options)` from image bytes |
| `sdk/ascii2d` | `Search(ctx, options)` from image bytes, color/BOVW matching |
| `sdk/download` | `New(options).Save(ctx, pages...)` for selected Pixiv artwork pages |

## Quickstart

This function searches one upstream batch and saves its artwork pages. Supply an App OAuth refresh token through your application's private input path. The in-memory store retains rotated tokens only for this process; use a persistent `CredentialStore` for a lasting login.

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

`pixiv.New` performs no network or store I/O. The first operation loads credentials and refreshes them when needed. `Client.Credentials(ctx)` explicitly prepares authentication without a content request. Browser cookies such as `PHPSESSID` and `refresh_token=...` are rejected as App refresh tokens.

## References and objects

`client.Artwork(id)`, `User(id)`, `Novel(id)`, `ArtworkSeries(id)` and `NovelSeries(id)` construct bound references without I/O. Artwork, user and novel references load metadata with `Get(ctx)`. Lists return the same bound object types, so list results support operations directly. For example, an artwork embeds `*ArtworkRef`, exposes `Author *User`, and has `Pages []ArtworkPage`; each page links back through `Artwork` and carries its original `Image`.

| Starting point | Common operations |
| --- | --- |
| `ArtworkRef` / `Artwork` | `Get`, `Comments`, `PostComment`, `Bookmark`, `AddBookmark`, `RemoveBookmark`, `UgoiraMetadata` |
| `UserRef` / `User` | `Get`, `Profile`, `Artworks`, `Novels`, `ArtworkBookmarks`, `NovelBookmarks`, following/follower lists and follow mutations |
| `NovelRef` / `Novel` | `Get`, `Comments`, `PostComment`, bookmark operations, `Content`, `ParseContent` |
| `CommentRef` / `Comment` | `Reply`, `Delete`; reply targets come from the owning artwork/novel or a comment list |
| `ArtworkSeriesRef` | `Artworks(ctx, options)` lists complete works |
| `NovelSeriesRef` | `Get(ctx, options)` returns metadata and a `Novels` page |
| `Client` | artwork/novel/user search, rankings, recommendations, following/latest/MyPixiv feeds, `Stamps` |

`User` values in lists may be summaries; `Profile(ctx)` explicitly retrieves detailed profile data. `PostComment(ctx, CommentOptions{Text: "..."})` returns a reference with the upstream-assigned ID; `StampID` also permits stamp-only comments. Mutations are sent once, with no read-back. An injected transport must preserve this behavior and avoid retrying POST requests.

`NovelRef.Content(ctx)` reports `ContentUnavailable` under the App API contract. If your application already has a novel webview document, `ParseContent(rawHTML)` parses it locally and binds its media to the client.

JSON preserves metadata while dropping client bindings and page/artwork back-references. A decoded reference or object needs a fresh reference from a client before further operations, for example `client.Artwork(decoded.ID).Get(ctx)`. `client.ParseURL(raw)` resolves supported Pixiv page URLs into bound references without I/O; their `CanonicalURL()` supplies the canonical page URL. FANBOX has its own `Client.ParseURL`.

## Authentication

Implement `pixiv.CredentialStore` for one account:

- `Load(ctx)` returns the latest saved `Credentials`, with a positive `Revision`, an established `UserID` when known, and tokens. Access-token state may be omitted; the client caches it by revision.
- `CompareAndSwap(ctx, expectedRevision, replacement)` atomically saves only the matching revision. The SDK supplies `replacement.Revision == expectedRevision+1`. Return `false, nil` for a conflict and `true, nil` after persistence completes. Preserve the established account identity.
- Release storage locks before either method returns. OAuth runs between the two calls. The SDK adopts newer saved credentials after conflicts and propagates persistence failures before using rotated tokens for content.

`pixiv.NewCredentials(refreshToken)` creates a revision-1 seed. `NewMemoryCredentials` validates it and provides a concurrency-safe in-memory store. Persistent stores restore raw tokens with `Credentials.SetTokens`; token fields redact themselves in fmt/slog and are omitted from JSON. Read their `Value()` only for private persistence or explicit credential transfer.

### Programmatic browser login

The application opens the authorization URL and obtains the callback URL or code through its own UI. Persist the credentials returned by `Complete` before issuing content requests.

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

`AcceptsCallbackURL(raw)` validates a callback without consuming the session. A valid completion input consumes the session before network I/O, including on failure or cancellation; begin a new login to retry. Invalid input leaves the session available.

## Connections and logging

All network-capable operations take `ctx` first; use that context for cancellation and deadlines. Constructors, reference creation, URL parsing and local HTML parsing perform no network I/O.

Pixiv `Options` accepts `HTTPClient`, `AcceptLanguage`, `RequestInterval`, additional `ResourceHosts` and `OnArtwork`. A nil HTTP client creates a dedicated client with no fixed whole-request timeout; `CloseIdleConnections()` releases its idle sockets. An injected client remains caller-owned, including its proxy, transport and close policy.

`OnArtwork`, when set, receives each successfully mapped artwork with the operation context before the operation returns. This includes works later filtered locally, recommendation and trending-tag samples, and works mapped before a later batch error. Keep it fast and safe for concurrent calls. `BeginLogin` ignores it.

FANBOX and ascii2d accept explicit proxy, User-Agent and optional FlareSolverr settings. Their default native transport uses a browser TLS profile; an injected `HTTPClient` needs an explicit `Transport`, and the product manages its own cookie/redirect policy. FANBOX `Close(ctx)` cancels and joins solver work; finish native requests first. FlareSolverr control is direct, with its browser proxy set separately by `FlareSolverrProxyURL`; only challenge recovery uses it.

Every SDK package uses the current `slog` default logger. Configure it once in your application's startup, for example with imports `log/slog` and `os`:

```go
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
	Level: slog.LevelDebug,
})))
```

Events use controlled request/decision attributes. Routine pacing and retries use Debug; rate-limit waits may use Warn. SDK errors preserve causes for inspection, so apply your own privacy policy before logging arbitrary error chains. CLI/MCP install their stderr handler at startup and restore the previous default on cleanup.

## Pagination

Lists return `sdk.Page[T]` with `Items`, `Cursor` and `Next(ctx)`. A live page retains its account/query binding. A zero cursor marks the last batch; `Next` then returns an empty page without a request. Persist the cursor after consuming a complete batch:

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

Here an empty `saved` starts a new search. Store completion separately: an empty saved cursor also marks an exhausted list, so check that state before invoking `Search` again. On resume, restore every original query option and the same account. Mismatches return `InvalidArgument` before a content request; authentication preparation may still occur. Cursors encode live continuation, not a frozen result snapshot.

JSON-decoded pages retain items and cursor but lose the `Next` binding. Resume a nonterminal decoded page through the original list operation with `sdk.ParseCursor(saved)` and its `Cursor` option. Cursor JSON/text is an opaque string; preserve it unchanged.

### Resuming within an artwork search batch

Call `client.CheckpointSearchArtworks(ctx, word, options, consumed)` with the options used to obtain the current batch, including its **incoming** `Cursor`. `consumed` counts entries after SDK AI filtering and before your application's own filters. Persist the returned cursor and resume through `SearchArtworks` with identical options. Checkpoint creation prepares credentials and may refresh OAuth. Resume refetches the live batch and skips the consumed prefix.

## Resources and downloads

Pixiv `Image` exposes `URL`, `Width`, `Height`, `Quality(...)`, `Open(ctx)`, `OpenResponse(ctx)` and `Save(ctx, path)`. `client.Image(rawURL)` binds an allowed CDN URL. `Open` requires HTTP 200 and returns an `io.ReadCloser`; always close it. `OpenResponse` exposes status, content type/length and body for callers that need to interpret HTTP status themselves. `Save` atomically replaces the destination after a complete successful read; its parent directory must already exist.

`sdk/download` saves only the supplied `pixiv.ArtworkPage` values, in order. Select pages by slicing `artwork.Pages` after checking bounds. Options control root directory, filename/directory templates, static `Quality` and GIF/APNG `UgoiraFormat`. Template fields are `{id}`, `{title}`, `{author}`, `{author_id}`, `{date}`, `{tags}` and zero-based `{num}`. Ugoira is represented by one page and requires original quality.

`Save` returns both a `Report` and an error: `Files` retains published files, `Warnings` retains diagnostics, and `Failures` retains per-page causes. Page failures allow later pages to proceed; cancellation stops the batch with partial results. Inspect the report even when the error is non-nil. FANBOX media uses its own `Image.Save` / `File.Save` operations.

## FANBOX

Pass only the opaque `FANBOXSESSID` value to `fanbox.New`. Construction is offline; `CurrentUser(ctx)` or `ValidateSession(ctx)` explicitly validates it. The caller owns session acquisition and storage.

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

`client.Creator(id).Get(ctx)` loads a creator; `CreatorRef.Posts(ctx, PostsRequest{...})`, `client.Home` and `client.Supporting` return account/query-bound pages. `Post.Body` is nil when the upstream withholds the body. Available bodies expose text, ordered blocks, images, files and complete upstream `Payload`, including unknown fields and signed media locators. Choose the fields your output should publish.

Bound `Image` / `File` values provide `Open(ctx, OpenRequest{})` and atomic `Save(ctx, path)`. `OpenRequest` supports GET/HEAD and `Range`, `IfNoneMatch`, `IfModifiedSince`, `IfRange`. Inspect `MediaResponse.StatusCode`, representation headers and `Filename`, and close `Body`; HEAD/304 have empty bodies. Treat upstream filenames as untrusted when choosing a local path. `Save` requires HTTP 200 and creates parent directories. Session cookies go only to validated FANBOX API/home identity requests and `downloads.fanbox.cc`; CDN reads keep them out.

## Reverse image search

Load the source yourself and pass image bytes to the independent provider packages. These operations need no Pixiv client or account.

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

SauceNAO returns ordered `Matches` and optional `Quota`. ascii2d accepts JPEG/PNG/WEBP up to `ascii2d.MaxImageBytes` (10 MiB); its default mode is `Color`. `Both` uploads once and preserves independent `Color`/`BOVW` page outcomes. Each requested page carries `Matches` and `Err`; `Search` also returns the combined error. Inspect partial results before handling that error. File/URL loading, provider combination and Pixiv enrichment belong to the caller.

## Errors and migration

Use `sdk.ReasonOf(err)`, `errors.As` or `errors.Is(err, &sdk.Error{Reason: sdk.NotFound})`. `sdk.Error` exposes `Reason`, `Operation`, controlled `Message`, `HTTPStatus`, optional `RetryAfter` and an unwrapped `Err`. Cancellation and persistence causes remain inspectable through the error chain.

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

| Reason | Meaning |
| --- | --- |
| `InvalidArgument` | invalid input, mismatched cursor or unbound object |
| `Unauthorized`, `CredentialsExpired` | rejected authentication or credentials needing renewal/login |
| `Forbidden`, `NotFound`, `ContentUnavailable` | access restriction, missing object or unavailable content |
| `ChallengeRequired`, `RateLimited` | unresolved browser challenge or upstream rate limit |
| `UpstreamError`, `UpstreamUnavailable`, `MalformedUpstreamResponse` | upstream failure, transport/unavailability or invalid response |
| `ResourceForbidden`, `LocalStateError` | rejected resource origin or credential/storage/local state failure |

`RetryAfter` is present only for a valid upstream value; callers choose any subsequent scheduling policy.

When migrating from the previous SDK, replace `Open`/`OpenWith`/`NewWith` with `pixiv.New(store, options)` or `fanbox.New(sessionID, options)`, move entity operations onto bound references/objects, and pass `ctx` to network calls. Replace constructor loggers with `slog.SetDefault`, and old error/resource wrappers with `sdk.Error` and product media objects. Move provider search to `sdk/saucenao` / `sdk/ascii2d` and batch saving to `sdk/download`. The module/import root stays unchanged. See [architecture](maintainers/architecture.md) for internal ownership and [authentication](authentication.md) for CLI-managed credentials.
