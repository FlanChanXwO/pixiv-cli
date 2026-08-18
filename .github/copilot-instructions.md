# Pixiv CLI Copilot Instructions

本仓库的主规则在 [`AGENTS.md`](../AGENTS.md)。本文件只给 Copilot 提供短提示，避免补全时发明不存在的包、命令或 API。

## Project Shape

- Go module: `github.com/FlanChanXwO/pixiv-cli`
- Binary: `cmd/pixiv`
- CLI controller: `internal/cli`
- Public SDKs: `sdk/pixiv`, `sdk/fanbox`
- CLI production wiring: `internal/cli/root.go` and owner-local narrow ports
- Pixiv/FANBOX protocol adapters: `internal/services/{pixiv,fanbox}` (internal only)
- MCP tool adapter: `internal/mcpserver`
- Download and ugoira owners: `internal/media/{downloader,ugoira}`
- Config/auth storage: `internal/config/settings`, `internal/storage/database`; legacy `auth.json` is never auto-read

## Commands

```bash
go test ./...
sh scripts/build.sh
```

Do not suggest package-manager, frontend, database, Docker, or release commands unless the repository already contains that workflow.

## Guardrails

- Keep `internal/cli` as the controller and composition root; command owners use explicit narrow ports and public `sdk/pixiv` or `sdk/fanbox` clients.
- CLI/MCP capabilities must not import `internal/services/{pixiv,fanbox}` protocol adapters directly. There is no cross-product `internal/application.SDKService` or `internal/bootstrap` layer in the v1 architecture.
- MCP tool changes must update focused tests plus `docs/en/mcp-tools.md` and `docs/zh-CN/mcp-tools.md`; update localized README files or `CHANGELOG.md` when user-visible behavior changes.
- Do not print refresh tokens or hide authentication, network, Pixiv API, or filesystem errors behind empty success results.
- Do not add arbitrary truncation, retry limits, item caps, timeout behavior, or silent fallback without evidence and documentation.
