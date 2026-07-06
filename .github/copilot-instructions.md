# Pixiv CLI Copilot Instructions

本仓库的主规则在 [`AGENTS.md`](../AGENTS.md)。本文件只给 Copilot 提供短提示，避免补全时发明不存在的包、命令或 API。

## Project Shape

- Go module: `github.com/FlanChanXwO/pixiv-cli`
- Binary: `cmd/pixiv`
- CLI controller: `internal/cli`
- Application use cases: `internal/application`
- Production wiring: `internal/bootstrap`
- Pixiv facade/source: `internal/pixiv`
- MCP tool adapter: `internal/mcpserver`
- Download manager: `internal/download`
- Config/auth storage: `internal/config`, `internal/storage/auth`

## Commands

```bash
go test ./...
go build -o pixiv ./cmd/pixiv
```

Do not suggest package-manager, frontend, database, Docker, or release commands unless the repository already contains that workflow.

## Guardrails

- Keep `internal/cli` as controller code; put use-case orchestration in `internal/application` and production wiring in `internal/bootstrap`.
- CLI/MCP should depend on `internal/pixiv` facade rather than reaching into `internal/pixiv/api` or `internal/pixiv/web` without a clear package-level reason.
- MCP tool changes must update focused tests and `docs/mcp-tools.md`; update `README.md` or `CHANGELOG.md` when user-visible behavior changes.
- Do not print refresh tokens or hide authentication, network, Pixiv API, or filesystem errors behind empty success results.
- Do not add arbitrary truncation, retry limits, item caps, timeout behavior, or silent fallback without evidence and documentation.
