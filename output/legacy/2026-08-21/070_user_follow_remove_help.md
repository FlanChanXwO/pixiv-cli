# Inspect user follow remove syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv user follow remove --help
```

## Output
```text
Unfollow a user

Usage:
  pixiv user follow remove [USER_ID] [flags]

Flags:
  -h, --help              help for remove
      --no-proxy          clear the configured proxy for this command
      --on-error string   record failure strategy: skip or fail-fast (default "skip")
      --proxy string      proxy URL (http, https, socks5, or socks5h) for this command
```

Exit code: 0

Verdict: PASS
