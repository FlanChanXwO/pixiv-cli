# Troubleshooting decision tree

Expose the real cause. Do not hide an error with retries, empty output, or a
silent backend switch.

## Authentication and fallback

| Symptom | Meaning | Action |
| --- | --- | --- |
| token refresh or `invalid_grant` failure | Refresh token expired/revoked | Ask the user to complete `pixiv auth login`. |
| R18/R18G/mature search requires authentication | Anonymous Web session | Log in before retrying; never add a Cookie workaround. |
| `search-options` requires authentication | Options are App-only | Authenticate, then rerun for the same word. |
| Cookie rejected | Expected security boundary | Use only a raw App API refresh token through the supported OAuth/import flow. |

With a refresh token, App search errors are final and never fall back to Web.
Without a token and with `web_fallback_enabled=true`, anonymous search only
uses filters the Web adapter can reliably express. An unsupported restricted
filter must return an error, not an empty list.

## Empty or short search results

- Confirm the filter combination. Rating and `ai-mode=only` are applied over
  App results; `AIType==2` is the AI-generated value.
- Confirm resolution semantics use both dimensions: high `>=3000`, medium
  `1000..2999`, low `<=999`.
- Confirm `--tool` exactly matches a value returned by `search-options WORD`.
- A positive `--limit` continues upstream pagination to collect matching
  results. No limit retains the one-batch compatibility behavior.

## Diagnostics

- `pixiv auth check --json` validates the selected account without exposing a
  token.
- `PIXIV_LOG_LEVEL=info pixiv <command>` increases diagnostics on stderr while
  JSON remains on stdout.
- For proxy/network failure, report the current cause before proposing a
  one-command `--proxy` override; never persist config without approval.
