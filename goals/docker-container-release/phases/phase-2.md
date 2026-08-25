---
phase_id: 2
goal_id: docker-container-release
name: "Build the container runtime contract"
status: pending
criterion_id: C2
---

# Phase 2: Build the container runtime contract

## Objective

Create the minimal production Docker packaging layer around the existing versioned Linux binary without creating a new compile/runtime product model.

## Exit Criterion

C2 is satisfied only when a real image build proves the exact release version, non-root execution, glibc runtime compatibility, expected home/state path, and `/work` working directory.

## Dependencies

- Phase 1 complete.
- Existing Linux production build and ugoira staticlib contract.
- Existing `internal/config/paths` behavior.

## Plan

- [ ] **Step 1 — Red:** Add `scripts/tests/containerrelease` tests defining the Dockerfile/package contract: immutable base digest, no Alpine/musl/scratch base, non-root final user, expected `HOME`, `/work`, `ENTRYPOINT`, OCI metadata inputs, and no embedded secret/state files. Run the focused tests and witness failure before creating the Dockerfile.
- [ ] **Step 2 — Green:** Add a minimal `Dockerfile` that copies a prebuilt versioned `pixiv` binary into a pinned Debian slim runtime, installs only required runtime material such as CA certificates, creates a dedicated non-root user, sets `HOME=/home/pixiv`, `WORKDIR /work`, and `ENTRYPOINT ["/usr/local/bin/pixiv"]`.
- [ ] **Step 3 — Context hygiene:** Add `.dockerignore` that excludes repository-only/build-noise content without excluding files required by the packaging contract. Do not add secret-specific guesses; rely on the minimal build context and existing repository secret rules.
- [ ] **Step 4 — Real smoke:** Build the image on native Linux and run assertions for `id -u != 0`, exact `pixiv --version`, and `pixiv config path` resolving under `/home/pixiv/.pixiv-cli/`. Also verify `/work` is the default working directory.
- [ ] **Step 5 — Functional boundary:** Verify the container still uses the same binary and existing CLI/MCP entrypoints. Do not introduce wrapper scripts that reinterpret CLI arguments or add container-only config environment variables.
- [ ] **Step 6 — Verify:** Re-run focused container tests and the relevant package/staticlib tests. Record image ID/digest and smoke outputs as evidence without recording credentials or local database contents.

## Phase Progress Log

### Phase iteration 0 — plan created

- **Status**: pending
- **Next step**: Step 1 — define the failing Docker contract tests.
