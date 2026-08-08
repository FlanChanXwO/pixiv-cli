<div align="center">

# pixiv-cli

**Pixiv CLI · MCP stdio server · Go SDK**

[English](README.md) · [简体中文](README.zh-CN.md) · [日本語](README.ja.md)

<p><a href="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml"><img alt="Quality gate" src="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml/badge.svg"></a> <a href="https://github.com/FlanChanXwO/pixiv-cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/FlanChanXwO/pixiv-cli?style=flat-square"></a> <img alt="Views" src="https://hits.sh/github.com/FlanChanXwO/pixiv-cli.svg?style=flat-square&amp;label=views"></p>

[Install](#install) · [Quick start](#60-second-quick-start) · [Interfaces](#choose-your-interface) · [Documentation](#documentation) · [Contributing](CONTRIBUTING.md)

</div>

`pixiv-cli` brings the Pixiv ecosystem to the terminal: discover works and creators, manage accounts and collections, follow artists, bookmark artworks, and download visual works. It is an independent, unofficial third-party CLI, MCP server, and public Go SDK; it is not affiliated with or endorsed by Pixiv Inc. The CLI and MCP server both call the same public Go SDK, with the Pixiv App API as the authenticated source of truth. Use it in accordance with Pixiv's terms and applicable law.

## Why pixiv-cli?

- **One capability surface** — search, details, rankings, recommendations, users, bookmarks, follows, downloads, and ugoira across CLI, MCP, and SDK.
- **Read-only FANBOX access** — authenticate with `FANBOXSESSID`, inspect creators, posts, home/supporting feeds, tags, and first-party file resources through the CLI, MCP, or `sdk/fanbox`.
- **Composable visual pipelines** — visual lists automatically emit canonical NDJSON when piped; use `--filter` for typed local artwork rules and pass matching records straight to `download`.
- **Local account pools** — enable database-backed scheduling for read workloads with `pixiv auth pool status|enable|disable`; selection honors Pixiv `Retry-After` responses without exposing credentials.
- **Guided account sign-in** — complete browser OAuth with `pixiv auth login`, then use `auth list`, `auth use`, and `auth check` to manage local multi-account access.
- **Four ugoira output modes** — choose GIF, APNG, lossless ZIP, or extracted frames.
- **Reliable, organized downloads** — revalidate `.pixiv-cache` metadata, resume verified partials, retry eligible resource failures, optionally archive completed artwork IDs, write sidecars, and show exact terminal progress when available.
- **Authenticated App API discovery** — read R18 details, pages, ugoira metadata, and all 16 ranking modes through the App API.
- **Useful search filters** — rating, content type, AI mode, aspect ratio, resolution, and a versioned drawing-tool catalog.
- **Direct Pixiv references** — paste supported artwork URLs into detail or download; authenticated profile and artworks URLs expand to that creator's visual works.
- **Local multi-account OAuth** — browser login, account selection, refresh-token rotation, and an optional cross-machine callback relay.
- **Automation-ready integration** — typed SDK errors, JSON output, clean MCP stdio, signed release updates, and complete result reporting.
- **Explicit diagnostics** — opt in with `--debug` for safe live stderr events covering routing, challenge recovery, account-pool decisions, and downloads; no log file is created.

## Install

### Installer scripts (Windows, Linux, and macOS)

Linux/macOS (`sh`):

```bash
curl -fsSLo /tmp/pixiv-install.sh https://github.com/FlanChanXwO/pixiv-cli/releases/latest/download/install.sh && sh /tmp/pixiv-install.sh --add-to-path
```

Windows Command Prompt (`cmd.exe`, no PowerShell):

```bat
curl.exe -fsSLo "%TEMP%\pixiv-install.cmd" https://raw.githubusercontent.com/FlanChanXwO/pixiv-cli/main/scripts/install.cmd && call "%TEMP%\pixiv-install.cmd" --add-to-path
```

Both scripts detect AMD64/ARM64, select the latest stable official Release archive, verify its published SHA-256,
preflight the staged binary, and install per-user before changing PATH. Use `--no-path` to leave PATH untouched or
`--install-dir DIR` to choose another destination. You can inspect the downloaded script before running it.

Versioned installers keep `checksums.txt` on the official GitHub HTTPS path. Embedded free source candidates are
probed only for the platform archive, must return the same checksum content, and the downloaded archive still has to
pass SHA-256 verification. This changes transport availability, never Release identity or integrity.

### Install with an AI agent

Copy this single prompt into Codex, Claude Code, Cursor, or another local AI agent with terminal access:

```text
Install the latest stable pixiv-cli from https://github.com/FlanChanXwO/pixiv-cli for this machine: inspect the repository's scripts/install.sh or scripts/install.cmd first, choose the script matching the detected OS and architecture (the Windows path must use cmd.exe and must not invoke PowerShell), download only official GitHub Release assets, require the published SHA-256 check to pass before replacing anything, install per-user without administrator or root privileges, add only the chosen install directory to the user PATH, ask before installing any missing prerequisite, never read or output Pixiv credentials, verify with pixiv version, and report the installed version plus every file and PATH change.

Also install the `pixiv-cli` Skill that matches the same stable release tag (not main): download the full skills/pixiv-cli/ directory from that tag into the agent skills directory the user confirms. Do not guess the skills path and do not follow the main branch for skill content.
```

### Install the pixiv-cli Skill from SkillHub

Agents with SkillHub support can install the published [`pixiv-cli` Skill](https://www.skillhub.cn/skills/pixiv-cli) directly from SkillHub. Each Skill version matches the CLI release it teaches; always use `pixiv <cmd> --help` as the final source of command syntax.

### Install the pixiv-cli Skill from ClawHub

Agents using ClawHub can install the published [`pixiv-cli` Skill](https://clawhub.ai/flanchanxwo/skills/pixiv-cli) with `clawhub install pixiv-cli`; pin the installed skill to the matching published release version rather than following an unversioned latest tag.

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

# Save a FANBOX session through hidden input, then inspect FANBOX content.
pixiv fanbox auth import
pixiv fanbox post 123456

# Search with App-side filters.
pixiv search "初音ミク" --type illust --ai-mode exclude --resolution high
pixiv novel search "初音ミク" --rating sfw --min-text-length 1000

# Follow creators and build your collection.
pixiv follow add 12345678
pixiv bookmark add 123456

# Inspect, discover recommendations, and download.
pixiv detail https://www.pixiv.net/artworks/123456
pixiv recommended all --limit 10
pixiv timeline latest --type illust --limit 20
pixiv download https://www.pixiv.net/artworks/123456 --pages 1,3-5 --quality regular
pixiv download 123456 https://i.pximg.net/img-original/example.jpg --concurrency 8

# Batch-download every visual work from a creator.
pixiv download https://www.pixiv.net/users/12345678/artworks
```

Run `pixiv --help` or open the [complete CLI reference](docs/en/cli-reference.md) for every command, flag, configuration key, environment variable, fallback rule, and update behavior.

## Choose your interface

### CLI

Use the default text output interactively and `--json` where the command supports machine output:

```bash
pixiv ranking --mode day --json
pixiv user search "miku" --limit 10 --json
pixiv user detail 12345678
pixiv timeline latest --type illust --limit 10 --json
```

### MCP

Diagnostics are opt-in and write only safe event fields to stderr; MCP stdout remains JSON-RPC only.

Start the stdio server explicitly. stdout remains reserved for JSON-RPC; tool failures are returned as structured results with `isError=true`. No project-level or daily log files are created by default.

```bash
pixiv mcp
# Optional safe diagnostics go to stderr and never change MCP stdout.
pixiv --debug mcp
# FANBOX tools use their own runtime credential selection.
pixiv --debug fanbox mcp
```

See the [MCP tool contract](docs/en/mcp-tools.md) for tools, parameters, structured output, and authentication behavior.
Fixed MCP status, error, and display text is English; Pixiv metadata and user-supplied text are preserved verbatim.

### Go SDK

The public SDK receives credentials explicitly and does not read the CLI's local account store. This example reads a refresh token from the process environment only to keep the secret out of source code; persist the rotated credentials returned by `Open` in your own application:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func main() {
	ctx := context.Background()
	client, _, err := pixiv.Open(ctx, os.Getenv("PIXIV_REFRESH_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	result, err := client.SearchArtworks(ctx, pixiv.SearchArtworksRequest{Word: "初音ミク"})
	if err != nil {
		log.Fatal(err)
	}
	for _, artwork := range result.Items {
		fmt.Printf("%d %s\\n", artwork.ID, artwork.URL)
	}
}
```

The import path is `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv`. `sdk/fanbox` accepts a `FANBOXSESSID` explicitly and supports native Firefox 148 routing with optional service-scoped proxy, user-agent, and challenge-only FlareSolverr options. `Download`/`DownloadAll` use documented beginner defaults; `DownloadWith`/`DownloadAllWith` expose paths, naming, pages, quality, and concurrency. The [SDK guide](docs/en/sdk.md) documents models, cursors, resources, errors, and caller responsibilities.

## Authentication and token safety

`pixiv auth login` is the recommended setup. It saves raw Pixiv App OAuth refresh tokens by UID in the local account store.

### Account information disclaimer

Account names, IDs, membership signals, and the active local-account selection are convenience information from the local store and Pixiv responses. Treat them as convenience data rather than proof of account ownership, entitlement, or current Pixiv status. Verify important account details in Pixiv and use only accounts you are authorized to manage.

On macOS, Windows, and desktop Linux, `pixiv` prepares the current-user `pixiv://` callback handler for the installed binary. When a server has `login_relay_public_url` and `login_relay_listen_addr`, `pixiv auth login` prints a one-time remote hand-off URL. Opening it transfers the session directly to the installed desktop handler, which starts OAuth and returns its callback to the server. Remote login therefore uses a desktop with pixiv-cli installed; it has no project confirmation page or callback-copy form. See the [CLI reference](docs/en/cli-reference.md#getting-a-refresh-token).

```bash
pixiv auth list
pixiv auth pool status
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
