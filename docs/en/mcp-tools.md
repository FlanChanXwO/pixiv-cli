# MCP tools

[简体中文](../zh-CN/mcp-tools.md) | English | [Documentation index](../index.md)

`pixiv mcp` serves Pixiv/FANBOX over local stdio by default. Explicit `--port PORT` enables authenticated Streamable HTTP at the canonical `/mcp` endpoint. Stdio uses local product credentials without HTTP owner state or its read-session file watcher; HTTP uses its persisted owner selection. See [startup and proxy configuration](cli-reference.md#mcp-http-server). Migrate `pixiv fanbox mcp` to the single `pixiv mcp` connector.

`pixiv_novel_content` retains the unavailable-content error contract; its App API
content endpoint is no longer available. A positive `novel_id` returns a
structured `content_unavailable` error with `isError=true` and an empty content
block list; it does not call `/v1/novel/content` and does not fall back to WebView.
Use `pixiv_novel_detail` for novel metadata.

The separate local `pixiv mcp auth init [--reset]` administration command prepares owner state and prints a secret once; it is not a tool or a server launch. See [owner initialization](cli-reference.md#mcp-owner-initialization).

Tool discovery schemas keep optional inputs optional: when an object has no required fields, omit `required` or use `required: []`, never `required: null`. Strict clients may reject the entire tool discovery response when any tool contains an invalid schema.

## Transport capabilities

| Capability | stdio | HTTP |
| --- | --- | --- |
| Pixiv/FANBOX query and mutation tools, opaque FANBOX resources | Yes | Yes |
| `pixiv_artwork_media` complete images / GIF / APNG and PNG preview | Inline content | Inline or protected App delivery |
| `pixiv_download`, `pixiv_download_random_from_recommendation` | Local files | Unregistered |
| `pixiv_show_gallery`, `pixiv_gallery_animation`, Gallery resources | Unregistered | Available with media service |
| `pixiv_account_*` owner management / helper login | Use local CLI authentication | Owner state and relay |

## HTTP authorization

The built-in single-owner OAuth server publishes protected-resource and authorization-server metadata, accepts DCR public clients (`token_endpoint_auth_method=none`), and uses Authorization Code + PKCE S256 with scope `mcp`. The owner enters the local initialization secret only on the instance consent page, never in chat or tool arguments; each authorization requires explicit consent. Cloud deployment requires an HTTPS reverse proxy. Local HTTP is supported. This protocol contract is not evidence that ChatGPT or Gemini live connection testing has passed.

Every `/mcp` request needs a valid bearer token. Missing, expired or revoked credentials return HTTP 401 with canonical `WWW-Authenticate` metadata, not a tool result; unavailable private auth state fails closed. MCP responses are `Cache-Control: no-store`, including SSE. Access tokens last one hour. Refresh rotates the token pair; reusing an old refresh token revokes its grant. Reset revokes grants and invalidates owner sessions and unexchanged codes while retaining registered clients. Restart drops in-memory authorization sessions/codes, not persisted clients or grants. Expiration/reset affects new requests, not work already authorized. Server cancellation stops acceptance and cancels/waits for active handlers; modern-revision request cancellation is propagated through the official SDK.

OAuth control requests and query strings are limited to 64 KiB; oversized bodies return HTTP 413 without truncation. OAuth control bodies must finish reading within 10 seconds; incomplete slow bodies return HTTP 408 and close the connection. Successful reads clear the deadline before parsing and authorization/storage work; early rejection retains the deadline so HTTP cleanup cannot drain an incomplete body indefinitely. DCR stores at most 1024 clients. At capacity, a new registration replaces an arbitrary client that has neither owner approval nor a grant, so anonymous registrations cannot permanently exhaust the registry.

A pending client may need to register again under registration pressure. Explicit owner consent durably protects the client before an authorization code is issued; DCR metadata cannot set this private approval marker. Existing grants, including revoked grants from older state files, also protect their clients; reset preserves that approval before clearing grants. If all 1024 clients are protected, registration returns `temporarily_unavailable` (503); administrators must review stale approved clients and their associated grants in private state while stopped. Older binaries that reject unknown private-state fields cannot read state containing the new approval marker.

Pending owner sessions, forms and codes each allow at most 1024 entries and return `temporarily_unavailable` (503) at capacity without eviction. Owner sessions/forms expire after 10 minutes, matching authorization-code lifetime, and are reclaimed on the next auth state read. HTTP header admission has a 10-second deadline; tool execution, media transfer and shutdown have no new completion deadline. These bounds address unauthenticated resource growth, not upstream media size.

## Pixiv account selection

This section and the account/login tools apply to HTTP. Stdio uses local CLI account selection and authentication.

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

`pixiv_account_status({"login_id":"..."})` returns progress in the `login` object: `waiting_for_user`, `exchanging`, `completed`, `failed`, or `not_found`. It does not read local account storage, so the usual selection/credential fields are `unknown` with zero/empty placeholders; query without `login_id` for actual selection. Successful persistence includes `login.account` and `account_saved=true`. Login preserves an existing selection with local credentials (not a live upstream validity check); only an unset, missing-account or credential-less selection adopts the saved account. `selection_updated=true` means this login committed a changed selection; preserving it returns `selection_updated=false` with `status=completed`.

Availability is checked under the selection lock, so a concurrent explicit choice is not overwritten using stale account data. If selection fails after saving, the query sets `isError=true`, retains `account_saved=true`, and reports `selection_updated=false`; fix the local write problem and use `pixiv_account_use` rather than logging in again. `login.error_code` is `login_failed` or `login_cancelled`, never a raw storage cause. An unknown/replaced ID returns `not_found` without an error.

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

`pixiv_comments` publishes the closed input object
`{kind, id, page, limit}` with required `kind=artwork|novel`. `id` must be positive; the output envelope is
`{comments, pagination}` with optional `total` and `access_control` fields that
are omitted when the upstream response does not provide them. Artwork comments
use the current artwork-comments operation and novel comments use the current
novel-comments operation. When the current App API supplies the opaque numeric
`comment_access_control` wire field, it is preserved as
`access_control.comment_access_control`; the SDK and server do not infer
`can_comment` or `is_locked` from that number. The read tool does not accept
mutation-only `stamp_id`, and the legacy MCP registry does not add a standalone
`stamps` tool.

Most tools keep SDK cursors inside the server. List results expose `pagination.page`,
`limit`, `returned`, and `has_more`; they may also expose `next_page` when another
logical page is available. `pixiv_recommended(kind="all")` exposes independent
pagination objects for illustration, manga, novel, and user streams.

`pixiv_search_illust`, `pixiv_illust_related`, `pixiv_illust_ranking`, and `pixiv_illust_recommended` additionally return `pagination.next_cursor` when a default batch has more data. Repeat the original arguments (including the search query/strategy, artwork ID, ranking mode/date, and filters where applicable), pass that opaque value as `cursor`, and omit `page`/`limit`. The MCP envelope binds the original arguments and contains SDK continuation state, not credentials; it is encoding, not encryption or authorization. The SDK revalidates operation/query/account binding on every continuation. Changed arguments, an incompatible account, malformed or immediately repeated cursors fail explicitly. The batch consumes all returned items before advancing, including traversal past filtered-empty upstream batches; existing positive-limit logical pagination is unchanged. The field is omitted on exhaustion or failure.

For `pixiv_recommended`, default batches attach `next_cursor` independently to `pagination.illust`, `manga`, `novel`, and `user` when that stream can continue. A stream cursor requires its matching single `kind`, the same applicable filter, and omitted `page`/`limit`; `kind=all` with a cursor is rejected. Drop the other kinds’ filters. The initial illustration/manga projections share one upstream window; continuing either does not consume the other projection. An empty filtered visual window can still carry continuation. Filters excluding a visual kind produce no cursor for that kind. Failed or replayed attempts do not leak records or pagination from a prior attempt.

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

HTTP clients advertising the `io.modelcontextprotocol/ui` extension with `text/html;profile=mcp-app` receive an indexed App-only array in `_meta["pixiv/mediaContents"]`; model-facing `content` contains only the independent PNG preview. When the existing private animation publisher has retained the complete original, that array contains a protected `resource_link` for the animation and its inline PNG preview. Play reads the original binary only on demand; downloads use the same original file, grant/account-version checks and expiry. `content_index` and `preview_content_index` address this private array when present.

Without a published HTTP artifact, including stdio, the original full inline content is retained; no App metadata hides the animation. Non-App or unsupported-MIME clients retain the original full `content` response; static media and public inputs are unchanged. For linked animations, Refresh animation explicitly reacquires authorization through `pixiv_gallery_animation` in the original format; it replaces expired playback/download addresses and uses the existing status polling and subscription cleanup. A failed image does not automatically prepare again. Hosts without server tool calls cannot renew expired links.

### Ugoira animation

For Ugoira, omit static `pages` and `quality` parameters; supplying either returns `static_parameters_not_applicable` before archive reads. `animation_format` defaults to `gif` and accepts `apng`. The existing CLI archive-selection policy is reused: prefer original, otherwise use the available archive. The returned `archive_quality` records the actual selected quality; medium is never labeled original. The static regular default is not an animation resize setting.

GIF avoids repeated unchanged regions in opaque animations without changing its quantized pixels, frame count or timing; animations containing transparency keep full-frame background clearing.

APNG uses lossless frame differences where compatible with exact alpha compositing; all frames, dimensions and delays remain intact. This reduces redundant encoding without guaranteeing acceptance of large embedded results by every host.

The HTTP media tool also attaches the existing MCP App view for complete media playback and its independent static preview. When the host advertises file download, the view requests its confirmation flow. Otherwise an available protected HTTP attachment opens in the browser; browser restrictions still apply. The server retains the already encoded animation and preview under the existing animation temporary capacity, grant, account version and TTL, without another download or encoding. Private UI metadata carries these short-lived addresses; no filesystem path is exposed. Optional publication failure leaves the complete inline result intact but makes the browser download unavailable. Restart, expiry or authorization changes invalidate these addresses.

An animation is one logical page (`total_pages=1`, requested/delivered page 1 on success). Its `pages` entry contains `filename` (`<illust_id>.gif` or `.apng`), actual MIME and byte size, `archive_quality`, and `content_index` pointing to standard `ImageContent` with all animation bytes. APNG uses the PNG-family `image/png` content MIME with all animation chunks intact; structured `mime_type` remains `image/apng` and the filename remains `.apng`. Consumers must use the content index and filename to distinguish it from the independent preview, rather than assume every `image/png` item is a preview. A separate `preview_content_index` points to PNG `ImageContent` decoded from the generated animation's first frame; this is only a preview, never a substitute for the complete animation. No ffmpeg or new encoder is introduced.

The native encoder uses a private temporary workspace. SDK archive saving, native encoding, preview decoding and cleanup all run within the same account lease; the workspace is removed before a successful response. Failures/cancellation do not publish local paths or mark a preview-only result complete. Host playback/download support still requires deferred real-host acceptance; local GIF/APNG decoding and cleanup tests are not that acceptance.

## Tool annotations and downloads

Pixiv tools use the `pixiv_` prefix and FANBOX tools use `fanbox_`; old Pixiv names are not aliases. Both products share one server, with independent SDK runtimes and credential selection.

All tools publish explicit standard annotations. Account-tool initialization effects are described above; other reads are read-only, non-destructive and idempotent. Bookmark/follow additions are non-destructive, idempotent writes; removals and comment deletion are destructive, idempotent writes. Comment create/reply/stamp operations are non-destructive, non-idempotent writes. These hints support host approval and are not server-enforced authorization.

`openWorldHint` is true for external operations, including reverse-search uploads to third-party providers and Pixiv reads that may authenticate. `pixiv_account_list`, `pixiv_account_status`, and `pixiv_account_use` are closed-world local operations. `fanbox_resolve_url` is also closed-world: opening its local account snapshot and parsing the URL do not access the network.

Stdio registers `pixiv_download` (`src` or `srcs`, optional `pages`, `quality`, `ugoira_mode`, and `delivery=local_path`) and `pixiv_download_random_from_recommendation` (optional `count`, default 5, established range 1–20, and the same download options). They reuse the shared downloader and CLI download directory/filename/directory templates. Results preserve `items`, `files` (path/file URI/MIME/size/page), warnings and partial failures; failures set `isError=true`. Download tools declare write/destructive effects because the shared saver may replace existing destination files. No arbitrary destination parameter is exposed. HTTP never registers these filesystem tools. Migrate old `download` and `download_random_from_recommendation` names to the prefixed names; no permanent aliases are registered.

`pixiv_novel_bookmark_tags` migrates to `pixiv_bookmark_tags` with `kind="novel"`; `kind="artwork"` is the existing default. Input paging/visibility and the tag output envelope are preserved. The old name is unregistered; the migration table below covers the other consolidated entry points.

## Tool migration

The consolidated tools require an explicit selector; there is no implicit target type. The old names below are unregistered, not permanent aliases. Output schemas, mutation `action` values and output ID fields remain unchanged: only target inputs become `id` where shown. Filters for another entity type are rejected before account access.

| Previous entry | Current call / argument change |
| --- | --- |
| `pixiv_illust_comments`, `pixiv_novel_comments` | `pixiv_comments`, add `kind=artwork` / `novel`; retain `id` |
| `pixiv_create_artwork_comment`, `pixiv_create_novel_comment` | `pixiv_comment_create`, add `kind`; rename `illust_id` / `novel_id` to `id` |
| `pixiv_reply_artwork_comment`, `pixiv_reply_novel_comment` | `pixiv_comment_reply`, same target mapping; retain `parent_comment_id` |
| `pixiv_stamp_artwork_comment`, `pixiv_stamp_novel_comment` | `pixiv_comment_stamp`, same target mapping; retain `stamp_id` and optional `comment` |
| `pixiv_delete_artwork_comment`, `pixiv_delete_novel_comment` | `pixiv_comment_delete`, add `kind=artwork` / `novel`; retain `comment_id` |
| `pixiv_add_bookmark`, `pixiv_add_novel_bookmark` | `pixiv_bookmark_add`, add `kind=artwork` / `novel`; rename target to `id`; retain `restrict` and `tags` |
| `pixiv_remove_bookmark`, `pixiv_remove_novel_bookmark` | `pixiv_bookmark_remove`, add `kind=artwork` / `novel`; rename target to `id` |
| `fanbox_creator_posts`, `fanbox_tagged_posts` | `fanbox_creator_posts`, omit `tag` / retain required non-empty tag |
| `fanbox_home`, `fanbox_supporting` | `fanbox_feed(source=home)` / `fanbox_feed(source=supporting)` |
| `pixiv_user_bookmarks`, `pixiv_user_novel_bookmarks` | `pixiv_user_bookmarks`, add `kind=artwork` / `novel` |
| `pixiv_user_followers`, `pixiv_user_following`, `pixiv_related_users` | `pixiv_user_relations`, add `relation=followers` / `following` / `related` |
| `pixiv_timeline_illust_following`, `pixiv_timeline_novel_following` | `pixiv_timeline_following`, add `kind=artwork` / `novel` |
| `pixiv_timeline_illust_latest`, `pixiv_timeline_novel_latest` | `pixiv_timeline_latest`, retain `content_type=illust\|manga` / add `content_type=novel` |
| `pixiv_mypixiv_illusts`, `pixiv_mypixiv_novels`, `pixiv_mypixiv_users` | `pixiv_mypixiv`, add `kind=artwork` / `novel` / `user` |

`kind=artwork` includes manga and Ugoira where the original artwork operation did. Only the matching `illust_filter`, `novel_filter` or `user_filter` may be supplied; novel user bookmarks have no new filter. `restrict` affects followers/following only; related users retain their existing ignored-field behavior.

`pixiv_bookmark_list_all` stays separate: it applies one logical budget across artwork-then-novel streams. `pixiv_illust_recommended` also stays separate from `pixiv_recommended`: the former filters during collection and returns a records pagination envelope; the latter projects typed streams from a fetched window and has different cursor binding and output pagination. Reads/writes, additions/removals, account login/status, Gallery/media, local downloads/protected HTTP media and Pixiv/FANBOX credentials remain distinct.

## Read tools

`search_illust` and `illust_detail` records retain upstream artwork `is_bookmarked`, `is_muted`, `visible`, `sanity_level`, `restriction_attributes`, and optional `series` (`id`, `title`); detail also includes `total_comments` when supplied. Viewer state reflects the account used for the read. No per-result enrichment request is made. Absent viewer/safety fields and series/comments are omitted; an explicitly supplied empty restriction-attribute list remains `[]`. Missing means unknown, not `false` or `0`.

| Tool | Input and semantics |
| --- | --- |
| `pixiv_search_illust` | Required `word`; optional `search_target`, `sort`, `duration`, `start_date`, `end_date`, `content_type`, `ai_mode`, `aspect_ratio`, `resolution`, exact `tool`, bookmark range/strategy, `illust_filter`, `page`, `limit`. Stable enum/date validation happens before opening the SDK. Default-batch `cursor` also preserves bookmark strategy checkpoints; omit `page`/`limit` and repeat the same query, filters and strategy. |
| `pixiv_search_novel` | Required `word`; optional `search_target`, `sort`, `duration`, `novel_filter`, `page`, `limit`. Rating, text-length, and original-only fields are intentionally not published. |
| `pixiv_reverse_search` | Exactly one of `source` (server-local regular file, local `file://` URI, HTTP(S) URL) or host `image` object; optional `provider` enum. Uses the startup proxy/key/pixiv-only snapshot and returns the reverse-search envelope described above. |
| `pixiv_illust_detail` | Exactly one of positive `illust_id` or a supported artwork `url`; returns one safe record. |
| `pixiv_novel_detail` / `pixiv_novel_content` | Positive `novel_id`; the first returns metadata. The second is a retained compatibility tool that returns `content_unavailable` with empty blocks and does not call the rejected content endpoint. |
| `pixiv_illust_related` | Positive `illust_id`, optional `illust_filter`, `page`, `limit`; default-batch `cursor` requires omitted `page`/`limit`. |
| `pixiv_illust_series` / `pixiv_novel_series` | Positive `series_id`, `page`, `limit`; novel series also returns safe series metadata. |
| `pixiv_comments` | Required `kind=artwork\|novel` and positive `id`; optional `page`, `limit`. Returns `{comments, pagination}` and optional `total`/`access_control`; opaque numeric access metadata is preserved. No mutation-only `stamp_id`. |
| `pixiv_illust_ranking` | Optional `mode`, `date`, `illust_filter`, `page`, `limit`; `mode` is a closed ranking enum, dates must be valid `YYYY-MM-DD`, and omitted mode is `day`. Default-batch `cursor` requires omitted `page`/`limit`. |
| `pixiv_search_user` | Required non-blank `word`, optional `user_filter`, `page`, `limit`; blank input is rejected before SDK execution and valid input uses the App user-search operation. |
| `pixiv_illust_recommended` | Artwork recommendations with optional `illust_filter`, `page`, `limit`. Default-batch `cursor` requires omitted `page`/`limit`. |
| `pixiv_recommended` | Required `kind`: `all`, `illust`, `manga`, `novel`, or `user`; optional matching typed filters, `page`, `limit`. `illust`/`manga` select the corresponding artwork subtype, conflicting filters are rejected before SDK execution, and `all` keeps four independent streams with atomic failure semantics. Default-batch `cursor` requires a matching single kind and omitted page/limit. |
| `pixiv_trending_tags_illust` | No input; returns the complete current artwork trending-tag list. An empty upstream list is a successful empty result. |
| `pixiv_timeline_following` | Required `kind=artwork\|novel`; optional `restrict` (`public` by default or `private`), matching typed filter, `page`, `limit`. Artwork includes illustrations, manga and Ugoira. |
| `pixiv_timeline_latest` | Required `content_type=illust\|manga\|novel`; optional matching typed filter, `page`, `limit`. |
| `pixiv_mypixiv` | Required `kind=artwork\|novel\|user`; optional matching typed filter, `page`, `limit`. Reads only the authenticated account’s MyPixiv, not an arbitrary user ID. |
| `pixiv_user_detail` | Required positive `user_id`; returns one safe public profile record. |
| `pixiv_user_artworks` | Optional `user_id`, `type` (`illust`, `manga`, `ugoira`), `illust_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `pixiv_user_novels` | Optional `user_id`, `novel_filter`, `page`, `limit`; omitted ID resolves to the authenticated user. |
| `pixiv_user_bookmarks` | Required `kind=artwork\|novel`; optional `user_id`, `restrict`, `tag`, `page`, `limit`. `illust_filter` is artwork-only. Omitted/zero user ID resolves to the authenticated account; null paging/filter values retain their existing meaning. |
| `pixiv_bookmark_list_all` | Optional `user_id`, `restrict`, `tag`, `page`, `limit`; additive aggregate that reads artwork bookmarks before novel bookmarks. `page`/`limit` apply to the concatenated streams, and any required stream failure returns an error with no partial records. |
| `pixiv_bookmark_tags_all` | Optional `user_id`, `restrict`, `page`, `limit`; additive aggregate that reads artwork tags before novel tags. Each tag retains `content_type` and its original `count`; same-name tags are not merged, and any required stream failure returns no partial tags. |
| `pixiv_novel_bookmark_detail` | Required positive `novel_id`; returns `{bookmarked, restrict, tags}` for one novel and preserves the absent/unbookmarked state. It uses the candidate novel-bookmark detail App API. |
| `pixiv_user_relations` | Required `relation=followers\|following\|related`; optional `user_id`, `restrict`, `page`, `limit`. `user_filter` is supported only for following/related. Omitted ID uses the authenticated account. `restrict` remains accepted but ignored for related users. |
| `pixiv_blocked_users` | Optional `user_id`, compatibility `restrict`, `page`, `limit`; omitted ID resolves to the authenticated user. App API failure is reported and never changed to a Web fallback. |
| `pixiv_bookmark_tags` | Optional `kind` (`artwork` default or `novel`), `user_id`, `restrict`, `page`, `limit`; returns `{bookmark_tags, pagination}`. |
| `pixiv_bookmark_detail` | Required positive `illust_id`; returns `{bookmarked, restrict, tags}` and preserves the unbookmarked state. `tags` is an empty array when no tags exist, including unbookmarked artworks. |

All read tools use the same application/public-SDK path and preserve typed
authentication, authorization, not-found, upstream, cancellation, and malformed
response errors. They do not manufacture an empty success result from a missing
optional port or a failed App request.

## Write tools

| Tool | Input | Structured output |
| --- | --- | --- |
| `pixiv_bookmark_add` | `kind=artwork\|novel`, positive `id`, optional `restrict`, `tags` | `{success, action, illust_id}` or `{success, action, novel_id}` |
| `pixiv_bookmark_remove` | `kind=artwork\|novel`, positive `id` | `{success, action, illust_id}` or `{success, action, novel_id}` |
| `pixiv_comment_create` | `kind=artwork\|novel`, positive `id`, non-empty `comment` | `{success, action, illust_id/novel_id, comment_id}` |
| `pixiv_comment_reply` | Required `kind=artwork\|novel`, positive `id`, non-empty `comment` and positive `parent_comment_id` | `{success, action, illust_id/novel_id, comment_id}` |
| `pixiv_comment_stamp` | `kind=artwork\|novel`, positive `id` and `stamp_id`, optional `comment` (empty for sticker-only) | `{success, action, illust_id/novel_id, comment_id}` |
| `pixiv_comment_delete` | `kind=artwork\|novel`, positive `comment_id`; no content ID | `{success, action, comment_id}` |
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

The same server exposes these FANBOX tools. Their schemas are available through `tools/list`; credentials remain FANBOX-specific.

| Tools | Semantics |
| --- | --- |
| `fanbox_current_user` | Current authenticated FANBOX user. |
| `fanbox_creator`, `fanbox_creators` | One creator profile, or supporting/following creators. |
| `fanbox_creator_tags` | Tags used by a creator. |
| `fanbox_creator_posts` | Required `creator_id`; optional non-empty `tag`, `page`, `limit`. Omitted tag uses CreatorPosts; a supplied tag uses TaggedPosts. An explicit empty tag is an error. |
| `fanbox_post` | One post and its safe resource references. |
| `fanbox_feed` | Required `source=home\|supporting`; optional `page`, `limit`. Preserves each feed’s API, post structure and FANBOX authentication. |
| `fanbox_resolve_url` | Local URL parsing into a typed reference. |
| `fanbox_open_resource` | Opens an opaque `ref`; GET delivers actual image/blob content and safe metadata, HEAD returns metadata only. |

### FANBOX resource content

`fanbox_open_resource` uses only the independent FANBOX SDK lease. GET returns actual bytes: static images as `ImageContent`, GIF/APNG and other attachments as standard embedded-resource blobs. PNG/APNG is distinguished by its animation-control chunk, not only a Content-Type claim. Valid non-image MIME types are preserved; absent or malformed MIME headers are detected from bytes. Bytes are not written to server files and no arbitrary byte cap is added.

The existing `ref`, `status_code`, `content_type`, and advertised `content_length` remain. `size` is the actual byte count, `content_index` identifies the returned content entry, `delivered` means bytes are included in this response, and `complete` indicates request/cleanup success. HEAD returns metadata only (`delivered=false`, no content index). Embedded resource URIs are content-hash URNs, not server paths or promises of a separate resources/read endpoint. When file metadata supplies a name, GET and HEAD also return `filename`, including the supplied extension without duplicating an existing suffix.

The SDK owns name resolution along with the resource URL; MCP does not decode refs or guess names from URLs. MCP applies the existing CLI filename-character normalization, so separators and reserved punctuation are replaced with underscores. This is a suggested name, not a trusted destination path; the receiving client still chooses and validates its own destination. Missing names stay absent.

Non-success status, body read/close errors, cancellation, and lease-close errors remain structured failures. SDK error categories are retained without raw upstream bodies, locations, or invalid input. `status_code=0` means the SDK did not expose a status; a classified forbidden error is not assigned an invented numeric status. Schema errors return `invalid_request`; local ref/method errors return `invalid_ref`/`invalid_method`. No failed whole-response transmission proves host delivery. These are local synthetic tests, not real FANBOX account or host acceptance evidence.

## Authentication and fallback

Pixiv reads and writes require the configured App API access path. There is no
anonymous or Web fallback, and an App API error is final. A removed
`web_fallback_enabled` setting is reported as `removed_setting`.

FANBOX tools share the protocol server, not Pixiv credentials or its account pool. Each product retains its own SDK and service configuration; a command-level proxy override applies to both native clients.

## MCP Apps artwork previews

`pixiv_show_gallery` and `pixiv_artwork_media` associate their results with the Gallery UI (`text/html;profile=mcp-app`, `_meta.ui.resourceUri`). The stable `ui://pixiv-cli/gallery` remains readable. Tools reference an additional `ui://pixiv-cli/gallery/sha256-<digest>` alias derived from the rendered frontend content, including its injected canonical media origin so host UI caching does not conceal updates. Historical revision aliases deliver the current view on a fresh read; the digest is a cache identity, not an integrity guarantee for historical content. Ordinary search, ranking, recommendation and detail tools return data without a UI association. Call the gallery only when the user explicitly requests images or a gallery; the host ultimately decides whether to render it.

Input: required nonempty positive-integer array `illust_ids`; optional positive `page` (default 1) and `quality` (`thumbnail` default, `regular`, `original`). Duplicate IDs retain their first position. The tool fetches metadata, not another search, and returns `records`, `images` (artwork ID, page, URL), `failures` and, when configured, `media_origin`. Individual failures retain successful records and set `isError`. The gallery displays only supplied IDs; use the original data tools for further search pages before explicitly opening another gallery.

Static previews default to same-origin image proxy URLs, not base64 tool content or raw CDN links. GET/HEAD under the canonical base’s `/media/` route validate a signed capability bound to the OAuth grant, selected account, artwork, page and quality. It expires no later than the issuing access token; grant revocation/reset or server restart invalidates it. Account selection changes do not retarget old URLs. Reads use the public SDK and configured outbound proxy, reject arbitrary URL inputs and non-raster content, and return `Cache-Control: private, no-store`. HEAD validates the same image without sending its body. There is no disk/public cache. Treat complete image URLs as short-lived credentials: never share or log them.

### Cards and detail

Cards display only complete, uncropped thumbnails, including static Ugoira covers, without text metadata or badges. Detail displays the full title, tags, description and available author, publication date, views, bookmarks, artwork ID and dimensions; the redundant Pages metadata row is omitted. R-18/R-18G badges remain in detail. Authors with a valid ID open their Pixiv user page through the host. Wide containers place media beside information; narrow containers stack them. Metadata is rendered as text, not HTML. Pixiv caption `<br>` line breaks display as newlines and valid decimal/hexadecimal numeric entities display as characters; other markup and invalid numeric entities remain inert text. Icon controls have accessible names, keyboard focus and host-theme colors.

#### Loading and page navigation

Images load when visible. With tool-call support, the view obtains fresh thumbnail capabilities in one authenticated batch before loading, so historical URLs invalidated by expiry or server restart are not reused. Without tool-call support, supplied URLs remain best effort. Failures expose explicit retry; no automatic retry is added. Click a thumbnail for detail. Single-page works omit page navigation; multipage works provide bounded previous/next arrows, horizontal swipes and a native progress slider with a read-only position. Dragging previews the target; committing loads it and superseded responses are ignored. Vertical gestures remain available to the chat. Dimensions follow the selected page when supplied; original downloads use that page.

#### Originals and downloads

Detail images use regular quality. Click/double-click a static detail image, or focus it and press Enter/Space, to open its full-quality original in a modal with fit/actual-size controls, Escape/close and host-confirmed download. Original viewing and host-confirmed downloading share successful bytes. If the host does not advertise file downloads, the loaded static original requests a fresh original-quality capability for the selected page and offers a user-clicked HTTPS browser attachment link. Only the exact `?download=1` selector is accepted, with a filename derived from the artwork ID, one-based page and validated image MIME type. The attachment and inline HTTP image share the authorized server cache; grant, expiry and account-version checks remain active. Browser download policy still applies, and no successful save is claimed.

#### Browser session cache

A separate 256MiB estimated session LRU retains loaded static images, successful animation/PNG preview elements, detail metadata and original bytes; paging and reentering detail reuse them and restore the selected page. When an uncached page is selected, the displayed image stays visible until its replacement loads; rapid changes discard superseded replacements. New query/results, cancellation and teardown clear it. Re-delivery of an identical result without new tool input preserves the active detail and session cache. Failures and bookmark state are not cached; oversized images display fully without retention. The estimate includes decoded pixels and encoded data allowances, not total browser/GPU memory. Explicit original-page downloads use the full-byte media tool and host download capability. Ugoira retains complete GIF/APNG delivery with a separate PNG preview.

#### Bookmark actions

The bookmark toolbar offers a primary save action, public/private visibility, removal for bookmarked works and explicit state refresh. Unknown or unconfirmed bookmark state disables writes; writes remain explicit actions with host approval and subsequent state refresh. Display/playback and download confirmation remain host-dependent.

### Host integration

The standard Apps bridge handles initialization, tool results and calls. CSP permits only canonical proxy-origin and Blob images, no direct fetch connections or external scripts/styles; image requests use `no-referrer`. The iframe receives no OAuth bearer, Pixiv credential or FANBOX session. Replacing views cancels pending tool requests and releases animation subscriptions. See the [MCP Apps specification](https://github.com/modelcontextprotocol/ext-apps/blob/main/specification/2026-01-26/apps.mdx).

Local bridge/browser fixtures do not prove cloud-host interoperability. Without Apps support, use structured data and `pixiv_artwork_media` directly.

### Read sessions and server cache

Gallery deduplicates IDs, queries artwork concurrently within the configured limit, and preserves input order and per-item failures. MCP read sessions reuse the access token and HTTP connections until expiry, keyed by account, credential revision and network configuration. Only refresh and credential persistence hold the rotation gate; content requests start after successful persistence and do not hold it during transfer. An upstream authentication failure invalidates the session without automatic retry. Trusted SDK artwork metadata registers quality variants, avoiding another artwork query when opening them. Cancelling a single credential check preserves the shared session and unrelated transfers; actual credential or authorization invalidation still cancels them.

The grant-scoped process cache uses the startup settings documented in [the CLI reference](cli-reference.md#mcp-http-server): 256 MiB, 15 minutes, concurrency 4 by default. It merges concurrent metadata lookups and stores only successful metadata and complete raster images. Each hit reauthorizes the grant and resolves the current account/session generation. Bookmark state and writes are not cached. Images stream after upstream status and file-header validation; trusted Content-Length is preserved, otherwise transfer is streamed without a declared length. Interrupted transfers close upstream resources, discard filling buffers and never append error text to image bytes. Fixed stage observations distinguish session wait/refresh, artwork lookup, first image bytes, transfer and cache hits; they contain no token, grant or capability URL.

ZIP body failures additionally emit the fixed `animation_zip_incomplete` marker for unexpected EOF or a Content-Length mismatch, `animation_zip_reset` for a reported connection reset, or `animation_zip_timeout` for a timeout reported by the underlying reader, provided the job context is still active. These markers contain only the partial byte count; they do not include raw errors or signed addresses and do not change retries, deadlines, output quality or tool errors.

### `pixiv_gallery_animation`

Gallery animation control accepts `action: prepare` with positive `illust_id` and optional `format` (`gif` default, or `apng`). `status` and `release` accept only an opaque `handle`. Each preparation returns an independent subscription. The control response never embeds animation bytes. States are `queued`, `downloading`, `encoding`, `ready`, `failed`, `cancelled`, and `expired`; progress percentages are not supplied. Poll `status` every two seconds, stop on terminal states, and retry only after an explicit user action.

Ready output includes `animation_url`, independent `preview_url` (PNG), `media_origin`, `filename`, `mime_type`, `size`, and `expires`. Gallery reports loading until the selected image has loaded, then confirms static preview or animation playback. Switching between loaded animation and preview reuses their image elements without another HTTP read. Successful animation/preview elements also share the existing 256MiB estimated session LRU across detail reentry. Reentry with retained media makes a new prepare request to validate current authorization before showing it; only media URLs matching the new ready response can reuse old elements.

Unloaded animation starts only on Play; failure requires explicit retry. Once preparation is ready, Gallery hides the redundant Load animation action; delivery failure, an unconfirmed host download or a format change restores it without automatically preparing again. Leaving the view releases the subscription and removes its image callbacks. Failed, incomplete and over-budget media sources are cleared; successful retained elements are released on LRU eviction, new query/results or teardown. Changing format and explicit retry still obtain a new subscription. The animation estimate includes encoded animation bytes and decoded pixels for all frames, using the private UI hint `_meta["pixiv/animationFrameCount"]`; missing or invalid frame counts prevent new animation retention.

This hint does not add tool inputs or structured output fields; the PNG estimate uses decoded pixel allowances, not measured compressed bytes. Decoder overhead and total browser/GPU memory are outside this estimate. Gallery shows the static preview initially; Play selects the complete GIF/APNG HTTP resource. Downloads use `ui/download-file` with a `resource_link` when the host advertises download support. Otherwise, Gallery offers a user-clicked browser link to the same protected animation with the exact `?download=1` selector. GET/HEAD retain authorization, expiry and account-version checks; the response uses an attachment filename derived from artwork ID and format.

Other query parameters and preview downloads are rejected. Browser download policy still applies, and Gallery does not claim the file was saved. The existing `pixiv_artwork_media` retains full inline delivery for ordinary clients and uses the negotiated App delivery described above when supported.

Duplicate preparation reuses one job and a complete successful artifact within the same grant/account/session generation/artwork/format. Releasing the last subscription cancels unfinished work. Ready artifacts can remain until expiry, bounded by cache TTL and issuing authorization expiry. Every GET/HEAD under `/media/animation/` checks authorization and current account generation; responses are `private, no-store`. Addresses are short-lived capabilities and must not be logged or shared.

Private temporary directories/files use 0700/0600 permissions. ZIP, native output reservations, previews and retained artifacts share the separate startup disk budget (`mcp_gallery_animation_temp_capacity_mib`, default 1024 MiB). Native writes claim their actual bytes through an internal synchronous callback, so encoding another format preserves unused ready artifacts while capacity remains. Writes fail explicitly before exceeding the budget; output is neither truncated for delivery nor encoded at reduced quality. Expired/unused ready artifacts may be evicted. Files pinned by a transfer are cleaned after its handle closes.

Service shutdown joins workers and transfers before removing its private workspace. At startup, only marked workspaces whose lifetime OS lock has been released are removed; live instances and unknown directories remain intact. Temporary producers queue together with the native output reservation, avoiding false capacity failures between concurrent preparations; ready-file reads remain concurrent. The first preparation still downloads and encodes; no fixed public-network latency is promised.

#### Diagnostics

With debug diagnostics enabled, MCP responses for `pixiv_artwork_media` and `pixiv_gallery_animation` report `mcp_response_serialization` with the actual result JSON encoding duration and byte count. This excludes the JSON-RPC envelope and network transfer. Debug HTTP POST responses separately report each underlying `mcp_http_write` (accepted bytes and call duration) and `mcp_http_flush` (call duration), including write/flush failures, request cancellation and service shutdown. These observations preserve SDK framing and do not prove peer receipt; they record neither request/response bodies nor headers. Normal logging keeps the existing encoding path, and overlapping HTTP/protocol diagnostic scopes do not duplicate encoding or observations. Animation delivery interruptions are reported as failed events with fixed request-cancelled or transport-interrupted reasons; raw errors, credentials and capability URLs are omitted.

The existing `pixiv_artwork_media` animation path also reports artwork lookup, animation metadata, ZIP download, observed native encoding wait/encoding, and result-file read stages. `animation_generation` records the final generation outcome after temporary-file cleanup, including failures and request cancellation; its byte count is zero on failure and the animation size on success. These generation stages exclude response serialization and network delivery. An encoding attempt that fails before entering the native queue does not report a fabricated queue duration.

#### Cancellation and invalidation

Background animation cancellation records the first cancellation source after worker cleanup: last subscription release, service shutdown, or media expiry. The CLI presenter uses fixed reasons rather than task handles or raw cancellation text. Authorization/account-session invalidation detected by subscription or media checks has its own fixed reason. A subscription check with a revoked grant cancels its existing job while still rejecting the request; foreign accounts cannot cancel that job. Control-request cancellation, including during account-session acquisition, leaves subscribed work running and returns a cancellation error.

Cancellation of one animation HTTP request also preserves the shared artifact and independent subscriptions; authorization or credential invalidation still expires them. Active binary transfers are bound to the job lifecycle: shutdown or revocation cancels the transfer, closes its file and sets an immediate HTTP write deadline to unblock a stalled write. This is cancellation-triggered interruption, not a fixed transfer timeout. Cancellation callbacks finish before the handler returns; private files are removed after pinned transfers release them. The animation owner watches the authorization directory to detect atomic state replacement and proactively cancel revoked/reset grants without another status or media request.

Losing the watcher stops animation work and prevents new animation requests; shutdown reports a fixed watcher error. Account-session invalidation now publishes a generation-scoped cancellation signal: upstream authentication failure stops other transfers and animation jobs using that generation without retrying the failed request. Errors from an older generation cannot cancel its replacement. The MCP account owner watches the database parent directory for DB, WAL and journal changes and revalidates local credential revisions without refreshing tokens. External replacement/removal cancels the old generation; metadata-only changes preserve it.

After rotation persistence, publication rechecks the local credential revision, rejecting an obsolete client even if its change notification was consumed before publication. Losing the directory or database path stops old sessions and reports a fixed watcher error on close. Shutdown cancels the account lifecycle before joining the watcher, including database-lock waits; normal shutdown cancellation is not reported as watcher failure. On kqueue, an ENOENT lstat for the just-removed SQLite journal is treated as a commit notification only while the parent watch and regular main database still exist; current credential revisions are revalidated. Other watcher errors, directory/database loss and failed revalidation remain fatal.

Background animation metadata, ZIP transfer and native encoding observations carry their actual failure outcome. ZIP success is reported after close and Content-Length validation; encoding success is reported after the encoder returns. The final generation event contains the complete animation size on success and zero on failure or cancellation. Partial ZIP bytes describe bytes transferred, not a successfully delivered animation.

Shared artwork lookup and session acquisition observations also distinguish failed results. OAuth refresh includes rotated-credential persistence in its outcome, so persistence failure is not reported as refresh success. Cancellation while acquiring the rotation gate records a fixed request-cancelled reason and does not proceed to refresh or content reads. Existing error returns and retry behavior remain unchanged.

Before publishing an animation subscription, authorization is checked again after session acquisition. Grant notifications read their snapshot under the job-publication mutex. Idle static metadata and image entries are actively reclaimed on grant revocation, credential-generation invalidation and expiry; pinned content remains charged until its readers release it. Late cache fills validate current grant and generation under the cache mutex. Artifact deletion failures retain their occupied budget and are reported on shutdown even if final cleanup later succeeds; HTTP and cleanup shutdown errors are joined rather than discarded.
