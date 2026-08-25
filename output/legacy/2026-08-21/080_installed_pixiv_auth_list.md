# Probe installed pixiv CLI account list without exposing credentials

## Command
```shell
command -v pixiv; pixiv --version; pixiv auth list --json
```

## Output
```text
/Users/flanchan/.local/bin/pixiv
pixiv v0.10.0
{
  "default_user_id": 25649510,
  "accounts": [
    {
      "user_id": 25649510,
      "username": "Flan",
      "default": true,
      "has_token": true
    }
  ]
}
```

Exit code: 0

Verdict: PASS
