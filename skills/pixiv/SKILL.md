---
name: pixiv
description: Operate Pixiv through the pixiv-cli binary — search works, inspect Pixiv artwork or user IDs/URLs, view rankings and recommendations, manage bookmarks/follows, and download works. Load only when the user explicitly mentions Pixiv or pixiv-cli, provides a pixiv.net URL or ID in a clear Pixiv context, or requests a specific Pixiv operation or `pixiv` command. Do not trigger for generic illustration, artist, image-search, or download requests without Pixiv context. Verify current syntax with `pixiv <cmd> --help`.
---

# Pixiv CLI Skill

Teaches an agent to drive the `pixiv` CLI correctly. This skill encodes workflow
orchestration, safety rules, and semantic traps — flag details always defer to
the installed binary's `pixiv <cmd> --help` output.

## Preflight and account checks

1. Run `pixiv version --json`. The binary itself is the only environment probe:
   if it is missing or not executable, report that blocker and do not install
   or guess an installation method. Refer to installation instructions only
   when the user supplied them or they already exist in the current context.
2. Do not enumerate local accounts on every session. Run
   `pixiv auth list --json` only when authentication, account selection, or an
   anonymous-fallback decision actually requires it. Presence in this list does
   not prove a credential is currently valid; use the networked
   `pixiv auth check [UID] --json` only when validation is needed. Treat both
   `{"accounts": null}` and `{"accounts": []}` as an empty account list.

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
| Read | `search` `search-options` `detail` `ranking` `recommended` `user *` `config get/path` `version` `update --check` | Execute when the user's task requires it |
| Account diagnosis | `auth list/check` | List only for authentication/account/fallback decisions; check only when network validation is needed |
| Write | `bookmark add/remove` `follow add/remove` | State the target (illust/user ID) in one line before executing |
| Disk | `download` | Confirm target directory and exact ID list before each invocation; approval never carries over; see `references/download.md` |
| Interactive credential | `auth login` | Run only on explicit request while the user is present for browser OAuth |
| Account/config state | `auth use/remove` `config set/unset` `update` (actual install) | Ask for explicit confirmation each time; approval does not carry over |
| MCP server | `mcp` | Run only when the user explicitly asks to start it; it is a long-lived stdio JSON-RPC server, not a data command—do not auto-probe, auto-wait, or include it in preflight |

## Output & token control (in priority order)

1. **Reduce at the source (preferred):** pass a positive `--limit N` only when
   that command's help exposes it. In the audited binary these commands are
   `search`, `ranking`, `recommended`, `user artworks`, `user bookmarks`, and
   `user following`. Add `--page`, `--type`, `--rating`, or other flags only
   when that specific command's help exposes them.
   `--limit 0` requests all results, so never use it unless the user explicitly
   asks for everything.
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
pixiv auth list --json                    # only when an account decision needs it
pixiv auth check [UID] --json             # validate token, shows user_id/username
pixiv auth use [UID]                      # switch default account (confirm first)
pixiv config path                         # print config.toml location
pixiv config get download_path            # read one effective setting
pixiv config set download_path ./downloads # config write; confirm first
pixiv search "WORD" --limit 10 --json     # illustration search
pixiv search "WORD" --rating sfw --type illust --ai-mode exclude
pixiv search "WORD" --resolution high --aspect-ratio landscape --tool "CLIP STUDIO PAINT"
pixiv search-options "WORD" --json         # authenticated dynamic tool choices
pixiv detail ILLUST_ID --json             # single artwork detail
pixiv ranking --mode day
pixiv recommended illust --limit 10       # kind is REQUIRED; needs auth
pixiv recommended all --limit 10          # request all supported kinds; needs auth
pixiv user detail USER_ID --json          # full public profile (USER_ID required)
pixiv user artworks [USER_ID] --limit 20  # omit USER_ID = current account
pixiv user bookmarks [USER_ID] --tag TAG --limit 20
pixiv user following [USER_ID] --limit 20
pixiv bookmark add ILLUST_ID --tag TAG    # --tag repeatable; write op
pixiv bookmark remove ILLUST_ID           # write op
pixiv follow add USER_ID                  # write op
pixiv follow remove USER_ID               # write op
pixiv download ILLUST_ID... [--download-path DIR]
pixiv update --check --json               # read-only update check
```

Common per-command flags on Pixiv data commands: `--uid UID` (pick local account),
`--json`, `--proxy URL` / `--no-proxy` (this command only, never persisted).

Common config keys include `download_path`, `filename_template`, `https_proxy`,
and `web_fallback_enabled`; inspect a key with `pixiv config get KEY` instead of
assuming its effective value.

## Critical semantics (traps — read before assuming a bug)

1. **No silent fallback, by design.** With a refresh token present, App API
   errors are surfaced as-is and NEVER auto-fall back to the anonymous web API.
   Anonymous fallback happens only when *no* token exists anywhere AND
   `web_fallback_enabled=true`, and only for `search` / `detail` / `ranking` /
   `download`.
2. **`recommended` requires a kind.** Choose one of the kinds shown by
   `pixiv recommended --help`; it requires authentication and does not work
   anonymously.
3. **`--limit` is command-specific.** It is available on `search`, `ranking`,
   `recommended`, `user artworks`, `user bookmarks`, and `user following`;
   do not attach it to `detail`, `search-options`, auth/config commands, or
   other commands whose help omits it. Where supported, a positive value sets
   the maximum result count and `0` requests all results. `--page` requires a
   positive `--limit`.
4. **Search flags are command-scoped.** Verify `--rating`, `--type`,
   `--ai-mode`, `--aspect-ratio`, `--resolution`, `--tool`, and
   `--search-target` against `pixiv search --help`; do not infer undocumented
   aliases or attach search filters to other commands.
5. **Anonymous restricted search fails explicitly.** Web fallback uses only
   reliable search filters. `r18`, `r18g`, `mature`, and `search-options`
   require App authentication; do not present the failure as an empty result
   or add a Cookie workaround. Bookmark-count filtering is unavailable.
6. **`update --json` is only valid with `--check`.** The actual install never
   emits JSON.
7. **Proxy is per-command.** The browser's system proxy is NOT inherited.
   Persist with `pixiv config set https_proxy URL`; override per command with
   `--proxy` / `--no-proxy` (mutually exclusive).
8. **Long downloads may legitimately take time.** Do not impose an arbitrary
   timeout or kill the process merely because it is slow; wait for completion,
   user cancellation, or a real error.
9. **`--tag` has two narrow meanings.** `user bookmarks --tag TAG` filters
   bookmark listings; `bookmark add --tag TAG` adds a repeatable bookmark tag.
   `search` has no `--tag` flag — put the tag text in its required `WORD` and
   choose `--search-target` when exact matching is needed.

## Routing

| Task | Read |
| --- | --- |
| Find works/artists (search → filter → detail chains) | `references/discover.md` |
| Download workflows (single, batch, ugoira) | `references/download.md` |
| Errors: auth failures, network/proxy, empty results | `references/troubleshooting.md` |
