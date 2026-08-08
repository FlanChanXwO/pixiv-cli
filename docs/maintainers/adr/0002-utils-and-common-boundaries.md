# ADR 0002: Utils And Common Boundaries

## Status

Accepted.

## Context

Several small helpers were duplicated across config, auth storage, download, Pixiv web fallback, MCP output, and CLI parsing. At the same time, a broad `common` or `constants` package would make package ownership unclear and conflict with the guardrails in ADR 0001.

## Decision

- Use `internal/utils/{parse,text,uri}` only for protocol-free helper APIs.
- Keep filename generation and ID deduplication in `internal/downloader/{filename,ids}`, MIME mapping in `internal/record/media`, refresh-token validation in `internal/credentials`, and filesystem/secret writes in `internal/filesystem`.
- Do not keep a generic constants package. Local-state path/permission constants live in `internal/filesystem`.
- Keep adapter-specific helpers in their adapter packages: CLI Cobra/prompt/OAuth loopback helpers stay in `internal/cli/auth` or the matching command package; Pixiv/FANBOX delivery and tool result helpers stay in `internal/mcpserver/{pixiv,fanbox}`.

## Consequences

- Repeated low-level helpers are shared without creating a catch-all `common` package.
- Pixiv protocol constants, MCP delivery constants, config keys/defaults, and search/ranking enums remain in their owning domain packages.
- Package ownership is explicit: local-state constants do not become a place for product or protocol policy.

## Guardrails

- A helper may enter `internal/utils/*` only if it has no CLI, MCP, OAuth, Pixiv protocol, or config schema semantics.
- A constant belongs to the narrow package that owns its behavior: local-state path/permission values belong in `internal/filesystem`.
- Do not move values merely because they are repeated once; prefer local helpers until a real dependency or duplication problem exists.
