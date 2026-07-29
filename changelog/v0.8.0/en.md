# v0.8.0 — 2026-07-28

## Breaking changes

- The public Go SDK has a new two-level download API. Use `Download` or `DownloadAll` for the common case, and
  `DownloadWith` or `DownloadAllWith` with `DownloadOptions` when choosing paths, naming, pages, quality, or
  concurrency. The previous raw `Download(ctx, ResourceRef, path)` is now `DownloadResource`.
- SDK construction is now explicit: use `OpenDefault()` for local defaults, `OpenDefaultWith(OpenDefaultOptions)` for
  local-state customisation, and `NewClient(NewClientOptions)` for access-token clients. The former combined
  `Options` type has been removed.
- Configuration is type-safe. `GetConfig`, `SetConfig`, and `UnsetConfig` now use `ConfigKey`, `ConfigInput`, and
  `ConfigValue`; CLI text is parsed at the boundary. Sensitive relay secrets cannot be read or written through the
  generic configuration API.
- `pixiv download` now takes `SRC...` and exposes `--concurrency`. MCP `download` accepts `src` or `srcs` plus
  `concurrency`; its separate legacy PID/URL input fields have been removed.

## Added

- Downloads accept an artwork ID, an official Pixiv artwork URL, or an allowed CDN URL. The beginner defaults are
  `./downloads`, the documented naming template, and automatic concurrency of `2 × GOMAXPROCS`; a positive
  concurrency value is used exactly as requested.
- Batch download results preserve source order and report each item's start state, result, cache state, and error.
  Direct CDN downloads retain their URL filename and reject artwork-only page, quality, or custom-template options.
- SDK, CLI, and MCP downloads now use persistent `.pixiv-cache` metadata revalidation, atomic replacement, safe
  `Range` plus `If-Range` resume, and cached Ugoira ZIP inputs. Resource results expose their cache state.
- `ParseUserReference` complements artwork-reference parsing without weakening the type-safe ID APIs.
- Cross-machine `auth login` callback relay configuration is available for macOS, Windows, and desktop Linux clients.
  It uses an on-demand persistent `pixiv://` handler, strict callback allowlisting, one-time secret/state validation,
  optional TLS, and explicit warnings when HTTP plaintext is configured or started. It does not use browser
  automation or Web fallback. macOS does not silently retain pixiv-cli as the default handler when no previous
  handler exists.

## Fixed

- Restored the browser success/failure page for cross-machine `auth login`. The client handler opens a one-time,
  non-sensitive relay result page that waits for the server's actual OAuth exchange outcome.
