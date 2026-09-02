<div align="center">

# pixiv-cli

**Pixiv CLI · MCP stdio server · Go SDK**

[English](README.md) · [简体中文](README.zh-CN.md)

<p><a href="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml"><img alt="Quality gate" src="https://github.com/FlanChanXwO/pixiv-cli/actions/workflows/ci.yml/badge.svg"></a> <a href="https://github.com/FlanChanXwO/pixiv-cli/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/FlanChanXwO/pixiv-cli?style=flat-square"></a> <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/FlanChanXwO/pixiv-cli?style=flat-square"></a> <img alt="Views" src="https://hits.sh/github.com/FlanChanXwO/pixiv-cli.svg?style=flat-square&amp;label=views"></p>

[Install](#install) · [Quick start](#60-second-quick-start) · [Interfaces](#choose-your-interface) · [Documentation](#documentation) · [Contributing](CONTRIBUTING.md)

</div>

`pixiv-cli` brings the Pixiv ecosystem to the terminal: discover works and creators, manage accounts and collections, follow artists, bookmark artworks, and download visual works. It is an independent, unofficial third-party CLI, MCP server, and public Go SDK; it is not affiliated with or endorsed by Pixiv Inc. The CLI and MCP server both call the same public Go SDK, with the Pixiv App API as the authenticated source of truth. Use it in accordance with Pixiv's terms and applicable law.

## Why pixiv-cli?

- **One capability surface** — keyword search, details, rankings, recommendations, users, bookmarks, follows, downloads, and ugoira across CLI, MCP, and SDK; reverse-image search is integrated into the CLI/MCP surface.
- **Read-only FANBOX access** — authenticate with `FANBOXSESSID`, inspect creators, posts, home/supporting feeds, tags, and first-party file resources through the CLI, MCP, or `sdk/fanbox`.
- **Composable visual pipelines** — visual lists automatically emit canonical NDJSON when piped; use `--filter` for typed local artwork rules and pass matching records straight to `download`.
- **Local account pools** — enable database-backed scheduling for read workloads with `pixiv auth pool status|enable|disable`; selection honors Pixiv `Retry-After` responses without exposing credentials.
- **Guided account sign-in** — complete browser OAuth with `pixiv auth login`, then use `auth list`, `auth use`, and `auth check` to manage local multi-account access.
- **Four ugoira output modes** — choose GIF, APNG, lossless ZIP, or extracted frames.
- **Reliable, organized downloads** — revalidate `.pixiv-cache` metadata, resume verified partials, retry eligible resource failures, optionally archive completed artwork IDs, write sidecars, and show exact terminal progress when available.
- **Authenticated App API discovery** — read R18 details, pages, ugoira metadata, and all 16 ranking modes through the App API.
- **Useful search filters** — rating, content type, AI mode, aspect ratio, resolution, and a versioned drawing-tool catalog; reverse-image search can query SauceNAO or ascii2d from a local file or URL.
- **Direct Pixiv references** — paste supported artwork URLs into detail or download; authenticated profile and artworks URLs expand to that creator's visual works.
- **Local multi-account OAuth** — browser login, account selection, refresh-token rotation, and an optional cross-machine callback relay.
- **Automation-ready integration** — typed SDK errors, JSON output, clean MCP stdio, signed release updates, and complete result reporting.

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

### Docker (Linux amd64/arm64)

Official images are published to GHCR as `ghcr.io/flanchanxwo/pixiv-cli`. Pull an exact release when reproducibility matters:

```bash
docker pull ghcr.io/flanchanxwo/pixiv-cli:v1.2.3
```

`latest` follows stable releases only; Prerelease tags never move `latest`. To track the current stable release, pull `ghcr.io/flanchanxwo/pixiv-cli:latest`. Images are built natively for `linux/amd64` and `linux/arm64`. The container runs the same `pixiv` binary and uses the same `~/.pixiv-cli` state namespace as other installations.

Keep account state persistent and expose a download workspace:

```bash
docker run --rm \
  -v pixiv-cli-state:/home/pixiv/.pixiv-cli \
  -v "$PWD:/work" \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  --version
```

Use `/work` for downloaded files with an explicit output path; it is the container's working directory, not a separate product mode.

Bind mounts preserve host ownership. If the host directory is not writable by image UID 1000, run as the host identity, give the container an ephemeral `HOME`, and choose an explicit output path:

```bash
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp/pixiv-cli \
  -v "$PWD:/work" \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  download URL --output /work/downloads
```

For persistent state with that same host identity, bind a host directory you own instead of the default named volume:

```bash
mkdir -p "$PWD/pixiv-cli-state"
docker run --rm \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp/pixiv-cli \
  -v "$PWD/pixiv-cli-state:/home/pixiv/.pixiv-cli" \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  auth list
```

Import a refresh token through stdin instead of passing it as an argv value. For manual import, allocate a TTY so the hidden prompt can suppress echo: run the command, paste the opaque token, and send EOF (`Ctrl-D`).

```bash
docker run --rm -it \
  -v pixiv-cli-state:/home/pixiv/.pixiv-cli \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  auth import
```

For automation, pipe your secret manager's stdout directly into stdin with `-i`, not `-it`. The container writes only to the persistent state volume:

```bash
secret-manager print-token | docker run --rm -i \
  -v pixiv-cli-state:/home/pixiv/.pixiv-cli \
  ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 \
  auth import
```

This uses the existing `pixiv auth import` behavior; it does not add a Docker-specific OAuth callback flow for `auth login`.

For MCP, the transport remains `docker run --rm -i ghcr.io/flanchanxwo/pixiv-cli mcp`, and stdout stays reserved for MCP JSON-RPC. Pin a release as shown below:

```bash
docker run --rm -i ghcr.io/flanchanxwo/pixiv-cli:v1.2.3 mcp
```

Add `-v pixiv-cli-state:/home/pixiv/.pixiv-cli` when the MCP server should reuse persisted accounts.

Upgrade by pulling a newer image and redeploying with the same state volume. This workflow does not make `pixiv update` container-aware.

### Install with an AI agent

Copy this single prompt into Codex, Claude Code, Cursor, or another local AI agent with terminal access:

```text
Install the latest stable pixiv-cli from https://github.com/FlanChanXwO/pixiv-cli for this machine: inspect the repository's scripts/install.sh or scripts/install.cmd first, choose the script matching the detected OS and architecture (the Windows path must use cmd.exe and must not invoke PowerShell), download only official GitHub Release assets, require the published SHA-256 check to pass before replacing anything, install per-user without administrator or root privileges, add only the chosen install directory to the user PATH, ask before installing any missing prerequisite, never read or output Pixiv credentials, verify with pixiv --version, and report the installed version plus every file and PATH change.

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

# Reverse-search a local image or HTTP(S) image URL. Results can be JSON or NDJSON.
pixiv search ./image.png --provider ascii2d-color --json
pixiv search https://your-image-url.example/image.png --provider all --ndjson

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

Reverse-image search is available through the CLI/MCP integration; the public Go SDK remains unchanged.

Start the stdio server explicitly. stdout remains reserved for JSON-RPC; tool failures are returned as structured results with `isError=true`. No project-level or daily log files are created by default.

```bash
pixiv mcp
# FANBOX tools use their own runtime credential selection.
pixiv fanbox mcp
```

See the [MCP tool contract](docs/en/mcp-tools.md) for tools, parameters, structured output, and authentication behavior.
Fixed MCP status, error, and display text is English; Pixiv metadata and user-supplied text are preserved verbatim.

The `reverse_search` tool accepts a regular local file or HTTP(S) URL and may upload
that source to third-party providers. Because trusted local MCP clients may request
private files and private/loopback/link-local URLs, run it only from a client you
trust; see the [reverse-search MCP contract](docs/en/mcp-tools.md#reverse-image-search).
For advanced reverse-search proxy, User-Agent, and challenge-recovery settings, see
the [CLI reference](docs/en/cli-reference.md). FlareSolverr is a JSON
challenge-recovery control path and never receives the native ascii2d image upload.

### Go SDK

The public SDK receives credentials explicitly and does not read the CLI's local account store or process environment. Obtain the credential from your application's secret store and persist the rotated credentials returned by `Open`:

```go
package main

import (
	"context"
	"fmt"
	"log"

	pixiv "github.com/FlanChanXwO/pixiv-cli/sdk/pixiv"
)

func main() {
	ctx := context.Background()
	refreshToken := "replace-with-a-refresh-token-from-your-secret-store"
	client, _, err := pixiv.Open(ctx, refreshToken)
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

The import path is `github.com/FlanChanXwO/pixiv-cli/sdk/pixiv`. `sdk/fanbox` accepts a `FANBOXSESSID` explicitly and supports native Chrome 146 TLS routing with the built-in Firefox 148 HTTP User-Agent baseline, with optional service-scoped proxy, user-agent, and challenge-only FlareSolverr options. `Download`/`DownloadAll` use documented beginner defaults; `DownloadWith`/`DownloadAllWith` expose paths, naming, pages, quality, and concurrency. The [SDK guide](docs/en/sdk.md) documents models, cursors, resources, errors, caller responsibilities, and the explicit DTO boundary used by CLI/MCP JSON output; media resources cross those boundaries only as opaque references.

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
| [Architecture](docs/en/maintainers/architecture.md) | Package boundaries and runtime flow |
| [Development](docs/en/maintainers/development.md) | Toolchain, tests, builds, and releases |
| [Changelog](changelog/README.md) | User-visible changes |

## Contributing

Bug reports, documentation fixes, tests, and focused features are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request; discuss large or public-interface changes first.

## License

[MIT](LICENSE) © FlanChanXwO
