# Inspect root CLI syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv --help
```

## Output
```text
Pixiv CLI and MCP server

Usage:
  pixiv [flags]
  pixiv [command]

Available Commands:
  auth        Manage local Pixiv authentication
  bookmark    Manage illustration bookmarks
  comment     List artwork or novel comments
  config      Manage global Pixiv CLI settings
  detail      Show one artwork, novel, or user
  download    Download illustrations
  fanbox      Browse and download Pixiv FANBOX content
  follow      Manage followed users
  mcp         Run the MCP stdio server
  mypixiv     Browse authenticated MyPixiv data
  novel       Query Pixiv novels
  ranking     Show illustration ranking
  recommended Show personalized recommendations
  search      Search artworks, novels, or users
  series      List the artworks or novels in a series
  timeline    Browse authenticated Pixiv timelines
  update      Check for or install updates
  user        Query a Pixiv user

Flags:
  -h, --help      help for pixiv
  -v, --version   version for pixiv

Use "pixiv [command] --help" for more information about a command.
```

Exit code: 0

Verdict: PASS
