<div align="center">

# pixiv-cli

**Pixiv CLI · MCP stdio server · Go SDK**

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md)

<p><a href="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml"><img alt="Quality gate" src="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml/badge.svg"></a> <a href="https://github.com/FlanChanXwO/pixiv-cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/FlanChanXwO/pixiv-cli?style=flat-square"></a> <img alt="Views" src="https://hits.sh/github.com/FlanChanXwO/pixiv-cli.svg?style=flat-square&amp;label=views"></p>

[Install](#install) · [Quick start](#60-second-quick-start) · [Interfaces](#choose-your-interface) · [Documentation](#documentation) · [Contributing](CONTRIBUTING.md)

</div>

`pixiv-cli` is an independent, unofficial third-party tool that gives humans, coding agents, and Go applications one consistent way to use Pixiv; it is not affiliated with or endorsed by Pixiv Inc. The CLI and MCP server both call the same public Go SDK, with the Pixiv App API as the authenticated source of truth. Use it in accordance with Pixiv's terms and applicable law.

Maintainers: release tags are blocked by a protected authenticated E2E gate. Its refresh token belongs only in the GitHub `pixiv-e2e` Environment Secret; work IDs and search inputs are Environment Variables. Pull request and `main` CI remain offline and secret-free. See the [development guide](docs/maintainers/development.md#测试).

## Why pixiv-cli?

- **One capability surface** — search, details, rankings, recommendations, users, bookmarks, follows, downloads, and ugoira across CLI, MCP, and SDK.
- **App API first** — a configured refresh token always uses the authenticated App path; App failures never silently fall back to Web.
- **Authenticated R18 reads** — details, pages, ugoira metadata, and all 16 ranking modes use the App API; a verified medium ugoira ZIP is reported honestly when original is unavailable.
- **Useful search filters** — rating, content type, AI mode, aspect ratio, resolution, and dynamic drawing tools.
- **Local multi-account OAuth** — browser login, account selection, and refresh-token rotation without reading browser cookies or profiles.
- **Safe automation** — typed SDK errors, JSON output, clean MCP stdio, signed release updates, and no hidden result truncation.
- **Limited anonymous access** — supported read operations can use the Web API when no token exists and fallback is enabled.

## Install

### Installer scripts (Windows, Linux, and macOS)

Linux/macOS (`sh`):

```bash
curl -fsSLo /tmp/pixiv-install.sh https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.sh && sh /tmp/pixiv-install.sh --add-to-path
```

Windows Command Prompt (`cmd.exe`, no PowerShell):

```bat
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd && call "%TEMP%\pixiv-install.cmd" --add-to-path
```

Both scripts detect AMD64/ARM64, select the latest stable official Release archive, verify its published SHA-256,
preflight the staged binary, and install per-user before changing PATH. Use `--no-path` to leave PATH untouched or
`--install-dir DIR` to choose another destination. You can inspect the downloaded script before running it.

### Install with a coding agent

Copy this single prompt into Codex, Claude Code, Cursor, or another local coding agent with terminal access:

```text
Install the latest stable pixiv-cli from https://github.com/FlanChanXwO/pixiv-cli for this machine: inspect the repository's scripts/install.sh or scripts/install.cmd first, choose the script matching the detected OS and architecture (the Windows path must use cmd.exe and must not invoke PowerShell), download only official GitHub Release assets, require the published SHA-256 check to pass before replacing anything, install per-user without administrator or root privileges, add only the chosen install directory to the user PATH, ask before installing any missing prerequisite, never read or output Pixiv credentials, verify with pixiv version, and report the installed version plus every file and PATH change.

Also install the product skill that matches the same stable release tag (not main): download the full skills/pixiv-cli/ directory from that tag into the agent skills directory the user confirms. Do not guess the skills path and do not follow the main branch for skill content.
```

### Homebrew (recommended on macOS and Linux)

```bash
brew install FlanChanXwO/tap/pixiv-cli
```

Upgrade later with:

```bash
brew update
brew upgrade pixiv-cli
```

### Go

Use an exact published tag. A source install requires Go, cgo, a C linker, and the committed Rust static library for your target.

```bash
go install github.com/FlanChanXwO/pixiv-cli/cmd/pixiv@vX.Y.Z
```

### Release archive or source build

Download a supported archive from [GitHub Releases](https://github.com/FlanChanXwO/pixiv-cli/releases), or build the checkout:

```bash
sh scripts/build.sh
```

Direct downloads include checksums and a signed manifest. See the [CLI reference](docs/en/cli-reference.md#installation) for platform and trust details.

## 60-second quick start

```bash
# Save a Pixiv account through browser OAuth.
pixiv auth login

# Search with App-side filters.
pixiv search "初音ミク" --type illust --ai-mode exclude --resolution high
pixiv novel search "初音ミク" --rating sfw --min-text-length 1000

# Inspect, discover recommendations, and download.
pixiv detail 123456
pixiv recommended all --limit 10
pixiv download 123456 --pages 1,3-5 --quality regular
```

Run `pixiv --help` or open the [complete CLI reference](docs/en/cli-reference.md) for every command, flag, configuration key, environment variable, fallback rule, and update behavior.

## Choose your interface

### CLI

Use human-readable output interactively and `--json` where the command supports machine output:

```bash
pixiv ranking --mode day --json
pixiv user search "miku" --limit 10 --json
pixiv user detail 12345678
pixiv search-options "初音ミク"
```

### MCP

Start the stdio server explicitly. stdout remains reserved for JSON-RPC. Operation summaries are written as daily plain-text files named `YYYY-MM-DD.txt` under `~/.pixiv-cli/logs` (on Windows, `%USERPROFILE%\.pixiv-cli\logs`; default retention 7 days); the terminal stays free of log traces by default.

```bash
pixiv mcp
```

See the [MCP tool contract](docs/en/mcp-tools.md) for tools, parameters, structured output, and authentication behavior.
Fixed MCP status, error, and display text is English; Pixiv metadata and user-supplied text are preserved verbatim.

### Go SDK

```go
client, err := pixiv.OpenDefault(pixiv.Options{})
if err != nil {
    // Handle local auth/configuration failure.
}
result, err := client.SearchIllust(ctx, pixiv.SearchIllustRequest{Word: "初音ミク"})
```

Import `github.com/FlanChanXwO/pixiv-cli/pixiv`. The [SDK guide](docs/en/sdk.md) documents models, cursors, resources, errors, and caller responsibilities.

## Authentication and token safety

`pixiv auth login` is the recommended setup. It saves raw Pixiv App OAuth refresh tokens by UID in the local account store; browser cookies such as `PHPSESSID` are rejected and are never converted into App credentials.

On macOS, desktop Linux, and Windows, the `pixiv://` callback handler is installed only for the active login and then restored. For a headless SSH server, use the existing `--no-open --addr` flow with a local `ssh -L` tunnel; the forwarded fallback page can continue a validated Pixiv relay in that same browser without a local pixiv installation. See the [CLI reference](docs/en/cli-reference.md#getting-a-refresh-token).

```bash
pixiv auth list
pixiv auth use 12345678
pixiv auth check
```

## Documentation

| Guide | Use it for |
| --- | --- |
| [CLI reference](docs/en/cli-reference.md) | Commands, flags, auth, configuration, fallback, downloads, and updates |
| [Go SDK](docs/en/sdk.md) | Public client, models, pagination, resources, and typed errors |
| [MCP tools](docs/en/mcp-tools.md) | Tool schemas and output semantics |
| [Architecture (Simplified Chinese)](docs/maintainers/architecture.md) | Package boundaries and runtime flow |
| [Development (Simplified Chinese)](docs/maintainers/development.md) | Toolchain, tests, builds, and releases |
| [Changelog](changelog/README.md) | User-visible changes |

## Contributing

Bug reports, documentation fixes, tests, and focused features are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request; discuss large or public-interface changes first.

## License

[MIT](LICENSE) © FlanChanXwO
