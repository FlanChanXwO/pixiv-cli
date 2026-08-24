# Import exported Pixiv bundle from protected stdin into isolated state

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv auth import --json < /private/tmp/pixiv-cli-e2e-shell-20260821/credentials/pixiv-auth.bundle
```

## Output
```text
{
  "accounts": [
    {
      "user_id": 25649510,
      "username": "Flan",
      "status": "added"
    }
  ],
  "default_user_id": 25649510
}
```

Exit code: 0

Verdict: PASS
