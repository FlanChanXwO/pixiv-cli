# v1.0.2 — 2026-09-09

## Added

- Add direct Pixiv CDN resource saving through `sdk/pixiv.Client.SaveResourceURL`, with HTTPS/host/redirect validation, Pixiv referer handling, cookie isolation, and atomic destination writes. This flow is used by the CLI/MCP download adapters and preserves the selected quality and page semantics. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))
- Let `pixiv detail` consume canonical artwork, novel, and user records from normal or reverse-search pipelines, infer compatible entity types, and project detail results back to canonical record output while preserving the existing raw ID/URL text path. ([#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76))
- Publish the same verified native `linux/amd64` and `linux/arm64` release images to Docker Hub at `docker.io/flanchanxwo/pixiv-cli` after a successful Release workflow. The post-Release publisher reuses immutable verified artifacts and supports explicit recovery with the original release tag and run ID instead of rebuilding images. ([#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## Changed

- Preserve structured batch results across CLI and MCP downloads: completed files remain visible when later items fail, per-item failures retain typed causes, cancellations propagate, and account-pool retry decisions only replay calls before a file is published. Non-blocking ugoira filename-template problems now return safe warnings with fallback names instead of failing the whole download. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))

## Fixed

- Normalize multi-page artwork indexes from `meta_pages` array order across Pixiv artwork responses. The final detail-path regression fix covers real responses that omit `page_index`, so each page receives a distinct resource reference and `--pages` or full-work downloads no longer collapse different output files onto the same image. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79), [#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81))
- Normalize static image MIME types to stable `.jpg`, `.png`, `.gif`, and `.webp` extensions, and reject unsupported image types explicitly. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))

## Security

- Keep direct CDN saving inside the SDK resource-policy boundary: redirects are revalidated, caller cookies are not forwarded, signed source URLs stay out of public download results and diagnostics, and failed writes do not publish partial destination files. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79))
- Keep Docker Hub credentials confined to the protected `release` Environment and pass the token only through `docker login --password-stdin`; publication verifies the immutable tag, public Release, source run, image architecture, and OCI provenance labels before pushing. ([#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## Documentation

- Align CLI, MCP, SDK, README, maintainer, and product-skill documentation with direct-resource downloads, quality and closed page selection, structured results and warnings, canonical detail pipelines, and the Docker Hub release/recovery contract. ([#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76), [#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79), [#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## Maintenance

- Expand regression and release-contract coverage for page ordering, CDN policy, MIME mapping, partial results, cancellation, CLI/MCP projections, multi-page resource identity, and verified container provenance. ([#79](https://github.com/FlanChanXwO/pixiv-cli/pull/79), [#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80), [#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81))

**Full Changelog**: [v1.0.1...v1.0.2](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.1...v1.0.2)
