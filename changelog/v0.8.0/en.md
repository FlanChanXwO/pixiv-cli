# v0.8.0 — 2026-07-28

## Breaking changes

- Reworked the public SDK download and construction APIs, type-safe configuration API, and CLI/MCP download inputs around explicit options and concurrency. ([`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3))

## Added

- Added persistent download-cache revalidation and resume, ordered batch results, artwork/user/CDN references, APNG support, canonical records, pools, feeds, and cross-machine OAuth callback relay configuration. ([`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3))

## Fixed

- Restored the browser success and failure result pages for cross-machine authentication. ([`3145bf3`](https://github.com/FlanChanXwO/pixiv-cli/commit/3145bf3))

## Documentation

- Clarified the Pixiv ecosystem positioning and product-skill guidance. ([#35](https://github.com/FlanChanXwO/pixiv-cli/pull/35))

## Maintenance

- Made documentation-only pull requests and main pushes use the focused documentation contract gate. ([#36](https://github.com/FlanChanXwO/pixiv-cli/pull/36))

**Full Changelog**: [v0.7.2...v0.8.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.7.2...v0.8.0)
