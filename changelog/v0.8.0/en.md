# v0.8.0 — 2026-07-28

## Breaking changes

- The public SDK download and constructor APIs, configuration API, and CLI/MCP download inputs were reworked around explicit options and concurrency. SDK users need to update their call sites. ([`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3))

## Added

- Downloads support persistent cache revalidation and resume, ordered batch results, APNG output, and artwork, user, and CDN references. CLI/MCP add pools, feeds, and cross-machine OAuth callback relay configuration. ([`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3))

## Changed

- The README explains how pixiv-cli fits into the wider Pixiv tooling ecosystem, and the product skill points to the right workflows. ([#35](https://github.com/FlanChanXwO/pixiv-cli/pull/35))

## Fixed

- Cross-machine authentication shows the browser success and failure pages again. ([`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3))

**Full Changelog**: [v0.7.2...v0.8.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.7.2...v0.8.0)

<!-- internal:
  [#36](https://github.com/FlanChanXwO/pixiv-cli/pull/36)
-->
