# Unreleased

> Release-prep workspace. Audit the target tag range, then write the next bilingual notes directly. Every PR or direct commit in the audit must appear in both languages.

## Configuration and diagnostics

- Restored unified `[logging].level`/`[logging].format` configuration with `PIXIV_LOG_LEVEL` and
  `PIXIV_LOG_FORMAT` overrides; `debug` diagnostics remain stderr-only and MCP stdout stays JSON-RPC.
- `pixiv config` now manages the logging, download directory, request pacing, proxy, and account-pool keys
  from one schema; first-run `config.toml` is generated from schema metadata and never overwrites an existing file.

## Maintenance

- Hardened the FANBOX identity-scoped cursor binding (Home, Supporting, Creators) to the verified FANBOX account id so a cursor minted under one account cannot be replayed against another account's feed; CreatorPosts and TaggedPosts remain public-scoped. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Replaced the embedded-URL FANBOX resource ref with a stable identity-only envelope (kind, owning creator/post, attachment id); `OpenResource`/`SaveResource` re-resolve a fresh allowlisted locator from trusted metadata when no in-session locator is cached, and the session cookie is sent only on the credentialed `downloads.fanbox.cc` host. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Fixed logical pagination `has_more` to stay true when a batch is truncated mid-way by a limit even if the upstream cursor is empty, across the shared traversal engine and the FANBOX MCP runtime. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Bounded page-range expansion in download page specs and made directory-template segments reject absolute, empty, or traversal segments; direct-resource filenames now use a full-ref digest instead of a truncated prefix that could collide across distinct resources. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Mapped non-original download qualities to the corresponding artwork variant resource so `SaveResource` re-resolves the correct locator, and made filename generation fail early on unknown placeholders, unmatched braces, or missing `{date}` values rather than writing empty filenames. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Download now reports partial success: each artwork writes its files atomically, independent per-artwork failures are returned as a failure set rather than aborting the whole batch, and only context cancellation stops immediately. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Account removal now confirms on a TTY by default and reselects the first remaining account after the default is removed. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
