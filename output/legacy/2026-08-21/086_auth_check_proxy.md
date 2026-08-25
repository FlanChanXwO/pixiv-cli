# Validate imported Pixiv account through explicit one-command proxy

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv auth check 25649510 --json --proxy http://127.0.0.1:7890
```

## Output
```text
{
  "user_id": 25649510,
  "username": "Flan",
  "default": false,
  "has_token": true
}
```

Exit code: 0

Verdict: PASS
