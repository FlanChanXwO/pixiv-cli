# v1.0.1 — 2026-09-02

## Added

- Add a browser-compatible ascii2d transport and optional challenge recovery through FlareSolverr's JSON control API, with separate native/source and solver-browser proxy surfaces. Native ascii2d uploads remain direct multipart requests, and the solver never receives the image upload. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))
- Reuse valid FlareSolverr state across repeated MCP calls, coalesce concurrent solves, and close provider and solver clients with the command or MCP lifecycle. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## Changed

- Match Chrome browser User-Agent and client hints for ascii2d, reject inconsistent custom identity values, and keep the ascii2d proxy separate from standard source and SauceNAO traffic. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))
- Detect challenge responses explicitly, replay the native request once with the solver-provided session, and parse live ascii2d result pages consistently. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## Fixed

- Map unavailable, failed, or malformed FlareSolverr responses to stable public reverse-search error codes without exposing wrapped causes or upstream bodies. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))
- Make reverse-search E2E and MCP runner cleanup wait for process termination and close clients, avoiding leaked resources during verification and long-running sessions. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## Security

- Keep challenge recovery challenge-only and JSON-control-only: solver traffic does not carry native multipart image data, solver state is not written to disk, and source, credential, cookie, CSRF, temporary-path, and upstream-body values remain outside public output and diagnostics. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## Documentation

- Document the reverse-search transport and proxy separation, Chrome-146 identity pairing, challenge-only solver boundary, provider upload limit, stable solver errors, and CLI/MCP lifecycle contracts. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

## Maintenance

- Add focused coverage for ascii2d transport and challenge classification, solver lifecycle and reuse, live result parsing, all-provider aggregation, and MCP cleanup. ([#75](https://github.com/FlanChanXwO/pixiv-cli/pull/75))

**Full Changelog**: [v1.0.0...v1.0.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.0.1)
