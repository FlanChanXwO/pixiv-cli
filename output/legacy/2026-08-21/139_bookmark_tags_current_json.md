# List current account Pixiv bookmark tags

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv bookmark tags --type artwork --restrict public --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "bookmark_tags": [
    {
      "name": "7月4日はフランちゃんの日",
      "count": 1
    },
    {
      "name": "M字騎乗位",
      "count": 1
    },
    {
      "name": "R-18",
      "count": 12
    }
  ]
}
```

Exit code: 0

Verdict: PASS
