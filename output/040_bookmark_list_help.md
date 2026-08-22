# Inspect bookmark list syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv bookmark list --help
```

## Output
```text
List artwork or novel bookmarks

Usage:
  pixiv bookmark list [USER_ID] [flags]

Flags:
  -h, --help              help for list
  -j, --json              print JSON
  -l, --limit int         maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson            print one Pixiv entity record as JSON per line
      --no-proxy          clear the configured proxy for this command
  -p, --page int          1-based logical page (requires --limit > 0)
      --proxy string      proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string   bookmark visibility (public or private) (default "public")
      --tag string        filter by bookmark tag
  -t, --type string       entity type: artwork or novel (default "artwork")
```

Exit code: 0

Verdict: PASS
