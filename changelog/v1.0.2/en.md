# v1.0.2 — 2026-09-09

## Added

- Add `sdk/pixiv.Client.SaveResourceURL` and wire download adapters to save allowlisted Pixiv CDN resources directly. The path validates HTTPS and official or explicitly allowed hosts, preserves Pixiv referer and redirect checks, disables cookies, and uses atomic writes. ([`1ac3e47`](https://github.com/FlanChanXwO/pixiv-cli/commit/1ac3e4750dde9221b94c43cf9ff19be234623772), [`b4e339c`](https://github.com/FlanChanXwO/pixiv-cli/commit/b4e339c15a276755f8acba2637e4b3aabc3d3ea8))
- Let `pixiv detail` consume canonical artwork, novel, and user records from normal or reverse-search pipelines, infer compatible entity types, and project detail results back to canonical record output while preserving the existing raw ID/URL text path. ([#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76))
- Publish the same verified native `linux/amd64` and `linux/arm64` release images to Docker Hub at `docker.io/flanchanxwo/pixiv-cli` after a successful Release workflow. The post-Release publisher reuses immutable verified artifacts and supports explicit recovery with the original release tag and run ID instead of rebuilding images. ([#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## Changed

- Preserve structured batch results across CLI and MCP downloads: completed files remain visible when later items fail, per-item failures retain typed causes, cancellations propagate, and account-pool retry decisions only replay calls before a file is published. ([`ee7be4a`](https://github.com/FlanChanXwO/pixiv-cli/commit/ee7be4afde62c448a370196f27771d7c3aaf3458), [`038208e`](https://github.com/FlanChanXwO/pixiv-cli/commit/038208e3a2bd7b6137f77d8cfbdd16ca54a259c1), [`9bd9701`](https://github.com/FlanChanXwO/pixiv-cli/commit/9bd9701d0bd5435ebd41061af716d3856f8da057), [`3c4d035`](https://github.com/FlanChanXwO/pixiv-cli/commit/3c4d03507ad1f7170ac2b348ef5b50a1ccd002d2))
- Report non-blocking ugoira filename-template problems as safe warnings with fallback names instead of failing the whole download; CLI writes warnings to stderr while MCP returns them in the structured result. ([`0e4980d`](https://github.com/FlanChanXwO/pixiv-cli/commit/0e4980d980bb17e78fbd7caeeb4f33f615adb2c8))

## Fixed

- Normalize multi-page artwork indexes from `meta_pages` array order across Pixiv artwork responses. The final detail-path regression fix covers real responses that omit `page_index`, so each page receives a distinct resource reference and `--pages` or full-work downloads no longer collapse different output files onto the same image. ([`5d25741`](https://github.com/FlanChanXwO/pixiv-cli/commit/5d2574190f2424630635c0b715d5d3cc2e3c13bd), [#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81), [#78](https://github.com/FlanChanXwO/pixiv-cli/issues/78))
- Normalize static image MIME types to stable `.jpg`, `.png`, `.gif`, and `.webp` extensions, and reject unsupported image types explicitly. ([`33c915d`](https://github.com/FlanChanXwO/pixiv-cli/commit/33c915da8e5c778ae7b9a1bb0c533f7c9ef09871))

## Security

- Keep direct Pixiv CDN saving inside the SDK resource-policy boundary: redirects are revalidated, caller cookies are not forwarded, signed source URLs stay out of public download results and diagnostics, and failed writes do not publish partial destination files. ([`1ac3e47`](https://github.com/FlanChanXwO/pixiv-cli/commit/1ac3e4750dde9221b94c43cf9ff19be234623772), [`b4e339c`](https://github.com/FlanChanXwO/pixiv-cli/commit/b4e339c15a276755f8acba2637e4b3aabc3d3ea8))
- Keep Docker Hub credentials confined to the protected `release` Environment and pass the token only through `docker login --password-stdin`; publication verifies the immutable tag, public Release, source run, image architecture, and OCI provenance labels before pushing. ([#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## Documentation

- Align CLI, MCP, SDK, README, and product skill documentation with direct-resource downloads, quality and closed page selection, structured results and warnings, canonical detail pipelines, and the Docker Hub release/recovery contract. ([`a023e14`](https://github.com/FlanChanXwO/pixiv-cli/commit/a023e1483a94771e6fac312ff4f9db69e626d560), [`4eb436c`](https://github.com/FlanChanXwO/pixiv-cli/commit/4eb436cd02085931e50620efa14457874d2732d1), [#76](https://github.com/FlanChanXwO/pixiv-cli/pull/76), [#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80))

## Maintenance

- Complete the direct-resource flow audit and expand download/report adapter coverage for page ordering, CDN policy, MIME mapping, partial results, cancellation, CLI/MCP projections, and release-container provenance. ([`b5fff06`](https://github.com/FlanChanXwO/pixiv-cli/commit/b5fff0603e2cfe1a2495f6d30c6bf207ab2759ac), [`1abbf46`](https://github.com/FlanChanXwO/pixiv-cli/commit/1abbf467e6c35d14918b54e9d64216b8282712e6), [#80](https://github.com/FlanChanXwO/pixiv-cli/pull/80), [#81](https://github.com/FlanChanXwO/pixiv-cli/pull/81))

**Full Changelog**: [v1.0.1...v1.0.2](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.1...v1.0.2)
