# Pixiv CLI Reference

English | [简体中文](../zh-CN/cli-reference.md) | [Project home](../../README.md)

This is the complete contract for the `pixiv` command: installation, authentication, commands, flags,
configuration, environment variables, authentication, and updates. It does not duplicate SDK or MCP details;
those interfaces are linked under [Related documentation](#related-documentation).

Visual lists automatically emit canonical NDJSON when piped. Downloads currently expose page selection, static
quality, GIF/APNG mode, output directory, filename templates, and `--on-error`; unsupported options fail as unknown
flags. Proxy URIs accept `http`, `https`, `socks5`, and `socks5h`.

`pixiv search SOURCE` also performs reverse-image search when `SOURCE` is an explicit HTTP(S) URL or an existing
regular local file. Reverse search is independent of the authenticated Pixiv account; it uses the configured
third-party provider and may upload the source outside the machine.

Review the [SauceNAO privacy and terms](https://saucenao.com/legal.html) before
using it: uploaded images may be retained for a short period, and URL queries
may be cached for longer than the request itself.

User-visible changes are recorded in the [versioned changelog](../../changelog/README.md).

[GitHub Releases page]: https://github.com/FlanChanXwO/pixiv-cli/releases

## Installation

> **Release status**: the Ed25519 public key, key ID, and fingerprint for supported binaries are committed in
> [`internal/update/installer/release_installer.go`](../../internal/update/installer/release_installer.go); the public source/tap repositories,
> the protected `release` Environment, and isolated credentials are configured. v0.4.4 is a published public GitHub
> Release with six platform archives, checksums, and a signed manifest. Use the official [GitHub Releases page]
> and `brew info FlanChanXwO/tap/pixiv-cli` for the current Release and tap state: they are independently published
> artifacts. Every future version must pass the same tag, signing, asset, and Homebrew gates before it is treated as
> a trusted download source.

### Official installer scripts

The repository provides two per-user bootstrap scripts for the latest stable Release:

```bash
# Linux/macOS
curl -fsSLo /tmp/pixiv-install.sh https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/install.sh
sh /tmp/pixiv-install.sh --add-to-path
```

```bat
rem Windows Command Prompt; no PowerShell dependency
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd
call "%TEMP%\pixiv-install.cmd" --add-to-path
```

`install.sh` supports Linux/macOS AMD64 and ARM64 and defaults to `$HOME/.local/bin`. `install.cmd` supports Windows
AMD64 and ARM64 and defaults to `%LOCALAPPDATA%\Programs\pixiv`. Both download `checksums.txt` and exactly one matching
archive from the official latest stable Release, verify SHA-256 before extraction, preflight the staged binary, and
only then replace `pixiv`. `--install-dir DIR` selects a different destination; `--no-path` suppresses profile/registry
changes. `--add-to-path` is restricted to `$HOME/.local/bin` on Unix and updates only the current user's `Path` value
on Windows. Neither script requests administrator/root privileges, installs prerequisites, reads Pixiv credentials,
or bypasses system reputation warnings.

Linux Release assets require glibc 2.35 or newer. The release, native-evidence, and packaged-smoke jobs build both
Linux architectures on Ubuntu 22.04 and reject any ELF whose GNU version requirements exceed `GLIBC_2.35`. The
installer binary preflight exposes a loader failure before it replaces an existing installation.

This is an initial-bootstrap boundary: before `pixiv` exists, the scripts have no embedded Ed25519 verifier. The
SHA-256 check detects corruption or a mismatched archive, while authenticity still depends on HTTPS and the official
GitHub repository/Release account. Inspect the installer before execution. Once installed, `pixiv update` uses the
binary's embedded Ed25519 trust root for subsequent Release updates.

The versioned installers embed a static Release-source list. They always download the authoritative `checksums.txt`
directly from GitHub HTTPS, then probe free candidates only for the matching platform archive; a candidate is usable
only when its checksum response is byte-for-byte identical to the direct file. The archive still requires the direct
SHA-256 match before installation. The list is never fetched remotely and changes only with a signed Release.

After a verified install, the official scripts initialize the per-user, on-demand `pixiv://` handler. Homebrew does
the same in `post_install`. A warning means the binary was installed successfully but desktop integration was not.
On macOS and Windows, the next normal `pixiv` command retries initialization; a manually extracted archive therefore
repairs its integration on first use. Desktop Linux needs both `xdg-mime` and `gio`; headless Linux supports the
relay server without registering a desktop handler.

### Build from source

```bash
sh scripts/build.sh
```

A supported source build requires Go `1.26.3`, `CGO_ENABLED=1`, a working C linker for the target platform, and a
Rust ugoira staticlib matching the target. It outputs `build/pixiv` or `build/pixiv.exe`. On Windows, run the build
command via Git Bash, MSYS2, or WSL.

The working tree ships six runner-verified staticlibs for darwin/linux/windows × amd64/arm64 plus a same-origin
`manifest.json`; `scripts/build.sh` verifies the source digest, target/path, and each library's SHA-256 before
building the native binary. See [the development guide](maintainers/development.md#rust-ugoira-staticlib) for the full
requirements, evidence backfill process, and failure semantics.

### Go install

After an official tag is published, install with the exact tag:

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@vX.Y.Z
```

This still uses the local Go toolchain, cgo, a C linker, and the committed staticlib for the target. The six target
libraries and manifest are complete — for example, the published v0.4.4 release can be installed with `@v0.4.4`;
always use the exact published tag, never a branch name.

### Homebrew

macOS/Linux users install the stable formula with:

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

A future beta/pre-release channel uses:

```bash
brew install FlanChanXwO/tap/pixiv-cli-beta
```

Both formulas install the same `pixiv` binary and therefore conflict with each other; they only download verified
macOS/Linux Release assets and introduce no `ffmpeg` dependency. The GitHub Release and public tap are separate
publication channels, so use `brew info FlanChanXwO/tap/pixiv-cli` and the [GitHub Releases page] rather than a
version hard-coded in this reference. The beta formula only ships with pre-releases.

### Direct download

The release process produces six fixed-name archives for darwin, linux, and windows on amd64/arm64:
`pixiv-cli_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows), plus `checksums.txt` and an Ed25519-signed
`checksums.json`. Published v0.4.4 ships the complete asset set; later versions should only be trusted as direct
download sources after passing the same release gates.

Current Releases do not include Apple notarization or Windows Authenticode. Even when downloading from a verified
Release, macOS Gatekeeper or Windows SmartScreen may still show a system reputation prompt; only obtain assets from
the project's GitHub Release page, verify the version, checksum, and signature notes, and never bypass warnings for
assets of unknown origin.

## Getting a refresh token

Refresh tokens are raw Pixiv App API OAuth credentials saved in the local account store.

The recommended flow is the CLI browser OAuth login, which saves directly to a local account:

```bash
pixiv auth login
```

The `auth login` flow:

| Stage | Behavior |
| --- | --- |
| Init | The CLI generates a PKCE verifier/challenge and OAuth state, and starts a local loopback HTTP server. |
| Browser | On macOS and Windows, normal CLI startup prepares the current-user `pixiv://` handler; desktop Linux initializes its XDG handler for interactive login. The CLI opens the default browser so an existing Pixiv login session can be reused. With `--no-open`, it prints the login URL and local page address. |
| Callback | The CLI accepts this round's loopback callback, a one-time desktop hand-off, a terminal paste, or the local page form. After a helper hand-off, the default browser opens the final local success or failure page once OAuth exchange completes. |
| Validation | The local loopback callback must match this round's state; Pixiv's official callback URL and `pixiv://account/login` can be used as an explicit fallback when Pixiv doesn't return a state. |
| Save | Refresh/access tokens are never printed; the refresh token is saved to the local SQLite database keyed by Pixiv UID. Unix-like systems actively use `0700` parent directories and `0600` files. On Windows, first creation inherits the parent ACL and replacement preserves the existing target ACL; the CLI does not claim to tighten or loosen the DACL. |

The handler is persistent but runs only when the OS opens `pixiv://`: macOS uses `PixivCLIURLHandler.app`, Windows a
current-user protocol association, and desktop Linux an XDG desktop entry. It records the prior handler privately.
An active local loopback bridge always wins. Without one, `pixiv://account/login` is accepted only for an active
one-time desktop hand-off, and `pixiv://account/remote-login` starts that hand-off. Other `pixiv://` URLs are launched
in the prior handler. Use the operating system's normal association UI to choose a different handler when needed.

On a headless SSH server, keep the listener on loopback and choose an unused fixed port so the local machine can
forward it. Run on the server:

```bash
pixiv auth login --no-open --addr 127.0.0.1:41871
```

Then run on the local machine in another terminal:

```bash
ssh -N -L 41871:127.0.0.1:41871 USER@SERVER
```

Open `http://127.0.0.1:41871/` in the local browser. The tunnel reaches only the server's loopback listener and does
not expose the callback port publicly. It makes the manual page reachable, but it cannot receive Pixiv's final
`pixiv://` callback on behalf of a browser-only machine. Use the one-time desktop hand-off below when the browser
machine has pixiv-cli installed. A complete final callback URL can also be pasted into the original `auth login` prompt.
Never bind the login listener to a public interface; `--addr` intentionally accepts loopback addresses only.

### Cross-machine one-time hand-off

To save the account on a server while authorizing in another browser, configure the server with
`login_relay_public_url` and `login_relay_listen_addr`. Starting `pixiv auth login` then prints one remote hand-off URL.
Opening it redirects directly to `pixiv://account/remote-login`; no pixiv-cli session, confirmation, or callback-copy
page is rendered.

On a desktop with pixiv-cli installed, the local CLI claims that one session, starts its OAuth URL, and returns the
resulting callback to the server. The hand-off exists only for that session and a new hand-off replaces the prior local
one. A client without the desktop handler cannot complete this relay flow; use a desktop that has pixiv-cli installed.

The relay can use HTTP or HTTPS. Supply a certificate/key pair for direct TLS, or use a same-host reverse proxy with
the relay listener on loopback. Legacy `login_relay_secret` and `login_relay_target_url` settings are silently ignored.
`pixiv auth devices` has been removed. `pixiv config` manages download path, filename and directory templates, request interval, and proxy settings;
advanced relay settings stay in private `config.toml`.

The system proxy used by the browser is not automatically passed to the Go CLI. If the Pixiv token endpoint needs a
proxy on your network, configure it first:

```bash
pixiv config set https_proxy http://127.0.0.1:7890
```

You can also override the proxy for a single network command:

```bash
pixiv auth login --proxy http://127.0.0.1:7890
```

`--proxy URL` and `--no-proxy` only affect the current command and are never written to `config.toml`; they cannot
be used together. `--no-proxy` clears the proxy for this command even if `https_proxy` exists in the environment or
config.

When an HTTP or HTTPS proxy is configured, media-resource transfers such as `download` (including Ugoira) deliberately use
HTTP/1.1. App API, OAuth, and Web metadata requests retain their normal protocol negotiation. This avoids
proxy-specific HTTP/2 stream resets and does not change authentication or the selected download quality.

Real login depends on the Pixiv OAuth web flow being available; automated tests use a fake OAuth server and never
touch real Pixiv.

### Importing authentication

Direct import accepts a raw Pixiv App OAuth refresh token:

```bash
pixiv auth import                         # hidden prompt on a TTY
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth import
pixiv auth import 'YOUR_REFRESH_TOKEN'    # visible in argv/shell history
```

`pixiv auth import [REFRESH_TOKEN]` validates a raw token through App OAuth, uses Pixiv's returned UID as
authoritative, and stores the rotated refresh token. With no argument, a TTY uses a hidden prompt; non-TTY stdin is
read as one opaque line, removing only one final LF or CRLF. A positional token is convenient but can be exposed by
process listings, shell history, wrappers, and audit tooling. `--json` changes only the safe account summary.
`--proxy` and `--no-proxy` apply only to this direct validation and cannot be combined.

Successful direct import reports `added uid:UID` or `updated uid:UID`; text adds `username:NAME` when available.
JSON is exactly one secret-free account item such as
`{"user_id":12345678,"username":"display name","status":"added"}`. `status` is `added` or `updated`; neither
form exposes default state, token presence, the input token, or the rotated token.

To restore an export bundle without contacting Pixiv, redirect or pipe the
bundle into the same command:

```bash
pixiv auth import < account.pxauth
pixiv auth export --all | ssh trusted-host pixiv auth import
```

With no positional token, `auth import` classifies the first non-whitespace byte: `{` selects strict versioned bundle
decode and every other value is one opaque refresh token. Bundle mode is offline, never falls back to OAuth, rejects
`--proxy`/`--no-proxy`, and atomically merges every account by UID. A malformed bundle is an error rather than a token
attempt. An explicit positional value is always a token, including one beginning with `{`. Existing accounts are
replaced, new accounts are added, the current default is preserved, and the bundle default is adopted only when the
local store has no default. Default text output lists each safe added/updated UID in input-bundle order and the
resulting default. `--json` returns
`{"accounts":[{"user_id":12345678,"username":"display name","status":"added"}],"default_user_id":12345678}`;
account items expose only `user_id`, `username`, and `status`.

### Exporting and backing up authentication

```bash
pixiv auth export                         # raw token for the default account
pixiv auth export 12345678                # raw token for one exact local account
pixiv auth export --all                   # versioned all-account bundle on stdout
pixiv auth export 12345678 --output account.pxauth
pixiv auth export --all --output accounts.pxauth
pixiv auth export --all --output accounts.pxauth --force
```

Without `--output`, exactly two forms may write secrets to stdout: default/UID export writes the raw stored token
plus one newline; `--all` writes only the versioned JSON bundle. They write no stderr on success. Export is strictly
local-only: it reads the local SQLite database, never refreshes or contacts Pixiv, never
mutates auth/config, and skips startup pending-update cleanup and automatic update checks.
`--all` cannot be combined with a UID, `--force` requires `--output`, and export accepts no JSON/proxy flags.

With `--output PATH`, both a single-account selection and `--all` write a bundle instead of a raw token. The command
refuses an existing destination unless `--force` is explicit; successful stdout contains only the output path and
account count, never a token or bundle. On Unix-like systems the destination file is `0600`; existing parent
permissions and ownership are unchanged. On Windows the file owner and protected DACL explicitly grant full control
only to the current user, LocalSystem, and builtin Administrators. CI tests cover this Windows policy; this document
does not claim the current release validation was run on a real Windows filesystem.

The bundle is an unencrypted point-in-time secret backup, not live sync. Store and transport it like the original
tokens. Token rotation can make an older bundle or a copy on another machine stale. The strict versioned codec
rejects unsupported schema/version, unknown or duplicate fields, trailing JSON, duplicate/non-positive UIDs, empty
tokens, and a default UID that does not name an included account. Top-level and account-object keys must use the
documented canonical spelling and case exactly; aliases such as `Schema`, `Default_User_ID`, `User_ID`, or
`Refresh_Token` are rejected even when a canonical key is also present.

If export selection or I/O fails, stdout never receives a secret diagnostic. If restore's atomic write fails,
`LocalWriteCommitOutcome=not_committed` means replacement did not occur; `committed` means replacement occurred but
a later durability/cleanup step failed and the store must be reloaded; `unknown` means recovery could not establish
the target state and requires inspection. Neither `committed` nor `unknown` may be treated as a successful rollback.
All other stdout/stderr, JSON, MCP results, logs, and errors remain forbidden from exposing refresh tokens. No
persistent auth import/export MCP tools are added; existing session-scoped MCP authentication behavior is unchanged.

## CLI usage

Log in and save an account first:

```bash
pixiv auth login
```

Advanced/scripted setups can also import an existing token without placing it in argv:

```bash
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth import
```

Common commands:

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
pixiv auth refresh
pixiv config path
pixiv config get download_path
pixiv config set download_path ~/Downloads/pixiv
pixiv config unset https_proxy

pixiv --version
pixiv update --check
pixiv update --check --json

pixiv search "初音ミク" --type artwork --limit 10
pixiv search "初音ミク" --type novel --json
pixiv search "artist" --type user --limit 10
pixiv search ./image.png --provider ascii2d-color --json
pixiv search https://example.com/image.png --provider all --ndjson
pixiv search --trending-tags --json
pixiv detail 123456 --type artwork --json
pixiv detail 123456 --type novel --content --json
pixiv series 42 --type artwork --limit 20
pixiv comment 123456 --type artwork --limit 20
pixiv bookmark list --type artwork --limit 20
pixiv bookmark tags --limit 20
pixiv user followers 123456 --limit 20
pixiv ranking --mode day
pixiv recommended --type all --limit 5
pixiv download 123456 789012 --output ./downloads
```

All persistent application-managed data is stored directly under the current user's home directory: `~/.pixiv-cli` on
macOS/Linux and `%USERPROFILE%\.pixiv-cli` on Windows. It contains `pixiv-cli.db`, `config.toml`, callback-bridge state,
the Release-check cache, and the macOS callback helper. Account credentials are keyed by Pixiv UID.
Unix-like systems actively use `0700` parent directories and `0600` files. On Windows, first creation inherits the
parent ACL and replacement preserves the existing target ACL; the CLI does not claim to tighten or loosen the DACL.
On the first ordinary command, a missing `config.toml` is created with the common download, output, login, and
update settings. It never overwrites an existing file. Advanced settings such as proxy, login
timeout, and the Premium-status cache are intentionally omitted until explicitly configured; help, the root
`--version` flag, secret export, and the internal OAuth callback do not create it.
Output defaults to text; commands that expose `--json` can produce machine-parseable JSON. `auth export`
deliberately does not expose that flag.
The CLI uses Cobra/pflag, so options may appear before or after positional arguments — both
`pixiv auth check 12345678 --json` and `pixiv search "初音ミク" --json` are officially supported forms.

### Data-operation contract

All non-mutating data reads, recommendations, timelines, and downloads use the local account selected by `pixiv auth use` when the account pool is disabled. Account pooling is active only when `[account_pool]` explicitly sets `enabled = true`; the database `schedulable` flag controls membership and `strategy` defaults to `round_robin` with `random` also supported. Use `pixiv auth pool status|enable|disable` to inspect or change membership. Writes, authentication, and configuration do not use the pool. Data commands reject `--uid` and `--refresh-token`.

Visual lists write canonical Record NDJSON automatically when stdout is a non-terminal and no explicit output format was selected. Each line has stable string `id`, `type`, and `url`; `download`, `bookmark add/remove`, and `follow add/remove` consume compatible records without positional IDs. `--json` and explicit `--ndjson` retain precedence.

### Reverse image search

`pixiv search SOURCE` enters image mode before any Pixiv SDK or account-pool setup:

- An input with an explicit, case-insensitive `http:` or `https:` scheme always enters image mode. Invalid URLs fail as
  reverse-search source errors and never fall back to a keyword search.
- Any other input enters image mode only when it is an existing regular file after following symlinks. Directories,
  FIFOs, devices, sockets, and other non-regular paths are not image sources; all other text remains a keyword.
- Image mode accepts `--provider`, `--json`, `--ndjson`, `--proxy`, and `--no-proxy`. Search filters, `--type`,
  pagination, and `--trending-tags` are rejected rather than ignored.

The provider values are `saucenao`, `ascii2d-color`, `ascii2d-bovw`, and `all`. The configured default is
`saucenao`; `--provider` overrides it for one invocation. `all` runs the providers in the fixed order SauceNAO,
ascii2d color, ascii2d bovw and may return a partial success. `reverse_search_pixiv_only` controls whether
non-Pixiv matches remain in `results`; it defaults to `true` and is not a per-command flag.

The source is fetched or opened once into a private temporary snapshot and represented in output only by
`input.kind` (`file` or `url`) and `input.sha256`. SauceNAO and ascii2d are third-party services: the image may be
uploaded or retained according to their policies, and URL requests may be cached. ascii2d accepts JPEG, PNG, and
WEBP and applies its provider-specific 10 MB upload limit; this does not impose a common limit on SauceNAO-only
search. Do not submit images or URLs that the user is not authorized to share.

JSON output is the complete envelope `{input, providers, results, records, provider_errors, partial}`. `records`
contains only canonical Pixiv identities: artwork matches use the generic `type:"artwork"` because reverse search
does not know the artwork subtype, while user matches use `type:"user"`. The CLI does not call Pixiv detail merely
to infer a subtype. External-only matches can remain in `results` when Pixiv-only filtering is disabled but do not
become records. Human output is a safe summary; piped or explicit NDJSON emits only those canonical records.

For `all`, a successful provider plus a failed provider sets `partial=true`, writes a safe warning to stderr, and
exits successfully. A single-provider failure or an all-provider failure exits non-zero while preserving any
available JSON envelope. Provider errors use stable `code` and `message` values; source strings, API keys, cookies,
CSRF/redirect values, temporary paths, and upstream response bodies never appear in output or diagnostics.

Public positional inputs also accept one implicit non-TTY stdin value. A missing required value or an omitted optional
value consumes the complete stream as one value and removes only one trailing LF/CRLF; it never splits shell whitespace.
For example, `printf '%s\n' 13214141 | pixiv search` is equivalent to `pixiv search 13214141`. Explicit positional
arguments take precedence and do not read stdin. Download and mutation commands classify an implicit stream by its
first non-whitespace byte: `{` selects strict streaming canonical NDJSON, otherwise the complete stream is one raw
ID/URL. `-` has no stdin sentinel meaning and is passed as ordinary text.

The canonical data actions are `search`, `detail`, `ranking`, `series`, `comment`, `bookmark`, `download`, `user`, `timeline`, `mypixiv`, and `recommended`. The common short options are `-t/--type`, `-p/--page`, `-l/--limit`, `-o/--output` (download directory), and `-j/--json` where the command supports that semantic. They are option spellings, not command aliases. For example, `pixiv timeline latest --type artwork` is the canonical latest artwork feed. `novel search`, `user search`, and the root `follow` command remain compatibility routes and must map to the same application use cases.

Only the structured entity filters documented by each command are accepted. The CLI does not publish an ignored top-level expression filter. Ugoira downloads currently accept `--ugoira-mode gif|apng` (`gif` by default); page selection and non-original quality remain unsupported for Ugoira.

### CLI command table

| Command | Usage | Description |
| --- | --- | --- |
| `auth import` | `pixiv auth import [REFRESH_TOKEN] [--json] [--proxy URL\|--no-proxy]` | Direct input validates and stores the rotated token; no-argument TTY input is hidden and non-TTY input auto-detects an opaque token or strict offline bundle. Explicit values are always tokens; bundle mode conflicts with proxy flags. |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--relay-public-url URL --relay-listen-addr ADDR] [--relay-tls-cert-file PATH --relay-tls-key-file PATH] [--proxy URL\|--no-proxy]` | Uses ordinary loopback OAuth. With a complete relay server configuration, it prints a one-time hand-off URL that directly starts an installed desktop CLI handler. It saves by Pixiv UID and never prints the refresh token. |
| `auth list` | `pixiv auth list [--json]` | Lists local accounts; never prints refresh tokens. Text output uses `*` for the default and `✓`/`-` for whether a local refresh token is stored/missing. These are local-state markers, not an online validity claim. |
| `auth pool` | `pixiv auth pool status [--json]`; `pixiv auth pool enable UID... [--all]`; `pixiv auth pool disable UID... [--all]` | Shows or changes non-secret database scheduling state. `status` reports `enabled`, `strategy`, `schedulable`, `frozen_until`, and current `eligible`; enable/disable validates every UID before committing the batch. |
| `auth export` | `pixiv auth export [UID] [--all] [--output PATH] [--force]` | Exports a default/exact account or all accounts locally. Without `--output`, one account is raw token stdout and `--all` is bundle stdout; with `--output`, both write a private bundle and only a safe summary goes to stdout. `--force` requires `--output`. |
| `auth use` | `pixiv auth use [UID] [--json]` | Sets the default account; interactive selection on a TTY. |
| `auth remove` | `pixiv auth remove [UID] [--yes] [--json]` | Removes an account; confirms by default on a TTY. After removing the default account, the first remaining account is selected automatically. |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL\|--no-proxy]` | Refreshes the token and validates the account; on success records `user_id` and the username when available. |
| `auth refresh` | `pixiv auth refresh [UID] [--all] [--json] [--proxy URL\|--no-proxy]` | Refreshes the selected/default saved account's OAuth access token and rotated refresh token, then forces a profile read to update its cached Pixiv Premium status. `--all` refreshes every stored account. JSON always returns `accounts`. |
| `config path` | `pixiv config path` | Prints the `config.toml` path, creating the baseline file if it is missing. |
| `config get` | `pixiv config get KEY` | Prints one effective config value. |
| `config set` | `pixiv config set KEY [VALUE]` | Writes one known config key, including `account_pool_enabled`, `account_pool_strategy`, `download_path`, `filename_template`, `directory_template`, `request_interval`, `https_proxy`, `log_level`, `log_format`, `reverse_search_provider`, `reverse_search_pixiv_only`, and the stdin-only `saucenao_api_key`. |
| `config unset` | `pixiv config unset KEY` | Deletes one known config key from `config.toml`. |
| `update` | `pixiv update [--check] [--prerelease] [--proxy URL]` | Checks for or performs an update matching the current install source; `--json` is only valid together with `--check`. |
| `search` | `pixiv search [WORD\|IMAGE_PATH_OR_URL] [-t artwork\|novel\|user] [options]` | Canonical entity search or automatic reverse-image search. A regular file or explicit HTTP(S) source selects image mode; `--trending-tags` is the no-word artwork tag-list mode and does not accept search filters or pagination. |
| `detail` | `pixiv detail ID_OR_URL [-t artwork\|novel\|user] [--content] [--json]` | Reads one artwork, novel, or user. `--content` is explicit and valid only for novels. |
| `ranking` | `pixiv ranking [--mode MODE --date YYYY-MM-DD --page N --limit N]` | Reads illustration rankings. Novel ranking is not part of the v1 contract. |
| `series` | `pixiv series SERIES_ID -t artwork\|novel [--page N --limit N --json\|--ndjson]` | Lists the artworks or novels in one series. The entity type is required. |
| `comment` | `pixiv comment ID -t artwork\|novel [--page N --limit N --json\|--ndjson]` | Reads artwork or novel comments. Comment write/reply/delete/stamp is not exposed. |
| `bookmark` | `pixiv bookmark list\|tags\|detail\|add\|remove ...` | Lists artwork/novel bookmarks, reads artwork bookmark tags/detail, or mutates artwork bookmarks. `list` uses `--type artwork\|novel`; `tags` is artwork-only. |
| `user` | `pixiv user search\|detail\|artworks\|novels\|bookmarks\|following\|followers\|related\|blocked\|follow ...` | Reads user/profile/relationship data and manages artwork follows. Omitted user IDs use the current account only where that subcommand says so. |
| `download` | `pixiv download [options] SRC...` | Downloads artwork IDs/URLs, allowed CDN URLs, or visual works expanded from supported user and public-bookmark URLs. Artwork-series URLs are not download sources. `--output/-o` aliases `--download-path`. |
| `timeline` | `pixiv timeline following\|latest -t artwork\|novel [--content-type TYPE ...]` | Reads followed-user or latest artwork/novel streams. Artwork subtype is a separate `--content-type` option. |
| `mypixiv` | `pixiv mypixiv users\|works [-t artwork\|novel ...]` | Reads MyPixiv users and artwork/novel feeds. |
| `recommended` | `pixiv recommended [-t artwork\|novel\|user\|all] [--page N --limit N --json]` | Reads personalized recommendations. Positional `KIND` remains accepted for compatibility; `all` keeps each entity stream separate in the result. |
| `novel search` | `pixiv novel search WORD [options]` | Compatibility route for novel search; prefer `pixiv search WORD --type novel`. It exposes only the documented basic novel search fields. |
| `user search` | `pixiv user search WORD [options]` | Compatibility route for user search; prefer `pixiv search WORD --type user`. |
| `follow` | `pixiv follow add\|remove USER_ID ...` | Compatibility route for user follow mutation; prefer `pixiv user follow add\|remove`. |
| `mcp` | `pixiv mcp [--proxy URL\|--no-proxy]` | Starts the MCP stdio server; the proxy override applies only to this launch. |
| `fanbox auth` | `pixiv fanbox auth import|list|use|remove|status` | Imports and manages local FANBOX sessions. Session values are never printed. Native `--proxy`/`--no-proxy` applies only to the FANBOX command. |
| `fanbox creators` | `pixiv fanbox creators [--kind supporting\|following] [--page N --limit N]` | Lists supporting or following FANBOX creators. |
| `fanbox posts` | `pixiv fanbox posts SOURCE [--page N --limit N]` | Lists posts from a creator, tag, post ID, or supported FANBOX URL. |
| `fanbox tags` | `pixiv fanbox tags CREATOR` | Lists featured tags used by a creator. |
| `fanbox home` / `supporting` | `pixiv fanbox home|supporting [--page N --limit N]` | Reads the authenticated FANBOX home or supporting feed. |
| `fanbox post` | `pixiv fanbox post POST_ID` | Reads one post and its safe asset summary. |
| `fanbox download` | `pixiv fanbox download SOURCE...` | Saves FANBOX post assets below the configured download path. |
| `fanbox mcp` | `pixiv fanbox mcp [--proxy URL\|--no-proxy]` | Starts the read-only FANBOX MCP stdio server; the native proxy override does not alter FlareSolverr settings. |

Downloaded filenames normalize cross-platform-invalid characters in both the filename template and URL-derived
extension. For Pixiv thumbnail artwork, a successful resource Content-Type may replace an ambiguous URL extension
such as `.png` so the published filename matches the actual image bytes. Unknown media types retain the URL
extension. Extensions also replace ASCII control characters and remove trailing dots or spaces rejected by Windows.

### `auth login` flags

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Prints the save result as JSON; never prints refresh/access tokens. |
| `--no-open` | `false` | Prints the login URL and local loopback page address for manual opening. |
| `--addr` | `127.0.0.1:0` | Local loopback listen address; port `0` means auto-assign. |
| `--use` | `false` | Sets the account as default after a successful login; also becomes default automatically when no default exists. |
| `--timeout` | `0` | Maximum time to wait for login completion; `0` means the CLI imposes no deadline. |
| `--relay-public-url` | config | Public HTTP(S) base URL of this server relay for one login. |
| `--relay-listen-addr` | config | Listen host:port of this server relay for one login. |
| `--relay-tls-cert-file` / `--relay-tls-key-file` | config | PEM pair for direct TLS; both must be provided. Without them, an HTTPS public URL requires a loopback listener for a same-host reverse proxy. |
| `--proxy URL` / `--no-proxy` | empty | Proxy override for this token exchange only; never saved to `config.toml`. |

### Data command flags

| Command | Flag | Default | Description |
| --- | --- | --- | --- |
| `search` | `--type` / `-t` | `artwork` | Entity route: `artwork`, `novel`, or `user`. Artwork subtype is a separate `--content-type`; `illust` is not an entity value. |
| `search` | `--provider` | `reverse_search_provider` (`saucenao`) | Reverse-image provider: `saucenao`, `ascii2d-color`, `ascii2d-bovw`, or `all`; valid only for image sources and overrides the config for this invocation. |
| `search` | `--content-type` | `all` | Artwork subtype: `all`, `illust-and-ugoira`, `illust`, `manga`, or `ugoira`; artwork search only. |
| `search`, `novel search` | `--search-by` | `tag-partial` | Artwork search accepts `tag-partial`, `tag-exact`, `title-caption`, and `tag-title-caption`; novel search accepts the first three only. |
| `search`, `novel search` | `--sort` | `date_desc` | Sort order: `date_desc` or `date_asc`. |
| `search` | `--period` | empty | Artwork range: `day`, `week`, `month`, `half-year`, or `year`; mutually exclusive with `--start-date`/`--end-date`. |
| `novel search` | `--period` | empty | Novel range: `day`, `week`, or `month`. |
| `search` | `--start-date` / `--end-date` | empty | Inclusive `YYYY-MM-DD` date bounds; when both are present, start cannot be later than end. Artwork search only. |
| `search` | `--rating` | empty | Compatibility flag only. Any non-empty value is rejected because the v1 App API search request has no verified rating field; it never filters results. |
| `search` | `--ai-mode` | `all` | Artwork AI filter: `all`, `exclude`, or `only`; Pixiv `AIType==2` is AI-generated. |
| `search` | `--aspect-ratio` | `all` | Artwork aspect ratio: `all`, `landscape`, `portrait`, or `square`. |
| `search` | `--resolution` | `all` | Artwork resolution tier: `all`, `high`, `medium`, or `low`. |
| `search` | `--draw-tool` | empty | Exact drawing-tool name from the versioned catalog below; ambiguous values are rejected. |
| `search` | `--bookmark-min` / `--bookmark-max` | empty | Inclusive non-negative public bookmark-count bounds. Application filtering uses `TotalBookmarks`; `min` cannot exceed `max`. The result metadata identifies the strategy and completeness. |
| `search` | `--bookmark-strategy` | `auto` | `auto` currently resolves to `local`; `local` performs exact filtering on fetched candidates; `best_effort` retains App candidate bounds and reports partial completeness; `server` fails explicitly until reliable server-side evidence exists. |
| `novel search` | advanced rating/text/original flags | unsupported | `--rating`, `--min-text-length`, `--max-text-length`, and `--original-only` are not part of the v1 compatibility command. Do not send them or treat them as local filters. |
| list commands | `--limit` / `-l` | one upstream batch | Omit for one upstream batch; a positive value fills the logical page across upstream batches; `0` traverses until the upstream cursor ends. |
| list commands | `--page` / `-p` | empty | 1-based logical page; must be used with a positive `--limit`. |
| `ranking` | `--mode` | `day` | One of `day`, `day_male`, `day_female`, `week`, `week_original`, `week_rookie`, `month`, `day_manga`, `week_manga`, `month_manga`, `week_rookie_manga`, `day_r18`, `day_male_r18`, `day_female_r18`, `week_r18`, `week_r18g`. The final nine require authentication. |
| `ranking` | `--date` | empty | Ranking date, typically `YYYY-MM-DD`. |
| `detail` | `--type` / `-t` | `artwork` | Entity type: `artwork`, `novel`, or `user`; `--content` is valid only with `novel`. |
| `series`, `comment` | `--type` / `-t` | required | Entity type: `artwork` or `novel`; the ID is interpreted only after the type is selected. |
| `bookmark list` | `--type` / `-t` | `artwork` | Entity type: `artwork` or `novel`; `--restrict` and `--tag` are passed to the matching bookmark list. |
| `bookmark tags` | `--type` / `-t` | `artwork` | Artwork bookmark tags only; `--restrict` selects public/private tags. |
| `user artworks` | `--type` | `illustration` | Artwork subtype: `illust`, `manga`, or `ugoira`. |
| `user bookmarks` | `--restrict`, `--tag` | `public`, empty | Bookmark visibility and exact bookmark-tag filter. |
| `user following`, `user followers` | `--restrict` | `public` | Follow visibility: `public` or `private`. |
| `timeline following` | `--type` / `-t`, `--content-type` | required, `all` | Entity type is `artwork` or `novel`; artwork subtype is separate and `--restrict` is `public` or `private`. |
| `timeline latest` | `--type` / `-t`, `--content-type` | required, `illust` | Entity type is `artwork` or `novel`; the latest-artwork endpoint supports `illust` or `manga`, and omitted `--content-type` selects `illust`. |
| `mypixiv works` | `--type` / `-t` | required | Without `USER_ID`, use entity type `artwork` or `novel`; with `USER_ID`, `manga` is also supported. Legacy `illust` remains an alias for `artwork`. |
| `recommended` | `--type` / `-t` | empty | `artwork`, `novel`, `user`, or `all`; positional `KIND` is compatibility syntax. |
| record actions | `--on-error` | `skip` | Skip malformed/incompatible records with a stderr diagnostic, or use `fail-fast`. |
| `download` | `--pages` | empty | 1-based closed page selection such as `1,3-5`; default downloads every page. Missing pages fail explicitly. |
| `download` | `--quality` | `original` | Static image quality: `original`, `regular` (longest side 1200), `small` (longest side 540), `thumb` (250×250 center crop), or `mini` (48×48 center crop). Ugoira rejects non-original quality or page selection as unsupported.
| `download` | `--ugoira-mode` | `gif` | Ugoira output: `gif` or `apng`. |
| `download` | `--download-path` / `--output` / `-o` | `DOWNLOAD_PATH`, `config.toml`, or `./downloads` | Download directory. `--output` is an alias for this option and conflicts if both specify different directories. |
| `download` | `--filename-template` | `FILENAME_TEMPLATE`, `config.toml`, or `{author} - {title}_{id}` | Supports `{id}`, `{title}`, `{author}`, `{author_id}`, `{date}`, `{tags}`, and `{num}`. Unknown placeholders and unmatched braces are errors. |
| `bookmark add` | `--restrict` | `public` | Visibility of the new bookmark: `public` or `private`. |
| `bookmark add` | `--tag` | empty | Bookmark tag; may be repeated. |
| `follow add` | `--restrict` | `public` | Visibility of the new follow: `public` or `private`. |
| `download` | `SRC...` | required | Artwork PID, artwork URL, allowed CDN resource URL, user profile/artworks URL, or public bookmarks URL. Artwork-series URLs are not download sources. CDN files use the URL filename; metadata-dependent options do not apply. |

All Pixiv content reads use the authenticated local account selected by `pixiv auth use` (or the eligible account
pool) and the App API. App failures are final; the CLI does not fall back to an anonymous Web/API path. Search
filters are bound to the opaque SDK cursor, and logical `--page`/`--limit` traversal continues across upstream
batches until the requested logical results are filled or the upstream cursor ends. Omitting `--limit` reads one
upstream batch; `--limit 0` traverses the current upstream result until exhaustion. A positive `--page` requires a
positive `--limit`.

`--rating` is retained only as a compatibility diagnostic. Passing any value returns an unsupported usage error
before the SDK request; it is not a filter. Artwork `--bookmark-min`/`--bookmark-max` are inclusive non-negative
conditions on public `TotalBookmarks`. The application reports the selected strategy and completeness: `auto`
currently uses exact local filtering over fetched candidates, `local` has the same behavior, `best_effort` keeps
App candidate bounds but reports partial completeness, and `server` fails explicitly because reliable server-side
evidence is not yet available. Premium is not a local hard gate, and bookmark counts are not like counts.

Artwork JSON/NDJSON preserves public entity data and an opaque resource reference where needed; it does not emit
resolved/signed resource URLs, request headers, Cookies, expiry metadata, tokens, or other transport credentials.
Download is an action: success keeps stdout empty, while failures are explicit diagnostics and a non-zero exit.

### Drawing-tool catalog

`--draw-tool` and MCP `tool` require one exact catalog value. The catalog is part of this version's public contract;
it is not expanded by command help or error output.

```text
SAI · Photoshop · CLIP STUDIO PAINT · IllustStudio · ComicStudio · Pixia · AzPainter4 · Painter · Illustrator · GIMP
FireAlpaca · 網上描繪 · AzPainter · CGillust · 描繪聊天室 · 手畫博克 · MS_Paint · PictBear · openCanvas · PaintShopPro
EDGE · drawr · COMICWORKS · AzDrawing · SketchBookPro · PhotoStudio · Paintgraphic · MediBang Paint · NekoPaint · Inkscape
ArtRage · AzDrawing4 · Fireworks · ibisPaint · AfterEffects · mdiapp · GraphicsGale · Krita · kokuban.in · RETAS STUDIO
emote · 4thPaint · ComiLabo · pixiv Sketch · Pixelmator · Procreate · Expression · PicturePublisher · Processing · Live2D
dotpict · Aseprite · Pastela · Poser · Metasequoia · Blender · Shade · 3dsMax · DAZ Studio · ZBrush
Comi Po! · Maya · Lightwave3D · 六角大王 · Vue · SketchUp · CINEMA4D · XSI · CARRARA · Bryce
STRATA · Sculptris · modo · AnimationMaster · VistaPro · Sunny3D · 3D-Coat · Paint 3D · VRoid Studio · 筆芯筆
鉛筆 · 原子筆 · 毫筆 · 顏色鉛筆 · Copic麥克筆 · 沾水筆 · 透明水彩 · 毛筆 · 記號筆 · 麥克筆
水溶性彩色铅笔 · 涂料 · 丙烯顏料 · 鋼筆 · 粉彩 · 噴筆 · 顏色墨水 · 蠟筆 · 油彩 · COUPY-PENCIL · 顏彩
```

### Illustration tag-query syntax

The authenticated App API has been verified for illustration `search` when `--search-by` selects a tag mode. Use
`tag-exact` for boolean tag filtering: `tagA tagB` means both complete tags are required (AND), and
`tagA OR tagB` means either complete tag is accepted (OR). `OR` must be uppercase. The literal word `AND` is not a
verified operator; write the two tags with a space instead.

`tag-partial` (the default) also accepts the tested uppercase `OR` syntax, but each term is a fuzzy tag condition.
Its results must not be described as a strict exact-tag AND: a result may match a partial, alias, or translated tag
without visibly listing the supplied complete label. `title-caption` and App-only `tag-title-caption` have no documented boolean-tag contract. No
escape syntax for a literal uppercase `OR` tag/keyword is verified, so use exact tags without that token when a
strict query is required.

`novel search` is App-only and exposes keyword target, sort, duration, pagination, and the documented search target
values. Rating, text-length, and original-only flags are not part of this v1 contract. Novel detail and content
are separate requests: `detail --type novel` returns metadata and `detail --type novel --content` reads structured
blocks. The content is not data-layer truncated.

`detail --type artwork` accepts a positive artwork ID or a canonical HTTPS `pixiv.net`/`www.pixiv.net` artwork URL
in the form `/artworks/{id}` (an optional locale segment, query, and fragment are allowed). `detail --type novel`
and `detail --type user` require positive numeric IDs. User and novel URLs are not silently interpreted as artwork
URLs; unsupported URL shapes fail locally.

`download` accepts the same artwork references, allowed CDN URLs, plus `/users/{id}`, `/users/{id}/artworks`,
and `/users/{id}/bookmarks/artworks` URLs. User and public-bookmarks sources follow pagination for `illust`,
`manga`, and `ugoira`, deduplicating artwork IDs by first appearance. Artwork-series URLs are rejected as unsupported
download sources. All need App OAuth and have no anonymous Web fallback. URL
parsing is local only: it does not fetch HTML or follow redirects. The current CLI download contract exposes page
selection, static quality, GIF/APNG mode, output directory, filename template, and `--on-error`; unsupported
options fail as unknown flags. Downloads continue after independent artwork failures according to `--on-error` and
report every outcome through diagnostics; cancellation stops immediately.

### Common flags

| Flag | Applies to | Default | Description |
| --- | --- | --- | --- |
| `--ndjson` | data list/read commands | `false` | Emits one canonical Record per line for streaming filters and actions; cannot be combined with `--json`. |
| `--json` | safe data reads, auth summaries, `update --check` | `false` | Emits one complete result document where the command exposes it. Download and mutation actions do not emit a success report. |
| `--proxy URL` | network commands and `mcp` | `https_proxy`/`HTTPS_PROXY`, `config.toml`, or empty | Uses an `http`, `https`, `socks5`, or `socks5h` proxy URI for this command only; forbidden with bundle-form `auth import`. |
| `--no-proxy` | same as `--proxy` | empty | Clears the proxy for this command; cannot be combined with `--proxy` or bundle restore. |

### CLI-managed `config` aliases

`pixiv config get/set/unset` accepts only the aliases in this table. All other runtime settings are hand-maintained in the private `config.toml`; the CLI does not expose a generic setting editor.

| KEY | Type | Default | Description |
| --- | --- | --- | --- |
| `account_pool_enabled` | boolean | `false` | Enables database-backed account-pool selection for safe read/download operations. |
| `account_pool_strategy` | string | `round_robin` | Account-pool strategy: `round_robin` or `random`. |
| `download_path` | string | `./downloads` | Download directory. |
| `filename_template` | string | `{author} - {title}_{id}` | Filename template. |
| `directory_template` | string | empty | Relative download directory template. |
| `request_interval` | duration | `0` | Minimum interval between network request starts; configure it with `PIXIV_REQUEST_INTERVAL` or `[network].request_interval`. |
| `https_proxy` | string | empty | Global proxy URI (`http`, `https`, `socks5`, or `socks5h`); the lowercase `https_proxy` environment variable takes precedence. |
| `log_level` | string | `info` | Diagnostic level: `info` is silent; `debug` enables typed stderr diagnostics. The value is written as `[logging].level`. |
| `log_format` | string | `text` | Diagnostic stderr format: `text` or one JSON event per line. The value is written as `[logging].format`. |
| `reverse_search_provider` | string | `saucenao` | Default reverse-image provider: `saucenao`, `ascii2d-color`, `ascii2d-bovw`, or `all`. |
| `reverse_search_pixiv_only` | boolean | `true` | Keeps only explicitly identified Pixiv artwork/user matches in the canonical result set; external evidence can still remain in `results` when disabled. |
| `saucenao_api_key` | sensitive string | unset | SauceNAO credential. `config get` always prints `<redacted>`; `config set` accepts the value only from non-TTY stdin, never as an argument. |

The first command that needs configuration creates a compact baseline `config.toml` from the current schema.
It includes the core download, output, login, update, and logging defaults; advanced settings such as
`directory_template` and `request_interval` remain omitted until explicitly configured. Existing files are never
overwritten. Configuration is read when the command starts, so a change applies on the next invocation.

Set the SauceNAO key through a private terminal or an authorized secret-manager pipeline, not an argument or chat:

```bash
printf '%s\n' 'YOUR_SAUCENAO_API_KEY' | pixiv config set saucenao_api_key
pixiv config get saucenao_api_key    # always prints <redacted>
```

The environment variable `SAUCENAO_API_KEY` overrides the file value without displaying its contents. Do not put the
key in shell history, diagnostics, JSON, MCP input, or a checked-in TOML file.

Manual TOML may contain advanced runtime sections such as `[account_pool]`, `[network]`, `[pixiv.network]`,
`[fanbox.network]`, `[fanbox.flaresolverr]`, `[reverse_search]`, `[login]`, and `[update]`:

```toml
[network]
https_proxy = "http://global-proxy.example:7890"

[pixiv.network]
proxy_url = "socks5h://pixiv-proxy.example:1080"

[fanbox.network]
proxy_url = ""                    # explicit direct native FANBOX access
user_agent = "my-native-agent/1.0"

[fanbox.flaresolverr]
url = "http://127.0.0.1:8191"
proxy_url = "socks5://solver-upstream.example:1080"

[reverse_search]
provider = "saucenao"
pixiv_only = true
```

`[pixiv.network].proxy_url` and `[fanbox.network].proxy_url` preserve absent versus explicit empty:
the command override (`--proxy`/`--no-proxy`) wins, then the corresponding service value, then the global
environment/config proxy, then direct access. FANBOX native accepts only userinfo-free HTTP(S) CONNECT;
Pixiv accepts HTTP(S), SOCKS5, and SOCKS5H. `user_agent` changes only the FANBOX native header and does not
change its Chrome 146 TLS profile or guarantee Cloudflare access. FlareSolverr is optional and challenge-only;
its service URL and upstream proxy are independent from the native FANBOX proxy. The default config generator
does not create any of these optional tables.

`[account_pool]` stores only `enabled` and `strategy`; per-account `schedulable`, freeze, and marker state lives
in `pixiv-cli.db`. The removed `account_pool.accounts` key is not migrated; if present, runtime configuration
returns `removed_setting` and it must be cleared explicitly with `pixiv config unset account_pool_accounts`.
Never put a refresh token in `config.toml`. The historical
`data/account-pool.json` scheduler is not read, migrated, or deleted automatically. Legacy `[logging]` tables
are no longer ignored: `[logging].level` accepts `info` or `debug`, and `[logging].format` accepts `text` or
`json`. `PIXIV_LOG_LEVEL` and `PIXIV_LOG_FORMAT` override the file values. Debug diagnostics are emitted only to
stderr, omit query strings, headers, cookies, tokens, response bodies, and proxy userinfo, and never create log
files; `config` management and secret export remain quiet, while MCP stdout remains JSON-RPC only.

The v1 CLI does not read or migrate a legacy `~/.pixiv-cli/auth.json`. Before switching from an older CLI, run
`pixiv auth export --all --output <private bundle>` with the old version, then run `pixiv auth import < bundle.json`
with v1. The transfer is explicit so a stale or unexpected local file cannot become an implicit credential source.

### Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `DOWNLOAD_PATH` | `./downloads` | Download directory. |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | Filename template. |
| `DIRECTORY_TEMPLATE` | empty | Relative download directory template. |
| `PIXIV_REQUEST_INTERVAL` | empty | Minimum interval between network request starts. |
| `PIXIV_LOG_LEVEL` | `info` | Diagnostic level: `info` or `debug`; overrides `[logging].level`. |
| `PIXIV_LOG_FORMAT` | `text` | Diagnostic stderr format: `text` or `json`; overrides `[logging].format`. |
| `SAUCENAO_API_KEY` | empty | SauceNAO credential; overrides the private config value and is never printed. |
| `https_proxy` / `HTTPS_PROXY` | empty | Proxy URI (`http`, `https`, `socks5`, or `socks5h`); the lowercase `https_proxy` takes precedence. |

CLI data commands select the explicit/default account through `pixiv auth use` when pooling is disabled, or an eligible database account when pooling is enabled; they do not accept credential-selection flags.

Settings precedence is service-scoped: command `--proxy URL` / `--no-proxy` > the corresponding service
proxy key (including an explicit empty value) > `https_proxy`/`HTTPS_PROXY` > `[network].https_proxy` > direct.
CLI proxy overrides are never persisted. Update checks use only their general network fallback and never consume
FANBOX service or solver settings.

### Removed anonymous web fallback

v1 removed the anonymous Web API fallback. Content commands require an
authenticated local account selected through `pixiv auth use` or a manual
`[account_pool]` or `pixiv auth use`; without one they return an authentication requirement. The
removed `web_fallback_enabled` setting returns `removed_setting` if still
present in `config.toml`, and `pixiv config unset web_fallback_enabled` clears
it.

Invalid tokens and App API network or server errors return a safe, classified
failure.

## Version and updates

`pixiv --version` is the only public version surface. It writes exactly one line, `pixiv <version>`, to stdout,
writes no stderr, and does not run startup update checks.
The former `version` subcommand, its `--json` form, and the public `commit`/`build_date` fields were removed as a
breaking change; they now produce a non-zero unknown-command error with empty stdout. Scripts must migrate to the root flag.

```bash
pixiv --version
```

Explicit updates check first, then install; the check supports JSON while the actual install does not accept
`--json`:

```bash
pixiv update --check
pixiv update --check --json
pixiv update --check --prerelease
pixiv update --proxy http://127.0.0.1:7890
```

Development builds show `dev` and refuse to self-update. For official installs, the updater detects Homebrew
stable/beta, `go install`, or a Release binary: stable/beta switch between the two conflicting formulas according
to `--prerelease`; if the switch install fails, it explicitly attempts to restore the original formula and reports
both the original error and the recovery result. `go install` uses the exact Release tag; Release binaries verify
the Ed25519-signed checksum manifest and the archive SHA-256 before downloading, then require an exact
`pixiv --version` match and atomically replace the executable.

Unless an explicit `--proxy`, configured `https_proxy`, or `HTTPS_PROXY` is in effect, Release-binary updates probe
the embedded source list concurrently. API-capable candidates are used for the GitHub Releases API; archive-capable
candidates are used for the signed manifest, checksum, and platform archive. The first valid response becomes the
preferred route, and an asset download may silently try each remaining declared route once; if all fail, the reported
error names every failed route. Candidates never alter canonical Release URLs, SemVer selection, Ed25519 verification,
or SHA-256 verification. The automatic notification uses only API-capable candidates and retains its existing shared
three-second limit and 24-hour cache.

Update checks only select canonical SemVer tags. Stable checks first exclude GitHub Releases marked as prerelease;
`--prerelease` includes them in the current channel. If any non-draft published Release in that channel uses a
non-SemVer tag, the check reports that tag and fails closed.

Supported binaries embed the production Ed25519 public key/key ID/fingerprint; the private key exists only in the
protected `release` Environment and a controlled macOS Keychain recovery copy. Select the currently published signed
Release from the [GitHub Releases page]; `pixiv update --check` remains a read-only check and is not a substitute for
verifying the selected version's assets, checksums, and signatures at install time.

Successful regular CLI commands make a best-effort stable-update check. It skips MCP, help and root `--version`, `update`, every `auth export`, and bundle-form `auth import`,
and development builds, queries at most once per 24 hours per user cache, and caps the automatic check at 3
seconds. A discovered new version or a failed check only writes to stderr (failures as warnings), never changes
the business command's exit code, and never pollutes JSON stdout or MCP JSON-RPC stdout. To disable the automatic
check:

```bash
# ~/.pixiv-cli/config.toml
[update]
check_enabled = false
```

## Related documentation

This reference intentionally stops at the CLI boundary. Use the authoritative guides for other interfaces and
maintainer workflows:

- [Go SDK](sdk.md): public client, models, pagination, resources, and typed errors.
- [MCP tools](mcp-tools.md): tool names, input schemas, output, and stdio behavior.
- [Architecture](maintainers/architecture.md): package responsibilities and runtime flow.
- [Development](maintainers/development.md): environment, tests, builds, and release gates.
- [Agent skill](../../skills/pixiv-cli/SKILL.md): safe instructions for an agent driving the installed CLI.
