# pixiv-cli Agent Contract

## Task routes

Use the relevant checked-in maintenance skill for the workflow and its linked handbook or reference for the rules. If skill discovery is unavailable, open the linked `SKILL.md` directly.

| Task | Read first | Before completion, check |
| --- | --- | --- |
| Implement, debug, refactor, or design Go code | [pixiv-cli-develop](.agents/skills/pixiv-cli-develop/SKILL.md), [coding standards](docs/en/maintainers/coding-standards.md), [architecture](docs/en/maintainers/architecture.md), [development](docs/en/maintainers/development.md) | Confirm the owning package and callers, apply the handbook to touched code, run the relevant tests/build/vet, and inspect the final diff. |
| Decide test admission, write tests, or validate a change | [pixiv-cli-test](.agents/skills/pixiv-cli-test/SKILL.md), [test submission rules](docs/en/maintainers/coding-standards.md#test-submission-rules), [development](docs/en/maintainers/development.md#tests) | Match each maintained test to a contract; respect size caps and CI exemptions; report actual pass, fail, skip, and unrun checks. |
| Write or review code comments, API docs, or numbered stages | [pixiv-cli-code-commenting](.agents/skills/pixiv-cli-code-commenting/SKILL.md), [package documentation and comments](docs/en/maintainers/coding-standards.md#package-documentation-and-comments) | Check comment language, package-doc placement, references, and any directive's actual consumer. |
| Change package ownership or a shared mechanism | [architecture](docs/en/maintainers/architecture.md), [coding standards](docs/en/maintainers/coding-standards.md) | Verify the owner, allowed dependency direction, and all affected callers against the current package map. |
| Add or change an MCP tool | [pixiv-cli-mcp-tool](.agents/skills/pixiv-cli-mcp-tool/SKILL.md), [MCP contract](docs/en/mcp-tools.md), [MCP architecture](docs/en/maintainers/architecture-mcp.md) | Check product-local ownership, schema and structured errors, authorization, and the stdio/HTTP output boundary. |
| Change ugoira encoding, platform builds, or native smoke | [pixiv-cli-native](.agents/skills/pixiv-cli-native/SKILL.md), [development](docs/en/maintainers/development.md) | Verify the required target builds and native evidence; distinguish fixture results from platform execution. |
| Change frontend pages or embedded assets | [pixiv-cli-docs](.agents/skills/pixiv-cli-docs/SKILL.md), [frontend architecture](docs/en/maintainers/architecture.md#frontend-source-and-embedded-pages) | Check the embedded source and exercise the affected page in its browser or MCP host. |
| Edit documentation, skills, or agent routing | [pixiv-cli-docs](.agents/skills/pixiv-cli-docs/SKILL.md), [English documentation index](docs/index.md), [简体中文文档索引](docs/index-zh-CN.md), and the relevant maintainer reference | Verify source claims, paired locale content, local links/anchors, metadata routes, and `git diff --check`. |
| Review code or assess a pull request | [pixiv-cli-review](.agents/skills/pixiv-cli-review/SKILL.md), [coding standards](docs/en/maintainers/coding-standards.md) | Tie each finding to changed lines and evidence; identify unresolved risks and required checks. |
| Diagnose checks or operate an authorized workflow run | [pixiv-cli-ci](.agents/skills/pixiv-cli-ci/SKILL.md), the relevant workflow under `.github/workflows/` | Check the exact revision and report each required check's actual conclusion, including skips and failures. |
| Prepare, update, or verify a pull request | [pixiv-cli-pr](.agents/skills/pixiv-cli-pr/SKILL.md), [contributing](CONTRIBUTING.md) | Validate PR metadata and inspect required checks and review threads on the current head. |
| Prepare a release or recover a publisher | [pixiv-cli-release-notes](.agents/skills/pixiv-cli-release-notes/SKILL.md), [release development guide](docs/en/maintainers/release.md) | Verify the authorized version, release assets, and required release gates. |
| Write a commit message | [pixiv-cli-commit-message](.agents/skills/pixiv-cli-commit-message/SKILL.md) | Base it on the staged diff and name only the change it contains. |
| Use an installed `pixiv` binary | [product skill](skills/pixiv-cli/SKILL.md) | Follow the documented command and credential flow for the installed version. |
| Follow contributor setup or project expectations | [contributing](CONTRIBUTING.md) | Check the documented local workflow and the contribution requirements for this change. |

## Hard constraints

- Keep credentials and other secrets out of logs, errors, fixtures, pull requests, and artifacts; never inspect the SQLite account store as a debugging shortcut. Only an explicitly requested `pixiv auth export [UID]`, `pixiv auth export --all`, or local `pixiv mcp auth init [--reset]` may emit its documented secret to stdout; otherwise use the documented private-output or transfer path.
- Explain network access and material side effects before execution. Real Pixiv/FANBOX calls, browser-credential access, image uploads, and account writes require explicit scope and authorization.
- Obtain approval before adding a dependency or installing a tool.
- Keep production builds permanently CGO-free and use only pure-Go dependencies. Race instrumentation follows the development guide's C-toolchain requirement.
- A pull request does not authorize merging, force-pushing, tagging, publishing, changing branch protection, or using production secrets. Release authorization is separate and version-specific.
- Preserve the App-only boundary: content operations require an authenticated local account or an eligible database-managed pool account, and errors never select an anonymous Web path. Data commands do not accept `--uid` or `--refresh-token`; the public SDK and MCP retain their explicit credential contracts.
- Keep CLI machine output clean, reserve MCP stdio stdout for JSON-RPC, and require OAuth bearer authorization for HTTP. Preserve MCP runtime `isError=true` and report cancellation, transport, authentication, upstream, and persistence failures instead of returning success-shaped empty data.
- Perform Git write operations in an independent worktree and preserve existing changes.

## Authoritative references

- Coding rules: [English handbook](docs/en/maintainers/coding-standards.md) · [简体中文手册](docs/zh-CN/maintainers/coding-standards.md)
- Ownership and dependency direction: [English architecture](docs/en/maintainers/architecture.md) · [简体中文架构](docs/zh-CN/maintainers/architecture.md) · [MCP architecture](docs/en/maintainers/architecture-mcp.md)
- Toolchain, tests, native checks, and releases: [English development guide](docs/en/maintainers/development.md) · [简体中文开发流程](docs/zh-CN/maintainers/development.md)
- Contributor expectations: [English](CONTRIBUTING.md) · [简体中文](CONTRIBUTING.zh-CN.md)
- Public behavior: [CLI reference](docs/en/cli-reference.md) · [MCP tools](docs/en/mcp-tools.md)
- Workflow YAML, `scripts/build/platforms.json`, and tool manifests define executable CI/build configuration.
