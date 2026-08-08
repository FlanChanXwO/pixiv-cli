# Pixiv CLI Copilot Instructions

本仓库的主规则在 [`AGENTS.md`](../AGENTS.md)。本文件只给 Copilot 提供短提示，避免补全时发明不存在的包、命令或 API。

## Project Shape

- Go module: `github.com/FlanChanXwO/pixiv-cli`
- Binary: `cmd/pixiv`
- CLI controller: `internal/cli`
- Application use cases: `internal/application`
- Production wiring: `internal/bootstrap`
- Pixiv protocol adapters: `internal/services/pixiv/{appapi,model,oauth,protocol,resource}`
- MCP tool adapter: `internal/mcpserver`
- Download manager: `internal/downloader`; download use case/port: `internal/application/download`
- Config/auth storage: `internal/application/config`, `internal/persistence/authdb`; legacy `auth.json` is never auto-read

## Commands

```bash
go test ./...
sh scripts/build.sh
```

Do not suggest package-manager, frontend, database, Docker, or release commands unless the repository already contains that workflow.

## Guardrails

- Keep `internal/cli` as controller code; put use-case orchestration in `internal/application` and production wiring in `internal/bootstrap`.
- CLI/MCP Pixiv capabilities should use the top-level `pixiv` public SDK through `internal/application.SDKService`; do not import `internal/services/pixiv/appapi`, `webapi`, `oauth`, or `resource` protocol adapters directly.
- MCP tool changes must update focused tests plus `docs/en/mcp-tools.md` and `docs/zh-CN/mcp-tools.md`; update localized README files or `CHANGELOG.md` when user-visible behavior changes.
- Do not print refresh tokens or hide authentication, network, Pixiv API, or filesystem errors behind empty success results.
- Do not add arbitrary truncation, retry limits, item caps, timeout behavior, or silent fallback without evidence and documentation.
