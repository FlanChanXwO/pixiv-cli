---
name: pixiv-cli-native
description: Change or validate pixiv-cli ugoira encoding, pure-Go platform builds, and native runner evidence. Use when encoder behavior, target builds, or artifact provenance change; routine leaf Go changes use the develop/test workflow.
---

# Maintain Encoding and Platform Builds

Read `AGENTS.md`, the affected encoder/build owner, and [Ugoira Go encoder](../../../docs/en/maintainers/development.md#ugoira-go-encoder). `scripts/build/platforms.json` owns runners and capabilities; `go.mod` owns the toolchain. `scripts/build/platform.sh` owns exact-target Go builds and canonical archives; `scripts/build/matrix.sh` validates the registry.

## Change the responsible boundary

Use [the test workflow](../pixiv-cli-test/SKILL.md) for behavior changes. Keep the downloader's `ugoira.Encoder` seam: ZIP metadata order, frame timing, resizing, transparency, playback, cancellation and atomic publication belong to `internal/media/ugoira`. Preserve the documented decoder-derived allocation boundary and buffer ownership. A slow encode is not a timeout failure.

Production builds use `CGO_ENABLED=0`; only explicit race-detector steps enable cgo. Ported algorithms keep their source/license notices through `third_party/attributions.json` and the generated license bundle. A toolchain change needs verification of affected targets, not just a version-field edit.

## Verify the changed behavior

Select the relevant existing checks:

```bash
CGO_ENABLED=0 go test ./internal/media/ugoira/... -count=1
go run ./scripts/build/licensebundle --check
sh scripts/build/platform.sh --target linux/amd64 --output-dir /tmp/pixiv-platform-check
sh scripts/build/matrix.sh --capability smoke
```

The license check consumes cached module sources. Report missing inputs instead of downloading or installing tools without approval. For quality/resource comparisons, use the documented offline corpus and fixed legacy/Go settings. Keep frozen baselines and complete inputs; compare each required sample/format rather than hiding failures in an overall average.

`CGO_ENABLED=0` produces a static Linux ELF; confirm with `file` when the build recipe changes. Cross-compilation proves compilation, not native execution. Check the built and unpacked binary's version and archive's sole executable member.

## Consume matching evidence

The Platform smoke workflow executes the built binary and the native encoder tests on every shipped target; those runs are the native execution evidence. Its executable workflow comes from the trusted base ref. Inspect the actual run and source SHA rather than assuming a PR-head workflow edit changes it. Keep failed or missing targets visible; fixture success is not six-platform evidence. Report actual native executions and outstanding platforms. Release, signing, tags and publisher actions require separate authorization.
