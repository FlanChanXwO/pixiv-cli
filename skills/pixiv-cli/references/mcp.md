# MCP operation

## Tool names and migration

Use `pixiv_comments` and `pixiv_comment_create|reply|stamp|delete` with required `kind=artwork|novel`. Create/reply/stamp take positive `id`; delete takes `comment_id`. `pixiv_bookmark_add|remove` also require `kind` and positive `id`; add retains `restrict` and `tags`. Mutation outputs retain the original action and `illust_id`/`novel_id` fields. These are independent actions with distinct approval hints, not an `action` dispatcher.

Read selectors are explicit: `pixiv_user_bookmarks(kind=artwork|novel)`, `pixiv_user_relations(relation=followers|following|related)`, `pixiv_timeline_following(kind=artwork|novel)`, `pixiv_timeline_latest(content_type=illust|manga|novel)`, and `pixiv_mypixiv(kind=artwork|novel|user)`. Keep matching typed filters only; novel user bookmarks have no new filter, and followers do not accept `user_filter`. Related users accept but ignore `restrict`. `pixiv_bookmark_list_all` retains artwork-then-novel pagination; the two recommendation tools remain distinct.

Use `fanbox_creator_posts` with `creator_id` and an optional non-empty `tag` (omit it for untagged creator posts); `fanbox_feed` requires `source=home|supporting`. The old type-specific names and `fanbox_tagged_posts|home|supporting` are unregistered. Use current discovery and the [migration table](https://github.com/FlanChanXwO/pixiv-cli/blob/main/docs/en/mcp-tools.md#tool-migration), not cached tool metadata.

## Choose the transport

`pixiv mcp` defaults to local stdio for both Pixiv and FANBOX, with JSON-RPC on stdout and diagnostics on stderr. Use local CLI authentication; no HTTP owner is needed. Only explicit `--port PORT` (1–65535) enables HTTP, for example `pixiv mcp --port 1270` after authorized owner initialization. `--host` defaults to `127.0.0.1`; non-loopback hosts require `--base-url`, and non-loopback public URLs require HTTPS. `--host`/`--base-url` alone fail.

An existing HTTPS proxy may use `pixiv mcp --port 1270 --host 0.0.0.0 --base-url https://mcp.example.com`. The unpublished `mcp_listen_addr`/`mcp_base_url` settings no longer control startup; use flags. Do not deploy, restart or alter persistent settings without authorization. `pixiv fanbox mcp` migrates to the unified entry.

## Cache and temporary storage

Private animation temporary storage is configured by mcp_gallery_animation_temp_capacity_mib (1024; positive integer, separate from memory, startup/restart only). The Gallery cache budget is `mcp_gallery_cache_capacity_mib` (256; 0 disables caching); the 15-minute cache lifetime and the read limit of 4 are fixed. The removed `mcp_gallery_cache_ttl` and `mcp_gallery_concurrency` settings return `removed_setting` until cleared with `pixiv config unset`. The remaining settings map to the corresponding `gallery_*` keys under `[mcp]` and support `config get/set/unset`; changes require an explicitly authorized MCP restart.

The in-memory budget includes metadata and image filling buffers. Successful artwork metadata may lag updates within the 15-minute TTL; bookmark state and writes stay live. Grant/credential/network changes invalidate access to cached content and reclaim idle cache entries; expiry also releases idle entries without another read. Active cached transfers retain their budget until release. Late fills cannot repopulate invalidated content. Large images stream without caching or truncation.

Do not promise fixed network latency or restart a running server merely to apply configuration. Cancelling one MCP credential check preserves the shared read session; real credential or grant invalidation still cancels affected media.

## Credentials and transport capabilities

One MCP server exposes `pixiv_*` and `fanbox_*` tools with independent product credentials. Use discovery for exact names; unprefixed Pixiv aliases do not exist. Stdio exposes `pixiv_download` and `pixiv_download_random_from_recommendation` through the shared CLI download configuration; HTTP never registers filesystem downloads. Gallery, animation subscriptions and owner tools are HTTP-only. `pixiv_novel_bookmark_tags` migrates to `pixiv_bookmark_tags(kind="novel")`.

Standard read-only, destructive, idempotent and open-world annotations are approval hints, not a replacement for the user's authorization. Reverse search uploads the chosen source to third-party providers. MCP paths refer to the server filesystem, and URLs are fetched from the server network; authorize only trusted connectors. HTTP OAuth uses DCR, owner consent and PKCE, with bearer validation on every HTTP MCP request.

Do not treat a local fixture or successful launch as proof that a cloud connector completed authorization.

## Reverse-search attachments

For MCP reverse search, provide exactly one of `source` (a server-local regular-file path, local `file://` URI, or HTTP(S) URL) or `image`. The tool publishes `_meta["openai/fileParams"] = ["image"]`; the top-level image object has required string `download_url` and non-empty `file_id`, plus optional string `mime_type` and `file_name`, with no extra fields. Use only the host-supplied HTTP(S) download URL; file IDs are not addresses.

If it expires or cannot be read, ask the user to reattach instead of guessing or retrying. File URIs require an absolute path, empty/`localhost` authority and no userinfo/query/fragment/UNC path; percent-encode literal path characters. Other hosts may use HTTP(S) sources; this does not imply live connector compatibility has been verified. Upload authorization is separate from MCP login. Never echo the source URL, file ID, file name or private path in results/logs.

These file URI and attachment rules do not change CLI search mode selection.

## Add an account through HTTP

For a new Pixiv account, `pixiv_account_login_start` returns a server relay URL and `login_id`; open the URL on a computer with the installed pixiv-cli helper. Query `pixiv_account_status` with that ID for progress. Restart only when explicitly intended: `restart=true` cancels the previous flow. Never paste callbacks or credentials into chat. A saved account with failed MCP selection does not require another login; repair the write issue and use `pixiv_account_use`.

Login preserves any MCP selection with local credentials; it adopts the saved account only when the current selection is unset, missing or lacks credentials. A completed login with `selection_updated=false` is successful and preserves that selection; use `pixiv_account_use` for an intentional switch. CLI default selection is not changed.

## Deliver full media

For static artwork media, call `pixiv_artwork_media` with `illust_id`; default quality is `regular` and omitted `pages` means every page. Use `original` only on explicit request, or `thumbnail` for previews. Inspect `complete`/`failures` and each page's `content_index`; a host-rejected response is not delivery. For Ugoira omit static `pages`/`quality`; `animation_format` is `gif` by default or `apng`.

When the client advertises the standard MCP Apps HTML MIME capability, use `_meta["pixiv/mediaContents"]` as the indexed media array; model-facing `content` contains only the PNG preview. A published animation is a protected `resource_link`: use the App playback/download controls or retrieve that authorized URL, never treat its preview as the full file or log its signed address. If private publication is unavailable, the array retains full inline image contents.

Clients without App support use the original full `content` array. Save the full GIF/APNG `ImageContent` bytes at `content_index` using its `filename`; `preview_content_index` is only a separate PNG preview. APNG full ImageContent uses the PNG-family `image/png` MIME while structured metadata keeps `image/apng` and `.apng`; distinguish animation from preview by the explicit indexes, not MIME alone. `archive_quality` records the actual selected source archive. Do not pass `animation_format` for static artwork.

The attached media App can play the full animation and show its static preview. Download uses the advertised host confirmation flow, or an available protected browser attachment from private UI metadata; temporary links can expire or be invalidated. Never substitute the preview for the full file. ZIP-read diagnostics distinguish incomplete reads (including length mismatches), connection resets and timeouts using fixed markers; no marker triggers an automatic retry.

### Encoding behavior

GIF encoding omits unchanged rectangles only for opaque animations, preserving quantized pixels and every frame delay; transparent animations keep full-frame background clearing.
APNG encoding removes unchanged pixels with lossless differences when alpha permits; it retains all frames, dimensions and timing. Do not infer host delivery success from a smaller generated file.

## Gallery interaction and delivery

MCP Apps hosts may render Gallery cards linked to visual discovery tools. Gallery tools advertise a frontend-content revision URI for host caching; the stable Gallery resource and valid historical revision aliases remain readable. Refresh the existing plugin tool metadata after deploying an updated UI, then verify the rendered revision rather than inferring freshness from server restart. Cards display thumbnails only, including Ugoira covers.

Static page-1 thumbnails are fetched only when visible; one authenticated batch renews their capabilities before loading historical results. Detail omits the redundant Pages field and opens valid author IDs through host user-page links. Opening detail supports selected static pages with explicit partial status. Click/double-click the detail image or press Enter/Space while it is focused for fit/actual-size viewing and host-confirmed original download.

A separate 256MiB estimated browser-session LRU reuses loaded static pages, successful animation/PNG elements, metadata and original bytes across paging and detail reentry; new results and teardown clear it, oversized images render without retention, and bookmark state remains live. Detail provides live bookmark state with click-triggered public/private/remove operations that preserve tags. Writes remain subject to host approval; refresh state after an unconfirmed write before retrying.

Animation detail uses pixiv_gallery_animation prepare/status/release with two-second status polling and protected HTTP GIF/APNG resources plus independent PNG previews. Release the opaque subscription and remove view callbacks on leaving the view; stop polling on failure and retry explicitly. Successful animation/preview elements share the existing estimated session LRU.

Detail reentry obtains a new prepare response to reauthorize before reusing elements with identical media URLs; failed, incomplete and over-budget media are not retained. New results/teardown and LRU eviction release retained sources. Animation estimates count encoded bytes and pixels for all frames using the private UI hint `_meta["pixiv/animationFrameCount"]`; unknown frame counts are not newly retained. PNG estimates use decoded pixel allowances, not measured compressed bytes or total browser/GPU memory.

Ready preparation hides the redundant Load animation action; delivery failure, an unconfirmed host download or changing format restores it without automatic preparation. The existing pixiv_artwork_media retains full inline delivery for ordinary clients and negotiated protected-original delivery for App clients. Use Refresh animation to renew an expired linked animation in its original format; failed playback does not automatically prepare again. Host downloads accept resource_link URLs.

When downloadFile is absent, user-clicked browser attachment links deliver animations or the selected static original through protected HTTPS; the static original obtains an original-quality capability through the existing Gallery tool, avoiding host-rejected Blob links. Browser download policy still applies, and do not infer a saved file from a link click.

Download controls use standard ui/download-file only when the host advertises downloadFile; denied or failed responses are not success, and host-confirmed downloads do not prove a particular save path or name. The view follows host theme/container constraints without injecting external CSS or fonts. With standard host toolInfo and complete tool-input, the view can append positive-limit pages using explicit next_page, preserving query filters and independently advancing mixed recommendation kinds.

Failed continuation retains loaded records; retry is explicit. For pixiv_search_illust, pixiv_illust_related, pixiv_illust_ranking and pixiv_illust_recommended default batches, pass pagination.next_cursor unchanged as cursor with the same original arguments (including search query/strategy, ID, ranking mode/date and filters where applicable), omitting page/limit; the Gallery supports this route. The cursor contains no credentials and is not an authorization token; SDK account binding still applies.

For pixiv_recommended default batches, each pagination stream may return its own next_cursor: continue with that stream’s kind, retain only its applicable filters, and omit page/limit. Do not send a stream cursor with kind=all or another kind, and do not invent cursors/page sizes. Local mocks do not prove live display, playback or download support.

## FANBOX resource delivery

For FANBOX resource bytes, use `fanbox_open_resource` with its opaque ref and GET (default). HEAD is metadata-only. Check `complete`, `delivered`, `size`, and `content_index`; file/GIF/APNG payloads are embedded blobs with content-hash URNs, not server paths. Use the optional `filename` suggestion derived from SDK file metadata; MCP normalizes path separators/reserved filename punctuation, but the receiving client must still choose and validate its own destination. Do not invent a name from the resource URL when filename metadata is absent. Do not claim host receipt when the response is rejected.

## Initialize the HTTP owner

Local `pixiv mcp auth init [--reset]` is owner administration, not an MCP tool or server-start probe. Run it only on an explicit setup/reset request: it prints a new secret once after committing only its verifier. Never copy that secret into chat or tool arguments. Ordinary re-init does not reveal it; reset revokes OAuth grants but preserves registered clients and MCP account selection. It does not start a listener. For HTTP, start `pixiv mcp --port PORT` separately after owner initialization. Local stdio needs no owner initialization or address configuration.
