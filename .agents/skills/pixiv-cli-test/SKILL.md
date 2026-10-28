---
name: pixiv-cli-test
description: Select and run pixiv-cli checks for a behavior change, refactor, platform boundary, or PR. Follow the coding handbook for test admission and the repository's offline, native, and authorized live verification workflows.
---

# Test pixiv-cli

## Read before selecting tests

Read [Test submission rules](../../../docs/en/maintainers/coding-standards.md#test-submission-rules) before adding, changing, or deleting test code. Use the [development guide](../../../docs/en/maintainers/development.md) for test layout, CI scope, and manual live-verification procedures.

## Workflow

1. Read the acceptance conditions, changed owner, existing tests, and workflow selectors. Identify the public contract and the boundary responsible for proving it.
2. Apply the handbook's Red/Green and characterization requirements; record any approved exception to an applicable Red requirement.
3. Admit permanent tests using the handbook checklist, caps, CI exemptions, and removal criteria. For temporary probes, follow the handbook's ignored-path and synthetic-credential guidance.
4. Select the smallest checks that cover the changed behavior and its callers. Reuse valid evidence until a relevant change or failure invalidates it. Use [native](../pixiv-cli-native/SKILL.md) guidance for platform execution and the development guide's live procedure only when explicitly authorized.
5. For a suite rewrite, record counted files, line counts and checklist rows, preserved CI inventory, and total test-line change in the PR.

## Select checks

| Change | Checks |
| --- | --- |
| Documentation or instructions | Links, metadata, commands, `git diff --check`, and affected existing tooling |
| Local Go behavior | Focused test, scoped vet, and executable build when affected |
| Shared/public contract or release candidate | On the final candidate: `go test ./... -count=1`, `go vet ./...`, and `sh scripts/build.sh` |
| Concurrency, persistence, or shared runtime | Add `CGO_ENABLED=1 go test -race ./... -count=1` and the relevant native gate |
| PR metadata | `go run ./scripts/pr validate --body-file PATH --cli pixiv` |
| Platform or release | Existing release/CI checks and the [native workflow](../pixiv-cli-native/SKILL.md) |

For PR readiness, use the [PR workflow](../pixiv-cli-pr/SKILL.md) to inspect statuses and checks on the exact head, keeping skipped checks distinct from executed passes. CI scope comes from `.github/ci-change-scope.gitignore` and `scripts/ci/classify-change-scope.sh`.

## Before handoff

- Report the tested revision, commands, actual pass/fail/skip results, and the effect of any unrun required check.
- Separate offline test evidence from native or live observations; describe only evidence actually obtained.
- Stop when the requested behavior and mandatory evidence are complete. Do not expand verification without new evidence or a changed premise.
