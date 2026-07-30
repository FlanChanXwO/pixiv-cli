# v0.4.0 — 2026-07-19

## Breaking changes

- Replaced the legacy `auth add` and `auth token` entry points with secure import/export flows, and made authenticated search use the App API contract. ([#14](https://github.com/FlanChanXwO/pixiv-cli/pull/14))

## Added

- Added standalone macOS/Linux and Windows installers, versioned offline auth bundles, complete search filters across the SDK/CLI/MCP surfaces, multilingual public documentation, and the `pixiv-cli` product skill. ([#14](https://github.com/FlanChanXwO/pixiv-cli/pull/14))

## Fixed

- Pinned native Rust provenance and narrowed the v0.3 recovery overlay so an immutable release tag is rebuilt from the reviewed source set. ([#8](https://github.com/FlanChanXwO/pixiv-cli/pull/8), [#9](https://github.com/FlanChanXwO/pixiv-cli/pull/9))

## Documentation

- Consolidated SDK documentation and completed the final v0.3 audit trail used by this release preparation. ([#10](https://github.com/FlanChanXwO/pixiv-cli/pull/10), [#11](https://github.com/FlanChanXwO/pixiv-cli/pull/11), [#13](https://github.com/FlanChanXwO/pixiv-cli/pull/13))

## Maintenance

- Established the Linux `GLIBC_2.35` release baseline and closed the release review findings. ([#12](https://github.com/FlanChanXwO/pixiv-cli/pull/12), [#14](https://github.com/FlanChanXwO/pixiv-cli/pull/14))

**Full Changelog**: [v0.3.0...v0.4.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.3.0...v0.4.0)
