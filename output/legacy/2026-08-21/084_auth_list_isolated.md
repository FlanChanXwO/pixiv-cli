# List imported account in isolated state

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv auth list --json
```

## Output
```text
{
  "default_user_id": 25649510,
  "accounts": [
    {
      "user_id": 25649510,
      "username": "Flan",
      "default": true,
      "has_token": true,
      "schedulable": true,
      "eligible": true
    }
  ]
}
```

Exit code: 0

Verdict: PASS
