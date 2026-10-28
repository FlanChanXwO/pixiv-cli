---
name: pixiv-cli-develop
description: Implement, diagnose, refactor, and design Go changes in pixiv-cli. Use before source changes or engineering design; follow the coding handbook and the relevant architecture, test, MCP, native, or documentation workflow.
---

# Develop pixiv-cli

## When to use

Use for Go implementation, debugging, refactoring, and engineering design. For MCP tools, native/platform behavior, documentation, or review, follow the corresponding focused skill as well.

## Read before changing Go

Read root `AGENTS.md`, the relevant [architecture](../../../docs/en/maintainers/architecture.md) and [development](../../../docs/en/maintainers/development.md) guidance, plus the applicable sections of the [coding standards](../../../docs/en/maintainers/coding-standards.md):

- Design and ownership: [style baseline](../../../docs/en/maintainers/coding-standards.md#style-baseline), [naming](../../../docs/en/maintainers/coding-standards.md#naming-and-terminology), [declaration placement](../../../docs/en/maintainers/coding-standards.md#declaration-placement-and-file-names), and [named constants](../../../docs/en/maintainers/coding-standards.md#named-constants).
- Boundaries and written contracts: [boundary validation](../../../docs/en/maintainers/coding-standards.md#boundary-validation-no-over-defensive-code), [program output](../../../docs/en/maintainers/coding-standards.md#program-output), and [comments](../../../docs/en/maintainers/coding-standards.md#package-documentation-and-comments).
- Tests: [test submission rules](../../../docs/en/maintainers/coding-standards.md#test-submission-rules) and the test layout in the development guide.

## Workflow

1. Define the observable result, acceptance evidence, and out-of-scope behavior. Settle consequential unknowns before editing; a small explicit change needs no design ceremony. Read the exact owner and callers; reproduce a reported bug before selecting a fix.
2. Inspect the worktree, branch, and base. Preserve existing changes, use the independent worktree required by `AGENTS.md` for Git writes, set an explicit working directory, and quote command arguments safely.
3. Use available semantic navigation for definitions, references, and callers. If unavailable, say so, use targeted `rg`, and verify affected packages through existing checks.
4. Use the existing issue, PR, or conversation for decisions instead of creating process files automatically. For multi-step work, keep the client plan current; delegate only bounded work with a clear owner and integrate the evidence yourself.
5. Implement at the owning layer. Follow [the test workflow](../pixiv-cli-test/SKILL.md); use [MCP](../pixiv-cli-mcp-tool/SKILL.md) or [native](../pixiv-cli-native/SKILL.md) guidance when those boundaries change. Update affected public contracts through [the documentation workflow](../pixiv-cli-docs/SKILL.md).

## Before handoff

- Compare the result with each acceptance condition and the existing caller, data, and public contracts.
- Run the affected regression and quality checks; review the actual diff with [the review workflow](../pixiv-cli-review/SKILL.md).
- Report commands and outcomes, remaining risks, and any platform or live behavior not verified. Keep evidence claims tied to the tested revision.
