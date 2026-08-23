# List users blocked by a public Pixiv user

## Command
```shell
cd /private/tmp/pixiv-cli-e2e-shell-20260821 && env HOME=/private/tmp/pixiv-cli-e2e-shell-20260821/home ./bin/pixiv user blocked 7621567 --limit 3 --json --proxy http://127.0.0.1:7890
```

## Output
```text
error: pixiv:UserBlockedUsers: not_found
```

Exit code: 1

Verdict: FAIL
