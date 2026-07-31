# ADR 0002: Utils And Common Boundaries

## Status

Accepted.

## Context

Several small helpers were duplicated across config, auth storage, download, Pixiv web fallback, MCP output, and CLI parsing. At the same time, a broad `common` or `constants` package would make package ownership unclear and conflict with the guardrails in ADR 0001.

## Decision

- Use `internal/utils/*` subpackages for protocol-free helper APIs:
  - `files`: user config paths, private file writes, and cross-platform file replacement.
  - `text`: string defaulting and first non-empty string selection.
  - `uri`: URL path extraction and file URI generation.
  - `media`: MIME type inference from file extension.
  - `parse`: generic positive integer parsing.
- Keep `internal/utils` root for Pixiv/download utility APIs that carry project semantics, such as filename generation, ID deduplication, and Pixiv web refresh-token input parsing.
- Do not keep a generic constants package. Local-state path/permission constants live in `internal/platform/localstate`.
- Keep adapter-specific helpers in their adapter packages: CLI Cobra/prompt/OAuth loopback helpers stay in `internal/cli`; MCP delivery, formatting, and tool result helpers stay in `internal/mcpserver`.

## Consequences

- Repeated low-level helpers are shared without creating a catch-all `common` package.
- Pixiv protocol constants, MCP delivery constants, config keys/defaults, and search/ranking enums remain in their owning domain packages.
- Package ownership is explicit: local-state constants do not become a place for product or protocol policy.

## Guardrails

- A helper may enter `internal/utils/*` only if it has no CLI, MCP, OAuth, Pixiv protocol, or config schema semantics.
- A constant belongs to the narrow package that owns its behavior: local-state path/permission values belong in `internal/platform/localstate`.
- Do not move values merely because they are repeated once; prefer local helpers until a real dependency or duplication problem exists.
