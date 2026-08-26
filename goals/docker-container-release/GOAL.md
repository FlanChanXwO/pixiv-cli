---
goal_id: docker-container-release
title: "Ship Docker as a first-class pixiv-cli release target"
status: PLANNED
planning_level: phase
iteration: 0
max_iterations: 50
current_phase: 1
phases_total: 5
started_at: null
last_evaluation: null
blocker: null
active_step: "phase-1 step 1"
execution_mode: tdd
last_memory_checkpoint: 0
max_hours: null
---

# GOAL: Ship Docker as a first-class pixiv-cli release target

## Objective

Add official Docker container support to `pixiv-cli` as a first-class distribution target. Container builds must start from the same immutable release tag as the existing native production builds, run in parallel with the native production build path after the shared release quality gates, produce native `linux/amd64` and `linux/arm64` images, and publish a multi-architecture image to GHCR as part of the tagged release workflow.

The container distribution must preserve the existing product model instead of creating a Docker-specific product: the same `pixiv` binary, the same `~/.pixiv-cli` state namespace, the same CLI/MCP behavior, and the same release provenance and validation expectations apply.

## Repository facts that constrain the goal

- Release source is an immutable `v*` tag and the existing release policy is fail-closed.
- Production builds require `CGO_ENABLED=1` and the committed Rust ugoira static library.
- Linux release compatibility is explicitly tied to glibc 2.35 and native `ubuntu-22.04` / `ubuntu-22.04-arm` runners.
- User state lives under `~/.pixiv-cli`; downloads already support an explicit output path.
- The repository has no `.cursor/goal.config.yml`, so evidence commands for this goal are derived from `AGENTS.md`, maintainer docs, and the existing release/documentation gates.

## Scope decisions

- Registry: GitHub Container Registry (`ghcr.io/flanchanxwo/pixiv-cli`).
- Architectures: `linux/amd64` and `linux/arm64` only for the first container release.
- Runtime base: a glibc-based Debian slim image pinned by immutable digest; Alpine/musl and `scratch` are out of scope.
- Builds use native Linux runners; QEMU cross-build is out of scope.
- Container build jobs must not receive registry write permission. They produce verified image artifacts; registry publication happens in a separate publish job.
- Stable releases publish `vX.Y.Z` and advance `latest`; prereleases publish only the exact version tag and must not advance `latest`.
- Container upgrades are performed by pulling a newer image. This goal documents that rule but does not change `pixiv update` behavior.

## Non-goals

- No Docker-specific authentication protocol or token storage format.
- No rewrite of `auth login`, OAuth callback handling, or MCP transport.
- No Docker Hub publication.
- No Kubernetes/Helm/Compose deployment layer.
- No Alpine/musl support, QEMU build path, or additional CPU architectures.
- No new Go runtime dependency solely for Docker support.
- No change to self-update implementation in this goal.
- No versioned changelog entry during this implementation PR; release notes remain release-preparation work per repository policy.

## Completion Criteria (ALL must be satisfied)

- [ ] C1: The release policy formally recognizes container build/publish jobs, preserves immutable-tag provenance, keeps registry write permission out of build jobs, and fails closed if the Docker release contract drifts.
- [ ] C2: A production container image can be built from the versioned native Linux binary with a pinned glibc runtime base, runs as a non-root user, preserves the `~/.pixiv-cli` state path, uses `/work` as the working directory, and reports the exact release version.
- [ ] C3: Tagged releases build `linux/amd64` and `linux/arm64` container images on native runners in parallel with native production builds and publish a GHCR multi-arch image with correct stable/prerelease tag semantics and OCI provenance labels.
- [ ] C4: English and Simplified Chinese user/maintainer documentation describe Docker installation, persistent state, download bind mounts, stdin-based `auth import`, MCP stdio usage, release tagging, and pull-based upgrades without claiming Docker-specific product behavior.
- [ ] C5: Focused container/release tests, documentation tests, repository Go tests, release workflow policy validation, diff checks, and the credential-free container smoke workflow all pass with fresh evidence.

## Evidence Required

| Criterion | Verification command / evidence | Evidence location |
|-----------|---------------------------------|-------------------|
| C1 | `go test ./scripts/internal/releaseworkflow -count=1` and `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` | Test output + Progress Log |
| C2 | `go test ./scripts/tests/containerrelease -count=1` plus native Docker smoke: `pixiv --version`, `pixiv config path`, and non-root user assertion | Test output + container smoke CI log |
| C3 | `go test ./scripts/tests/containerrelease -count=1` plus successful `linux/amd64` and `linux/arm64` jobs in the credential-free container smoke workflow; release policy statically proves GHCR permissions/tag rules | Test output + GitHub Actions run |
| C4 | `go test ./scripts/tests/documentation -count=1` | Test output |
| C5 | `go test ./...`, `sh scripts/test-package-release.sh`, `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml`, `go test ./scripts/tests/documentation -count=1`, `git diff --check`, and required GitHub CI checks | Local/CI verification logs |

## Budget Limits

- Max iterations: 50 Goal Mode iterations.
- Max hours: unset; use host/session limits rather than inventing a repository-specific wall-clock cap.

These are agent-execution controls only and must not become product runtime timeouts, retry limits, truncation rules, or hidden fallbacks.

## Master Plan

| Phase | Name | Status | Plan file | Exit criterion |
|-------|------|--------|-----------|----------------|
| 1 | Encode the release contract | pending | `phases/phase-1.md` | C1 passes with a witnessed Red → Green policy test cycle |
| 2 | Build the container runtime contract | pending | `phases/phase-2.md` | C2 passes on a real Docker image smoke build |
| 3 | Integrate native multi-arch build and GHCR publication | pending | `phases/phase-3.md` | C3 passes without registry write permission in build jobs |
| 4 | Document the supported Docker UX | pending | `phases/phase-4.md` | C4 passes and English/Chinese semantics match |
| 5 | Run integrated verification and review | pending | `phases/phase-5.md` | C5 is green with fresh evidence and no unresolved blocking review findings |

## Current Execution Context

- **Planning level**: phase
- **Current phase**: 1
- **Active plan file**: `phases/phase-1.md`
- **Active step**: phase-1 step 1

## Release consistency boundary

GitHub Release and GHCR are separate publication systems and cannot be made transactionally atomic. The implementation must keep container **build** failures ahead of GitHub Release publication by making verified container artifacts a prerequisite of the release publish path. Registry push occurs in a dedicated post-Release job with only `packages: write`; a push failure leaves the release workflow failed and must be recoverable by rerunning container publication without rebuilding or resigning the native release.

The implementation must document this recovery boundary rather than hiding it behind retries or pretending cross-service rollback exists.

## Progress Log

### Iteration 0 — intake and planning completed

- **Action**: Converted the Docker release brainstorm into a Goal Mode contract and five verifiable phases.
- **Status**: PLANNED
- **Evidence basis**: Existing repository release policy, native Linux/Rust build contract, configuration path contract, documentation rules, and Goal Mode GOAL/PHASE format.
- **Next step**: Phase 1 Step 1 — add a failing release-policy test that expresses the container job contract before editing the release workflow implementation.
