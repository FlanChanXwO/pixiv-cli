# Invalid proxy scheme error

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv search miku --limit 1 --json --proxy ftp://127.0.0.1:7890
```

## Output
```text
error: proxy URL must use http, https, socks5, or socks5h: invalid proxy configuration
```

Exit code: 1

Verdict: FAIL
