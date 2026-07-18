# Auth Import and Export Implementation Plan

Design source: `docs/superpowers/specs/2026-07-18-auth-import-export-design.md`

Worktree: `/Users/flanchan/Development/SourceCode/GithubProjects/pixiv-cli/.worktrees/auth-import-export`

Branch: `codex/auth-import-export`

Target release: `v0.4.0`

## Task 1: Direct refresh-token import and CLI breaking rename

Implement the single-token vertical slice:

- Replace `pixiv auth add` with `pixiv auth import [REFRESH_TOKEN]`.
- Remove `auth add`, its `--token` flag, and all compatibility aliases/stubs.
- Accept exactly one optional positional token.
- With no positional token, use the existing hidden prompt in a TTY; otherwise
  read the complete stdin stream, remove exactly one terminal LF/CRLF, reject
  additional line breaks and blank input, and do not generally trim whitespace.
- Preserve `--json`, `--proxy`, and `--no-proxy` for direct import.
- Rename application-layer Add vocabulary to Import without changing the
  public SDK `ImportAccount` contract.
- The positional form is allowed for Agent execution and must never echo the
  supplied or rotated token in output, errors, JSON, or logs.
- Keep `auth token` temporarily during this task; Task 3 replaces/removes it so
  each TDD slice stays runnable.

TDD tracer order:

1. Auth help and positional import.
2. TTY/no-TTY input framing and pipe round trip.
3. argument/flag conflicts, Cookie rejection, proxy behavior, output redaction,
   upsert, and default preservation.

Focused verification:

```bash
go test ./internal/cli ./internal/application ./pixiv -count=1
git diff --check
```

## Task 2: Public SDK bundle snapshot and offline atomic restore

Add the versioned bundle domain independently of CLI file flags:

- Add stable top-level `pixiv` SDK types and operations for a coherent local
  auth bundle snapshot and offline restore.
- Bundle schema is `pixiv-cli.auth-export`, version `1`, with default UID and
  ordered UID/username/refresh-token accounts only.
- Snapshot reads only the configured auth store; it never reads runtime/env
  token overrides, creates a transport, performs HTTP, refreshes a token, or
  mutates state.
- Restore parses/validates the complete bundle before writing, rejects empty
  accounts, unknown versions, invalid/duplicate UIDs, missing tokens, and an
  invalid default reference.
- Restore merges atomically: replace matching UIDs, append new UIDs in bundle
  order, preserve an existing destination default, otherwise restore the source
  default, and leave the store unchanged on parse, validation, destination-load,
  or pre-commit write errors. Post-commit cleanup/durability errors must expose a
  typed `committed` outcome; unresolved replacement recovery must expose
  `unknown`, requiring callers to inspect the destination and recovery artifacts
  instead of assuming rollback or the continued existence of old bytes.
- Return only non-secret added/updated account summaries.
- Keep existing `ImportAccount` and `ExportAccountRefreshToken` public SDK
  operations stable.

TDD tracer order:

1. Single coherent export snapshot and exact codec round trip.
2. Offline merge/default behavior and repeat restore.
3. validation, malformed/future schema, recursive duplicate JSON keys,
   local-state typed errors, zero HTTP, byte-for-byte unchanged store on
   pre-commit failure, explicit post-commit outcomes, and token redaction.

Focused verification:

```bash
go test ./internal/utils/files ./internal/storage/auth ./pixiv -count=1
go test -race ./internal/utils/files ./internal/storage/auth ./pixiv -count=1
git diff --check
```

## Task 3: Export CLI, bundle import, and arbitrary-destination secret writer

Complete the CLI surface and remove the last old command:

- Add `pixiv auth export [UID] [--all] [--output PATH] [--force]`.
- Remove `pixiv auth token` completely with no alias/stub.
- Default/UID stdout export writes exactly one raw token plus LF; `--all`
  stdout writes exactly one bundle and LF; success stderr is empty.
- `--output` always writes a bundle and emits only a non-secret path/count
  summary. Reject existing destinations unless `--force`.
- Add `auth import --file PATH`; `--file -` reads a bundle from stdin.
  Positional token and proxy flags conflict with `--file`.
- Add an arbitrary-destination secret writer that never chmods a parent:
  race-free exclusive no-clobber; cleanup on failure; same-directory synced
  temporary plus replacement for `--force`; Unix `0600`; explicit current-user
  secret ACL on Windows.
- Export stays offline, local-store-only, mutation-free, update-check-free, and
  free of success logs.
- Bundle restore produces safe text/JSON account status and default UID.

TDD tracer order:

1. Default/UID export replaces `auth token` with exact stdout/stderr.
2. `--all` bundle stdout and `--file`/`--file -` restore.
3. output file/no-clobber/force/parent permission/cleanup platform contracts.
4. zero HTTP, no environment override, no auth mutation, no update request,
   invalid combinations, typed errors, and secret non-leakage.

Focused verification:

```bash
go test ./internal/cli ./internal/application ./pixiv ./internal/storage/auth -count=1
go test ./test/e2e -count=1
git diff --check
```

## Task 4: Policies, localized docs, product skill, and release notes

Synchronize every user/Agent-visible contract:

- Update `AGENTS.md` and maintainer review checklist so only stdout forms of
  `auth export` are the explicit secret-output exception.
- Update maintainer architecture, ADR wording, development docs, English/
  Simplified Chinese/Japanese README and CLI references.
- Update `skills/pixiv-cli/**`: all auth commands are Agent-usable; a supplied
  RFT may be imported positionally; undisclosed tokens prefer prompt/stdin;
  stdout export is piped/redirected unless the user explicitly asks to receive
  the raw secret.
- Update `[Unreleased]` CHANGELOG as a `v0.4.0` breaking change.
- Update documentation/help contract tests. Do not generate knowledge graphs.

Verification:

```bash
go test ./scripts/documentation ./internal/cli -count=1
python -m pre_commit run --all-files
git diff --check
```

## Task 5: Whole-feature review, integration, and release

- Build the final binary and run isolated black-box import/export workflows
  with synthetic fixtures only. Never print a real user token.
- Run final spec review, then final code-quality/security review; fix and
  re-review every blocker.
- Run all release gates:

```bash
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
sh scripts/build.sh
python -m pre_commit run --all-files
git diff --check
```

- Integrate the feature branch without losing the staged Linux ABI work. If
  that work is required for release, commit/integrate it first and rerun all
  gates on the exact release commit.
- Verify GitHub authentication, remote/default branch state, release workflow,
  tags, changelog version, and absence of an existing `v0.4.0` tag/release.
- Commit, merge to `main`, push, create/push `v0.4.0`, monitor GitHub Actions,
  verify release assets/checksums/installers, and report the release URL only
  after the published artifacts pass verification.
