# Inspect detail command syntax

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && ./bin/pixiv detail --help
```

## Output
```text
Show one artwork, novel, or user

Usage:
  pixiv detail ID_OR_URL [flags]

Flags:
      --content        for novels, read structured novel content instead of metadata
  -h, --help           help for detail
  -j, --json           print JSON
      --no-proxy       clear the configured proxy for this command
      --proxy string   proxy URL (http, https, socks5, or socks5h) for this command
  -t, --type string    entity type: artwork, novel, user (default "artwork")
```

Exit code: 0

Verdict: PASS
