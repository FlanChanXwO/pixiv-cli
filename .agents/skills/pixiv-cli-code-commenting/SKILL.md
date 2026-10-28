---
name: pixiv-cli-code-commenting
description: Write or review pixiv-cli package documentation and comments when contracts, invariants, workflow stages, or stale prose need attention. Use the coding handbook for the rules and preserve consumed directives.
---

# pixiv-cli Code Commenting

Use this skill when a code change needs package documentation, API comments, field or concurrency explanations, meaningful workflow navigation, or a focused comment review.

## Read before editing

Read [Package documentation and comments](../../../docs/en/maintainers/coding-standards.md#package-documentation-and-comments), the affected owner, and the relevant callers. Use [the test workflow](../pixiv-cli-test/SKILL.md) when comment-like content is consumed by a test or tool; use [the development workflow](../pixiv-cli-develop/SKILL.md) for authorized structural changes.

## Editing workflow

1. Identify the missing or inaccurate contract, intent, invariant, or navigation cue. Check its source in code and callers before documenting it.
2. Choose the nearest useful location, make only the authorized comment change, and report any out-of-scope structural or behavior issue separately.
3. Re-read the final comment against the code. Check its claims, language, stage order, symbol references, and protected directives.
4. If a directive, doctest, generated description, or other consumed comment changed, inspect its consumer and run the relevant existing check. Ordinary prose edits need only the applicable documentation/diff checks.

## Before handoff

- Each retained comment helps a caller understand a contract, reason, invariant, or workflow.
- No claim exceeds verified behavior; protected directives and generated provenance remain intact.
- Report relevant checks and any unresolved factual question. Self-explanatory code may remain uncommented.
