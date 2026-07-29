# v0.8.0 Implementation Plan

## Integration baseline

- [x] Preserve current main WIP on `codex/release-v0.8.0-integration`.
- [x] Port the feeds, stream pipeline and account-pool worktrees by behavior,
      not by a blind merge; resolve overlapping CLI and SDK code against the
      final design.
- [x] Keep fanbox and login-page paths/branches out of every integration step.

## Service and command boundaries

- [x] Add failing package-boundary/documentation tests, then move Pixiv service
      code under `internal/services/pixiv` while retaining the public `pixiv`
      imports and `internal/application.SDKService` facade.
- [x] Replace `internal/common` imports with `internal/platform/localstate` and
      `internal/logging`; delete the old package only after all callers move.
- [x] Split CLI and MCP command/contract wiring without import cycles; preserve
      existing output contracts except approved v0.8 breaking changes.
- [x] Add exported-doc AST coverage and package documentation for core/public
      packages.

## Features and configuration

- [x] Add red tests for feeds and canonical NDJSON pipeline records, filtering,
      actions and committed-output error behavior; implement the minimum shared
      application interfaces.
- [x] Add red tests for manual account-pool parsing, state, sticky pagination,
      valid Retry-After rotation and post-commit no-replay; then wire every
      non-mutating read through the pool boundary.
- [x] Remove data-command credential flags/environment selection and add CLI
      help/behavior regression tests.
- [x] Restrict `pixiv config` aliases to the three approved keys; remove the
      two obsolete settings and update TOML/docs tests.
- [x] Add failing GIF/APNG public, CLI and MCP tests; expose
      `pixiv.UgoiraFormat`, then route all ugoira encodes through one encoder.

## Distribution and documentation

- [x] Add a ClawHub workflow and tests/policies for immutable release handoff,
      dry-run, temporary token config, publish JSON and inspect verification.
- [x] Update the product skill to 0.8.0 and synchronize all locale references,
      CLI/MCP/SDK docs, architecture docs and bilingual changelog.

## Verification and finish

- [x] Run focused package tests after every red-green-refactor task, then full
      test, race, vet, build, workflow-policy and documentation suites.
- [x] Update real E2E to seed an isolated local auth store through stdin and
      define only CLI real API paths, including GIF/APNG download; retain
      MCP/SDK offline smoke coverage. Successful live execution remains a
      protected-release-environment gate.
- [x] Conduct spec and code-quality reviews; write `review.md`, fix all
      Critical/Major findings, then write `final_report.md` and present the
      integration branch for merge.
