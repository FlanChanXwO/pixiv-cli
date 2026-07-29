---
slug: pixiv-cli
version: 0.8.0
displayName: Pixiv CLI
summary: Safely operate Pixiv with the pixiv-cli binary for discovery, account actions, and downloads.
license: MIT
homepage: https://github.com/FlanChanXwO/pixiv-cli
tags: [pixiv, cli, mcp]
name: pixiv-cli
description: Operate Pixiv through the pixiv-cli binary — search illustrations, novels, and users; inspect Pixiv artwork or user IDs/URLs; view rankings and recommendations; manage bookmarks/follows; and download works. Load only when the user explicitly mentions Pixiv or pixiv-cli, provides a pixiv.net URL or ID in a clear Pixiv context, or requests a specific Pixiv operation or `pixiv` command. Do not trigger for generic illustration, artist, image-search, or download requests without Pixiv context. Verify current syntax with `pixiv <cmd> --help`.
---

# pixiv-cli Operator

Teaches an agent to drive the `pixiv` CLI correctly. This skill encodes workflow
orchestration, safety rules, and semantic traps — flag details always defer to
the installed binary's `pixiv <cmd> --help` output.

## Preflight and account checks

1. Run `pixiv version --json`. The binary itself is the only environment probe:
   if it is missing or not executable, report that blocker. Install only when
   the user explicitly asked for installation; then read
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
5. Run interactive `pixiv auth login` only when the user explicitly asks and
   is present to complete browser OAuth.
6. Do not accept or forward cookies (`PHPSESSID`, `refresh_token=...` cookie
   strings) as credentials — the CLI rejects them by design; only raw Pixiv App
   API refresh tokens work.

## Operation tiers

| Tier | Commands | Behavior |
| --- | --- | --- |
| Credential transfer | `auth import` `auth export` | Execute only for the user's explicit import/export task; follow `references/auth.md` so secret input/output is not exposed accidentally |
| Read | `search` `novel search` `search-options` `detail` `ranking` `recommended` `feed *` `mypixiv *` `user *` `config get/path` `version` `update --check` | Execute when the user's task requires it |
| Account diagnosis | `auth list/check` | List only for authentication/account/fallback decisions; check only when network validation is needed |
| Account maintenance | `auth refresh` | Rotates saved OAuth credentials and refreshes the cached account profile/Premium status; run only on an explicit request |
| Write | `bookmark add/remove` `follow add/remove` | State the target (illust/user ID) in one line before executing; for NDJSON stdin actions, state the record type and scope before starting |
| Disk | `download` | Confirm target directory and exact targets (IDs or supported Pixiv URLs) before each invocation; a user URL expands every visual work, so state that scope explicitly; approval never carries over; see `references/download.md` |
| Interactive credential | `auth login` | Run only on explicit request while the user is present for browser OAuth |
| Account/config state | `auth use/remove` `config set/unset` `update` (actual install) | Ask for explicit confirmation each time; approval does not carry over |
| MCP server | `mcp` | Run only when the user explicitly asks to start it; it is a long-lived stdio JSON-RPC server, not a data command—do not auto-probe, auto-wait, or include it in preflight |

## Output & token control (in priority order)

1. **Reduce at the source (preferred):** pass a positive `--limit N` only when
   that command's help exposes it. In the audited binary these commands are
   `search`, `novel search`, `ranking`, `recommended`, `user search`, `user artworks`, `user bookmarks`, and
   `user following`. Add `--page`, `--type`, `--rating`, or other flags only
   when that specific command's help exposes them.
   `--limit 0` requests all results, so never use it unless the user explicitly
   asks for everything. A result that reaches `--limit N` is not proof that
   only N matches exist; follow the pagination and completeness contract in
   [`references/discover.md`](references/discover.md#pagination-and-completeness).
2. **Small result, display only:** use the default human-readable output and
   relay it. JSON carries field names and metadata — it is *larger* than the
   table for display purposes.
3. **Programmatic processing** (extract IDs, filter, chain into a next
   command): use `--ndjson`, then use `pixiv filter` or pass the records to a
   compatible action. NDJSON is streaming: do not add a collector merely to
   make a pipeline convenient. `--json` remains for one complete result
   document when a command exposes it, but cannot be combined with `--ndjson`.
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
pixiv auth check [UID] --json             # validate token, shows user_id/username
pixiv auth refresh [UID] --json           # explicit maintenance: token + Premium cache
pixiv auth import                         # hidden TTY prompt, or automatic non-TTY stdin
pixiv auth import --file PATH             # restore a versioned bundle offline
pixiv auth export UID --output PATH       # write one private versioned bundle
pixiv auth export --all --output PATH     # write all accounts to a private bundle
pixiv auth use [UID]                      # switch default account (confirm first)
pixiv config path                         # print location; creates baseline config if missing
pixiv config get download_path            # read one effective setting
pixiv config set download_path ./downloads # config write; confirm first
pixiv search "WORD" --limit 10 --json     # illustration search
pixiv search "WORD" --rating sfw --type illust --ai-mode exclude
pixiv search "WORD" --resolution high --aspect-ratio landscape --draw-tool "CLIP STUDIO PAINT"
pixiv novel search "WORD" --rating sfw --min-text-length 1000 --limit 10 --json
pixiv search-options "WORD" --json         # authenticated dynamic tool choices
pixiv detail ILLUST_ID_OR_URL --json      # single artwork detail
pixiv ranking --mode day
pixiv recommended illust --limit 10       # kind is REQUIRED; needs auth
pixiv recommended all --limit 10          # request all supported kinds; needs auth
pixiv feed following --type illust --limit 20
pixiv feed latest --type novel --limit 20
pixiv mypixiv works --type illust --limit 20
pixiv user search "WORD" --limit 10 --json # source labels App search vs related-author fallback
pixiv user detail USER_ID --json          # full public profile (USER_ID required)
pixiv user artworks [USER_ID] --limit 20  # omit USER_ID = current account
pixiv user bookmarks [USER_ID] --tag TAG --limit 20
pixiv user following [USER_ID] --limit 20
pixiv search "miku" --ndjson --limit 50 | pixiv filter --min-views 1000
pixiv search "miku" --ndjson --limit 20 | pixiv download --ugoira-format apng
pixiv bookmark add ILLUST_ID --tag TAG    # --tag repeatable; write op
pixiv bookmark remove ILLUST_ID           # write op
pixiv follow add USER_ID                  # write op
pixiv follow remove USER_ID               # write op
pixiv download [SRC...] [--pages 1,3-5] [--quality original|regular|small|thumb|mini] [--ugoira-format gif|apng] [--download-path DIR] [--concurrency N]
pixiv update --check --json               # read-only update check
```

Data commands use the account selected by `pixiv auth use`; they do not accept
credential-selection flags and ignore `PIXIV_REFRESH_TOKEN`. A manually maintained
`[account_pool]` can choose local accounts only for safe non-mutating reads and downloads.
Common data flags are command-scoped `--json` or `--ndjson`, plus `--proxy URL` /
`--no-proxy` (this command only, never persisted).

`pixiv config` manages only `download_path`, `filename_template`, and `https_proxy`.
Other TOML settings are hand-maintained; inspect the installed help before suggesting them.

## Critical semantics (traps — read before assuming a bug)

1. **No silent fallback, by design.** With a refresh token present, App API
   errors are surfaced as-is and NEVER auto-fall back to the anonymous web API.
   Anonymous fallback happens only when *no* token exists anywhere AND
   `web_fallback_enabled=true`, and only for `search` / `detail` / `ranking` /
   `download`. Authenticated `detail`, pages, and ugoira metadata therefore
   come from App API; a missing App page resource is a real error, not a reason
   to scrape or retry Web.
2. **`recommended` requires a kind.** Choose one of the kinds shown by
   `pixiv recommended --help`; it requires authentication and does not work
   anonymously.
3. **`--limit` is command-specific.** It is available on `search`, `ranking`,
   `recommended`, `user search`, `user artworks`, `user bookmarks`, and `user following`;
   do not attach it to `detail`, `search-options`, auth/config commands, or
   other commands whose help omits it. Where supported, a positive value sets
   the maximum result count and `0` requests all results. `--page` requires a
   positive `--limit`.
4. **Search flags are command-scoped.** Verify `--search-by`, `--period`,
   `--start-date`, `--end-date`, `--sort`, `--rating`, `--type`, `--ai-mode`, `--aspect-ratio`,
   `--resolution`, `--draw-tool`, `--bookmark-min`, and `--bookmark-max` against `pixiv search --help`; do not infer
   undocumented aliases or attach illustration-only filters to other commands.
   `novel search` supports `--search-by`, `--sort`, `--period`, `--rating`,
   `--min-text-length`, `--max-text-length`, and `--original-only`; it does not
   support illustration AI, drawing-tool, resolution, aspect-ratio, or type filters.
5. **Anonymous restricted search fails explicitly.** Web fallback uses only
   reliable illustration-search filters. `r18`, `r18g`, `mature`, `--search-by tag-title-caption`,
   bookmark-count bounds, and `search-options`
   require App authentication; bookmark-count bounds additionally require Pixiv Premium. For a saved account the CLI
   checks the cached self-profile status before search (24h by default); a non-Premium result fails locally rather than
   making a misleading search request. Use explicit `auth refresh` to force that cache. Do not
   present the failure as an empty result or add a Cookie workaround. `novel search` is App-only and requires authentication.
   Bookmark count is a public bookmark total, never a like count.
6. **Extended rankings need authentication.** Valid modes are `day`,
   `day_male`, `day_female`, `week`, `week_original`, `week_rookie`, `month`,
   `day_manga`, `week_manga`, `month_manga`, `week_rookie_manga`, `day_r18`,
   `day_male_r18`, `day_female_r18`, `week_r18`, `week_r18g`. The final nine
   must not be replaced with an anonymous day ranking.
7. **Empty filtered batches are skipped.** With local filters, search continues past leading empty upstream batches to the first non-empty logical batch or true end; `--limit N` fills logical results and `--limit 0` walks all filtered results. Do not invent request caps.
8. **No like-count field.** Do not invent or label bookmark totals as likes.
9. **`update --json` is only valid with `--check`.** The actual install never
   emits JSON.
10. **Proxy is per-command.** The browser's system proxy is NOT inherited.
   Persist with `pixiv config set https_proxy URL`; override per command with
   `--proxy` / `--no-proxy` (mutually exclusive). With an HTTP(S) proxy,
   resource downloads deliberately use HTTP/1.1; App API, OAuth, and Web
   metadata requests retain normal protocol negotiation.
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
    and `/users/{id}/artworks`, which expand every illust, manga, and ugoira for
    that user — no novels and no implicit limit. User URL downloads require App
    OAuth and never use Cookie, WebView, or anonymous fallback. Do not suggest
    short links, old URLs, FANBOX, Pixivision, Sketch, or HTML scraping.

## Routing

| Task | Read |
| --- | --- |
| Explicitly install or repair the missing `pixiv` binary | `references/install.md` |
| Import, export, back up, or restore authentication | `references/auth.md` |
| Find works/artists (search → filter → detail chains) | `references/discover.md` |
| Download workflows (single, batch, ugoira) | `references/download.md` |
| Errors: auth failures, network/proxy, empty results | `references/troubleshooting.md` |

## Image delivery for agents

After `pixiv download` or MCP `download`, share local file paths via the host attachment API. If the host cannot attach files, share the artwork `url` only and do not claim an image was sent.
