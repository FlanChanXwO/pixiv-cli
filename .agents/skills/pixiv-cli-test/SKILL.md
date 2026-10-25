---
name: pixiv-cli-test
description: Select and run pixiv-cli checks, admit tests within the seven-contract checklist and size caps, and distinguish offline, native CI, and authorized manual live verification.
---

# Test pixiv-cli

## Determine the evidence

Read the diff, affected owner and existing tests. Work from the repository root; compare `go version` with `go.mod`. Source builds use `CGO_ENABLED=0`; race checks use `CGO_ENABLED=1` and a compatible C toolchain. Obtain approval before installing missing tools or adding dependencies.

For a behavior change, observe a relevant behavioral Red before implementation, then Green and the affected regression. An import or toolchain failure does not supply Red. Use an ignored local probe when the scenario is outside the permanent checklist. Pure restructuring uses before/after characterization. Obtain an explicit exception when an applicable Red requirement cannot be met. Keep expected output tied to the contract and investigate unexplained changes.

## Decide whether test code must change

Reuse existing checks first. Maintained code/behavior tests cover only the following checklist, with one representative case and necessary failure cases per contract. Keep each assertion at its owning boundary; CLI/MCP output cases exercise their presentation contract rather than duplicating SDK mapping tables.

| Contract | Owning boundary |
| --- | --- |
| Credential privacy: account/Cookie values stay private under `%v`, `%+v`, `%#v` and slog; request logs omit query, token, Cookie and signed URLs; MCP tool output omits signed URLs and Cookie | Account/Cookie types, request logging, MCP output |
| Account storage: revision conflicts, transaction rollback, opening a v1.2.x database, private permissions; atomic replacement and file locking | `internal/storage/*` |
| Rotated token is persisted before the client is returned | `internal/pixivaccount` or `internal/session` |
| Malformed upstream responses return errors; cancellation propagates | `sdk/pixiv`, `sdk/fanbox` |
| Public snapshots: one full MCP `tools/list` golden; one CLI command's text/JSON/NDJSON; usage exit code; stdio stdout contains only JSON-RPC; HTTP requires bearer | `internal/mcpserver`, `internal/cli` |
| Environment > file > default, including explicit empty values | `internal/config/settings` |
| Failed update verification preserves the installed binary | `internal/selfupdate` |

### Enforce the budget

| Item | Maximum |
| --- | --- |
| Non-exempt test files across the repository | 25 |
| Lines per test file, including imports, blank lines and helpers | 200 |
| Lines per test function | 40 |
| Cases in a table | 8 |
| `testdata` | Old-version database samples and a few goldens |

Count files for every platform/build tag, not just files selected on the current host. Prefer `httptest`, temporary directories and actual stores. Do not create test helper packages, fixture constructors, mock hierarchies or production exports for tests. Use the [test file layout](../../../docs/en/maintainers/development.md#test-file-layout) and its documented same-package exceptions.

Before retaining a change, identify the checklist row, the missing failure and the smallest proof in the PR/handoff. Keep out-of-checklist experiments under ignored `tmp/test/_probe/`; run that package explicitly so ordinary `go test ./...` discovery excludes it. Keep probe output there too and use synthetic credentials. No permanent live-test files or opt-in live harnesses.

### Preserve CI-selected tests

Keep workflows and their steps intact during test deletion/rewrites. Preserve these tests verbatim, together with required support and fixtures; they are exempt from the file/function/case caps and `testdata` restriction:

- Entire packages: `scripts/citest`, `scripts/release`, `internal/cli/auth/loginhelper`, `internal/media/ugoira/...`.
- Named tests: `cmd/pixiv/TestPixivBinaryPackagedSmoke`; `internal/storage/database/TestOpenOnWindowsAcceptsForwardSlashFileURI`; `internal/cli/TestAuthURLCallback`, `TestAuthURLHandlerInstall`, `TestNormalCLIInvocationEnsuresPersistentHandlerWithoutBlockingCommand`.

Recheck this list against actual workflow references before merging. Broad `go test ./...` steps do not exempt the whole repository. For a mixed file, retain the selected function, comments and necessary declarations exactly; unrelated tests still follow the checklist. If a workflow selector matches no definition, report the coverage gap rather than claiming that named test passed.

### Remove low-value tests

Within authorized test cleanup, remove code/behavior tests outside the checklist and their orphan helpers, fixtures and test-only production seams. Retain CI-selected tests as above. Delete duplicate assertions, getter/forwarding tests, test-harness tests and completed-migration characterization; preserve v1.2.x database-opening evidence. Temporary investigation belongs under `tmp/test/`.

For a suite rewrite, include each counted file, its line count and checklist row, the preserved CI inventory, and total test-line change in the PR. Delete and replace in the same PR so main always retains the complete suite.

## Select checks

During implementation, run the affected package and direct callers, narrowing with `-run` when useful. Use `CGO_ENABLED=0 go build ./...` for source compilation. Reuse passing evidence until a relevant change or failure invalidates it.

| Change | Checks |
| --- | --- |
| Documentation/instructions | Links, metadata, commands, `git diff --check`; affected existing tooling |
| Local Go behavior | Focused test and scoped vet; build the executable when affected |
| Shared/public contract or release candidate | Once on the final candidate: `go test ./... -count=1`, `go vet ./...`, `sh scripts/build.sh` |
| Concurrency/persistence/shared runtime | Also `CGO_ENABLED=1 go test -race ./... -count=1`; native gates for changed platform behavior |
| PR metadata | `go run ./scripts/pr validate --body-file PATH --cli pixiv` |
| Platform/release | Existing `scripts/release`/`scripts/citest` tests, `sh scripts/ci/test-package-release.sh`, `sh scripts/build/matrix.sh --capability smoke`; follow [native](../pixiv-cli-native/SKILL.md) |

Use `gofmt` and the installed local pre-commit hook on affected Go files. Cross-compilation supplies compilation evidence; native runners supply execution evidence.

CI scope comes from `.github/ci-change-scope.gitignore` and `scripts/ci/classify-change-scope.sh`. `ci.yml` owns Quality; trusted `pr-metadata.yml` dispatches base-ref Platform/Container workers. Inspect Check Runs on the exact PR head, including actual pass or skip conclusions; keep classification and gates unchanged. Ordinary branch/main pushes do not run CI; matching tags retain their Quality/release path.

## Manual live and native verification

The ordinary suite is offline. Follow [manual live verification](../../../docs/en/maintainers/development.md#manual-live-verification) after explicit authorization for account, targets, network requests, downloads, browser-secret access and any mutation. Use normal authenticated CLI/SDK entry points and private credential input; record only safe IDs, counts, byte totals and error categories. Image upload requires its own authorization. For writes, agree the initial state, read-back and cleanup before executing.

Keep manual observations separate from offline test results. Preserve upstream errors, including 429 diagnostics; add no guessed waits, retries or account rotation. Native/browser acceptance needs the actual authorized OS/secret backend. Use existing native CI for platform claims, retaining the documented Windows ARM64 race-detector limitation and exact diagnostic handling.

## Handle failures and finish

Record the first causal failure, command, owner and environment. Continue safely independent checks; retry only after a relevant change or a stated infrastructure hypothesis. Report the tested revision, actual pass/fail/skip, unrun required checks and their effect on acceptance. Completion needs the requested behavior and every mandatory gate.
