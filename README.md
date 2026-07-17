# Pixiv CLI / MCP / Go SDK

English | [简体中文](README.zh-CN.md)

A Pixiv toolset in Go: use it as the `pixiv` CLI by default, run `pixiv mcp` explicitly when you need MCP; Go programs can import the public `pixiv` package (`github.com/FlanChanXwO/pixiv-cli/pixiv`).

It prefers the Pixiv App API and supports search, artwork detail, rankings, recommendations, downloads, multi-account refresh token management, and an MCP stdio server. When no refresh token is configured, it enables an anonymous Pixiv web/ajax API fallback for search, detail, ranking, user search, and download by default. It is not an HTTP service and does not provide Discover, Probe, Capabilities, RSS, or a crawler.

The source is split into CLI controller, application services, bootstrap, public SDK, config, Pixiv facade/protocol adapters, download, and MCP server packages; accounts are stored in `internal/storage/auth`, and shared utilities live under `internal/utils/*` subpackages. The public SDK is a concrete `*pixiv.Client`; internal protocol implementations are split into `internal/pixiv/appapi`, `webapi`, `oauth`, and `resource`.

User-visible changes are recorded in [CHANGELOG.md](CHANGELOG.md). See [docs/sdk.md](docs/sdk.md) for the SDK contract and [ADR 0009](docs/adr/0009-public-pixiv-sdk-and-caller-adapter.md) for architecture boundaries.

## Installation

> **Release status**: the Ed25519 public key, key ID, and fingerprint for supported binaries are committed in
> [`internal/bootstrap/release_trust.go`](internal/bootstrap/release_trust.go); the public source/tap repositories,
> the protected `release` Environment, and isolated credentials are configured. v0.3.0 has been published as an
> official GitHub Release with six platform archives, checksums, and a signed manifest; the stable Homebrew formula
> has been pushed. Future versions must still pass the same tag, signing, asset, and Homebrew gates before they can
> be treated as trusted download sources.

### Build from source

```bash
sh scripts/build.sh
```

A supported source build requires Go `1.26.3`, `CGO_ENABLED=1`, a working C linker for the target platform, and a
Rust ugoira staticlib matching the target. It outputs `build/pixiv` or `build/pixiv.exe`. On Windows, run the build
command via Git Bash, MSYS2, or WSL.

The working tree ships six runner-verified staticlibs for darwin/linux/windows × amd64/arm64 plus a same-origin
`manifest.json`; `scripts/build.sh` verifies the source digest, target/path, and each library's SHA-256 before
building the native binary. See [the development guide](docs/development.md#rust-ugoira-staticlib) for the full
requirements, evidence backfill process, and failure semantics.

### Go install

After an official tag is published, install with the exact tag:

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@vX.Y.Z
```

This still uses the local Go toolchain, cgo, a C linker, and the committed staticlib for the target. The six target
libraries and manifest are complete — for example, the current official release can be installed with `@v0.3.0`;
always use the exact tag for later versions, never a branch name.

### Homebrew

Once the official stable Release and the real tap have both passed audit/install verification, macOS/Linux users can
install:

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

A future beta/pre-release channel uses:

```bash
brew install FlanChanXwO/tap/pixiv-cli-beta
```

Both formulas install the same `pixiv` binary and therefore conflict with each other; they only download verified
macOS/Linux Release assets and introduce no `ffmpeg` dependency. The current stable `pixiv-cli` formula is in the
public tap; the beta formula will only ship with future pre-releases.

### Direct download

The release process produces six fixed-name archives for darwin, linux, and windows on amd64/arm64:
`pixiv-cli_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows), plus `checksums.txt` and an Ed25519-signed
`checksums.json`. v0.3.0 ships the complete asset set; later versions should only be trusted as direct download
sources after passing the same release gates.

Current Releases do not include Apple notarization or Windows Authenticode. Even when downloading from a verified
Release, macOS Gatekeeper or Windows SmartScreen may still show a system reputation prompt; only obtain assets from
the project's GitHub Release page, verify the version, checksum, and signature notes, and never bypass warnings for
assets of unknown origin.

## Go SDK

External Go programs use the concrete `*pixiv.Client` directly and define their own narrow business interfaces in
their adapters:

```go
client, err := pixiv.OpenDefault(pixiv.Options{})
if err != nil { /* handle local auth/config failure */ }
result, err := client.SearchIllust(ctx, pixiv.SearchIllustRequest{Word: "初音ミク"})
_ = result
```

`NewClient` only uses explicit transport/token/options and reads no local state; `OpenDefault` reads one current
auth/config snapshot per public operation. Callers own their crawl patterns, budgets, filters, cursor persistence,
storage, scheduling, and their own HTTP APIs. See the [SDK contract](docs/sdk.md) for the full models, resource
streams, error, and pagination semantics.

Search callers can use the stable `SearchIllustFilters` contract for rating, content type, AI mode, aspect ratio,
resolution, and drawing tool. `SearchIllustOptions` returns the current App API tool choices for a word, and
`Illust.Tools` preserves the upstream tool list. App API performs content-type, AI-exclusion, aspect-ratio,
resolution, and tool filtering; the public SDK applies rating and AI-only filtering to each returned batch.

## Getting a refresh token

`PIXIV_REFRESH_TOKEN` must be a raw Pixiv App API OAuth refresh token. Web cookies (including `refresh_token=...`,
`PHPSESSID`, `device_token`) are always rejected — credentials are never extracted or converted from them.

The recommended flow is the CLI browser OAuth login, which saves directly to a local account:

```bash
pixiv auth login
```

The `auth login` flow:

| Stage | Behavior |
| --- | --- |
| Init | The CLI generates a PKCE verifier/challenge and OAuth state, and starts a local loopback HTTP server. |
| Browser | On macOS it first registers a local `pixiv://` callback helper and opens the default browser, so an existing Pixiv login session can be reused; the user must confirm the account on the Pixiv page. With `--no-open`, only the login URL and the local page address are printed. |
| Callback | The CLI only accepts this round's loopback callback, a hand-off from the `pixiv://` helper registered for the current login attempt, a terminal paste, or the local page form; if the browser doesn't return, you can manually paste the callback URL, a `pixiv://...` URL, a Pixiv relay URL, or the raw authorization code. |
| Validation | The local loopback callback must match this round's state; Pixiv's official callback URL and `pixiv://account/login` can be used as an explicit fallback when Pixiv doesn't return a state. |
| Save | Refresh/access tokens are never printed; the refresh token is saved to `auth.json` keyed by Pixiv UID. Unix-like systems actively use `0700` parent directories and `0600` files. On Windows, first creation inherits the parent ACL and replacement preserves the existing target ACL; the CLI does not claim to tighten or loosen the DACL. |

When the default browser opens, macOS registers a local `PixivCLIURLHandler.app` that serves only the current login
attempt and merely forwards the `pixiv://account/login?...` URL returned by Pixiv to this round's CLI loopback; it
does not read browser cookies, storage, history, session files, tabs, or network traffic. If the helper is
unavailable, the CLI still opens a normal browser and waits for the loopback or a manual paste — it never launches a
managed Chromium, DevTools/CDP, or any browser state scanning. When Pixiv shows a `post-redirect` authorization
relay page, you can manually paste the relay URL; the CLI opens that relay URL exactly once, and only after
verifying it belongs to this OAuth round. The browser may stay on a blank relay page — the terminal's final output
is the source of truth; if Pixiv never produced a callback, the CLI will not fake success.

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

Real login depends on the Pixiv OAuth web flow being available; automated tests use a fake OAuth server and never
touch real Pixiv.

## CLI usage

Log in and save an account first:

```bash
pixiv auth login
```

Advanced/scripted setups can also import an existing token:

```bash
printf '%s\n' 'YOUR_REFRESH_TOKEN' | pixiv auth add
```

Passing the token directly also works, but `--token` may end up in shell history:

```bash
pixiv auth add --token 'YOUR_REFRESH_TOKEN'
```

Common commands:

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
pixiv config path
pixiv config get download_path
pixiv config set download_path ~/Downloads/pixiv
pixiv config unset https_proxy

pixiv version
pixiv version --json
pixiv --version
pixiv update --check
pixiv update --check --json

pixiv search "初音ミク"
pixiv search "初音ミク" --json
pixiv detail 123456
pixiv ranking --mode day
pixiv recommended all
pixiv download 123456 789012
```

Account credentials are saved to `os.UserConfigDir()/pixiv/auth.json`, keyed by Pixiv UID; global settings live in
`os.UserConfigDir()/pixiv/config.toml`. Unix-like systems actively use `0700` parent directories and `0600` files.
On Windows, first creation inherits the parent ACL and replacement preserves the existing target ACL; the CLI does
not claim to tighten or loosen the DACL. Output is human-readable by default; add `--json` for machine-parseable JSON.
The CLI uses Cobra/pflag, so options may appear before or after positional arguments — both
`pixiv auth check 12345678 --json` and `pixiv search "初音ミク" --json` are officially supported forms.

### CLI command table

| Command | Usage | Description |
| --- | --- | --- |
| `auth add` | `pixiv auth add [--token TOKEN] [--json] [--proxy URL\|--no-proxy]` | Validates a raw Pixiv App API refresh token and adds or replaces the account keyed by Pixiv UID; cookie input is rejected. Without `--token`, reads from TTY/stdin. |
| `auth login` | `pixiv auth login [--json] [--no-open] [--addr 127.0.0.1:0] [--use] [--timeout DURATION] [--proxy URL\|--no-proxy]` | Logs in via a local loopback server and browser OAuth, saving the account keyed by Pixiv UID; never prints the refresh token. |
| `auth list` | `pixiv auth list [--json]` | Lists local accounts; never prints refresh tokens. |
| `auth use` | `pixiv auth use [UID]` | Sets the default account; interactive selection on a TTY. |
| `auth remove` | `pixiv auth remove [UID] [--yes]` | Removes an account; confirms by default on a TTY. After removing the default account, the first remaining account is selected automatically. |
| `auth check` | `pixiv auth check [UID] [--json] [--proxy URL\|--no-proxy]` | Refreshes the token and validates the account; on success records `user_id` and the username when available. |
| `config path` | `pixiv config path` | Prints the `config.toml` path. |
| `config get` | `pixiv config get KEY` | Prints one effective config value. |
| `config set` | `pixiv config set KEY VALUE` | Writes one known config key to `config.toml`. |
| `config unset` | `pixiv config unset KEY` | Deletes one known config key from `config.toml`. |
| `version` | `pixiv version [--json]` | Prints the binary's `version`, `commit`, and `build_date`; the root `pixiv --version` prints only the version. |
| `update` | `pixiv update [--check] [--prerelease] [--proxy URL]` | Checks for or performs an update matching the current install source; `--json` is only valid together with `--check`. |
| `search` | `pixiv search [options] WORD` | Searches illustrations. |
| `search-options` | `pixiv search-options [options] WORD` | Lists the App API drawing-tool choices for a word; requires authentication and supports the common account/token/proxy flags and `--json`. |
| `detail` | `pixiv detail [options] ILLUST_ID` | Shows details for a single artwork. |
| `ranking` | `pixiv ranking [options]` | Shows Pixiv illustration rankings. |
| `recommended` | `pixiv recommended all\|illust\|manga\|novel\|user [--page N --limit N --json]` | Shows personalized recommendations for the given kind; `all` returns illustrations, manga, novels, and users in full, in order, and requires authentication. |
| `user detail` | `pixiv user detail USER_ID [--json]` | Shows a user's full public profile; `USER_ID` is required. |
| `user artworks` | `pixiv user artworks [USER_ID] [--page N --limit N]` | Shows a user's artworks; uses the current authenticated user when `USER_ID` is omitted. |
| `user bookmarks` | `pixiv user bookmarks [USER_ID] [--page N --limit N]` | Shows a user's bookmarks; uses the current authenticated user when `USER_ID` is omitted. |
| `user following` | `pixiv user following [USER_ID] [--page N --limit N]` | Shows who a user follows; uses the current authenticated user when `USER_ID` is omitted. |
| `bookmark add/remove` | `pixiv bookmark add/remove ILLUST_ID` | Bookmarks or un-bookmarks an artwork. |
| `follow add/remove` | `pixiv follow add/remove USER_ID` | Follows or unfollows a user. |
| `download` | `pixiv download [options] ILLUST_ID...` | Downloads one or more artworks; without a token, uses the anonymous web fallback by default. |
| `mcp` | `pixiv mcp [--proxy URL\|--no-proxy]` | Starts the MCP stdio server; the proxy override applies only to this launch. |

Downloaded filenames normalize cross-platform-invalid characters in both the filename template and URL-derived
extension. Extensions also replace ASCII control characters and remove trailing dots or spaces rejected by
Windows. The extension still comes from the upstream URL; no allowlist, MIME guessing, or silent substitution is
used.

### `auth login` flags

| Flag | Default | Description |
| --- | --- | --- |
| `--json` | `false` | Prints the save result as JSON; never prints refresh/access tokens. |
| `--no-open` | `false` | Does not auto-open the default browser and performs no browser observation; only prints the login URL and the local loopback page address. |
| `--addr` | `127.0.0.1:0` | Local loopback listen address; port `0` means auto-assign. |
| `--use` | `false` | Sets the account as default after a successful login; also becomes default automatically when no default exists. |
| `--timeout` | `0` | Maximum time to wait for login completion; `0` means the CLI imposes no deadline. |
| `--proxy URL` / `--no-proxy` | empty | Proxy override for this token exchange only; never saved to `config.toml`. |

### Data command flags

| Command | Flag | Default | Description |
| --- | --- | --- | --- |
| `search` | `--search-target` | `partial_match_for_tags` | Search scope. |
| `search` | `--sort` | `date_desc` | Sort order. |
| `search` | `--duration` | empty | Pixiv API duration parameter. |
| `search` | `--rating` | `all` | Rating filter: `sfw`, `r18`, `r18g`, `mature`, or `all`. |
| `search` | `--type` | `all` | Content type: `all`, `illust-and-ugoira`, `illust`, `manga`, or `ugoira`; `comics` is a compatibility alias for `manga`. |
| `search` | `--ai-mode` | `all` | AI filter: `all`, `exclude`, or `only`; Pixiv `AIType==2` is AI-generated. |
| `search` | `--ai-type` | `2` | Deprecated alias: `0=exclude`, `1=only`, `2=all`; conflicts with an explicitly supplied `--ai-mode`. |
| `search` | `--aspect-ratio` | `all` | Aspect ratio: `all`, `landscape`, `portrait`, or `square`. |
| `search` | `--resolution` | `all` | Resolution: `all`, `high`, `medium`, or `low`; both dimensions are respectively `>=3000`, `1000..2999`, or `<=999`. |
| `search` | `--tool` | empty | Exact upstream drawing-tool name; obtain current values with authenticated `search-options`. |
| list commands | `--limit` | one upstream batch | Maximum item count; `0` keeps reading until there is no next batch. |
| list commands | `--page` | empty | 1-based logical page; must be used with a positive `--limit`. |
| list commands | `--offset` | `0` | Deprecated logical offset; cannot be combined with `--page`. |
| `search` | `--r18` | `false` | Deprecated alias for `--rating r18`; it does not modify the search word and conflicts with an explicitly supplied non-R18 rating. |
| `ranking` | `--mode` | `day` | Ranking mode. |
| `ranking` | `--date` | empty | Ranking date, typically `YYYY-MM-DD`. |
| `ranking` | `--offset` | `0` | Pagination offset. |
| `recommended KIND` | `--page`, `--limit`, deprecated `--offset` | per-stream pagination | Each stream paginates independently; `all` applies the same pagination semantics to illustrations, manga, novels, and users separately. |
| `detail` | `ILLUST_ID` | required | Pixiv artwork ID. |
| `download` | `ILLUST_ID...` | required | One or more Pixiv artwork IDs. |

With a refresh token, `search` always uses App API. App applies resolution, aspect-ratio, tool, content-type, and
`ai-mode=exclude` filters, while the public SDK applies rating and `ai-mode=only` to each App result batch. App
failures never fall back to Web. Every filter is bound to the opaque cursor, so a cursor cannot be reused with a
different filter set. With a positive `--limit` or `--page`, the CLI keeps reading upstream batches until it
collects enough matching works, the upstream has no next batch, or a repeated cursor is detected; without
`--limit`, the compatible one-batch default is preserved. Bookmark-count filtering is not provided.

### Common flags

| Flag | Applies to | Default | Description |
| --- | --- | --- | --- |
| `--uid UID` | `search/search-options/detail/ranking/recommended/user/download` | `auth.json.default_user_id` | Selects a local account. |
| `--profile UID` | `search/search-options/detail/ranking/recommended/user/download` | empty | Deprecated alias of `--uid`. |
| `--refresh-token TOKEN` | `search/search-options/detail/ranking/recommended/user/download` | empty | Temporarily overrides the account/env token; only raw App API refresh tokens are accepted. |
| `--json` | `auth` subcommands and data commands | `false` | Machine-parseable JSON output. |
| `--download-path PATH` | data commands; effectively only `download` | `DOWNLOAD_PATH`, `config.toml`, or `./downloads` | Download directory. |
| `--filename-template TEMPLATE` | data commands; effectively only `download` | `FILENAME_TEMPLATE`, `config.toml`, or `{author} - {title}_{id}` | Filename template. |
| `--proxy URL` | `auth add/login/check`, data commands, `mcp` | `https_proxy`/`HTTPS_PROXY`, `config.toml`, or empty | Uses an HTTP(S) proxy for this command only. |
| `--no-proxy` | `auth add/login/check`, data commands, `mcp` | empty | Clears the HTTP(S) proxy for this command; same precedence as `--proxy`, cannot be combined with it. |

### Supported `config` keys

| KEY | Type | Default | Description |
| --- | --- | --- | --- |
| `download_path` | string | `./downloads` | Download directory. |
| `filename_template` | string | `{author} - {title}_{id}` | Filename template. |
| `https_proxy` | string | empty | HTTP(S) proxy; the lowercase `https_proxy` environment variable takes precedence. |
| `web_fallback_enabled` | bool | `true` | Allows the anonymous Pixiv web/ajax API fallback when no refresh token exists; stored as `[web] fallback_enabled = true/false`. |
| `log_level` | string | `warn` | Structured stderr log level; can be overridden by `PIXIV_LOG_LEVEL`. Set explicitly to `info` to keep operational diagnostics. |
| `log_format` | string | `text` | Log format, `text` or `json`; can be overridden by `PIXIV_LOG_FORMAT`. |
| `update_check_enabled` | bool | `true` | Whether successful regular CLI commands check for stable updates; stored as `[update] check_enabled = true/false`. |
| `output_json` | bool | `false` | Makes data commands output JSON by default. |
| `login_open_browser` | bool | `true` | Whether `auth login` auto-opens the browser by default. |
| `login_timeout` | duration | `0s` | Default `auth login` wait duration. |
| `login_use_after_login` | bool | `false` | Whether `auth login` sets the new account as the current default. |

### Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `PIXIV_REFRESH_TOKEN` | empty | Pixiv App API OAuth refresh token; can be overridden by account selection or `--refresh-token`. |
| `PIXIV_LOG_LEVEL` | empty | Overrides `log_level`. |
| `PIXIV_LOG_FORMAT` | empty | Overrides `log_format`. |
| `DOWNLOAD_PATH` | `./downloads` | Download directory. |
| `FILENAME_TEMPLATE` | `{author} - {title}_{id}` | Filename template. |
| `https_proxy` / `HTTPS_PROXY` | empty | HTTP(S) proxy; the lowercase `https_proxy` takes precedence. |

Authentication precedence: `--refresh-token` > `--uid`/deprecated `--profile` > `PIXIV_REFRESH_TOKEN` >
`auth.json.default_user_id`.

Settings precedence: CLI flag > environment variable > `config.toml` > built-in default. The only CLI proxy
overrides are `--proxy URL` / `--no-proxy`, and they are never persisted.

### Anonymous web fallback

When none of `--refresh-token`, `PIXIV_REFRESH_TOKEN`, and the default account provides a refresh token, and
`web_fallback_enabled=true`, the following capabilities automatically use the Pixiv web/ajax API: `search`,
`detail`, `ranking`, `download`, and the MCP tools `search_illust`, `illust_detail`, `illust_ranking`,
`search_user`, `download`, and `get_thumbnail_base64`.

With a refresh token, the App API is always preferred; an invalid token, App API network error, or server error
never triggers an automatic fallback. The CLI surfaces a safe, classified failure instead of disguising it as a
normal empty result.

Differences in the anonymous fallback:

- Anonymous `search` only applies filters that Web API can express reliably. Resolution, aspect ratio, drawing
  tool, and content type are translated to Web parameters; AI filtering uses returned artwork fields.
- `rating=r18`, `r18g`, or `mature` fails before an anonymous request with an authentication requirement rather
  than pretending the result is empty. `rating=all` means only content visible anonymously.
- `search-options` is App-only and explicitly unsupported without a refresh token. Search does not read or store
  browser cookies such as `PHPSESSID`, and never converts a refresh token into a Web session.
- `search_user` is not Pixiv's official user search; it dedupes web work-search results by `userId` and returns
  "authors of related works".
- Static single/multi-page downloads use the `original` URLs from `/ajax/illust/{id}/pages`.
- Ugoira downloads use the `originalSrc` zip and frames from `/ajax/illust/{id}/ugoira_meta`; supported release
  builds encode GIF/APNG with the built-in Rust encoder, with no runtime `ffmpeg` dependency.
- The web fallback adds no dedicated proxy environment variable; it keeps using `--proxy` / `--no-proxy`,
  `https_proxy` / `HTTPS_PROXY`, or `pixiv config set https_proxy ...`.

An invalid proxy URL makes the SDK, CLI, MCP server, and update check fail before any network request. Diagnostics
retain only safe classification and static context; they never echo input userinfo, path, or query.

To disable it:

```bash
pixiv config set web_fallback_enabled false
```

## Version and updates

`pixiv version` prints a human-readable version, commit, and build date; `pixiv version --json` writes JSON
containing only `version`, `commit`, and `build_date` to stdout. The root `pixiv --version` is handy for a quick
version check.

```bash
pixiv version
pixiv version --json
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
the Ed25519-signed checksum manifest and the archive SHA-256 before downloading, then preflight
`pixiv version --json` and atomically replace the executable.

Update checks only select canonical SemVer tags. Stable checks first exclude GitHub Releases marked as prerelease;
`--prerelease` includes them in the current channel. If any non-draft published Release in that channel uses a
non-SemVer tag, the check reports that tag and fails closed instead of skipping it for an older version.

Supported binaries embed the production Ed25519 public key/key ID/fingerprint; the private key exists only in the
protected `release` Environment and a controlled macOS Keychain recovery copy. v0.3.0 is the current signed
Release; `pixiv update --check` remains a read-only check and is not a substitute for verifying the selected
version's assets, checksums, and signatures at install time.

Successful regular CLI commands make a best-effort stable-update check. It skips MCP, help, `version`, `update`,
and development builds, queries at most once per 24 hours per user cache, and caps the automatic check at 3
seconds. A discovered new version or a failed check only writes to stderr (failures as warnings), never changes
the business command's exit code, and never pollutes JSON stdout or MCP JSON-RPC stdout. To disable the automatic
check:

```bash
pixiv config set update_check_enabled false
```

## MCP usage

The MCP stdio server must be started explicitly:

```bash
PIXIV_REFRESH_TOKEN=... \
DOWNLOAD_PATH=./downloads \
FILENAME_TEMPLATE="{author} - {title}_{id}" \
./build/pixiv mcp
```

The MCP proxy override is a launch-time setting:

```bash
./build/pixiv mcp --proxy http://127.0.0.1:7890
./build/pixiv mcp --no-proxy
```

When `PIXIV_REFRESH_TOKEN` is unset, `pixiv mcp` first falls back to `auth.json.default_user_id`; if there is
still no refresh token and `web_fallback_enabled=true`, MCP tools that support the anonymous fallback use the
Pixiv web/ajax API directly. A real token written as an inline environment variable may also end up in shell
history; for long-term use, prefer the MCP client's private environment configuration or local account management.

Logs go to stderr; stdout is reserved for MCP JSON-RPC. Typed SDK tool failures use `isError=true`. To preserve
wire compatibility, legacy tool failures retain their existing `isError=false`, Content, structured output, and
text, while the stderr operation event records error level and `result=error`. Events contain only safe
operation/classification metadata — never raw errors, tool input, queries, tokens, cookies, URLs, paths, or
response bodies. Normal empty results are still logged as success.

Example MCP client configuration:

```json
{
  "mcpServers": {
    "pixiv-server": {
      "command": "/absolute/path/to/pixiv-cli/build/pixiv",
      "args": ["mcp"],
      "env": {
        "PIXIV_REFRESH_TOKEN": "your Pixiv App API refresh token",
        "DOWNLOAD_PATH": "./downloads",
        "FILENAME_TEMPLATE": "{author} - {title}_{id}"
      }
    }
  }
}
```

## Agent Skill

The repository ships a skill for coding agents (`skills/pixiv/`, all English) that teaches an agent to drive this
CLI correctly: environment preflight, credential safety rules, confirmation tiers for write/download operations,
output and token control strategy, and the common semantic traps. The entry point is `skills/pixiv/SKILL.md`, with
workflow details under `skills/pixiv/references/`.

Claude Code users install it as a symlink (the skill stays up to date with the repository):

```bash
ln -s "$(pwd)/skills/pixiv" ~/.claude/skills/pixiv
```

On Windows, use `mklink /J` (a junction) or copy the directory. Other agents that support the SKILL.md format can
place it in their own skill directory. The skill contains no credentials; it only invokes the installed `pixiv`
binary.

## Command overview

Representative CLI commands (see the authoritative [CLI command table](#cli-command-table)):

- `auth add/login/list/remove/use/check`
- `config path/get/set/unset`
- `version`
- `update`
- `search`
- `search-options`
- `detail`
- `ranking`
- `recommended`
- `download`
- `mcp`

Representative MCP tools (see the authoritative [MCP tool contract](docs/mcp-tools.md)):

`set_download_path`, `download`, `refresh_token`, `set_refresh_token`,
`download_random_from_recommendation`, `search_illust`, `search_illust_options`, `illust_detail`,
`illust_related`, `illust_ranking`, `search_user`, `illust_recommended`, `recommended`,
`trending_tags_illust`, `illust_follow`, `user_detail`, `user_bookmarks`, `user_following`,
and `get_thumbnail_base64`.

`user_detail` takes a required `user_id` and returns a complete, stable user-detail structured output; it requires
authentication and does not support the anonymous web fallback. See [MCP tools](docs/mcp-tools.md) for each tool's
parameters and return semantics.

`search_illust` accepts the same six filter fields as the SDK: `rating`, `content_type`, `ai_mode`, `aspect_ratio`,
`resolution`, and `tool`. Its deprecated `search_r18` field remains an alias for `rating=r18`; combining it with an
explicit `rating` is rejected. The legacy tool keeps its `{text}` structured output. Authenticated
`search_illust_options` accepts `word` and returns exactly `{tools,text}`, preserving dynamic tool names in upstream
order. Neither interface provides bookmark-count filtering or Cookie authentication.

Artwork-list MCP text shows every tag in upstream order; SDK tools with artwork models keep returning their
unchanged complete structured output. Known `illust_ranking` modes use stable Chinese titles, while future modes
preserve the raw value and append `排行榜`.

`download_random_from_recommendation.count` is optional and defaults to 5; an explicit value must be 1..20. See
[MCP tools](docs/mcp-tools.md#配置认证与下载) for complete parameter, error, and insufficient-recommendation
semantics.

On failure, `download` and `download_random_from_recommendation` preserve the business error text and return a
valid structured output with empty `items`/`files` arrays. See [MCP tools](docs/mcp-tools.md#配置认证与下载) for
the complete wire semantics.

`recommended` takes a required `kind`: `all`, `illust`, `manga`, `novel`, or `user`. It returns structured
recommendations via the authenticated App SDK; `all` applies `page`/`limit` to each of the four streams
separately, exposes no SDK cursor, and does not support the anonymous web fallback. `illust_recommended` keeps the
legacy tool name, parameters, and text output for compatibility, but also executes through the public SDK call
chain.

## Development verification

```bash
go test ./...
sh scripts/build.sh
./build/pixiv --help
./build/pixiv mcp --help
PIXIV_E2E_WEB_API=1 PIXIV_WEB_API_PROXY=http://127.0.0.1:7890 go test ./test/e2e -run WebAPIFallbackReal -count=1 -v
```

The real Pixiv web fallback e2e is skipped by default; it only goes online when `PIXIV_E2E_WEB_API=1` is set. The
authenticated App API canary additionally requires explicitly opting into an isolated test token or the local
account store; see [the testing section of the development guide](docs/development.md#测试) for triggers and
credential boundaries.
