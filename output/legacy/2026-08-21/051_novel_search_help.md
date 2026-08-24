# Inspect novel search syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv novel search --help
```

## Output
```text
Search novels

Usage:
  pixiv novel search WORD [flags]

Examples:
pixiv novel search "miku" --json

Flags:
  -h, --help               help for search
  -j, --json               print JSON
  -l, --limit int          maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson             print one Pixiv entity record as JSON per line
      --no-proxy           clear the configured proxy for this command
  -p, --page int           1-based logical page (requires --limit > 0)
      --period string      time range: day, week, month
      --proxy string       proxy URL (http, https, socks5, or socks5h) for this command
      --search-by string   search field: tag-partial, tag-exact, title-caption (default "tag-partial")
      --sort string        sort mode: date_desc, date_asc (default "date_desc")
```

Exit code: 0

Verdict: PASS
