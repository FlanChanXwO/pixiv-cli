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
| `download` | Exactly one of `src` or non-empty `srcs`; each source is an artwork PID, supported Pixiv artwork/user/public-bookmark URL, or allowed CDN URL. Optional `pages` uses closed 1-based ranges such as `1,3-5`; `quality` is `original`, `regular`, `small`, `thumb`, or `mini`; `ugoira_mode` is `gif` or `apng`; `delivery` is `local_path`. | `{delivery, items, failures, files, text}`; each file includes its safe local path/URI, MIME type, size, and page. Any failure keeps its entry and sets `isError=true`. |
| `download_random_from_recommendation` | Optional `count` (default `5`, explicit `1..20`), `pages`, `quality`, `ugoira_mode`, and `delivery: "local_path"`. | The same local-file report shape. |

The download schema does not publish concurrency, filter, archive,
directory-template, metadata-sidecar, retry-count, or retry-delay fields because
the current MCP handler does not map them to `DownloadRequest`. Downloads use
the configured application path/template and surface option errors rather than
silently ignoring input.

User and public-bookmark URLs expand authenticated visual works in source order
and do not include novels; artwork-series URLs are not download sources. URL parsing is local and does
not fetch HTML or follow redirects. A CDN source has no artwork metadata and is
not treated as an artwork detail request.

## Read tools

| Tool | Input and semantics |
| --- | --- |
| `search_illust` | Required `word`; optional `search_target`, `sort`, `duration`, `start_date`, `end_date`, `content_type`, `ai_mode`, `aspect_ratio`, `resolution`, exact `tool`, bookmark range/strategy, `illust_filter`, `page`, `limit`. Stable enum/date validation happens before opening the SDK. |
| `search_novel` | Required `word`; optional `search_target`, `sort`, `duration`, `novel_filter`, `page`, `limit`. Rating, text-length, and original-only fields are intentionally not published. |
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
