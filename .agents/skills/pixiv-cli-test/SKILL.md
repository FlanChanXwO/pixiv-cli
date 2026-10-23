---
name: pixiv-cli-test
description: Select and run pixiv-cli regression, TDD, document, SDK, CLI/MCP, and build checks. Use when changing behavior, reproducing a bug, deciding which tests may enter main, or reporting readiness. Separates offline fixtures, native evidence, and explicitly authorized live API tests.
---

# Test pixiv-cli

## Determine the evidence

Read the diff and the affected owner/tests before choosing commands. Work from the repository root unless a command explicitly needs another directory. Check `go version` against `go.mod`; source builds use `CGO_ENABLED=0`; race-detector checks separately enable cgo and need a compatible C toolchain. Do not install missing tools or fetch dependencies without authorization. Report environment failures separately from regressions.

For source behavior, select the smallest existing test that exposes the missing behavior, or extend it only where coverage is missing, and observe its expected failure before implementation. A test that fails to compile or cannot reach its assertion is not a behavioral Red. Implement one slice, run Green, then refactor and rerun. For structural-only work, reuse before/after characterization evidence; obtain an explicit exception if an applicable Red requirement cannot be met. Never modify expected output solely to make an unexplained failure disappear.

Use existing Go testing and HTTP/FS fixtures. Test public behavior through real boundaries rather than reproducing the implementation in mocks. Prefer deterministic inputs and temporary stores; exercise cancellation, error propagation, and ordering when affected. Follow the canonical [test file layout](../../../docs/en/maintainers/development.md#test-file-layout), including owner-matched names and documented same-package exceptions; do not create task-numbered test files or public exports only for tests.

## Decide whether test code must change

Apply Ponytail **full**: verify the whole change with the least maintained test code. The default is **no new permanent test code**. Run existing checks first; a missing assertion is a reason to investigate, not automatic permission to grow the suite. This admission rule applies to new tests, extra table rows/assertions, fixtures, and test helpers alike.

### Admit only durable core regressions

A test change may enter main only when **all** of these hold:

- It protects a long-lived core contract: credential/security boundaries, data integrity and persistence, public CLI/MCP/SDK contracts, or a critical execution path whose failure loses results, breaks cancellation, or reports false success. Name the concrete failure and user impact; merely touching a core package does not qualify.
- Existing assertions do not catch that failure. Prefer the smallest extension at the stable owning boundary; add a new test only when reuse cannot express it. Do not repeat the same assertion across CLI/MCP/SDK layers unless their distinct contracts require it.
- The evidence will remain useful after the current task, refactor, or upstream investigation ends. Use deterministic offline inputs for ordinary regressions; retain explicitly authorized live/native gates only for contracts that need that environment.
- The check is the smallest maintainable proof, with direct assertions and minimal fixtures. Reuse Go testing and existing fixtures; avoid new frameworks, mock hierarchies, fixture builders, or production exports introduced only for tests.

Before retaining test code, record the core contract, the uncovered failure, and why existing checks are insufficient in the current PR/handoff. If that justification is missing, keep the experiment temporary instead. Do not add tests for getters, forwarding wrappers, field projections, pure moves, prose, or internal file shape merely to satisfy TDD, a checklist, or a coverage target. Machine-consumed formats and security checks qualify only when they meet the same core-contract criteria.

A test double never justifies an exported setter, a package-level mutable variable swapped by tests, or an interface without a second production implementation. Use real boundaries such as `httptest` servers and temporary directories, or a same-package test that sets unexported fields; register a new same-package exception in the test layout as required.

### Remove low-value tests

When a change touches tests or a task asks to slim them, delete or merge a test that:

- asserts file text (YAML, Dockerfile, Markdown, Go source) instead of executing behavior, unless the text itself is a security contract no execution can check, such as full-SHA action pins;
- repeats a contract another test already asserts; keep the one at the most public boundary (CLI/MCP output, SDK method, configuration entry point);
- tests test-harness code, such as live-test environment parsing, fixture builders, or target finders;
- guards against reintroducing something already removed;
- preserves characterization of a finished migration; file names containing `red`, `baseline`, or `parity` are a hint;
- checks getters, forwarding, field projection, or a constant equal to itself.

Keep tests that protect secret redaction, data integrity and persistence, public CLI/MCP/SDK output contracts, cancellation and error propagation, and release artifact correctness. Prefer a check that executes real code over a string assertion. When moving tests to their owner file, do not carry duplicated assertions along. In the PR, list each deleted or merged test, the criterion applied, and the retained test that still covers the contract; do not change a retained assertion to fit a refactor.

### Keep temporary verification out of main

Put one-off reproductions, exploratory probes, benchmarks, migration characterization, and task-specific fixtures **only under a `tmp/test/` directory**, such as repository-root `tmp/test/` or an owner's `tmp/test/`. They are ignored local evidence, not deliverables. Keep their generated output there too; never force-add them or move them into tracked sources merely to satisfy a gate.

For Go probes, use `tmp/test/_probe/` and run that package explicitly, for example `go test ./tmp/test/_probe -run '^TestProbe$' -count=1`. The underscore directory keeps it out of ordinary `go test ./...` discovery. Exercise existing public/owner-accessible boundaries; do not export private symbols to make a temporary probe compile.

Temporary Red/Green evidence can verify a change without becoming a permanent regression. Promote only the minimal case that independently meets every admission criterion above, then follow the canonical test-file layout. Record commands and results in the existing PR/handoff, not a new test-plan/report artifact. Keep secrets out of temporary files as well.

Keep established security and release gates. Remove or consolidate existing tests only within the authorized scope after identifying equivalent retained protection and running it. Fewer lines alone do not justify deleting evidence. Finish when changed behavior and credible failures are verified; mandatory gates still apply, without a per-function, per-file, test-count, or coverage-percentage quota.

## Select checks

Tests are one form of evidence; compilation, output comparison, and reading the diff are often more direct. While editing, run only what the change affects and never the whole suite after each edit:

- `go build ./...` for any Go change; a pure move, rename, or import rewrite needs only compilation.
- The touched package and its direct callers, narrowed with `-run` where possible, for example `go test ./path/to/owner -run '^TestName$' -count=1`, replacing both placeholders with inspected values.
- Only the affected command output or MCP `tools/list` when an external output may change.

After a check passes, do not rerun or widen it unless something changed or failed. Run the repository-wide checks once, before handoff or opening/updating a PR, according to actual impact:

| Change | Relevant checks |
| --- | --- |
| Documentation, instructions, product skills | Inspect links, skill metadata, examples, and `git diff --check`; run affected existing script/tool contracts |
| Local Go implementation | Focused test, affected integration packages, and scoped `go vet`; run `sh scripts/build.sh` when the change affects the build or executable |
| Shared contract, public SDK, core behavior, or release candidate | Once before handoff: `go test ./... -count=1`, `go vet ./...`, and `sh scripts/build.sh` |
| Concurrency, account lifecycle, persistence, shared runtime | Add `go test -race ./... -count=1` once before handoff; use native platform checks where platform code is involved |
| PR metadata and verification tooling | `go test ./tools/prmeta ./tools/verification -count=1`; inspect the changed workflow's existing tests |
| Platform/release contracts | `go test ./tools/release ./tools/platformmatrix -count=1`, affected `scripts/...` tests, and the relevant build/package/Homebrew fixture scripts |
| Encoding, platform builds, ABI or native evidence | Follow [pixiv-cli-native](../pixiv-cli-native/SKILL.md) for the affected execution/artifact boundary |

Use `gofmt -l` on affected Go files and the existing `.pre-commit-config.yaml`. Run installed pre-commit when applicable; do not introduce a new linter or download hook environments silently. `sh -n` checks shell syntax, not shell behavior. Cross-compilation checks compilation, not execution on another operating system.

CI scope is determined by `.github/ci-change-scope.gitignore` through `scripts/classify-change-scope.sh`, not the file extension or this table. `ci.yml` owns Quality; trusted `pr-metadata.yml` classifies PRs and dispatches base-ref Platform/Container workers. Required smoke gates are Check Runs on the exact PR head and use a real skipped conclusion when their scope is not required. Honor those gates without changing classification to save a run. Ordinary branch/main pushes do not run CI; matching tags retain the current Quality/release path.

## Live and native boundaries

The ordinary suite must remain offline-stable. Real SDK E2E is opt-in and can rotate/persist credentials. Read the live-test section of `docs/en/maintainers/development.md` and obtain account/target authorization first. Select an authorized secondary Pixiv account with `PIXIV_E2E_READ_USER_ID`; never read a real credential store into the transcript. FANBOX sessions come from the agreed Keychain environment, not command arguments or logs.

`TestRealPixivSDKRead` and `TestRealFanboxSDKRead` prove their specific live paths only when actually run. The FANBOX full read requires a first-party attachment; `TestRealFanboxSDKPostInfo` is supplemental partial evidence, not a replacement. Default skips are not live evidence. Reverse-search E2E additionally needs explicit image-upload authorization. An upstream 429 is not permission to invent retries or rotate accounts.

Native/browser fixture checks do not establish real keychain/DPAPI access, and one host does not establish native linking on every shipped platform. Select the existing native/browser evidence workflows for those claims. Preserve the documented Windows ARM64 race-detector limitation and its exact diagnostic check; do not call unsupported execution a successful race run.

## Handle failures and finish

Identify the first causal failure, command, owner, and relevant environment. For a fail-fast gate, run relevant safely independent downstream checks to expose other blockers. Do not repeatedly run unchanged deterministic failures; retry only after a relevant change or to test a stated infrastructure/flakiness hypothesis. Fix only in-scope regressions.

Report actual commands, pass/fail/skip, unrun checks and why, tested commit, and remaining risk. Completion requires the requested behavior and mandatory evidence, not a selected subset of green checks.
