---
phase_id: 4
goal_id: docker-container-release
name: "Document the supported Docker UX"
status: pending
criterion_id: C4
---

# Phase 4: Document the supported Docker UX

## Objective

Make Docker a documented official installation/usage path while preserving the existing CLI, auth, state, MCP, and update contracts.

## Exit Criterion

C4 is satisfied only when English and Simplified Chinese documentation agree on Docker installation and operational boundaries and the repository documentation tests pass.

## Dependencies

- Phases 2 and 3 define the final image/tag/runtime behavior.
- `.agents/skills/pixiv-cli-docs/SKILL.md` documentation routing rules.

## Plan

- [ ] **Step 1 — Red where applicable:** Extend documentation fixtures/tests if the new official installation path needs a stable link/command contract. Run the focused documentation test before documentation changes when a testable contract is added.
- [ ] **Step 2 — User docs:** Update `README.md` first as the canonical English installation/quick-start entry, then synchronize `README.zh-CN.md`. Cover exact-version and `latest` pull semantics, persistent `~/.pixiv-cli` volume, `/work` bind mount for downloads, and image architecture support.
- [ ] **Step 3 — Auth docs:** Recommend stdin-based `pixiv auth import` with the persistent state volume for container use so refresh tokens do not need to enter image layers or CLI argv. Do not claim that `auth login` has new container-specific callback behavior.
- [ ] **Step 4 — MCP docs:** Show `docker run --rm -i ... mcp` as the stdio pattern. Preserve the rule that stdout belongs to MCP JSON-RPC and do not add a network MCP transport.
- [ ] **Step 5 — Upgrade docs:** State that container installations upgrade by pulling/redeploying a newer image. Do not claim `pixiv update` is container-aware or change updater semantics in this goal.
- [ ] **Step 6 — Maintainer docs:** Update `docs/en/maintainers/development.md` with container build/release verification and then synchronize `docs/zh-CN/maintainers/development.md`. Route rather than duplicate long release rules in multiple places.
- [ ] **Step 7 — Verify:** Run `go test ./scripts/tests/documentation -count=1` and `git diff --check`; inspect bilingual commands, paths, registry names, and security language for semantic parity.

## Phase Progress Log

### Phase iteration 0 — plan created

- **Status**: pending
- **Next step**: Step 1 — determine which documentation behavior can be locked by a focused failing test before edits.
