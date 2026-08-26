---
phase_id: 5
goal_id: docker-container-release
name: "Run integrated verification and review"
status: pending
criterion_id: C5
---

# Phase 5: Run integrated verification and review

## Objective

Prove the complete Docker release goal against the repository's actual gates and review the release/security boundary before declaring Goal Mode complete.

## Exit Criterion

C5 is satisfied only when all relevant focused and repository-wide checks pass with fresh evidence, the credential-free two-architecture container smoke run is green, and review finds no unresolved blocking issue in scope.

## Dependencies

- Phases 1–4 complete.

## Plan

- [ ] **Step 1 — Focused verification:** Run `go test ./scripts/internal/releaseworkflow -count=1`, `go test ./scripts/tests/containerrelease -count=1`, `go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml`, and `go test ./scripts/tests/documentation -count=1`.
- [ ] **Step 2 — Package/regression verification:** Run `sh scripts/test-package-release.sh`, then `go test ./...`. Run any additional directly affected lint/build checks required by the files actually changed; do not substitute static checks for the real Docker smoke.
- [ ] **Step 3 — Real container evidence:** Confirm the latest credential-free container smoke workflow for the goal branch/commit is green on both native Linux architectures and that version/non-root/state-path assertions ran rather than skipped.
- [ ] **Step 4 — Diff hygiene:** Run `git diff --check` and confirm only files justified by this goal changed. Verify no generated image archives, local database, token, cache, or registry credential entered Git history.
- [ ] **Step 5 — Release boundary review:** Use the repository review checklist to inspect immutable tag binding, permissions, action SHA pinning, GHCR auth reachability, stable/prerelease tag semantics, glibc/native runner mapping, and error behavior. Fix blocking findings and rerun affected evidence.
- [ ] **Step 6 — Goal verifier:** Re-read every C1–C5 criterion against current repository state and evidence. Mark `COMPLETE` only when every criterion has direct current-state evidence; otherwise set `CONTINUE` or `BLOCKED` with the exact missing proof.

## Phase Progress Log

### Phase iteration 0 — plan created

- **Status**: pending
- **Next step**: Wait until phases 1–4 are complete, then execute the full verification sequence.
