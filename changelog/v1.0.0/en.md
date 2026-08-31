# v1.0.0 — 2026-08-31

## Breaking changes

- Complete the v1 SDK and CLI rewrite, consolidating the public Pixiv/FANBOX SDK, CLI, MCP, authentication, storage, and resource contracts around the v1 architecture. ([#59](https://github.com/FlanChanXwO/pixiv-cli/pull/59))

## Added

- Add reverse-image search to the CLI and Pixiv MCP, with local-file/URL source classification, SauceNAO and ascii2d providers, stable result envelopes, and explicit partial-provider semantics. ([#62](https://github.com/FlanChanXwO/pixiv-cli/pull/62))
- Add first-class Docker container release support with pinned runtime, provenance, non-root state handling, and exact-version publication checks. ([#63](https://github.com/FlanChanXwO/pixiv-cli/pull/63))

## Changed

- Align Pixiv timeline and MyPixiv operations with their verified response contracts and public SDK types. ([#60](https://github.com/FlanChanXwO/pixiv-cli/pull/60))

## Fixed

- Restore release validation for the audited v0.10.0 recovery path so valid release preparation is not rejected. ([#49](https://github.com/FlanChanXwO/pixiv-cli/pull/49))
- Handle pending ClawHub security scans without misreporting a received publication as a failed release. ([#50](https://github.com/FlanChanXwO/pixiv-cli/pull/50))
- Fix Pixiv authentication service initialization errors, verified current-user and endpoint contracts, FANBOX proxy conflict validation, and forward-compatible local account migrations. ([#67](https://github.com/FlanChanXwO/pixiv-cli/pull/67))
- Make Windows release gates portable across file URIs, ACL-backed permissions, platform paths, and executable naming. ([#68](https://github.com/FlanChanXwO/pixiv-cli/pull/68))

## Documentation

- Simplify the pull request template so review records contain the change, verification, and self-check evidence required by the repository workflow. ([#61](https://github.com/FlanChanXwO/pixiv-cli/pull/61))

## Maintenance

- Remove generated goal and output artifacts from the repository. ([#64](https://github.com/FlanChanXwO/pixiv-cli/pull/64))
- Remove unused repository artifacts and keep the committed source tree focused. ([#65](https://github.com/FlanChanXwO/pixiv-cli/pull/65))
- Simplify oversized Go test files while retaining the installer, FANBOX, and other behavior coverage. ([#66](https://github.com/FlanChanXwO/pixiv-cli/pull/66))

**Full Changelog**: [v0.10.0...v1.0.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.10.0...v1.0.0)
