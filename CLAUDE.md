# CLAUDE.md

This repository is a Go implementation of a Pixiv MCP stdio server. It exposes Pixiv search, browsing, recommendation, user, bookmark, download, token refresh, and thumbnail tools to MCP clients.

Answer in Simplified Chinese by default. Keep code identifiers, commands, paths, and protocol/tool names in English. Code comments should usually be Chinese and explain intent, constraints, and edge cases.

## Repository Shape

- `cmd/pixiv/`: official binary entrypoint. It only delegates to `internal/cli`.
- `internal/cli/`: Cobra command tree, TTY prompts, loopback OAuth login, CLI output, and `pixiv mcp` dispatch.
- `internal/application/`: application use cases for accounts, config, artwork queries, downloads, and login completion.
- `internal/bootstrap/`: production composition root for config, auth storage, Pixiv source, OAuth client, MCP runtime, and application services.
- `internal/storage/auth/`: `auth.json` account storage, default account selection, auth file path, and private-file writes.
- `internal/config/`: `config.toml` schema, defaults, effective runtime config, and sparse get/set/unset writes.
- `internal/pixiv/`: Pixiv source facade plus stable constructors and common model aliases for CLI/MCP.
- `internal/pixiv/api/`: Pixiv App API client, OAuth refresh flow, authorization-code exchange, and API error handling.
- `internal/pixiv/web/`: anonymous Pixiv web/ajax API client used for tokenless fallback.
- `internal/pixiv/model/`: shared response/domain types and typed Pixiv protocol constants.
- `internal/mcpserver/`: MCP server construction, tool registration, input structs, authentication checks, and text formatting.
- `internal/download/`: background download manager, deduplication, multi-page storage, ugoira zip handling, and `ffmpeg` conversion.
- `internal/utils/`: filename sanitization, filename template expansion, ID deduplication, and Pixiv web refresh-token input parsing.
- `internal/utils/*`: protocol-free files/text/uri/media/parse helper packages.
- `internal/common/constants/`: infrastructure constants only; no Pixiv/MCP/config protocol values.
- `manifest.json`: DXT/MCP packaging metadata and user config.
- `docs/`: project documentation.

## Local Commands

Always run commands from the repository root:

```bash
go test ./...
go build -o pixiv ./cmd/pixiv
```

The module currently declares `go 1.26.3`.

## Runtime Configuration

Runtime config files:

- `os.UserConfigDir()/pixiv/auth.json`: account auth storage and `default_account`.
- `os.UserConfigDir()/pixiv/config.toml`: sparse global settings edited by users or `pixiv config`.

Environment variables:

- `PIXIV_REFRESH_TOKEN`: Pixiv refresh token.
- `DOWNLOAD_PATH`: download directory, default `./downloads`.
- `FILENAME_TEMPLATE`: filename template, default `{author} - {title}_{id}`.
- `https_proxy` / `HTTPS_PROXY`: optional proxy URL. Lowercase `https_proxy` wins when both are set.

Optional external dependency:

- `ffmpeg`: required only for ugoira GIF conversion.

Do not commit tokens, downloaded artwork, local databases, caches, or machine-specific files.

## Engineering Rules

- Prefer local inspection and local tests before broad searching.
- Preserve user changes. Never revert unrelated work.
- Use `rg` for text search. Read large files by focused ranges.
- Keep package boundaries clear and avoid turning one file into many responsibilities.
- Do not add arbitrary timeouts, truncation, retry limits, item limits, hidden fallbacks, or silent downgrade paths. If a limit is justified by product docs, API behavior, existing precedent, or tests, document the reason in code and in the delivery note.
- Error handling should reveal the real cause. Avoid broad catch-and-ignore patterns or returning empty success for real failures.
- When functionality, API behavior, configuration, workflow, tests, or known limitations change, update `docs/` or `README.md`.

## Current Tests

Existing test files:

- `internal/cli/*_test.go`
- `internal/application`
- `test/e2e/pixiv_binary_test.go`
- `internal/config`
- `internal/storage/auth`
- `internal/utils`
- `internal/pixiv/...`
- `internal/download/manager_test.go`
- `internal/mcpserver/server_test.go`

For code changes, update or add focused tests and run `go test ./...` before finishing whenever feasible.

Real Pixiv web fallback e2e is opt-in and skipped by default. Run it explicitly with `PIXIV_E2E_WEB_API=1`, plus `PIXIV_WEB_API_PROXY` when the network needs a proxy.
