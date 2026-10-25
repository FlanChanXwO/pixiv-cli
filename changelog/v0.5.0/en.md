# v0.5.0 — 2026-07-22

## Breaking changes

- `pixiv search` and MCP search/list tools accept the new filter fields only; the legacy search and account compatibility flags are gone. `pixiv download` flags are scoped to the download command. ([`27c151b`](https://github.com/FlanChanXwO/pixiv-cli/commit/27c151b), [`41430fe`](https://github.com/FlanChanXwO/pixiv-cli/commit/41430fe), [`7249b1d`](https://github.com/FlanChanXwO/pixiv-cli/commit/7249b1d), [`8502851`](https://github.com/FlanChanXwO/pixiv-cli/commit/8502851))
- MCP downloads are delivered as local files with explicit `pages` and `quality` options instead of inline payloads. ([`8502851`](https://github.com/FlanChanXwO/pixiv-cli/commit/8502851))

## Added

- CLI and MCP download commands accept page selection and static image quality options, and artwork records expose a stable artwork URL. ([`bc00211`](https://github.com/FlanChanXwO/pixiv-cli/commit/bc00211), [`ecfc1ee`](https://github.com/FlanChanXwO/pixiv-cli/commit/ecfc1ee))
- All App API ranking modes are available through the CLI. ([`bc00211`](https://github.com/FlanChanXwO/pixiv-cli/commit/bc00211))
- `debug` writes redacted daily operation logs under the user state directory. ([`907fe1b`](https://github.com/FlanChanXwO/pixiv-cli/commit/907fe1b))

## Changed

- CLI, MCP, and product-skill documentation now matches the App API search behavior and the updated download flow. ([`1ffad53`](https://github.com/FlanChanXwO/pixiv-cli/commit/1ffad53))

## Fixed

- Authenticated R18 artwork reads work again; search filtering skips empty upstream batches, login shows its final page, and log writers close when a command finishes. ([`1030531`](https://github.com/FlanChanXwO/pixiv-cli/commit/1030531), [`1e01b74`](https://github.com/FlanChanXwO/pixiv-cli/commit/1e01b74), [`274ccac`](https://github.com/FlanChanXwO/pixiv-cli/commit/274ccac), [`71734d0`](https://github.com/FlanChanXwO/pixiv-cli/commit/71734d0), [`79c14ac`](https://github.com/FlanChanXwO/pixiv-cli/commit/79c14ac), [`8a172da`](https://github.com/FlanChanXwO/pixiv-cli/commit/8a172da))
- AI-generated artwork filtering prefers the current upstream `illust_ai_type` field, so filter results match what Pixiv reports. ([`1729806`](https://github.com/FlanChanXwO/pixiv-cli/commit/1729806), [`8505651`](https://github.com/FlanChanXwO/pixiv-cli/commit/8505651))
- CLI prompts are consistently English and search parameters are explained more clearly. ([`76d8e7b`](https://github.com/FlanChanXwO/pixiv-cli/commit/76d8e7b), [`b6c0ac8`](https://github.com/FlanChanXwO/pixiv-cli/commit/b6c0ac8), [`d13a4a1`](https://github.com/FlanChanXwO/pixiv-cli/commit/d13a4a1))

**Full Changelog**: [v0.4.5...v0.5.0](https://github.com/FlanChanXwO/pixiv-cli/compare/v0.4.5...v0.5.0)

<!-- internal:
  [`02fe9f5`](https://github.com/FlanChanXwO/pixiv-cli/commit/02fe9f5)
  [`058aaad`](https://github.com/FlanChanXwO/pixiv-cli/commit/058aaad)
  [`0acc0d7`](https://github.com/FlanChanXwO/pixiv-cli/commit/0acc0d7)
  [`0e23694`](https://github.com/FlanChanXwO/pixiv-cli/commit/0e23694)
  [`26cf51b`](https://github.com/FlanChanXwO/pixiv-cli/commit/26cf51b)
  [`3053df6`](https://github.com/FlanChanXwO/pixiv-cli/commit/3053df6)
  [`a58f261`](https://github.com/FlanChanXwO/pixiv-cli/commit/a58f261)
  [`afbb293`](https://github.com/FlanChanXwO/pixiv-cli/commit/afbb293)
  [`cd8d278`](https://github.com/FlanChanXwO/pixiv-cli/commit/cd8d278)
  [`f026316`](https://github.com/FlanChanXwO/pixiv-cli/commit/f026316)
  [`fba19f0`](https://github.com/FlanChanXwO/pixiv-cli/commit/fba19f0)
-->
