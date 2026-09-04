# MCP tools

[简体中文](../zh-CN/mcp-tools.md) | English | [Documentation index](../index.md)

`pixiv mcp` starts the Pixiv stdio MCP server. MCP uses its configured runtime
credential selection; it does not accept CLI data-command account overrides.
The stdout stream is reserved for JSON-RPC.

## Errors, pagination, and output

Schema-invalid input is rejected as a JSON-RPC/tool input error before the SDK
operation is opened. A failure after handler execution preserves the tool's
structured result and sets `isError=true`; an entity read returns an empty
`records` collection, while a download returns its report shape. A normal empty
page is successful and is not converted into an error.

List tools accept `page` and `limit`:

- Omitting `limit` reads one upstream batch.
- A positive `limit` fills the requested logical page across upstream batches.
- `limit: 0` traverses the current upstream cursor until it ends.
- `page` is 1-based and requires a positive `limit`.
- Entity filters are applied before logical pagination and duplicate records are
  removed by their stable entity identity.

Opaque SDK cursors never leave the server. List results expose `pagination.page`,
`limit`, `returned`, and `has_more`; they may also expose `next_page` when another
logical page is available. `recommended(kind="all")` exposes independent
pagination objects for illustration, manga, novel, and user streams.

Records keep public entity fields and an opaque resource reference when one is
needed. They do not expose resolved/signed resource URLs, request headers,
Cookies, expiry metadata, access tokens, or other resource transport credentials.
Novel content blocks and comment/profile-image references follow the same rule.
Structured results use explicit DTOs and typed envelopes rather than runtime SDK
models. The separate FANBOX MCP server follows the same resource shape: a
first-party resource contains its opaque `ref` and optional
`requires_credentials`, never `url`, `request_headers`, or `expires_at`.

## Reverse image search

`reverse_search` is a Pixiv MCP tool with a closed input object:

```json
{"source":"/private/path/image.png","provider":"ascii2d-color"}
```

`source` is required and `provider` is optional. The provider enum is
`saucenao`, `ascii2d-color`, `ascii2d-bovw`, or `all`; omitting it uses the
MCP process's startup configuration, whose default is `saucenao`. The
`reverse_search_pixiv_only` configuration is also captured at startup and
controls whether non-Pixiv matches remain in `results`. A tool call cannot
change the proxy, API key, or other transport configuration.

The startup transport has three distinct network surfaces: the standard source/
SauceNAO client, the dedicated ascii2d browser client, and the FlareSolverr JSON
control client. `[reverse_search.network].proxy_url` selects the ascii2d proxy
when present (an explicit empty value selects direct access); the standard client
keeps the global proxy route. `[reverse_search.network].user_agent` applies only
to ascii2d. Chromium User-Agents receive matching `Sec-CH-UA`,
`Sec-CH-UA-Mobile`, and `Sec-CH-UA-Platform` hints, while non-Chromium
User-Agents omit those Chromium hints. `[reverse_search.flaresolverr].proxy_url`
is only the browser upstream proxy sent in `sessions.create`; solver control
traffic does not inherit either native route.

The source may be any readable regular local file or HTTP(S) URL. Under the
intentional trusted-local-client model, private, loopback, and link-local URL
targets are allowed, and the server may read private files. The server fetches
or opens the source once into a private snapshot, uploads it to the selected
third-party provider(s), and never returns the original source, temporary path,
request headers, cookies, API key, CSRF value, redirect `Location`, or upstream
response body. Run `pixiv mcp` only for MCP clients that are trusted to request
these local resources. SauceNAO/ascii2d processing and retention follow their
own policies; URL queries may be cached. ascii2d accepts JPEG, PNG, and WEBP
and applies its provider-specific 10 MB limit.

The structured output is always the closed envelope
`{input, providers, results, records, provider_errors, partial}`. `input`
contains only `kind` and `sha256`; `providers` is the fixed provider status
list; `results` keeps provider evidence and optional canonical Pixiv identity;
`provider_errors` contains only stable `provider`, `code`, and `message`;
`records` contains canonical `artwork` or `user` records. An artwork record is
deliberately generic because the search providers do not establish Pixiv's
artwork subtype, and the tool does not call artwork detail to guess it.
External-only results remain outside `records`.

When at least one provider succeeds and another fails, `partial=true` and the
tool result is successful (`isError=false`). A single-provider failure or an
all-provider failure preserves the envelope and sets `isError=true`; schema
errors are rejected before provider execution. Cancellation remains a full
request cancellation, not a partial success.

The image is uploaded natively to ascii2d's `/search/file` multipart endpoint;
FlareSolverr receives only JSON challenge-recovery requests and never receives
the image upload. Its solver state is process/client-scoped and is not persisted
to disk. ascii2d's 10 MB provider-specific limit is not a reverse-search-wide
1 MiB compressed-upload rule; `gzip, deflate, br` is response negotiation.

The stable reverse-search error-code vocabulary is `unknown`, `invalid_request`,
`invalid_source`, `source_not_regular_file`, `source_read_failed`,
`source_http_status`, `snapshot_failed`, `source_loader_not_configured`,
`provider_not_configured`, `missing_credential`, `malformed_upstream_response`,
`upstream_http_status`, `provider_failed`, `all_providers_failed`,
`challenge_required`, `solver_unavailable`, `solver_failed`, and
`malformed_solver_response`. Provider failure causes are sanitized before they
enter `provider_errors`; only the reviewed stable code and safe message are
published.

## Entity filters and bookmark-count search

Only the typed filters below are input fields. The former top-level expression
`filter` input is not published because it was not connected to the handler; a
caller must use the entity-specific filter instead.

| Filter | Fields |
| --- | --- |
| `illust_filter` | `id` (positive), `type` (`illust`, `manga`, or `ugoira`), `tags` (all exact matches), `min_views` (non-negative), `min_pages` (non-negative) |
| `novel_filter` | `id` (positive), `tags` (all exact matches), `min_views` (non-negative) |
| `user_filter` | `id` (positive) |

Artwork search also accepts `bookmark_min`, `bookmark_max`, and
`bookmark_strategy` (`auto`, `local`, `best_effort`, or `server`). The range is
inclusive and non-negative. The application outcome reports `filter.min`,
`filter.max`, `membership`, `strategy`, and `completeness`:

- `auto` currently resolves to local `TotalBookmarks` filtering over fetched
  candidates.
- `local` performs the same exact candidate filtering without claiming global
  result completeness.
- `best_effort` retains App candidate bounds and reports partial completeness.
- `server` fails explicitly until reliable server-side behavior is evidenced;
  it does not silently fall back to another strategy.

Unknown membership is not treated as non-Premium. Premium is not a local hard
gate, and a bookmark count must not be described as a like count.

## Download tools

| Tool | Input | Structured output |
| --- | --- | --- |
| `download` | Exactly one of `src` or non-empty `srcs`; each source is an artwork PID, supported Pixiv artwork/user/public-bookmark URL, or allowed CDN URL. Optional `pages` uses individual 1-based page numbers and closed ranges such as `1,3-5`; `quality` is `original`, `regular`, `small`, `thumb`, or `mini`; `ugoira_mode` is `gif` or `apng`; `delivery` is `local_path`. | `{delivery, items, failures, warnings, files, text}`; each file includes its safe local path/URI, MIME type, size, and page. `warnings` contains non-blocking ugoira filename fallbacks; a warning alone does not set `isError`. Any failure keeps its entry and sets `isError=true`. |
| `download_random_from_recommendation` | Optional `count` (default `5`, explicit `1..20`), `pages`, `quality`, `ugoira_mode`, and `delivery: "local_path"`. | The same local-file report shape, including `warnings` and retained `failures`. |

The download schema does not publish concurrency, filter, archive,
directory-template, metadata-sidecar, retry-count, or retry-delay fields because
the current MCP handler does not map them to `DownloadRequest`. Downloads use
the configured application path/template and surface option errors rather than
silently ignoring input. An invalid or empty-rendered ugoira filename template falls back to the default
filename and is recorded in `warnings` without changing a successful item into a failure. Partial reports retain
completed items and files when another item or the operation fails; a retained failure or operation error sets `isError=true`, while a warning alone does not.

User and public-bookmark URLs expand authenticated visual works in source order
and do not include novels; artwork-series URLs are not download sources. URL parsing is local and does
not fetch HTML or follow redirects. A CDN source has no artwork metadata and is
not treated as an artwork detail request.

## Read tools

| Tool | Input and semantics |
| --- | --- |
| `search_illust` | Required `word`; optional `search_target`, `sort`, `duration`, `start_date`, `end_date`, `content_type`, `ai_mode`, `aspect_ratio`, `resolution`, exact `tool`, bookmark range/strategy, `illust_filter`, `page`, `limit`. Stable enum/date validation happens before opening the SDK. |
| `search_novel` | Required `word`; optional `search_target`, `sort`, `duration`, `novel_filter`, `page`, `limit`. Rating, text-length, and original-only fields are intentionally not published. |
| `reverse_search` | Required `source` (regular local file or HTTP(S) URL); optional `provider` enum. Uses the startup proxy/key/pixiv-only snapshot and returns the reverse-search envelope described above. |
| `illust_detail` | Exactly one of positive `illust_id` or a supported artwork `url`; returns one safe record. |
| `novel_detail` / `novel_content` | Positive `novel_id`; the first returns metadata and the second returns complete structured content blocks. |
| `illust_related` | Positive `illust_id`, optional `illust_filter`, `page`, `limit`. |
| `illust_series` / `novel_series` | Positive `series_id`, `page`, `limit`; novel series also returns safe series metadata. |
| `illust_comments` / `novel_comments` | Positive artwork/novel `id`, `page`, `limit`; output includes safe comments, pagination, and available `total`/`access_control` metadata. |
| `illust_ranking` | Optional `mode`, `date`, `illust_filter`, `page`, `limit`; omitted mode is `day`. |
| `search_user` | Required `word`, optional `user_filter`, `page`, `limit`; uses the App user-search operation. |
| `illust_recommended` | Artwork recommendations with optional `illust_filter`, `page`, `limit`. |
| `recommended` | Required `kind`: `all`, `illust`, `manga`, `novel`, or `user`; optional matching typed filters, `page`, `limit`. |
| `trending_tags_illust` | No input; returns the complete current artwork trending-tag list. |
| `timeline_illust_following` / `timeline_novel_following` | `restrict` (`public`/`private`), matching entity filter, `page`, `limit`. |
| `timeline_illust_latest` | Required `content_type` (`illust` or `manga`), optional `illust_filter`, `page`, `limit`. |
| `timeline_novel_latest` | Optional `novel_filter`, `page`, `limit`. |
| `mypixiv_users` | Optional `user_filter`, `page`, `limit`. |
| `mypixiv_illusts` / `mypixiv_novels` | Matching typed filter, `page`, `limit`. |
| `user_detail` | Required positive `user_id`; returns one safe public profile record. |
| `user_artworks` | Optional `user_id`, `type` (`illust`, `manga`, `ugoira`), `illust_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `user_novels` | Optional `user_id`, `novel_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `user_bookmarks` | Optional `user_id`, `restrict`, `tag`, `illust_filter`, `page`, `limit`; reads artwork bookmarks. |
| `user_novel_bookmarks` | Optional `user_id`, `restrict`, `tag`, `page`, `limit`; reads novel bookmarks. |
| `user_following` / `user_followers` | Optional `user_id`, `restrict`, `user_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `related_users` | Positive `user_id`, optional `user_filter`, `page`, `limit`. |
| `blocked_users` | Optional `user_id`, `page`, `limit`; omitted ID resolves to the authenticated user. App API failure is reported and never changed to a Web fallback. |
| `bookmark_tags` | Optional `user_id`, `restrict`, `page`, `limit`; returns `{bookmark_tags, pagination}`. |
| `bookmark_detail` | Required positive `illust_id`; returns `{bookmarked, restrict, tags}` and preserves the unbookmarked state. |

All read tools use the same application/public-SDK path and preserve typed
authentication, authorization, not-found, upstream, cancellation, and malformed
response errors. They do not manufacture an empty success result from a missing
optional port or a failed App request.

## Write tools

| Tool | Input | Structured output |
| --- | --- | --- |
| `add_bookmark` | `illust_id`, optional `restrict`, repeated `tags` | `{success, action, illust_id}` |
| `remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `follow_user` | `user_id`, optional `restrict` | `{success, action, user_id}` |
| `unfollow_user` | `user_id` | `{success, action, user_id}` |

Writes are artwork-bookmark and user-follow mutations only. A post-submit
unknown state is not replayed under another account. Failed writes return
`success=false` with `isError=true` and a safe diagnostic.

## Authentication and fallback

Pixiv reads and writes require the configured App API access path. There is no
anonymous or Web fallback, and an App API error is final. A removed
`web_fallback_enabled` setting is reported as `removed_setting`.

FANBOX uses a separate MCP server and does not share Pixiv credentials, proxy
settings, tools, or routes.
