# v0.8.0 Design

## Purpose

Release v0.8.0 turns the existing CLI, MCP and SDK work-in-progress into one
coherent Pixiv service architecture. It adds feeds, a record-based pipeline,
manual account pooling, APNG ugoira output and independent ClawHub publication.
Fanbox and login-page worktrees are explicitly outside this release.

## Architecture

`cmd/pixiv` remains a thin entry point. CLI and MCP depend on
`internal/application.SDKService`, which calls the stable public `pixiv`
facade. Pixiv-specific application, protocol, storage, configuration, download
and ugoira code move under `internal/services/pixiv`. Generic configuration,
logging, bootstrap and local private state remain outside that service root.
`internal/common` is removed: local-state mechanics belong to
`internal/platform/localstate`; logging vocabulary belongs to `internal/logging`.

## User contracts

CLI data commands no longer accept `--uid` or `--refresh-token` and ignore
`PIXIV_REFRESH_TOKEN`. `pixiv auth use` selects the default account; a manually
maintained `[account_pool]` may select eligible local accounts for non-mutating
reads. The public Go SDK retains its existing explicit credential options.

Only `download_path`, `filename_template` and `https_proxy` are managed by
`pixiv config`. `login_timeout` and `premium_status_cache_ttl` are removed.
Other settings, including the pool, stay hand-maintained in TOML.

## Account-pool safety

Every eligible operation leases one configured local account and keeps it for
the complete pagination or download preparation. A valid typed 429 with
`Retry-After` can select another untried account only before any stdout record
or local file is committed. Pool state stores only the latest UID and freeze
timestamps; it never stores credentials. Writes, auth and configuration are
not pooled.

## APNG

`pixiv.DownloadOptions.UgoiraFormat`, CLI `--ugoira-format` and MCP
`ugoira_format` accept `gif` or `apng`. The zero/default value remains GIF. A
single encoder chooses the matching extension and output format. No persistent
format or concurrency setting is added.

## Release distribution and evidence

SkillHub remains unchanged and independent. A second ClawHub workflow consumes
the trusted completed-release tag, applies the same immutable-tag and skill-diff
checks, then uses pinned `clawhub@0.23.1` with a temporary config path. The
product skill is published as `pixiv-cli` / `Pixiv CLI` version `0.8.0`.

Release real E2E is CLI-only. It creates an isolated temporary local auth store
from the protected test token through stdin, performs no login or Pixiv writes,
and verifies authenticated reads plus GIF/APNG downloads. MCP and SDK retain
offline contract tests and build checks only.
