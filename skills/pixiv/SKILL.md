---
name: pixiv
description: Operate Pixiv through the pixiv-cli binary — search illustrations, view details, rankings, personalized recommendations, user profiles/artworks, manage bookmarks and follows, and download artworks (including ugoira). Load when the user mentions Pixiv, illustrations, artists, illust IDs, or before running any `pixiv` command. Command syntax may change between versions — verify with `pixiv <cmd> --help` instead of relying on pre-trained knowledge.
---

# Pixiv CLI Skill

Teaches an agent to drive the `pixiv` CLI correctly. This skill encodes workflow
orchestration, safety rules, and semantic traps — flag details always defer to
`pixiv <cmd> --help` and the project README.

## Preflight (run once per session)

1. Run `pixiv version --json`. The binary itself is the only environment probe:
   if it fails, report the real error (not found / not executable) and point the
   user to the "Installation" section of the project README. Do not guess an
   installation method.
2. Run `pixiv auth list --json` to see configured local accounts. Presence in
   this list does not prove a credential is currently valid; use the networked
   `pixiv auth check [UID] --json` when validation is needed. No account is not
   an error: on an empty profile, the successful JSON shape is
   `{"accounts": null}` rather than an empty array. Normalize `null` to an
   empty account list. The documented anonymous web fallback remains available
   when `web_fallback_enabled=true`.

## Hard rules

1. NEVER run `pixiv auth token`. It prints a stored refresh token to stdout.
   If the user needs it, tell them to run `pixiv auth token [UID]` in their own
   private terminal and never ask them to paste the output into the chat.
2. NEVER directly read, print, echo, or persist a raw refresh/access token
   outside the CLI-managed account store. `auth list` / `auth check` outputs
   are safe because they do not expose the stored token.
3. Run interactive `pixiv auth login` only when the user explicitly asks and
   is present to complete browser OAuth.
4. NEVER run `pixiv auth add` or place a secret in `--token` /
   `--refresh-token`. The binary exposes `auth add --token string`, not an
   interactive token prompt, so credential import is user-only: show
   `pixiv auth add --token '<refresh-token>'` with a placeholder and ask the
   user to run it in a private terminal. Warn that command arguments may be
   retained by shell history or visible to local process-inspection tools.
5. NEVER accept or forward cookies (`PHPSESSID`, `refresh_token=...` cookie
   strings) as credentials — the CLI rejects them by design; only raw Pixiv App
   API refresh tokens work.

## Operation tiers

| Tier | Commands | Behavior |
| --- | --- | --- |
| Secret (user-only) | `auth token` `auth add`; any literal `--token` / `--refresh-token` | Never execute; give a placeholder-only command for the user's private terminal |
| Read | `search` `search-options` `detail` `ranking` `recommended` `user *` `config get/path` `auth list/check` `version` `update --check` | Execute freely |
| Write | `bookmark add/remove` `follow add/remove` | State the target (illust/user ID) in one line before executing |
| Disk | `download` | Confirm target directory (`pixiv config get download_path`) and item count first; see `references/download.md` |
| Interactive credential | `auth login` | Run only on explicit request while the user is present for browser OAuth |
| Account/config state | `auth use/remove` `config set/unset` `update` (actual install) | Ask for explicit confirmation each time; approval does not carry over |

## Output & token control (in priority order)

1. **Reduce at the source (preferred):** pass a positive `--limit N` only when
   that command's help exposes it. In the audited binary these commands are
   `search`, `ranking`, `recommended`, `user artworks`, `user bookmarks`, and
   `user following`. Add `--page`, `--type`, `--rating`, or other flags only
   when that specific command's help exposes them.
   Note: the default (no `--limit`) returns one upstream batch; `--limit 0`
   keeps fetching until exhausted — never use `--limit 0` unless the user
   explicitly asks for everything.
2. **Small result, display only:** use the default human-readable output and
   relay it. JSON carries field names and metadata — it is *larger* than the
   table for display purposes.
3. **Programmatic processing** (extract IDs, filter, chain into a next
   command): use `--json`. If the result is large, redirect to a temp file and
   use built-in Grep/Read tools to search and read in segments.
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
pixiv auth list --json                    # local accounts (never shows tokens)
pixiv auth check [UID] --json             # validate token, shows user_id/username
pixiv auth use [UID]                      # switch default account (confirm first)
pixiv config path | get KEY | set KEY V   # config.toml management
pixiv search "WORD" --limit 10 --json     # illustration search
pixiv search "WORD" --rating sfw --type illust --ai-mode exclude
pixiv search "WORD" --resolution high --aspect-ratio landscape --tool "CLIP STUDIO PAINT"
pixiv search-options "WORD" --json         # authenticated dynamic tool choices
pixiv detail ILLUST_ID --json             # single artwork detail
pixiv ranking --mode day [--date YYYY-MM-DD]
pixiv recommended all|illust|manga|novel|user --limit 10     # kind is REQUIRED, needs auth
pixiv user detail USER_ID --json          # full public profile (USER_ID required)
pixiv user artworks [USER_ID] --limit 20  # omit USER_ID = current account
pixiv user bookmarks [USER_ID] --tag TAG --limit 20
pixiv user following [USER_ID] --limit 20
pixiv bookmark add ILLUST_ID --tag TAG    # --tag repeatable; write op
pixiv bookmark remove ILLUST_ID           # write op
pixiv follow add|remove USER_ID           # write op
pixiv download ILLUST_ID... [--download-path DIR]
pixiv update --check --json               # read-only update check
```

Common per-command flags on Pixiv data commands: `--uid UID` (pick local account),
`--json`, `--proxy URL` / `--no-proxy` (this command only, never persisted).

Config keys: `download_path`, `filename_template`, `https_proxy`,
`web_fallback_enabled`, `log_level`, `log_format`, `update_check_enabled`,
`output_json`, `login_open_browser`, `login_timeout`, `login_use_after_login`.

Auth priority: `--refresh-token` > `--uid` > `PIXIV_REFRESH_TOKEN` env >
default account in `auth.json`. Settings priority: flag > env > `config.toml` >
built-in default.

## Critical semantics (traps — read before assuming a bug)

1. **No silent fallback, by design.** With a refresh token present, App API
   errors are surfaced as-is and NEVER auto-fall back to the anonymous web API.
   Anonymous fallback happens only when *no* token exists anywhere AND
   `web_fallback_enabled=true`, and only for `search` / `detail` / `ranking` /
   `download`.
2. **`recommended` requires a kind.** `all` returns four independently
   paginated streams (illust, manga, novel, user) in order; it requires
   authentication and never works anonymously.
3. **`--limit` is command-specific.** It is available on `search`, `ranking`,
   `recommended`, `user artworks`, `user bookmarks`, and `user following`;
   do not attach it to `detail`, `search-options`, auth/config commands, or
   other commands whose help omits it. Where supported, unset = one upstream
   batch (compat default), positive = keep fetching until N matches, and `0` =
   fetch everything. `--page` requires a positive `--limit`. `--offset` is
   deprecated.
4. **Search filters are a shared SDK contract.** With a token, App API filters
   content type, resolution, aspect ratio, tool, and AI exclusion; the public
   SDK filters rating and AI-only (`AIType==2`) per batch. Every filter is
   bound to the opaque cursor. With `--limit`, the CLI keeps reading upstream
   batches until enough matches accumulate — a strict query may take multiple
   requests. `--ai-type` is deprecated (`0=exclude`, `1=only`, `2=all`) and
   conflicts with explicit `--ai-mode`; `--r18` is a deprecated alias for
   `--rating r18` and never changes the keyword.
5. **Anonymous restricted search fails explicitly.** Web fallback uses only
   reliable search filters. `r18`, `r18g`, `mature`, and `search-options`
   require App authentication; do not present the failure as an empty result
   or add a Cookie workaround. Bookmark-count filtering is unavailable.
6. **`update --json` is only valid with `--check`.** The actual install never
   emits JSON.
7. **Proxy is per-command.** The browser's system proxy is NOT inherited.
   Persist with `pixiv config set https_proxy URL`; override per command with
   `--proxy` / `--no-proxy` (mutually exclusive).
8. **Anonymous `search_user` is approximate** — it dedupes authors from a web
   work search, not the official user search.
9. **Ugoira downloads take time**: the CLI downloads a zip and encodes GIF/APNG
   with a built-in Rust encoder (no ffmpeg). Do not kill a slow ugoira
   download; let it finish or fail with a real error.
10. **`--tag` has two narrow meanings.** `user bookmarks --tag TAG` filters
    bookmark listings; `bookmark add --tag TAG` adds a repeatable bookmark tag.
    `search` has no `--tag` flag — put the tag text in its required `WORD` and
    choose `--search-target` when exact matching is needed.

## Routing

| Task | Read |
| --- | --- |
| Find works/artists (search → filter → detail chains) | `references/discover.md` |
| Download workflows (single, batch, ugoira) | `references/download.md` |
| Errors: auth failures, network/proxy, empty results | `references/troubleshooting.md` |
