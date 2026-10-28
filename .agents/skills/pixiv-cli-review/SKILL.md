---
name: pixiv-cli-review
description: Review a pixiv-cli diff, commit, or pull request against its requested behavior, architecture, coding standards, security boundaries, and verification evidence. Use for independent review or pre-delivery self-review; review-only unless edits are authorized.
---

# Review pixiv-cli

## Set the review boundary

Inspect `git status --short`, the actual diff, and the intended base/head SHAs. For a PR or commit range, review that exact range; an empty working diff says nothing about a remote change. Read the originating requirement and root `AGENTS.md` routes. Do not infer scope from a summary.

Use [PR scope checks](../pixiv-cli-pr/SKILL.md#verify-scope) to keep unrelated files and local evidence out of the deliverable.

## Review against the contracts

Read the relevant [coding standards](../../../docs/en/maintainers/coding-standards.md), [architecture](../../../docs/en/maintainers/architecture.md), and [development](../../../docs/en/maintainers/development.md) sections before judging implementation details.

- Trace changed inputs through their owning layer to observable results and compare them with acceptance conditions.
- Check the affected CLI, SDK, MCP, account, persistence, security, and platform boundaries against their established contracts. Use the focused [MCP](../pixiv-cli-mcp-tool/SKILL.md) or [native](../pixiv-cli-native/SKILL.md) workflow when relevant.
- Apply the handbook's [naming](../../../docs/en/maintainers/coding-standards.md#naming-and-terminology), [declaration placement](../../../docs/en/maintainers/coding-standards.md#declaration-placement-and-file-names), [boundary validation](../../../docs/en/maintainers/coding-standards.md#boundary-validation-no-over-defensive-code), [program output](../../../docs/en/maintainers/coding-standards.md#program-output), [commenting](../../../docs/en/maintainers/coding-standards.md#package-documentation-and-comments), and [test submission](../../../docs/en/maintainers/coding-standards.md#test-submission-rules) guidance as applicable.
- Verify suspected issues at exact changed locations and use the smallest safe check that can establish them. Distinguish observed failures from unverified risks; avoid real account operations or external writes during review.

## Report

Lead with actionable findings: severity, `file:line`, trigger, impact, evidence, and a minimal correction. If no findings remain, state the reviewed scope, checks run, and material limitations. Self-review is not approval by another maintainer. Do not modify code, merge, or dismiss remote reviews without authorization.
