# v0.9.0 — 2026-07-31

## Breaking changes

- Use `pixiv timeline latest|following` instead of `pixiv feed`. MCP timeline tools are renamed, SDK drawing-tool discovery uses `SupportedDrawingTools()`, SDK and project loggers and the MCP account/configuration tools are removed, and remote login uses a one-time desktop confirmation instead of device pairing and copying callback forms. ([#40](https://github.com/FlanChanXwO/pixiv-cli/pull/40))

## Added

- Ugoira APNG output, interactive download progress, richer list filters, a static drawing-tool catalog, and canonical download de-duplication are new in this release. ([#40](https://github.com/FlanChanXwO/pixiv-cli/pull/40))

## Changed

- Release notes are published in English and Simplified Chinese, installation and account guides for AI agents are clearer, and the SkillHub and ClawHub install paths are explained separately. ([`66d5fd7`](https://github.com/FlanChanXwO/pixiv-cli/commit/66d5fd7), [`92261fa`](https://github.com/FlanChanXwO/pixiv-cli/commit/92261fa), [`ea02b29`](https://github.com/FlanChanXwO/pixiv-cli/commit/ea02b29))

## Fixed

- The ClawHub skill and Homebrew formula can be published or recovered when moderation, pending cards, or an unavailable beta formula interrupted the first attempt. ([`0915746`](https://github.com/FlanChanXwO/pixiv-cli/commit/0915746), [`2a6e9f2`](https://github.com/FlanChanXwO/pixiv-cli/commit/2a6e9f2), [`384a3e8`](https://github.com/FlanChanXwO/pixiv-cli/commit/384a3e8), [`3e67b28`](https://github.com/FlanChanXwO/pixiv-cli/commit/3e67b28), [`73b30c2`](https://github.com/FlanChanXwO/pixiv-cli/commit/73b30c2), [`b09f161`](https://github.com/FlanChanXwO/pixiv-cli/commit/b09f161))

**Full Changelog**: [v0.8.0...v0.9.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.8.0...v0.9.0)

<!-- internal:
  [#38](https://github.com/FlanChanXwO/pixiv-cli/pull/38)
  [#37](https://github.com/FlanChanXwO/pixiv-cli/pull/37)
  [#41](https://github.com/FlanChanXwO/pixiv-cli/pull/41)
  [`b8daaa1`](https://github.com/FlanChanXwO/pixiv-cli/commit/b8daaa1)
-->
