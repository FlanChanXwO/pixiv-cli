# Inspect timeline following syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv timeline following --help
```

## Output
```text
Browse new works from followed users

Usage:
  pixiv timeline following [flags]

Flags:
      --content-type string   artwork subtype: all, illust-and-ugoira, illust, manga, ugoira (default "all")
  -h, --help                  help for following
  -j, --json                  print JSON
  -l, --limit int             maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson                print one Pixiv entity record as JSON per line
      --no-proxy              clear the configured proxy for this command
  -p, --page int              1-based logical page (requires --limit > 0)
      --proxy string          proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string       follow visibility: public or private (default "public")
  -t, --type string           required entity type: artwork or novel
```

Exit code: 0

Verdict: PASS
