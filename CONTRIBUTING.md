# Contributing to pixiv-cli

English | [简体中文](CONTRIBUTING.zh-CN.md)

Thanks for helping improve `pixiv-cli`. Focused bug reports, documentation fixes, tests, and well-scoped features are welcome.

## Before you start

- Search existing issues and pull requests before opening a duplicate.
- Discuss large features, public API changes, new dependencies, or authentication changes before implementation.
- Never include Pixiv tokens, cookies, downloaded works, local databases, cache files, machine-specific configuration, or private API responses in an issue, fixture, commit, or CI log.
- Keep changes focused. Unrelated cleanup is easier to review as a separate pull request.

## Development environment

The supported source build uses:

- the Go version declared in `go.mod`;
- `CGO_ENABLED=1` and a working C linker for the target;
- the committed, manifest-verified Rust ugoira static library for the target.

Rust is only required when changing the ugoira encoder or rebuilding a static library. Do not install or regenerate native artifacts as part of an unrelated contribution. Windows builds run through Git Bash, MSYS2, or WSL.

Build and test from the repository root:

```bash
go test ./...
sh scripts/build.sh
./build/pixiv --help
```

See the [development guide](docs/en/maintainers/development.md) for native-library verification, opt-in real API tests, release gates, and platform details.

## Architecture guardrails

- `cmd/pixiv` delegates to `internal/cli`; `internal/cli/root.go` owns command registration, global lifecycle, and production wiring, while concrete commands live under `internal/cli/commands`.
- CLI and MCP Pixiv/FANBOX capabilities call the public `sdk/pixiv` and `sdk/fanbox` APIs through owner-local narrow ports; they do not call `internal/services/{pixiv,fanbox}` protocol adapters directly.
- MCP product aggregators live in `internal/mcpserver/{pixiv,fanbox}`, tools in each product's `tools/<tool>` directory, and stdio is started by the CLI MCP command; stdout remains reserved for JSON-RPC.
- Keep cross-subsystem mechanisms under `internal/shared/*`, protocol-independent leaf helpers under `internal/utils/*`, config/path ownership under `internal/config/{settings,paths}`, and files focused on one responsibility.

Read [the architecture guide](docs/en/maintainers/architecture.md) and the repository [AGENTS.md](AGENTS.md) before changing these boundaries.

## Develop with tests

Use a red-green-refactor loop for features and behavior fixes:

1. Inspect existing assertions; reuse a failing test or extend the smallest relevant case to expose the intended behavioral failure. Add a new test only for a coverage gap, not for each function or file.
2. Implement the smallest coherent change that makes it pass.
3. Refactor without changing the verified public behavior.
4. Run the focused tests, then the relevant regression suite.

Pure restructuring reuses characterization before and after; ordinary comment-only edits use document/tool checks. See [test selection](.agents/skills/pixiv-cli-test/SKILL.md#decide-whether-test-code-must-change) for coverage gaps, duplication, and applicable Red exceptions. A required regression or security check is not optional merely because its implementation is short.

Test public behavior through the public boundary whenever practical. Do not hide real authentication, network, Pixiv API, filesystem, or encoding failures behind empty success results or silent fallback. Do not add arbitrary timeouts, truncation, pagination caps, retry limits, or hidden downgrade paths.

Real Pixiv/FANBOX SDK and reverse-search checks are opt-in. Do not run them with a user's local account or upload an image unless the user has explicitly authorized that action; never put a real token on a command line that may be stored in shell history.

## Agent-assisted development

Start with [AGENTS.md](AGENTS.md). The checked-in `pixiv-cli-*` maintenance skills define Go design, focused testing, MCP/native work, review, PR, and release workflows without requiring any personal global instructions or CCS installation. Clients without skill discovery can read their `SKILL.md` files directly. Keep agent contracts, skill content, references, and UI metadata English; public locale documentation remains bilingual.

## Documentation

Source comments may be English or Chinese. Follow [code commenting](.agents/skills/pixiv-cli-code-commenting/SKILL.md) for accurate API contracts, intent, and optional numbered phases; the English instruction-file requirement does not impose English comments or per-function comment quotas.

Update documentation in the same pull request when changing a command, flag, SDK API, MCP tool, configuration key, environment variable, output contract, authentication flow, proxy behavior, download behavior, or known limitation.

- Keep `README.md` and `README.zh-CN.md` behaviorally aligned.
- Keep all existing locale versions under `docs/<locale>/` behaviorally aligned; never use untranslated placeholder content.
- Update localized SDK/MCP contracts or `docs/en/maintainers/` according to their documented responsibility.
- Keep the pull-request body useful for review: explain what changed, record the verification you actually ran, and complete the checklist. During release preparation, maintainers audit every merged PR and direct commit, then write matching English and Simplified Chinese notes with inline sources. Internal-only work belongs under `Maintenance`. See [release notes and publication](docs/en/maintainers/development.md#release-notes-and-publication).
- Check `skills/pixiv-cli/` when CLI commands, flags, or safety semantics change.

Keep stable rules in one authoritative document and link to them elsewhere instead of copying large sections.

## Pull request checklist

Before requesting review:

- [ ] The change is focused and its user-visible behavior is explained.
- [ ] Behavior changes have observed Red/Green and relevant regression evidence, or an explicitly accepted blocker; document-only work has document/link/metadata checks.
- [ ] Applicable local checks from [pixiv-cli-test](.agents/skills/pixiv-cli-test/SKILL.md) passed, including full/race/native checks when their scope applies; unrun checks are identified.
- [ ] Required CI is evaluated on the current head and its actual path classifier, not a presumed Markdown exemption.
- [ ] Existing pre-commit checks pass when installed and applicable; missing tooling is reported rather than installed silently.
- [ ] `git diff --check` passes.
- [ ] English and Simplified Chinese documentation are synchronized where required.
- [ ] No credential, downloaded content, local state, or machine-specific artifact is included.

Conventional Commits are recommended for commit messages, for example `fix(cli): preserve record identity` or `docs: clarify account selection`. The project does not require a CLA, DCO sign-off, or signed commits unless a future policy explicitly says otherwise.

## License

By contributing, you agree that your contribution may be distributed under the repository's [MIT License](LICENSE).
