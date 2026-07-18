# ADR 0011: Locale-based public documentation layout

## Status

Accepted.

## Context

The repository originally mixed an English default README, suffixed Chinese mirrors, a bilingual CLI reference,
and Chinese-only SDK, MCP, architecture, development, ADR, and Agent documents in one `docs/` directory. Adding a
third language by suffixing every file would multiply maintainer-only documents, obscure which version was
canonical, and make incomplete translations look authoritative.

## Decision

- Keep GitHub entry files at the repository root: `README.md`, `README.zh-CN.md`, and `README.ja.md`.
- Store public interface contracts under BCP 47 locale directories: `docs/en/`, `docs/zh-CN/`, and `docs/ja/`.
- Treat English as the canonical public contract and require existing translations to preserve its behavior.
- Store architecture, development, ADR, and Agent collaboration documents under `docs/maintainers/` with one
  canonical language unless real maintainer demand justifies a translation.
- Do not create placeholder locale files containing another language. The documentation index links to English
  explicitly when a translation is unavailable.
- Retain small compatibility stubs at former paths during migration, while all new links target canonical paths.

## Consequences

Public language coverage is visible and scalable without triplicating internal material. Adding a locale requires
an explicit navigation and maintenance commitment. Existing bookmarks continue to resolve through compatibility
stubs, but link checks must reject new canonical documents that point back to those stubs.
