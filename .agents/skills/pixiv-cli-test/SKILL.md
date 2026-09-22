---
name: pixiv-cli-test
description: Select and run pixiv-cli regression, TDD, document, SDK, CLI/MCP, and build checks. Use when changing behavior, reproducing a bug, choosing test scope, or reporting readiness. Separates offline fixtures, native evidence, and explicitly authorized live API tests.
---

# Test pixiv-cli

## Determine the evidence

Read the diff and the affected owner/tests before choosing commands. Work from the repository root unless a command explicitly needs another directory. Check `go version` against `go.mod`; source builds need cgo and a compatible C linker plus the committed native libraries. Do not install missing tools or fetch dependencies without authorization. Report environment failures separately from regressions.

For source behavior, write the smallest test that exercises the real owner and observe its expected failure before implementation. A test that fails to compile or cannot reach its assertion is not a behavioral Red. Implement one slice, run Green, then refactor and rerun. For structural-only work, retain before/after characterization evidence; obtain an explicit exception if a required Red cannot be defined. Never modify expected output solely to make an unexplained failure disappear.

Use existing Go testing and HTTP/FS fixtures. Test public behavior through real boundaries rather than reproducing the implementation in mocks. Prefer deterministic inputs and temporary stores; exercise cancellation, error propagation, and ordering when affected. Follow the canonical [test file layout](../../../docs/en/maintainers/development.md#test-file-layout), including owner-matched names and documented same-package exceptions; do not create task-numbered test files or public exports only for tests.

## Select checks

Start with the affected package and named test, for example `go test ./path/to/owner -run '^TestName$' -count=1`, replacing both placeholders with inspected values. Then expand according to actual impact:

| Change | Relevant checks |
| --- | --- |
| Documentation, instructions, product skills | `go test ./scripts/tests/documentation -count=1`; inspect links, skill metadata, examples, and `git diff --check` |
| Local Go implementation | Focused test, affected integration packages, and scoped `go vet`; run `sh scripts/build.sh` when the change affects the build or executable |
| Shared contract, public SDK, core behavior, or release candidate | `go test ./... -count=1`, `go vet ./...`, and `sh scripts/build.sh`, in addition to focused regression |
| Concurrency, account lifecycle, persistence, shared runtime | Add `go test -race ./... -count=1`; use native platform checks where platform code is involved |
| PR metadata and verification tooling | `go test ./tools/prmeta ./tools/verification -count=1`; inspect the changed workflow's existing tests |
| Platform/release contracts | `go test ./tools/release ./tools/platformmatrix -count=1`, affected `scripts/...` tests, and the relevant build/package/Homebrew fixture scripts |
| Rust/cgo/staticlib or ABI | Follow [pixiv-cli-native](../pixiv-cli-native/SKILL.md), not an unrelated Go-only substitute |

Use `gofmt -l` on affected Go files and the existing `.pre-commit-config.yaml`. Run installed pre-commit when applicable; do not introduce a new linter or download hook environments silently. `sh -n` checks shell syntax, not shell behavior. Cross-compilation checks compilation, not execution on another operating system.

CI scope is determined by `scripts/internal/changescope`, not the file extension or this table. In particular, do not assume root instruction files qualify as docs-only. Honor the actual required Quality/platform checks without changing their classifier to save a run.

## Live and native boundaries

The ordinary suite must remain offline-stable. Real SDK E2E is opt-in and can rotate/persist credentials. Read the live-test section of `docs/en/maintainers/development.md` and obtain account/target authorization first. Select an authorized secondary Pixiv account with `PIXIV_E2E_READ_USER_ID`; never read a real credential store into the transcript. FANBOX sessions come from the agreed Keychain environment, not command arguments or logs.

`TestRealPixivSDKRead` and `TestRealFanboxSDKRead` prove their specific live paths only when actually run. The FANBOX full read requires a first-party attachment; `TestRealFanboxSDKPostInfo` is supplemental partial evidence, not a replacement. Default skips are not live evidence. Reverse-search E2E additionally needs explicit image-upload authorization. An upstream 429 is not permission to invent retries or rotate accounts.

Native/browser fixture checks do not establish real keychain/DPAPI access, and one host does not establish native linking on every shipped platform. Select the existing native/browser evidence workflows for those claims. Preserve the documented Windows ARM64 race-detector limitation and its exact diagnostic check; do not call unsupported execution a successful race run.

## Handle failures and finish

Identify the first causal failure, command, owner, and relevant environment. For a fail-fast gate, run relevant safely independent downstream checks to expose other blockers. Do not repeatedly run unchanged deterministic failures; retry only after a relevant change or to test a stated infrastructure/flakiness hypothesis. Fix only in-scope regressions.

Report actual commands, pass/fail/skip, unrun checks and why, tested commit, and remaining risk. Completion requires the requested behavior and mandatory evidence, not a selected subset of green checks.
