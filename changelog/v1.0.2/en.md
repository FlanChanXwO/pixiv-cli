# v1.0.2 — 2026-09-10

## Added

- The SDK's `SaveResourceURL` and the CLI/MCP download commands can save Pixiv CDN resources directly, keeping the selected quality and page. `pixiv detail` accepts artwork, novel, and user records from search or reverse search and infers the entity type. ([#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76), [#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))
- Verified container images are published to Docker Hub at `docker.io/flanchanxwo/pixiv-cli` after a Release. ([#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## Changed

- Batch downloads keep completed files when a later item fails, report per-item errors, and stop immediately on cancellation. A ugoira filename template that cannot be applied now produces a warning and a fallback name instead of failing the whole download. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))

## Fixed

- Multi-page artwork page indexes follow the `meta_pages` order even when the upstream response omits `page_index`, so `--pages` and full-work downloads no longer map different pages to the same image. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79), [#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81))
- Static images get stable `.jpg`, `.png`, `.gif`, or `.webp` extensions, and unsupported image types are rejected explicitly. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))

## Security

- Direct CDN saving revalidates every redirect, does not forward your cookies, and keeps signed source URLs out of results and diagnostics. Failed writes do not leave partial files. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))

**Full Changelog**: [v1.0.1...v1.0.2](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.1...v1.0.2)
