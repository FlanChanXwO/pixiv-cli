---
slug: pixiv-cli
version: 1.0.0
displayName: Pixiv CLI
summary: Safely operate Pixiv with the pixiv-cli binary for discovery, account actions, and downloads.
license: MIT
homepage: https://github.com/FlanChanXwO/pixiv-cli
tags: [pixiv, cli, mcp]
name: pixiv-cli
description: Operate Pixiv through the pixiv-cli binary — search illustrations, novels, and users; reverse-search images with SauceNAO or ascii2d; inspect Pixiv artwork or user IDs/URLs; view rankings and recommendations; manage bookmarks/follows; and download works. Load only when the user explicitly mentions Pixiv or pixiv-cli, provides a pixiv.net URL or ID in a clear Pixiv context, or requests a specific Pixiv operation or `pixiv` command. Do not trigger for generic illustration, artist, image-search, or download requests without Pixiv context. Verify current syntax with `pixiv <cmd> --help`.
---

# pixiv-cli Operator

Teaches an agent to drive the `pixiv` CLI correctly. This skill encodes workflow
orchestration, safety rules, and semantic traps — flag details always defer to
the installed binary's `pixiv <cmd> --help` output.

## Preflight and account checks

1. Run `pixiv --version` and require one line in the form `pixiv <version>`.
   The binary itself is the only environment probe: if it is missing or not
   executable, report that blocker. Install only when the user explicitly asked
   for installation; then read
   `references/install.md` and use the official platform script. Otherwise do
   not install or guess an installation method.
2. Do not enumerate local accounts on every session. Run
   `pixiv auth list --json` only when authentication, account selection, or an
   anonymous-fallback decision actually requires it. Presence in this list does
   not prove a credential is currently valid; use the networked
   `pixiv auth check [UID] --json` only when validation is needed. Treat both
   `{"accounts": null}` and `{"accounts": []}` as an empty account list.
   `pixiv auth refresh [UID] [--all]` rotates saved OAuth credentials and forces
   a profile/Premium-cache refresh, so run it only for an explicit maintenance request.

## Hard rules

1. Refresh tokens and authentication export bundles contain secrets. Except
   for the explicit bare-stdout export case below, do not echo, log, summarize,
   or reproduce them in commentary or results. Use the controlled workflows in
   `references/auth.md` when the user's task requires moving a credential.
2. A bare `auth export` is allowed only when the user explicitly asks to
   receive or see its raw token or bundle for that invocation. Before running
   it, explain that the secret necessarily enters tool output/transcript and
   may be retained there, then obtain the user's explicit confirmation. After
   execution, do not repeat, transform, parse, or log the secret. This is the
   sole exception to rule 1; otherwise use `--output` or a direct same-command
   pipeline to the intended consumer.
3. If the user already disclosed a refresh token in the conversation and
   explicitly asks to import it, the agent may run the positional
   `pixiv auth import 'RFT'` form. Do not create an extra file or repeat the
   secret in the result. The token will still be copied into the tool call,
   process arguments, shell history, and process context; disclose that before
   execution, and explain that an already-disclosed token cannot be erased.
4. For a token not already disclosed, keep it out of chat and command
   arguments. Use the hidden `pixiv auth import` prompt only when the runtime
   gives the user a terminal they can type into directly. Do not start it in a
   standard agent PTY with no user-input channel and leave it waiting; instead,
   give the command to the user for their private terminal, or use an authorized
   secret-manager-to-stdin pipeline. Non-TTY input is read automatically; there
   is no `--stdin` flag. This is an environment constraint, not a command ban.
5. Before running `pixiv auth login`, read [`references/auth.md`](references/auth.md)
   and select the route from the machine that stores the account. Run interactive
   login only when the user explicitly asks and is present to complete browser OAuth.
   A server with both relay settings prints a one-login hand-off URL. Opening
   it transfers directly to an installed desktop CLI, which claims that session,
   opens its OAuth page, and returns its callback. A remote login requires that
   desktop handler; it has no project confirmation page or manual callback form.
   Do not invent relay URLs or callback values.
6. Authenticate with a raw Pixiv App API refresh token through the documented
   configuration or environment variables.

## Operation tiers

| Tier | Commands | Behavior |
| --- | --- | --- |
| Credential transfer | `auth import` `auth export` | Execute only for the user's explicit import/export task; follow `references/auth.md` so secret input/output is not exposed accidentally |
| Read | `search` `detail` `ranking` `series` `comment` `bookmark list/tags/detail` `recommended` `timeline *` `mypixiv *` `user *` `config get/path` root `--version` `update --check` | Execute when the user's task requires it |
| Account diagnosis | `auth list/check` | List only for authentication/account/fallback decisions; check only when network validation is needed |
| Account maintenance | `auth refresh` | Rotates saved OAuth credentials and refreshes the cached account profile/Premium status; run only on an explicit request |
| Write | `bookmark add/remove` `follow add/remove` | State the target (illust/user ID) in one line before executing; for NDJSON stdin actions, state the record type and scope before starting |
| Disk | `download` | Confirm target directory and exact targets (IDs or supported Pixiv URLs) before each invocation; a user URL expands every visual work, so state that scope explicitly; approval never carries over; see `references/download.md` |
| Interactive credential | `auth login` | Read `references/auth.md`, then run only on an explicit request while the user is present for browser OAuth; use the one-time desktop hand-off URL when the account host is remote |
| Account/config state | `auth use/remove` `config set/unset` `update` (actual install) | Ask for explicit confirmation each time; approval does not carry over |
| MCP server | `mcp` | Run only when the user explicitly asks to start it; it is a long-lived stdio JSON-RPC server, not a data command—do not auto-probe, auto-wait, or include it in preflight |

## Output & token control (in priority order)

1. **Reduce at the source (preferred):** pass a positive `--limit N` only when
   that command's help exposes it. In the audited binary this includes list
   forms of `search`, `novel search`, `ranking`, `series`, `comment`, `bookmark list/tags`,
   `recommended`, `timeline`, `mypixiv`, and `user`. Add `--page`, `--type`,
   or other flags only when that specific command's help exposes them.
   `--limit 0` requests all results, so never use it unless the user explicitly
   asks for everything. A result that reaches `--limit N` is not proof that
   only N matches exist; follow the pagination and completeness contract in
   [`references/discover.md`](references/discover.md#pagination-and-completeness).
2. **Small result, display only:** use the default text output and
   relay it. JSON carries field names and metadata — it is *larger* than the
   table for display purposes.
3. **Programmatic processing** (extract IDs, filter, chain into a next
   command): visual lists automatically emit canonical NDJSON when stdout is a
   pipe. Use `--ndjson` when explicit output is clearer. The current v1 search
   contract has no top-level expression filter; use the supported typed search
   flags and inspect records before passing them to a compatible action.
   NDJSON is streaming: inspect the selected records before passing them to a
   compatible action; do not add a collector merely to make a pipeline
   convenient. `--json` remains for one complete result document when a command
   exposes it, but cannot be combined with `--ndjson`.
4. **Opportunistic tooling:** probe once for `jq`; if present, prefer
   `--json` + `jq` for field selection. If absent, fall back to tier 3
   silently — never ask the user to install anything.
5. **Check status before parsing JSON:** `--json` controls successful output;
   it does not guarantee that usage, validation, flag, or authentication errors
   are JSON. Inspect the exit code first. On failure, expect stdout may be empty
   and report the plain stderr error rather than treating it as malformed JSON.

## Command cheat sheet

Verify flags with `--help` before use; this list is orientation, not a contract.

```
pixiv auth list --json                    # only when an account decision needs it
pixiv auth pool status --json              # inspect non-secret database scheduling state
pixiv auth pool enable UID...              # enable selected local accounts for pooling
pixiv auth pool disable UID...             # disable selected local accounts for pooling
pixiv auth check [UID] --json             # validate token, shows user_id/username
pixiv auth refresh [UID] --json           # explicit maintenance: token + Premium cache
pixiv auth import                         # hidden TTY prompt, or automatic non-TTY stdin
pixiv auth import < bundle.json           # restore a versioned bundle offline
pixiv auth export UID --output PATH       # write one private versioned bundle
pixiv auth export --all --output PATH     # write all accounts to a private bundle
pixiv auth use [UID]                      # switch default account (confirm first)
pixiv config path                         # print location; creates baseline config if missing
pixiv config get download_path            # read one effective setting
pixiv config set download_path ./downloads # config write; confirm first
pixiv search "WORD" --type artwork --limit 10 --json
pixiv search "WORD" --type novel --limit 10 --json
pixiv search "NAME" --type user --limit 10 --json
pixiv search IMAGE_PATH_OR_URL --provider saucenao --json
pixiv search IMAGE_PATH_OR_URL --provider all --ndjson
pixiv search "WORD" --content-type manga --ai-mode exclude
pixiv search "WORD" --resolution high --aspect-ratio landscape --draw-tool "CLIP STUDIO PAINT"
pixiv search --trending-tags --json
pixiv detail ARTWORK_ID_OR_URL --type artwork --json
pixiv detail NOVEL_ID --type novel --content --json
pixiv series SERIES_ID --type novel --limit 20 --json
pixiv comment ID --type artwork --limit 20 --json
pixiv bookmark list --type artwork --limit 20 --json
pixiv bookmark tags --limit 20 --json
pixiv bookmark detail ARTWORK_ID --json
pixiv user novels USER_ID --limit 20 --json
pixiv ranking --mode day
pixiv recommended --type artwork --limit 10 # type is required; needs auth
pixiv recommended --type all --limit 10     # request all supported kinds; needs auth
pixiv timeline following --type artwork --content-type illust --limit 20
pixiv timeline latest --type artwork --limit 20 # defaults to the supported illust feed
pixiv timeline latest --type novel --limit 20
pixiv mypixiv works --type artwork --limit 20
pixiv user search "WORD" --limit 10 --json # authenticated App user search
pixiv user detail USER_ID --json          # full public profile (USER_ID required)
pixiv user artworks [USER_ID] --limit 20  # omit USER_ID = current account
pixiv user bookmarks [USER_ID] --tag TAG --limit 20
pixiv user following [USER_ID] --limit 20
pixiv bookmark add ILLUST_ID --tag TAG    # --tag repeatable; write op
pixiv bookmark remove ILLUST_ID           # write op
pixiv follow add USER_ID                  # write op
pixiv follow remove USER_ID               # write op
pixiv download [SRC...] [--pages 1,3-5] [--quality original|regular|small|thumb|mini] [--ugoira-mode gif|apng] [--output DIR] [--on-error skip|fail-fast]
pixiv update --check --json               # read-only update check
```

Public positional commands can fill one missing value from non-TTY stdin. The
stream is read as one complete value and only one final LF or CRLF is removed;
spaces and internal newlines are preserved. An explicit positional value wins,
an empty optional stream leaves the original omission/default behavior intact,
and commands missing two or more required values do not read stdin. This
applies to queries such as `pixiv search`, `detail`, `series`, `comment`,
`user`, `config`, and account operations. `download` and bookmark/follow
actions classify stdin as strict canonical NDJSON when its first non-whitespace
byte is `{`; otherwise they use one raw ID/URL value. Once selected, a mode
does not fall back to the other mode. `-` is ordinary positional text and is
not a stdin sentinel.

Data commands use the account selected by `pixiv auth use` when the pool is disabled;
when `[account_pool].enabled = true`, the database selects eligible local accounts
for safe non-mutating reads and downloads. The pool's `schedulable` flags are managed
with `pixiv auth pool status|enable|disable`; the config stores only `enabled` and
`strategy`. Data commands do not accept credential-selection flags and use the
local account selected by `pixiv auth use`.
Common data flags are command-scoped `--json` or `--ndjson`, plus `--proxy URL` /
`--no-proxy` (this command only, never persisted). Proxy URIs may use `http`,
`https`, `socks5`, or `socks5h`.

`pixiv search` also recognizes reverse-image sources. An explicit case-insensitive
`http:`/`https:` input always uses image mode, including an invalid URL (which
must fail without keyword fallback); other inputs use image mode only when they
are existing regular files after symlink resolution. Image mode accepts only
`--provider`, output flags, and proxy flags. It does not accept keyword filters,
`--type`, pagination, or trending flags and does not open the Pixiv account pool
just to resolve output settings.

`pixiv config` manages `account_pool_enabled`, `account_pool_strategy`,
`download_path`, `filename_template`, `directory_template`, `request_interval`,
`https_proxy`, `log_level`, `log_format`, `reverse_search_provider`, and
`reverse_search_pixiv_only`. `saucenao_api_key` is a sensitive config key: set
it only through non-TTY stdin (`printf '%s\n' 'KEY' | pixiv config set
saucenao_api_key`), and `pixiv config get saucenao_api_key` is always
`<redacted>`. `SAUCENAO_API_KEY` overrides the file without being displayed.
Other TOML settings are hand-maintained; inspect the installed help before suggesting them.

FANBOX is a separate read-only service surface. Import its `FANBOXSESSID` with
`pixiv fanbox auth import`, select it with `pixiv fanbox auth use --auto` or an
explicit UID, and inspect creators, posts, tags, home/supporting feeds, or
resources using the installed `pixiv fanbox --help` output. `pixiv fanbox mcp`
uses its own runtime credential selection and does not reuse the Pixiv account
pool.

Request pacing is configured with `PIXIV_REQUEST_INTERVAL` or
`[network].request_interval`. Set `log_level=debug` (or
`PIXIV_LOG_LEVEL=debug`) to enable safe typed diagnostics; use `log_format=json`
or `PIXIV_LOG_FORMAT=json` for one JSON event per stderr line. MCP stdout remains JSON-RPC, and `auth export`
plus hidden callback/installer paths retain their quiet/secret-output contracts.
FANBOX's optional `[fanbox.flaresolverr]` table is a challenge-only recovery
route; it does not proxy ordinary API/resource requests or receive the FANBOX
session.

## Critical semantics (traps — read before assuming a bug)

1. **No anonymous Web fallback.** v1 removed the anonymous Web API. Every
   content command requires an authenticated local account selected through
   `pixiv auth use`, or an eligible database account when the account pool is
   enabled; without one it returns an authentication requirement. App API
   errors are surfaced as-is and NEVER auto-fall back to a Web path. A missing
   App page resource is a real error, not a reason to scrape or retry Web.
2. **`recommended` requires a kind.** Choose one of the kinds shown by
   `pixiv recommended --help`; it requires authentication and does not work
   anonymously.
3. **`--limit` is command-specific.** Verify the installed help before adding it;
   list forms of `search`, `novel search`, `ranking`, `series`, `comment`, `bookmark`,
   `recommended`, `timeline`, `mypixiv`, and `user` expose it where applicable.
   Where supported, a positive value fills logical results across upstream
   batches and `0` traverses the current upstream result to exhaustion. `--page`
   requires a positive `--limit`.
4. **Search flags are command-scoped.** `search` uses `--type artwork|novel|user`
   for the entity route and `--content-type` for artwork subtype. Verify
   `--search-by`, `--period`, `--start-date`, `--end-date`, `--sort`, `--ai-mode`,
   `--aspect-ratio`, `--resolution`, `--draw-tool`, bookmark bounds, and
   `--bookmark-strategy` against `pixiv search --help`. `--rating` is a retained
   compatibility flag whose non-empty value is rejected; it is not a filter.
   `novel search` exposes only its basic search-by/sort/period contract and does
   not publish rating, text-length, or original-only filters.
   `timeline latest --type artwork` defaults `--content-type` to `illust` and
   accepts `illust|manga`; it does not use search's broader `all` subtype.
   `mypixiv works --type artwork` maps the public artwork entity to Pixiv's
   `illust` feed; the older `--type illust` spelling remains compatible.
5. **Restricted search fails explicitly.** There is no anonymous search path.
   Restricted rating requests are not represented by a silent `--rating` filter;
   use the command's actual authenticated/API contract and surface failures.
   Bookmark-count bounds use the application strategy/completeness result:
   Premium is not a local hard gate, `auto` currently means local candidate
   filtering, `best_effort` is explicitly partial, and `server` fails until
   reliable evidence exists. Do not present a strategy error as an empty result.
   `novel search` is App-only and requires authentication. Bookmark count is a
   public bookmark total, never a like count.
6. **Extended rankings need authentication.** Valid modes are `day`,
   `day_male`, `day_female`, `week`, `week_original`, `week_rookie`, `month`,
   `day_manga`, `week_manga`, `month_manga`, `week_rookie_manga`, `day_r18`,
   `day_male_r18`, `day_female_r18`, `week_r18`, `week_r18g`. The final nine
   must not be replaced with an anonymous day ranking.
7. **Empty filtered batches are skipped.** With application-side bookmark
   filtering, search continues past leading empty upstream batches to the first
   non-empty logical batch or true end; `--limit N` fills logical results and
   `--limit 0` walks the current filtered result. Do not invent request caps.
8. **No like-count field.** Do not invent or label bookmark totals as likes.
9. **`update --json` is only valid with `--check`.** The actual install never
   emits JSON.
10. **Proxy is per-command or service-scoped.** The browser's system proxy is
   NOT inherited. Pixiv command overrides take precedence over
   `[pixiv.network].proxy_url`, environment, and the global `[network]` value;
   persist the global value with `pixiv config set https_proxy URL` when that is
   the intended scope. `--proxy` / `--no-proxy` are mutually exclusive. HTTP,
   HTTPS, SOCKS5, and SOCKS5H proxy URIs are supported. FANBOX has independent
   `[fanbox.network].proxy_url` and `user_agent` settings, plus an optional
   `[fanbox.flaresolverr]` challenge-only route. With an explicit Pixiv proxy,
   resource downloads deliberately use HTTP/1.1; App API and OAuth retain
   normal protocol negotiation.
11. **Long downloads may legitimately take time.** Do not impose an arbitrary
   timeout or kill the process merely because it is slow; wait for completion,
   user cancellation, or a real error.
12. **Tag search has query grammar.** `user bookmarks --tag TAG` filters
   bookmark listings; `bookmark add --tag TAG` adds a repeatable bookmark tag.
   `search` has no `--tag` flag — put the tag expression in its required `WORD`.
   For a reliable boolean tag query, use `--search-by tag-exact`: `tagA tagB`
   requires both complete tags, and uppercase `tagA OR tagB` accepts either.
   Literal `AND` is not an operator. The default `tag-partial` also accepts the
   verified uppercase `OR` syntax, but its fuzzy/alias/translated matches are
   not a strict exact-tag AND. `title-caption` and App-only `tag-title-caption` have no boolean-tag contract;
   no literal-uppercase-`OR` escape syntax is verified.
13. **Direct URLs are intentionally narrow.** `detail` accepts only an artwork
    ID or a `pixiv.net`/`www.pixiv.net` HTTPS `/artworks/{id}` URL (an optional
    locale, query, or fragment is harmless). `download` also accepts `/users/{id}`
    and `/users/{id}/artworks`, plus `/users/{id}/bookmarks/artworks`. These
    expand visual works in first-seen artwork-ID order; user and bookmark
    downloads use App OAuth. Artwork-series URLs are rejected as unsupported
    download sources.
14. **Reverse-image search has a separate privacy and result contract.** The
    providers are `saucenao`, `ascii2d-color`, `ascii2d-bovw`, and `all`; the
    default is `reverse_search_provider=saucenao`, and `--provider` is a
    one-command override. `reverse_search_pixiv_only=true` keeps only explicit
    Pixiv artwork/user identities in the canonical result set. The source is
    loaded once into a private snapshot and may be uploaded or retained by a
    third party, so use only authorized images/URLs. JSON returns
    `input/providers/results/records/provider_errors/partial`; piped or explicit
    NDJSON returns only canonical records. Reverse-search artwork records use
    generic `type="artwork"` (not `illust`) because the provider does not prove
    the Pixiv subtype. With `all`, one success plus one failure is `partial` and
    exits successfully with a stderr warning; one-provider or all-provider
    failure is non-zero. Never expose source, key, temporary path, CSRF,
    redirect location, or upstream response body.

## Routing

| Task | Read |
| --- | --- |
| Explicitly install or repair the missing `pixiv` binary | `references/install.md` |
| Import, export, back up, or restore authentication | `references/auth.md` |
| Find works/artists (keyword and reverse-image search → filter → detail chains) | `references/discover.md` |
| Download workflows (single, batch, ugoira) | `references/download.md` |
| Errors: auth failures, network/proxy, empty results | `references/troubleshooting.md` |

## Image delivery for agents

After `pixiv download` or MCP `download`, share local file paths via the host attachment API. If the host cannot attach files, share the artwork `url` only and do not claim an image was sent.
