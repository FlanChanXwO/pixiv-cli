---
name: pixiv-cli-native
description: Change or validate pixiv-cli Rust ugoira, cgo/FFI, committed static libraries, vendored dependencies, Linux ABI, and native runner evidence. Use when native sources, library hashes, target toolchains, or encoder behavior change; not for routine Go-only or documentation edits.
---

# Maintain pixiv-cli Native Code

Read `AGENTS.md`, the affected FFI/encoder owner, and the Rust/staticlib sections of `docs/en/maintainers/development.md`. Use `ci/platforms.json` for target/compiler/toolchain metadata; do not maintain a second platform list in a new helper or skill.

## Change safely

1. Identify whether the change affects Rust source, the C ABI, cgo selection, vendoring, staticlib provenance, or release linking. Record the affected behavior and evidence required before altering binary inputs.
2. Use the repository's Red/Green workflow for behavior changes. Keep unsafe FFI ownership, allocation/free pairing, panic containment, cancellation, and error translation explicit. Avoid `unwrap`/`expect` on fallible external input or crossing the FFI boundary with unwinding.
3. Work in `internal/media/ugoira/rust` for Cargo commands so its `.cargo/config.toml` source replacement applies. Use locked/offline dependency resolution. Never hand-edit vendored crates or hide a network fallback; approved dependency changes must regenerate the complete closure and license evidence.
4. Preserve the single encoder/FFI seam. A slow normal encode is not a timeout failure. Keep decoder-derived resource constraints and explicit errors; do not add another encoder or ffmpeg runtime fallback.

## Verify the touched boundary

From the crate directory, run the appropriate focused test, then:

```bash
cargo fmt --check
cargo test --locked --offline
cargo clippy --locked --offline --all-targets -- -D warnings
```

From the repository root, relevant checks include:

```bash
go test ./internal/media/ugoira/staticlib -run '^TestCommittedManifestWhenPresent$' -count=1
go test ./internal/media/ugoira/... -count=1
go run ./scripts/cmd/licensebundle --check
sh scripts/test-build-staticlibs.sh
sh scripts/test-build-platform.sh
sh scripts/test-rust-vendor.sh
```

These commands can compile native code and consume significant resources; confirm the required existing toolchains and scope first. Report missing targets, compilers, or components instead of installing or pretending to validate them.

`scripts/build-platform.sh` owns the exact-target Rust/Go/archive build. Linux release binaries must pass `go run ./scripts/cmd/linuxabi --binary PATH` against the actual ELF, not merely execute on a newer host. Read the existing script and workflow before supplying target/version/compiler arguments.

## Provenance and publication

Committed libraries and `manifest.json` bind every shipped target from the registry to one source digest. A single-target rebuild invalidates cross-platform completeness; do not manufacture a new manifest around partial evidence. Do not commit `rust/target/`. Preserve `.gitattributes` byte rules for source/vendor/license inputs.

Use the credential-free `native-evidence.yml` workflow for actual native runner evidence. Consolidate only a complete same-source, same-version run using the existing `nativeevidence` tool. Never splice successful artifacts from different source digests or label a fixture test as all-target native evidence. Review any backfilled libraries and manifest together, then rerun integrity and real GIF/APNG linking smoke.

A toolchain upgrade requires rebuilding and verifying every affected shipped target, not just editing a version field. Release, signing, tags, and publisher actions remain separately authorized. Report which native executions actually occurred and which remain unverified.
