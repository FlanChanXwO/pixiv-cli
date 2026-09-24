# pixiv-cli documentation

English | [简体中文](index-zh-CN.md)

Looking for commands, the SDK, or MCP tools? Start with the user docs. Preparing to develop, review, or release? Go straight to the maintainer docs.

## Using pixiv-cli

| What you want | English | 简体中文 |
| --- | --- | --- |
| Project overview and installation | [README](../README.md) | [README](../README.zh-CN.md) |
| CLI commands | [CLI reference](en/cli-reference.md) | [CLI 参考](zh-CN/cli-reference.md) |
| Go SDK | [SDK](en/sdk.md) | [SDK](zh-CN/sdk.md) |
| MCP tools | [MCP tools](en/mcp-tools.md) | [MCP tools](zh-CN/mcp-tools.md) |
| Filing an Issue or PR | [Contributing](../CONTRIBUTING.md) | [贡献指南](../CONTRIBUTING.zh-CN.md) |
| Upgrading from v0 | [Migration guide](en/v1.0.0-migration.md) | [迁移指南](zh-CN/v1.0.0-migration.md) |

English docs define the canonical public interface. Simplified Chinese may rephrase for natural flow, but commands, parameters, schemas, output, and security rules stay consistent.

## Maintaining pixiv-cli

| Task | Where |
| --- | --- |
| Package boundaries and call edges | [Architecture](en/maintainers/architecture.md) |
| Environment, tests, or release | [Development](en/maintainers/development.md) |
| Released changes | [Changelog](../changelog/README.md) |

## For automation

Read [AGENTS.md](../AGENTS.md) at the repository root first for the complete task routing table. The checked-in skills require no personal global instructions or CCS; clients without skill discovery can read each `SKILL.md` directly. Common routes:

| Task | Skill |
| --- | --- |
| Preparing a PR | `.agents/skills/pixiv-cli-pr/` |
| Go implementation and design | `.agents/skills/pixiv-cli-develop/` |
| Focused testing and evidence | `.agents/skills/pixiv-cli-test/` |
| Code contracts, intent comments, and numbered phases | `.agents/skills/pixiv-cli-code-commenting/` |
| MCP tool changes | `.agents/skills/pixiv-cli-mcp-tool/` |
| Rust/cgo and native evidence | `.agents/skills/pixiv-cli-native/` |
| Diagnosing CI | `.agents/skills/pixiv-cli-ci/` |
| Reviewing changes | `.agents/skills/pixiv-cli-review/` |
| Maintaining docs | `.agents/skills/pixiv-cli-docs/` |
| Release notes | `.agents/skills/pixiv-cli-release-notes/` |

`CLAUDE.md` only references `AGENTS.md`. Clients without native discovery read the same contract and local routes; do not restore removed client-specific copies. Source-comment language is independent of the English skill files.
