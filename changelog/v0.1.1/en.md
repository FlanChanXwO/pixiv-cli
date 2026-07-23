# v0.1.1 — 2026-07-13

## Fixed

- Correctly recognizes a directly downloaded Release binary when no expected `GOBIN`/`GOPATH/bin/pixiv` exists, allowing `pixiv update --check` to run.
- Fixed Windows release linking between MinGW GCC and the MSVC Rust static library. Six-platform Go tests, race/vet/pre-commit checks, and final builds use audited cgo linkers; Windows uses LLD-backed Clang.
- Made login-test callback fixtures race-safe and isolated tests that must not invoke real macOS URL handlers or AppleScript.
- Added an audited recovery route for a failed first publication of an immutable tag. Recovery remains tied to the original tag, uses separate test/production runners, and rebuilds production assets from a clean checkout.
- Corrected Windows recovery-gate assumptions around ACLs, `.exe`, CRLF, sharing, and paths while retaining static policy restrictions and immutable-tag-only production builds.
