---
phase_id: 1
goal_id: docker-container-release
name: "Encode the release contract"
status: pending
criterion_id: C1
---

# Phase 1: Encode the release contract

## Objective

Extend the repository's fail-closed release policy so Docker is an audited release target rather than an unaudited side workflow.

## Exit Criterion

C1 is satisfied only when the release policy tests and policy command prove the container job topology, immutable-tag source, native Linux runner mapping, and least-privilege registry boundary.

## Dependencies

- Existing `.github/workflows/release.yml` contract.
- Existing `scripts/internal/releaseworkflow` policy and tests.
- Existing Linux release target/toolchain contract.

## Plan

- [ ] **Step 1 — Red:** Add focused release-policy tests that require the planned container jobs and reject missing jobs, wrong `needs`, movable source refs, registry permission in build jobs, QEMU/cross-build drift, and prerelease `latest` publication. Run the focused tests and confirm they fail because current behavior lacks the container contract.
- [ ] **Step 2 — Green:** Extend `scripts/internal/releaseworkflow` with the smallest container-policy rules needed to satisfy the failing tests. Reuse existing workflow YAML helpers and Linux target provenance instead of creating a second parser or duplicated policy framework.
- [ ] **Step 3 — Refactor:** Consolidate only genuinely shared Linux release target metadata if duplication is now stable and harmful; do not generalize unrelated release policy.
- [ ] **Step 4 — Verify:** Run `go test ./scripts/internal/releaseworkflow -count=1` and `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml`. Record both the initial Red evidence and final Green evidence in the Goal progress log.
- [ ] **Step 5 — Scope audit:** Confirm no production behavior, auth behavior, updater behavior, or registry credential has been introduced in this phase.

## Phase Progress Log

### Phase iteration 0 — plan created

- **Status**: pending
- **Next step**: Step 1 — write and run the failing policy tests first.
