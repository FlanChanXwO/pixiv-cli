# Troubleshooting decision tree

The CLI is designed to expose real causes — read the error text first; it is
usually the answer. Never mask an error with retries or silent fallbacks.

## Binary / environment

- `pixiv: command not found` → report that the binary is unavailable as a
  blocker. If the user explicitly asks you to install it, follow
  `references/install.md`; otherwise do not install it or guess a method.
- Config/auth file locations: `pixiv config path` prints the config file path
  and creates a baseline config if missing; `auth.json` lives in the same
  directory. Never read `auth.json` contents.

## Authentication

| Symptom | Meaning | Action |
| --- | --- | --- |
| `invalid_grant` / token refresh failure | Refresh token expired or revoked | Use `pixiv auth login` only when the user explicitly asks and is present for the interactive OAuth flow |
| "requires authentication" on `recommended`/`user`/`bookmark`/`follow` | Anonymous session | Expected — these are App-API-only. Ask whether to log in |
| R18/R18G/mature search requires authentication | Anonymous Web session | Authenticate before retrying; never add a Cookie workaround |
| `search-options` is unsupported | No App credential | Authenticate, then retry with the same word |
| Bookmark-count search is forbidden | Cached self-profile says the saved account is not Pixiv Premium | Explain that the bound requires Premium; only run `pixiv auth refresh [UID]` if the user explicitly wants to refresh account status |
| Wrong account acting | Multiple local accounts | `pixiv auth list --json`, then `pixiv auth use UID` (confirm first); data commands do not accept per-command account overrides. |
| `auth import` waits for hidden input the user cannot enter | Agent PTY has no direct user-input channel | Cancel the waiting command; give it to the user for their private terminal, or use an authorized secret-manager-to-stdin pipeline as described in `auth.md` |
| Cookie string rejected | By design | Only raw App API refresh tokens are accepted; for an explicit import request follow `auth.md` without asking the user to disclose an undisclosed token |

`pixiv auth list --json` only shows configured accounts. `pixiv auth check
--json` performs the network validation and prints user_id / username (never
the token) — use it when credential validity actually needs diagnosis. Do not
list accounts as a routine session probe. Text `auth list` markers describe only local token storage, not online validity. `pixiv auth refresh [UID] [--all]` rotates saved credentials and forces profile/Premium-cache refresh, so treat it as explicit account maintenance. Treat both `{"accounts": null}` and
`{"accounts": []}` as zero accounts. Check the process exit code before
parsing `--json`, because CLI failures can use plain stderr with empty stdout.
For credential import/export, backup, or restore, follow `auth.md`; successful
safe metadata output does not make raw token or bundle stdout safe to display.

## Network / proxy

- Timeouts or connection resets reaching `oauth.secure.pixiv.net` /
  `app-api.pixiv.net`: likely needs a proxy. Try once with
  `--proxy http://127.0.0.1:7890` (tell the user first); persist only on
  explicit request via `pixiv config set https_proxy ...`.
- `--proxy` and `--no-proxy` are mutually exclusive and never persisted.
- Env fallback: lowercase `https_proxy` is preferred over `HTTPS_PROXY`.

## Fallback semantics (not a bug)

- Token present + App API error → error is final. The CLI never auto-falls
  back to the anonymous web API. Report the real cause.
- No token anywhere + `web_fallback_enabled=true` → `search` / `detail` /
  `ranking` / `download` silently use the anonymous web API. Anonymous results
  can differ: restricted search fails with an authentication requirement,
  `search-options` is unavailable, and some fields may be absent.
- Anonymous path failing entirely → check `pixiv config get
  web_fallback_enabled`; if `false`, that is the configured behavior.

## Empty or "missing" results

- Empty search with filters: verify `--rating`, `--type`, `--ai-mode`,
  `--aspect-ratio`, `--resolution`, and exact `--draw-tool` together; a strict
  combination can legitimately return nothing.
- Wrong AI or resolution result: verify the documented `--ai-mode` and
  `--resolution` values with `pixiv search --help`, then inspect the returned
  records rather than assuming undocumented numeric mappings or thresholds.
- Fewer items than expected: default `--limit` is one upstream batch. Pass an
  explicit `--limit N`, or `--limit 0` only if the user wants everything.
- `--page` errors: it requires a positive `--limit`.

## Diagnostics

- Operation summaries are written as daily plain-text files named `YYYY-MM-DD.txt`
  under `~/.pixiv-cli/logs` (on Windows, `%USERPROFILE%\.pixiv-cli\logs`; default retention 7 days). The terminal stays free of log
  traces by default; JSON stdout stays clean.
- Increase file-log verbosity per run with `PIXIV_LOG_LEVEL=info pixiv <cmd>`
  (or `debug`). Only special non-auth upstream failures may print a log-directory
  hint; login/token failures do not.
- `pixiv update --check --json` is read-only and safe; a real `pixiv update`
  installs a new binary — treat as account/config-state tier (confirm).
