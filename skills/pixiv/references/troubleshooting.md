# Troubleshooting decision tree

The CLI is designed to expose real causes — read the error text first; it is
usually the answer. Never mask an error with retries or silent fallbacks.

## Binary / environment

- `pixiv: command not found` → report that the binary is unavailable as a
  blocker. If the user explicitly asks you to install it, follow
  `references/install.md`; otherwise do not install it or guess a method.
- Config/auth file locations: `pixiv config path` prints the config file path;
  `auth.json` lives in the same directory. Never read `auth.json` contents.

## Authentication

| Symptom | Meaning | Action |
| --- | --- | --- |
| `invalid_grant` / token refresh failure | Refresh token expired or revoked | Use `pixiv auth login` only when the user explicitly asks and is present for the interactive OAuth flow |
| "requires authentication" on `recommended`/`user`/`bookmark`/`follow` | Anonymous session | Expected — these are App-API-only. Ask whether to log in |
| R18/R18G/mature search requires authentication | Anonymous Web session | Authenticate before retrying; never add a Cookie workaround |
| `search-options` is unsupported | No App credential | Authenticate, then retry with the same word |
| Wrong account acting | Multiple local accounts | `pixiv auth list --json`, then `--uid UID` per command, or `pixiv auth use UID` (confirm first) |
| Cookie string rejected | By design | Only raw App API refresh tokens are accepted; if the user already has one, show the placeholder-only `pixiv auth add --token '<refresh-token>'` and have them run it in a private terminal |

`pixiv auth list --json` only shows configured accounts. `pixiv auth check
--json` performs the network validation and prints user_id / username (never
the token) — use it when credential validity actually needs diagnosis. Do not
list accounts as a routine session probe. Treat both `{"accounts": null}` and
`{"accounts": []}` as zero accounts. Check the process exit code before
parsing `--json`, because CLI failures can use plain stderr with empty stdout.

Never run `pixiv auth token`: it prints the stored refresh token. If the user
needs it, instruct them to run `pixiv auth token [UID]` only in their own
private terminal and never ask them to paste its output into the chat.

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
  `--aspect-ratio`, `--resolution`, and exact `--tool` together; a strict
  combination can legitimately return nothing.
- Wrong AI or resolution result: verify the documented `--ai-mode` and
  `--resolution` values with `pixiv search --help`, then inspect the returned
  records rather than assuming undocumented numeric mappings or thresholds.
- Fewer items than expected: default `--limit` is one upstream batch. Pass an
  explicit `--limit N`, or `--limit 0` only if the user wants everything.
- `--page` errors: it requires a positive `--limit`.

## Diagnostics

- Increase verbosity per run: `PIXIV_LOG_LEVEL=info pixiv <cmd>` (or `debug`).
  Logs go to stderr; JSON output on stdout stays clean.
- `pixiv update --check --json` is read-only and safe; a real `pixiv update`
  installs a new binary — treat as account/config-state tier (confirm).
