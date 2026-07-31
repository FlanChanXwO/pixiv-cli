# v0.9.0 — 2026-07-31

## Breaking changes

- Unify the v0.9 command and automation surface: use `pixiv timeline latest|following` instead of `feed`; migrate to renamed MCP timeline tools; use `SupportedDrawingTools()` for SDK drawing-tool discovery; remove SDK and project loggers plus MCP account/configuration tools; and use a one-time desktop remote-login handoff instead of devices, pairing, and callback-copy forms. The release also adds APNG ugoira output, interactive download progress, richer list filters, a static drawing-tool catalog, canonical download de-duplication, and safer account-pool behavior. ([#40](https://github.com/FlanChanXwO/pixiv-cli/pull/40))

## Fixed

- Make the released ClawHub skill and Homebrew formula deployment recover cleanly from moderation, pending-card, and unavailable beta-formula states. ([`0915746`](https://github.com/FlanChanXwO/pixiv-cli/commit/09157462ab6e36da209eae6973a5fc60c7c1bb8a), [`2a6e9f2`](https://github.com/FlanChanXwO/pixiv-cli/commit/2a6e9f2c6f6967c8698928f2c37a9f8bc461f183), [`384a3e8`](https://github.com/FlanChanXwO/pixiv-cli/commit/384a3e825c5a966c524e5d974e7480c18610928a), [`3e67b28`](https://github.com/FlanChanXwO/pixiv-cli/commit/3e67b286e72c3052a86d3f14675b53c0b0ce7c48), [`73b30c2`](https://github.com/FlanChanXwO/pixiv-cli/commit/73b30c24dbee98bbcc7d8b82d1b2d679cde4519e), [`b09f161`](https://github.com/FlanChanXwO/pixiv-cli/commit/b09f16140600644b48d3393217d52398ff10c6f0))

## Documentation

- Introduce auditable bilingual release notes, clearer AI-agent installation and account guidance, and localized contribution templates; distinguish the SkillHub and ClawHub installation paths. ([`66d5fd7`](https://github.com/FlanChanXwO/pixiv-cli/commit/66d5fd705f1e5aebf6413b5ced270c40670a8527), [`92261fa`](https://github.com/FlanChanXwO/pixiv-cli/commit/92261faf3f2ca8fd7afde6776ea882d0686c2b48), [`ea02b29`](https://github.com/FlanChanXwO/pixiv-cli/commit/ea02b29fcf3c14f6c6edabb94aa92b3d09c4eefd), [#37](https://github.com/FlanChanXwO/pixiv-cli/pull/37), [#38](https://github.com/FlanChanXwO/pixiv-cli/pull/38))

## Maintenance

- Use stable packaged-binary smoke job names without GitHub expression placeholders. ([`b8daaa1`](https://github.com/FlanChanXwO/pixiv-cli/commit/b8daaa1c4de024d369410fcabf18041b631e4f11))

**Full Changelog**: [v0.8.0...v0.9.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.8.0...v0.9.0)
