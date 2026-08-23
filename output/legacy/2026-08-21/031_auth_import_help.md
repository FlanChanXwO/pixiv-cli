# Inspect auth import syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv auth import --help
```

## Output
```text
Import or replace an account

Usage:
  pixiv auth import [REFRESH_TOKEN] [flags]

Examples:
pixiv auth import YOUR_REFRESH_TOKEN

Flags:
  -h, --help           help for import
      --json           print JSON
      --no-proxy       clear the configured proxy for this command
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
