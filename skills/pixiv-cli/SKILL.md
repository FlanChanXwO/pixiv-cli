---
slug: pixiv-cli
version: 1.1.0
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
| `mcp` or `fanbox mcp` | Explicit server-start request; these are long-lived stdio servers, not preflight probes |

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
