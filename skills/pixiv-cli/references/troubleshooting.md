# Troubleshooting decision tree

The CLI is designed to expose real causes — read the error text first; it is
usually the answer. Never mask an error with retries or silent fallbacks.

## Binary / environment

- `pixiv: command not found` → report that the binary is unavailable as a
  blocker. If the user explicitly asks you to install it, follow
  `references/install.md`; otherwise do not install it or guess a method.
- Config/auth file locations: `pixiv config path` prints the config file path
  and creates a baseline config if missing; the authoritative credential store
  is the sibling `pixiv-cli.db`. The new runtime never reads or migrates a
  legacy `auth.json`; use the old CLI to run `pixiv auth export --all --output
  <private bundle>`, then use shell redirection or a pipe to run
  `pixiv auth import < bundle.json`. Never read credential-store contents directly.

## Authentication

| Symptom | Meaning | Action |
| --- | --- | --- |
| `invalid_grant` / token refresh failure | Refresh token expired or revoked | Use `pixiv auth login` only when the user explicitly asks and is present for the interactive OAuth flow |
| "requires authentication" on `recommended`/`user`/`bookmark`/`follow` | Anonymous session | Expected — these are App-API-only. Ask whether to log in |
| A restricted-content operation requires authentication | No authenticated local account selected | Select an account with `pixiv auth use UID` (or import credentials) before retrying; never add a Cookie workaround |
| `--draw-tool` is rejected | Value is absent from this version's catalog | Choose an exact catalog value; a unique one-edit spelling mistake includes a suggestion |
| Bookmark-count search is incomplete or rejected | The selected strategy controls whether candidate bounds are filtered locally; `server` is evidence-gated | Inspect the returned `filter.strategy` and `filter.completeness`. `auto`/`local` use local candidate filtering, `best_effort` is explicitly partial, and `server` returns an explicit unsupported error. Premium is not a local hard gate; only run `pixiv auth refresh [UID]` for an explicit account-maintenance request |
| Reverse search reports `missing_credential` | SauceNAO is selected without a key | Set `saucenao_api_key` through non-TTY stdin or provide `SAUCENAO_API_KEY`; `config get` is redacted. ascii2d-only providers do not require the SauceNAO key |
| Reverse search reports `partial=true` | At least one provider succeeded and another failed | Inspect the safe `provider_errors` entries and retry only when the user explicitly wants another upstream request; do not hide the partial result or invent a fallback |
| A reverse-search input was treated as a keyword | The value was not an explicit HTTP(S) source and did not resolve to a regular file | Check the exact path and provider/output flags. Explicit `http:`/`https:` values never fall back to keyword search, even when the URL is invalid |
| Wrong account acting | Multiple local accounts | `pixiv auth list --json`, then `pixiv auth use UID` (confirm first); data commands do not accept per-command account overrides. |
| `auth import` waits for hidden input the user cannot enter | Agent PTY has no direct user-input channel | Cancel the waiting command; give it to the user for their private terminal, or use an authorized secret-manager-to-stdin pipeline as described in `auth.md` |
| Cookie string rejected | By design | Only raw App API refresh tokens are accepted; for an explicit import request follow `auth.md` without asking the user to disclose an undisclosed token |

`pixiv auth list --json` only shows configured accounts. `pixiv auth check
--json` performs the network validation and prints user_id / username (never
the token) — use it when credential validity actually needs diagnosis. Do not
list accounts as a routine session probe. Text `auth list` markers describe only local token storage, not online validity. `pixiv auth refresh [UID] [--all]` rotates saved credentials and refreshes account profile metadata, so treat it as explicit account maintenance. Treat both `{"accounts": null}` and
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
- Pixiv service configuration can be scoped with `[pixiv.network].proxy_url`;
  FANBOX uses independent `[fanbox.network].proxy_url` and `user_agent` values.
  `[fanbox.flaresolverr]` is challenge-only and is not a general FANBOX proxy.
- Request pacing comes from `PIXIV_REQUEST_INTERVAL` or
  `[network].request_interval`.
- Reverse search may upload the image to SauceNAO/ascii2d. Verify the source is
  authorized before retrying a provider/network failure; do not paste the key,
  source URL, or upstream response into diagnostics.

## Authentication semantics (not a bug)

- No authenticated local account → content commands return an authentication
  requirement. Select one with `pixiv auth use UID` (confirm first) or import
  credentials; never fall back to an anonymous Web path.
- Token present + App API error → error is final. The CLI never auto-falls
  back to a Web path. Report the real cause.
- Removed `web_fallback_enabled` in `config.toml` → returns
  `removed_setting`; clear it with `pixiv config unset web_fallback_enabled`.

## Empty or "missing" results

- Empty search with filters: verify `--type` (entity route),
  `--content-type` when `--type artwork`, `--ai-mode`, `--aspect-ratio`,
  `--resolution`, and exact `--draw-tool` together; a strict combination can
  legitimately return nothing. `--rating` is a compatibility diagnostic and
  cannot be used to filter v1 App API search results.
- Wrong AI or resolution result: verify the documented `--ai-mode` and
  `--resolution` values with `pixiv search --help`, then inspect the returned
  records rather than assuming undocumented numeric mappings or thresholds.
- Fewer items than expected: default `--limit` is one upstream batch. Pass an
  explicit `--limit N`, or `--limit 0` only if the user wants everything.
- `--page` errors: it requires a positive `--limit`.

## Error output

- CLI failures are written directly to stderr; `--json` and `--ndjson` stdout stay
  protocol-clean.
- Go SDK calls return typed errors to the caller. MCP tool failures return their
  structured result with `isError=true`.
- Set `pixiv config set log_level debug` (or `PIXIV_LOG_LEVEL=debug`) to enable
  safe typed diagnostics on stderr. `log_format=json` (or
  `PIXIV_LOG_FORMAT=json`) emits one JSON event per stderr line. Diagnostics are
  startup-scoped, omit query strings, headers, cookies, tokens, response bodies,
  and proxy userinfo, and never mix with MCP stdout.
- `pixiv update --check --json` is read-only and safe; a real `pixiv update`
  installs a new binary — treat as account/config-state tier (confirm).
