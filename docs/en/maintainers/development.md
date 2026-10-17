# Development

English | [简体中文](../../zh-CN/maintainers/development.md) | [Documentation index](../../index.md)

| What you want to do | Start here |
| --- | --- |
| Check the local toolchain | [Environment check](#environment-check) |
| Build or verify ugoira encoding | [Ugoira Go encoder](#ugoira-go-encoder) |
| Run the CLI and MCP | [Run](#run) |
| Handle login and credentials | [Obtain refresh token](#obtain-refresh-token) |
| Choose test scope | [Tests](#tests) |
| Check the release workflow | [Release gates, signing and Homebrew boundaries](#release-gates-signing-and-homebrew-boundaries) |
| Prepare release notes | [Release notes and publication](#release-notes-and-publication) |

## Environment check

The project is a Go module. `go.mod` is the single source of truth for the required Go toolchain version.

Before starting, verify the declared Go toolchain and standard test environment:

```bash
go version
go env GOVERSION CGO_ENABLED GOOS GOARCH
CGO_ENABLED=0 go test ./...
```

## Ugoira Go encoder

The default downloader constructs `ugoira.NewEncoder()` and produces GIF/APNG in Go, without `ffmpeg`. PNG frames use Go's standard decoder; JPEG frames use the owner-local `internal/media/ugoira/jpeg` decoder.

The 512 MiB per-frame bound applies separately to ZIP member source bytes and the pre-decode allocation estimate, inheriting the pinned Rust `image` crate's `Limits::default().max_alloc`. Declared source sizes above this bound fail before reading; actual expanded sizes above it fail during chunked reading. Invalid or overflowing dimensions and decode estimates above the bound also return errors, without truncation or a backend fallback. Cancellation is checked before and after each read chunk and before and after image decoding. The PNG/JPEG decoding loops have no cancellation callback, so cancellation during a single-frame decode is observed when that decode returns.

Source builds use the Go version declared in `go.mod`. `scripts/build.sh` explicitly sets `CGO_ENABLED=0` and writes `build/pixiv` (`build/pixiv.exe` on Windows). No Rust toolchain, native library or C linker is needed:

```bash
sh scripts/build.sh
CGO_ENABLED=0 go test ./internal/media/ugoira/... -count=1
```

Only race-detector tests need `CGO_ENABLED=1` and a working C toolchain. The encoder has no backend selector or runtime fallback.

`scripts/build-platform.sh` owns an exact-target CGO0 build and optional canonical archive. It accepts `--target GOOS/GOARCH`, `--output-dir DIR`, optional `--version VERSION` (without `v`) and `--package` (requires a version). It retains Windows executable suffixes and the Linux ABI gate. Runner/capability metadata stays in `ci/platforms.json`; tests, source identity, permissions, approvals and publication stay in workflows.

The public ABI baseline for Linux releases is glibc 2.35. Linux runners for release test/production, native evidence, packaged-binary smoke and the Homebrew install matrix must be pinned to `ubuntu-22.04` and `ubuntu-22.04-arm`; quality, validate, publish and other jobs that do not produce Linux binaries may continue to use newer runners. After every Linux executable is produced, the following must be run:

```bash
go run ./scripts/cmd/linuxabi --binary <linux-elf>
```

This gate reads the real loader contract from the ELF `SHT_GNU_verneed` and also checks imported symbol versions; any dependency above `GLIBC_2.35` fails before packaging. Relying solely on "the binary runs on the build runner" is not acceptable, because that lets the runner's own newer glibc hide backward-compatibility regressions.

The release archive's `LICENSE`, `THIRD_PARTY_LICENSES.md` and `third_party/licenses/**` retain `text eol=lf` in `.gitattributes`, keeping archive audits and byte-for-byte license checks stable on Windows.

### Historical Rust provenance

The removed libraries came from native run `29567721284`, superseding the initial collection in run `29192425899`. The historical compiler versions were Rust `1.96.0` for `x86_64-apple-darwin` and `x86_64-pc-windows-msvc`, and Rust `1.96.1` for `aarch64-apple-darwin`, `aarch64-pc-windows-msvc`, `x86_64-unknown-linux-gnu` and `aarch64-unknown-linux-gnu`. Their sources, vendor tree, libraries and manifest remain in Git history; they are not inputs to current builds. Independent ported-source notices remain below.

### License bundle

`go run ./scripts/cmd/licensebundle` unions the production modules selected by `go list -mod=readonly -deps -json ./cmd/pixiv` on the fixed six release targets with `CGO_ENABLED=0`. Resolution is offline (`GOPROXY=off`, `GOSUMDB=off`) and ignores local workspace/flag overrides; the required module sources must already be cached. It does not use Cargo or include test/tool-only modules that are absent from the production graph. `--check` compares both the index and the complete generated text tree without publishing changes.

`third_party/attributions.json` pins the additional ported-source notices. Their maintained texts live in `third_party/attributions/`, separately from generated `third_party/licenses/`; selected MIT texts, the Go BSD/patent grant, and the SunPro notice remain after Rust source removal. The version-specific `fhttp v0.6.8` entry supplies the Go BSD text referenced by its source headers because that module archive omits a standalone license file. Missing or empty texts fail generation before publication. Nested component notices are included; CRLF is normalized to LF for the archive's existing checkout contract, with the full text retained. The index and text tree retain their existing rollback-safe publication and release archive member contract.

### Native runner evidence

`.github/workflows/native-evidence.yml` remains an explicit maintenance entry point requiring `refs/heads/main`, not an ordinary main-push gate. It retains empty global permissions, job-only `contents: read`, full-SHA actions, credential-free checkout and no publishing/signing environment. The registry's `native-evidence` capability selects runners. Each runs the Go license check, `scripts/build-platform.sh`, and Go GIF/APNG regressions, then uploads a versioned binary/archive record.

Schema 3 binds the workflow's exact `source_commit`, target, binary version and binary/archive/member SHA-256 values. It executes `--version` and checks the archive's binary, root notices and complete license tree. It rejects old schemas and Rust source-digest/staticlib fields. `consolidate` rechecks exactly six complete, same-source, same-version bundles, rejects symlinks and unsafe file/directory paths, and atomically publishes a new directory with per-target bundles and `summary.json`. It does not backfill libraries into source.

Set `RUN_ID` and `SOURCE_COMMIT` from one audited native-evidence run, download its six artifacts, and use new absolute paths:

```bash
go run ./scripts/cmd/nativeevidence consolidate \
  --repo-root . --expected-version "v0.1.0-native-evidence.${RUN_ID}" \
  --expected-commit "$SOURCE_COMMIT" --input-dir /absolute/evidence \
  --output-dir /absolute/new-complete-evidence
```

PR Platform workers also upload schema 3 `native-evidence-<goos>-<goarch>` bundles after native and unpacked-binary smoke, at `v0.2.0-ci.<run-id>`. Their producer remains the trusted base-ref workflow: editing a PR-head workflow does not update that trusted producer. Inspect the actual run and exact head; never splice runs or treat a fixture, cross-build or workflow definition as six-platform execution.

`.github/workflows/browser-evidence.yml` is another explicitly dispatched, credential-free native provider contract matrix: it runs platform code and synthetic-fixture regression for `internal/browsercookies/...` on macOS, Linux and Windows amd64/arm64 runners without attaching another matrix to ordinary `main` pushes. GitHub Windows runners do not provide the required `sqlite3` CLI, so both Windows jobs provision the architecture-matched official SQLite 3.53.4 tools archive through `scripts/install-browser-sqlite.ps1`, with fixed URLs and SHA-256 verification before the existing SQLite preflight runs. A focused test-only workflow contract covers the credential, pinned-action, fixture, pinned SQLite provisioning and cleanup boundaries without reimplementing the workflow in production code; `scripts/cmd/browsernativeevidence firefox-contract` remains the runtime helper that exercises the isolated Firefox profile/schema. The `firefox_native` job only unpacks the official package in the runner's temporary directory, lets Firefox generate an isolated profile/schema, then injects explicit synthetic cookies to exercise the provider contract; it does not read user browser profiles, Keychain, DPAPI or Secret Service, nor upload package/profile/database. Real profile/session evidence can still only be obtained on the protected release-prep host; the success of this workflow must not be treated as real user-browser import success.

## Run

Build:

```bash
sh scripts/build.sh
```

The default output is `build/pixiv` or `build/pixiv.exe` for the current platform. Run on Windows via Git Bash, MSYS2 or WSL; for cross-builds continue to use `go build` directly.

Run the CLI:

```bash
pixiv auth login
pixiv search "初音ミク" --json
pixiv download 123456
```

Run MCP stdio:

```bash
pixiv auth use 12345678
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./build/pixiv mcp
```

MCP uses the Pixiv account selected by local `auth use`; refresh token is not a config-file or environment-variable input.

If your network environment requires a proxy, you can additionally set:

```bash
https_proxy=http://127.0.0.1:7890 ./build/pixiv mcp
```

Or override the proxy only for this launch:

```bash
./build/pixiv mcp --proxy http://127.0.0.1:7890
./build/pixiv mcp --no-proxy
```

The CLI's credentials, configuration, callback bridge, release check cache and callback helper all live under the current user's home directory in `.pixiv-cli`.

### Local paths and permissions

- macOS/Linux: `~/.pixiv-cli`; Windows: `%USERPROFILE%\.pixiv-cli`.
- Account credentials are stored in `pixiv-cli.db` (SQLite; the account key is the Pixiv UID / FANBOX UID).
- Embedded migrations advance the database through schema v3. Legacy v1 databases are upgraded in place; when the initial schema already contains a later column, the corresponding migration is recorded without replaying duplicate DDL. Unknown newer schemas remain fail-closed.
- Global configuration is stored in `config.toml`.
- Unix-like systems actively use `0700` parent directories and `0600` files; Windows inherits the parent-directory ACL on first creation, preserves the existing ACL when replacing an existing target, and does not actively tighten or relax the DACL.

> [!WARNING]
> New versions do not automatically read or delete the legacy `auth.json`. Cross-version migration must run `pixiv auth export --all --output <private bundle>` on the old version, then use shell redirection or a pipe to run `pixiv auth import < bundle.json` on the new version.

### Login methods

The recommended path is `pixiv auth login` via a local loopback server and browser OAuth. When the server is configured with both `login_relay_public_url` and `login_relay_listen_addr`, it emits a one-time remote handoff URL and directly hands off to the installed pixiv-cli desktop handler to complete login, without rendering an intermediate project page or manual callback form.

Other login entry points:

- An existing raw token can be entered via `pixiv auth import`.
- Account backups use `auth export` and `auth import < bundle.json`.

### Configuration management

`pixiv config path/get/set/unset` manages `account_pool_enabled`, `account_pool_strategy`, `download_path`,
`filename_template`, `directory_template`, `request_interval`, `https_proxy`, `log_level`, `log_format`,
`reverse_search_provider`, `reverse_search_pixiv_only`, and `saucenao_api_key`.
Other advanced TOML is maintained by the user by hand. In particular, reverse-search transport and challenge
recovery live in `[reverse_search.network]` and `[reverse_search.flaresolverr]`; these tables are not generated by the
baseline config and are read as a startup snapshot. The first configuration bootstrap is driven by the
**struct field tags** in `internal/config/settings` and generated with `tomledit`; it is compact, includes only
entries marked `example:"true"`, and never overwrites an existing file.

Adding one **ordinary setting** (manageable via `config get/set/unset`, or readable only from `config.toml` /
the environment) takes three steps:

1. Add the field and its declaration to `RuntimeConfig` (or a nested config group) in
   `internal/config/settings/config.go`, following the tag contract linked below.
2. Add one focused behavioural test (defaults, source, visibility, or a schema error).
3. Update the matching documentation.

There is no registry, environment `switch`, per-field `Runtime()` assignment, baseline list, or CLI alias list left
to keep in sync. See the [configuration declaration section of the architecture doc](architecture.md#configuration-declaration-struct-field-tags-are-the-single-source-of-truth)
for the full tag contract and per-file responsibilities.

> [!NOTE]
> The removed `[web] fallback_enabled` returns `removed_setting` if it still exists; clean it with `pixiv config unset web_fallback_enabled`. `[logging].level` (`info|debug`) and `[logging].format` (`text|json`) are live startup settings; `PIXIV_LOG_LEVEL` and `PIXIV_LOG_FORMAT` override them.

### Flag parsing

The CLI uses Cobra/pflag, and flags may appear before or after positional arguments; for example, both `pixiv auth check 12345678 --json` and `pixiv search "初音ミク" --json` are supported.

The Pixiv command proxy, `[pixiv.network]`, environment variables and `[network]`, the FANBOX-independent
`[fanbox.network]`/`[fanbox.flaresolverr]` configuration, and the reverse-search
`[reverse_search.network]`/`[reverse_search.flaresolverr]` configuration are all resolved per their own service
boundaries. Reverse search has separate standard source/SauceNAO, ascii2d browser, and FlareSolverr JSON-control
surfaces; FlareSolverr is used only for challenge recovery.

## Obtain refresh token

Browser cookies (including `refresh_token=...`, `PHPSESSID`, `device_token`) are not acceptable Pixiv App API OAuth refresh tokens; the CLI, MCP, environment variables, SDK and stored accounts all reject such input. The recommended path is to log in directly and save the account:

```bash
pixiv auth login
```

| Item | Description |
| --- | --- |
| Local service | The CLI generates PKCE/state and starts a local loopback HTTP server. |
| Browser | A normal CLI launch on macOS and Windows prepares the current user's persistent `pixiv://` callback helper; local login opens the default browser, so an existing Pixiv login session can be reused; `--no-open` switches to only printing the login URL. |
| Callback reception | The CLI receives the loopback callback for this round, the one-time desktop handoff and the local page form. Remote handoff does not offer manual callback backfill. |
| State validation | The local loopback callback must match the state for this round; the official Pixiv callback URL and `pixiv://account/login` serve as explicit fallbacks when Pixiv does not return a state. |
| Token storage | The refresh/access token is not printed; the refresh token is written to `pixiv-cli.db` keyed by Pixiv UID. The legacy `auth.json` is not part of the new CLI's read, migration or deletion paths; cross-version migration must be an explicit bundle export by the old CLI and an explicit import by the new CLI. Unix-like systems actively use `0700` parent directories and `0600` files; Windows inherits the parent-directory ACL on first creation, preserves the existing ACL when replacing an existing target, and does not actively tighten or relax the DACL. |

For local login, the active loopback bridge preferentially receives the `pixiv://account/login?...` returned by Pixiv and hands the callback off to this round's CLI listener; after the OAuth exchange completes, the browser shows a fixed result page. For cross-machine login, the server only shows a one-time handoff URL after starting; once the browser opens, it directly hands off to `pixiv://account/remote-login`, the local machine claims this OAuth URL, and the callback is relayed back to the same session; only this handoff state is saved locally, and a new handoff replaces the old state. The remote flow requires the installed CLI desktop handler and does not offer mobile manual backfill. The server verifies that the submitted content belongs to this session and is an official callback; when Pixiv carries a state it must match, and then this round's PKCE verifier completes the exchange. `pixiv auth devices` has been removed; any existing `remote-devices.json` are silently ignored. Both HTTP and HTTPS can be used for the relay; direct TLS and same-host TLS reverse proxy are both supported. The legacy `login_relay_secret` and `login_relay_target_url` configuration are silently ignored.

The system proxy used by the browser is not automatically forwarded to the Go CLI. `https_proxy`, `--proxy` and the update path all accept `http`, `https`, `socks5`, `socks5h` URIs. If Pixiv token exchange needs a proxy, configure `pixiv config set https_proxy socks5h://127.0.0.1:7890`, set `https_proxy=...` before a single command, or use the runtime override `--proxy socks5h://127.0.0.1:7890` for network commands. `--no-proxy` clears the proxy for this command even if `https_proxy` is set via environment variable or `config.toml`; `--proxy` and `--no-proxy` cannot be used together and are not written to `config.toml`. Request pacing is configured with `PIXIV_REQUEST_INTERVAL` or `[network].request_interval`. Debug diagnostics are configured with `pixiv config set log_level debug` and optionally `pixiv config set log_format json`; they are stderr-only and startup-scoped.

The network entry points currently supporting the proxy override are direct-token `auth import`, `auth login`, `auth check`, `search`, `timeline`, `detail`, `ranking`, `recommended`, `download` and `mcp` startup. Bundle-form `auth import` explicitly rejects the proxy flag; `auth export/list/use/remove` and `config path/get/set/unset` do not accept these flags.

### Auth import/export

> [!WARNING]
> The following secret boundaries are hard constraints: the token may only be written to stdout by an explicit `pixiv auth export` without `--output`; no other path may expose the secret; the bundle is an unencrypted, secret-bearing point-in-time backup, not a live sync.

`pixiv auth import [REFRESH_TOKEN]` validates the input via App OAuth and saves the rotated token. A positional argument ends up in argv/shell history; a no-argument TTY uses hidden input, a no-argument non-TTY reads the full stdin, and the first non-whitespace byte automatically distinguishes a raw token from a versioned bundle. The bundle is strictly decoded, fully offline-merged and atomically written back; on failure it must not fall back to OAuth, and it conflicts with a positional token, `--proxy` and `--no-proxy`. Restore preserves the existing default; the bundle default is adopted only when no local default exists yet.

`pixiv auth export [UID]` selects the default account when UID is omitted; without `--output` it writes only the raw token and a newline to stdout. `pixiv auth export --all` without `--output` writes only the versioned secret bundle to stdout. Both are the only secret-stdout exceptions; both only read the local store, do not refresh, do not go online, do not modify state, and skip startup pending-update cleanup and automatic update. `--output PATH` always writes a bundle, refuses to overwrite by default, and only `--force` allows replacement; stdout is only a path/account-count summary. Other stdout, stderr, JSON, MCP results and errors still must not expose secrets.

The bundle is an unencrypted, secret-bearing point-in-time backup, not a live sync; after token rotation, old bundles and copies on other machines may be stale. Any target export writer uses `0600` files on Unix-like systems and does not change the existing parent; on Windows it explicitly sets the owner and protected DACL, authorizing only the current user, LocalSystem and builtin Administrators. The Windows behavior has CI tests, and subsequent acceptance can be cross-compiled locally; this does not claim execution on a real Windows host.

When an atomic restore write fails, inspect the public `LocalWriteCommitOutcome`: pre-commit is `not_committed`; a durability/cleanup failure after replacement is `committed` and must be reloaded to confirm; an indeterminate recovery state is `unknown` and must be manually verified. `committed` or `unknown` must not be described as a successful rollback.

Real login depends on the Pixiv OAuth web flow being available. Automated tests use a fake OAuth server to cover callback and token exchange, and do not access real Pixiv.

## Tests

Current test coverage spans CLI commands and build metadata, explicit/automatic updates, `internal/services/{pixiv,fanbox}/account` account services, `internal/services/pixiv/pool` account pool, `internal/services/reversesearch` source/provider fixtures and aggregation, `internal/storage/database` auth storage and `internal/config/settings` configuration, `internal/shared/lifecycle` lifecycle, `internal/shared/pagination` logical pagination, `internal/shared/traversal` generic reentrant traversal, Pixiv App API auth retry, the public SDK (`sdk`/`sdk/pixiv`/`sdk/fanbox`), HTTP client wiring, download management, the Go encoder/native-evidence contract and `internal/mcpserver/{pixiv,fanbox}/tools` tool registration. `internal/account` and `internal/session` have been deleted and no compatibility test entry points are retained. Test file layout and same-package exceptions follow [Test file layout](#test-file-layout):

```bash
go test ./...
sh scripts/build.sh
# Offline fixture/crypto/permission-classification regression for the browser provider; real cross-platform host evidence is run separately on release-prep.
go test ./internal/browsercookies/... -count=1
# Real SDK e2e requires local credentials (Pixiv reads the selected account from the local pixiv-cli.db, FANBOX reads the Keychain):
PIXIV_E2E_READ_USER_ID=<secondary-uid> \
PIXIV_SDK_E2E=1 go test ./e2e -run TestRealPixivSDKRead -count=1 -v
FANBOX_E2E_CREATOR_ID=<non-secret-creator-id> FANBOX_E2E_TAG=<non-secret-tag> \
FANBOX_E2E_POST_ID=<non-secret-post-id> FANBOX_E2E_POST_URL=<non-secret-post-url> \
FANBOX_SDK_E2E=1 go test ./e2e -run TestRealFanboxSDKRead -count=1 -v
# If native requests trigger a real challenge, recovery can be additionally and explicitly enabled; it is not configured by default.
FANBOX_E2E_SOLVER_URL=http://127.0.0.1:8191 \
FANBOX_E2E_SOLVER_PROXY=http://host.docker.internal:7890 \
FANBOX_E2E_CREATOR_ID=<non-secret-creator-id> FANBOX_E2E_TAG=<non-secret-tag> \
FANBOX_E2E_POST_ID=<non-secret-post-id> FANBOX_E2E_POST_URL=<non-secret-post-url> \
FANBOX_SDK_E2E=1 go test ./e2e -run TestRealFanboxSDKRead -count=1 -v
# Single-post post.info acceptance; only requires a post id/page URL and allows a legitimate zero-file resource summary.
FANBOX_E2E_POST_ID=<non-secret-post-id> FANBOX_E2E_POST_URL=<non-secret-post-url> \
FANBOX_SDK_E2E=1 FANBOX_E2E_POST_ONLY=1 go test ./e2e -run TestRealFanboxSDKPostInfo -count=1 -v
# Explicit reverse-search upstream compatibility observation; never run by default.
# Pre-export SAUCENAO_API_KEY from a private environment; do not inline it.
export SAUCENAO_API_KEY
PIXIV_REVERSE_SEARCH_E2E=1 \
PIXIV_REVERSE_SEARCH_SOURCE=<private-test-image-path-or-url> \
PIXIV_REVERSE_SEARCH_PROVIDER=all \
go test ./e2e -run TestRealReverseSearch -count=1 -v
```

There is no separate E2E wrapper script. Real network observation runs the `go test` commands above directly, so the
offline suite and the credentialed observation share one orchestration path.

`go test ./...` stays offline-stable by default; real SDK e2e is skipped when `PIXIV_SDK_E2E=1` or `FANBOX_SDK_E2E=1` is not explicitly set. Once explicitly enabled, missing local authorization credentials or a missing non-secret FANBOX target fails directly and exposes the gap, rather than disguising a skip as release evidence.

Reverse-search provider fixtures and CLI/MCP/config regressions remain part of the offline suite. Real reverse-search
network observation is separate and runs only when `PIXIV_REVERSE_SEARCH_E2E=1` is explicitly set; the source is
required, and SauceNAO or `all` additionally requires `SAUCENAO_API_KEY` while ascii2d-only runs do not. The test
takes no source or key arguments, does not echo either value, and must be run only with an authorized test image.
It observes third-party compatibility, not a default release gate; a skipped or unavailable upstream must not be
reported as a successful real-network result.

The separate `TestRealReverseSearchMCPReusesSolverSession` check additionally requires
`PIXIV_REVERSE_SEARCH_SOLVER_URL` when explicitly enabled; `PIXIV_REVERSE_SEARCH_SOLVER_PROXY` is optional and denotes
only the FlareSolverr browser upstream proxy. It does not proxy solver control requests or the native ascii2d image
upload, and neither solver session state nor the source is persisted as test evidence.

The `PIXIV_SDK_E2E=1` / `FANBOX_SDK_E2E=1` commands above only select the current public SDK E2E tests: the Pixiv test reads the selected account from the local `pixiv-cli.db`, and the FANBOX test reads `FANBOXSESSID` from the agreed macOS Keychain item. `PIXIV_E2E_READ_USER_ID` is an optional non-secret local-account selector: when supplied it must name a stored Pixiv account and fails closed before network access if malformed or missing, with no fallback to the configured default; release evidence should explicitly select a secondary account. Omitting it preserves the configured-default compatibility behavior. The FANBOX `FANBOX_E2E_CREATOR_ID`, `FANBOX_E2E_TAG`, `FANBOX_E2E_POST_ID` and `FANBOX_E2E_POST_URL` only accept explicit, non-secret test targets; refresh tokens, sessions or full cookies are not accepted as arguments or environment variables. The optional `PIXIV_E2E_PROXY` only denotes a non-secret proxy URI; `FANBOX_E2E_SOLVER_URL` and `FANBOX_E2E_SOLVER_PROXY` are optional non-secret recovery topology configuration and the solver is not enabled by default. When real E2E is not explicitly enabled, tests skip by default; when explicitly enabled but missing local credentials or FANBOX targets, they fail, and a default skip or automatic discovery must not be recorded as release evidence.

The real SDK E2E for v1 is `TestRealPixivSDKRead` and `TestRealFanboxSDKRead` (see the `PIXIV_SDK_E2E=1` / `FANBOX_SDK_E2E=1` commands under [Tests](#tests)). The Pixiv test process only reads the refresh token of the selected account from the local `pixiv-cli.db`; release-prep should set `PIXIV_E2E_READ_USER_ID` to an authorized secondary account so the configured main/default account is not selected implicitly. It opens `sdk/pixiv` to verify identity and completes a stable detail/list and `Resource` read; the rotated credentials are first persisted via a normal repository transaction before continuing content requests. The FANBOX side directly reads the authorized `FANBOXSESSID` item from the macOS Keychain, and uses explicit creator/tag/post/page URL targets to verify `Creator`, `Creators`, `CreatorTags`, `CreatorPosts`, `TaggedPosts`, `Post`, `Home`, `Supporting`, `ResolveURL`, `OpenResource` and `SaveResource` one by one; list targets each follow up one continuation when the server returns a cursor, and post details must discover a file attachment and fully read it in a temporary directory. On session expiry, it explicitly reports `credentials_expired` and asks for re-import, without fallback. After release-prep runs, the operator scans stdout, stderr, test logs and evidence; tokens, cookies, signed URLs and raw response bodies must not end up in argv, environment dumps, logs, test names, artifacts or failure diffs. The above describes test coverage, not that real e2e has already been run; do not write tokens into shell history, logs or repository files.

For legitimate article details without a file attachment, `TestRealFanboxSDKPostInfo` is the supplemental test: it only requires an explicit post ID/page URL, verifies the public SDK's `Post`, a non-empty body, `ResolveURL` and the resource manifest, and allows `file_assets=0`; it cannot replace `TestRealFanboxSDKRead` for the strict resource path, which performs HEAD, full save and byte-count verification for every file attachment in the detail.

Under an explicit proxy, resource transport is fixed to negotiate HTTP/1.1, while App API and OAuth keep their original protocol negotiation. The resource read of this e2e is used to regress this resource-transport boundary; it does not add a fixed timeout for slow normal downloads. If Pixiv returns a 429 without a valid `Retry-After`, the real e2e retains the diagnostic and fails explicitly, without guessing a wait or retrying indefinitely.

`PIXIV_E2E_BINARY` and `PIXIV_E2E_EXPECTED_VERSION` let CI run offline e2e against a built, unpacked release binary; they do not inject tokens and do not enable the real Pixiv API. `platform-smoke.yml` builds, packages, unpacks and runs this set of CLI/config/MCP stdio verifications on six supported runners.

Before code changes are complete, tests should be added or updated according to the scope of the change. If tests cannot be run, the reason and risk must be stated in the delivery notes.

Release-related local fixture/policy gates also include:

```bash
sh scripts/test-build-platform.sh
sh scripts/test-package-release.sh
go test ./tools/release ./tools/platformmatrix -count=1
go test ./scripts/internal/nativeevidence -count=1
go test ./scripts/internal/browsernativeevidence -count=1
sh scripts/test-homebrew-formula.sh
git diff --check
```

The fixtures only prove format, failure semantics and local policy; they do not replace the real native GIF/APNG smoke, versioned archive content and Homebrew install acceptance of the six native runners.

`.github/workflows/ci.yml` owns only the read-only `Quality gate`, responding to `pull_request` and `workflow_dispatch` but no longer to tag pushes. Its change-scope classification uses the same trust model as `pr-metadata.yml`: it resolves the protected base branch's current tip, checks out `scripts/classify-change-scope.sh` and `.github/ci-change-scope.gitignore` from that tip, and then fetches the exact PR HEAD only for the diff range, so a PR cannot change its own skip decision; classifier failure fails closed instead of degrading to a skip. `.github/workflows/pr-metadata.yml` is the trusted `pull_request_target` coordinator: from the current base tip it validates the PR template and verification declaration, classifies the exact PR-head diff with `.github/ci-change-scope.gitignore`, publishes `PR template gate` / `PR commands gate` commit statuses, dispatches required smoke workers, and exposes `Platform smoke` / `Container smoke` as real job-level required checks without executing PR code in the coordinator jobs. Distinct `Platform smoke worker` / `Container smoke worker` Check Runs are only the trusted result bridge completed by the base-ref worker workflows; the required smoke jobs wait for those results. Plain scope patterns are documentation-only, `?pattern` requires Quality only, and `!pattern` requires Quality + Platform + Container; `pr-metadata.yml` is the smoke controller (it owns classification, both worker dispatches and the required gates), so it must use `!pattern`. Unneeded Quality and smoke jobs are native job-level `Skipped`, never synthetic success. Body-only PR metadata edits reclassify the unchanged head: required smoke jobs mirror its existing worker result, while genuinely unnecessary smoke jobs remain skipped, so editing the PR body cannot replace a failed smoke result with a bypassing skip.

PR state maintenance in `pr-metadata.yml` uses `pull-requests: write` for PR comments and does not request ordinary issue write access. Failed reads of the current PR or policy-state comments and failed comment creation, updates, or deletion retain the original GitHub API error and identify the failed request. Metadata failure still blocks the required smoke gates. The actual workflow-shell API fixture runs only on Linux to match the Ubuntu runner tools; the permission contract remains cross-platform.

Smoke gates query the exact PR head and worker name with `filter=all` and paginate matching checks, selecting the latest `started_at` with ID as the tie-breaker. Response order is not assumed. `filter=latest` can hide a worker from another check suite after a PR update; missing or failed workers still fail the gate.

The Platform worker resolves its six-platform matrix from the trusted workflow ref, then only the matrix jobs checkout the exact PR head and run with `contents: read`; a separate publisher job never checks out PR code and holds the minimal `checks: write` permission needed to complete the internal worker Check Run. The Container worker uses the same isolation model for Linux amd64/arm64. The six native jobs still run in parallel, including Windows root callback wiring and the native `loginhelper` contract, while container jobs remain parallel. Worker matrices do not appear as required PR checks; the corresponding PR gate job mirrors the aggregate worker outcome, and failures retain the worker details URL. Ordinary branch and `main` pushes run no CI. Stable `vX.Y.Z` tag pushes run only `release.yml` (the Quality gate no longer responds to tags, so Release solely owns the tag gates); Release itself performs the formal six-platform tests/builds and two-platform container verification, so the PR smoke matrices are not duplicated on the tag. The `dispatch` job in `pr-verification.yml` cheaply pre-filters with `contains(github.event.comment.body, '/test')` before allocating a runner: it is a loose superset of `tools/prmeta --check-trigger`, so it only yields a few false positives and never drops a legitimate trigger, while `tools/prmeta` remains the authoritative decision. Browser/native evidence remain explicit maintenance workflows. Real Pixiv/FANBOX SDK E2E does not enter regular PR CI; only the tag-publishing `release.yml` runs the credential-free SDK E2E contract gate after validate, and real SDK E2E is still accepted independently on release-prep in an authorized environment.

`scripts/tests/installers` verifies the installers using a local fake Release, fake `curl` and checksum fixtures, without accessing GitHub. The Unix job actually runs `install.sh`, covering SHA-256, directories with spaces, version preflight and not overwriting the old binary on verification failure; the Windows amd64/arm64 platform-smoke also runs `install.cmd` with real `cmd.exe`, `certutil.exe` and `tar.exe`, always passing `--no-path`, so the test does not modify the runner user's registry. The platform-smoke workflow runs this installer contract directly as part of the platform job; there is no separate runtime workflow-policy implementation.

The Windows `.zip` is produced by `7z` preinstalled on the GitHub runner image; other platforms continue to use `zip`. `scripts/test-package-release.sh` delegates the faked invocations to the real `7z` on the Windows runner, uses the `zip` fixture on other dev machines, and checks archive members; therefore a Git Bash missing `zip` is directly exposed at the release test gate. It uses MSYS's `winsymlinks:nativestrict` to create the checked links: if the runner cannot create native Windows links, the test explicitly fails, preventing Git Bash's plain-file pseudo-links from neutering the output-ancestor safety gate.

Quality also handles `pull_request.edited`, so retargeting a PR to `main` publishes its required Quality result. Title/body edits also trigger classification and any required Quality verification. The job keeps its literal required name, including documentation-only skips; skipped dynamic job names are not evaluated by GitHub. Documentation-only changes retain their real job-level skip.

### Offline ugoira parity

Ugoira encoder comparisons use the separate, opt-in `TestRealPixivUgoiraCapture`. It shares the selected local account
and credential-rotation persistence with `TestRealPixivSDKRead`, then calls public SDK `UgoiraMetadata` and
`SaveResource`. Choose positive comma-separated artwork IDs and an explicit `original` or `medium` quality;
an unavailable quality fails without substitution. Only enable this with authorization to download the samples.

```bash
PIXIV_UGOIRA_CAPTURE=1 PIXIV_UGOIRA_IDS=150808387,150247639,150356234 \
PIXIV_UGOIRA_QUALITY=medium PIXIV_UGOIRA_DIR="$PWD/build/ugoira-parity/samples" \
go test ./e2e -run '^TestRealPixivUgoiraCapture$' -count=1 -v
```

This command captures medium archives. The 1080p original in the corpus table below was acquired separately
from the documented `originalSrc` and saved in its own directory.

Each new ID directory contains the complete `frames.zip` and a `sample.json` with ordered filenames/delays,
actual quality, dimensions, duration, byte count, ZIP SHA-256 and frame-metadata SHA-256. The log also records the
manifest SHA-256. URL, resource ref, headers and credentials are not serialized. Existing samples are refused;
a failed capture removes only its newly created directory. Keep these private artifacts under ignored `build/`,
never commit them. `PIXIV_E2E_PROXY` has the same non-secret meaning as in the SDK E2E. Ordinary tests skip capture.

Compile the old baseline from its pre-migration source once, record its source commit, Go/build settings and binary
SHA-256, and keep that binary unchanged. The current test-only driver uses the same `ugoira.NewEncoder()` as the
default downloader and needs no credentials or network. Build it separately; never overwrite `baseline.test`:

```bash
mkdir -p build/ugoira-parity
CGO_ENABLED=0 go test -c -trimpath -o build/ugoira-parity/current.test ./e2e
GOMAXPROCS=1 RAYON_NUM_THREADS=1 \
UGOIRA_PARITY_SAMPLE="$PWD/build/ugoira-parity/samples/150808387" \
UGOIRA_PARITY_OUTPUT="$PWD/build/ugoira-parity/current-150808387.gif" \
UGOIRA_PARITY_FORMAT=gif UGOIRA_PARITY_MAX_EDGE=0 \
build/ugoira-parity/current.test -test.run '^TestUgoiraParityEncode$' -test.v
```

The driver requires absolute paths, a new output matching `gif` or `apng`, and a uint32 maximum edge (`0` means no
resize). It checks the ZIP and frame hashes before encoding and emits `UGOIRA_PARITY_RESULT` with the input hashes,
format, maximum edge, time spent in `Encode`, output bytes, Go version and `GOMAXPROCS`. Use a new output for each
trial and preserve the same inputs and runtime settings across implementations. Measure peak process RSS separately
with the same host tool; the logged time excludes compilation, acquisition, input hashing and process startup.
This fixture/driver is not itself proof of real-image quality, performance or six-platform parity.

When the toolchain changes, retain the frozen baseline and build a separately named legacy comparator from the same audited producer if needed. Verify its output against the frozen output and record both build modes; compare both implementations under the same toolchain. Migration resource acceptance uses a warmup and five alternating trials per real sample/format, with time, peak process RSS and output-byte ratios each at most 1.10. RSS is not Go heap usage.

Set `GOMAXPROCS=1` and `RAYON_NUM_THREADS=1` for every frozen-legacy/Go comparison. The legacy Rust quantizer's worker count can change output pixels as well as runtime. Match the fixed baseline's worker settings when selecting saved outputs for quality scoring.

`TestUgoiraSyntheticCorpus` writes the three original quality scenes plus transparency/repeated-frame, single-pixel
and odd-dimension cases. Without `UGOIRA_PARITY_CORPUS` it uses temporary directories; an explicit absolute directory
preserves schema-1 samples for both binaries. Existing sample directories are refused. `TestUgoiraParityInspect`
independently decodes full-canvas GIF frames and replays APNG Source/None rectangles, checking frame count, size,
loop, delays and GIF transparency. It requires unscaled output (`MaxEdge=0`). PNG inputs must reconstruct exactly;
JPEG pixel differences are reported separately because the reference uses Go's JPEG decoder. Inspection is outside
performance measurements and emits per-frame `UGOIRA_QUALITY_RESULT` with input/output hashes.

```bash
UGOIRA_PARITY_CORPUS="$PWD/build/ugoira-parity/synthetic" \
go test ./e2e -run '^TestUgoiraSyntheticCorpus$' -count=1
UGOIRA_PARITY_SAMPLE="$PWD/build/ugoira-parity/samples/150808387" \
UGOIRA_PARITY_OUTPUT="$PWD/build/ugoira-parity/baseline-150808387.gif" \
go test ./e2e -run '^TestUgoiraParityInspect$' -count=1 -v
```

RGB SSIM averages the three channel scores. MSSIMSimple uses non-overlapping 8×8 windows weighted by actual area,
including smaller edge windows; its covariance is a sum, not a population variance. The old `image` 0.25 CICP
conversion used by `DynamicImage::into_luma8` uses the BT.709/D65 matrix, float32 multiply-accumulate and rounding.
The metric preserves the measured arm64/FMA baseline; the older integer `Pixel::to_luma` helper is not that path.
`UGOIRA_PARITY_RUST_QUALITY` enables `TestUgoiraSSIMRustCalibration` against the old quality gate's saved
`<scene>-rust.gif` and `<scene>-ssim.csv`, with 1e-9 tolerance for its nine-decimal CSV.

The production Go encoder uses the owner-local `internal/media/ugoira/jpeg` decoder: Go 1.26.3's JPEG parsing with
the legacy zune-jpeg 0.5.15 integer IDCT and 14-bit BT.601 conversion. Standard-library IDCT rounding changed
palette selection enough to fail the fixed quality gate; matching only the color coefficients did not fix all
samples. This decoder does not register a global image format or add a module dependency. The parser's BSD license
is kept in that package; the zune-jpeg MIT attribution must remain in the final license bundle. The synthetic
17×9 `jpeg-{444,progressive,gray,restart}` fixtures and their legacy-decoded RGBA references cover these paths
without requiring Rust, FFmpeg or jpegtran at test time. The Go resize kernel uses bounded sine/cosine
polynomials with explicit `math.FMA`, fixed against the legacy runtime `sinf` results. Its 1,031 numeric
references in `testdata/lanczos-kernel.json` preserve Lanczos3 rounding without calling a platform math library.
The Go APNG writer uses fixed Sub prediction and an independent level-7 zlib stream per frame, retaining
RGBA8, lossless pixels and Source/None rectangle playback. Go 1.27's fast compression levels exceeded the original
10% output-size allowance on the high-texture corpus; the deterministic opaque-texture regression preserves the
same allowance against its frozen Rust reference. Compressed bytes may change with the toolchain; reuse prior
quality evidence only after proving identical frame controls and uncompressed pixels, not file size alone.

The migration corpus's six public artwork IDs are listed below. Archive dimensions were verified by decoding every
frame; artwork display dimensions alone are insufficient. The first original ZIP was explicitly provided by Pixiv's
public `ugoira_meta` `originalSrc`, with its complete frame list checked against authenticated SDK metadata; the
SDK had only offered medium for that artwork. This was a test-data acquisition step, not a product fallback.

| Artwork ID | Archive quality | Actual dimensions | Frames | Coverage |
| --- | --- | --- | --- | --- |
| 150808387 | original | 1920×1080 | 63 | High resolution, local motion, fine lines |
| 150247639 | medium | 423×600 | 8 | Complex illustration, broad changes |
| 150356234 | medium | 219×320 | 121 | Small canvas, long sequence, gradients |
| 150863948 | medium | 600×420 | 12 | Local motion, flat colors |
| 150863611 | medium | 600×338 | 37 | Gradients, texture, local motion |
| 150863218 | medium | 480×270 | 128 | Full-frame changes, dense texture |

Keep the original and medium archives in separate directories. Preserve full metadata and hashes locally;
never infer an original ZIP URL or relabel a medium sample. These sources do not replace later per-sample quality
and five-trial alternating old/new performance checks.


### Optional FFmpeg quality reference

Set `PIXIV_UGOIRA_QUALITY_FFMPEG=1` to compare the Go GIF encoder with a local FFmpeg executable. Without that exact opt-in, both tests skip before looking for FFmpeg or reading input files. The default command is `ffmpeg`; `PIXIV_UGOIRA_QUALITY_FFMPEG_BIN` selects another executable. An enabled comparison fails visibly if the executable is missing or fails; it never installs a tool or switches the production encoder.

```bash
PIXIV_UGOIRA_QUALITY_FFMPEG=1 \
PIXIV_UGOIRA_QUALITY_OUTPUT_DIR="$PWD/build/ugoira-parity/ffmpeg-synthetic" \
go test ./e2e -run '^TestUgoiraFFmpegSynthetic$' -count=1 -v

PIXIV_UGOIRA_QUALITY_FFMPEG=1 \
PIXIV_UGOIRA_QUALITY_ZIP=/absolute/path/to/frames.zip \
PIXIV_UGOIRA_QUALITY_FRAMES=/absolute/path/to/frames.json \
PIXIV_UGOIRA_QUALITY_CASE=public-sample \
PIXIV_UGOIRA_QUALITY_OUTPUT_DIR="$PWD/build/ugoira-parity/ffmpeg-public" \
go test ./e2e -run '^TestUgoiraFFmpegPublic$' -count=1 -v
```

The public test reads local files only. `frames.json` must contain the complete ordered `[{"file":"000000.jpg","delay":80}]` array, not a `sample.json` wrapper; extract its full `frames` array when using a captured sample. Quality input filenames must be flat, as in the old gate. The reference uses the original concat durations and `palettegen=stats_mode=full`/`paletteuse` filter, with generated extraction names so metadata cannot inject concat directives.

The three original synthetic scenes require each frame and the averages to meet the FFmpeg RGB/luma MSSIMSimple scores. Public inputs require the averages; both use the original `1e-12` comparison tolerance. GIF delta/transparent frames and disposal are replayed, both loops and frame counts are checked, and Go delays must match the metadata rounded up to centiseconds. This comparator's JPEG reference uses the legacy IDCT/14-bit color conversion, and 16-bit source channels retain the old rounding; alpha is dropped for the original RGB metric. The separate parity inspector's standard-library JPEG reference stays unchanged.

`QUALITY_RESULT` logs the case, frame count and four means. An output directory preserves `<case>-go.gif`, `<case>-go-ffmpeg.gif` and `<case>-go-ssim.csv` (columns `go_luma_ssim`, `ffmpeg_luma_ssim`, `go_rgb_ssim`, `ffmpeg_rgb_ssim`), including when a quality comparison fails. Choose a new directory for each trial; saved case names must be flat artifact names. The Go-run filenames are separate from historical `*-rust.gif`, `*-ffmpeg.gif` and `*-ssim.csv` calibration records, so those remain untouched. A public sample can fail this optional FFmpeg comparison even when its legacy encoder also did; retain the negative result and compare the same frozen legacy output before attributing it to migration.

### Test file layout

A production file `x.go` corresponds to at most one `x_test.go` in the same directory; platform-specific tests use `x_<platform>_test.go` and must have a real base owner. Tests for new owners always use the external test package (`X_test`); only the directories below are allowed to be same-package, because they observe unexported internal state. New same-package exceptions must be registered here with a permanent reason; otherwise they are treated as violations.

| Directory | Reason for same-package |
| --- | --- |
| `internal/media/ugoira` | Tests exercise `readFrameSource`, `frameSource`, `validateDecodedSize`, `frameNRGBA`, `decodeInto`, `resizeFrame`, and `apngWriter` directly to verify allocation boundaries and cancellation without huge allocations, preserve transparent RGB and ownership of the previous frame during buffer reuse, compare Lanczos3 pixels with fixed upstream reference vectors, exercise APNG sequence overflow and write failures, and check reservoir state, deduplication weights/indices, Oklab/Wu reference values, fixed-seed K-means centers and per-pixel dither indices. `colorGrid` tests verify exact candidate bounds at cell edges; `frameQuantizer` tests verify that reused error rows are cleared, ordinary quantization returns independent indices, and explicit buffer reuse starts only after the previous frame is consumed. LZW tests cover dictionary/code-width reset across frames and recovery after a write failure. GIF tests observe `gifWriter`/`gifBlocks` to verify global tables, streaming LZW sub-blocks and write failures; lifecycle tests observe `encodeGate` to coordinate queued cancellation without sleeps. Exposing these codec internals would expand the `Encoder` contract solely for tests. |
| `internal/config/settings` | `schema_test.go` passes synthetic types to the private `deriveSchemaFromTags` compiler to test declaration rejection without exporting reflection metadata or resetting the production cache. |
| `internal/cli` | The composition root test observes unexported root wiring, invocation lifecycle, and close ordering; these seams are not a public API. |
| `internal/cli/commands/pixiv/search` | Tests observe private searchArtworks logical-page continuation through a real SDK with an HTTP fixture. CLI/MCP wire contracts do not expose these cursors; exporting application internals only for tests would widen the public surface. |
| `internal/mcpserver/pixiv/tools/search_illust` | Tests observe private searchArtworks logical-page continuation through a real SDK with an HTTP fixture. CLI/MCP wire contracts do not expose these cursors; exporting application internals only for tests would widen the public surface. |
| `internal/browsercookies/chromium` | Tests construct the provider directly and inject an encryption key override, observing the unexported cookie record decryption path and profile discovery logic. |
| `internal/browsercookies/firefox` | Tests observe unexported profile discovery (`profiles.ini` parsing), cookie database path resolution, and record layout. |
| `internal/browsercookies/safari` | Tests directly call the unexported `parseBinaryCookies`, asserting binarycookies record layout. |
| `internal/browsercookies/secret` | Tests construct `SecretService{command: ...}` injecting unexported fields and assert unexported sentinel errors and command-output redaction behavior. |
| `internal/update/installer` | Tests inject the unexported `assetURLValidator` seam and checksum verification function, and use real fixture binaries to verify root `--version` preflight and that the old executable is not replaced on failure. |
| `internal/update/release` | `source_route_test.go` observes unexported source route selection and canonical API URL cache state; the rest of the directory already uses the external package. |
| `internal/storage/database` | Tests observe the unexported `tableInfoQuery` allowlist and migration-compatibility seams so SQL identifiers remain fixed literals and legacy schemas cannot silently bypass their contract. |
| `sdk/pixiv` | `cursor_test.go` observes unexported cursor construction and client-instance binding to verify exact query-bound invalid continuations without widening the public SDK surface. |
| `scripts/internal/browsernativeevidence` | Tests observe unexported environment probes and inject synthetic Firefox cookie seeds. |
| `scripts/internal/homebrewformula` | Tests directly call unexported formula rendering and version validation (`renderFormula`, `validateFormulaVersion`, `checkDynamicVersionNeeds`). |
| `scripts/internal/licensebundle` | Tests observe unexported `defaultBundleFileOps`, `modulesFromMetadata`, `packageLicenseFiles`, and license text normalization; executable Go fixtures verify the six-target offline command contract. |
| `scripts/internal/linuxabi` | Tests directly call unexported glibc version parsing and ABI comparison (`parseGLIBCVersion`, `checkImportedSymbols`). |
| `scripts/internal/nativeevidence` | Tests observe unexported record/consolidate state and the workflow security/ownership contract, covering schema 3, independent `source_commit`, exact binary `--version` output, six-target hash/archive verification, and mutation rollback. |
| `scripts/internal/publicapi` | Tests observe the unexported parser handling of `unexported`/`hidden` symbols and the golden comparison logic, using `writeFixture` to generate fixtures. |
| `scripts/internal/releaseassets` | Tests inject unexported `injectReleaseSources`/`injectWindowsReleaseSources`, observing asset archive naming (`archiveName`) and checksums generation. |
| `scripts/internal/releasenotes` | Tests observe unexported GitHub client call mappings, injecting a fake client to assert source auditing. |

There are currently no temporary items. This list does not accept open-ended phrases like "migration period" or "in the future". Adding a directory requires explaining the specific unexported symbol being observed and confirming that exporting a minimal interface is not a viable substitute; removing a directory requires a deletion task and test migration evidence (external package compiles + coverage unchanged).

`e2e/` is also `package X`, but it has no production code (a pure test carrier), so the "observing unexported production state" problem does not apply and it is outside the scope of this list. Cross-platform differences: test files with build tags (such as `scripts/internal/*`) have a different number of visible files under different `GOOS`/`GOARCH` combinations; when verifying, run `go list` separately with `GOOS=darwin`, `GOOS=windows`, and `GOOS=linux` to confirm the directory set is consistent.

```bash
# List all directories whose tests stay inside the production package (package X rather than X_test)
go list -json ./... | python3 -c 'import json,sys
dec=json.JSONDecoder(); s=sys.stdin.read(); i=0; same=[]
while i<len(s):
    try: p,i=dec.raw_decode(s,i)
    except json.JSONDecodeError: break
    while i<len(s) and s[i] in " \n\t": i+=1
    if p.get("Dir") and p.get("TestGoFiles") and not p["ImportPath"].endswith(("_test",)):
        same.append((p["ImportPath"],len(p["TestGoFiles"])))
for ip,n in sorted(same): print(ip,n)'
# Expected result = the Permanent directories above
```

### Capability scope

This is the maintainer-side authority for capabilities that **must not have a release-ready entry point** in v1. It is a negative contract: adding a shipped CLI/MCP/SDK entry point for any of these is a defect. An SDK-only migration seam tracked in the evidence-gated table is not release-ready and must not be treated as a completed capability. Schema-only placeholders or mock empty results are forbidden.

**Unsupported (explicitly not supported in v1; a new entry point is a defect):**

| ID | Unique owner | Current evidence | Close-out condition |
| --- | --- | --- | --- |
| `ART-SEARCH-RATING` | `internal/cli/commands/pixiv/search` + `internal/shared/searchfilter` + `sdk/pixiv` | CLI artwork search filters normalized `x_restrict` locally and binds the filter to its cursor; it does not send an upstream rating field. MCP `search_illust` has no standalone rating parameter | Preserve the local CLI contract and test cursor/filter consistency; exposing a new MCP parameter or upstream field requires its own supported behavior and synchronized schema/docs |
| `NOVEL-SEARCH-ADVANCED` | No owner (must not be added) | SDK/MCP schema has no advanced field | May be evaluated once the upstream contract appears; schema-only placeholders are forbidden |

**Evidence-gated (an SDK-only migration seam may exist; release-ready entry points still require the close-out condition):**

| ID | Unique owner | Current evidence | Close-out condition |
| --- | --- | --- | --- |
| `NOVEL-RANKING` | `sdk/pixiv` + `internal/cli/commands/pixiv/ranking` (T18/T30; MCP later) | SDK and CLI expose the additive `NovelRanking` seam over the internal `/v1/novel/ranking` adapter; CLI selects it explicitly with `--type novel`; MCP has no `novel_ranking` tool, and live/public release evidence is still incomplete | Complete the live second-page, shared cursor, MCP and release compatibility gates; until then this remains evidence-gated and is not `public_ready` |
| `NOVEL-BOOKMARK-MUTATION` | `sdk/pixiv` + `internal/mcpserver/pixiv` (G1-T13; CLI later) | SDK exposes additive typed `AddNovelBookmark`/`RemoveNovelBookmark`; MCP exposes `add_novel_bookmark`/`remove_novel_bookmark`; offline outcome, validation and no-replay evidence is present, while strict/live, read-back and release evidence remain incomplete | Complete strict/live mutation evidence, same-account read-back, cleanup and compatibility/release gates; until then this remains evidence-gated and is not `public_ready` |
| `COMMENT-WRITE` | `sdk/pixiv` (T16; CLI/MCP later) | SDK exposes namespace-specific `PostArtworkComment`/`ReplyArtworkComment`/`DeleteArtworkComment` and novel equivalents; MCP `comment_post`/`comment_add` directories remain = 0; response ID, read-back, cleanup, and strict live evidence are incomplete | After strict/live write evidence plus same-account read-back, cleanup, and T33/T38 compatibility gates; until then this remains evidence-gated and is not `public_ready` |
| `NOTIFICATION` | No owner | MCP `notification` directory = 0; SDK `Notification*` exports = 0 | Same as above |
| `AUTOCOMPLETE` | No owner | MCP `autocomplete` directory = 0; SDK `Autocomplete*` exports = 0; not merged into `search` | Same as above |
| `WEB-RESTRICTED-READ` | No owner | No `webapi` package; `web_fallback_enabled` is a tombstone key (`config get/set` → `removed_setting`) | Do not reopen the anonymous Web path; any proposal to restore Web/AJAX must first amend the AGENTS frozen contract and pass an ADR |
| `USER-BLOCK-MUTE-REPORT` | No owner | MCP `mute`/`report` directories = 0; SDK `BlockUser`/`MuteUser`/`ReportUser` exports = 0 | After the upstream provides a verifiable mutation contract |
| `WATCHLIST-MARKER` | No owner | MCP `watchlist` directory = 0; SDK `Watchlist*` exports = 0 | After the upstream provides it |
| `BOOKMARK-USERS` | No owner | No corresponding tool/SDK export; `bookmark_detail` only covers the current user detail | May be evaluated after the upstream provides it |
| `SPOTLIGHT-PIXIVISION` | No owner (out of scope) | MCP `spotlight`/`pixivision` directories = 0; SDK `Spotlight*`/`Pixivision*` exports = 0 | Explicitly out of scope; re-evaluate only when the product scope changes |

Adding or restoring an entry point for any capability above is a functional change: update the corresponding row here and synchronize the relevant user documentation. Reviewers re-run the negative grep / directory existence checks under each item manually.

## Release gates, signing and Homebrew boundaries

`.github/workflows/release.yml` is triggered by `v[0-9]*` tags by default: it first verifies SemVer, then runs the credential-free SDK E2E contract gate on the immutable tag to confirm that test entry points and the default skip/offline boundary have not been broken; only then does it build Go binaries for darwin/linux/windows × amd64/arm64 with CGO0, run Go tests (race steps separately enable cgo), check licenses and package the fixed-name archives. This workflow does not read or inject Pixiv/FANBOX credentials. Real SDK E2E, native browser and one-time solver acceptance must be completed in an authorized environment per the corresponding process on this page; the contract gate must not be treated as real release evidence.

`releaseassets finalize` also reads `scripts/install.sh` and `scripts/install.cmd` from the immutable tag, copies them into the Release under fixed names, and writes `checksums.txt` and the Ed25519 signature manifest alongside the six platform archives. The publish policy locks the finalize parameters and the complete eight-asset upload set; the Homebrew renderer also requires the checksum set to include both installers, but the formula still only downloads the platform-appropriate archive.

release.yml only accepts `v[0-9]*` tag pushes and no longer offers `workflow_dispatch`, a `release_tag` input or a test-only overlay. When a tag run fails, fix the cause on the default branch and re-run the normal immutable-tag release process; new verifiers, tests or production sources are not injected into an old tag from the default branch, and no manual recovery entry is provided for an existing Release. validate, test build, production build and publish are all bound to the same tag; the production build uses CGO0 from a clean tag tree on a separate runner and continues to use `git diff --exit-code` for byte-for-byte verification.

GitHub Release and the registries are separate systems and cannot commit atomically. `.github/workflows/publish-dockerhub.yml` is therefore the independent post-Release container publisher for both GHCR and Docker Hub. It receives the immutable handoff and both verified container artifacts from the completed Release run, checks the release tag and source identity, then publishes the same loaded image bytes to `ghcr.io/flanchanxwo/pixiv-cli` and `docker.io/flanchanxwo/pixiv-cli`; only Docker Hub authentication uses the protected `release` Environment secret `DOCKER_HUB_TOKEN`, passed through stdin. It never rebuilds the image. If registry publication fails, dispatch it from the default branch with only the original `release_run_id` so the same verified artifacts are reused. Exact-version tags are always published, an older stable recovery succeeds without changing `latest`, and only the newest stable release advances `latest`. No retry loop hides push failures.

### Container release verification

`build_container` runs after the shared `build` gate and beside `build_production`; it never waits for production assets to be rebuilt. The two native targets are `ubuntu-22.04` for `linux/amd64` and `ubuntu-22.04-arm` for `linux/arm64`. Each target checks out the immutable tag and builds a versioned CGO0 Linux binary through the Linux ABI gate, runs container packaging tests, builds a pinned glibc runtime image, verifies non-root execution, the exact version, `pixiv config path` under `/home/pixiv/.pixiv-cli/`, `/work`, and OCI provenance (`org.opencontainers.image.source`, revision, version, and licenses), then exports `verified-container-linux-amd64` and `verified-container-linux-arm64`. Build jobs receive only `contents: read`; only the independent container publisher requests `packages: write` when it consumes those artifacts after GitHub Release.

Focused maintainer checks are:

```bash
go test ./scripts/tests/containerrelease -count=1
go test ./tools/release ./tools/platformmatrix -count=1
```

The credential-free container smoke workflow builds both native architectures on relevant changes and executes version, non-root, state-path, and working-directory assertions; these local checks do not replace that CI evidence for a tagged release.

The shared platform runner/capability metadata lives in `ci/platforms.json` and is validated/emitted by `tools/platformmatrix`; release archive identity remains in `scripts/internal/releasecontract`. Workflow tests keep behavior and security boundaries rather than mirroring exact job counts, step positions or multiline shell text. Historical recovery plans and acceptance reports retain their original text and paths and do not serve as current process descriptions.

### Verifier source navigation

Release verification now favors behavior contracts over a second workflow-policy implementation. `tools/platformmatrix` validates the shared platform registry, while `tools/release` owns reusable release trust and artifact checks: immutable tag/default-branch ancestry, published Release state, Release-run handoff identity, exact archive/container sets, and checksums. Publisher workflows keep only their channel-specific credentials, permissions, change detection, and publication commands; recovery runs may verify a successful `Release` handoff without binding the workflow run head to the tag, while publishers whose existing contract requires that identity (such as Docker Hub) additionally bind the handoff run to the immutable tag commit.

`scripts/cmd/nativeevidence/` only dispatches `record` and `consolidate`. The owner package under `scripts/internal/nativeevidence/` stores the evidence schema, records single-runner evidence, validates and merges six-target results, audits archive members/hashes and enforces filesystem safety. Its workflow test covers only the credential/action/ownership boundary; it does not reimplement the workflow's exact YAML shape.

The release workflow remains the source of truth for ordering and permissions. Production archives, container images, prepared checksums, and Homebrew installation against the exact production archives all finish before `release-approval`; publication then uses the secret-bearing `release` environment and consumes the approved artifacts without rebuilding them.

The Go toolchain declared in `go.mod` does not support the race detector on Windows ARM64. The release matrix nevertheless executes the race gate on all six native targets: five targets run `go test -race ./...`, while Windows ARM64 must execute the same command and match Go's exact `-race is not supported on windows/arm64` diagnostic. Any other failure remains a failed gate, and no matrix entry is skipped. The test matrix also pins `GIT_CONFIG_*` to `core.autocrlf=false` so that Git for Windows checkout preserves the LF blob bytes of the immutable tag; otherwise pre-commit's `gofmt` would misreport the runner's CRLF conversion as unformatted source. This configuration is only for the test gate; the independent production build still builds assets from the tag's clean default checkout.

Release preparation fixes an immutable handoff (`release/release-handoff.json`) that records the release run, tag, commit, and every production and container artifact's size and SHA256, and publication re-verifies it against the approved `release/checksums.txt`; the policy rejects intermediate steps, path replacement or post-publish rewriting. Homebrew is no longer part of the Release workflow: the independent `publish-homebrew.yml` publisher consumes that handoff, verifies the production section, and maps the releaseassets stable/prerelease result directly to `pixiv-cli`/`pixiv-cli-beta`. The precise four-target matrix (macOS Intel/arm64, Linux amd64/arm64) first uses `brew tap-new pixiv-cli-release/staging --no-git` to create each runner's isolated local tap, then `brew trust --tap pixiv-cli-release/staging` to explicitly trust this single temporary namespace; it places the single staging formula into its `Formula/`, then runs a real `brew install --formula` via `pixiv-cli-release/staging/<formula>`. macOS runs in the native runner's temporary tap; Linux runs inside a short-lived, fixed-digest `homebrew/brew` container, and the staging formula directory is passed into the container as a read-only bind mount. It then runs `test "$(pixiv --version)" = "pixiv $RELEASE_TAG"` and compares against the tag. It does not use a workspace formula path, developer/environment-variable bypass, nor clone, write or trust the public tap. Only when all of the above succeeds does the protected `deploy_homebrew_tap` job inside `publish-homebrew.yml` HTTPS-clone the public tap and run the monotonic decision with `scripts/cmd/homebrewrecovery` from the trusted default-branch tip: the requested version must not be lower than the formula currently published in the tap (reusing `internal/releaseversion` SemVer comparison rather than string comparison); an identical same-version formula is reported as already published and no-ops without creating a commit or push; an older requested version, or a same version with different content, fails closed before the deploy key is read, so recovering the same `release_run_id` is idempotent and an older recovery can never roll the published formula back (`pixiv-cli` and `pixiv-cli-beta` are compared independently). Only when the decision requires a write does it verify the single staged formula and read the deploy key in the last step; the SSH push pins the official GitHub ED25519 known_hosts, enables strict checking, and targets exactly `HEAD:main`. If any preceding job fails, the tap is not written.

This local check only proves the declared dependencies and semantics of the workflow, and **does not** verify the remote actual state of the GitHub `release` Environment, secrets and tag protection; it does not replace the remote configuration audit, nor the four-architecture external Homebrew install evidence produced by a formal tag. Because the anonymous URL of a draft asset cannot be downloaded by Homebrew, the workflow first publicizes the Release before installing; if installation fails, the Release is already public but the tap is unchanged, and the maintainer must explicitly handle it — the gate must not be bypassed by a manual push.

There is direct backtrace evidence that Linuxbrew's `Resource` staging cleanup on GitHub hosted Linux runners triggers `EINVAL` on `FileUtils.chmod`, and that this error occurs earlier than options such as `--keep-tmp` can intervene. To keep the gate a real formula install, the Linux branch uses `docker run --rm` to start a fixed-digest `homebrew/brew` image, passing the read-only absolute staging-formula bind mount into the container; the container only creates a local staging tap, copies that formula, runs an ordinary tap-qualified `brew install --formula` and compares `pixiv --version` exactly against `RELEASE_TAG`. `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_ENV_HINTS=1` only eliminate drift from auto-update and hints, and do not change the formula or install semantics. The container does not read secrets, does not write to the host mount, does not use the public tap, and does not use `HOMEBREW_TEMP`, source/debug/keep-tmp flags. The fixed Homebrew 4.6 container image does not provide `brew trust`; this is not a security bypass: that tap is only created by `brew tap-new` inside the `--rm` container, the single formula is copied from the read-only mount, and the public tap is never touched. Native macOS Homebrew keeps the explicit `brew trust --tap`; both Linux and macOS use the same root `--version` gate and do not depend on a Python/Ruby JSON parser. The version comparison happens after `brew install` and cannot change the install acceptance path. Local Docker has been used for same-formula install experiments on both arm64 and amd64 QEMU; the GitHub runner's pre-release rehearsal is still the external evidence that must be obtained before a formal release.

### Pre-release read-only Homebrew rehearsal

<details>
<summary>Expand the operational boundary and platform evidence</summary>

Before creating any new tag or Release, a maintainer may manually run `.github/workflows/homebrew-prepublish-verify.yml` from the default branch, passing in an **already public, non-draft, non-prerelease** stable Release tag. It first verifies that the input is a `v`-prefixed SemVer, that the executing branch is the default branch, and that the GitHub Release's tag matches the input; it then downloads only that Release's published `checksums.txt`, renders the `pixiv-cli` staging formula, and finally runs a real local staging-tap install on the four production-identical runners for macOS Intel/arm64 and Linux amd64/arm64.

This is a read-only rehearsal: it has no `release` Environment, secret, tag checkout, Release/asset editing or creation, and does not clone, commit or push the Homebrew tap. Homebrew has exactly one publishing path; the rehearsal workflow deliberately has no deploy job, so a tap write can never bypass the immutable handoff and checksum verification. The Linux branch installs the read-only-mounted local staging formula in a fixed-digest, short-lived Homebrew container; macOS keeps the native ordinary install command.
It is used to reproduce the Homebrew install chain before a formal release, and **does not replace** formal tag publishing, signed Releases, tap deployment or post-publish install acceptance. The quality gate keeps behavior-level formula and artifact checks instead of mirroring the workflow as a second policy implementation.

The formal release must still be blocked by a formal tag, signed GitHub Release, tap formula and subsequent install acceptance. Complete same-source/version six-target Go binary/archive evidence must have been collected and verified (see [Native runner evidence](#native-runner-evidence)); the protected `release` Environment, the production signing private key and the public repository must also be configured, but these preconditions themselves do not mean that a Release/tap has been created or that the install path has been accepted.

The production Ed25519 public trust root is committed in source at [`internal/update/installer/release_installer.go`](../../../internal/update/installer/release_installer.go): the key ID is `ed25519-2c27e77742d3c33a`, and its SPKI DER SHA-256 fingerprint is `2c27e77742d3c33ad14be867d4e0519229a220898c9a7c868447eaef0951b4cf`. The same-package test verifies this mapping against a known real signature; it only proves that the public trust root has entered production wiring, not that actual signing, Release assets or install acceptance have been completed.

The remaining rules for the production Ed25519 trust root are as follows:

- The public key, key ID and fingerprint have entered supported binaries via auditable source changes; the private key never enters source, release assets, logs or formulas.
- The private key may only be used as a secret of the protected `release` Environment; recovery copies may only be stored in a controlled macOS Keychain. It must not enter source, logs, Release assets or formulas.
- When rotating, first publish a version that can trust the new key ID, retain the old public key until the old version goes out of support, and then stop using the old key via a new signed Release. Existing binaries must not suddenly depend on an uncommitted, unverifiable new trust root.

The Homebrew tap is an independent publishing surface: stable uses `pixiv-cli`, pre-release uses `pixiv-cli-beta`, and both install `pixiv` and conflict with each other. `publish-homebrew.yml` verifies the original Release handoff and archive checksums before rendering and pushing the formula; its native install checks use the platform registry. The dedicated `HOMEBREW_TAP_DEPLOY_KEY` is available only to the authorized tap-write step through the `release` Environment; it must never enter source, logs, or artifacts. This downstream publisher is not an additional manual approval boundary; `release-approval` owns the single final approval.

The current Release does not perform Apple notarization or Windows Authenticode. Direct downloads may still be blocked or prompted by Gatekeeper or SmartScreen; this is a system reputation boundary that must be retained in user documentation and must not be bypassed via docs or scripts.

A successfully completed `Release` workflow triggers the independent Homebrew, Docker Hub, SkillHub, and ClawHub publishers through `workflow_run`. The Release handoff binds the original run, tag, commit, and prepared artifact identities; it is not merely a tag file, and it does not depend on Homebrew deployment finishing first. Each publisher revalidates the identity and its required artifacts before publication. Do not infer a version from `workflow_run.head_branch` or substitute later `main` content.

SkillHub checks the immutable tag, default-branch ancestry, public Release, SemVer, and product-skill changes since the previous merged semantic-version tag. An unchanged skill skips publication; a changed skill goes through dry-run and commit. The product `SKILL.md` version must match the CLI Release tag. `SKILLHUB_TOKEN` enters only the final commit step; a returned `skillId` and review state proves receipt, not immediate public acceptance.

An authorized recovery dispatch supplies the original `release_run_id` to the specific publisher, never a replacement tag or current main. ClawHub additionally supports `verify_only` to inspect an already submitted version without republishing. Its dry-run is credential-free and the final publish/inspect verifies the exact artifact fingerprint. A clean static scan with aggregate security still `pending`, or a delayed `skill-card.md`, produces an explicit warning rather than a false failure of receipt. Neither warning proves final acceptance: `verify_only` requires clean aggregate security. Where the platform does not expose server-resolved GitHub provenance, retain the limitation and use the trusted tag checkout plus fingerprint evidence rather than claiming independent provenance verification.

</details>

## Git and local artifacts

`.gitignore` already excludes:

- `.DS_Store`
- build artifacts `build/`, `pixiv`, `pixiv-cli`
- local download directories `downloads/`
- local databases `*.db`
- common cache and temporary files

Do not commit Pixiv tokens, downloaded content, local databases, machine-specific configuration, Ed25519 private keys or tap deploy keys.

## Release notes and publication

`changelog/` maintains English and Simplified Chinese release notes per version directory. Each non-empty section uses, in order, `Breaking changes`, `Added`, `Changed`, `Fixed`, `Security`, `Documentation`, `Maintenance`; the bilingual files use the corresponding translated names and provide a same-scope `Full Changelog` compare link at the end. The first version uses the link to that tag's commits.

Each outcome-oriented note inlines the source PR; changes without an associated PR use a real short-SHA commit link. A single entry may merge multiple related sources. Changes with no user-visible impact go under `Maintenance` and must not be skipped. Each version also lists external contributors who first merged a contribution and are neither the repository owner nor a bot. `changelog/unreleased/` only retains release-prep hints and is not an editing target for ordinary PRs.

PR text only keeps "Changes", "Verification", "Self-check". Classification, breaking-change judgment and version summaries are decided by the release-prep maintainer based on the final Markdown sections and compatibility assessment; they are not read from the PR body, and PR authors are not required to decide the version number in advance.

After merge, test and review, use `scripts/cmd/releasenotes audit` to collect PRs, direct commits, authors and first-time contributors within the tag range. The audit report only goes to a local temporary directory or CI `$RUNNER_TEMP` and is not committed to the repository. The maintainer checks item by item, then directly writes `changelog/vX.Y.Z/en.md` and `zh-CN.md`, and uses `validate --audit` to check section order, the bilingual source set, the compare footer, missing sources and out-of-scope sources. The `release_notes_audit` job on the formal tag reruns the same audit and validation with read-only `contents` and `pull-requests` permissions.

Version selection is not release authorization. Before creating a release-prep PR, merging, creating or pushing a tag, triggering a release, or syncing historical GitHub Releases, the maintainer must explicitly confirm the specific version, commit/tag range and expected impact in the current session. The full operational path is in `.agents/skills/pixiv-cli-release-notes/SKILL.md`: ordinary PR → post-merge audit → directly write bilingual Markdown → validate → release-prep PR → tag and GitHub Release → SkillHub / ClawHub verification.

`sync-history` is dry-run by default. Only after explicitly passing `--apply` does it update the body of an existing GitHub Release; a missing historical Release is created from the existing tag without assets. Both cases read the remote body and compare it against the local bilingual rendering result.

## Documentation sync

When any of the following change, update the bilingual README, the bilingual CLI reference or the corresponding `docs/` in sync:

- MCP tools, parameters or return semantics.
- CLI commands, parameters, account configuration or output semantics.
- Environment variables or default values.
- Download, auth, proxy, ugoira and similar flows.
- Install channels, update channels, signing trust roots, Release/tap release gates or system reputation hints.
- New limits, retries, timeouts, truncation, degradation or error-handling policies.
- Test or build commands.
