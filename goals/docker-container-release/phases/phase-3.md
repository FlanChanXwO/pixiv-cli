---
phase_id: 3
goal_id: docker-container-release
name: "Integrate native multi-arch build and GHCR publication"
status: pending
criterion_id: C3
---

# Phase 3: Integrate native multi-arch build and GHCR publication

## Objective

Add native `linux/amd64` and `linux/arm64` container builds to the tagged release graph in parallel with native production builds, then publish a multi-arch GHCR image through a least-privilege publication boundary.

## Exit Criterion

C3 is satisfied only when both architectures build natively from the immutable tag, container build jobs hold no registry write permission, publication uses only verified image artifacts, and stable/prerelease tag behavior is proven by tests and workflow policy.

## Dependencies

- Phase 1 policy contract complete.
- Phase 2 runtime image contract complete.
- Existing `ubuntu-22.04` / `ubuntu-22.04-arm` Linux release provenance.

## Plan

- [ ] **Step 1 — Red:** Extend focused workflow tests for the concrete release graph: `build_container` starts after the shared quality gate and runs alongside `build_production`; exact native Linux runners/toolchains are required; `publish_container` is the only job with `packages: write`; exact-version tags are always published; `latest` is stable-only. Run and witness failure before editing the workflow.
- [ ] **Step 2 — Green/build:** Add a two-target `build_container` matrix using native `ubuntu-22.04` and `ubuntu-22.04-arm`. Each target checks out the immutable release tag, validates source, uses the audited Rust toolchain/staticlib path, builds a versioned Linux binary, applies the existing Linux ABI gate, builds the Docker image, and exports a transportable image artifact. It must not log in to a registry or receive `packages: write`.
- [ ] **Step 3 — Parallelism proof:** Ensure the release DAG makes `build_container` and `build_production` siblings after the required shared quality gate rather than serializing one behind the other.
- [ ] **Step 4 — Credential-free smoke workflow:** Add a narrowly triggered container smoke workflow for relevant PR/main changes. It builds both native architectures without registry credentials and runs the Phase 2 smoke assertions. Reuse full-SHA pinned actions and existing repository permission conventions.
- [ ] **Step 5 — Green/publish:** Add `publish_container` after the verified container artifacts and GitHub Release publication. Grant only `packages: write` (and the minimum read permission required), authenticate to GHCR with the workflow token, load/push the two architecture images, create the multi-arch manifest, and apply OCI source/revision/version/license labels.
- [ ] **Step 6 — Tag policy:** Publish `ghcr.io/flanchanxwo/pixiv-cli:vX.Y.Z` for every release. Advance `latest` only when the existing release channel classifier reports `stable`; prereleases must never move `latest`.
- [ ] **Step 7 — Recovery semantics:** Make publication rerunnable/idempotent for the same immutable tag where the registry permits it. Do not hide registry errors with retry loops or report a failed push as success. Document that GHCR publication failure leaves the workflow failed after GitHub Release publication and is repaired by rerunning the publication job/path.
- [ ] **Step 8 — Verify:** Run release policy/container tests and inspect both architecture artifacts in the smoke workflow. Confirm no QEMU setup, Docker Hub credentials, or new third-party GitHub Action dependency was added without explicit necessity and approval.

## Phase Progress Log

### Phase iteration 0 — plan created

- **Status**: pending
- **Next step**: Step 1 — add failing release-graph and tag-policy tests.
