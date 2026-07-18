# Auth Import and Export Design

## Status

Proposed for `v0.4.0` after user approval. The latest release is `v0.3.0`, so
this is a breaking pre-1.0 CLI change.

## Context

The current authentication CLI has two low-level names that obscure the actual
operation:

- `pixiv auth add --token TOKEN` imports and validates a Pixiv App API refresh
  token.
- `pixiv auth token [UID]` exports a stored refresh token.

The flag form duplicates the secret in command arguments, while the command
names do not form a clear migration or backup workflow. Agents also need to be
able to execute every authentication command, including the case where a user
has deliberately supplied a refresh token in the conversation.

## Goals

- Replace the old CLI entries with one import command and one export command.
- Support direct human input, direct Agent input, non-interactive pipelines,
  single-account migration, and complete account backup/restore.
- Keep refresh tokens opaque and prevent accidental copies in logs, errors,
  JSON status output, or ordinary success messages.
- Preserve the existing public SDK and architecture boundaries: CLI calls the
  application account service, which calls the top-level `pixiv` SDK.
- Keep the bundle format versioned so future auth-store changes do not break
  existing exports.

## Non-goals

- No Cookie import or conversion.
- No access-token export.
- No MCP tool for persistent account import/export. Agents use the CLI; the
  existing session-scoped MCP refresh-token tool is unchanged.
- No built-in cloud synchronization or credential vault.
- No knowledge-graph generation.
- No mandatory encryption dependency. Export files use a dedicated
  arbitrary-destination secret writer and may be piped into an external secret
  manager or encryption tool when desired.

## CLI Contract

### Removed entries

The following entries are removed completely. They are not aliases, hidden
commands, or compatibility stubs:

```text
pixiv auth add
pixiv auth token
pixiv auth add --token
```

Invoking an old command returns Cobra's ordinary unknown-command error and a
non-zero exit status. The auth help lists only the new names.

### Import

```text
pixiv auth import [REFRESH_TOKEN] [--file PATH] [--json]
                  [--proxy URL | --no-proxy]
```

Input selection is deterministic:

1. A positional argument is always an opaque Pixiv App API refresh token. It is
   never interpreted as a file path.
2. `--file PATH` reads a versioned auth-export bundle. `--file -` reads that
   bundle from stdin.
3. With neither a positional argument nor `--file`, an interactive terminal
   displays a hidden `Refresh token` prompt.
4. With neither a positional argument nor `--file` and non-interactive stdin,
   the command reads one opaque refresh token from stdin.

The positional token and `--file` are mutually exclusive. More than one
positional argument is invalid. A blank input is invalid. `--proxy` and
`--no-proxy` retain their existing mutual-exclusion and precedence semantics
for raw-token import; either proxy flag conflicts with offline `--file` restore.
Raw-token stdin consumes the complete stream, removes exactly one terminating
`LF` or `CRLF`, and rejects blank input or any remaining line break. It does not
perform general whitespace normalization. This makes the single-account export
output directly pipeable while preserving token framing.

Examples:

```bash
# User-controlled hidden input.
pixiv auth import

# A user has already given an Agent the token and asked it to import it.
pixiv auth import 'REFRESH_TOKEN'

# CI or secret-manager input without a token argument.
secret-manager read pixiv-refresh-token | pixiv auth import

# Restore a bundle from disk or a pipeline.
pixiv auth import --file accounts.pxauth
pixiv auth export --all | ssh server pixiv auth import --file -
```

Every directly supplied refresh token is validated through the App OAuth flow.
The returned Pixiv UID is authoritative, and the rotated refresh token is
stored. Direct import is an upsert by UID: an existing account is updated, a new
account is added, the first account becomes the default, and importing another
account does not change an existing default. Bundle restore follows the offline
contract below instead.

The positional form is intentionally convenient for a token the user has
already disclosed to an Agent. It does not improve argv security: the token may
also be recorded in the Agent tool call, shell history, or a process listing.
Tokens that have not already been disclosed should use the hidden prompt or
stdin path.

For a single import, stdout reports only the UID, username when available,
default status, and whether the account was added or updated. `--json` exposes
the same non-secret fields. stderr and errors never include input or rotated
tokens.

### Export

```text
pixiv auth export [UID] [--all] [--output PATH] [--force]
```

- With neither UID nor `--all`, export the default stored account.
- With UID, export exactly that stored account.
- `--all` exports every stored account in stable auth-store order and conflicts
  with UID.
- Without `--output`, single-account export writes only
  `<refresh-token>\n` to stdout. `--all` writes only the versioned bundle to
  stdout. stderr remains empty on success.
- With `--output`, both single and all-account exports write a versioned bundle
  through an arbitrary-destination secret writer. stdout reports only the
  destination path and account count; it never contains a token.
- Existing files are rejected unless `--force` is explicit.

The export writer is separate from the config/auth-store writer. It never
changes permissions on an existing parent directory. Without `--force`, it
uses exclusive creation so a concurrent writer cannot be overwritten; a failed
write removes its incomplete destination. With `--force`, it writes and syncs a
private same-directory temporary file before platform-appropriate replacement.
On Unix the resulting file mode is `0600`. On Windows it applies an explicit
ACL limited to the current user and required system principals rather than
claiming that inherited parent ACLs are private. Platform tests cover parent
permission preservation, no-clobber, replacement, cleanup, and effective secret
access policy.

Export is entirely local: it does not read `PIXIV_REFRESH_TOKEN`, apply a
runtime UID override, refresh a token, perform HTTP requests, change the auth
store, or trigger an automatic update check.

Examples:

```bash
pixiv auth export
pixiv auth export 12345678
pixiv auth export 12345678 --output account.pxauth
pixiv auth export --all --output accounts.pxauth
pixiv auth export 12345678 | ssh server pixiv auth import
```

## Bundle Format

The UTF-8 JSON bundle is a public, versioned interchange format:

```json
{
  "schema": "pixiv-cli.auth-export",
  "version": 1,
  "default_user_id": 12345678,
  "accounts": [
    {
      "user_id": 12345678,
      "username": "display name",
      "refresh_token": "opaque secret"
    }
  ]
}
```

The bundle contains no access token, proxy, application config, cache, download
path, or machine metadata. `username` is display metadata; token validation
supplies the authoritative current identity during import.

Bundle parsing validates schema, version, positive and unique declared UIDs,
non-empty tokens, a valid default reference, unknown structural conflicts, and
duplicate object keys at every nesting level before a local write. Unknown
future versions fail explicitly.

Bundle import is an offline restore, not a series of OAuth imports. After fully
validating the bundle, the SDK merges all declared accounts into a copy of the
local store and saves that copy once through the auth-store atomic writer. It
does not refresh tokens or reinterpret declared UIDs, so there is no partially
rotated state or UID mismatch. Existing UIDs are replaced, new UIDs are added in
bundle order, and accounts not present in the bundle remain unchanged.

If the destination already has a default, it is preserved. Otherwise the valid
source default is restored. A bundle with no accounts is rejected. Bundle parse,
validation, destination-load, and pre-commit save failures leave the destination
store unchanged and return a non-zero exit status with stdout empty. If file
replacement commits but cleanup or durability synchronization then fails, the
operation returns a typed `committed` local-write outcome instead of claiming a
rollback; callers must reload and inspect the destination store.

On success, text mode prints one redacted status line per restored account and
the resulting default UID. `--json` prints one object with `accounts` and
`default_user_id`:

```json
{
  "accounts": [
    {
      "user_id": 12345678,
      "username": "display name",
      "status": "added"
    }
  ],
  "default_user_id": 12345678
}
```

`status` is `added` or `updated`. The report never contains a token.

An exported bundle is a point-in-time credential backup, not live multi-machine
sync. It can be restored repeatedly only while its tokens remain valid. Any
later OAuth refresh on any machine may rotate a token and make older stores or
bundles stale; users should re-export after authenticated use when they require
an up-to-date backup. Import never claims to keep the source machine usable.

## Agent Contract

All auth commands are Agent-usable. The product skill must not contain a
command-level prohibition for import or export.

When a user explicitly provides a refresh token and requests import, an Agent
may invoke the positional form. It must not repeat the token in prose, persist
an extra copy, or display it after the command. This convenience mode cannot
erase the copy already present in the conversation or tool-call record, and the
positional value may also be visible in argv, shell history, or process tools.

When the token has not already been disclosed, the skill should prefer hidden
terminal input, a secret-manager pipeline, `--output`, or an opaque export-to-
import pipeline. An Agent may execute stdout export without a consumer only
when the user explicitly asks to display or receive the raw token. Otherwise it
must connect stdout to the intended consumer in the same command or use
`--output`, so the secret is not copied into the conversation by default.

## Architecture

- `internal/cli` owns Cobra parsing, terminal/stdin selection, file flags, and
  human/JSON presentation.
- `internal/application.AccountService` exposes import and export use cases but
  never reads auth storage directly.
- The top-level `pixiv` SDK remains the only local-account authority. Existing
  `ImportAccount` and `ExportAccountRefreshToken` operations remain stable for
  SDK consumers; coherent local bundle snapshot and offline restore operations
  are added to avoid CLI/application access to `internal/storage/auth`.
- `internal/storage/auth` owns bundle-independent private atomic storage. A
  dedicated SDK-facing codec owns the public versioned bundle format so storage
  schema changes do not silently redefine exported files.
- CLI and MCP do not call App OAuth or storage adapters directly.

The single-account import path reuses the existing validation, identity lookup,
token rotation, and upsert behavior. Bundle export takes one coherent local
snapshot. Bundle restore validates and atomically merges one local snapshot
without OAuth, then produces a structured non-secret report.

## Errors and Secret Handling

- Cookie-shaped input remains rejected before any network call.
- Invalid token, proxy, network, OAuth, malformed bundle, permission, missing
  account, and existing output-file errors expose their real redacted cause.
- No error includes positional input, stdin content, bundle token content,
  rotated tokens, private file contents, or secret-bearing URLs.
- Export-to-stdout bypasses ordinary success logging and automatic update
  checks so stdout contains only the requested secret payload.
- Import/export operations do not introduce retry limits, fixed request
  truncation, silent fallback, or hidden success.

## Testing

TDD starts with failing focused tests for:

- Auth help contains `import` and `export`, and no longer contains `add`,
  `token`, or `--token`.
- Positional, hidden prompt, automatic non-TTY stdin, `--file`, and `--file -`
  import paths; all mutual-exclusion and empty-input errors.
- Agent-style positional import does not echo a supplied token in stdout,
  stderr, JSON, errors, or logs.
- Import validates identity, stores the rotated token, upserts by UID, and
  preserves/defaults accounts according to this design.
- Default/explicit/all export, exact stdout/stderr, stable ordering, offline
  behavior, no environment override, no auth-file mutation, and no automatic
  update request.
- Bundle schema/version validation, duplicate UID handling, offline atomic
  merge, default restoration/preservation, repeat restore, and rollback on
  parse, validation, or save failure.
- Raw-token stdin accepts one optional terminal `LF`/`CRLF`, rejects extra
  lines, and round-trips exact export output without general whitespace trim.
- Private output mode, race-free no-clobber writes, atomic forced replacement,
  cleanup, `--force`, and platform-appropriate permission behavior.
- Public SDK/application boundaries and typed redacted failures.
- Final binary black-box workflows, including direct Agent import and an
  export-to-import pipeline using isolated HOME/XDG fixtures.

Required regression gates are focused tests during red/green/refactor, followed
by `go test ./... -count=1`, `go test -race ./... -count=1`, `go vet ./...`, the
build script, pre-commit, and `git diff --check`. Real Pixiv OAuth validation is
opt-in and must never print a real token.

## Documentation and Release

Update the English, Simplified Chinese, and Japanese README/CLI reference auth
sections, maintainer development documentation, affected ADR wording, product
skill and references, `AGENTS.md`, the maintainer review checklist and
architecture's single secret-output exception, and `[Unreleased]` CHANGELOG.
Only single-account `auth export [UID]` and all-account `auth export --all`
without `--output` may write secret payloads to stdout; every other stdout,
stderr, JSON, MCP, log, and error path remains redacted. The change is
documented as a `v0.4.0` breaking CLI rename/removal. No knowledge graph is
generated.
