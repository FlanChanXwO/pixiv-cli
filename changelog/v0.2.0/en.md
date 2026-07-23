# v0.2.0 — 2026-07-13

## Added

- Added the public Go SDK: concrete `*pixiv.Client`, stable models, typed errors, opaque cursors, account/config handling, and policy-constrained resource streams.
- Added `pixiv user artworks/bookmarks/following [USER_ID]`, bookmark add/remove, follow add/remove, matching MCP tools, paged user lists, and structured output.
- Added injectable `slog` diagnostics with `log_level` / `log_format` and `PIXIV_LOG_LEVEL` / `PIXIV_LOG_FORMAT` overrides.
- Added Linux quality gates and six-platform packaged-binary smoke tests that stay offline and use no Pixiv credentials or live upstream network.

## Changed

- List commands use `--limit` and logical `--page`; `--offset` was deprecated and SDK cursors are not exposed by CLI/MCP.
- A configured refresh token never falls back from failed App API calls to Web. Web is reserved for anonymous allowlisted reads and explicit metadata enrichment.
