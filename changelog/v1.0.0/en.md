# v1.0.0 — 2026-08-31

## Breaking changes

- The v1 rewrite changes the public Pixiv/FANBOX SDK, CLI, MCP, authentication, storage, and resource contracts. SDK users should re-check their imports and type usage against the v1 API. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))

## Added

- `pixiv search` and the Pixiv MCP `reverse_search` tool add reverse-image search with local files and URLs, SauceNAO and ascii2d providers, a stable result envelope, and explicit partial-provider results. ([#62](https://github.com/FlanChanXwO/pixiv-cli/pull/62))
- Official container images are published for `linux/amd64` and `linux/arm64` with a pinned runtime, non-root state handling, and version checks. ([#63](https://github.com/FlanChanXwO/pixiv-cli/pull/63))

## Changed

- Pixiv timeline and MyPixiv reads follow the verified response contracts and public SDK types. ([#60](https://github.com/FlanChanXwO/pixiv-cli/pull/60))

## Fixed

- `pixiv auth` initialization and current-user reads work correctly, FANBOX rejects conflicting proxy settings, and existing account databases migrate in place. ([#67](https://github.com/FlanChanXwO/pixiv-cli/pull/67))
- The Windows build and installer work with file URIs, ACL permissions, platform paths, and the executable name. ([#68](https://github.com/FlanChanXwO/pixiv-cli/pull/68))
- Ugoira encoding in release builds uses libraries matched to the tagged source, so a rebuild produces the same bytes. ([#72](https://github.com/FlanChanXwO/pixiv-cli/pull/72))
- Container images report the same version as the release tag. ([#74](https://github.com/FlanChanXwO/pixiv-cli/pull/74))

## Security

- A pending security scan on ClawHub is reported as pending instead of a failed release, and a failed v0.10.0 release can be recovered. ([#49](https://github.com/FlanChanXwO/pixiv-cli/pull/49), [#50](https://github.com/FlanChanXwO/pixiv-cli/pull/50))

**Full Changelog**: [v0.10.0...v1.0.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.10.0...v1.0.0)

<!-- internal:
  [#61](https://github.com/FlanChanXwO/pixiv-cli/pull/61)
  [#64](https://github.com/FlanChanXwO/pixiv-cli/pull/64)
  [#65](https://github.com/FlanChanXwO/pixiv-cli/pull/65)
  [#66](https://github.com/FlanChanXwO/pixiv-cli/pull/66)
  [#69](https://github.com/FlanChanXwO/pixiv-cli/pull/69)
  [#70](https://github.com/FlanChanXwO/pixiv-cli/pull/70)
  [#71](https://github.com/FlanChanXwO/pixiv-cli/pull/71)
  [#73](https://github.com/FlanChanXwO/pixiv-cli/pull/73)
-->
