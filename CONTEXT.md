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

## Provider Language

**Artwork provider**:
A transport-neutral capability boundary that supplies normalized Pixiv-compatible discovery, artwork, page, ugoira, and resource access.
_Avoid_: HTTP provider, Provider server

## Provider Relationships

- An **Artwork provider** can be exposed through an importable package or a machine-readable CLI adapter.
- HTTP is not part of the **Artwork provider** replacement boundary.

## Provider Example Dialogue

> **Dev:** “Does replacing the **Artwork provider** require running another HTTP service?”
> **Domain expert:** “No. The provider is the capability boundary; callers use its package API or CLI adapter.”

## Download Language

**Download quality**:
A single user-selected tier—`original`, `standard`, or `thumbnail`—that applies consistently to illustrations, manga pages, and ugoira, including both source resource selection and animation encoding strategy.
_Avoid_: Encoder quality, image quality

**Animation format**:
The output container produced from ugoira frames; GIF remains the default and APNG is an explicit alternative.
_Avoid_: Ugoira format

## Download Relationships

- **Download quality** selects the Pixiv resource variant for every downloadable media type and, for ugoira, also selects the matching animation encoding strategy.
- `original` ugoira resource resolution may query Pixiv web metadata even in an authenticated session; this is resource selection, not fallback after an App API failure.

## Download Example Dialogue

> **Dev:** “用户选择 `original`，但 App API 只有 medium ugoira zip，是否降级？”
> **Domain expert:** “不降级；通过 web metadata 解析 original zip。此访问用于选择资源版本，不表示 App API 失败后 fallback。”

## Flagged Ambiguities

- “质量”曾同时表示源图分辨率和编码器参数；现统一为 **Download quality**，并由一个档位同时决定资源版本与动图编码策略。
- GIF/APNG 是 ugoira 的本地 **Animation format**，不是 Pixiv 作品本身的类型。

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
- Runtime proxy priority is `--proxy URL` > `https_proxy`/`HTTPS_PROXY` > `config.toml`; CLI proxy flags apply only to network commands and are never persisted.
- JSON/text output shapes should remain stable; refresh tokens must not be printed.
- OAuth URL callbacks must validate `state`; Pixiv official callback/code input and authorization relay URLs remain explicit fallback paths when the browser flow does not reach loopback.
- No new arbitrary timeout, truncation, retry, item limit, silent fallback, or hidden downgrade should be added without evidence and documentation.
