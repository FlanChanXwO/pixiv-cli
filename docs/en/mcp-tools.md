# MCP tools

[简体中文](../zh-CN/mcp-tools.md) | English | [Documentation index](../index.md)

`pixiv mcp` starts one authenticated Pixiv/FANBOX Streamable HTTP server at the canonical `/mcp` endpoint. MCP uses its configured runtime credential selection; it does not accept CLI data-command account overrides. JSON-RPC is carried over HTTP, never stdin/stdout. See [HTTP startup and proxy configuration](cli-reference.md#mcp-http-server). The `pixiv fanbox mcp` command, stdio transport and standalone legacy SSE transport are removed; Streamable HTTP may still use SSE responses.

`pixiv_novel_content` retains the unavailable-content error contract; its App API
content endpoint is no longer available. A positive `novel_id` returns a
structured `content_unavailable` error with `isError=true` and an empty content
block list; it does not call `/v1/novel/content` and does not fall back to WebView.
Use `pixiv_novel_detail` for novel metadata.

The separate local `pixiv mcp auth init [--reset]` administration command prepares owner state and prints a secret once; it is not a tool or a server launch. See [owner initialization](cli-reference.md#mcp-owner-initialization).

## HTTP authorization

The built-in single-owner OAuth server publishes protected-resource and authorization-server metadata, accepts DCR public clients (`token_endpoint_auth_method=none`), and uses Authorization Code + PKCE S256 with scope `mcp`. The owner enters the local initialization secret only on the instance consent page, never in chat or tool arguments; each authorization requires explicit consent. Cloud deployment requires an HTTPS reverse proxy. Local HTTP is supported. This protocol contract is not evidence that ChatGPT or Gemini live connection testing has passed.

Every `/mcp` request needs a valid bearer token. Missing, expired or revoked credentials return HTTP 401 with canonical `WWW-Authenticate` metadata, not a tool result; unavailable private auth state fails closed. MCP responses are `Cache-Control: no-store`, including SSE. Access tokens last one hour. Refresh rotates the token pair; reusing an old refresh token revokes its grant. Reset revokes grants and invalidates owner sessions and unexchanged codes while retaining registered clients. Restart drops in-memory authorization sessions/codes, not persisted clients or grants. Expiration/reset affects new requests, not work already authorized. Server cancellation stops acceptance and cancels/waits for active handlers; modern-revision request cancellation is propagated through the official SDK.

## Pixiv account selection

The MCP owner has one persisted Pixiv selection shared by connectors. On first use it adopts the explicit CLI default, or the sole local account when no explicit default exists. Multiple accounts without a default return `selection_required`; no local accounts return `no_local_account`. An existing selection is never replaced implicitly: a removed account returns `account_not_found`, and a missing local credential returns `credentials_missing`. Local credential presence does not prove upstream validity.

The first SDK access in a tool call fixes its account snapshot for subsequent reads and writes. Explicit selection overrides pool scheduling without modifying the CLI default. Use `pixiv_account_list`, `pixiv_account_status`, and `pixiv_account_use` to inspect or change this shared selection.

### Account tools

| Tool | Input and semantics |
| --- | --- |
| `pixiv_account_list` | Empty object; all local account summaries and the shared selection. |
| `pixiv_account_status` | Empty object for local selection/credential presence; optional string `login_id` queries login progress without reading account storage. |
| `pixiv_account_use` | Required positive integer `user_id`; checks local existence, then persists the selection for all connectors. |
| `pixiv_account_login_start` | Optional boolean `restart` (default `false`); starts or reuses the server-owned helper login. |

Inputs are closed objects. The three local account operations (status without `login_id`) return `selected_user_id`, `selection_state`, `credential_state`, and `accounts` (each with `user_id`, `username`, `has_credentials`). Status inspection may persist the initial default/sole-account choice; therefore all three publish `readOnlyHint=false`, `destructiveHint=false`, `idempotentHint=true`, and `openWorldHint=false`. A successful status query can report an unusable account; it is not an upstream authentication check.

Failures keep the structured envelope with an `error` category and `isError=true`. Categories are `invalid_request` (schema), `invalid_user_id`, `account_not_found`, `owner_not_initialized`, `canceled`, `deadline_exceeded`, or `local_state_error`. Unavailable state reports selection/credential state as `unknown`, not as a successful empty account. Raw arguments, credential values, filesystem paths, and internal storage causes are not echoed in these error results.

### New-account login

`pixiv_account_login_start` returns `login_id`, `authorization_url` (this server's relay entry, not the SDK OAuth URL), `status`, `requires_local_helper`, and `instructions`. `requires_local_helper` is true for successful starts. Open the URL on a computer with pixiv-cli installed and its `pixiv://` helper registered; if the helper does not open, install/reinstall using the official installer and reopen the URL. No timer guesses helper availability. Never paste callback URLs, authorization codes, or refresh tokens into chat.

Repeated starts reuse a waiting/exchanging flow. `restart=true` cancels and joins the previous flow before replacing it. After a finished flow, a new start replaces its result; no history is accumulated. This tool advertises `readOnlyHint=false`, `destructiveHint=true`, `idempotentHint=false`, `openWorldHint=true`: restarting discards a pending login, and the flow can exchange credentials, save an account and update shared MCP selection. CLI default selection is untouched.

`pixiv_account_status({"login_id":"..."})` returns progress in the `login` object: `waiting_for_user`, `exchanging`, `completed`, `failed`, or `not_found`. It does not read local account storage, so the usual selection/credential fields are `unknown` with zero/empty placeholders; query without `login_id` for actual selection. Successful persistence includes `login.account`, `account_saved=true` and `selection_updated=true`. If selection fails after saving, the query sets `isError=true`, retains `account_saved=true`, and reports `selection_updated=false`; fix the local write problem and use `pixiv_account_use` rather than logging in again. `login.error_code` is `login_failed` or `login_cancelled`, never a raw storage cause. An unknown/replaced ID returns `not_found` without an error.

Start failures return the login envelope with `status=failed`, `error=login_start_failed` and `isError=true`; invalid input uses the same envelope with `error=invalid_request`. The login lifetime belongs to the server, not the start request or helper connection. Explicit restart and shutdown cancel and join it; no fixed login timeout or raw-callback completion tool is added. Loopback fixtures cover this contract, not real helper installation or target-host browser acceptance.

## Errors, pagination, and output

Schema-invalid input is rejected before the SDK operation is opened: the MCP SDK
returns a tool result with `isError=true` and a text diagnostic, without the
handler's structured output. It is not a JSON-RPC protocol error. Account tools and reverse search instead return their sanitized structured error envelope for schema failures. A failure after
handler execution preserves the tool's
structured result and sets `isError=true`; an entity read returns an empty
`records` collection. A normal empty
page is successful and is not converted into an error.

List tools accept `page` and `limit`:

- Omitting `limit` reads one upstream batch.
- A positive `limit` fills the requested logical page across upstream batches.
- `limit: 0` traverses the current upstream cursor until it ends.
- `page` is 1-based and requires a positive `limit`.
- Entity filters are applied before logical pagination and duplicate records are
  removed by their stable entity identity.

`pixiv_illust_comments` and `pixiv_novel_comments` publish the closed input object
`{id, page, limit}`. `id` must be positive; the output envelope is
`{comments, pagination}` with optional `total` and `access_control` fields that
are omitted when the upstream response does not provide them. Artwork comments
use the current artwork-comments operation and novel comments use the current
novel-comments operation. When the current App API supplies the opaque numeric
`comment_access_control` wire field, it is preserved as
`access_control.comment_access_control`; the SDK and server do not infer
`can_comment` or `is_locked` from that number. Neither read tool accepts
mutation-only `stamp_id`, and the legacy MCP registry does not add a standalone
`stamps` tool.

Opaque SDK cursors never leave the server. List results expose `pagination.page`,
`limit`, `returned`, and `has_more`; they may also expose `next_page` when another
logical page is available. `pixiv_recommended(kind="all")` exposes independent
pagination objects for illustration, manga, novel, and user streams.

Records keep public entity fields and an opaque resource reference when one is
needed. They do not expose resolved/signed resource URLs, request headers,
Cookies, expiry metadata, access tokens, or other resource transport credentials.
Available novel content blocks and comment/profile-image references follow the
same rule.
Structured results use explicit DTOs and typed envelopes rather than runtime SDK
models. FANBOX tools follow the same resource shape: a
first-party resource contains its opaque `ref` and optional
`requires_credentials`, never `url`, `request_headers`, or `expires_at`.

## Reverse image search

`pixiv_reverse_search` is a Pixiv MCP tool with a closed input object:

```json
{"source":"/private/path/image.png","provider":"ascii2d-color"}
```

Provide exactly one of `source` or `image`; supplying neither or both is an error.
`source` is a server-local regular-file path, a local `file://` URI, or an HTTP(S) URL.
For host attachments, the tool advertises `_meta["openai/fileParams"] = ["image"]`:

```json
{"image":{"download_url":"https://host.example/image?signature=temporary","file_id":"host-file-id","mime_type":"image/png","file_name":"image.png"}}
```

`image` is a closed object with exactly four declared string fields: required
`download_url` and `file_id`, optional `mime_type` and `file_name`. The download
URL must use HTTP(S), have a host and no user information; local paths and file
URIs are not valid download URLs. `file_id` must be non-empty and is descriptive,
never a download address. Only the download URL enters the existing snapshot
loader; file ID, MIME type and file name are not forwarded to providers. If the
host URL is expired or unavailable, the error asks the user to reattach the image;
the server does not guess another URL or retry it.

Local file URIs must have an absolute path and an empty or `localhost` authority.
Other authorities, user information, query strings, fragments, NULs and UNC paths
are rejected rather than silently discarded or reinterpreted. Percent-encode
literal path characters such as spaces, `#` and `%`. These URI rules apply to
this MCP input, not CLI keyword/image mode selection.

`provider` is optional. The provider enum is
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

The source may be any readable regular file (path or local file URI) on the MCP
server or an HTTP(S) URL fetched through the server network, not the connector
device. Private, loopback,
and link-local URL targets are allowed, and the server may read private files.
A single-owner OAuth grant permits the connector to request these server-side
resources; authorize only connectors you trust. The server fetches or opens the
source once into a private snapshot, uploads it to the selected third-party
provider(s), and never returns or logs the original source, attachment download
URL, file ID, file name, temporary path, request headers, cookies, API key, CSRF
value, redirect `Location`, or upstream response
body. SauceNAO/ascii2d processing and retention follow their own policies; URL
queries may be cached. ascii2d accepts JPEG, PNG, and WEBP and applies its
provider-specific 10 MB limit.

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
errors return a safe `invalid_request` envelope before provider execution, without
echoing invalid values. Snapshots are removed on completion, failure, or
cancellation. Cancellation remains a full request cancellation, not a partial success.

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

## Artwork media

`pixiv_artwork_media` reads artwork bytes through the selected account's SDK resource client, not a server filesystem download. Search/detail/list tools remain metadata-only; they do not download every image implicitly.

| Input | Contract |
| --- | --- |
| `illust_id` | Required positive integer. |
| `pages` | Optional array of one-based page numbers; omission means every page. Empty arrays, nonpositive or out-of-range pages fail. Duplicates are removed in request order. |
| `quality` | Static images use `regular` by default; `thumbnail` uses the SDK thumb variant, and `original` must be explicitly requested. No local resizing. |
| `animation_format` | Ugoira: `gif` (default) or `apng`; any supplied value on static artwork fails. |

The closed input object is validated before opening an account snapshot where possible; page bounds require artwork metadata but are checked before reading any media. One SDK lease binds detail and all media reads to one account, preserves Pixiv Referer and resource-host validation, and closes bodies and the lease on success, failure, or cancellation. Static reads create no temporary files; there are no implicit page/byte caps or cross-account retries of partial delivery.

The structured result contains `illust_id`, `title`, `total_pages`, `requested_pages`, `delivered_pages`, `pages`, `failures`, and `complete`. Each delivered page has its one-based `page`, actual `mime_type`, byte `size`, and zero-based `content_index` into the MCP content array. For static artwork that entry is standard `ImageContent` with real bytes, not a URL or local path. MIME is detected from bytes rather than assumed from a filename or header. An unexpected HTTP status, body read/close error, or non-image body is not a delivered page.

Partial results retain successful images in requested order, list each failed page with an `error` category and optional `http_status`, and set `complete=false` and `isError=true`. Retry explicitly selected failed pages. Input errors use `invalid_request` (schema), `invalid_illust_id`, `invalid_pages`, `invalid_quality`, `invalid_animation_format`, or `animation_format_not_applicable`. Invalid page metadata, unavailable pages, SDK failures, cancellation, or local read/cleanup failures are explicit and redact raw upstream bodies, URLs, and credentials. A top-level `error` describes a whole-call failure; page failures live in `failures`.

`complete=true` describes the response constructed by the server, not proof that a host accepted/displayed it. If a host rejects the entire payload, do not claim delivery; request explicit pages instead. No universal payload limit is invented. Tool annotations are read-only, non-destructive, idempotent, and open-world. Synthetic SDK/MCP tests do not establish ChatGPT/Gemini display support; real-host validation remains deferred.

### Ugoira animation

For Ugoira, omit static `pages` and `quality` parameters; supplying either returns `static_parameters_not_applicable` before archive reads. `animation_format` defaults to `gif` and accepts `apng`. The existing CLI archive-selection policy is reused: prefer original, otherwise use the available archive. The returned `archive_quality` records the actual selected quality; medium is never labeled original. The static regular default is not an animation resize setting.

An animation is one logical page (`total_pages=1`, requested/delivered page 1 on success). Its `pages` entry contains `filename` (`<illust_id>.gif` or `.apng`), actual MIME and byte size, `archive_quality`, and `content_index` pointing to a standard embedded-resource blob with all animation bytes. A separate `preview_content_index` points to PNG `ImageContent` decoded from the generated animation's first frame; this is only a preview, never a substitute for the complete animation. No ffmpeg or new encoder is introduced.

The native encoder uses a private temporary workspace. SDK archive saving, native encoding, preview decoding and cleanup all run within the same account lease; the workspace is removed before a successful response. Failures/cancellation do not publish local paths or mark a preview-only result complete. Host playback/download support still requires deferred real-host acceptance; local GIF/APNG decoding and cleanup tests are not that acceptance.

## Tool annotations and downloads

Pixiv tools use the `pixiv_` prefix and FANBOX tools use `fanbox_`; old Pixiv names are not aliases. Both products share one server, with independent SDK runtimes and credential selection.

All tools publish explicit standard annotations. Account-tool initialization effects are described above; other reads are read-only, non-destructive and idempotent. Bookmark/follow additions are non-destructive, idempotent writes; removals and comment deletion are destructive, idempotent writes. Comment create/reply/stamp operations are non-destructive, non-idempotent writes. These hints support host approval and are not server-enforced authorization.

`openWorldHint` is true for external operations, including reverse-search uploads to third-party providers and Pixiv reads that may authenticate. `pixiv_account_list`, `pixiv_account_status`, and `pixiv_account_use` are closed-world local operations. `fanbox_resolve_url` is also closed-world: opening its local account snapshot and parsing the URL do not access the network.

MCP no longer registers `download` or `download_random_from_recommendation`, nor any renamed download alias. Use CLI `pixiv download` for server/local filesystem downloads.

## Read tools

| Tool | Input and semantics |
| --- | --- |
| `pixiv_search_illust` | Required `word`; optional `search_target`, `sort`, `duration`, `start_date`, `end_date`, `content_type`, `ai_mode`, `aspect_ratio`, `resolution`, exact `tool`, bookmark range/strategy, `illust_filter`, `page`, `limit`. Stable enum/date validation happens before opening the SDK. |
| `pixiv_search_novel` | Required `word`; optional `search_target`, `sort`, `duration`, `novel_filter`, `page`, `limit`. Rating, text-length, and original-only fields are intentionally not published. |
| `pixiv_reverse_search` | Exactly one of `source` (server-local regular file, local `file://` URI, HTTP(S) URL) or host `image` object; optional `provider` enum. Uses the startup proxy/key/pixiv-only snapshot and returns the reverse-search envelope described above. |
| `pixiv_illust_detail` | Exactly one of positive `illust_id` or a supported artwork `url`; returns one safe record. |
| `pixiv_novel_detail` / `pixiv_novel_content` | Positive `novel_id`; the first returns metadata. The second is a retained compatibility tool that returns `content_unavailable` with empty blocks and does not call the rejected content endpoint. |
| `pixiv_illust_related` | Positive `illust_id`, optional `illust_filter`, `page`, `limit`. |
| `pixiv_illust_series` / `pixiv_novel_series` | Positive `series_id`, `page`, `limit`; novel series also returns safe series metadata. |
| `pixiv_illust_comments` / `pixiv_novel_comments` | Closed input `{id, page, limit}` with positive `id`; output is `{comments, pagination}` plus optional `total`/`access_control` metadata. An opaque numeric `comment_access_control` is retained inside `access_control` without boolean inference. Read tools do not accept mutation-only `stamp_id`, and no standalone `stamps` tool is exposed in the legacy registry. |
| `pixiv_illust_ranking` | Optional `mode`, `date`, `illust_filter`, `page`, `limit`; `mode` is a closed ranking enum, dates must be valid `YYYY-MM-DD`, and omitted mode is `day`. |
| `pixiv_search_user` | Required non-blank `word`, optional `user_filter`, `page`, `limit`; blank input is rejected before SDK execution and valid input uses the App user-search operation. |
| `pixiv_illust_recommended` | Artwork recommendations with optional `illust_filter`, `page`, `limit`. |
| `pixiv_recommended` | Required `kind`: `all`, `illust`, `manga`, `novel`, or `user`; optional matching typed filters, `page`, `limit`. `illust`/`manga` select the corresponding artwork subtype, conflicting filters are rejected before SDK execution, and `all` keeps four independent streams with atomic failure semantics. |
| `pixiv_trending_tags_illust` | No input; returns the complete current artwork trending-tag list. An empty upstream list is a successful empty result. |
| `pixiv_timeline_illust_following` / `pixiv_timeline_novel_following` | `restrict` (`public`/`private`), matching entity filter, `page`, `limit`. |
| `pixiv_timeline_illust_latest` | Required `content_type` (`illust` or `manga`), optional `illust_filter`, `page`, `limit`. |
| `pixiv_timeline_novel_latest` | Optional `novel_filter`, `page`, `limit`. |
| `pixiv_mypixiv_users` | Optional `user_filter`, `page`, `limit`. |
| `pixiv_mypixiv_illusts` / `pixiv_mypixiv_novels` | Matching typed filter, `page`, `limit`. |
| `pixiv_user_detail` | Required positive `user_id`; returns one safe public profile record. |
| `pixiv_user_artworks` | Optional `user_id`, `type` (`illust`, `manga`, `ugoira`), `illust_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `pixiv_user_novels` | Optional `user_id`, `novel_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `pixiv_user_bookmarks` | Optional `user_id`, `restrict`, `tag`, `illust_filter`, `page`, `limit`; reads artwork bookmarks. |
| `pixiv_user_novel_bookmarks` | Optional `user_id`, `restrict`, `tag`, `page`, `limit`; reads novel bookmarks. |
| `pixiv_bookmark_list_all` | Optional `user_id`, `restrict`, `tag`, `page`, `limit`; additive aggregate that reads artwork bookmarks before novel bookmarks. `page`/`limit` apply to the concatenated streams, and any required stream failure returns an error with no partial records. |
| `pixiv_bookmark_tags_all` | Optional `user_id`, `restrict`, `page`, `limit`; additive aggregate that reads artwork tags before novel tags. Each tag retains `content_type` and its original `count`; same-name tags are not merged, and any required stream failure returns no partial tags. |
| `pixiv_novel_bookmark_tags` | Optional `user_id`, `restrict`, `page`, `limit`; returns `{bookmark_tags, pagination}` for novel bookmarks. The current candidate App API has no continuation contract; a continuation outside that contract is reported as a typed error. |
| `pixiv_novel_bookmark_detail` | Required positive `novel_id`; returns `{bookmarked, restrict, tags}` for one novel and preserves the absent/unbookmarked state. It uses the candidate novel-bookmark detail App API. |
| `pixiv_user_following` | Optional `user_id`, `restrict`, `user_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `pixiv_user_followers` | Optional `user_id`, `restrict`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `pixiv_related_users` | Optional positive `user_id` (defaults to the authenticated user), compatibility `restrict`, optional `user_filter`, `page`, `limit`. |
| `pixiv_blocked_users` | Optional `user_id`, compatibility `restrict`, `page`, `limit`; omitted ID resolves to the authenticated user. App API failure is reported and never changed to a Web fallback. |
| `pixiv_bookmark_tags` | Optional `user_id`, `restrict`, `page`, `limit`; returns `{bookmark_tags, pagination}`. |
| `pixiv_bookmark_detail` | Required positive `illust_id`; returns `{bookmarked, restrict, tags}` and preserves the unbookmarked state. |

All read tools use the same application/public-SDK path and preserve typed
authentication, authorization, not-found, upstream, cancellation, and malformed
response errors. They do not manufacture an empty success result from a missing
optional port or a failed App request.

## Write tools

| Tool | Input | Structured output |
| --- | --- | --- |
| `pixiv_add_bookmark` | `illust_id`, optional `restrict`, repeated `tags` | `{success, action, illust_id}` |
| `pixiv_add_novel_bookmark` | `novel_id`, optional `restrict`, repeated `tags` | `{success, action, novel_id}` |
| `pixiv_remove_bookmark` | `illust_id` | `{success, action, illust_id}` |
| `pixiv_remove_novel_bookmark` | `novel_id` | `{success, action, novel_id}` |
| `pixiv_create_artwork_comment` | positive `illust_id`, non-empty `comment` | `{success, action, illust_id, comment_id}` |
| `pixiv_reply_artwork_comment` | positive `illust_id`, non-empty `comment`, positive `parent_comment_id` | `{success, action, illust_id, comment_id}` |
| `pixiv_stamp_artwork_comment` | positive `illust_id`, optional `comment` (empty for sticker-only), positive `stamp_id` | `{success, action, illust_id, comment_id}` |
| `pixiv_delete_artwork_comment` | positive `comment_id` | `{success, action, comment_id}` |
| `pixiv_create_novel_comment` | positive `novel_id`, non-empty `comment` | `{success, action, novel_id, comment_id}` |
| `pixiv_reply_novel_comment` | positive `novel_id`, non-empty `comment`, positive `parent_comment_id` | `{success, action, novel_id, comment_id}` |
| `pixiv_stamp_novel_comment` | positive `novel_id`, optional `comment` (empty for sticker-only), positive `stamp_id` | `{success, action, novel_id, comment_id}` |
| `pixiv_delete_novel_comment` | positive `comment_id` | `{success, action, comment_id}` |
| `pixiv_follow_user` | `user_id`, optional `restrict` | `{success, action, user_id}` |
| `pixiv_unfollow_user` | `user_id` | `{success, action, user_id}` |

Writes are artwork/novel-bookmark, artwork/novel-comment/stamp, and user-follow
mutations. An empty `restrict` on an add defaults to `public`; only `public`
and `private` are accepted. Artwork and novel comment create/reply/stamp
operations return the upstream `comment_id`; delete returns the supplied
comment ID. The server never reads the latest comment to guess an ID, never
falls back to the candidate v3 comments contract, and a post-submit unknown
state is not replayed under another account. Failed writes return
`success=false` with `isError=true` and a safe diagnostic.

## FANBOX tools

The same server exposes these unchanged FANBOX names. Their schemas are available through `tools/list`; credentials remain FANBOX-specific.

| Tools | Semantics |
| --- | --- |
| `fanbox_current_user` | Current authenticated FANBOX user. |
| `fanbox_creator`, `fanbox_creators` | One creator profile, or supporting/following creators. |
| `fanbox_creator_tags` | Tags used by a creator. |
| `fanbox_creator_posts`, `fanbox_tagged_posts` | Posts for a creator or one creator tag. |
| `fanbox_post` | One post and its safe resource references. |
| `fanbox_home`, `fanbox_supporting` | Authenticated home/supporting feeds. |
| `fanbox_resolve_url` | Local URL parsing into a typed reference. |
| `fanbox_open_resource` | Opens an opaque `ref`; GET delivers actual image/blob content and safe metadata, HEAD returns metadata only. |

### FANBOX resource content

`fanbox_open_resource` uses only the independent FANBOX SDK lease. GET returns actual bytes: static images as `ImageContent`, GIF/APNG and other attachments as standard embedded-resource blobs. PNG/APNG is distinguished by its animation-control chunk, not only a Content-Type claim. Valid non-image MIME types are preserved; absent or malformed MIME headers are detected from bytes. Bytes are not written to server files and no arbitrary byte cap is added.

The existing `ref`, `status_code`, `content_type`, and advertised `content_length` remain. `size` is the actual byte count, `content_index` identifies the returned content entry, `delivered` means bytes are included in this response, and `complete` indicates request/cleanup success. HEAD returns metadata only (`delivered=false`, no content index). Embedded resource URIs are content-hash URNs, not server paths or promises of a separate resources/read endpoint. When file metadata supplies a name, GET and HEAD also return `filename`, including the supplied extension without duplicating an existing suffix. The SDK owns name resolution along with the resource URL; MCP does not decode refs or guess names from URLs. MCP applies the existing CLI filename-character normalization, so separators and reserved punctuation are replaced with underscores. This is a suggested name, not a trusted destination path; the receiving client still chooses and validates its own destination. Missing names stay absent.

Non-success status, body read/close errors, cancellation, and lease-close errors remain structured failures. SDK error categories are retained without raw upstream bodies, locations, or invalid input. `status_code=0` means the SDK did not expose a status; a classified forbidden error is not assigned an invented numeric status. Schema errors return `invalid_request`; local ref/method errors return `invalid_ref`/`invalid_method`. No failed whole-response transmission proves host delivery. These are local synthetic tests, not real FANBOX account or host acceptance evidence.

## Authentication and fallback

Pixiv reads and writes require the configured App API access path. There is no
anonymous or Web fallback, and an App API error is final. A removed
`web_fallback_enabled` setting is reported as `removed_setting`.

FANBOX tools share the protocol server, not Pixiv credentials or its account pool. Each product retains its own SDK and service configuration; a command-level proxy override applies to both native clients.
