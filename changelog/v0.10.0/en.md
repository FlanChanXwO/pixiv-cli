# v0.10.0 — 2026-08-01

## Breaking changes

- Artwork filtering moved from `pixiv filter` to `--filter EXPR` on visual list and download commands, and the CLI `--ugoira-format` flag is now `--ugoira-mode`. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

## Added

- A typed local illustration filter is shared by the SDK, CLI, and MCP, including tag and drawing-tool predicates validated before any Pixiv request. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))
- Downloads can archive to SQLite, write atomic metadata sidecars, use extended filename and directory templates, configure retries and request pacing, download open page ranges, and output ugoira as GIF, APNG, ZIP, or frames. Public bookmark and illustration-series URLs are accepted as sources, and HTTP(S)/SOCKS5/SOCKS5H proxies work for both downloads and updates. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

## Changed

- When output is piped, visual list commands emit canonical Record NDJSON that can be passed straight to `pixiv download`. ([#46](https://github.com/FlanChanXwO/pixiv-cli/pull/46))

**Full Changelog**: [v0.9.1...v0.10.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.9.1...v0.10.0)

<!-- internal:
  [#47](https://github.com/FlanChanXwO/pixiv-cli/pull/47)
  [#48](https://github.com/FlanChanXwO/pixiv-cli/pull/48)
-->
