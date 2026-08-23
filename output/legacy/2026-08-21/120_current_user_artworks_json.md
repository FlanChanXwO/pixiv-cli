# List current authenticated user artworks using omitted USER_ID

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv user artworks --type illust --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "illusts": [
  ]
}
```

Exit code: 0

Verdict: PASS
