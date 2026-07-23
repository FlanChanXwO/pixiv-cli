# v0.1.0 — 2026-07-13

## Added

- Introduced project-level release notes, `sh scripts/build.sh`, `pixiv version [--json]`, and `pixiv update [--check] [--prerelease] [--proxy URL]` for Homebrew, exact-tag `go install`, and signed Release-binary update paths. Development builds reject updates.
- Added an opt-out `update_check_enabled` stable-update hint. Successful normal CLI commands check at most once per 24 hours, wait at most three seconds, and never pollute JSON/MCP stdout or change the business-command exit code.
- Added the built-in Rust ugoira GIF/APNG encoder so production downloads no longer require `ffmpeg`.
- Added committed Rust static libraries, manifests, source-digest verification, six-platform native build/smoke evidence, fixed six-target release assets, checksum/Ed25519 signatures, and Homebrew stable/beta formula rendering and verification.

## Changed

- **Breaking:** local auth accounts are identified by Pixiv UID rather than custom names. `auth add/login` no longer accepts an account name; `auth use/remove/check` use UID, and `--uid` replaces `--profile`.
- **Breaking:** `auth.json` now uses `default_user_id` and `accounts[].user_id/username`. Existing `default_account/accounts[].name` files require fresh authentication.
- Browser login keeps a terminal-paste fallback for official callbacks, `pixiv://` URLs, or raw authorization codes. It accepts official callback and `pixiv://account/login` code handoff while retaining strict local-loopback state validation.
- On macOS, login can register a local `pixiv://` helper and open the default browser to reuse an existing Pixiv session. It relays only the final callback to the current CLI loopback and retains explicit manual recovery if no callback is available.

## Fixed

- Fixed Linux Rust `libm` linkage and Windows checkout/text handling for Rust sources, Cargo vendor data, licenses, and source-digest inputs. Windows uses LLD-backed Clang with the necessary Rust import libraries, and release ZIP creation uses preinstalled 7-Zip.

## Security

- Established protected release infrastructure with isolated Ed25519 signing and Homebrew tap credentials; private keys stay in the protected Environment and recovery Keychain copy.
- Added fail-closed native-evidence and release-workflow policies for pinned actions, minimal permissions, required job ancestry, canonical immutable source checkouts, and strict publication-channel mapping.
- Kept Homebrew deployment credentials confined to the final protected push step and validated the complete vendored Rust dependency closure in an empty Cargo cache.
- Added a rotatable embedded Ed25519 public-key trust root and signature/checksum/version verification before installer replacement. Release sources, selected archives, extraction paths, cancellation, cache permissions, and symlink targets are validated conservatively.
- v0.1.0 does not include Apple notarization or Windows Authenticode; directly downloaded binaries may still trigger Gatekeeper or SmartScreen.
