# Inspect user follow add syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv user follow add --help
```

## Output
```text
Follow a user

Usage:
  pixiv user follow add [USER_ID] [flags]

Flags:
  -h, --help              help for add
      --no-proxy          clear the configured proxy for this command
      --on-error string   record failure strategy: skip or fail-fast (default "skip")
      --proxy string      proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string   follow visibility (public or private) (default "public")
```

Exit code: 0

Verdict: PASS
