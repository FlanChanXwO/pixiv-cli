# List followers of a public Pixiv user

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv user followers 7621567 --restrict public --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "user_previews": [
  ]
}
```

Exit code: 0

Verdict: PASS
