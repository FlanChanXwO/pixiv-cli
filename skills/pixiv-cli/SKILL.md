---
slug: pixiv-cli
version: 1.1.1
displayName: Pixiv CLI
summary: Safely operate Pixiv with the pixiv-cli binary for discovery, account actions, and downloads.
license: MIT
homepage: https://github.com/FlanChanXwO/pixiv-cli
tags: [pixiv, cli, mcp]
name: pixiv-cli
description: Operate the installed pixiv CLI to search works and users, reverse-search images with SauceNAO or ascii2d, inspect Pixiv references, read feeds and rankings, and perform explicitly authorized bookmark, follow, comment, account, or download actions. Use only for an explicit Pixiv or pixiv-cli request, a pixiv command, or a Pixiv URL/ID with clear operation context. Do not trigger for generic illustration, artist, image-search, or download requests. Verify syntax with pixiv COMMAND --help.
---

# Pixiv CLI Operator

Use the installed `pixiv` binary. This bundle describes operation, privacy, and semantic traps, not repository development. It works without a source checkout or personal maintenance skills.

## Preflight and routing

Run `pixiv --version` and require a successful `pixiv VERSION` line. A missing or non-executable binary is a blocker, not authorization to install it. Inspect the relevant command's `--help` before execution and read only the needed reference:

| Task | Reference |
| --- | --- |
| Explicit installation or repair | [install.md](references/install.md) |
| Login, import/export, backup/restore, account pool | [auth.md](references/auth.md) |
| Search, reverse image, detail, users, feeds, rankings, bookmarks | [discover.md](references/discover.md) |
| Single, multi-page, batch, or animated downloads | [download.md](references/download.md) |
| Authentication, proxy, output, or incomplete results | [troubleshooting.md](references/troubleshooting.md) |

Do not enumerate accounts on every turn. Use `pixiv auth list --json` only when an account/authentication decision requires it; both `accounts: null` and `accounts: []` mean none. Stored credentials do not prove current validity. Run networked `auth check` only when needed; `auth refresh` rotates credentials and refreshes profile/Premium metadata, so requires an explicit maintenance request.

## Secrets and side effects

Refresh tokens and export bundles are plaintext secrets. Keep them out of commentary, results, logs, and diagnostics. Before any import/export or browser login, follow `auth.md`: an explicitly requested bare stdout export has a narrow disclosure/confirmation workflow; every other export uses a private output or direct consumer pipeline. Do not inspect credential-store contents to debug an account.

Do not ask for an undisclosed token in chat or launch a hidden prompt in an agent terminal the user cannot type into. If the user already disclosed a token and explicitly requests positional import, first explain the additional tool/process-record exposure, safely quote it, and never repeat it in results. Non-TTY import reads stdin automatically; there is no `--stdin` flag. Strict bundle decoding must not fall back to OAuth.

| Operation | Required scope |
| --- | --- |
| Discovery and read commands | Execute only as needed for the task; do not add account probes or extra result traversal |
| `auth list/check` | Only an account decision or needed validation |
| `auth refresh` | Explicit account-maintenance request |
| Bookmark/follow/comment mutations | Explicit current targets, types, and action; state the scope before execution |
| `download` | Confirm exact targets and destination for each invocation; a user URL expands the whole supported visual listing |
| `auth use/remove`, pool enable/disable, `config set/unset`, actual `update` | Explicit current state-change authorization; it does not carry to later targets |
| `auth login` | Explicit request and a user present for browser OAuth; follow the documented desktop/remote handoff |
| `mcp` | Explicit server-start request; starts the unified long-lived HTTP server, not a preflight probe |

`config path` can create baseline configuration if missing. A successful read-looking command is not necessarily filesystem-side-effect-free. `update --check --json` checks only; actual installation does not emit JSON and requires authorization.

## Output and complete results

Use the requested scope and command-specific flags. A supported positive `--limit N` requests N logical results; reaching N is not proof that only N exist. Use `--limit 0` only for an explicit exhaustive request. `--page` requires positive `--limit`; an omitted limit normally means one logical batch, not the entire corpus. Local filters can skip empty upstream batches; do not invent request caps.

Use text for a small display-only result. Visual listing commands emit canonical NDJSON automatically when stdout is piped; `--ndjson` makes that explicit. `--json` is one complete result document where supported and cannot combine with `--ndjson`. Check status before parsing; failed commands may produce ordinary stderr and empty stdout. Do not install processing tools merely to format output.

```text
pixiv search "landscape" --type artwork --limit 20 --ndjson | pixiv detail
pixiv search ./image.png --provider all --ndjson | pixiv detail --ndjson
pixiv detail ARTWORK_ID --json
pixiv search "fantasy" --type novel --limit 10 --json
pixiv user search "NAME" --limit 10 --json
pixiv ranking --type novel --mode week --limit 10
pixiv recommended --type artwork --limit 10
```

Use these examples only for the requested operation and inspect selected records before any downstream action. Aggregate `search --json` is not a record stream. In `detail` record mode, record types choose artwork/novel/user endpoints; explicit `--type` is only a compatibility constraint. Record `--json` produces an array, not a single-object promise.

Public positional commands can fill one missing value from non-TTY stdin. Only a final newline is removed; an explicit positional value wins, and two missing required values do not trigger generic stdin reading. For `detail`, `download`, and bookmark/follow actions, an initial non-whitespace `{` selects strict record input; failures do not switch to raw ID input. `-` is ordinary text, not a universal stdin sentinel.

## Authentication and service boundaries

Pixiv content is App-only. Use the selected local `auth use` account or an eligible database-managed account when the pool is enabled. There is no anonymous Web fallback and no per-command `--uid`/`--refresh-token` selection or `PIXIV_REFRESH_TOKEN` credential shortcut. The public SDK and MCP have their own explicit runtime credential contracts.

Pool enabled/strategy settings are configuration; schedulable accounts are managed by `auth pool status|enable|disable`. Reads/downloads may use the pool, but mutations must not be silently assigned to a different account. Removed settings fail explicitly; use the authorized cleanup described in `auth.md`, not an automatic account migration.

FANBOX is separate: use `pixiv fanbox --help` and its read-only creator/post/tag/feed/resource capabilities. Importing `FANBOXSESSID` with `pixiv fanbox auth import` or selecting an account with `pixiv fanbox auth use --auto`/an explicit UID is a separately authorized credential/state action, not automatic preflight. FANBOX MCP does not reuse the Pixiv pool. Its optional challenge-only solver does not carry ordinary API/resource traffic or receive the session; never send the session to a non-first-party host.

## Search and identity traps

- Artwork `--rating sfw|r18|r18g|mature|all` filters normalized `x_restrict` locally and binds the filter to its cursor. It is not an upstream request field and is not supported for novel search. Artwork subtype, AI, resolution, aspect, and exact drawing-tool filters have their own validated scope.
- Bookmark counts are not likes. `auto`/`local` filter candidates locally, `best_effort` is explicitly partial, and `server` fails without verified upstream support. Premium is not a local hard gate. Do not present partial candidates as a complete site-wide result.
- `recommended` needs an entity kind and authentication; local artwork subtype filtering is not an upstream query. Rankings and feeds have command-specific kinds/modes; do not substitute a failed extended ranking with `day`.
- User detail/list routes take numeric user IDs where documented; not every command accepts a user URL. Follow writes accept a positive numeric ID or canonical user record, and `--restrict` is `public|private` with public default. Use only verified IDs and supported URL namespaces.
- `detail --type novel --content` is a compatibility flag whose unavailable App content endpoint returns `content_unavailable`; it is not permission to scrape a WebView. Plain novel detail remains metadata.

## Comment operations

Comment reads preserve optional `total` and `access_control`, including numeric upstream `comment_access_control`; do not invent boolean permission fields. `comment create|reply|stamp|delete` requires an explicit artwork/novel type and positive numeric IDs, not URLs or `all`. Create/reply needs non-empty `--comment`; reply also needs `--parent-comment-id`, and stamp needs `--stamp-id` with optional text. Confirm the user's exact content and target before writing.

Create/reply/stamp return the upstream comment ID. Delete returns a success status without an automatic read-back. `comment stamps` is a non-paginated read with safe opaque stamp references. Bookmark/follow actions retain empty-success stdout; do not invent a JSON report.

## Reverse search, proxies, and delivery

Reverse-search image sources are existing regular files or explicit HTTP(S) URLs; an invalid explicit URL is an error, never a keyword fallback. Binary image bytes on stdin do not select image mode. Provider/output/proxy flags apply; keyword filters, entity type, and pagination do not.

Providers are SauceNAO, ascii2d color/BoVW, and `all`. `reverse_search_pixiv_only` controls canonical Pixiv identities. Configure `saucenao_api_key` only through non-TTY stdin or `SAUCENAO_API_KEY`; reads redact it. Upload only authorized images and preserve provider privacy boundaries. One successful provider plus another failure can be a successful **partial** result with a warning; single/all-provider failure is nonzero. Generic reverse-search `type=artwork` records do not prove a Pixiv subtype.

`--proxy` and `--no-proxy` are mutually exclusive invocation overrides. Pixiv, FANBOX, reverse-search transport, and solver-browser settings have distinct scopes; the browser's proxy is not automatically inherited. Do not assume a private maintainer's loopback proxy exists. Read `troubleshooting.md` before changing persistent settings or solver configuration.

Downloads write files and have empty successful stdout; inspect exit status and the requested destination, following `download.md`. A legitimate long encode/download is not a timeout failure. Share generated files only through an available attachment API; otherwise report the source artwork URL and do not claim an image was sent.

## MCP transport and namespace

`pixiv mcp` requires explicit `--listen-addr` and `--base-url` values or persisted `mcp_listen_addr`/`mcp_base_url` settings. For an authorized local launch, use `pixiv mcp --listen-addr 127.0.0.1:8080 --base-url http://127.0.0.1:8080` and connect to `/mcp`. Cloud connectors need the configured public HTTPS base through an existing reverse proxy; do not deploy or change persistent configuration without authorization. The endpoint is printed to stderr; JSON-RPC uses HTTP, not stdin/stdout. `pixiv fanbox mcp` is removed.

One MCP server exposes `pixiv_*` and `fanbox_*` tools with independent product credentials. Use discovery for exact names; unprefixed Pixiv aliases do not exist. MCP filesystem download tools have been removed; use the explicitly authorized CLI download workflow for local files. Standard read-only, destructive, idempotent and open-world annotations are approval hints, not a replacement for the user's authorization. Reverse search uploads the chosen source to third-party providers. MCP paths refer to the server filesystem, and URLs are fetched from the server network; authorize only trusted connectors. OAuth uses DCR, owner consent and PKCE, with bearer validation on every MCP request. Do not treat a local fixture or successful launch as proof that a cloud connector completed authorization.

For MCP reverse search, provide exactly one of `source` (a server-local regular-file path, local `file://` URI, or HTTP(S) URL) or `image`. The tool publishes `_meta["openai/fileParams"] = ["image"]`; the top-level image object has required string `download_url` and non-empty `file_id`, plus optional string `mime_type` and `file_name`, with no extra fields. Use only the host-supplied HTTP(S) download URL; file IDs are not addresses. If it expires or cannot be read, ask the user to reattach instead of guessing or retrying. File URIs require an absolute path, empty/`localhost` authority and no userinfo/query/fragment/UNC path; percent-encode literal path characters. Other hosts may use HTTP(S) sources; this does not imply live connector compatibility has been verified. Upload authorization is separate from MCP login. Never echo the source URL, file ID, file name or private path in results/logs. These file URI and attachment rules do not change CLI search mode selection.

For a new Pixiv account, `pixiv_account_login_start` returns a server relay URL and `login_id`; open the URL on a computer with the installed pixiv-cli helper. Query `pixiv_account_status` with that ID for progress. Restart only when explicitly intended: `restart=true` cancels the previous flow. Never paste callbacks or credentials into chat. A saved account with failed MCP selection does not require another login; repair the write issue and use `pixiv_account_use`. Login preserves any MCP selection with local credentials; it adopts the saved account only when the current selection is unset, missing or lacks credentials. A completed login with `selection_updated=false` is successful and preserves that selection; use `pixiv_account_use` for an intentional switch. CLI default selection is not changed.

For static artwork media, call `pixiv_artwork_media` with `illust_id`; default quality is `regular` and omitted `pages` means every page. Use `original` only on explicit request, or `thumbnail` for previews. Inspect `complete`/`failures` and each page's `content_index`; a host-rejected response is not delivery. For Ugoira omit static `pages`/`quality`; `animation_format` is `gif` by default or `apng`. Save the full embedded blob at `content_index` using its `filename`; `preview_content_index` is only a separate PNG preview. `archive_quality` records the actual selected source archive. Do not pass `animation_format` for static artwork.

MCP Apps hosts may render Gallery cards linked to visual discovery tools. Static page-1 thumbnails are fetched only when visible. Opening detail supports selected/all static pages with explicit partial status, and bookmark state with click-triggered public/private/remove operations that preserve tags. Writes remain subject to host approval; refresh state after an unconfirmed write before retrying. Animation detail loads GIF/APNG as complete blobs with separate PNG previews. Download controls use standard ui/download-file only when the host advertises downloadFile; denied or failed responses are not success, and host-confirmed downloads do not prove a particular save path or name. The view follows host theme/container constraints without injecting external CSS or fonts. With standard host toolInfo and complete tool-input, the view can append positive-limit pages using explicit next_page, preserving query filters and independently advancing mixed recommendation kinds. Failed continuation retains loaded records; retry is explicit. For pixiv_search_illust, pixiv_illust_related, pixiv_illust_ranking and pixiv_illust_recommended default batches, pass pagination.next_cursor unchanged as cursor with the same original arguments (including search query/strategy, ID, ranking mode/date and filters where applicable), omitting page/limit; the Gallery supports this route. The cursor contains no credentials and is not an authorization token; SDK account binding still applies. For pixiv_recommended default batches, each pagination stream may return its own next_cursor: continue with that stream’s kind, retain only its applicable filters, and omit page/limit. Do not send a stream cursor with kind=all or another kind, and do not invent cursors/page sizes. Local mocks do not prove live display, playback or download support.

For FANBOX resource bytes, use `fanbox_open_resource` with its opaque ref and GET (default). HEAD is metadata-only. Check `complete`, `delivered`, `size`, and `content_index`; file/GIF/APNG payloads are embedded blobs with content-hash URNs, not server paths. Use the optional `filename` suggestion derived from SDK file metadata; MCP normalizes path separators/reserved filename punctuation, but the receiving client must still choose and validate its own destination. Do not invent a name from the resource URL when filename metadata is absent. Do not claim host receipt when the response is rejected.

Local `pixiv mcp auth init [--reset]` is owner administration, not an MCP tool or server-start probe. Run it only on an explicit setup/reset request: it prints a new secret once after committing only its verifier. Never copy that secret into chat or tool arguments. Ordinary re-init does not reveal it; reset revokes OAuth grants but preserves registered clients and MCP account selection. It does not start a listener. Start `pixiv mcp` separately after owner initialization and explicit address/base configuration.
