---
name: pixiv-cli-docs
description: Edit pixiv-cli documentation, AGENTS.md, client bridges, repository maintenance skills, or the distributed product skill. Use when commands, APIs, configuration, architecture, testing, or contributor workflows change and their documentation must stay accurate and portable.
---

# Maintain pixiv-cli Documentation

## Choose the owner

Read the changed implementation or authoritative configuration first. Use the existing document for its audience rather than creating a second specification.

| Content | Owner |
| --- | --- |
| Install, quick start, public capability overview | `README.md`, `README.zh-CN.md` |
| CLI, SDK, MCP, configuration and errors | Corresponding pages under `docs/en/` and `docs/zh-CN/` |
| Architecture, test layout, native and release details | Existing locale maintainer pages |
| Contribution entry and PR expectations | `CONTRIBUTING.md`, `CONTRIBUTING.zh-CN.md`, actual PR template |
| Always-loaded agent constraints and routing | Root `AGENTS.md`; `CLAUDE.md`/Copilot are thin bridges |
| Task-specific maintenance instructions | `.agents/skills/pixiv-cli-*/` |
| Safe use of the installed product | `skills/pixiv-cli/` |
| Versioned release notes | `changelog/vX.Y.Z/`, only during authorized release preparation |

Keep agent contracts, both kinds of skill, their references, and metadata English. Public locale pages retain their language; update affected English and Simplified Chinese contracts together without requiring literal translation. Source comments are not a reason to translate the whole codebase.

## Write only supported claims

Preserve current command spelling, output cardinality, account selection, pagination, error and privacy semantics. Check code, tests, and built `--help` rather than inferring behavior from another client. Distinguish local filtering from upstream support, partial results from completeness, and fixture coverage from executed native/live evidence.

Keep README examples short. Put long tables or conditional workflows in the existing reference, with a precise trigger/link. Explain current behavior instead of listing historical non-features. Do not insert a fixed command/file count, pin duplicate toolchain versions, or create design/plan/report documents merely because a skill has stages.

Maintenance skills are repository-scoped and may link to repository files. Product bundles must work outside a checkout: bundle essential references and use official public URLs for optional repository documentation. Keep product installation/state changes explicitly authorized and verify syntax with the installed binary; never import a maintainer's server path, proxy, account, or private tool assumptions.

Use lowercase `pixiv-cli-` names for maintenance skill directories and frontmatter; retain the product's existing name and publisher metadata. Descriptions identify distinct task triggers, not all coding or all image requests. Keep each skill focused and load extra references only on the relevant branch. Avoid creating another skill for a single readily discoverable command.

Do not bump product versions, edit published release notes, or publish a skill during ordinary documentation maintenance. Follow [release preparation](../pixiv-cli-release-notes/SKILL.md) when a version is explicitly authorized.

## Verify

Check local links, referenced files/symbols/commands, English skill text, frontmatter names, and `agents/openai.yaml` routes. Remove stale references to deleted owners. Run `git diff --check` and relevant existing tool/workflow tests when their documented behavior changes; no separate documentation test framework is required. Use the actual change classifier to determine CI requirements, not an assumed Markdown exemption.

Review as a contributor with no personal skills: every required workflow must be reachable from `AGENTS.md` or an explicit local link. Report document verification separately from application or native execution.
