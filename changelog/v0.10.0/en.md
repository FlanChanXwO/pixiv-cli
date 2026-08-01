# v0.10.0 — 2026-08-01

## Breaking changes

- Move artwork filtering to <code>--filter EXPR</code> on visual list and download commands, replacing <code>pixiv filter</code>; replace the CLI <code>--ugoira-format</code> flag with <code>--ugoira-mode</code>. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

## Added

- Add a typed local illustration filter shared by the SDK, CLI, and MCP, including tag/tool collection predicates and preflight validation without extra Pixiv requests. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))
- Add resilient downloads with SQLite artwork archives, atomic metadata sidecars, extended file and directory templates, configurable retries and request pacing, open page ranges, and GIF/APNG/ZIP/frame ugoira outputs. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))
- Accept public-bookmark and illustration-series URLs as download sources with canonical artwork deduplication, and support HTTP(S), SOCKS5, and SOCKS5H proxy URIs consistently across runtime and update requests. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

## Changed

- Make visual list commands emit canonical Record NDJSON when stdout is non-interactive, so their output can be piped directly into <code>pixiv download</code> without an external JSON processor. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

**Full Changelog**: [v0.9.1...v0.10.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.9.1...v0.10.0)
