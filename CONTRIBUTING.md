# Contributing to pixiv-cli

English | [简体中文](CONTRIBUTING.zh-CN.md)

Thanks for helping improve `pixiv-cli`. Focused bug reports, documentation fixes, tests, compatibility work, and well-scoped features are welcome.

## Before you start

- Search existing issues and pull requests before opening a duplicate.
- Discuss large features, public API changes, new dependencies, authentication changes, or compatibility breaks before implementation.
- Never include Pixiv tokens, cookies, downloaded works, local databases, cache files, machine-specific configuration, or private API responses in an issue, fixture, commit, or CI log.
- Keep changes focused. Unrelated cleanup is easier to review as a separate pull request.

## Development environment

The supported source build uses:

- Go `1.26.3`;
- `CGO_ENABLED=1` and a working C linker for the target;
- the committed, manifest-verified Rust ugoira static library for the target.

Rust is only required when changing the ugoira encoder or rebuilding a static library. Do not install or regenerate native artifacts as part of an unrelated contribution. Windows builds run through Git Bash, MSYS2, or WSL.

Build and test from the repository root:

```bash
go test ./...
sh scripts/build.sh
./build/pixiv --help
```

See the [development guide](docs/development.md) for native-library verification, opt-in real API tests, release gates, and platform details.

## Architecture guardrails

- `cmd/pixiv` delegates to `internal/cli`; CLI controllers keep business use cases in `internal/application`.
- Production wiring belongs in `internal/bootstrap`.
- CLI and MCP Pixiv capabilities call the top-level public `pixiv` SDK through `internal/application.SDKService`; they do not call App/Web/OAuth/resource protocol adapters directly.
- MCP registration and transport adaptation belong in `internal/mcpserver`; stdout is reserved for JSON-RPC.
- Keep protocol-independent utilities under `internal/utils/*`, and keep files focused on one responsibility or a few tightly related responsibilities.

Read [docs/architecture.md](docs/architecture.md) and the repository [AGENTS.md](AGENTS.md) before changing these boundaries.

## Develop with tests

Use a red-green-refactor loop for code changes:

1. Add a focused test that fails for the intended behavioral reason.
2. Implement the smallest coherent change that makes it pass.
3. Refactor without changing the verified public behavior.
4. Run the focused tests, then the relevant regression suite.

Test public behavior through the public boundary whenever practical. Do not hide real authentication, network, Pixiv API, filesystem, or encoding failures behind empty success results or silent fallback. Do not add arbitrary timeouts, truncation, pagination caps, retry limits, or hidden downgrade paths.

Real Pixiv Web and authenticated App canaries are opt-in. Never run them with a user's local account unless that user has explicitly authorized it; never put a real token on a command line that may be stored in shell history.

## Documentation and compatibility

Update documentation in the same pull request when changing a command, flag, SDK API, MCP tool, configuration key, environment variable, output contract, authentication flow, proxy behavior, download behavior, or known limitation.

- Keep `README.md` and `README.zh-CN.md` aligned.
- Keep `docs/cli-reference.md` and `docs/cli-reference.zh-CN.md` aligned.
- Update `docs/sdk.md`, `docs/mcp-tools.md`, `docs/development.md`, or `docs/architecture.md` according to their documented responsibility.
- Update `[Unreleased]` in `CHANGELOG.md` for user-visible additions, fixes, changes, deprecations, removals, or security effects.
- Check `skills/pixiv/` when CLI commands, flags, or safety semantics change.

Keep stable rules in one authoritative document and link to them elsewhere instead of copying large sections.

## Pull request checklist

Before requesting review:

- [ ] The change is focused and its user-visible behavior is explained.
- [ ] New or changed code has focused tests that first demonstrated the failure.
- [ ] `go test ./... -count=1` passes.
- [ ] `go test -race ./... -count=1` passes for shared, authentication, download, CLI, MCP, or SDK behavior.
- [ ] `go vet ./...` passes.
- [ ] `sh scripts/build.sh` passes.
- [ ] `python -m pre_commit run --all-files` passes when pre-commit is available.
- [ ] `git diff --check` passes.
- [ ] English and Simplified Chinese documentation are synchronized where required.
- [ ] No credential, downloaded content, local state, or machine-specific artifact is included.

Conventional Commits are recommended for commit messages, for example `feat(cli): add account selection` or `docs: clarify anonymous fallback`. The project does not require a CLA, DCO sign-off, or signed commits unless a future policy explicitly says otherwise.

## License

By contributing, you agree that your contribution may be distributed under the repository's [MIT License](LICENSE).
