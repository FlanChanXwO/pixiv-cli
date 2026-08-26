# Development

English | [简体中文](../../zh-CN/maintainers/development.md) | [Documentation index](../../index.md)

| What you want to do | Start here |
| --- | --- |
| Check the local toolchain | [Environment check](#environment-check) |
| Build or verify the ugoira native library | [Rust ugoira staticlib](#rust-ugoira-staticlib) |
| Run the CLI and MCP | [Run](#run) |
| Handle login and credentials | [Obtain refresh token](#obtain-refresh-token) |
| Choose test scope | [Tests](#tests) |
| Check the release workflow | [Release gates, signing and Homebrew boundaries](#release-gates-signing-and-homebrew-boundaries) |
| Prepare release notes | [Release notes and publication](#release-notes-and-publication) |

## Environment check

The project is a Go module; the current `go.mod` declares:

```text
go 1.26.3
```

Before starting, verify Go/cgo, Rust and the standard test environment:

```bash
go version
go env GOVERSION CGO_ENABLED CC GOOS GOARCH
cargo --version
go test ./...
```

## Rust ugoira staticlib

Production ugoira GIF/APNG is produced by the built-in Rust encoder; it does not depend on `ffmpeg` at runtime. `ffmpeg` is only used as a development quality comparison: only after explicitly setting `PIXIV_UGOIRA_QUALITY_FFMPEG=1` will the Rust quality gate call it; it is not a prerequisite for local builds or end-user execution.

Frame source reading shares the same memory boundary as the image decoder: the boundary value is taken directly from the pinned `image` crate `Limits::default().max_alloc`, rather than a separate empirical constant. ZIP member declared sizes exceeding this value fail before reading; actual expanded bytes exceeding it, failed memory reservation or cancellation also fail explicitly during chunked reading, without truncating input or falling back to another encoder. The cancellation token is checked before and after every read chunk and before and after image decode; however, the `image` crate's single-frame decoder has no cancellation callback, so a decode already entered cannot be interrupted midway and cancellation is only observed immediately after it returns. Focused regression covers declared-size overflow, actual cumulative-byte overflow, cancellation during reading, normal boundary inputs, normal GIF/APNG and corrupted ZIP; the purpose of this limit is solely to prevent frame sources from consuming unbounded memory before the decoder's own limits take effect, and the impact is that oversized works fail explicitly.

Supported Go source builds require:

- Go `1.26.3`;
- `CGO_ENABLED=1`;
- a C linker for the current `GOOS/GOARCH`;
- the committed `staticlib` for the corresponding Rust crate target;
- the six-target `staticlib/manifest.json` generated from the same Rust source.

The fixed targets are darwin/linux/windows for amd64/arm64. `scripts/build-staticlibs.sh` uses locked Cargo inputs to generate the target libraries; only after a single successful run produces all six real libraries and verifies each SHA-256 does it write `manifest.json` with the Rust source digest. Single-target invocations invalidate the existing manifest, preventing a partial rebuild from proving full-platform consistency.

The public ABI baseline for Linux releases is glibc 2.35. Linux runners for release test/production, native evidence, packaged-binary smoke and the Homebrew install matrix must be pinned to `ubuntu-22.04` and `ubuntu-22.04-arm`; quality, validate, publish and other jobs that do not produce Linux binaries may continue to use newer runners. After every Linux executable is produced, the following must be run:

```bash
go run ./scripts/cmd/linuxabi --binary <linux-elf>
```

This gate reads the real loader contract from the ELF `SHT_GNU_verneed` and also checks imported symbol versions; any dependency above `GLIBC_2.35` fails before packaging. Relying solely on "the binary runs on the build runner" is not acceptable, because that lets the runner's own newer glibc hide backward-compatibility regressions.

The six committed libraries and `manifest.json` are verified inputs: the manifest binds the Rust source digest, six targets, paths and per-target SHA-256s, and is locked by the integrity test of `internal/media/ugoira/staticlib`. The current manifest's source digest and the six library SHA-256s are byte-for-byte consistent with the audited source; upgrading Rust requires fully rebuilding, linking and smoke-verifying all six targets from the same audited source, and updating the six libraries, the manifest, native evidence and the release matrix at the same time — never update only a single platform's pin.

Compiler provenance of the committed libraries is pinned per target rather than using a movable runner-default toolchain: `x86_64-apple-darwin` and `x86_64-pc-windows-msvc` use Rust `1.96.0`; `aarch64-apple-darwin`, `aarch64-pc-windows-msvc`, `x86_64-unknown-linux-gnu` and `aarch64-unknown-linux-gnu` come from Rust `1.96.1`. Both release test and production matrices must carry this exact mapping and use it through `RUSTUP_TOOLCHAIN` and `rustup toolchain install` with `--no-self-update`; runner image `stable` updates must not change rebuild bytes. This mapping records provenance, not a standing license to mix toolchains; when upgrading Rust, rebuild and re-pin all six targets together.

In a controlled environment with the target toolchain available, run:

```bash
sh scripts/build-staticlibs.sh --target <rust-target>
go test ./internal/media/ugoira/staticlib -run '^TestCommittedManifestWhenPresent$' -count=1
```

Do not commit `internal/media/ugoira/rust/target/`; it is a machine artifact. The verified `internal/media/ugoira/rust/staticlib/` and its `manifest.json` are traceable inputs and must not be hidden by ignore rules.

The `.cargo/config.toml` of the Rust crate replaces crates.io with the complete locked dependency closure in the adjacent `vendor/`. Every package in `vendor/` carries a Cargo-generated `.cargo-checksum.json`; it, the Cargo config, `Cargo.toml`/`Cargo.lock`, `build.rs`, `.cargo/**`, the Rust source and the local `quantette` all count toward the staticlib source digest. Do not hand-edit vendor contents; when upgrading dependencies you must regenerate the full closure with `cargo vendor --locked --offline` and update the digest fixture and license bundle. The root `.gitattributes` sets `-text` for these first-party crate inputs, all of `vendor/**` and the pinned local `quantette` source; this only preserves the original Git blob bytes without rewriting normal content, and prevents Windows checkout from changing LF to CRLF and breaking Cargo checksums, source digests or license bundles. For the release archive `LICENSE`, the license bundle `THIRD_PARTY_LICENSES.md` and `third_party/licenses/**`, `text eol=lf` is fixed instead, so archive member audit and byte-for-byte `--check` stay stable on Windows. The digester must also normalize the platform separators from `filepath.Rel` to slashes before deciding `src/`, `.cargo/` and `vendor/`; otherwise Windows backslash paths would silently miss these inputs. `target/` remains a machine artifact, is not included in the digest, and must not be committed.

When running Cargo directly, you must start in the crate directory so Cargo discovers the source replacement:

```bash
(
  cd internal/media/ugoira/rust
  cargo test --locked --offline
  cargo clippy --locked --offline --all-targets -- -D warnings
)
go run ./scripts/cmd/licensebundle --check
sh scripts/test-rust-vendor.sh
```

`scripts/test-rust-vendor.sh` is the focused supply-chain regression for the release workflow: it sets up a temporary empty `CARGO_HOME` and `CARGO_TARGET_DIR`, then runs `cargo metadata/build/test --locked --offline` in turn, and with the same environment runs `go run ./scripts/cmd/licensebundle --check` for the six release targets. Therefore registry cache, network fallback, missing vendor contents or invalid checksums all fail explicitly; a runner's pre-warmed cache cannot serve as offline-reproducibility evidence.

### Native runner evidence

`.github/workflows/native-evidence.yml` is an independent, non-publishing runner entry point: it only allows an audited `main` push containing non-document inputs, or a `workflow_dispatch` targeting `refs/heads/main`. A push that touches only `README*.md`, `docs/**`, `changelog/**` or `skills/**` does not start it; any other path, as well as a manual dispatch, still runs the full matrix. Global `permissions: {}`, job-only `contents: read`. It has no `environment`, secret, tag/Release/tap/signing command; the YAML AST policy also pins six runners, full-SHA actions, credential-free checkout, vendored Rust checks, single-target staticlib, real cgo GIF/APNG smoke, the versioned binary's `pixiv --version`, release-style archive and artifact upload. The declaration itself can be checked offline:

Each target of the matrix must also declare the same `rust_toolchain` as release test/production; the job binds that value through `RUSTUP_TOOLCHAIN` and runs a precise `rustup toolchain install` with `--profile minimal --target ... --no-self-update`. The two verifiers share the single target-version mapping in `scripts/internal/releasecontract`; if either workflow removes, replaces, duplicates or mis-interpolates that mapping, policy fails closed.

For the two Windows targets, the Rust library uses `*-pc-windows-msvc`; the corresponding cgo selector must declare the library via `-L${SRCDIR}/… -lugoira_rs` and must not pass a drive-letter absolute `.lib` path directly to cgo; it must also explicitly carry the `advapi32`, `ntdll`, `userenv`, `ws2_32` and `dbghelp` import libraries required by the Rust `std`. Native evidence only sets `CC='clang -fuse-ld=lld'` explicitly in the Windows smoke and versioned-binary builds: LLD can handle MSVC `.lib` and also lets Go skip the GCC-specific debug linker script; this is not a runtime fallback and does not change the C linker choice on darwin/linux.

```bash
go test ./scripts/internal/nativeevidence -count=1
go run ./scripts/cmd/nativeevidence policy --workflow .github/workflows/native-evidence.yml
```

This policy command depends only on the source-digest/manifest contract of `internal/media/ugoira/staticlib` and does not import the cgo encoder; therefore it must be executable on each runner **before** that runner builds the target staticlib. If the policy gate fails due to a missing library or cgo link failure, that is a workflow bootstrap defect, not an acceptable "no native evidence yet" result.

Each runner artifact is only `evidence/`: the actually linked staticlib, the versioned binary, the archive and `native-evidence.json`. A schema 2 record independently records the `source_commit` provided by the workflow, recomputes the Rust source digest and the three SHA-256 values, runs the binary's `--version` and requires an exact single-line output, then checks the archive's binary, `LICENSE`, `THIRD_PARTY_LICENSES.md` and the full `third_party/licenses` regular-file tree one by one. It does not hold release/tap/signing credentials and does not create tags or Releases.

`.github/workflows/browser-evidence.yml` is another credential-free native provider contract matrix: it runs platform code and synthetic-fixture regression for `internal/browsercookies/...` on macOS, Linux and Windows amd64/arm64 runners, and `scripts/cmd/browsernativeevidence` verifies the workflow's runners, action SHAs, the pinned Firefox 153.0.3 release-package checksum, cleanup commands and the secret boundary. The `firefox_native` job only unpacks the official package in the runner's temporary directory, lets Firefox generate an isolated profile/schema, then injects explicit synthetic cookies to exercise the provider contract; it does not read user browser profiles, Keychain, DPAPI or Secret Service, nor upload package/profile/database. Real profile/session evidence can still only be obtained on the protected release-prep host; the success of this workflow must not be treated as real user-browser import success.

> [!WARNING]
> Native evidence must not be backfilled or spliced across runs. If the runner records of any single workflow run show divergent source digests, even if all six jobs completed, they must not be mixed and backfilled; the six-target evidence must be fully rerun from the fixed new SHA. Local unit fixtures, policy success or the mere existence of a workflow file are not six-target native evidence.

When a controlled backfill of the committed six-target libraries is needed, the workflow run's main SHA and the `v0.1.0-native-evidence.<run-id>` version it produced must match exactly; download exactly six `native-evidence-{darwin,linux,windows}-{amd64,arm64}` artifacts, then run `scripts/cmd/nativeevidence consolidate` against a clean non-symlink output directory. The consolidator only accepts the full six targets with the same source digest and the same expected version/commit; it re-verifies the staticlib/binary/archive SHA and archive member hashes and generates exactly six `manifest.json` entries, blocking on any missing/duplicated/mismatched target, metadata, archive member, hash or symlink. After manual review, backfill the six libraries and `manifest.json` into `internal/media/ugoira/rust/staticlib/`, then run `TestCommittedManifestWhenPresent`, `TestRustUgoiraEncoderNativeGIFAndAPNG` and `git diff --check` before committing the six blobs and manifest as an independent review commit. Any verification failure blocks release; partial artifacts must not be used.

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
`filename_template`, `directory_template`, `request_interval`, `https_proxy`, `log_level`, and `log_format`.
Other advanced TOML is maintained by the user by hand. The first configuration bootstrap is generated from
`internal/config/settings` schema metadata with `tomledit`; it is compact, includes only entries marked for the
baseline file, and never overwrites an existing file.

> [!NOTE]
> The removed `[web] fallback_enabled` returns `removed_setting` if it still exists; clean it with `pixiv config unset web_fallback_enabled`. `[logging].level` (`info|debug`) and `[logging].format` (`text|json`) are live startup settings; `PIXIV_LOG_LEVEL` and `PIXIV_LOG_FORMAT` override them.

### Flag parsing

The CLI uses Cobra/pflag, and flags may appear before or after positional arguments; for example, both `pixiv auth check 12345678 --json` and `pixiv search "初音ミク" --json` are supported.

The Pixiv command proxy, `[pixiv.network]`, environment variables and `[network]`, as well as the FANBOX-independent `[fanbox.network]`/`[fanbox.flaresolverr]` configuration, are all resolved per their own service boundaries, and FlareSolverr is used only for challenge recovery.

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

Current test coverage spans CLI commands and build metadata, explicit/automatic updates, `internal/services/{pixiv,fanbox}/account` account services, `internal/services/pixiv/pool` account pool, `internal/services/reversesearch` source/provider fixtures and aggregation, `internal/storage/database` auth storage and `internal/config/settings` configuration, `internal/shared/lifecycle` lifecycle, `internal/shared/pagination` logical pagination, `internal/shared/traversal` generic reentrant traversal, Pixiv App API auth retry, the public SDK (`sdk`/`sdk/pixiv`/`sdk/fanbox`), HTTP client wiring, download management, the Rust encoder/staticlib contract and `internal/mcpserver/{pixiv,fanbox}/tools` tool registration. `internal/account` and `internal/session` have been deleted and no compatibility test entry points are retained. Test file layout and same-package exceptions follow [Test file layout](#test-file-layout):

```bash
go test ./...
sh scripts/build.sh
# Offline fixture/crypto/permission-classification regression for the browser provider; real cross-platform host evidence is run separately on release-prep.
go test ./internal/browsercookies/... -count=1
# Real SDK e2e requires local credentials (Pixiv reads the selected account from the local pixiv-cli.db, FANBOX reads the Keychain):
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
# Run both current SDK E2E tests; the script does not accept token or other credential input.
scripts/test-e2e.sh
# Run only one of them, or only verify single-post post.info.
scripts/test-e2e.sh --pixiv-only
scripts/test-e2e.sh --fanbox-post-only
# Explicit reverse-search upstream compatibility observation; never run by default.
# Pre-export SAUCENAO_API_KEY from a private environment; do not inline it.
export SAUCENAO_API_KEY
PIXIV_REVERSE_SEARCH_E2E=1 \
PIXIV_REVERSE_SEARCH_SOURCE=<private-test-image-path-or-url> \
PIXIV_REVERSE_SEARCH_PROVIDER=all \
scripts/test-reverse-search-e2e.sh
```

`go test ./...` stays offline-stable by default; real SDK e2e is skipped when `PIXIV_SDK_E2E=1` or `FANBOX_SDK_E2E=1` is not explicitly set. Once explicitly enabled, missing local authorization credentials or a missing non-secret FANBOX target fails directly and exposes the gap, rather than disguising a skip as release evidence.

Reverse-search provider fixtures and CLI/MCP/config regressions remain part of the offline suite. Real reverse-search
network observation is separate and runs only when `PIXIV_REVERSE_SEARCH_E2E=1` is explicitly set; the source is
required, and SauceNAO or `all` additionally requires `SAUCENAO_API_KEY` while ascii2d-only runs do not. The script
accepts no source or key arguments, does not echo either value, and must be run only with an authorized test image.
It observes third-party compatibility, not a default release gate; a skipped or unavailable upstream must not be
reported as a successful real-network result.

`scripts/test-e2e.sh` only selects the current public SDK E2E tests: the Pixiv test reads the selected account from the local `pixiv-cli.db`, and the FANBOX test reads `FANBOXSESSID` from the agreed macOS Keychain item. The FANBOX `FANBOX_E2E_CREATOR_ID`, `FANBOX_E2E_TAG`, `FANBOX_E2E_POST_ID` and `FANBOX_E2E_POST_URL` only accept explicit, non-secret test targets; refresh tokens, sessions or full cookies are not accepted as arguments or environment variables. The optional `PIXIV_E2E_PROXY` only denotes a non-secret proxy URI; `FANBOX_E2E_SOLVER_URL` and `FANBOX_E2E_SOLVER_PROXY` are optional non-secret recovery topology configuration and the solver is not enabled by default. When real E2E is not explicitly enabled, tests skip by default; when explicitly enabled but missing local credentials or FANBOX targets, they fail, and a default skip or automatic discovery must not be recorded as release evidence.

The real SDK E2E for v1 is `TestRealPixivSDKRead` and `TestRealFanboxSDKRead` (see the `PIXIV_SDK_E2E=1` / `FANBOX_SDK_E2E=1` commands under [Tests](#tests)). The Pixiv test process only reads the refresh token of the selected account from the local `pixiv-cli.db`, opens `sdk/pixiv` to verify identity and completes a stable detail/list and `Resource` read; the rotated credentials are first persisted via a normal repository transaction before continuing content requests. The FANBOX side directly reads the authorized `FANBOXSESSID` item from the macOS Keychain, and uses explicit creator/tag/post/page URL targets to verify `Creator`, `Creators`, `CreatorTags`, `CreatorPosts`, `TaggedPosts`, `Post`, `Home`, `Supporting`, `ResolveURL`, `OpenResource` and `SaveResource` one by one; list targets each follow up one continuation when the server returns a cursor, and post details must discover a file attachment and fully read it in a temporary directory. On session expiry, it explicitly reports `credentials_expired` and asks for re-import, without fallback. After release-prep runs, the operator scans stdout, stderr, test logs and evidence; tokens, cookies, signed URLs and raw response bodies must not end up in argv, environment dumps, logs, test names, artifacts or failure diffs. The above describes test coverage, not that real e2e has already been run; do not write tokens into shell history, logs or repository files.

For legitimate article details without a file attachment, `TestRealFanboxSDKPostInfo` is the supplemental test: it only requires an explicit post ID/page URL, verifies the public SDK's `Post`, a non-empty body, `ResolveURL` and the resource manifest, and allows `file_assets=0`; it cannot replace `TestRealFanboxSDKRead` for the strict resource path, which performs HEAD, full save and byte-count verification for every file attachment in the detail.

Under an explicit proxy, resource transport is fixed to negotiate HTTP/1.1, while App API and OAuth keep their original protocol negotiation. The resource read of this e2e is used to regress this resource-transport boundary; it does not add a fixed timeout for slow normal downloads. If Pixiv returns a 429 without a valid `Retry-After`, the real e2e retains the diagnostic and fails explicitly, without guessing a wait or retrying indefinitely.

`PIXIV_E2E_BINARY` and `PIXIV_E2E_EXPECTED_VERSION` let CI run offline e2e against a built, unpacked release binary; they do not inject tokens and do not enable the real Pixiv API. `platform-smoke.yml` builds, packages, unpacks and runs this set of CLI/config/MCP stdio verifications on six supported runners.

Before code changes are complete, tests should be added or updated according to the scope of the change. If tests cannot be run, the reason and risk must be stated in the delivery notes.

Release-related local fixture/policy gates also include:

```bash
sh scripts/test-build-staticlibs.sh
sh scripts/test-package-release.sh
go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml
go test ./scripts/cmd/nativeevidence -count=1
go run ./scripts/cmd/nativeevidence policy --workflow .github/workflows/native-evidence.yml
go test ./scripts/internal/browsernativeevidence -count=1
go run ./scripts/cmd/browsernativeevidence policy --workflow .github/workflows/browser-evidence.yml
go test ./scripts/tests/platformsmokeworkflow -count=1
sh scripts/test-homebrew-formula.sh
git diff --check
```

The fixtures only prove format, failure semantics and local policy; they do not replace the real static linking, GIF/APNG smoke, versioned archive content and Homebrew install acceptance of the six native runners.

`.github/workflows/ci.yml` and `.github/workflows/platform-smoke.yml` first perform strict path classification on the PR/main diff. Changes that touch only `README*.md`, `docs/**`, `changelog/**` or `skills/**` keep a name-stable Quality gate, but only run `go test ./scripts/tests/documentation -count=1`; the six-platform packaged-binary smoke is marked skipped, and the always-on `Platform smoke gate` verifies that this is the expected result. Any other path, an empty diff, an incomparable initial push or a manual dispatch runs the full Linux quality gate (test, race, vet, build, package/release policy, pre-commit) and the six-platform offline packaged-binary smoke; the same aggregate gate only passes when all matrices succeed. The Windows runner job of CI focuses on root callback wiring's `TestAuthURLCallback*`, `TestAuthURLHandlerInstall` and `TestNormalCLIInvocationEnsuresPersistentHandlerWithoutBlockingCommand`, then runs the full native callback-handler contract of `internal/cli/commands/pixiv/auth/loginhelper`; the full `internal/cli` is already covered by the Linux quality gate, so unrelated whole-package SQLite stress is not dragged into the Windows handler job. `.github/workflows/browser-evidence.yml` only runs the credential-free macOS/Linux/Windows provider contract matrix on `main` pushes or manual dispatches whose inputs change browser-provider-related code. When the classifier cannot read the diff it fails explicitly and never silently skips. All workflows use read-only permissions and pinned-SHA actions; real Pixiv/FANBOX SDK E2E does not enter regular PR/main CI. Only the tag-publishing `release.yml` runs the credential-free SDK E2E contract gate after validate; the production build explicitly depends on that job. Real SDK E2E is still accepted independently on release-prep in an authorized environment.

`.github/workflows/pr-metadata.yml` uses `pull_request_target` on PR `opened`, `reopened` and `synchronize` to update metadata: `actions/labeler` adds existing `area: docs`, `area: frontend`, `area: backend`, `area: github-actions`, `area: tests` and `release` labels from the base branch's `.github/labeler.yml` by path; it then only appends the PR author as assignee and never removes human assignments or labels. That job only has `contents: read` and `pull-requests: write`, does not check out and does not run PR-branch code, so fork PRs do not gain write access or execute untrusted input. The workflow and configuration only take effect for subsequent PRs after they are first merged into the default branch; the PR that introduces this configuration itself needs manual labeling on GitHub.

`scripts/tests/installers` verifies the installers using a local fake Release, fake `curl` and checksum fixtures, without accessing GitHub. The Unix job actually runs `install.sh`, covering SHA-256, directories with spaces, version preflight and not overwriting the old binary on verification failure; the Windows amd64/arm64 platform-smoke also runs `install.cmd` with real `cmd.exe`, `certutil.exe` and `tar.exe`, always passing `--no-path`, so the test does not modify the runner user's registry. The platform workflow policy requires this test to exist.

The Windows `.zip` is produced by `7z` preinstalled on the GitHub runner image; other platforms continue to use `zip`. `scripts/test-package-release.sh` delegates the faked invocations to the real `7z` on the Windows runner, uses the `zip` fixture on other dev machines, and checks archive members; therefore a Git Bash missing `zip` is directly exposed at the release test gate. It uses MSYS's `winsymlinks:nativestrict` to create the checked links: if the runner cannot create native Windows links, the test explicitly fails, preventing Git Bash's plain-file pseudo-links from neutering the output-ancestor safety gate.

### Test file layout

A production file `x.go` corresponds to at most one `x_test.go` in the same directory; platform-specific tests use `x_<platform>_test.go` and must have a real base owner. Tests for new owners always use the external test package (`X_test`); only the directories below are allowed to be same-package, because they observe unexported internal state. New same-package exceptions must be registered here with a permanent reason; otherwise they are treated as violations.

| Directory | Reason for same-package |
| --- | --- |
| `internal/cli` | The composition root test observes unexported root wiring, invocation lifecycle, and close ordering; these seams are not a public API. |
| `internal/browsercookies/chromium` | Tests construct the provider directly and inject an encryption key override, observing the unexported cookie record decryption path and profile discovery logic. |
| `internal/browsercookies/firefox` | Tests observe unexported profile discovery (`profiles.ini` parsing), cookie database path resolution, and record layout. |
| `internal/browsercookies/safari` | Tests directly call the unexported `parseBinaryCookies`, asserting binarycookies record layout. |
| `internal/browsercookies/secret` | Tests construct `SecretService{command: ...}` injecting unexported fields and assert unexported sentinel errors and command-output redaction behavior. |
| `internal/update/installer` | Tests inject the unexported `assetURLValidator` seam and checksum verification function, and use real fixture binaries to verify root `--version` preflight and that the old executable is not replaced on failure. |
| `internal/update/release` | `source_route_test.go` observes unexported source route selection and canonical API URL cache state; the rest of the directory already uses the external package. |
| `scripts/internal/browsernativeevidence` | Tests observe unexported environment probes and inject synthetic Firefox cookie seeds. |
| `scripts/internal/changescope` | Tests directly call unexported path parsing (`splitNULPaths`, `docsOnlyPaths`) and change-scope determination. |
| `scripts/internal/homebrewformula` | Tests directly call unexported formula rendering and version validation (`renderFormula`, `validateFormulaVersion`, `checkDynamicVersionNeeds`). |
| `scripts/internal/licensebundle` | Tests observe unexported `defaultBundleFileOps`, `generateFromTargetMetadata`, and license text normalization, and inject fake cargo metadata. |
| `scripts/internal/linuxabi` | Tests directly call unexported glibc version parsing and ABI comparison (`parseGLIBCVersion`, `checkImportedSymbols`). |
| `scripts/internal/nativeevidence` | Tests directly call unexported record/consolidate/policy seams, covering schema 2, independent `source_commit`, exact binary `--version` output, six-target hash/archive verification, and mutation rollback. |
| `scripts/internal/prepublishhomebrew` | Tests inject unexported `test.mutate` and CI change detection, observing the failure paths of formula generation and pre-publish checks. |
| `scripts/internal/publicapi` | Tests observe the unexported parser handling of `unexported`/`hidden` symbols and the golden comparison logic, using `writeFixture` to generate fixtures. |
| `scripts/internal/releaseassets` | Tests inject unexported `injectReleaseSources`/`injectWindowsReleaseSources`, observing asset archive naming (`archiveName`) and checksums generation. |
| `scripts/internal/releasenotes` | Tests observe unexported GitHub client call mappings, injecting a fake client to assert source auditing. |
| `scripts/internal/releaseworkflow` | Tests inject unexported `mutation.mutate`/`mutation.run` to observe the workflow state machine and git environment injection, and lock the Version-only linker and root `--version` Homebrew gate. |

There are currently no temporary items. This list does not accept open-ended phrases like "migration period" or "in the future". Adding a directory requires explaining the specific unexported symbol being observed and confirming that exporting a minimal interface is not a viable substitute; removing a directory requires a deletion task and test migration evidence (external package compiles + coverage unchanged).

`e2e/` and `scripts/tests/clawhubworkflow/` are also `package X`, but they have no production code (pure test carriers), so the "observing unexported production state" problem does not apply and they are outside the scope of this list. Cross-platform differences: test files with build tags (such as `scripts/internal/*`) have a different number of visible files under different `GOOS`/`GOARCH` combinations; when verifying, run `go list` separately with `GOOS=darwin`, `GOOS=windows`, and `GOOS=linux` to confirm the directory set is consistent.

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

This is the maintainer-side authority for capabilities that **must not have any entry point** in v1. It is a negative contract: adding a CLI/MCP/SDK entry point for any of these is a defect. Schema-only placeholders or mock empty results are forbidden.

**Unsupported (explicitly not supported in v1; a new entry point is a defect):**

| ID | Unique owner | Current evidence | Close-out condition |
| --- | --- | --- | --- |
| `ART-SEARCH-RATING` | `internal/cli/commands/pixiv/search` + `sdk/pixiv` | CLI `--rating` reports "rating filter is not supported by the v1 App API search contract"; MCP `search_illust` schema has no rating parameter | Only when the v1 App API search contract adds rating semantics; then synchronize the SDK field, CLI flag, MCP schema, locale documentation, and this list |
| `NOVEL-SEARCH-ADVANCED` | No owner (must not be added) | SDK/MCP schema has no advanced field | May be evaluated once the upstream contract appears; schema-only placeholders are forbidden |

**Evidence-gated (no entry point today; adding one requires the close-out condition):**

| ID | Unique owner | Current evidence | Close-out condition |
| --- | --- | --- | --- |
| `NOVEL-RANKING` | No owner | SDK has no `NovelRanking` export; MCP has no `novel_ranking` tool | After the upstream App API provides novel ranking |
| `NOVEL-BOOKMARK-MUTATION` | No owner | SDK has no `AddNovelBookmark`-style export; `user_novel_bookmarks` is read-only | Same as above |
| `COMMENT-WRITE` | No owner | MCP `comment_post`/`comment_add` directories = 0; SDK `PostComment`/`DeleteComment` exports = 0 | After the upstream provides a verifiable write contract |
| `NOTIFICATION` | No owner | MCP `notification` directory = 0; SDK `Notification*` exports = 0 | Same as above |
| `AUTOCOMPLETE` | No owner | MCP `autocomplete` directory = 0; SDK `Autocomplete*` exports = 0; not merged into `search` | Same as above |
| `WEB-RESTRICTED-READ` | No owner | No `webapi` package; `web_fallback_enabled` is a tombstone key (`config get/set` → `removed_setting`) | Do not reopen the anonymous Web path; any proposal to restore Web/AJAX must first amend the AGENTS frozen contract and pass an ADR |
| `USER-BLOCK-MUTE-REPORT` | No owner | MCP `mute`/`report` directories = 0; SDK `BlockUser`/`MuteUser`/`ReportUser` exports = 0 | After the upstream provides a verifiable mutation contract |
| `WATCHLIST-MARKER` | No owner | MCP `watchlist` directory = 0; SDK `Watchlist*` exports = 0 | After the upstream provides it |
| `BOOKMARK-USERS` | No owner | No corresponding tool/SDK export; `bookmark_detail` only covers the current user detail | May be evaluated after the upstream provides it |
| `SPOTLIGHT-PIXIVISION` | No owner (out of scope) | MCP `spotlight`/`pixivision` directories = 0; SDK `Spotlight*`/`Pixivision*` exports = 0 | Explicitly out of scope; re-evaluate only when the product scope changes |

Adding or restoring an entry point for any capability above is a functional change: update the corresponding row here and synchronize the relevant user documentation. Reviewers re-run the negative grep / directory existence checks under each item manually.

## Release gates, signing and Homebrew boundaries

`.github/workflows/release.yml` is triggered by `v[0-9]*` tags by default: it first verifies SemVer, then runs the credential-free SDK E2E contract gate on the immutable tag to confirm that test entry points and the default skip/offline boundary have not been broken; only then does it build the Rust staticlib for darwin/linux/windows × amd64/arm64, test Go/Rust, check licenses and package the fixed-name archives. This workflow does not read or inject Pixiv/FANBOX credentials. Real SDK E2E, native browser and one-time solver acceptance must be completed in an authorized environment per the corresponding process on this page; the contract gate must not be treated as real release evidence.

`releaseassets finalize` also reads `scripts/install.sh` and `scripts/install.cmd` from the immutable tag, copies them into the Release under fixed names, and writes `checksums.txt` and the Ed25519 signature manifest alongside the six platform archives. The publish policy locks the finalize parameters and the complete eight-asset upload set; the Homebrew renderer also requires the checksum set to include both installers, but the formula still only downloads the platform-appropriate archive.

release.yml only accepts `v[0-9]*` tag pushes and no longer offers `workflow_dispatch`, a `release_tag` input or a test-only overlay. When a tag run fails, fix the cause on the default branch and re-run the normal immutable-tag release process; new verifiers, tests or production sources are not injected into an old tag from the default branch, and no manual recovery entry is provided for an existing Release. validate, test build, production build and publish are all bound to the same tag; the production build rebuilds the staticlib from a clean tag tree on a separate runner and continues to use `git diff --exit-code` for byte-for-byte verification.

GitHub Release and GHCR are separate systems and cannot commit atomically. Container publication is therefore split into `build_container` before Release and `publish_container` after Release. If GHCR publication fails, the release workflow remains failed; recovery reruns the failed `publish_container` job with the same verified container artifacts (retained for 90 days) and immutable tag—do not rebuild or resign native assets to repair registry publication. The exact-version manifest is always pushed; `latest` moves only when the existing channel classifier reports stable. No retry loop hides push failures.

### Container release verification

`build_container` runs after the shared `build` gate and beside `build_production`; it never waits for production assets to be rebuilt. The two native targets are `ubuntu-22.04` for `linux/amd64` and `ubuntu-22.04-arm` for `linux/arm64`. Each target checks out the immutable tag, rebuilds its Rust staticlib from that clean tree, builds a versioned Linux binary through the Linux ABI gate, runs container packaging tests, builds a pinned glibc runtime image, verifies non-root execution, the exact version, `pixiv config path` under `/home/pixiv/.pixiv-cli/`, `/work`, and OCI provenance (`org.opencontainers.image.source`, revision, version, and licenses), then exports `verified-container-linux-amd64` and `verified-container-linux-arm64`. Build jobs receive only `contents: read`; only `publish_container` consumes those artifacts with `packages: write` after GitHub Release.

Focused maintainer checks are:

```bash
go test ./scripts/internal/releaseworkflow -count=1
go test ./scripts/tests/containerrelease -count=1
go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml
```

The credential-free container smoke workflow builds both native architectures on relevant changes and executes version, non-root, state-path, and working-directory assertions; these local checks do not replace that CI evidence for a tagged release.

The shared contract for release policy lives in `scripts/internal/releasecontract` and `scripts/internal/workflow/yaml`. The former holds the single per-target Rust toolchain mapping and the six-platform contract; the latter provides YAML AST safe operations; both directly participate in normal release policy and production build validation. The retained release verifier tests only cover tag trigger, build quality, production isolation, publish/Homebrew policy, release notes and the workflow YAML safety boundary; historical recovery plans and acceptance reports retain their original text and paths and do not serve as current process descriptions.

### Verifier source navigation

`scripts/cmd/releaseworkflow/` is organized by release responsibility: `main.go` handles command entry, file reading and top-level dispatch; `build_policy.go` handles validate, test build and production build; `e2e_policy.go` locks the Environment, secret reachability, input mapping and build dependency of the protected real E2E; `workflow_policy.go` handles tag trigger, job, step, command, action and permission helpers; `publish_policy.go` handles source verification, publishing, signing and channel; `homebrew_policy.go` handles formula render, four-platform verification and tap deploy. Tests are concentrated in each policy file and in `releaseworkflow_test.go`, `homebrew_policy_test.go`, `release_notes_policy_test.go` and `workflow_policy_test.go`; the command entry no longer keeps a top-level `main_test.go` that only verifies pass-through.

`scripts/cmd/nativeevidence/` is organized by evidence lifecycle: `main.go` handles subcommands and flags; `models.go` stores target and evidence schema; `record.go` records single-runner evidence; `consolidate.go` validates and merges the six-target results; `archive.go` handles release archive members and JSON; `filesystem.go` handles paths, hashes and safe file operations; `workflow_policy.go` only verifies the native-evidence workflow. Tests cover policy, record and consolidate respectively, and fixture helpers are split by workflow and evidence/archive, avoiding piling policy tests back into a single file.

`go run ./scripts/cmd/releaseworkflow --workflow .github/workflows/release.yml` launches the YAML AST policy of the release workflow, rather than relying on text layout or line numbers. It precisely checks the tag trigger, the permissions/dependencies of the nine jobs, the secret mapping and publish blocking of the protected E2E, the six test/production runner matrices, the 40-bit SHA of every `uses`, and the SemVer channel invocation of publish. Default-branch ancestry must be done in the `verify_release_source` job with no `environment` and no secret; only after that job succeeds may publish depend on it and declare the precise `release` Environment and the two expected secrets used by the signature-metadata step. The policy scans every scalar's GitHub expression for the `secrets` context by expression boundary; `}`/`}}` in single-quoted strings and two single-quote escapes do not terminate the scan early, so formatted secret references outside the signature-metadata step also fail closed. The policy also rejects `continue-on-error` or conditional `if` on required jobs, default-branch ancestry steps and the quality gate; the validate and build checkouts must also explicitly set `persist-credentials: false`. To prevent shell control flow from hiding a gate, every quality check is the single-command `bash` step: the policy precisely verifies its run, crate cwd (Rust gate) and shell, and rejects unaudited `env`, `defaults` or other step fields. The only allowed variables are the root `RELEASE_TAG`, and the `CC` and per-target `RUSTUP_TOOLCHAIN` bound by the build matrix; Windows must use `clang -fuse-ld=lld` to link the MSVC Rust staticlib, avoiding mixing MinGW GCC and `.lib` ABI. The parser also fails closed on YAML aliases, merge keys and any duplicate mapping keys, so GitHub's override or working-directory semantics cannot diverge from the local check. The validate checkout pins the audited workflow SHA; the remaining production source checkouts are pinned to the exact tag. In particular, `verify_release_source` may only run, in order, the full-history, credential-free tag checkout and the default-branch ancestry gate — these two steps — and forbids any `ref`, `repository`, `path` or intermediate HEAD-switching step from altering the verified commit. The publish checkout likewise only allows credential-free tag source, preventing inconsistency between the signature metadata and the commit the build assets belong to. The build job must actually run the vendored Rust offline check, `cargo fmt --check` at the crate cwd, locked/offline Clippy `-D warnings`, plain Go tests, vet, licenses, packaging, the pinned `pre-commit==4.6.0`, pre-commit and `git diff --check`; the production build job only produces `verified-release-*` artifacts from a clean tag tree. The release channel may only be determined by `go run ./scripts/cmd/releaseassets channel --version ...`; the hyphen in build metadata must not turn a stable tag into a prerelease.

Go 1.26.3 does not support the race detector on Windows ARM64, so that single matrix entry explicitly skips `go test -race`; the other five native targets still run the race gate, and the workflow policy fixes this condition and forbids expanding it to arbitrary conditional skips. The test matrix also pins `GIT_CONFIG_*` to `core.autocrlf=false` so that Git for Windows checkout preserves the LF blob bytes of the immutable tag; otherwise pre-commit's `gofmt` would misreport the runner's CRLF conversion as unformatted source. This configuration is only for the test gate; the independent production build still builds assets from the tag's clean default checkout.

After publish verifies and publicizes the Release, it immediately uploads the same `release/checksums.txt`; the policy rejects intermediate steps, path replacement or post-publish rewriting. `render_homebrew_formula` only downloads that artifact and maps the releaseassets stable/prerelease result directly to `pixiv-cli`/`pixiv-cli-beta`. The precise four-target matrix (macOS Intel/arm64, Linux amd64/arm64) first uses `brew tap-new pixiv-cli-release/staging --no-git` to create each runner's isolated local tap, then `brew trust --tap pixiv-cli-release/staging` to explicitly trust this single temporary namespace; it places the single staging formula into its `Formula/`, then runs a real `brew install --formula` via `pixiv-cli-release/staging/<formula>`. macOS runs in the native runner's temporary tap; Linux runs inside a short-lived, fixed-digest `homebrew/brew` container, and the staging formula directory is passed into the container as a read-only bind mount. It then runs `test "$(pixiv --version)" = "pixiv $RELEASE_TAG"` and compares against the tag. It does not use a workspace formula path, developer/environment-variable bypass, nor clone, write or trust the public tap. Only when all of the above succeeds does the final protected `deploy_homebrew_tap` HTTPS-clone the public tap, verify the single staged formula, and read the deploy key in the last step; the SSH push pins the official GitHub ED25519 known_hosts, enables strict checking, and targets exactly `HEAD:main`. If any preceding job fails, the tap is not written.

This local check only proves the declared dependencies and semantics of the workflow, and **does not** verify the remote actual state of the GitHub `release` Environment, secrets and tag protection; it does not replace the remote configuration audit, nor the four-architecture external Homebrew install evidence produced by a formal tag. Because the anonymous URL of a draft asset cannot be downloaded by Homebrew, the workflow first publicizes the Release before installing; if installation fails, the Release is already public but the tap is unchanged, and the maintainer must explicitly handle it — the gate must not be bypassed by a manual push.

There is direct backtrace evidence that Linuxbrew's `Resource` staging cleanup on GitHub hosted Linux runners triggers `EINVAL` on `FileUtils.chmod`, and that this error occurs earlier than options such as `--keep-tmp` can intervene. To keep the gate a real formula install, the Linux branch uses `docker run --rm` to start a fixed-digest `homebrew/brew` image, passing the read-only absolute staging-formula bind mount into the container; the container only creates a local staging tap, copies that formula, runs an ordinary tap-qualified `brew install --formula` and compares `pixiv --version` exactly against `RELEASE_TAG`. `HOMEBREW_NO_AUTO_UPDATE=1` and `HOMEBREW_NO_ENV_HINTS=1` only eliminate drift from auto-update and hints, and do not change the formula or install semantics. The container does not read secrets, does not write to the host mount, does not use the public tap, and does not use `HOMEBREW_TEMP`, source/debug/keep-tmp flags. The fixed Homebrew 4.6 container image does not provide `brew trust`; this is not a security bypass: that tap is only created by `brew tap-new` inside the `--rm` container, the single formula is copied from the read-only mount, and the public tap is never touched. Native macOS Homebrew keeps the explicit `brew trust --tap`; both Linux and macOS use the same root `--version` gate and do not depend on a Python/Ruby JSON parser. The version comparison happens after `brew install` and cannot change the install acceptance path. Local Docker has been used for same-formula install experiments on both arm64 and amd64 QEMU; the GitHub runner's pre-release rehearsal is still the external evidence that must be obtained before a formal release.

### Pre-release read-only Homebrew rehearsal

<details>
<summary>Expand the operational boundary and platform evidence</summary>

Before creating any new tag or Release, a maintainer may manually run `.github/workflows/homebrew-prepublish-verify.yml` from the default branch, passing in an **already public, non-draft, non-prerelease** stable Release tag. It first verifies that the input is a `v`-prefixed SemVer, that the executing branch is the default branch, and that the GitHub Release's tag matches the input; it then downloads only that Release's published `checksums.txt`, renders the `pixiv-cli` staging formula, and finally runs a real local staging-tap install on the four production-identical runners for macOS Intel/arm64 and Linux amd64/arm64.

This is a read-only rehearsal: it has no `release` Environment, secret, tag checkout, Release/asset editing or creation, and does not clone, commit or push the Homebrew tap. The Linux branch installs the read-only-mounted local staging formula in a fixed-digest, short-lived Homebrew container; macOS keeps the native ordinary install command.
It is used to reproduce the Homebrew install chain before a formal release, and **does not replace** formal tag publishing, signed Releases, tap deployment or post-publish install acceptance. `go run ./scripts/cmd/prepublishhomebrew --workflow .github/workflows/homebrew-prepublish-verify.yml` checks the immutable boundary of this workflow locally and in the quality gate.

The formal release must still be blocked by a formal tag, signed GitHub Release, tap formula and subsequent install acceptance. The complete six-target staticlib/manifest and real native artifact evidence must have been collected and backfilled under control (see the "Rust ugoira staticlib" section); the protected `release` Environment, the production signing private key and the public repository must also be configured, but these preconditions themselves do not mean that a Release/tap has been created or that the install path has been accepted.

The production Ed25519 public trust root is committed in source at [`internal/update/installer/release_installer.go`](../../../internal/update/installer/release_installer.go): the key ID is `ed25519-2c27e77742d3c33a`, and its SPKI DER SHA-256 fingerprint is `2c27e77742d3c33ad14be867d4e0519229a220898c9a7c868447eaef0951b4cf`. The same-package test verifies this mapping against a known real signature; it only proves that the public trust root has entered production wiring, not that actual signing, Release assets or install acceptance have been completed.

The remaining rules for the production Ed25519 trust root are as follows:

- The public key, key ID and fingerprint have entered supported binaries via auditable source changes; the private key never enters source, release assets, logs or formulas.
- The private key may only be used as a secret of the protected `release` Environment; recovery copies may only be stored in a controlled macOS Keychain. It must not enter source, logs, Release assets or formulas.
- When rotating, first publish a version that can trust the new key ID, retain the old public key until the old version goes out of support, and then stop using the old key via a new signed Release. Existing binaries must not suddenly depend on an uncommitted, unverifiable new trust root.

The Homebrew tap is an independent publishing surface: stable uses `pixiv-cli`, pre-release uses `pixiv-cli-beta`, both install `pixiv` and conflict with each other. A dedicated tap deploy key is placed only in the protected `release` Environment secret `HOMEBREW_TAP_DEPLOY_KEY` of the source repository, and the public tap only registers the corresponding public key. The workflow generates the staging formula in an independent renderer, verifies the install on four native runners, and then makes a restricted commit/push in the final protected job. Subsequent stable/beta releases still must not read, generate or record the deploy key from this repository or workflow artifacts.

The current Release does not perform Apple notarization or Windows Authenticode. Direct downloads may still be blocked or prompted by Gatekeeper or SmartScreen; this is a system reputation boundary that must be retained in user documentation and must not be bypassed via docs or scripts.

A successfully completed `Release` workflow also triggers `.github/workflows/publish-skillhub.yml` and `.github/workflows/publish-clawhub.yml`. When GitHub creates a Release with `github.token`, it does not recursively trigger the `release` event, so that event cannot be used as a reliable automated handoff. The Release that completes Homebrew deployment hands over a short-lived artifact containing only the exact release tag; this prevents the recovered `workflow_run.head_branch` of `main` from being mistaken for the version. The SkillHub workflow only checks out that immutable tag, and after confirming that the tag belongs to the default branch, the corresponding GitHub Release is public and the version satisfies SemVer, it compares `skills/pixiv-cli/` with the previous merged semantic-version tag. When the directory is unchanged, the workflow succeeds and skips; when the directory has changed, it runs the SkillHub CLI's dry-run and commit. The SemVer of the product `SKILL.md` must match the CLI Release tag; the tag-source validation of the release rejects mismatched versions before the protected E2E and any release credentials. `SKILLHUB_TOKEN` only enters the final commit step, and the CLI must return `skillId` and review status; this proves that SkillHub has received the submission, but the public details page may still not be visible until platform review is complete.
If either independent publish fails, it may be recovered via the corresponding workflow's `workflow_dispatch` input of the existing release tag; a subsequent `main` must not be used to substitute for that tag. The ClawHub workflow uses the same immutable tag handoff as SkillHub: it first verifies the public non-draft Release, default-branch ancestry, `SKILL.md` version/tag consistency and product-skill changes, then runs a fixed-version ClawHub CLI dry-run in a credential-free environment, and uses its SHA-256 artifact fingerprint to verify the actual publish. Only the final publish/inspect step, and the `verify_only` manual recovery step that does not republish a version, receive `CLAWHUB_TOKEN`; the latter only logs in and reads the review result and never calls publish. A normal publish must confirm the product skill, the corresponding version and the exact artifact fingerprint; when ClawHub static scan is clean but the aggregate security conclusion is still `pending`, it issues an explicit warning rather than misreporting the received publish as a failure. `skill-card.md` may also be generated asynchronously and produce a warning. Neither class of warning equals the final security conclusion: `verify_only` only passes when aggregate security is clean, so that a non-republishing final check can be performed after platform scanning completes. Any other cause still fails. The current platform also does not expose server-resolved GitHub provenance for an ordinary CLI publish, so this is replaced by a trusted tag checkout and fingerprint match, also retaining the warning.

</details>

## Git and local artifacts

`.gitignore` already excludes:

- `.DS_Store`
- build artifacts `build/`, `pixiv`, `pixiv-cli`
- local download directories `downloads/`
- local databases `*.db`
- common cache and temporary files
- Rust `internal/media/ugoira/rust/target/`

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
