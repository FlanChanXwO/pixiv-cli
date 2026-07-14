# ADR 0001: CLI Thin Controller And Bootstrap

## Status

Accepted.

## Context

`internal/cli` originally owned Cobra command wiring, prompts, output formatting, account storage mutation, config reads/writes, Pixiv client construction, download manager construction, OAuth token exchange, and MCP runtime wiring. That made the directory feel like both frontend controller and application runtime.

The project needs a clearer internal structure without changing CLI/MCP behavior.

## Decision

- Keep `internal/cli` as the CLI controller layer:
  - Cobra command tree, flags and positional arguments.
  - TTY prompts and loopback browser OAuth receiver.
  - Text/JSON presenters and process exit handling.
- Add `internal/application` for use cases:
  - `AccountService`
  - `ConfigService`
  - `ArtworkService`
  - `DownloadService`
  - `LoginService`
- Add `internal/bootstrap` as the production composition root.
- Publish Pixiv access through concrete `pixiv.Client`; CLI/MCP reach it through application-owned narrow seams rather than importing internal App/Web transports.
- Move `internal/cli/state` to `internal/storage/auth`.
- Merge `internal/cli/mcpapp` into `internal/bootstrap` as MCP runtime wiring.

## Consequences

- CLI tests continue to validate user-visible behavior, while `internal/application` and `internal/storage/auth` gain focused package tests.
- Production wiring is centralized in `internal/bootstrap`, making future client/storage replacement less invasive.
- `auth login` intentionally keeps loopback HTTP and browser/prompt adapters in CLI; the application layer handles PKCE/state generation, authorization-code exchange, and account persistence.
- `internal/mcpserver` owns MCP tool registration; `internal/bootstrap` owns stdio runtime startup.
- Public SDK construction and logger injection remain in bootstrap/application. CLI and MCP remain presentation/protocol adapters, not Pixiv protocol owners.

## Guardrails

- Preserve token priority:
  - CLI: flag token, UID-selected account, env token, default UID.
  - MCP: env token, default UID.
- Preserve output contracts and do not print refresh tokens.
- Do not move durable config schema into CLI.
- Do not add generic `ports` packages unless a real duplication or dependency problem appears.
- Do not create an HTTP provider server or repository-owned generic Provider interface; external callers own their narrow adapters.
- `internal/common/constants` is allowed only as the narrow infrastructure-constant exception defined by ADR 0002.
