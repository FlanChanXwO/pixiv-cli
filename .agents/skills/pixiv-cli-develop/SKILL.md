---
name: pixiv-cli-develop
description: Implement, diagnose, refactor, and design pixiv-cli Go changes using its actual CLI, SDK, MCP, account, and storage boundaries. Use before source changes or engineering design; choose the separate native or MCP workflow when that boundary is involved. No personal skills are required.
---

# Develop pixiv-cli

## Establish the change

Read root `AGENTS.md`, the relevant architecture section, the exact owner code, and its tests. State the observable result and what is outside scope. For a bug, reproduce it and trace input through the owner before proposing a fix. For ambiguous or cross-boundary work, settle data ownership, user-data compatibility, and acceptance criteria first; a small explicit fix needs no design ceremony.

Inspect `git status --short`, branch, and base SHA. Reuse a suitable isolated worktree or create one at an unused agreed location with `git worktree add -b BRANCH PATH BASE`; substitute verified values, preserve other worktrees, and never reset someone else's changes. Set the working directory on every tool invocation.

Use available LSP definitions, references, callers, and impact analysis before changing symbols. If unavailable, report it, inspect imports/callers with targeted `rg`, and verify the affected packages with the compiler/tests. Read foundational documents and exact edited code yourself; delegated summaries are evidence, not design authority.

## Go design and language rules

- Use the `go.mod` toolchain and `gofmt`. Preserve Go initialisms, descriptive package names, and consistent receiver names. Document affected exported contracts, including cancellation, ownership, optional values, and side effects. Apply [pixiv-cli-code-commenting](../pixiv-cli-code-commenting/SKILL.md) to comments and numbered stages; English and Chinese prose are both acceptable, with valid Go doc syntax.
- Prefer concrete types and existing owners. Define small interfaces at a real consumer seam, not one interface per implementation; avoid public abstractions introduced only for tests. Keep constructors explicit and side-effect boundaries visible.
- Keep Cobra, terminal input, JSON presentation, and exit handling in CLI owners. Keep MCP schema/presentation in its tool owner. Reuse shared records and traversal rather than copying pagination, filtering, or output rules into each adapter.
- Pass `context.Context` as the first parameter where cancellation applies and propagate it through requests and loops. The caller owns cancellation policy. Keep goroutine lifetime and resource closure explicit; avoid detached workers, shared mutable globals, and unbounded background activity.
- Return contextual errors while preserving `errors.Is`/`errors.As` classification. Do not log and return the same failure at every layer. Keep the SDK silent by default and redact at trust boundaries; never wrap a secret-bearing upstream response wholesale. Business objects hold no logger field: transport/protocol owners record requests and their own retry or recovery decisions, the execution boundary records the final outcome, and request-scoped data travels in `context.Context`.
- Do not keep internal compatibility. When a change supersedes an entry point, alias, source-compatible wrapper, removed-setting tombstone, or a branch kept for an older version of this project's own components, remove the old path in the same change; do not add `Deprecated` transitions. Public SDK breaking changes are listed in the changelog only. Retain only upstream format variants and the logic needed to open users' existing databases and configuration files.
- Distinguish absent, zero, empty, and invalid values when the wire contract does. Preserve unknown SDK fields where the record contract requires them. Do not fill unavailable upstream fields with invented defaults or advertise schema-only capabilities.
- Preserve atomic persistence and observable committed/unknown outcomes. Test credential revision races, cleanup, cancellation, and file permissions when those mechanisms change; do not claim rollback after a committed replacement.
- Optimize only against an observed cost or requested requirement. Prefer removal/reuse, standard-library or native features, existing dependencies, then a direct local implementation. Add a shared abstraction only when stable current uses make it simpler.

Use the official [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) for additional language guidance; the repository's concrete API, storage, and ownership contracts above still apply.

## Naming and placement

- Name a package, file, type, function, or field for the responsibility it owns. Avoid names that fit anything: `Deps`, `Dependencies`, `Data`, `Info`, `Facade`, `Manager`, `Helper`, `Util`, `Common`, `Base`, `Impl`, `Wrapper`, and files such as `support.go`, `helpers.go`, `util.go`, `common.go`, or `types.go`. Do not repeat context the package name already provides (`session.Pixiv`, not `session.PixivSession`), and use one name for one capability across owners. Rename misleading internal names rather than working around them.
- Keep each directory name equal to its package name. Avoid package names that collide with the public SDK packages, commonly used standard-library packages (`runtime`, `log`, `context`), or another package imported by the same caller; a required import alias signals a naming problem.
- Start a package at `name/name.go` (its entry type, constructor, or command) or `doc.go`, and name other files by topic. Do not split one type per file or collect unrelated declarations in a topicless file. Declarations shared by several files of a package belong in a named declaration file such as `models.go`, `wire.go`, or `errors.go`; a declaration serving one behavior stays beside the function that uses it; methods stay with their type.
- Constructors receive only the dependencies they use. Do not build dependency bags that the root fills and passes everywhere, and do not inject function fields (`PromptSecretFn`) behind same-named forwarding methods. A constructor that needs a struct of many dependencies usually owns too much; split the responsibility first.
- Give identifiers with business meaning named constants: configuration keys, API paths and parameters, tool names, column names, and output keys. Give a fixed value set a named type with constants. Literals remain acceptable for struct tags, log/error text, SQL bodies, and test inputs.
- Place new code by asking in order: does it belong to one command, MCP tool, or SDK domain file; is it shared only by one product's commands, shared by every command, or upstream protocol detail; otherwise create a package named for its responsibility. If the containing package, file, or type name no longer summarizes its contents after the addition, the code is in the wrong place. Never create catch-all `common`, `shared`, `util`, or `misc` locations, and do not add unrelated code to an existing one.

## Readability and the smallest design

- Keep a coherent operation understandable in its owner. Extract a helper or package when it names a real responsibility, hides relevant complexity, or removes stable semantic duplication, not merely to shorten a function. Count the reader's cross-file jumps and parameters, not only lines removed.
- A useful interface makes its consumer simpler while its implementation owns the difficult details. Reject pass-through layers, option bags, and general frameworks whose callers still manage those details. Use SOLID as a diagnostic for actual coupling, not a requirement for one interface per type or an extension point per branch.
- Prefer explicit data flow, descriptive names, and a visible normal path over compressed expressions or clever reuse. Keep related validation and transformations near their data; remove nesting when it improves clarity without hiding distinct failures.
- Tolerate small duplication when the cases have different reasons to change. Share code only when their semantics are stable; avoid boolean modes and configuration added merely to combine unrelated cases.
- Do not build production abstractions to support oversized mocks. Select a real test boundary and the smallest fixture before changing design for test convenience. Preserve existing caller/security contracts while simplifying.
- For every structure, ask which caller would write more code without it and which concrete failure it prevents; if neither has an answer and it adds a hop for the reader, remove it. Treat these as defects in code you add or touch: exported symbols or `SetXxx` seams used only by tests; package-level functions that forward to a default instance and `At`/`WithX` entry variants; the same interface or mapping copied into several packages; parallel type layers converted field by field; conversions between structurally identical types; one concept split into tiny packages or files that must be read together; interfaces that only mirror one concrete type in the same layer; home-grown substitutes for the standard library or an existing dependency; code with no production caller; and comments citing documents or section numbers that are not in the repository. A consumer-owned narrow port that cuts a storage, network, or operating-system dependency is a legitimate interface even with one implementation.

## Execute with evidence

Read [the test workflow](../pixiv-cli-test/SKILL.md), including its coverage-gap decision before adding tests. For new behavior or a fix, reuse or extend a focused test and observe the expected behavioral failure before implementation. Do not accept an import/toolchain error as Red. Make the smallest implementation pass; then refactor while retaining the regression. For behavior-preserving restructuring, reuse passing characterization before and after, and use a failing test for any actual behavior correction. If an applicable Red requirement cannot be established, obtain an explicit exception before editing source.

Read [the MCP workflow](../pixiv-cli-mcp-tool/SKILL.md) for tool changes or [the native workflow](../pixiv-cli-native/SKILL.md) for encoder, platform-build, or native platform smoke changes.

Before adding a dependency, fallback, limit, cache, worker, or configuration switch, identify the present requirement and existing alternative. Follow the root approval policy. For a justified new constraint, record its trigger and effect, preserve valid success paths, and test both the protected failure and representative valid input.

Update only the affected public contracts through [the docs workflow](../pixiv-cli-docs/SKILL.md). Reconcile new requirements with the plan and tests rather than continuing a stale plan. Split files when ownership/navigation improves, not to meet an arbitrary size threshold.

## Finish

Run affected-file semantic diagnostics when available, the relevant regression and quality checks, and [the review workflow](../pixiv-cli-review/SKILL.md). Account for every acceptance criterion and report commands, outcomes, unverified platforms/live behavior, and risks. Stop when the requested behavior and required evidence are complete; release actions are a separate task.
