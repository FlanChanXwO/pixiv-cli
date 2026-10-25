# v1.0.1 — 2026-09-02

## Changed

- ascii2d reverse search uses a browser-compatible transport with the matching Chrome User-Agent and client hints, and its proxy setting is separate from SauceNAO and standard source traffic. An optional FlareSolverr JSON control API can solve challenges; the solver never receives the uploaded image. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## Fixed

- FlareSolverr unavailability, failures, and malformed responses map to stable error codes instead of leaking upstream bodies, and solver state is reused across MCP calls and released with the CLI or MCP lifecycle. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## Security

- Challenge solving is limited to challenge responses and JSON control: no image data is sent to the solver, solver state is not written to disk, and source, credentials, cookies, CSRF values, temporary paths, and upstream bodies stay out of output and diagnostics. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

**Full Changelog**: [v1.0.0...v1.0.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.0.1)
