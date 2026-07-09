# Context

## Domain

This project is a Pixiv MCP stdio server with a companion CLI. It exposes Pixiv artwork search, detail, ranking, recommendation, user/bookmark flows, downloads, thumbnail delivery, and local account/config management.

## Architecture Vocabulary

- CLI controller: `internal/cli`; Cobra commands, flags, prompts, loopback browser interaction, and stdout/stderr presenters.
- Application services: `internal/application`; use cases for accounts, config, artwork queries, downloads, and login completion.
- Composition root: `internal/bootstrap`; wires config, auth storage, Pixiv clients, OAuth clients, download manager dependencies, and application services.
- Auth storage: `internal/storage/auth`; `auth.json` UID-keyed account schema, default UID selection, private path and `0600` writes.
- Pixiv source: `internal/pixiv`; facade over App API and anonymous web fallback.
- MCP server: `internal/mcpserver`; MCP tool registration and protocol adapter.
- Utility packages: `internal/utils/*`; non-business helper packages such as files, text, uri, media, and parse.
- Infrastructure constants: `internal/common/constants`; cross-package constants with no domain or protocol meaning.
- Pixiv authorization relay page: `accounts.pixiv.net/post-redirect`; an intermediate Pixiv OAuth page that is not a callback. Its `return_to` identifies the official App API start URL, but the CLI must not bypass the relay page when reopening it.

## Boundary Rules

- `internal/cli` should not own durable state mutation or Pixiv/download construction logic.
- `auth login` keeps loopback HTTP server, browser opening, and terminal prompts in CLI because those are local UI adapters.
- `auth login` may register a local macOS `pixiv://` URL handler that forwards only the final callback URL to the current CLI loopback server; it must not read cookies/tokens, automate browser UI, install extensions, or spoof successful login.
- `auth login` may open a managed Chromium/Edge browser and read Pixiv OAuth request URLs through DevTools only as a fallback callback detector.
- `auth login` may still read real browser Pixiv OAuth URLs for fallback callback/relay detection when managed capture is unavailable.
- `internal/application.LoginService` owns PKCE/state creation, OAuth code exchange, and account save.
- `internal/bootstrap` is the only place that should know how production services are composed.
- `internal/config` remains focused on `config.toml` schema, defaults, effective values, and sparse writes.
- `internal/common/constants` must not contain Pixiv protocol values, MCP delivery values, config keys, or product defaults; `AppConfigDirName` is the only product-named path exception.
- Adapter helpers for CLI, MCP, and OAuth loopback stay in their adapter package unless they are protocol-free parsing helpers.

## Behavioral Constraints

- Local account: a saved Pixiv identity keyed by Pixiv UID, with refresh token and optional username.
- CLI token priority is `--refresh-token` > `--uid`/deprecated `--profile` > `PIXIV_REFRESH_TOKEN` > default UID.
- MCP token priority is `PIXIV_REFRESH_TOKEN` > default UID.
- Runtime proxy priority is `--proxy URL` or `--no-proxy` > `https_proxy`/`HTTPS_PROXY` > `config.toml`; CLI proxy flags apply only to network commands and are never persisted.
- JSON/text output shapes should remain stable; refresh tokens must not be printed.
- OAuth URL callbacks must validate `state`; Pixiv official callback/code input and authorization relay URLs remain explicit fallback paths when the browser flow does not reach loopback.
- No new arbitrary timeout, truncation, retry, item limit, silent fallback, or hidden downgrade should be added without evidence and documentation.
