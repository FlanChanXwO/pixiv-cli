# Inspect bookmark add syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv bookmark add --help
```

## Output
```text
Bookmark an illustration

Usage:
  pixiv bookmark add [ILLUST_ID] [flags]

Flags:
  -h, --help              help for add
      --no-proxy          clear the configured proxy for this command
      --on-error string   record failure strategy: skip or fail-fast (default "skip")
      --proxy string      proxy URL (http, https, socks5, or socks5h) for this command
      --restrict string   bookmark visibility (public or private) (default "public")
      --tag stringArray   bookmark tag; may be repeated
```

Exit code: 0

Verdict: PASS
