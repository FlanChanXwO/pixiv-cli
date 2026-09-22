---
name: pixiv-cli-review
description: Review pixiv-cli working changes, commits, or pull requests for requested behavior, Go design, SDK/CLI/MCP contracts, security, native boundaries, and verification gaps. Use for independent review or pre-delivery self-review; review-only unless edits are authorized.
---

# Review pixiv-cli

## Fix the review boundary

Inspect `git status --short`, `git diff --stat`, and the actual diff. For a PR/commit range, identify base and head SHAs and review that exact range; an empty working diff does not mean the PR has no changes. Read the originating requirement and relevant `AGENTS.md` routes. Do not guess the review range or treat another agent's summary as evidence.

## Review the change, not an imagined framework

- **Behavior:** trace changed inputs to outputs and compare with acceptance criteria. Check CLI text/JSON/NDJSON, record identity, logical pagination, optional fields, typed errors, and command-specific defaults. Verify local search filters are not falsely passed upstream.
- **Architecture/Go:** apply [the develop rules](../pixiv-cli-develop/SKILL.md). Check real ownership, interface necessity, context/goroutine lifetime, error classification, and public SDK compatibility. Prefer deleting speculative machinery; length alone is not a finding.
- **Security/state:** check secret redaction, SQL/query boundaries, credential revision transactions, atomic writes and commit outcomes, permissions, URL/redirect policy, and trusted-local versus remote input assumptions. Preserve explicitly supported private resource routes rather than applying a blanket network ban.
- **MCP:** verify schema validation, SDK routing, separate Pixiv/FANBOX runtimes, protocol-clean stdout and structured errors. Use [the MCP workflow](../pixiv-cli-mcp-tool/SKILL.md) for changes at that boundary.
- **Native/release:** use [native](../pixiv-cli-native/SKILL.md) or [CI](../pixiv-cli-ci/SKILL.md) only when touched. Review source/artifact identity, platform evidence, permissions, and immutable handoffs, not exact YAML formatting or arbitrary job counts.
- **Tests/docs:** confirm actual Red/Green evidence where required, relevant positive/failure paths, and locale/product-skill consistency. A fixture does not establish live API or real native/browser acceptance.

Verify suspected findings at exact code locations and with the smallest safe test where useful. Do not add dependencies, execute real account operations, or make external writes just to investigate. For broad changes, separate independent questions without duplicate reviews; final integration review remains the main agent's responsibility.

## Report

Lead with actionable findings: severity, `file:line`, triggering condition, impact, evidence, and a minimal correction. Separate demonstrated defects from questions and unverified risks. Do not request an abstraction or another test solely to satisfy a checklist.

If no blocking issues are found, state the reviewed scope, checks run, and remaining limitations. Self-review is not approval by another maintainer. Do not modify code, merge, or dismiss remote reviews without authorization.
