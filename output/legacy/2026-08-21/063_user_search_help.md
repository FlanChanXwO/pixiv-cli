# Inspect user search syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv user search --help
```

## Output
```text
Search users

Usage:
  pixiv user search WORD [flags]

Examples:
pixiv user search "miku" --json

Flags:
  -h, --help           help for search
  -j, --json           print JSON
  -l, --limit int      maximum results; omitted returns one upstream batch; 0 returns all results
      --ndjson         print one Pixiv entity record as JSON per line
      --no-proxy       clear the configured proxy for this command
  -p, --page int       1-based logical page (requires --limit > 0)
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
