# pixiv-cli Agent Contract

This repository provides the `pixiv` CLI, two MCP stdio servers, and the public Go packages `sdk`, `sdk/pixiv`, and `sdk/fanbox`.

## Start here

Read the relevant local skill below before its task. These are checked-in instructions, not dependencies on a contributor's personal configuration, CCS installation, or global skills. When the client cannot discover `.agents/skills`, open the linked `SKILL.md` directly. Read only the routes needed for the change.

| Task | Local instructions |
| --- | --- |
| Implement, debug, refactor, or design Go changes | [pixiv-cli-develop](.agents/skills/pixiv-cli-develop/SKILL.md) |
| Select tests, run Red/Green, or validate a change | [pixiv-cli-test](.agents/skills/pixiv-cli-test/SKILL.md) |
| Add or change an MCP tool | [pixiv-cli-mcp-tool](.agents/skills/pixiv-cli-mcp-tool/SKILL.md) |
| Change Rust, cgo, native libraries, or platform evidence | [pixiv-cli-native](.agents/skills/pixiv-cli-native/SKILL.md) |
| Edit documentation or either kind of skill | [pixiv-cli-docs](.agents/skills/pixiv-cli-docs/SKILL.md) |
| Review code or assess a PR | [pixiv-cli-review](.agents/skills/pixiv-cli-review/SKILL.md) |
| Prepare, update, or verify a PR | [pixiv-cli-pr](.agents/skills/pixiv-cli-pr/SKILL.md) |
| Diagnose checks or operate an authorized workflow run | [pixiv-cli-ci](.agents/skills/pixiv-cli-ci/SKILL.md) |
| Prepare a release or recover a publisher | [pixiv-cli-release-notes](.agents/skills/pixiv-cli-release-notes/SKILL.md) |
| Write a commit message from staged changes | [pixiv-cli-commit-message](.agents/skills/pixiv-cli-commit-message/SKILL.md) |

The separately distributed [product skill](skills/pixiv-cli/SKILL.md) teaches use of an installed binary; it is not a repository development workflow. Keep maintenance skill names prefixed with `pixiv-cli-` and the product name `pixiv-cli`.

## Non-negotiable boundaries

- Keep `cmd/pixiv` thin. `internal/cli/root.go` assembles the command tree and production dependencies; command owners live under `internal/cli/commands`. Do not resurrect a global service locator or the removed bootstrap/resource graph.
- CLI/MCP Pixiv and FANBOX operations use the public SDK and owner-local narrow ports, not protocol adapters. MCP tools belong to `internal/mcpserver/{pixiv,fanbox}/tools/<tool>`; their stdio runtimes are started by CLI commands.
- Reverse search is the explicit exception: only the CLI composition root may import `internal/services/reversesearch/assembly`; command and MCP owners may use the top-level `internal/services/reversesearch` contract, never its provider subpackages.
- Keep shared mechanisms in their existing owners: record, pagination, traversal, lifecycle, configuration, file persistence, and downloader. Generic utilities must not acquire product protocol or account semantics. See [architecture](docs/en/maintainers/architecture.md) for detailed ownership.
- Preserve the App-only boundary. Content requires an authenticated local account or an eligible database-managed pool account; errors never select an anonymous Web path. Data commands do not accept `--uid` or `--refresh-token`; the public SDK and MCP retain their own explicit credential contracts.
- Keep secrets out of logs, errors, fixtures, PRs, and artifacts. Only an explicitly requested bare `auth export [UID]` or `auth export --all` may emit secret stdout; otherwise use the documented private-output/transfer path. The SQLite account store is secret-bearing; never inspect real credentials as a debugging shortcut.
- Preserve clean CLI machine output and MCP JSON-RPC stdout. MCP runtime failures retain structured output with `isError=true`. Report cancellation, transport, authentication, upstream, and persistence failures rather than success-shaped empty data.
- Introduce limits, timeouts, retries, truncation, or fallbacks only for a verified requirement, platform constraint, established contract, or reproducible failure. Explain and test the trigger without silently discarding valid data.

## Working agreement

- Identify the requested behavior and acceptance evidence before editing. Keep small changes small; clarify consequential unknowns for larger work. Reuse the issue, PR, or conversation for decisions rather than creating process files automatically.
- Inspect the branch and existing changes; preserve unrelated work and use an isolated worktree when none exists. Use only available tools, explicit working directories, and safely quoted inputs. Prefer available semantic navigation for symbols and callers; disclose when only targeted search/compiler checks are available.
- For source changes, actually run a relevant failing test before implementation, then make it pass and refactor with regression checks. If a meaningful Red cannot run, report the blocker and obtain an explicit exception. Documentation-only edits need document, link, metadata, and affected-contract validation, not invented runtime tests.
- Use the Go version in `go.mod`, `gofmt`, and existing tests/vet/hooks. Follow the develop/test skills for language and ownership rules. Preserve native build requirements; a Linux fixture pass is not all-platform evidence.
- Reuse standard-library, platform, and existing project capabilities. Obtain approval before adding dependencies or installing missing tools; explain necessity, alternatives, and material lockfile, license, security, or deployment effects.
- Explain network access and material side effects before execution. Real Pixiv/FANBOX calls, browser credential access, image uploads, and account writes require explicit scope and authorization; they are not routine offline verification.
- Keep multi-step progress visible with the client's plan tool when available, otherwise a concise checklist. Restore actual state on continuation. Delegate only bounded work with clear ownership; integrated verification remains the main agent's responsibility.
- Write `AGENTS.md`, agent bridges, maintenance/product skills, their references, and UI metadata in English. Keep public English/Simplified Chinese documentation behaviorally aligned. Use the user's requested conversation language; do not impose a private maintainer's chat preferences on contributors.
- Review meaningful changes before handoff. Report the actual diff, exact checks and outcomes, remaining risks, and PR/worktree location. A local review is not a GitHub approval, and a pending or skipped check is not a pass.
- Creating a PR does not authorize merging it, moving tags, publishing, changing branch protection, or using production secrets. Release authorization is separate and version-specific.

## Authoritative references

[Development and test layout](docs/en/maintainers/development.md), [CLI contract](docs/en/cli-reference.md), [MCP contract](docs/en/mcp-tools.md), and [contributing](CONTRIBUTING.md) explain the checked-in behavior. Workflow YAML, `ci/platforms.json`, and tool manifests own executable configuration. When prose and implementation disagree, verify the disputed behavior and update the affected contract; do not guess or rewrite unrelated history.
