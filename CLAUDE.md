# CLAUDE.md

This repository is a Go implementation of a Pixiv MCP stdio server. It exposes Pixiv search, browsing, recommendation, user, bookmark, download, token refresh, and thumbnail tools to MCP clients.

Answer in Simplified Chinese by default. Keep code identifiers, commands, paths, and protocol/tool names in English. Code comments should usually be Chinese and explain intent, constraints, and edge cases.

## Repository Shape

- `main.go`: official binary entrypoint. It only delegates to the top-level `cmd` package.
- `cmd/`: Cobra command tree, `auth.json`/`config.toml` storage, TTY prompts, loopback OAuth login, CLI output, and `pixiv mcp` dispatch.
- `internal/pixiv/`: Pixiv app API client, OAuth refresh flow, API error handling, and JSON response types.
- `internal/mcpserver/`: MCP server construction, tool registration, input structs, authentication checks, and text formatting.
- `internal/download/`: background download manager, deduplication, multi-page storage, ugoira zip handling, and `ffmpeg` conversion.
- `pkg/pixivutil/`: filename sanitization, filename template expansion, and ID deduplication.
- `manifest.json`: DXT/MCP packaging metadata and user config.
- `docs/`: project documentation.

## Local Commands

Always run commands from the repository root:

```bash
go test ./...
go build .
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

- `cmd/*_test.go`
- `internal/pixiv/client_test.go`
- `internal/download/manager_test.go`
- `internal/mcpserver/server_test.go`

For code changes, update or add focused tests and run `go test ./...` before finishing whenever feasible.
