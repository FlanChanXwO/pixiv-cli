# Goal Input — Docker as a first-class pixiv-cli release target

## 原始需求

Add official Docker container support to `pixiv-cli` as a first-class distribution target.

### 核心要求

1. Container builds must start from the same immutable release tag as the existing native production builds.
2. Container builds run in parallel with the native production build path after the shared release quality gates.
3. Produce native `linux/amd64` and `linux/arm64` images (native runners, not QEMU cross-build).
4. Publish a multi-architecture image to GHCR (`ghcr.io/flanchanxwo/pixiv-cli`) as part of the tagged release workflow.

### 产品模型约束

- Container distribution must preserve the existing product model instead of creating a Docker-specific product.
- The same `pixiv` binary, the same `~/.pixiv-cli` state namespace, the same CLI/MCP behavior, and the same release provenance and validation expectations apply.

### Scope decisions

- Registry: GitHub Container Registry (`ghcr.io/flanchanxwo/pixiv-cli`).
- Architectures: `linux/amd64` and `linux/arm64` only for the first container release.
- Runtime base: a glibc-based Debian slim image pinned by immutable digest; Alpine/musl and `scratch` are out of scope.
- Builds use native Linux runners; QEMU cross-build is out of scope.
- Container build jobs must not receive registry write permission. They produce verified image artifacts; registry publication happens in a separate publish job.
- Stable releases publish `vX.Y.Z` and advance `latest`; prereleases publish only the exact version tag and must not advance `latest`.
- Container upgrades are performed by pulling a newer image. This goal documents that rule but does not change `pixiv update` behavior.

### Non-goals

- No Docker-specific authentication protocol or token storage format.
- No rewrite of `auth login`, OAuth callback handling, or MCP transport.
- No Docker Hub publication.
- No Kubernetes/Helm/Compose deployment layer.
- No Alpine/musl support, QEMU build path, or additional CPU architectures.
- No new Go runtime dependency solely for Docker support.
- No change to self-update implementation in this goal.
- No versioned changelog entry during this implementation PR; release notes remain release-preparation work per repository policy.

### Completion Criteria

- C1: The release policy formally recognizes container build/publish jobs, preserves immutable-tag provenance, keeps registry write permission out of build jobs, and fails closed if the Docker release contract drifts.
- C2: A production container image can be built from the versioned native Linux binary with a pinned glibc runtime base, runs as a non-root user, preserves the `~/.pixiv-cli` state path, uses `/work` as the working directory, and reports the exact release version.
- C3: Tagged releases build `linux/amd64` and `linux/arm64` container images on native runners in parallel with native production builds and publish a GHCR multi-arch image with correct stable/prerelease tag semantics and OCI provenance labels.
- C4: English and Simplified Chinese user/maintainer documentation describe Docker installation, persistent state, download bind mounts, stdin-based `auth import`, MCP stdio usage, release tagging, and pull-based upgrades without claiming Docker-specific product behavior.
- C5: Focused container/release tests, documentation tests, repository Go tests, release workflow policy validation, diff checks, and the credential-free container smoke workflow all pass with fresh evidence.

### Release consistency boundary

GitHub Release and GHCR are separate publication systems and cannot be made transactionally atomic. The implementation must keep container build failures ahead of GitHub Release publication by making verified container artifacts a prerequisite of the release publish path. Registry push occurs in a dedicated post-Release job with only `packages: write`; a push failure leaves the release workflow failed and must be recoverable by rerunning container publication without rebuilding or resigning the native release.

The implementation must document this recovery boundary rather than hiding it behind retries or pretending cross-service rollback exists.

### Evidence Required

| Criterion | Verification command / evidence | Evidence location |
|-----------|---------------------------------|-------------------|
| C1 | `go test ./scripts/internal/releaseworkflow -count=1` and `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` | Test output + Progress Log |
| C2 | `go test ./scripts/tests/containerrelease -count=1` plus native Docker smoke: `pixiv --version`, `pixiv config path`, and non-root user assertion | Test output + container smoke CI log |
| C3 | `go test ./scripts/tests/containerrelease -count=1` plus successful `linux/amd64` and `linux/arm64` jobs in the credential-free container smoke workflow; release policy statically proves GHCR permissions/tag rules | Test output + GitHub Actions run |
| C4 | `go test ./scripts/tests/documentation -count=1` | Test output |
| C5 | `go test ./...`, `sh scripts/test-package-release.sh`, `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml`, `go test ./scripts/tests/documentation -count=1`, `git diff --check`, and required GitHub CI checks | Local/CI verification logs |

### Budget Limits

- Max iterations: 50 Goal Mode iterations.
- Max hours: unset; use host/session limits rather than inventing a repository-specific wall-clock cap.
- These are agent-execution controls only and must not become product runtime timeouts, retry limits, truncation rules, or hidden fallbacks.
