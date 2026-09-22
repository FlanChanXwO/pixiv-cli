---
name: pixiv-cli-develop
description: Implement, diagnose, refactor, and design pixiv-cli Go changes using its actual CLI, SDK, MCP, account, and storage boundaries. Use before source changes or engineering design; choose the separate native or MCP workflow when that boundary is involved. No personal skills are required.
---

# Develop pixiv-cli

## Establish the change

Read root `AGENTS.md`, the relevant architecture section, the exact owner code, and its tests. State the observable result and what is outside scope. For a bug, reproduce it and trace input through the owner before proposing a fix. For ambiguous or cross-boundary work, settle compatibility, data ownership, and acceptance criteria first; a small explicit fix needs no design ceremony.

Inspect `git status --short`, branch, and base SHA. Reuse a suitable isolated worktree or create one at an unused agreed location with `git worktree add -b BRANCH PATH BASE`; substitute verified values, preserve other worktrees, and never reset someone else's changes. Set the working directory on every tool invocation.

Use available LSP definitions, references, callers, and impact analysis before changing symbols. If unavailable, report it, inspect imports/callers with targeted `rg`, and verify the affected packages with the compiler/tests. Read foundational documents and exact edited code yourself; delegated summaries are evidence, not design authority.

## Go design and language rules

- Use the `go.mod` toolchain and `gofmt`. Preserve Go initialisms, descriptive package names, and consistent receiver names. Document exported contracts in English, including cancellation, ownership, optional values, and side effects. Explain non-obvious intent in comments; do not rewrite unrelated comments for style.
- Prefer concrete types and existing owners. Define small interfaces at a real consumer seam, not one interface per implementation; avoid public abstractions introduced only for tests. Keep constructors explicit and side-effect boundaries visible.
- Keep Cobra, terminal input, JSON presentation, and exit handling in CLI owners. Keep MCP schema/presentation in its tool owner. Reuse shared records and traversal rather than copying pagination, filtering, or output rules into each adapter.
- Pass `context.Context` as the first parameter where cancellation applies and propagate it through requests and loops. The caller owns cancellation policy. Keep goroutine lifetime and resource closure explicit; avoid detached workers, shared mutable globals, and unbounded background activity.
- Return contextual errors while preserving `errors.Is`/`errors.As` classification. Do not log and return the same failure at every layer. Keep the SDK silent by default and redact at trust boundaries; never wrap a secret-bearing upstream response wholesale.
- Distinguish absent, zero, empty, and invalid values when the wire contract does. Preserve unknown SDK fields where the record contract requires them. Do not fill unavailable upstream fields with invented defaults or advertise schema-only capabilities.
- Preserve atomic persistence and observable committed/unknown outcomes. Test credential revision races, cleanup, cancellation, and file permissions when those mechanisms change; do not claim rollback after a committed replacement.
- Optimize only against an observed cost or requested requirement. Prefer removal/reuse, standard-library or native features, existing dependencies, then a direct local implementation. Add a shared abstraction only when stable current uses make it simpler.

Use the official [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) for additional language guidance; the repository's concrete API, storage, and ownership contracts above still apply.

## Execute with evidence

Read [the test workflow](../pixiv-cli-test/SKILL.md). For new behavior or a fix, run a focused test and observe the expected behavioral failure before implementation. Do not accept an import/toolchain error as Red. Make the smallest implementation pass; then refactor while retaining the regression. For behavior-preserving restructuring, bracket the move with passing characterization tests, and use a failing test for any actual behavior correction. If the required Red cannot be established, obtain an explicit exception before editing source.

Read [the MCP workflow](../pixiv-cli-mcp-tool/SKILL.md) for tool changes or [the native workflow](../pixiv-cli-native/SKILL.md) for Rust/cgo changes; do not force either on a Go-only task.

Before adding a dependency, fallback, limit, cache, worker, or configuration switch, identify the present requirement and existing alternative. Follow the root approval policy. For a justified new constraint, record its trigger and effect, preserve valid success paths, and test both the protected failure and representative valid input.

Update only the affected public contracts through [the docs workflow](../pixiv-cli-docs/SKILL.md). Reconcile new requirements with the plan and tests rather than continuing a stale plan. Split files when ownership/navigation improves, not to meet an arbitrary size threshold.

## Finish

Run affected-file semantic diagnostics when available, the relevant regression and quality checks, and [the review workflow](../pixiv-cli-review/SKILL.md). Account for every acceptance criterion and report commands, outcomes, unverified platforms/live behavior, and risks. Stop when the requested behavior and required evidence are complete; release actions are a separate task.
