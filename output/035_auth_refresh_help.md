# Inspect auth refresh syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv auth refresh --help
```

## Output
```text
Refresh account credentials and membership status

Usage:
  pixiv auth refresh [UID] [flags]

Flags:
      --all            refresh every stored account
  -h, --help           help for refresh
      --json           print JSON
      --no-proxy       clear the configured proxy for this command
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
