---
name: pixiv-cli-native
description: Change or validate pixiv-cli ugoira encoding, pure-Go platform builds, Linux ABI, and native runner evidence. Use when encoder behavior, target builds, or artifact provenance change; routine leaf Go changes use the develop/test workflow.
---

# Maintain Encoding and Platform Builds

Read `AGENTS.md`, the affected encoder/build owner, and [Ugoira Go encoder](../../../docs/en/maintainers/development.md#ugoira-go-encoder). `ci/platforms.json` owns runners and capabilities; `go.mod` owns the toolchain. `scripts/build-platform.sh` owns exact-target Go builds and canonical archives.

## Change the responsible boundary

Use [the test workflow](../pixiv-cli-test/SKILL.md) for behavior changes. Keep the downloader's `ugoira.Encoder` seam: ZIP metadata order, frame timing, resizing, transparency, playback, cancellation and atomic publication belong to `internal/media/ugoira`. Preserve the documented decoder-derived allocation boundary and buffer ownership. A slow encode is not a timeout failure.

Production builds use `CGO_ENABLED=0`; only explicit race-detector steps enable cgo. Ported algorithms keep their source/license notices through `third_party/attributions.json` and the generated license bundle. A toolchain change needs verification of affected targets, not just a version-field edit.

## Verify the changed behavior

Select the relevant existing checks:

```bash
CGO_ENABLED=0 go test ./internal/media/ugoira/... ./internal/media/downloader -count=1
go run ./scripts/cmd/licensebundle --check
sh scripts/test-build-platform.sh
go test ./tools/platformmatrix ./scripts/internal/nativeevidence -count=1
```

The license check consumes cached module sources. Report missing inputs instead of downloading or installing tools without approval. For quality/resource comparisons, use the documented offline corpus and fixed legacy/Go settings. Keep frozen baselines and complete inputs; compare each required sample/format rather than hiding failures in an overall average. FFmpeg is an explicit test reference, not a runtime backend.

Linux artifacts pass `scripts/cmd/linuxabi` against the actual ELF, including static binaries. Cross-compilation proves compilation, not native execution. Check the built and unpacked binary's version and complete archive/license contents.

## Consume matching evidence

The explicit `native-evidence.yml` entry requires audited main. PR Platform workers also emit schema 3 binary/archive bundles; their executable workflow comes from the trusted base ref. Inspect the actual producer and source SHA rather than assuming a PR-head workflow edit changes it.

Consolidate exactly six complete bundles from one source/version/run into a new directory with `scripts/cmd/nativeevidence`. Keep failed or missing results visible; fixture success is not six-platform evidence. Report actual native executions and outstanding platforms. Release, signing, tags and publisher actions require separate authorization.
