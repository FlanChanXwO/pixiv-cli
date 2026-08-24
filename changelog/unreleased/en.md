# Unreleased

> Release-prep workspace. Audit the target tag range, then write the next bilingual notes directly. Every PR or direct commit in the audit must appear in both languages.

## Added

- Added reverse-image search to `pixiv search SOURCE` and the Pixiv MCP `reverse_search` tool. The CLI automatically selects image mode for explicit HTTP(S) URLs and existing regular files; SauceNAO, ascii2d color/BOVW, and `all` providers return a stable JSON envelope, generic artwork/user records, and NDJSON for canonical records, with explicit partial-provider semantics. ([`69caa31`](https://github.com/FlanChanXwO/pixiv-cli/commit/69caa31), [`6599dec`](https://github.com/FlanChanXwO/pixiv-cli/commit/6599dec), [`ef0dcfe`](https://github.com/FlanChanXwO/pixiv-cli/commit/ef0dcfe), [`e67e21f`](https://github.com/FlanChanXwO/pixiv-cli/commit/e67e21f), [`ce03802`](https://github.com/FlanChanXwO/pixiv-cli/commit/ce03802), [`298e0f3`](https://github.com/FlanChanXwO/pixiv-cli/commit/298e0f3))

## Security

- Reverse search loads each source once into a private snapshot, removes it after provider work, and keeps source strings, credentials, temporary paths, cookies, CSRF/redirect values, and upstream bodies out of published output and diagnostics. The MCP contract deliberately permits private files and private/loopback/link-local URLs only for trusted local clients, while documenting third-party upload, retention, and URL-caching implications. ([`69caa31`](https://github.com/FlanChanXwO/pixiv-cli/commit/69caa31), [`3e2cb47`](https://github.com/FlanChanXwO/pixiv-cli/commit/3e2cb47), [`80d5729`](https://github.com/FlanChanXwO/pixiv-cli/commit/80d5729), [`8169787`](https://github.com/FlanChanXwO/pixiv-cli/commit/8169787), [`4632334`](https://github.com/FlanChanXwO/pixiv-cli/commit/4632334), [`4cfc4d4`](https://github.com/FlanChanXwO/pixiv-cli/commit/4cfc4d4))

## Documentation

- Documented reverse-search source classification, provider/configuration behavior, stdin-only SauceNAO credentials, third-party privacy implications, MCP trusted-client boundaries, partial results, generic artwork records, and the opt-in upstream compatibility workflow across the bilingual user, maintainer, and product-skill documentation. ([`d103eb4`](https://github.com/FlanChanXwO/pixiv-cli/commit/d103eb4), [`9cf51d7`](https://github.com/FlanChanXwO/pixiv-cli/commit/9cf51d7))

## Configuration and diagnostics

- Restored unified `[logging].level`/`[logging].format` configuration with `PIXIV_LOG_LEVEL` and
  `PIXIV_LOG_FORMAT` overrides; `debug` diagnostics remain stderr-only and MCP stdout stays JSON-RPC.
- `pixiv config` now manages the logging, download directory, request pacing, proxy, and account-pool keys
  from one schema; first-run `config.toml` is generated from schema metadata and never overwrites an existing file.
- Added `reverse_search_provider` and `reverse_search_pixiv_only` configuration, plus stdin-only/redacted
  `saucenao_api_key` and the `SAUCENAO_API_KEY` environment override; public SDK construction and APIs remain unchanged. ([`d4a1254`](https://github.com/FlanChanXwO/pixiv-cli/commit/d4a1254), [`ce03802`](https://github.com/FlanChanXwO/pixiv-cli/commit/ce03802), [`9cf51d7`](https://github.com/FlanChanXwO/pixiv-cli/commit/9cf51d7))

## Maintenance

- Fixed Pixiv current-user lookup to use the active `/v1/user/detail` route, accepted `max_illust_id` pagination for the latest-artwork feed, and corrected thumbnail filenames/MCP MIME metadata when CDN bytes are JPEG behind a `.png` URL.
- Hardened the FANBOX identity-scoped cursor binding (Home, Supporting, Creators) to the verified FANBOX account id so a cursor minted under one account cannot be replayed against another account's feed; CreatorPosts and TaggedPosts remain public-scoped. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Replaced the embedded-URL FANBOX resource ref with a stable identity-only envelope (kind, owning creator/post, attachment id); `OpenResource`/`SaveResource` re-resolve a fresh allowlisted locator from trusted metadata when no in-session locator is cached, and the session cookie is sent only on the credentialed `downloads.fanbox.cc` host. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Fixed logical pagination `has_more` to stay true when a batch is truncated mid-way by a limit even if the upstream cursor is empty, across the shared traversal engine and the FANBOX MCP runtime. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Bounded page-range expansion in download page specs and made directory-template segments reject absolute, empty, or traversal segments; direct-resource filenames now use a full-ref digest instead of a truncated prefix that could collide across distinct resources. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Mapped non-original download qualities to the corresponding artwork variant resource so `SaveResource` re-resolves the correct locator, and made filename generation fail early on unknown placeholders, unmatched braces, or missing `{date}` values rather than writing empty filenames. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Download now reports partial success: each artwork writes its files atomically, independent per-artwork failures are returned as a failure set rather than aborting the whole batch, and only context cancellation stops immediately. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Account removal now confirms on a TTY by default and reselects the first remaining account after the default is removed. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))
- Added a default-off `PIXIV_REVERSE_SEARCH_E2E=1` maintenance script for authorized real-provider compatibility observation; source and key are supplied through the private environment, never command arguments, and the check is not part of the normal release gate. ([`d103eb4`](https://github.com/FlanChanXwO/pixiv-cli/commit/d103eb4))
