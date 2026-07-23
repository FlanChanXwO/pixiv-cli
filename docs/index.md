# pixiv-cli Documentation

`pixiv-cli` is an unofficial third-party Pixiv CLI, MCP stdio server, and public Go SDK. Choose a language for
user-facing interface documentation; maintainer documents have one canonical version and are not mechanically
duplicated for every locale.

## User documentation

| Interface | English | 简体中文 | 日本語 |
| --- | --- | --- | --- |
| Project overview | [README](../README.md) | [README](../README.zh-CN.md) | [README](../README.ja.md) |
| CLI reference | [English](en/cli-reference.md) | [简体中文](zh-CN/cli-reference.md) | [日本語](ja/cli-reference.md) |
| Go SDK | [English](en/sdk.md) | [简体中文](zh-CN/sdk.md) | Not translated; use English |
| MCP tools | [English](en/mcp-tools.md) | [简体中文](zh-CN/mcp-tools.md) | Not translated; use English |
| Contributing | [English](../CONTRIBUTING.md) | [简体中文](../CONTRIBUTING.zh-CN.md) | Not translated; use English |

Public interface documents are organized by BCP 47 locale directory. English is the canonical public contract;
translations must preserve behavior while using natural language for their audience.

## Maintainer documentation

- [Architecture](maintainers/architecture.md): package boundaries, runtime flow, Release assets, and trust model.
- [Development](maintainers/development.md): local environment, tests, builds, releases, and native evidence.
- [AI collaboration](maintainers/agents/index.md): repository agent rules, review checklist, and documentation policy.
- [Architecture decisions](maintainers/adr/): long-lived decisions and their tradeoffs.
- [Changelog](../changelog/README.md): user-visible changes.

Maintainer documents keep their existing canonical language and are currently primarily Simplified Chinese.
Translate one only when there is a demonstrated maintainer need; do not create empty locale copies.
