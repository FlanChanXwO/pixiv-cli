# v1.0.1 — 2026-09-02

## Added

- Add a browser-compatible ascii2d transport and optional challenge recovery through FlareSolverr's JSON control API, with separate native/source and solver-browser proxy surfaces. Native ascii2d uploads remain direct multipart requests, and the solver never receives the image upload. ([`6071506`](https://github.com/FlanChanXwO/pixiv-cli/commit/60715066c28a2e5378f730e9af73205303b1728a), [`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f))
- Reuse valid FlareSolverr state across repeated MCP calls, coalesce concurrent solves, and close provider and solver clients with the command or MCP lifecycle. ([`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`e9ca76c`](https://github.com/FlanChanXwO/pixiv-cli/commit/e9ca76ce3bb2c9d9f1395183be01c4bddd14a463), [`05df0d5`](https://github.com/FlanChanXwO/pixiv-cli/commit/05df0d5a05c9fe6978eaddb7efb9e7d2bd8a0e00))

## Changed

- Match Chrome browser User-Agent and client hints for ascii2d, reject inconsistent custom identity values, and keep the ascii2d proxy separate from standard source and SauceNAO traffic. ([`6071506`](https://github.com/FlanChanXwO/pixiv-cli/commit/60715066c28a2e5378f730e9af73205303b1728a), [`c01402a`](https://github.com/FlanChanXwO/pixiv-cli/commit/c01402afc69f0d510a78e911bc38f9ab0039532b), [`dde3aca`](https://github.com/FlanChanXwO/pixiv-cli/commit/dde3aca98f5850802d87d97cefb1705a1b0bc50c))
- Detect challenge responses explicitly, replay the native request once with the solver-provided session, and parse live ascii2d result pages consistently. ([`7951909`](https://github.com/FlanChanXwO/pixiv-cli/commit/7951909d7491c82c59c283262b468650cd1c5784), [`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`471af9b`](https://github.com/FlanChanXwO/pixiv-cli/commit/471af9b9752c29bbe2f03f22c3cc6f4ae62c65f9), [`8337c6f`](https://github.com/FlanChanXwO/pixiv-cli/commit/8337c6f942db9522417df6d5d10e1aca225c1bf3))

## Fixed

- Map unavailable, failed, or malformed FlareSolverr responses to stable public reverse-search error codes without exposing wrapped causes or upstream bodies. ([`471af9b`](https://github.com/FlanChanXwO/pixiv-cli/commit/471af9b9752c29bbe2f03f22c3cc6f4ae62c65f9))
- Make reverse-search E2E and MCP runner cleanup wait for process termination and close clients, avoiding leaked resources during verification and long-running sessions. ([`05df0d5`](https://github.com/FlanChanXwO/pixiv-cli/commit/05df0d5a05c9fe6978eaddb7efb9e7d2bd8a0e00), [`a70c490`](https://github.com/FlanChanXwO/pixiv-cli/commit/a70c490e7000d4e4d123f0237a68539dadac550f))

## Security

- Keep challenge recovery challenge-only and JSON-control-only: solver traffic does not carry native multipart image data, solver state is not written to disk, and source, credential, cookie, CSRF, temporary-path, and upstream-body values remain outside public output and diagnostics. ([`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`e9ca76c`](https://github.com/FlanChanXwO/pixiv-cli/commit/e9ca76ce3bb2c9d9f1395183be01c4bddd14a463))

## Documentation

- Document the reverse-search transport and proxy separation, Chrome-146 identity pairing, challenge-only solver boundary, provider upload limit, stable solver errors, and CLI/MCP lifecycle contracts. ([`dde3aca`](https://github.com/FlanChanXwO/pixiv-cli/commit/dde3aca98f5850802d87d97cefb1705a1b0bc50c))

## Maintenance

- Add focused coverage for ascii2d transport and challenge classification, solver lifecycle and reuse, live result parsing, all-provider aggregation, and MCP cleanup. ([`6071506`](https://github.com/FlanChanXwO/pixiv-cli/commit/60715066c28a2e5378f730e9af73205303b1728a), [`c01402a`](https://github.com/FlanChanXwO/pixiv-cli/commit/c01402afc69f0d510a78e911bc38f9ab0039532b), [`7951909`](https://github.com/FlanChanXwO/pixiv-cli/commit/7951909d7491c82c59c283262b468650cd1c5784), [`50d13a3`](https://github.com/FlanChanXwO/pixiv-cli/commit/50d13a3bf5b8f082816a0cf080f97039f4914a60), [`f9c1525`](https://github.com/FlanChanXwO/pixiv-cli/commit/f9c15256042dd69bec6a404cf69f4bb1ed0a629f), [`341cfbb`](https://github.com/FlanChanXwO/pixiv-cli/commit/341cfbb233baabac57e9261f16e29946b2b043fc), [`471af9b`](https://github.com/FlanChanXwO/pixiv-cli/commit/471af9b9752c29bbe2f03f22c3cc6f4ae62c65f9), [`8337c6f`](https://github.com/FlanChanXwO/pixiv-cli/commit/8337c6f942db9522417df6d5d10e1aca225c1bf3), [`05df0d5`](https://github.com/FlanChanXwO/pixiv-cli/commit/05df0d5a05c9fe6978eaddb7efb9e7d2bd8a0e00), [`a70c490`](https://github.com/FlanChanXwO/pixiv-cli/commit/a70c490e7000d4e4d123f0237a68539dadac550f))

**Full Changelog**: [v1.0.0...v1.0.1](https://github.com/FlanChanXwO/pixiv-cli/compare/v1.0.0...v1.0.1)
